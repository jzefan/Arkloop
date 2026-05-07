---
name: Unvalidated model selector from run_events enables credential/model injection
description: generation_model extracted from run_events.data_json is used as routing selector without validation — user could select premium credentials not intended for them
type: pending
priority: p2
issue_id: "004"
tags: [code-review, security]
dependencies: []
---

## Problem Statement

`runGenerationModelOverride` reads `generation_model` from `run_events.data_json` and returns it as a routing selector string. This string is fed directly into `splitModelSelector` and then used to look up a credential+route in the account's routing config. If `run.started` event data is written from user-supplied API input, a user could craft a selector to pick a premium credential not intended for their account tier.

## Findings

- `runGenerationModelOverride` reads raw JSON from `run_events.data_json` for `type='run.started'`
  - File: `src/services/worker/internal/tools/builtin/video_generate/executor.go:192-212`
- The extracted `generation_model` string (e.g. `"premium-credential^expensive-model"`) is returned without validation and used directly in `resolveSelectedRoute` at line 155-157
- The only filter is `rawTask == "video"` (case-insensitive) — this does not restrict the model value
- If a user can supply `data_json` when starting a run (via the run-start API), they can select any credential in the account's routing config

**Attack path:** Start a run with `data_json = {"generation_task":"video","generation_model":"premium-cred^gpt-4o"}` → executor picks `premium-cred` route → video generated using premium quota.

**Note:** This only affects routes already in the account's routing config — no new credentials can be injected. Severity depends on whether multi-tier credentials are configured.

## Proposed Solutions

### Option A — Remove run-event model override entirely (Recommended)

The account-level `account_entitlement_overrides` table (checked second in `resolveSelectedRoute`) is the appropriate per-account override mechanism with proper access controls. The run-event path adds complexity for marginal benefit.

- **Pros:** Simplest fix. Eliminates the attack surface.
- **Cons:** Any callers that currently pass `generation_model` in the run event will break.
- **Effort:** Small
- **Risk:** Low if the feature is not in active use

### Option B — Validate the model selector against an allowlist

After extracting `generation_model`, validate it against a pre-fetched list of permitted model identifiers for the account. Reject any selector not in the allowlist.

- **Pros:** Preserves the override feature. Defense in depth.
- **Cons:** Requires an allowlist source; adds a DB query.
- **Effort:** Medium
- **Risk:** Medium — allowlist must be kept in sync with routing config

### Option C — Restrict to model-name-only (no credential selector)

Strip the `^credentialName` prefix from the override value, allowing only plain model name selection.

- **Pros:** Prevents cross-credential selection. Simpler than Option B.
- **Cons:** Does not prevent selecting a different model tier within the same credential.
- **Effort:** Small
- **Risk:** Low

## Recommended Action

Option A if the run-event override is not a shipped feature. Option C as a quick hardening if it is.

## Technical Details

- File: `src/services/worker/internal/tools/builtin/video_generate/executor.go:192-212`
- Same pattern likely exists in `image_generate/executor.go` — audit and fix both.

## Acceptance Criteria

- [ ] A user cannot select a credential not intended for them via the run-event `generation_model` field
- [ ] Test: crafted `generation_model` with `^` selector does not escalate to a different credential tier
- [ ] Decision documented on whether run-event override is a supported feature

## Work Log

- 2026-05-06: Identified by security review agent (SEC-002).
