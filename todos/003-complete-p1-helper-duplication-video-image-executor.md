---
name: Private helper functions duplicated between video_generate and image_generate executors
description: ~10 private helpers are copy-pasted verbatim between the two executor packages; bugs fixed in one silently diverge in the other (confirmed: missing Routes guard)
type: pending
priority: p1
issue_id: "003"
tags: [code-review, architecture, maintainability]
dependencies: []
---

## Problem Statement

The `video_generate` executor package was created by copying `image_generate`'s executor. Roughly 10 private helper functions are now byte-for-byte duplicated. A bug already exists: `image_generate` has a guard `if len(cfg.Routes) == 0` (line 193-195) that was never ported to `video_generate`, producing worse error messages for misconfigured video routing.

## Findings

**Confirmed duplicated functions (word-for-word or near-identical):**
- `splitModelSelector` — identical logic
- `runGenerationModelOverride` — identical logic
- `buildArtifactKey` — identical logic
- `stringArg`, `errResult`, `errResultWithDetails`, `errorClassForGenerateError`, `errorDetailsForGenerateError`, `copyMap`, `durationMs` — identical
- `resolveSelectedRoute` — structurally identical, differs only in config key string and error messages

**Confirmed divergence (bug):**
- `image_generate/executor.go:193-195` guards `len(cfg.Routes) == 0` with an actionable error
- `video_generate/executor.go:142-189` — guard is absent; misconfigured routing produces generic `route not found`

**Files:**
- `src/services/worker/internal/tools/builtin/video_generate/executor.go:214-352`
- `src/services/worker/internal/tools/builtin/image_generate/executor.go:239-430`

## Proposed Solutions

### Option A — Extract to shared internal package (Recommended)

Create `arkloop/services/worker/internal/tools/builtin/internal/toolutil` with the shared helpers. Both executor packages import from it.

Also extract the common `resolveSelectedRoute` logic into a `RouteResolver` struct parameterized by config key and media-kind label:

```go
type RouteResolver struct {
    ConfigKey    string
    MediaKind    string
    DB           workerdata.QueryDB
    Config       sharedconfig.Resolver
    RoutingLoader *routing.ConfigLoader
}
func (r *RouteResolver) Resolve(ctx context.Context, accountID, runID uuid.UUID) (*routing.SelectedProviderRoute, error)
```

- **Pros:** Single source of truth. Future fixes apply everywhere. `strPtr` and other micro-helpers also consolidated.
- **Cons:** Requires updating both executor packages + tests.
- **Effort:** Medium
- **Risk:** Low — purely mechanical refactor with test coverage

### Option B — Immediate: fix the diverged guard, defer extraction

Add the missing `len(cfg.Routes) == 0` guard to `video_generate/executor.go` now (1-line fix). Schedule the full extraction as a separate task.

- **Pros:** Immediate production improvement. Lower risk in short term.
- **Cons:** Does not fix root cause; next divergence is inevitable.
- **Effort:** Small (guard only) / Medium (full extraction)
- **Risk:** Low

## Recommended Action

Do Option B immediately (1-line guard fix), then Option A in a focused refactor PR.

## Technical Details

- `image_generate/executor.go:193-195` — the guard to port immediately:
  ```go
  if len(cfg.Routes) == 0 {
      return nil, fmt.Errorf("video routing config is empty")
  }
  ```
  Insert after `cfg, err := e.routingLoader.Load(ctx, &accountID)` at `video_generate/executor.go:170`.

## Acceptance Criteria

- [ ] `video_generate/executor.go` has the missing `len(cfg.Routes) == 0` guard
- [ ] All shared helpers exist in exactly one location
- [ ] Both executor packages pass their existing tests after extraction
- [ ] No test references internal helpers directly (package-private extraction is fine)

## Work Log

- 2026-05-06: Identified by maintainability review agent (MAINT-001, MAINT-005).
