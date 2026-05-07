---
name: Frontend fetches entire video as Blob before playback — no streaming, full video in JS heap
description: ArtifactVideo.tsx buffers 100s of MB in the browser before showing a single frame; preload="metadata" is ineffective on a pre-downloaded blob URL
type: pending
priority: p2
issue_id: "008"
tags: [code-review, performance]
dependencies: []
---

## Problem Statement

`ArtifactVideo.tsx` calls `fetch(url).then(res => res.blob())` and creates an object URL from the complete blob before setting `src` on the `<video>` element. On slow connections, a 100 MB video takes tens of seconds before any frame is visible. The full video is also copied into the JS heap, doubling memory relative to browser-native streaming.

## Findings

```typescript
// ArtifactVideo.tsx:18-26
fetch(url, { headers: { Authorization: `Bearer ${accessToken}` } })
  .then((res) => { ... return res.blob() })
  .then((blob) => {
    if (!cancelled) setBlobUrl(URL.createObjectURL(blob))
  })
```

- `preload="metadata"` at line 62 has no effect because `src` is a blob URL pointing to already-fully-downloaded data
- The `Authorization` header is the reason a direct `<video src={url}>` is not used

**Performance cost:**
1. User sees shimmer for the entire download duration (potentially minutes on slow connections)
2. Full video is in memory twice during the `fetch → blob → objectURL` pipeline
3. No progress feedback to the user while downloading

## Proposed Solutions

### Option A — Server-side time-limited signed URL (Recommended)

Generate a short-lived signed URL on the backend for authenticated artifact access. The frontend sets `src={signedUrl}` directly — the browser handles range requests, seeking, and progressive buffering natively.

- **Pros:** Native browser streaming. Seeking works. No auth header needed. Zero frontend changes beyond a different `src`.
- **Cons:** Requires backend endpoint for signed URL generation. Token expiry must be managed.
- **Effort:** Medium
- **Risk:** Low

### Option B — Proxy endpoint with Content-Range support

Add a server-side video proxy that validates the Bearer token and forwards range requests to the object store. Set `<video src={proxyUrl}>` with the Bearer token as a cookie or query param.

- **Pros:** Progressive download. Seeking works.
- **Cons:** More complex than Option A. Proxy must handle byte-range passthrough.
- **Effort:** Medium-Large
- **Risk:** Medium

### Option C — ReadableStream download progress indicator (Near-term improvement)

Keep the blob approach but use `ReadableStream` to show a download progress bar:

```typescript
const reader = res.body!.getReader()
// ... accumulate chunks, report progress, then createObjectURL
```

- **Pros:** Quick improvement. No backend changes.
- **Cons:** Still buffers full video. Does not fix seeking after blob is created.
- **Effort:** Small
- **Risk:** Low

## Recommended Action

Option A for a complete fix; Option C as a near-term UX improvement while Option A is built.

## Technical Details

- File: `src/apps/web/src/components/ArtifactVideo.tsx:18-26, 62`

## Acceptance Criteria

- [ ] Video playback starts within 2 seconds of opening (first frame visible before full download)
- [ ] Large videos (>50 MB) do not cause noticeable JS heap pressure in browser DevTools
- [ ] OR: download progress indicator is shown while buffering (Option C fallback)

## Work Log

- 2026-05-06: Identified by performance review agent (PERF-003).
