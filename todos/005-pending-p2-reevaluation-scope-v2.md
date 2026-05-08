---
name: Re-evaluation feature is v2 scope embedded in v1 plan
description: Versioned files, diff summary, supplemental instructions, and re-evaluation detection add ~30% Lua complexity with zero core-goal value for v1.
type: plan-correction
priority: p2
issue_id: "005"
tags: [plan-review, scope, re-evaluation, task-4, lua]
---

## Problem Statement

The plan embeds re-evaluation logic into Task 4's Lua orchestrator:
- Version numbering (`_v2`, `_v3` filename suffixes)
- Diff summary generation (chat-only, comparing score changes)
- Supplemental instruction handling (extra search/analysis focus)
- Re-evaluation detection (distinguishing new vs. re-evaluation in same thread)

Existing Lua personas in the repo are 15-37 lines. A 16-step orchestrator with this branching logic is 3-5x larger. Re-evaluation adds zero value to the core feature (evaluate a college, generate a report). It is also:

- **Undesigned:** The design review found no specified UX for re-evaluation trigger, supplemental instruction input, or diff format
- **Architecturally unclear:** How the agent detects a re-evaluation in the same thread is unspecified (keyword matching? button trigger? previous run lookup?)
- **Diff in wrong place:** The difference summary lives in a transient chat message, not in the formal versioned report — users sharing the new PDF have no visibility into what changed

## Proposed Solutions

### Option A: Remove re-evaluation from v1 plan entirely
Ship v1 with single-evaluation-per-thread semantics. Re-evaluation = start a new thread. Eliminates all related Lua complexity and the unresolved design decisions. Document as v2.

### Option B: Defer detection logic, support manual re-evaluation only
Keep the `_v2`/`_v3` versioning suffix in filenames (trivial), but remove automatic re-evaluation detection, diff generation, and supplemental instruction handling. User re-evaluates by sending a new message; agent treats it as fresh. Filename versioning is cosmetic and adds no Lua complexity.

**Recommended: Option A** — cleanest, eliminates three unresolved design decisions at once.

## Acceptance Criteria

- [ ] Re-evaluation logic removed from Task 4 Lua steps
- [ ] OR: Each re-evaluation sub-feature has a designed UX before implementation begins
- [ ] Plan's Confirmed Product Rules section updated to reflect v1 scope

## Work Log

- 2026-05-07: Identified by scope guardian, design reviewer, coherence reviewer
