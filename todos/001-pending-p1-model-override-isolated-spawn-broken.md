---
name: Model override path broken for isolated subagent spawns
description: SpawnParentContext.Model exists but is never copied into resolved snapshot when context_mode=isolated because Inherit.Runtime=false
type: plan-correction
priority: p1
issue_id: "001"
tags: [plan-review, subagent, model-override, task-1]
---

## Problem Statement

Task 1 adds a `Model` field to `SpawnRequest`/`ResolvedSpawnRequest` and says to "carry model into child run input JSON in `factory.go`." This approach is wrong and will not work.

**Why it fails:** The model override flows through `snapshot.Runtime.Model` → `mw_subagent_context.go` → `rc.InputJSON["model"]` → routing middleware. But this chain only activates when `req.Inherit.Runtime == true`. The plan uses `context_mode = "isolated"` for all evaluator spawns — and `planner.go:127-130` sets `resolved.Inherit.Runtime = false` for isolated spawns. Result: `snapshot.Runtime.Model` is never populated, and the model override silently has no effect.

**Why `factory.go` is not the right place:** `factory.go` never touches `input_json`. The plan describes the wrong file.

## Findings

- `SpawnParentContext.Model` already exists in `types.go`
- `context_snapshot.go:126-131` only copies `ParentContext.Model` when `inherit.runtime = true`
- `planner.go:127-130` always sets `resolved.Inherit.Runtime = false` for isolated context
- `spawn_agent/executor.go:376-377` shows the correct pattern: set `ParentContext.Model` AND clear `ParentContext.RouteID`
- The `ensureLuaTableKeys` allowlist in `lua.go:613` must also include the new `model` key

## Proposed Solutions

### Option A: Use non-isolated context for evaluators with memory isolation
Change evaluator spawns to use `context_mode = "fresh"` or pass `inherit.runtime = true` explicitly. This allows the model override path to work. Risk: evaluators may see unintended parent context.

### Option B: Write model directly into input JSON at spawn time (correct path)
In `factory.go` or the Lua `agent.spawn` handler, write `model` directly into the child run's `InputJSON` before run creation — bypassing the inheritance mechanism entirely. This is the most targeted fix and does not require changing the isolated context mode.

### Option C: Leverage existing `spawn_agent` tool pattern
The `spawn_agent` tool's `applyResolvedProfile()` already sets `ParentContext.Model` and clears `RouteID`. Replicate that exact pattern in the Lua `parseSpawnRequest` handler: when `model` is provided, set `req.ParentContext.Model` and clear `req.ParentContext.RouteID`. Then ensure isolated spawn path still reads `ParentContext.Model` even when `Inherit.Runtime = false`.

**Recommended: Option C** — minimal diff, mirrors existing pattern, doesn't change isolation semantics.

## Acceptance Criteria

- [ ] Lua `agent.spawn({ model = "deepseek^deepseek-chat", context_mode = "isolated" })` routes the child run to the specified model
- [ ] `TestLuaAgentSpawnAcceptsModelOverride` passes with isolated context mode
- [ ] `spawn_agent` tool test with `model` field also passes
- [ ] Existing spawn tests without model field continue to pass

## Work Log

- 2026-05-07: Identified by feasibility reviewer and learnings researcher
