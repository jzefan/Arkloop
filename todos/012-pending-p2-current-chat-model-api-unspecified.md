---
name: 综合评估 synthesis model default ("current chat model") has no specified Lua API
description: Plan says synthesis model defaults to "current chat model if available." No Lua API for reading the current model is documented or verified to exist.
type: plan-correction
priority: p2
issue_id: "012"
tags: [plan-review, lua, synthesis-model, task-4]
---

## Problem Statement

Confirmed Product Rules state:
> "In 综合评估 mode, synthesis model defaults to current chat model if available, otherwise first selected evaluator model."

Task 4's Lua logic has no step that:
1. Reads the current chat model from the Lua runtime
2. Checks if it's "available" (what does available mean here? configured? in the selected model list?)
3. Falls back to the first evaluator model if not

The existing Lua API (`agent.spawn`, `agent.wait`, `context.set_output`, `agent.loop_capture`) has no documented `agent.current_model()` or equivalent. If this API doesn't exist, the feature can't be implemented as described.

## Proposed Solutions

### Option A: Verify if Lua runtime exposes current model
Read `src/services/worker/internal/executor/lua.go` to check if there's a `agent.current_model()` binding or similar. If yes: document the API call in Task 4. If no: implement Option B.

### Option B: Pass current model via InputJSON
The orchestrator's own run has a model assigned. Pass it into the Lua script via `rc.InputJSON["model"]` so the script can read it with `input_json.model`. This would require reading `input_json` in Lua, which may or may not be exposed.

### Option C: Simplify: always use first selected evaluator model for synthesis
Remove the "current chat model" defaulting logic entirely. Always default synthesis to the first evaluator model. Simpler, no API dependency.

**Recommended: Option C** — removes a complex default rule that adds marginal UX value.

## Acceptance Criteria

- [ ] Plan specifies the exact Lua API call for reading current model, OR
- [ ] "Current chat model" defaulting rule is removed from Confirmed Product Rules

## Work Log

- 2026-05-07: Identified by coherence reviewer
