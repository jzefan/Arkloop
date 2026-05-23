# Book KB RAG M1.1 Implementation Plan

## Goal

Add multi-format textbook ingestion to the M1.0 KB RAG flow. M1.1 supports PDF, DOCX, and image uploads, extracts structured blocks through a Python parser runner, preserves parse/chunk metadata, and exposes mixed chunk types in search and console-lite.

## Inputs

- PRD/design: `docs/superpowers/specs/2026-05-23-book-kb-rag-m1.1-design.md`
- Spike S1 notes: `docs/superpowers/specs/2026-05-21-book-kb-rag-design.md`
- Eval sample: `tools/spike-s1/sample_outputs/fuchan-10-eval.txt`
- M1.0 implementation on `main`

## Acceptance Checklist

1. `.txt` and `.md` continue to parse via `TextOnlyParser`.
2. `.pdf` upload is accepted and routed to the sandbox parser runner.
3. `.docx` upload is accepted and routed to the sandbox parser runner.
4. `.png`, `.jpg`, `.jpeg`, and `.webp` uploads are accepted and routed to the sandbox parser runner.
5. Unsupported formats return `ErrUnsupportedMime` / HTTP 415.
6. Image blocks smaller than 32 px, smaller than 4096 bytes, or on pages with more than 50 image candidates are filtered.
7. Parser output records `heading_inferred_ratio`; chunking drops inferred heading paths when the ratio is below 0.5.
8. Table/image/formula blocks are emitted as independent chunks with metadata.
9. `kb_documents.parse_meta_json` and `kb_chunks.metadata_json` are persisted during ingest.
10. REST `GET /search` and worker `kb_search` return chunk metadata for non-text hits.
11. console-lite upload, document summary, and search results expose the new formats and chunk types.

## Tasks

### Task 1: Parser Dispatcher

- Add `bookparser.MultiFormatParser`.
- Keep `text/plain` and `text/markdown` on `TextOnlyParser`.
- Route PDF/DOCX/images to `SandboxParser`.
- Normalize common MIME aliases and file-extension-derived MIME values.
- Add tests for supported routing and unsupported MIME errors.

### Task 2: Sandbox Parser

- Add `src/services/shared/bookparser/sandbox.go`.
- Define a small `Runner` interface that takes MIME, bytes, and max pages, returning JSON stdout.
- Implement a local subprocess runner for production wiring with timeout and stderr diagnostics.
- Decode runner JSON into `ParsedDoc`.
- Keep tests hermetic by using fake runners.

### Task 3: Python Runner

- Add `src/services/shared/bookparser/sandbox/bookparser_runner.py`.
- Implement CLI contract:
  `bookparser_runner.py --mime application/pdf --max-pages 0 < blob.bin > out.json`
- Support PyMuPDF/pdfplumber/pytesseract for PDF, python-docx for DOCX, and Pillow/pytesseract for image OCR.
- Emit `meta` and `blocks` matching the Go `ParsedDoc` contract.
- Apply image filtering constants from the M1.1 design.

### Task 4: Chunk Metadata

- Add `Metadata map[string]any` to `bookchunker.TextChunk`.
- Copy block metadata into all chunks, including split paragraph chunks.
- Add heading fallback based on document meta `heading_inferred_ratio < 0.5`.
- Add tests for metadata preservation, special block chunking, and heading fallback.

### Task 5: Worker Ingest

- Allow `kbingest.Processor` to accept a parser override while defaulting to `MultiFormatParser`.
- Persist parser meta through document status updates.
- Persist chunk metadata into `kb_chunks.metadata_json`.
- Add tests proving parse meta and image/table metadata reach repository rows.

### Task 6: Data/Search Surfaces

- Extend API and worker chunk repositories with metadata JSON marshal/scan.
- Return `metadata` from REST search and worker `kb_search`.
- Keep existing score/order behavior unchanged.

### Task 7: Upload API

- Raise default KB upload limit to 100 MiB.
- Extend extension-to-MIME mapping to PDF/DOCX/images.
- Update upload error copy from M1.0 text-only wording.
- Add API handler tests for PDF/DOCX/image acceptance and unsupported rejection.

### Task 8: Console-Lite UX

- Extend file picker accepted types.
- Show parse metadata and block-type counts in document rows.
- Add type icons/labels for image/table/formula search hits.
- Update locale strings and TypeScript API types.

### Task 9: Persona Prompt

- Update `book-tutor-agent` prompt so it understands `kb_search` may return image/table/formula chunks and should cite them clearly.

### Task 10: Verification

- Run focused Go unit tests for `bookparser`, `bookchunker`, `kbingest`, `kbapi`, data repos, and `kb_search`.
- Run console-lite type-check/lint/build.
- Run a small runner smoke check where local Python dependencies are present; otherwise verify Go runner contract with fake-runner tests and document the dependency gap.

