---
name: Task 1 model override may be largely redundant with existing profile mechanism
description: The existing profile="strong"/"explore"/"task" already resolves to provider^model strings. Task 1 adds new infrastructure for a capability that may already exist.
type: plan-correction
priority: p2
issue_id: "009"
tags: [plan-review, scope, task-1, model-override, profile]
---

## Problem Statement

Task 1 adds `Model string` to `SpawnRequest`/`ResolvedSpawnRequest` as a new platform primitive. But the existing `profile` field on `agent.spawn({})` already does this: `profile = "strong"` → resolves to a `provider^model` string via `applyResolvedProfile()` → sets `ParentContext.Model`.

The scope guardian review flags: the only gap is if the plan needs a *specific* arbitrary model string (e.g., `"deepseek官方^deepseek-chat"`) that the three named tiers don't cover. The plan's use case — "use different LLM families per evaluator" — may or may not require arbitrary model strings.

## Proposed Solutions

### Option A: Verify whether profile satisfies the requirement
Check: can the orchestrator achieve "use DeepSeek for evaluator A, Qwen for evaluator B, Doubao for evaluator C" using the existing `profile` field and entitlement config? If yes, Task 1 is unnecessary.

### Option B: Implement a lighter-weight model override
Instead of adding `Model` to `SpawnRequest`/`ResolvedSpawnRequest` (platform-wide types), add model override only at the Lua layer: allow `agent.spawn({ model = "..." })` to write directly to `ParentContext.Model`/`RouteID` without touching the data model types. This is a smaller surface area.

### Option C: Proceed with Task 1 as written but as a separate PR
If arbitrary model strings are needed, Task 1 is valid but should be a standalone PR with its own review, not bundled with a persona feature.

**Recommended: Option A first** — validate if profile suffices. If not, proceed with Option B.

## Acceptance Criteria

- [ ] Plan documents why existing `profile` mechanism is insufficient (with a concrete model example)
- [ ] OR: Task 1 is removed and persona uses `profile` field instead

## Work Log

- 2026-05-07: Identified by scope guardian, feasibility reviewer
