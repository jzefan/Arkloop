---
name: ensureLuaTableKeys whitelist in lua.go must be updated for any new agent.spawn fields
description: lua.go line 613 has a hardcoded allowlist. Any new field passed to agent.spawn({}) not in this list causes an error. Plan must include this update.
type: plan-correction
priority: p3
issue_id: "014"
tags: [plan-review, lua, task-1, implementation-detail]
---

## Problem Statement

`lua.go` line 613 calls `ensureLuaTableKeys` with a hardcoded allowlist of valid `agent.spawn({...})` field names. The plan adds `model` as a new field but does not explicitly mention updating this allowlist.

If the whitelist is not updated, every `agent.spawn({ model = "..." })` call in the Lua orchestrator will fail with an "unknown field" error, even after `SpawnRequest.Model` is added.

This is a small but easy-to-miss implementation detail that blocks Task 1 from working.

## Proposed Solutions

Add `"model": {}` to the allowlist map in `ensureLuaTableKeys` call in `lua.go`. This is a one-line change but must be included in Task 1 Step 3.

## Acceptance Criteria

- [ ] Task 1 Step 3 explicitly lists adding `"model"` to the `ensureLuaTableKeys` allowlist in `lua.go`

## Work Log

- 2026-05-07: Identified by feasibility reviewer, learnings researcher
