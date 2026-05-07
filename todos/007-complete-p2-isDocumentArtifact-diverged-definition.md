---
name: isDocumentArtifact defined in two places with diverging implementations
description: messagebubble/utils.ts excludes image/ and text/html but NOT video/; MarkdownRenderer.tsx correctly excludes video/ — video artifacts classified as documents via the stale utils.ts path
type: pending
priority: p2
issue_id: "007"
tags: [code-review, correctness, maintainability]
dependencies: []
---

## Problem Statement

`isDocumentArtifact` is defined twice with different implementations. The version in `messagebubble/utils.ts` does not exclude `video/` MIME types, so a video artifact evaluated through that path would be classified as a document — leading to incorrect rendering or display.

## Findings

**Stale definition** (missing `video/` exclusion):
```typescript
// src/apps/web/src/components/messagebubble/utils.ts:5-7
export function isDocumentArtifact(artifact: ArtifactRef): boolean {
  return !artifact.mime_type.startsWith('image/') && artifact.mime_type !== 'text/html'
}
```

**Correct definition** (includes `video/` exclusion):
```typescript
// src/apps/web/src/components/MarkdownRenderer.tsx:36-39
function isDocumentArtifact(artifact: ArtifactRef): boolean {
  return !artifact.mime_type.startsWith('image/')
    && !artifact.mime_type.startsWith('video/')
    && artifact.mime_type !== 'text/html'
}
```

Any component importing from `messagebubble/utils.ts` will treat video artifacts as documents.

## Proposed Solutions

### Option A — Consolidate to a shared artifact utility (Recommended)

Create `src/apps/web/src/utils/artifact-utils.ts` with a single canonical `isDocumentArtifact`. Delete both current definitions and import from the shared location.

- **Pros:** One definition, impossible to diverge.
- **Cons:** Requires updating import paths in both files and any other consumers.
- **Effort:** Small
- **Risk:** Low

### Option B — Fix utils.ts and make MarkdownRenderer import it

Update `messagebubble/utils.ts` to include the `video/` exclusion. Remove the local redefinition from `MarkdownRenderer.tsx` and import from utils.ts.

- **Pros:** Smaller change surface.
- **Cons:** utils.ts might not be the right canonical location semantically.
- **Effort:** Small
- **Risk:** Low

## Recommended Action

Option A for long-term hygiene; Option B if a quick fix is preferred.

## Technical Details

- File: `src/apps/web/src/components/messagebubble/utils.ts:5-7`
- File: `src/apps/web/src/components/MarkdownRenderer.tsx:36-39`

## Acceptance Criteria

- [ ] `isDocumentArtifact` exists in exactly one location
- [ ] The single definition excludes `image/`, `video/`, and `text/html`
- [ ] All consumers import from the canonical location
- [ ] Test: `video/mp4` MIME type returns `false` from `isDocumentArtifact`

## Work Log

- 2026-05-06: Identified by maintainability review agent (MAINT-007).
