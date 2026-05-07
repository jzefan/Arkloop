---
name: Video content-type from provider stored without allowlist validation
description: normalizeVideoContentType only checks "video/" prefix — arbitrary video/* subtypes accepted; http.DetectContentType fallback cannot reliably distinguish video from other formats
type: pending
priority: p2
issue_id: "005"
tags: [code-review, security]
dependencies: [002]
---

## Problem Statement

`normalizeVideoContentType` accepts any string starting with `video/` — including `video/x-shockwave-flash`, `video/x-msvideo`, or other subtypes. Combined with the SSRF risk (todo 002), a malicious server can dictate the content-type of stored artifacts. The `http.DetectContentType` fallback does not reliably detect video containers.

## Findings

- `normalizeVideoContentType` checks only `strings.HasPrefix(cleaned, "video/")`, falling back to `"video/mp4"`
  - File: `src/services/worker/internal/tools/builtin/video_generate/executor.go:237-248`
- When video bytes come from `downloadGeneratedVideoURI`, the HTTP `Content-Type` header value is used as `video.MIMEType`
  - File: `src/services/worker/internal/llm/video_generation.go:158-160`
- If `MIMEType` is empty, `http.DetectContentType` is called on the raw bytes
  - File: `src/services/worker/internal/llm/video_generation.go:163-165`
- `http.DetectContentType` has limited video format recognition; a crafted payload could sniff as `application/octet-stream` and be overridden to `video/mp4`

## Proposed Solutions

### Option A — Explicit allowlist (Recommended)

Replace the prefix check with an explicit allowlist:

```go
var allowedVideoTypes = map[string]bool{
    "video/mp4":       true,
    "video/webm":      true,
    "video/quicktime": true,
    "video/ogg":       true,
}

func normalizeVideoContentType(contentType string) string {
    cleaned := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
    if allowedVideoTypes[cleaned] {
        return cleaned
    }
    return "video/mp4"
}
```

- **Pros:** Simple, auditable, future-safe. Rejects unusual subtypes.
- **Cons:** New video subtypes require an explicit addition (acceptable operational cost).
- **Effort:** Small
- **Risk:** Very low

### Option B — Magic byte validation

Before accepting video bytes, validate the container format magic bytes (MP4: `ftyp` box at offset 4; WebM: `0x1A45DFA3`). Reject non-matching payloads.

- **Pros:** Defense in depth against content confusion.
- **Cons:** More complex; different providers may use different packaging.
- **Effort:** Small-Medium
- **Risk:** Low

## Recommended Action

Option A immediately; Option B as a follow-on for defense in depth.

## Technical Details

- File: `src/services/worker/internal/tools/builtin/video_generate/executor.go:237-248`
- File: `src/services/worker/internal/llm/video_generation.go:158-168`

## Acceptance Criteria

- [ ] `normalizeVideoContentType` uses an explicit allowlist
- [ ] `video/x-shockwave-flash` and similar types are rejected and fall back to `video/mp4`
- [ ] Test: provider response with `Content-Type: video/x-msvideo` is normalized to `video/mp4`
- [ ] Test: provider response with `Content-Type: video/webm` is stored as `video/webm`

## Work Log

- 2026-05-06: Identified by security review agent (SEC-004).
