---
name: Video polling loop has no short initial poll — first check always waits 15 seconds
description: defaultVideoPollInterval=15s is used for all polls including the first; fast generations (e.g. <10s) always wait at least 15s for the first status check
type: pending
priority: p3
issue_id: "012"
tags: [code-review, performance]
dependencies: []
---

## Problem Statement

The polling loop in `GenerateVideo` waits `pollInterval` (default 15s) before every poll, including the very first one. If the provider completes the generation in under 15 seconds, the user waits an extra 15 seconds unnecessarily. This is pure latency tax.

## Findings

```go
// video_generation.go:93-105
for polls := 0; operation != nil && !operation.Done && polls < maxPolls; polls++ {
    timer := time.NewTimer(pollInterval)  // always 15s
    select {
    case <-ctx.Done(): ...
    case <-timer.C:
    }
    operation, err = client.Operations.GetVideosOperation(...)
}
```
File: `src/services/worker/internal/llm/video_generation.go:93-105`

## Proposed Solution

Add an initial shorter poll (e.g., 5s) and use the standard `pollInterval` for subsequent polls:

```go
initialInterval := 5 * time.Second
if req.PollInterval > 0 && req.PollInterval < initialInterval {
    initialInterval = req.PollInterval
}
for polls := 0; ...; polls++ {
    interval := pollInterval
    if polls == 0 { interval = initialInterval }
    timer := time.NewTimer(interval)
    ...
}
```

**Effort:** Small. The `PollInterval` field on `VideoGenerationRequest` is already configurable for tests.

## Acceptance Criteria

- [ ] First poll occurs after ~5 seconds (or a configurable initial interval)
- [ ] Subsequent polls use the standard 15-second interval
- [ ] Test: fast mock generation (completes before first standard poll) resolves within initial interval

## Work Log

- 2026-05-06: Identified by performance review agent (PERF-002 partial).
