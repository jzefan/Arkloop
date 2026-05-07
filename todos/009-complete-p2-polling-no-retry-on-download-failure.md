---
name: No retry on transient download failure after 10-minute polling wait
description: downloadGeneratedVideoURI has no retry — a single TCP reset after 10 min of polling re-queues the entire job from scratch, wasting API quota and user time
type: pending
priority: p2
issue_id: "009"
tags: [code-review, performance, reliability]
dependencies: []
---

## Problem Statement

`downloadGeneratedVideoURI` makes a single HTTP GET with no retry logic. The download is the final step after up to 10 minutes of polling. A single transient network error (TCP reset, brief DNS hiccup) causes the job to be re-queued with `ErrorClassProviderRetryable` — which starts a new `GenerateVideosFromSource` call, consuming additional API quota and requiring another 10-minute wait.

## Findings

- `downloadGeneratedVideoURI` does one HTTP GET with no retry
  - File: `src/services/worker/internal/llm/video_generation.go:172-200`
- Transport errors classified as `ErrorClassProviderRetryable` (line 185) — correct for job-queue retry, but wasteful when the generation itself succeeded
- `io.ReadAll` failure also classified as retryable (line 194)
- No exponential backoff; no distinction between transient and permanent failures at the download layer

## Proposed Solutions

### Option A — Retry loop in downloadGeneratedVideoURI (Recommended)

Wrap the HTTP GET in a 3-attempt retry with exponential backoff (1s, 2s):

```go
func downloadGeneratedVideoURI(ctx context.Context, client *http.Client, uri string) ([]byte, string, error) {
    const maxAttempts = 3
    var lastErr error
    for attempt := 0; attempt < maxAttempts; attempt++ {
        if attempt > 0 {
            select {
            case <-ctx.Done():
                return nil, "", ctx.Err()
            case <-time.After(time.Duration(attempt) * time.Second):
            }
        }
        data, ct, err := downloadOnce(ctx, client, uri)
        if err == nil {
            return data, ct, nil
        }
        if isNonRetryableDownloadError(err) {
            return nil, "", err
        }
        lastErr = err
    }
    return nil, "", lastErr
}
```

- **Pros:** Prevents re-queuing the entire job for transient network failures. Preserves the existing retryable error classification as a final fallback.
- **Cons:** Adds up to 3 seconds of delay on final failure.
- **Effort:** Small
- **Risk:** Low

### Option B — Store the operation name and resume on retry

When `ErrorClassProviderRetryable` is returned from a polling timeout, persist the `operation.Name` and resume polling on the next job attempt rather than starting a new generation.

- **Pros:** Eliminates redundant generation API calls on any failure type.
- **Cons:** Requires job state persistence across retries; much larger change.
- **Effort:** Large
- **Risk:** Medium

## Recommended Action

Option A immediately. Option B is a longer-term reliability improvement.

## Technical Details

- File: `src/services/worker/internal/llm/video_generation.go:172-200`

## Acceptance Criteria

- [ ] `downloadGeneratedVideoURI` retries up to 3 times on transport-level errors
- [ ] Context cancellation during retry backoff exits immediately
- [ ] HTTP 4xx responses are not retried (non-retryable)
- [ ] Test: single-failure-then-success returns the downloaded bytes

## Work Log

- 2026-05-06: Identified by performance review agent (PERF-004).
