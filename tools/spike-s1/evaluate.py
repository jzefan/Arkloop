#!/usr/bin/env python3
"""Spike S1: Interactive evaluator.

Loads parsed.json + rubric.yaml, then walks each sample asking the
operator (you) to score it. Aggregates and prints a report ready to
paste into docs/superpowers/specs/...-design.md S1 section.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any

try:
    import yaml  # type: ignore
except ImportError:
    print("pip install pyyaml", file=sys.stderr)
    sys.exit(2)


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("parsed_json", type=Path)
    ap.add_argument("pdf", type=Path, help="The original PDF, for cross-reference (not parsed here)")
    ap.add_argument("--rubric", type=Path, default=Path(__file__).parent / "rubric.yaml")
    args = ap.parse_args()

    parsed = json.loads(args.parsed_json.read_text())
    rubric = yaml.safe_load(args.rubric.read_text())

    print(f"=== Spike S1 Evaluation Report ===\nFile: {args.pdf.name} ({parsed['meta']['page_count']} pages)\n")

    heading_score = score_heading(parsed, rubric.get("heading_detection", {}))
    multicol_score = score_multicolumn(parsed, rubric.get("multi_column_pages", {}))
    ocr_score = score_ocr(parsed, rubric.get("ocr_pages", {}), args.pdf)
    image_score = score_images(parsed, rubric.get("images_with_captions", {}))
    table_score = score_tables(parsed, rubric.get("tables", {}))
    formula_score = score_formulas(parsed, rubric.get("formulas", {}))

    print(f"\n[Elapsed]\n  {parsed['meta']['elapsed_s']}s for {parsed['meta']['page_count']} pages "
          f"= {parsed['meta']['page_count'] / max(parsed['meta']['elapsed_s'], 0.1):.1f} pages/s\n")

    print("=== Recommendation ===")
    print(recommendation(heading_score, multicol_score, ocr_score, image_score, table_score, formula_score))


def score_heading(parsed: dict[str, Any], rubric: dict[str, Any]) -> dict[str, int]:
    samples = rubric.get("samples", [])
    if not samples:
        return {"total": 0, "type_ok": 0, "path_ok": 0}

    print("[Heading detection]")
    blocks_by_page: dict[int, list[dict]] = {}
    for b in parsed["blocks"]:
        blocks_by_page.setdefault(b["page"], []).append(b)

    type_ok = 0
    path_ok = 0
    for s in samples:
        page = s["page"]
        expected = s["expected_text"]
        nearby = blocks_by_page.get(page, [])
        match = next((b for b in nearby if expected[:8] in b["text"]), None)
        if match is None:
            print(f"  p{page} '{expected}': NOT FOUND in parsed blocks", file=sys.stderr)
            continue
        is_typed = match["type"] == "heading"
        path_match = match.get("heading_path") == s.get("expected_path", [])
        if is_typed:
            type_ok += 1
        if path_match:
            path_ok += 1
        marker_type = "✓" if is_typed else "✗"
        marker_path = "✓" if path_match else "✗"
        print(f"  p{page} '{expected[:30]}…' type{marker_type} path{marker_path}")

    print(f"  Correctly typed as heading: {type_ok}/{len(samples)} = {pct(type_ok, len(samples))}")
    print(f"  Correct heading_path:        {path_ok}/{len(samples)} = {pct(path_ok, len(samples))}\n")
    return {"total": len(samples), "type_ok": type_ok, "path_ok": path_ok}


def score_multicolumn(parsed: dict[str, Any], rubric: dict[str, Any]) -> dict[str, int]:
    samples = rubric.get("samples", [])
    if not samples:
        return {"total": 0, "ok": 0}
    print("[Text extraction order (multi-column)]")
    print("  This is a manual check. For each page below, open the PDF + parsed.json side by side")
    print("  and decide whether the parsed paragraphs read in source order.\n")
    ok = 0
    for s in samples:
        ans = input(f"  p{s['page']} ({s.get('description', '')}): reading order correct? [y/n] ").strip().lower()
        if ans == "y":
            ok += 1
    print(f"  Correct reading order: {ok}/{len(samples)} = {pct(ok, len(samples))}\n")
    return {"total": len(samples), "ok": ok}


def score_ocr(parsed: dict[str, Any], rubric: dict[str, Any], pdf: Path) -> dict[str, int]:
    samples = rubric.get("samples", [])
    if not samples:
        print("[OCR (scanned pages)]\n  No scanned pages in rubric.\n")
        return {"total": 0, "ok": 0}
    print("[OCR (scanned pages)]")
    print("  Manual check: for each scanned page, open the PDF and the OCR text in parsed.json,")
    print("  estimate char-level accuracy (close enough — count obvious garbled chars per ~200).\n")
    ok = 0
    for s in samples:
        ans = input(f"  p{s['page']} acceptable for retrieval-grade indexing? [y/n] ").strip().lower()
        if ans == "y":
            ok += 1
    print(f"  Acceptable OCR pages: {ok}/{len(samples)}\n")
    return {"total": len(samples), "ok": ok}


def score_images(parsed: dict[str, Any], rubric: dict[str, Any]) -> dict[str, int]:
    samples = rubric.get("samples", [])
    if not samples:
        return {"total": 0, "ok": 0}
    print("[Image extraction]")
    image_pages = {b["page"] for b in parsed["blocks"] if b["type"] == "image"}
    ok = 0
    for s in samples:
        present = s["page"] in image_pages
        marker = "✓" if present else "✗"
        print(f"  p{s['page']} '{s['expected_caption_contains']}' {marker}")
        if present:
            ok += 1
    print(f"  Images detected: {ok}/{len(samples)} = {pct(ok, len(samples))}\n")
    return {"total": len(samples), "ok": ok}


def score_tables(parsed: dict[str, Any], rubric: dict[str, Any]) -> dict[str, int]:
    samples = rubric.get("samples", [])
    if not samples:
        return {"total": 0, "ok": 0}
    print("[Table extraction]")
    tables_by_page = {b["page"]: b for b in parsed["blocks"] if b["type"] == "table"}
    ok = 0
    for s in samples:
        t = tables_by_page.get(s["page"])
        if t is None:
            print(f"  p{s['page']}: NOT FOUND")
            continue
        exp = s["expected_dims"]
        size_ok = abs(t.get("rows", 0) - exp["rows"]) <= 1 and abs(t.get("cols", 0) - exp["cols"]) <= 1
        marker = "✓" if size_ok else "✗"
        print(f"  p{s['page']} rows {t.get('rows')}/{exp['rows']} cols {t.get('cols')}/{exp['cols']} {marker}")
        if size_ok:
            ok += 1
    print(f"  Tables extracted: {ok}/{len(samples)} = {pct(ok, len(samples))}\n")
    return {"total": len(samples), "ok": ok}


def score_formulas(parsed: dict[str, Any], rubric: dict[str, Any]) -> dict[str, int]:
    samples = rubric.get("samples", [])
    if not samples:
        return {"total": 0, "ok": 0}
    print("[Formula extraction]")
    print("  PyMuPDF outputs formulas as plain text. Check whether the symbols survive.\n")
    blocks_by_page: dict[int, list[dict]] = {}
    for b in parsed["blocks"]:
        blocks_by_page.setdefault(b["page"], []).append(b)
    ok = 0
    for s in samples:
        expected = s["expected_text"]
        nearby = blocks_by_page.get(s["page"], [])
        found = any(expected[:4] in (b["text"] or "") for b in nearby)
        marker = "✓" if found else "✗"
        print(f"  p{s['page']} '{expected}' {marker}")
        if found:
            ok += 1
    print(f"  Formulas surviving as text: {ok}/{len(samples)} = {pct(ok, len(samples))}\n")
    return {"total": len(samples), "ok": ok}


def pct(n: int, d: int) -> str:
    if d == 0:
        return "n/a"
    return f"{n * 100 // d}%"


def recommendation(h, mc, ocr, img, tbl, fm) -> str:
    lines: list[str] = []
    if h["total"] and h["type_ok"] / h["total"] < 0.7:
        lines.append("- Heading detection below 70%; require the chunker fallback "
                     "(heading_inferred=false → sequential chunks) to kick in often. "
                     "Track in M1.1 logs.")
    if mc["total"] and mc["ok"] / mc["total"] < 0.7:
        lines.append("- Multi-column reading order unreliable; M1.1 should treat ambiguous "
                     "spreads as single-block-per-page rather than risk scrambled chunks.")
    if ocr["total"] and ocr["ok"] / ocr["total"] < 0.7:
        lines.append("- Tesseract OCR quality marginal; **evaluate PaddleOCR (chinese_cht / chinese_sim)** "
                     "before M1.1 commits an OCR backend.")
    else:
        lines.append("- Tesseract OCR acceptable for M1.1; revisit only if user complaints surface.")
    if fm["total"] and fm["ok"] / fm["total"] >= 0.7:
        lines.append("- Formula text survives PyMuPDF extraction; ship as plain `chunk_type=formula` "
                     "in M1.1, do NOT block on LaTeX conversion.")
    else:
        lines.append("- Formula extraction unreliable; document as a known M1.1 limitation; "
                     "deferred LaTeX work goes to v2.")
    if not lines:
        lines.append("- Pipeline quality acceptable across all dimensions. Proceed with M1.1 as designed.")
    return "\n".join(lines)


if __name__ == "__main__":
    main()
