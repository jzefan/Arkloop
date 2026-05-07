---
name: Blob URL orphaned when component unmounts mid-fetch (URL.createObjectURL called before cancelled check)
description: If fetch resolves after cancelled=true, URL.createObjectURL is called but its return value is discarded — blob URL is never revoked, causing a memory leak
type: pending
priority: p2
issue_id: "006"
tags: [code-review, correctness]
dependencies: []
---

## Problem Statement

In `ArtifactVideo.tsx`, the `cancelled` flag prevents `setBlobUrl` from being called after unmount, but `URL.createObjectURL(blob)` is called on the line before the guard. If the component unmounts between the `.then(blob => {` invocation and the `if (!cancelled)` check, the blob URL is created but never assigned to state and never revoked — an orphaned blob URL that leaks memory.

## Findings

Current code pattern:
```typescript
.then((blob) => {
  if (!cancelled) setBlobUrl(URL.createObjectURL(blob))  // line 25
})
```

The bug: `URL.createObjectURL(blob)` is evaluated as part of the argument to `setBlobUrl`. In JavaScript, the argument is evaluated before the function call. So the sequence is:
1. Evaluate `URL.createObjectURL(blob)` → creates blob URL, registers it in the browser
2. Evaluate `!cancelled` → if `true`, call `setBlobUrl(url)` (stored, will be revoked later)
3. If `!cancelled` is `false` → `setBlobUrl` is never called, but the blob URL created in step 1 is already registered and never revoked

The second `useEffect` (lines 37-39) only revokes `blobUrl` state — it cannot revoke the orphaned URL from step 1 because it was never assigned to state.

**File:** `src/apps/web/src/components/ArtifactVideo.tsx:24-26`

Note: The same pattern likely exists in `ArtifactImage.tsx`.

## Proposed Solutions

### Option A — Conditional creation (Recommended)

Move `createObjectURL` inside the guard:

```typescript
.then((blob) => {
  if (!cancelled) {
    setBlobUrl(URL.createObjectURL(blob))
  }
})
```

- **Pros:** Correct. Simple one-line fix. No API change.
- **Cons:** None.
- **Effort:** Trivial
- **Risk:** None

### Option B — Track and revoke in cleanup

Create the blob URL eagerly but store a ref to it for cleanup:

```typescript
let createdUrl: string | null = null
.then((blob) => {
  createdUrl = URL.createObjectURL(blob)
  if (!cancelled) setBlobUrl(createdUrl)
  else URL.revokeObjectURL(createdUrl)
})
```

- **Pros:** Handles the race correctly.
- **Cons:** More complex than Option A for the same result.
- **Effort:** Small
- **Risk:** Low

## Recommended Action

Option A — it's the obvious correct fix.

## Technical Details

- File: `src/apps/web/src/components/ArtifactVideo.tsx:24-26`
- Same pattern in: `src/apps/web/src/components/ArtifactImage.tsx` — check and fix both

## Acceptance Criteria

- [ ] `URL.createObjectURL` is only called when `!cancelled`
- [ ] Same fix applied to `ArtifactImage.tsx` if affected
- [ ] Test: component that unmounts during in-flight fetch leaves no orphaned blob URLs

## Work Log

- 2026-05-06: Identified by correctness review agent (F1).
