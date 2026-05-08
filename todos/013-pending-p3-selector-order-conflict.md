---
name: selector_order:0 conflicts with existing personas using order 1/2; plan should use order 3+
description: normal=1, work=1, extended-search=2. New persona should use 3 or higher to avoid ordering collisions.
type: plan-correction
priority: p3
issue_id: "013"
tags: [plan-review, selector-order, task-4, task-5]
---

## Problem Statement

The orchestrator persona.yaml specifies `selector_order: 0`, claiming first position in the UI. The learnings research confirms:
- `normal` persona: `selector_order: 1`
- `work` persona: `selector_order: 1`
- `extended-search` persona: `selector_order: 2`

Using `selector_order: 0` will place the new persona first, before `normal` and `work`. This may be the intent, but it will change the existing UI order for all users, not just add a new entry. If the intent is "appear first," that's a product decision that affects all existing users of the platform. If the intent is "appear in the list," use `selector_order: 3`.

Also: two personas sharing `selector_order: 1` (normal + work) shows that ties are already resolved by some secondary sort — the tiebreaker is not documented.

## Proposed Solutions

### Option A: Use selector_order:3 to place after existing entries
Non-disruptive. New persona appears after normal, work, extended-search. Add a note in persona.yaml if first position is a confirmed business requirement.

### Option B: Keep selector_order:0 but document intent
If "must be first" is a confirmed product requirement, keep 0 but acknowledge this pushes existing personas down. Update this in Confirmed Product Rules with explicit justification.

**Recommended: Clarify the intent first.** If "first in list" is required, justify it. If "available in list" is sufficient, use 3.

## Acceptance Criteria

- [ ] selector_order value chosen with explicit justification for its position relative to existing personas

## Work Log

- 2026-05-07: Identified by learnings researcher, coherence reviewer
