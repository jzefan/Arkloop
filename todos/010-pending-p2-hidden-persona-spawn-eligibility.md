---
name: Hidden personas (user_selectable:false) may not be spawnable by Lua — verify PersonaKeys population
description: spawn_agent/executor.go validates persona_id against PersonaKeys list. If PersonaKeys only includes user_selectable:true personas, evaluator and synthesizer cannot be spawned.
type: plan-correction
priority: p2
issue_id: "010"
tags: [plan-review, hidden-persona, subagent, task-3, task-4]
---

## Problem Statement

Task 3 creates evaluator and synthesizer personas with `user_selectable: false`. Task 4's orchestrator spawns them using `agent.spawn({ persona_id = "industry-education-evaluator", ... })`.

The `spawn_agent` tool executor validates `persona_id` against a `PersonaKeys` list (line 339 in `executor.go`). This list is populated during app composition. If it only includes user-selectable personas, spawning `industry-education-evaluator` will be rejected with "unknown persona" error.

The plan does not verify this assumption. The Lua `agent.spawn` path may bypass this validation (it goes through a different code path than the LLM `spawn_agent` tool), but this needs explicit verification.

## Proposed Solutions

### Option A: Verify PersonaKeys population in composition.go
Read `src/services/worker/internal/app/composition.go` and `composition_desktop.go` to confirm whether `PersonaKeys` includes all loaded personas or only user-selectable ones. If all personas: no action needed. If user-selectable only: fix in Option B.

### Option B: Update PersonaKeys to include all registered personas
Change the PersonaKeys population to include all personas regardless of `user_selectable` flag. The `user_selectable` flag only gates UI display, not programmatic spawning.

### Option C: Make hidden personas spawnable via a separate allowlist
Add a `spawnable_persona_ids` allowlist to the orchestrator persona config, passed into the Lua runtime, explicitly permitting hidden persona spawning.

**Recommended: Option A first** — verify before fixing.

## Acceptance Criteria

- [ ] Confirmed that `agent.spawn({ persona_id = "industry-education-evaluator" })` does NOT fail with "unknown persona"
- [ ] OR: PersonaKeys updated to include all registered personas

## Work Log

- 2026-05-07: Identified by feasibility reviewer
