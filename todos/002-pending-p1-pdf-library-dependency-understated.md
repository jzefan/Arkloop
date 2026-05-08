---
name: PDF library dependency is non-trivial and completely absent from go.mod
description: No PDF or CJK font library exists in any go.mod. "One minimal Go dependency" claim understates what formal A4+Chinese+links requires.
type: plan-correction
priority: p1
issue_id: "002"
tags: [plan-review, markdown-to-pdf, task-2, dependency]
---

## Problem Statement

Task 2 says "inspect existing Go dependencies; if absent, add one minimal Go dependency." The inspection result is: **zero PDF-related dependencies exist** in the worker's `go.mod`. The only font code is `golang.org/x/image` with `basicfont` and `inconsolata` — bitmap/monospace fonts that cannot render Chinese characters.

Producing a formal A4 PDF with:
- CJK character rendering (requires embedded TrueType font, ~10-20 MB)
- Page numbers (requires manual page tracking)
- Clickable hyperlinks (requires explicit PDF annotation API)
- Table layout and heading hierarchy

...requires a non-trivial PDF library. No existing lightweight Go library handles all four requirements out of the box. The "minimal dependency" framing will lead the implementer to pick an inadequate library and discover the gaps mid-implementation.

## Findings

- Worker `go.mod` has no `gofpdf`, `maroto`, `go-pdf`, `chromedp`, or any PDF library
- `golang.org/x/image` is present but only supports raster images, not PDF
- CJK font embedding requires a bundled `.ttf` file (Source Han Sans, Noto CJK, or similar)
- Clickable links in pure-Go PDF require explicit annotation calls — most lightweight libraries lack or poorly document this
- `jung-kurt/gofpdf` (most common Go PDF library) is archived/unmaintained; `go-pdf/fpdf` is the maintained fork

## Proposed Solutions

### Option A: Accept full PDF dependency (go-pdf/fpdf + CJK font)
Add `github.com/go-pdf/fpdf` and bundle a CJK TrueType font. Implement full Markdown→PDF rendering layer from scratch. Estimated: 3-5 days of non-trivial work. Output quality will be "functional but not beautiful."

### Option B: Reduce PDF scope to "structured text PDF" (no complex layout)
Use a simple PDF text library that handles UTF-8 and basic layout. Accept that the PDF won't have tables or pixel-perfect A4 styling. Matches the graceful-degradation clause in the plan ("PDF failure degrades to Markdown-only"), since a simple PDF is better than nothing.

### Option C: Defer PDF to post-MVP; deliver Markdown only for v1
The plan already has graceful degradation to Markdown. `document_write` already supports `.md` artifacts and they download correctly. Ship v1 with Markdown-only output, add PDF in v2 once the library choice is validated. The entire Task 2 (7 files) becomes unnecessary for v1.

**Recommended: Option C for v1**, Option A for v2 with explicit library evaluation time budgeted.

## Acceptance Criteria

- [ ] Plan updated with explicit library choice (not "inspect and decide")
- [ ] Binary size impact of CJK font embedding documented
- [ ] OR: Task 2 deferred to v2 with Markdown-only v1 output

## Work Log

- 2026-05-07: Identified by feasibility reviewer and scope guardian
