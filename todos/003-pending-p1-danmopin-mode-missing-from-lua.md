---
name: 单模型评估 mode listed in UI form but absent from Lua implementation
description: The user-facing form offers three modes including 单模型评估, but Task 4's 16-step Lua logic only handles 综合评估 and 分别评估.
type: plan-correction
priority: p1
issue_id: "003"
tags: [plan-review, single-model, task-4, lua, missing-feature]
---

## Problem Statement

Confirmed Product Rules list three evaluation modes:
1. 综合评估 (default)
2. 分别评估
3. **单模型评估**

Task 4's Lua orchestration steps cover Step 12 (综合评估: spawn synthesizer) and Step 13 (分别评估: combine model reports), but **no step covers 单模型评估**. A user selecting this mode will hit unhandled code.

This is a shipped UI option with no backend implementation — a functional gap, not a phasing decision.

## Findings

- Confirmed Product Rules (line 19): `单模型评估` is listed as a form option
- Task 4 Step 8: "Spawn evaluator subagents with explicit model" — doesn't conditionally handle single-model case
- Task 4 Steps 12-13: Only 综合/分别 branches present
- Multi-model failure rules also differ: "Single-model failure stops" vs "multi-model failure degrades gracefully"

## Proposed Solutions

### Option A: Add explicit 单模型评估 branch to Task 4 Lua
Add Step 14: if mode == "单模型评估": spawn one evaluator, no aggregation, no synthesizer, write report directly. Estimated: 1 day to add and test.

### Option B: Treat 单模型评估 as a degenerate case of 综合评估
If only one model is selected in 综合评估 mode, the behavior is effectively 单模型. Remove 单模型评估 as a separate form option and document that selecting one model in 综合模式 achieves the same result.

### Option C: Remove 单模型评估 from v1 form and ship it in v2
Hide the option from the form with a "coming soon" state. Reduces v1 scope. Risk: users who expected this feature are disappointed.

**Recommended: Option B** — simplest, no separate code path, honest about actual behavior.

## Acceptance Criteria

- [ ] All three listed modes have corresponding Lua handling, OR
- [ ] 单模型评估 is removed from v1 form with explicit rationale documented

## Work Log

- 2026-05-07: Identified by coherence reviewer and product reviewer
