#!/usr/bin/env python3
"""Spike S1: PDF parse driver.

Runs PyMuPDF (fitz) for text extraction + heading inference, pdfplumber
for tables, Tesseract for OCR on image-only pages. Emits a single JSON
file in the ParsedDoc-like shape that M1.1 bookparser PDF backend would
target — minus the Go types.

This is throwaway evaluation code. Do NOT import in production.
"""

from __future__ import annotations

import argparse
import json
import statistics
import sys
import time
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any

try:
    import fitz  # PyMuPDF
except ImportError:
    print("ERROR: install PyMuPDF — pip install pymupdf", file=sys.stderr)
    sys.exit(2)

try:
    import pdfplumber  # type: ignore
except ImportError:
    print("ERROR: install pdfplumber — pip install pdfplumber", file=sys.stderr)
    sys.exit(2)

try:
    import pytesseract  # type: ignore
    from PIL import Image  # type: ignore
except ImportError:
    pytesseract = None  # OCR optional but warned about


@dataclass
class Block:
    type: str
    text: str = ""
    page: int = 0
    heading_inferred: bool = False
    heading_confidence: float = 0.0
    heading_path: list[str] = field(default_factory=list)
    font_size: float | None = None
    caption: str | None = None
    ocr_text: str | None = None
    text_markdown: str | None = None
    rows: int | None = None
    cols: int | None = None


def main() -> None:
    ap = argparse.ArgumentParser(description="S1 PDF parser spike")
    ap.add_argument("pdf", type=Path, help="Path to the textbook PDF")
    ap.add_argument("--out", type=Path, default=Path("parsed.json"), help="Output JSON path")
    ap.add_argument("--no-ocr", action="store_true", help="Skip OCR on scanned pages")
    ap.add_argument("--max-pages", type=int, default=0, help="Limit to first N pages (debug)")
    args = ap.parse_args()

    if not args.pdf.is_file():
        sys.exit(f"PDF not found: {args.pdf}")

    start = time.time()
    blocks: list[Block] = []
    ocr_pages: list[int] = []

    doc = fitz.open(args.pdf)
    page_limit = args.max_pages or doc.page_count

    # Pass 1: collect font sizes to derive heading thresholds.
    sizes: list[float] = []
    for pn in range(min(doc.page_count, page_limit)):
        page = doc[pn]
        for block in page.get_text("dict")["blocks"]:
            for line in block.get("lines", []):
                for span in line.get("spans", []):
                    sizes.append(span["size"])
    if not sizes:
        sys.exit("No text spans found — is this a fully scanned PDF?")
    median = statistics.median(sizes)
    p90 = sorted(sizes)[int(len(sizes) * 0.9)]
    heading_threshold = max(median * 1.15, p90 - 1.0)
    print(f"[stats] median font {median:.1f}, p90 {p90:.1f} → heading >= {heading_threshold:.1f}", file=sys.stderr)

    # Pass 2: walk pages and emit blocks.
    heading_stack: list[str] = []  # top-down: chapter, section, subsection
    for pn in range(min(doc.page_count, page_limit)):
        page = doc[pn]
        text = page.get_text("text").strip()
        if not text:
            # Likely a scanned page; try OCR.
            if not args.no_ocr and pytesseract is not None:
                ocr_pages.append(pn + 1)
                pix = page.get_pixmap(dpi=200)
                img = Image.frombytes("RGB", (pix.width, pix.height), pix.samples)
                ocr_text = pytesseract.image_to_string(img, lang="chi_sim+eng")
                if ocr_text.strip():
                    blocks.append(Block(type="paragraph", text=ocr_text.strip(), page=pn + 1,
                                        heading_inferred=False, heading_confidence=0.0,
                                        ocr_text=ocr_text.strip()))
            continue

        page_dict = page.get_text("dict")
        for raw_block in page_dict.get("blocks", []):
            if raw_block.get("type", 0) == 1:
                # Image block in dict mode
                # Try to grab nearby caption (the next text line).
                blocks.append(Block(type="image", page=pn + 1, caption=None))
                continue
            buf: list[str] = []
            block_max_size = 0.0
            for line in raw_block.get("lines", []):
                line_text_parts: list[str] = []
                line_max = 0.0
                for span in line.get("spans", []):
                    line_text_parts.append(span["text"])
                    line_max = max(line_max, span["size"])
                line_text = "".join(line_text_parts).strip()
                if line_text:
                    buf.append(line_text)
                block_max_size = max(block_max_size, line_max)
            block_text = " ".join(buf).strip()
            if not block_text:
                continue
            is_heading = block_max_size >= heading_threshold and len(block_text) <= 50
            if is_heading:
                # Heuristic depth: bigger font = shallower depth.
                depth = max(0, min(len(heading_stack), int((p90 - block_max_size) // 2)))
                heading_stack = heading_stack[:depth] + [block_text]
                confidence = min(1.0, (block_max_size - median) / max(p90 - median, 0.1))
                blocks.append(Block(type="heading", text=block_text, page=pn + 1,
                                    heading_inferred=True, heading_confidence=float(confidence),
                                    font_size=block_max_size, heading_path=list(heading_stack)))
            else:
                blocks.append(Block(type="paragraph", text=block_text, page=pn + 1,
                                    heading_inferred=False, heading_confidence=0.0,
                                    font_size=block_max_size, heading_path=list(heading_stack)))

    doc.close()

    # Pass 3: tables via pdfplumber.
    with pdfplumber.open(args.pdf) as pp:
        for pn, page in enumerate(pp.pages):
            if args.max_pages and pn >= args.max_pages:
                break
            for tbl in page.extract_tables() or []:
                if not tbl or not tbl[0]:
                    continue
                rows = len(tbl)
                cols = max(len(r) for r in tbl)
                md = table_to_markdown(tbl)
                blocks.append(Block(type="table", page=pn + 1, text_markdown=md, rows=rows, cols=cols))

    elapsed = time.time() - start
    meta = {
        "page_count": min(doc.page_count, page_limit) if False else page_limit,  # placeholder
        "total_blocks": len(blocks),
        "ocr_pages": ocr_pages,
        "elapsed_s": round(elapsed, 1),
        "heading_threshold": round(heading_threshold, 2),
        "font_median": round(median, 2),
        "font_p90": round(p90, 2),
    }
    # Fix: page_count
    meta["page_count"] = page_limit

    payload: dict[str, Any] = {"meta": meta, "blocks": [asdict(b) for b in blocks]}
    args.out.write_text(json.dumps(payload, ensure_ascii=False, indent=2))
    print(f"[done] {len(blocks)} blocks → {args.out} in {elapsed:.1f}s")


def table_to_markdown(rows: list[list[str | None]]) -> str:
    if not rows:
        return ""
    header = [c or "" for c in rows[0]]
    width = len(header)
    out = ["| " + " | ".join(header) + " |", "|" + "|".join(["---"] * width) + "|"]
    for r in rows[1:]:
        cells = [(c or "").replace("\n", " ").strip() for c in r]
        # pad to width
        cells += [""] * (width - len(cells))
        out.append("| " + " | ".join(cells) + " |")
    return "\n".join(out)


if __name__ == "__main__":
    main()
