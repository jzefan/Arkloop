---
name: PDF artifact output has display:inline but Task 6 expects download rendering
description: Task 2's executor output shape sets "display":"inline"; Task 6 checks if PDF renders as download. These contradict each other.
type: plan-correction
priority: p1
issue_id: "004"
tags: [plan-review, pdf, artifact, task-2, task-6]
---

## Problem Statement

Task 2 Step 3 specifies the executor output shape:
```json
{ "display": "inline" }
```

Task 6 Step 2 tests that PDF artifacts "appear as a downloadable artifact row and does not try to inline-render as HTML/SVG/Markdown."

**These are directly contradictory.** If `display: "inline"` is returned, the artifact system may attempt inline rendering. If the goal is download-only (correct for PDF), the output should use `display: "download"` or leave the field empty.

Note from feasibility review: `DocumentPanel.canPreviewDocumentAsText` already returns `false` for `application/pdf` — so the existing code already handles PDF as binary/download regardless of the `display` field. But the spec should not say `inline` when the intent is `download`.

## Findings

- Task 2 Step 3 output shape example: `"display": "inline"`
- Task 6 Step 2 expected behavior: render as downloadable, not inline
- `DocumentPanel.tsx` line 60-72: `canPreviewDocumentAsText` returns `false` for `application/pdf`
- `document_write` uses `"display": "inline"` for Markdown files (which ARE inline-rendered in the document panel)

## Proposed Solutions

### Option A: Change display to "download" in the spec
Update Task 2 Step 3 output shape to `"display": "download"`. This matches intent and eliminates confusion.

### Option B: Omit the display field and rely on MIME type routing
Don't include `display` in the output. The frontend already routes `application/pdf` to download via MIME type. Less explicit but works.

**Recommended: Option A** — explicit and correct.

## Acceptance Criteria

- [ ] Task 2 executor output spec uses `"display": "download"` for PDF artifacts
- [ ] Task 6 check confirms no frontend code change needed (existing binary fallback handles it)

## Work Log

- 2026-05-07: Identified by coherence reviewer
