---
name: Signed provider URI leaked into tool result JSON and LLM context
description: provider_uri (potentially a signed GCS URL with temp credentials) is included in tool result JSON that flows into LLM context — prompt injection surface and log retention risk
type: pending
priority: p3
issue_id: "011"
tags: [code-review, security]
dependencies: []
---

## Problem Statement

When `video.URI` is non-empty, it is placed verbatim into the tool result as `provider_uri`. Gemini/GCS video URIs are typically time-limited signed URLs. Placing them in tool result JSON means they appear in: (1) the LLM context for downstream agent turns, (2) run event logs in the database. A signed URL in LLM context is a prompt-injection surface.

## Findings

```go
// executor.go:128-130
if strings.TrimSpace(video.URI) != "" {
    result["provider_uri"] = strings.TrimSpace(video.URI)
}
```
File: `src/services/worker/internal/tools/builtin/video_generate/executor.go:128-130`

## Proposed Solution

Remove `provider_uri` from the external tool result. If needed for diagnostics, store it in an internal audit record not exposed to the LLM context.

**Effort:** Trivial — delete 3 lines.

## Acceptance Criteria

- [ ] `provider_uri` is absent from tool result JSON returned to the LLM
- [ ] If diagnostic logging of the URI is needed, it goes to a structured log entry not persisted in `run_events`

## Work Log

- 2026-05-06: Identified by security review agent (SEC-003).
