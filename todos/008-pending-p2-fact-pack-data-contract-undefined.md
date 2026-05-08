---
name: Fact pack data contract between orchestrator → evaluators → synthesizer is unspecified
description: The Lua orchestrator builds a "fact pack" but its JSON schema is never defined. Evaluator prompt doesn't describe the input schema. Synthesizer input format is also unspecified.
type: plan-correction
priority: p2
issue_id: "008"
tags: [plan-review, data-contract, lua, evaluator, synthesizer]
---

## Problem Statement

Task 4 Step 5 says the orchestrator builds "a fact pack JSON with `sources`, `facts`, `missing`, `conflicts`, and `analysis_focus`."

The evaluator prompt says "you only evaluate based on fact pack, sources, and missing items in input" — but never specifies:
- The exact JSON keys and types of the fact pack
- Whether it's passed as a JSON string in the `input` field or as a structured message
- How the evaluator should parse it

The synthesizer receives "all successful evaluator outputs and computed numbers" — but never specifies:
- Whether evaluator outputs are concatenated, wrapped in an array, or keyed by model
- What the computed numbers look like (plain text? JSON?)
- Whether there's an envelope structure

Without these contracts, two implementers building the orchestrator and the evaluator prompt independently will produce incompatible systems.

## Proposed Solutions

### Option A: Add explicit JSON schemas to the plan
Define `FactPackSchema`, `EvaluatorInputSchema`, and `SynthesizerInputSchema` as inline JSON schemas in the plan. Each schema specifies required keys, types, and nullability.

### Option B: Define contracts in `report_template.md`
The plan creates `report_template.md` in Task 4 Step 2. Include the data contracts there as a reference for both the Lua script and the persona prompts.

### Option C: Define contracts in evaluator/synthesizer prompts
Add a `## Input Format` section to each persona's `prompt.md` with the exact JSON structure they receive.

**Recommended: Option C** — the prompt is the contract. Evaluator and synthesizer prompts should include explicit input schemas so the LLM knows what to expect.

## Acceptance Criteria

- [ ] Evaluator `prompt.md` includes an explicit `## Input Format` section with fact pack schema
- [ ] Synthesizer `prompt.md` includes an explicit `## Input Format` section with evaluator output schema
- [ ] Lua orchestrator builds inputs matching those documented schemas

## Work Log

- 2026-05-07: Identified by coherence reviewer
