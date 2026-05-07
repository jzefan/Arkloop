---
name: Video bytes fully buffered in worker memory before upload
description: Full video body (up to 512 MB) is read into []byte heap before S3 upload; at max concurrency 16 jobs = 8 GB RAM
type: pending
priority: p1
issue_id: "001"
tags: [code-review, performance, architecture]
dependencies: []
---

## Problem Statement

Every generated video is fully loaded into process memory as `[]byte` before being uploaded to the object store. With a 512 MB cap and up to 16 concurrent worker jobs, peak RAM usage for video jobs alone can reach 8 GB.

**Why it matters:** Workers are co-located with other job types. OOM kills on the worker pod will cancel all in-flight jobs (text, image, video), causing user-visible failures and queue thrashing.

## Findings

The pipeline materialises the full video body at three sequential points before releasing it:

1. `downloadGeneratedVideoURI` reads up to 512 MB via `io.ReadAll` → `data []byte`
   - File: `src/services/worker/internal/llm/video_generation.go:191-196`

2. `data` is returned as `GeneratedVideo.Bytes []byte` and held on the heap while the executor processes it
   - File: `src/services/worker/internal/llm/video_generation.go:20-27`

3. `executor.Execute` passes `video.Bytes` to `store.PutObject(ctx, key, video.Bytes, ...)` which wraps it in `bytes.NewReader` for the S3 upload
   - File: `src/services/worker/internal/tools/builtin/video_generate/executor.go:109`

The full 512 MB allocation lives in the goroutine heap for the entire download-plus-upload window. `maxDownloadedVideoBytes = 512 << 20` is set at `video_generation.go:17`.

## Proposed Solutions

### Option A — Streaming pipe via io.Reader in GeneratedVideo (Recommended)

Add a `Reader io.ReadCloser` field to `GeneratedVideo`. When downloading from URI, pipe the HTTP response body directly into the object store upload using `io.Reader` rather than buffering.

- **Pros:** Eliminates the heap allocation entirely. Scales to any video size. Minimal API surface change.
- **Cons:** Requires `PutObject` to accept an `io.Reader` (check `objectstore.Store` interface — S3 manager already supports this). Minor refactor to executor.
- **Effort:** Medium
- **Risk:** Low — changes are isolated to download path and executor

### Option B — Multipart S3 upload with configurable part size

Replace `store.PutObject([]byte)` with `s3manager.Uploader` piping the provider response body directly in 8 MB parts, never materialising the full body.

- **Pros:** Industry-standard approach for large object uploads. Already supported by AWS SDK v2.
- **Cons:** Requires changes to the `objectstore.Store` interface. Bigger change surface.
- **Effort:** Large
- **Risk:** Medium

### Option C — Enforce concurrent video job cap

Add a semaphore limiting concurrent video generation jobs (e.g., max 2) regardless of `MaxConcurrency`.

- **Pros:** Immediate protection with no structural change.
- **Cons:** Does not fix the root cause. Users experience queuing, not faster generation. Technical debt remains.
- **Effort:** Small
- **Risk:** Low

## Recommended Action

Option A in the near term; Option B as a follow-on if object store streaming is needed broadly.

## Technical Details

- **Affected files:**
  - `src/services/worker/internal/llm/video_generation.go` (download, GeneratedVideo struct)
  - `src/services/worker/internal/tools/builtin/video_generate/executor.go` (PutObject call)
  - `src/services/shared/objectstore/` (interface may need Reader-accepting overload)
- **Constant:** `maxDownloadedVideoBytes = 512 << 20` at `video_generation.go:17`

## Acceptance Criteria

- [ ] Video generation does not allocate a single `[]byte` of the full video size on the heap
- [ ] Concurrent video jobs do not multiply peak RSS proportionally to video file size
- [ ] Existing tests pass; add a test asserting streaming path is taken for URI-based downloads

## Work Log

- 2026-05-06: Identified by performance review agent. Confirmed by architecture agent (MAINT-004).
