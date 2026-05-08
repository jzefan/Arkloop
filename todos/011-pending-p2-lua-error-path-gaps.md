---
name: Lua orchestrator has three unaddressed error paths that will cause panics or silent failures
description: Empty search result, all-nil evaluator scores, and synthesizer spawn failure have no defined Lua handling in the plan.
type: plan-correction
priority: p2
issue_id: "011"
tags: [plan-review, lua, error-handling, task-4]
---

## Problem Statement

Task 4's 16-step Lua orchestration does not address three critical error paths:

1. **Web search returns empty fact pack**: `tools.call("web_search", ...)` may return an error or zero results. Plan jumps straight to "verify school is 高职/双高" — no error guard before JSON parsing.

2. **All evaluator scores are nil/failed**: Arithmetic mean with zero valid evaluators will attempt division by zero or produce NaN. Plan says "multi-model failure stops" but Lua division-by-zero may silently return `inf` or `nan` depending on the runtime.

3. **Synthesizer spawn failure**: If the synthesizer fails after all evaluators succeed, the orchestrator has valid scores and no way to write the report. No fallback (e.g., write Markdown directly without synthesizer) is described.

## Proposed Solutions

### Option A: Add explicit guards in each Lua step
- After `web_search`: check result count > 0, else `context.set_output("未找到相关数据...")` and return
- Before score averaging: check `#valid_scores > 0`, else stop with retry guidance
- After synthesizer wait: if `resolved.status == "failed"`, write a minimal Markdown report directly from the orchestrator

### Option B: Wrap entire orchestration in a pcall
Use Lua's `pcall` for error propagation and a top-level error handler that emits a user-friendly failure message. Less granular but prevents silent failures.

**Recommended: Option A** — granular guards produce better user messages.

## Acceptance Criteria

- [ ] Task 4 Lua logic includes an explicit check after web_search before proceeding
- [ ] Score aggregation guards against zero valid evaluators
- [ ] Synthesizer failure path has a defined Markdown-only fallback

## Work Log

- 2026-05-07: Identified by feasibility reviewer
