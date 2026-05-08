---
name: No progress/waiting state designed for the 5-minute parallel subagent execution window
description: Users will see nothing for up to 5 minutes while evaluators run. No loading message, no count of completed evaluators, no cancellation path.
type: plan-correction
priority: p2
issue_id: "007"
tags: [plan-review, ux, progress, subagent, design]
---

## Problem Statement

The orchestrator spawns N evaluator subagents and waits up to 5 minutes per evaluator. The plan says nothing about what the user sees during this window. The existing `SubAgentBlock` shows a blinking cursor for `spawning`/`active` status, but:
- No text explaining the expected wait time
- No indication of how many evaluators have completed vs. pending
- No cancellation affordance
- If user closes the tab and returns, behavior is undefined

At 5 minutes, users with no feedback will assume failure and retry, generating duplicate runs.

## Proposed Solutions

### Option A: Emit an intermediate chat message before spawning
Before spawning evaluators, the orchestrator calls `context.say("正在并行调用 N 个评估模型，预计 3-5 分钟，请稍候...")`. This is achievable in Lua with existing `context.set_output` or an intermediate message API. Requires checking if the Lua runtime supports streaming intermediate messages.

### Option B: Rely on SubAgentBlock UI (existing)
The platform's SubAgentBlock automatically shows spawned child agents with their status. If the frontend renders these in the thread view during execution, users can see N agents spawning and completing. No new code needed if this is already visible.

### Option C: Add explicit timeout guidance to the form confirmation
After the user submits the form, emit: "已收到评估请求。共调用 N 个模型，评估完成预计需要 [N×2] 至 [N×5] 分钟。" Then go silent until done.

**Recommended: Option B first** — verify SubAgentBlock visibility during execution. If not visible, implement Option C as the minimum.

## Acceptance Criteria

- [ ] Plan specifies what users see during the evaluation execution window
- [ ] Confirmed that SubAgentBlock renders during execution OR an intermediate message is emitted
- [ ] Behavior on tab-close-and-return is addressed

## Work Log

- 2026-05-07: Identified by design reviewer
