---
name: SSRF via unvalidated Gemini video URI download
description: downloadGeneratedVideoURI fetches any URI from Gemini response with no hostname allowlist — attacker-controlled credential can point download at arbitrary external host
type: pending
priority: p1
issue_id: "002"
tags: [code-review, security]
dependencies: []
---

## Problem Statement

`downloadGeneratedVideoURI` makes an HTTP GET to `video.URI` sourced directly from the Gemini API operation response. There is no allowlist check on the hostname before the request is issued. A compromised or misconfigured credential pointing at a malicious server can cause the worker to fetch an arbitrary external URL using the worker's network identity.

## Findings

- `generatedVideoFromOperation` calls `downloadGeneratedVideoURI(ctx, client, video.URI)` when `video.VideoBytes` is empty
  - File: `src/services/worker/internal/llm/video_generation.go:153-156`
- `downloadGeneratedVideoURI` constructs `http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)` and executes with no hostname validation
  - File: `src/services/worker/internal/llm/video_generation.go:172-200`
- The `protocolValidatingTransport` enforces `sharedoutbound.DefaultPolicy` (blocks private IPs) for LLM requests, and this same transport is passed as `g.transport.client` to the download function
  - File: `src/services/worker/internal/llm/video_generation.go:115`
- However, the policy only blocks private/internal IPs — it does not restrict the hostname to `*.googleapis.com`. Any public internet host reachable from the worker can be fetched.

**Attack path:** Attacker controls a Gemini credential (user-configurable) → configures base URL pointing at malicious server → malicious server returns a `GenerateVideosOperation` with `video.URI` pointing to `http://attacker.example.com/exfil` → worker fetches that URL with its HTTP client → response bytes are stored in the object store with the content-type the attacker specifies.

**Residual risk:** Redirect-based SSRF is mitigated by `CheckRedirect` in the validating transport. The primary risk is data exfiltration to arbitrary external hosts and content-type confusion (see todo 005).

## Proposed Solutions

### Option A — Hostname allowlist before fetch (Recommended)

Before issuing the HTTP GET in `downloadGeneratedVideoURI`, validate that `uri` has a hostname matching an allowlist:
- `*.googleapis.com`
- `*.google.com`
- The configured Gemini base URL's hostname

Reject with `GatewayError{ErrorClass: ErrorClassConfigInvalid}` if the hostname does not match.

```go
func isAllowedVideoURI(uri string, baseURL string) bool {
    u, err := url.Parse(uri)
    if err != nil { return false }
    host := strings.ToLower(u.Hostname())
    allowed := []string{"googleapis.com", "google.com"}
    if baseURL != "" {
        if b, err := url.Parse(baseURL); err == nil {
            allowed = append(allowed, strings.ToLower(b.Hostname()))
        }
    }
    for _, suffix := range allowed {
        if host == suffix || strings.HasSuffix(host, "."+suffix) {
            return true
        }
    }
    return false
}
```

- **Pros:** Direct fix. Minimal code change. Easy to test.
- **Cons:** Need to thread the base URL into `generatedVideoFromOperation`; current signature does not pass it.
- **Effort:** Small
- **Risk:** Low

### Option B — Use the protocol validating transport's allowlist config

Extend `sharedoutbound.Policy` to accept a per-request hostname allowlist and configure it for the video download path.

- **Pros:** Centralized policy enforcement; consistent with existing outbound control architecture.
- **Cons:** Larger change surface — requires modifying the shared outbound policy.
- **Effort:** Medium
- **Risk:** Low

## Recommended Action

Option A immediately; Option B as a follow-on to centralize policy enforcement.

## Technical Details

- **Affected files:**
  - `src/services/worker/internal/llm/video_generation.go` (lines 115, 153-156, 172-200)
- **Constants:** `maxDownloadedVideoBytes = 512 << 20` (secondary — size check occurs after full read)

## Acceptance Criteria

- [ ] `downloadGeneratedVideoURI` rejects URIs with hosts not in the allowlist before making any HTTP request
- [ ] Test: URI to `https://attacker.example.com/video.mp4` returns `ErrorClassConfigInvalid`
- [ ] Test: URI to `https://storage.googleapis.com/...` is permitted
- [ ] Test: Redirect from allowed to disallowed host is also blocked

## Work Log

- 2026-05-06: Identified by security review agent (SEC-001).
