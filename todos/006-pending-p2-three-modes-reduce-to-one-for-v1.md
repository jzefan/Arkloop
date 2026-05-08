---
name: Three evaluation modes for v1 triples Lua complexity with marginal additional value
description: 综合评估 delivers 80% of the stated goal. 分别评估 and 单模型评估 add Lua branching without a named user who needs them.
type: plan-correction
priority: p2
issue_id: "006"
tags: [plan-review, scope, evaluation-modes, task-4]
---

## Problem Statement

The plan implements three evaluation modes in a single Lua orchestrator:
1. **综合评估**: Multi-model parallel evaluation → aggregated scores → synthesizer → single report
2. **分别评估**: Multi-model parallel evaluation → separate per-model reports → combined into one document
3. **单模型评估**: Single model evaluation → direct report

Each mode has distinct control flow. 分别评估 mode requires three complete report pipelines, three sets of artifacts, and a combined document structure that is entirely undesigned (no section layout specified). 单模型评估 is currently missing from Task 4 entirely.

The plan has no stated user persona who specifically needs 分别评估. The use case — "compare how three different models evaluate the same institution" — is a researcher/developer use case, not a college administrator use case.

## Proposed Solutions

### Option A: Ship 综合评估 only for v1
Remove 分别评估 and 单模型评估 from v1 form. 综合评估 always runs all configured models and produces a consensus report. Add the other modes in v2 with designed UX and report templates.

### Option B: Ship 综合评估 + 单模型评估 only
单模型评估 is a degenerate case of 综合评估 with one model selected. Implement by checking `#selected_models == 1` and skipping aggregation/synthesizer. Effectively zero extra code.

### Option C: Make mode selection a developer-only config
Hide mode selection from the user form. Orchestrator always uses 综合评估. Mode becomes a config flag for future power users.

**Recommended: Option B** — trivially handles single-model case, eliminates the complex 分别评估 pipeline and its unresolved report structure design.

## Acceptance Criteria

- [ ] v1 plan specifies exactly which modes are shipped
- [ ] Unshipped modes are either removed from the form or clearly marked "coming soon"
- [ ] 分别评估 report structure design completed before it is implemented

## Work Log

- 2026-05-07: Identified by scope guardian, product reviewer, coherence reviewer
