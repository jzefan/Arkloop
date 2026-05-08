---
name: LLM scoring from single web search produces formal-looking output with unreliable inputs
description: Dimension scores from a single web search pass will vary by school web presence, not actual performance. Formal PDF with decimal precision amplifies rather than mitigates this risk.
type: product-concern
priority: p3
issue_id: "015"
tags: [plan-review, product, scoring, data-quality]
---

## Problem Statement

The product reviewer raises a fundamental concern: dimension scores (e.g., "资源共建共享: 74.3") come from LLMs interpreting public web fragments. A school with poor web presence scores differently than an equally-performing school with active online communications — not because of actual performance differences, but because of data availability.

The formal PDF output (decimal precision, letter grades, A4 layout) will be treated as authoritative by users, especially if submitted to education authorities. The "unknown values remain placeholder" rule is correct but insufficient — the report does not communicate which scores are well-evidenced vs. mostly inferred.

This isn't a blocker, but the plan should include:
- A disclaimer section in the report template
- Clear language that scores are evidence-based estimates, not official evaluations
- An evidence confidence indicator per dimension (e.g., high/medium/low data availability)

## Proposed Solutions

### Option A: Add disclaimer section to report_template.md
Add a standard "数据局限性声明" section to the report template explaining that scores are based on publicly available web data and may not reflect all dimensions of school performance.

### Option B: Add evidence confidence indicator to evaluator output schema
Add a `data_confidence: "high" | "medium" | "low"` field to each dimension in the evaluator JSON. Display in the report as a visual indicator. Helps users understand which scores are well-supported.

### Option C: Reframe output as "evidence brief" not "evaluation verdict"
Change report framing from "产教融合指数报告" (index report) to "产教融合评估参考材料" (reference material). Lower stakes, same content.

**Recommended: Option A minimum** — a one-paragraph disclaimer in the template is low cost and high risk mitigation.

## Acceptance Criteria

- [ ] Report template includes a data limitations disclaimer section
- [ ] Persona prompt instructs the synthesizer to include this disclaimer

## Work Log

- 2026-05-07: Identified by product reviewer
