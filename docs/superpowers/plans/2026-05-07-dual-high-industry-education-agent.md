# 双高产教融合评估 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a user-selectable “双高产教融合评估” agent that asks for evaluation mode and models, orchestrates multiple evaluator subagents, searches public college data once, generates Markdown, and automatically exports a formal PDF.

**Architecture:** Add one orchestrator persona with Lua control flow, one evaluator persona for isolated scoring, and one synthesis persona for formal report writing. Extend subagent spawn requests to support explicit model selectors, add a reusable `markdown_to_pdf` artifact tool, and expose the new persona first in the chat agent entry list.

**Tech Stack:** Go worker services, Lua persona executor, React/Vite web app, existing `ask_user`, `web_search`, `web_fetch`, `document_write`, artifact store, and subagent controller.

---

## Confirmed Product Rules

- Agent name: `双高产教融合评估`; internal persona id: `industry-education-index`.
- Scope: only 双高学院/高职院校. The agent must search and verify school type before report generation. If not verified, stop with a clear message.
- User interaction: form-based. Support natural input like “评估深圳职业技术大学”; otherwise ask for school name.
- Forms:
  - Mode: `综合评估` default, `分别评估`, `单模型评估`.
  - Models: tree by family, currently DeepSeek / Qwen / Doubao. Default one recommended concrete model per configured family. Unconfigured families are visible but disabled.
  - Year: default `2026`.
  - In 综合评估 mode, synthesis model defaults to the first selected evaluator model for v1.
- Subagent orchestration:
  - Main agent searches once and builds a shared fact pack.
  - Evaluator subagents are isolated and independent; they receive the same fact pack and do not see each other.
  - Synthesis subagent sees all successful evaluator outputs.
  - Evaluator timeout: 5 minutes. Synthesis timeout: 5 minutes.
  - Multi-model failure degrades if at least one evaluator succeeds. All evaluators fail means stop with retry/change-model guidance. Single-model failure stops.
- Scoring:
  - Four dimensions in fixed order, each 25%: 基础与机制, 资源共建共享, 产学建设与服务, 人才培养质量.
  - Ratings: 90.0-100.0 A+ 卓越; 80.0-89.9 A 优秀; 70.0-79.9 B 良好; 60.0-69.9 C 达标; below 60.0 D 待提升.
  - System mechanically computes dimension averages, total score, and rating. Models explain and draft, but do not invent total score.
  - Scores must be 0.0-100.0 with one decimal. Invalid values are treated as missing.
  - Public facts and honors must be sourced. Unknown/unverified values remain placeholders.
- Output:
  - Formal Markdown report shown in chat.
  - Automatically create `.md` and `.pdf` artifacts.
  - PDF template: A4 formal report, basic CJK-capable PDF text rendering, page numbers. Clickable source links are deferred to v2; URLs are preserved in generated PDF data.
  - Filename: `{{院校名称}}_产教融合指数报告_{{年份}}.md/pdf`.
  - 综合评估 hides model comparison in the report, but internal evaluator outputs can remain available in run details.
  - 分别评估 combines all model reports into one Markdown and one PDF, split by model section.
  - PDF export failure does not fail the report; return Markdown plus a “PDF 导出失败，可稍后重试导出” message.
- Re-evaluation:
  - Deferred to v2 after Claude review. In v1, a repeated request is treated as a fresh evaluation with a new form.
  - Supplemental analysis instructions are still supported in the initial form and affect search/analysis focus only.

## Files And Responsibilities

- Create `src/personas/industry-education-index/persona.yaml`: selectable orchestrator persona metadata, first selector order, tools.
- Create `src/personas/industry-education-index/prompt.md`: orchestrator rules, form schemas, source policy, failure policy.
- Create `src/personas/industry-education-index/agent.lua`: extract inputs, ask forms, search once, spawn evaluators, aggregate scores, spawn synthesis, write artifacts.
- Create `src/personas/industry-education-index/report_template.md`: fixed report structure and dimension rules.
- Create `src/personas/industry-education-evaluator/persona.yaml`: hidden evaluator persona.
- Create `src/personas/industry-education-evaluator/prompt.md`: strict JSON scoring output contract.
- Create `src/personas/industry-education-synthesizer/persona.yaml`: hidden synthesis persona.
- Create `src/personas/industry-education-synthesizer/prompt.md`: final Markdown report contract.
- Modify `src/services/worker/internal/subagentctl/types.go`: add explicit `Model` to `SpawnRequest` and `ResolvedSpawnRequest`.
- Modify `src/services/worker/internal/subagentctl/planner.go`: trim and carry model selector into resolved spawn plan.
- Modify `src/services/worker/internal/subagentctl/factory.go`: store model selector in child run input JSON.
- Modify `src/services/worker/internal/executor/lua.go`: allow `agent.spawn({ model = "provider^model" })`.
- Modify `src/services/worker/internal/tools/builtin/spawn_agent/executor.go`: allow public `spawn_agent` tool to pass `model`.
- Create `src/services/worker/internal/tools/builtin/markdown_to_pdf/spec.go`: reusable tool spec.
- Create `src/services/worker/internal/tools/builtin/markdown_to_pdf/executor.go`: Markdown to formal PDF artifact executor.
- Create `src/services/worker/internal/tools/builtin/markdown_to_pdf/executor_test.go`: validation, conversion, artifact metadata tests.
- Modify `src/services/worker/internal/app/artifact_tools.go`: register `markdown_to_pdf`.
- Modify `src/services/worker/internal/app/composition.go` and `src/services/worker/internal/app/composition_desktop.go`: update registered artifact tool log expectations if needed.
- Modify `src/services/worker/internal/pipeline/mw_tool_build.go`: ensure `markdown_to_pdf` is treated as stored artifact tool like `document_write`.
- Modify web persona entry logic after inspecting current untracked `src/apps/web/src/personaInvocation.ts`: place `industry-education-index` first without overwriting user changes.
- Add/modify tests in `src/services/worker/internal/executor/lua_test.go`, `src/services/worker/internal/tools/builtin/spawn_agent/executor_test.go`, `src/services/worker/internal/subagentctl/control_test.go`, and web tests around persona ordering.

---

### Task 1: Extend Subagent Spawn Model Override

**Files:**
- Modify: `src/services/worker/internal/subagentctl/types.go`
- Modify: `src/services/worker/internal/subagentctl/planner.go`
- Modify: `src/services/worker/internal/subagentctl/factory.go`
- Modify: `src/services/worker/internal/executor/lua.go`
- Modify: `src/services/worker/internal/tools/builtin/spawn_agent/executor.go`
- Test: `src/services/worker/internal/executor/lua_test.go`
- Test: `src/services/worker/internal/tools/builtin/spawn_agent/executor_test.go`
- Test: `src/services/worker/internal/subagentctl/control_test.go`

- [ ] Step 1: Add failing Lua parser test.

Add a test near existing `agent.spawn` tests in `src/services/worker/internal/executor/lua_test.go`:

```go
func TestLuaAgentSpawnAcceptsModelOverride(t *testing.T) {
	var captured subagentctl.SpawnRequest
	rt := newTestLuaRuntime(t, luaRuntimeConfig{
		subAgentControl: stubSubAgentControl{spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			captured = req
			return subagentctl.StatusSnapshot{ID: uuid.New(), Status: data.SubAgentStatusRunning}, nil
		}},
	})
	err := rt.Run(context.Background(), `
local child, err = agent.spawn({
  persona_id = "industry-education-evaluator",
  input = "score this school",
  context_mode = "isolated",
  profile = "task",
  model = "deepseek官方^deepseek-chat"
})
if err ~= nil then error(err) end
`)
	if err != nil {
		t.Fatalf("run lua: %v", err)
	}
	if captured.Model != "deepseek官方^deepseek-chat" {
		t.Fatalf("expected model override, got %q", captured.Model)
	}
}
```

- [ ] Step 2: Run the failing test.

Run:

```bash
GOCACHE=/private/tmp/arkloop-go-build go test ./src/services/worker/internal/executor -run TestLuaAgentSpawnAcceptsModelOverride
```

Expected: fail because `agent.spawn` rejects unknown parameter `model` or `SpawnRequest.Model` does not exist.

- [ ] Step 3: Implement model fields and parsing.

Add fields in `src/services/worker/internal/subagentctl/types.go`:

```go
type SpawnRequest struct {
	PersonaID   string
	Role        *string
	Nickname    *string
	ContextMode string
	Inherit     SpawnInheritRequest
	Input       string
	SourceType  string
	Profile     string
	Model       string

	ParentContext SpawnParentContext
}

type ResolvedSpawnRequest struct {
	PersonaID   string
	Role        *string
	Nickname    *string
	ContextMode string
	Inherit     ResolvedSpawnInherit
	Input       string
	SourceType  string
	Model       string

	ParentContext SpawnParentContext
}
```

In `resolveSpawnRequest`, assign:

```go
resolved.Model = strings.TrimSpace(req.Model)
```

Allow and parse `model` in `src/services/worker/internal/executor/lua.go`:

```go
"model": {},
```

and before return:

```go
model, err := luaOptionalString(tbl.RawGetString("model"), "agent.spawn: model")
if err != nil {
	return subagentctl.SpawnRequest{}, err
}
```

set:

```go
Model: model,
```

If no `luaOptionalString` helper exists, add:

```go
func luaOptionalString(value lua.LValue, field string) (string, error) {
	if value == lua.LNil {
		return "", nil
	}
	s, err := luaRequiredString(value, field)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(s), nil
}
```

- [ ] Step 4: Carry model into child run input JSON.

In `src/services/worker/internal/subagentctl/factory.go`, where the child run input JSON is assembled, add:

```go
if strings.TrimSpace(spawnReq.Model) != "" {
	inputJSON["model"] = strings.TrimSpace(spawnReq.Model)
}
```

This lets existing routing middleware use the same `rc.InputJSON["model"]` path already used by desktop routing.

- [ ] Step 5: Update `spawn_agent` tool parity.

In `parseSpawnArgs`, allow key `"model"`, parse it as a string, and assign `req.Model`. Add an executor test that passes:

```go
args := map[string]any{
	"persona_id": "industry-education-evaluator",
	"context_mode": "isolated",
	"input": "score",
	"model": "qwen官方^qwen-max",
}
```

Expected captured request model: `qwen官方^qwen-max`.

- [ ] Step 6: Run focused tests.

Run:

```bash
GOCACHE=/private/tmp/arkloop-go-build go test ./src/services/worker/internal/executor ./src/services/worker/internal/tools/builtin/spawn_agent ./src/services/worker/internal/subagentctl
```

Expected: PASS.

---

### Task 2: Add Reusable `markdown_to_pdf` Tool

**Files:**
- Create: `src/services/worker/internal/tools/builtin/markdown_to_pdf/spec.go`
- Create: `src/services/worker/internal/tools/builtin/markdown_to_pdf/executor.go`
- Create: `src/services/worker/internal/tools/builtin/markdown_to_pdf/executor_test.go`
- Modify: `src/services/worker/internal/app/artifact_tools.go`
- Modify: `src/services/worker/internal/app/composition_test.go`
- Modify: `src/services/worker/internal/app/composition_desktop_test.go`
- Modify: `src/services/worker/internal/pipeline/mw_tool_build.go`

- [ ] Step 1: Write executor tests.

Create tests for:

- missing filename returns `tool.args_invalid`;
- non-`.pdf` filename is normalized to `.pdf`;
- `template` accepts only empty or `formal_report`;
- output artifact has `application/pdf`;
- Markdown links remain represented in generated PDF bytes.

Use an in-memory store stub with `PutObject`.

- [ ] Step 2: Implement spec.

Create `spec.go`:

```go
package markdowntopdf

import (
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

const ToolName = "markdown_to_pdf"

var AgentSpec = tools.AgentToolSpec{
	Name:        ToolName,
	Version:     "1",
	Description: "convert Markdown content to a formal PDF artifact",
	RiskLevel:   tools.RiskLevelLow,
	SideEffects: true,
}

var LlmSpec = llm.ToolSpec{
	Name: ToolName,
	Description: strPtr("Convert Markdown report content into a downloadable formal A4 PDF artifact. Use after final report Markdown is complete."),
	JSONSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"title": map[string]any{"type": "string"},
			"filename": map[string]any{"type": "string", "description": "PDF filename, e.g. 深圳职业技术大学_产教融合指数报告_2026.pdf"},
			"template": map[string]any{"type": "string", "enum": []string{"formal_report"}, "default": "formal_report"},
			"content": map[string]any{"type": "string", "description": "final Markdown content"},
		},
		"required": []string{"filename", "content"},
		"additionalProperties": false,
	},
}

func strPtr(s string) *string { return &s }
```

- [ ] Step 3: Implement conversion.

Use a deterministic Go implementation for the first version:

- Convert Markdown to sanitized HTML with a small dependency already present if available; if no markdown package exists, add one minimal Go dependency only after checking `go.mod`.
- Render formal PDF with a Go PDF library already present if available; if absent, use `github.com/jung-kurt/gofpdf` or the actively maintained local equivalent used elsewhere in repo.
- Keep the executor pure Go so worker and desktop sidecar do not depend on external `pandoc`, `wkhtmltopdf`, or GUI Chrome.

Executor output shape must match `document_write` artifact shape:

```json
{
  "artifacts": [
    {
      "key": "...",
      "filename": "深圳职业技术大学_产教融合指数报告_2026.pdf",
      "size": 12345,
      "mime_type": "application/pdf",
      "title": "深圳职业技术大学 · 产教融合指数报告（2026年）",
      "display": "inline"
    }
  ]
}
```

- [ ] Step 4: Register tool.

In `src/services/worker/internal/app/artifact_tools.go`, import the package and add it to the registered stored tools using the same store as `document_write`.

Update tests that assert registered tool lists to include `markdown_to_pdf`.

- [ ] Step 5: Tool-build allowlist handling.

In `src/services/worker/internal/pipeline/mw_tool_build.go`, add `markdown_to_pdf` next to `document_write` in stored artifact tool handling so platform tool overrides and availability checks behave consistently.

- [ ] Step 6: Run focused tests.

Run:

```bash
GOCACHE=/private/tmp/arkloop-go-build go test ./src/services/worker/internal/tools/builtin/markdown_to_pdf ./src/services/worker/internal/app ./src/services/worker/internal/pipeline
```

Expected: PASS.

---

### Task 3: Add Evaluator And Synthesizer Personas

**Files:**
- Create: `src/personas/industry-education-evaluator/persona.yaml`
- Create: `src/personas/industry-education-evaluator/prompt.md`
- Create: `src/personas/industry-education-synthesizer/persona.yaml`
- Create: `src/personas/industry-education-synthesizer/prompt.md`

- [ ] Step 1: Create evaluator persona.

`persona.yaml`:

```yaml
id: industry-education-evaluator
version: "1"
title: 双高产教融合评估员
description: 针对共享事实包输出双高/高职产教融合维度评分。
user_selectable: false
budgets:
    reasoning_iterations: 99900
    temperature: 0.2
reasoning_mode: auto
prompt_cache_control: system_prompt
executor_type: agent.simple
```

`prompt.md` contract:

```md
你是一位产教融合指数分析师。你只根据输入中的事实包、来源与缺失项评估，不自行联网搜索。

输出必须是合法 JSON，不要包含 Markdown 代码块：
{
  "model_label": "...",
  "school_name": "...",
  "year": "2026",
  "eligible": true,
  "dimensions": [
    {"name":"基础与机制","weight":25,"score":0.0,"basis":"...","subscores":[...]},
    {"name":"资源共建共享","weight":25,"score":0.0,"basis":"...","subscores":[...]},
    {"name":"产学建设与服务","weight":25,"score":0.0,"basis":"...","subscores":[...]},
    {"name":"人才培养质量","weight":25,"score":0.0,"basis":"...","subscores":[...]}
  ],
  "honors": [{"text":"...","source_id":"..."}],
  "highlights": [{"name":"...","basis":"..."}],
  "improvements": [{"priority":"高","dimension":"...","text":"..."}],
  "missing_placeholders": ["{{就业率}}"]
}

所有真实评分必须是 0.0 到 100.0，保留 1 位小数。无法判断的分数使用 null，不要编造。
```

- [ ] Step 2: Create synthesizer persona.

`persona.yaml`:

```yaml
id: industry-education-synthesizer
version: "1"
title: 双高产教融合报告撰写员
description: 汇总评估员结果并生成正式 Markdown 报告。
user_selectable: false
budgets:
    reasoning_iterations: 99900
    temperature: 0.3
reasoning_mode: auto
prompt_cache_control: system_prompt
executor_type: agent.simple
```

`prompt.md` contract:

```md
你负责生成正式 Markdown 报告。必须遵循输入中的维度顺序、权重、系统计算后的总分与评级。

输出仅包含报告正文，不要解释过程。
报告必须包含：
# {{院校名称}} · 产教融合指数报告（{{年份}}年）
## 综合评级与产教融合指数得分
## 评分说明
## 基础与机制
## 资源共建共享
## 产学建设与服务
## 人才培养质量
## 核心荣誉与排名
## 优势亮点
## 提升方向
## 数据来源
## 复核说明

来源显示为简洁 Markdown 链接，例如 `- [学校官网](https://example.com)`，不要展示完整 URL 文本。
```

- [ ] Step 3: Persona load smoke.

Run existing persona-loading tests or worker tests that cover persona registry. If no narrow test exists, run:

```bash
GOCACHE=/private/tmp/arkloop-go-build go test ./src/services/worker/internal/app -run Persona
```

Expected: PASS or no tests. If no tests exist, verify by starting worker later in full validation.

---

### Task 4: Add Orchestrator Persona

**Files:**
- Create: `src/personas/industry-education-index/persona.yaml`
- Create: `src/personas/industry-education-index/prompt.md`
- Create: `src/personas/industry-education-index/report_template.md`
- Create: `src/personas/industry-education-index/agent.lua`

- [ ] Step 1: Create selectable persona metadata.

`persona.yaml`:

```yaml
id: industry-education-index
version: "1"
title: 双高产教融合评估
description: 搜索公开资料并使用多模型子智能体生成双高/高职产教融合指数报告。
soul_file: prompt.md
user_selectable: true
selector_name: 双高产教融合评估
selector_order: 0
core_tools:
    - ask_user
    - web_search
    - web_fetch
    - document_write
    - markdown_to_pdf
    - platform
budgets:
    reasoning_iterations: 99900
    temperature: 0.4
reasoning_mode: auto
prompt_cache_control: system_prompt
executor_type: agent.lua
executor_config:
    script_file: agent.lua
```

- [ ] Step 2: Add report template.

Create `report_template.md` with the fixed headings, dimensions, subitems, source rules, and placeholder policy from Confirmed Product Rules.

- [ ] Step 3: Implement Lua orchestration.

`agent.lua` should:

1. Read current user message and previous run context if available.
2. Extract school name with LLM only if obvious; otherwise call `ask_user` with a form requiring school name and year.
3. Call `ask_user` for mode with enum defaults.
4. Use the platform tool to list providers/models, group configured models into DeepSeek/Qwen/Doubao families, and ask model selection form with default one model per available family.
5. Search and fetch authoritative sources once:
   - school official site;
   - 教育部/省教育厅/双高计划名单;
   - official honor/ranking pages.
6. Verify the school is 高职/双高 relevant. Stop if not verified.
7. Build a fact pack JSON with `sources`, `facts`, `missing`, `conflicts`, and `analysis_focus`.
8. Spawn evaluator subagents with explicit `model`, isolated context, and 5-minute waits.
9. Validate evaluator JSON scores in Lua:
   - dimensions exactly match required order;
   - score is nil or 0-100;
   - round to 1 decimal;
   - invalid dimension score is nil.
10. Aggregate successful evaluator scores by arithmetic average.
11. Compute total and rating mechanically.
12. For 综合评估, spawn synthesizer with all successful evaluator outputs and computed numbers.
13. For 分别评估, create one combined Markdown split by model.
14. Write Markdown artifact using `document_write`.
15. Convert PDF using `markdown_to_pdf`; if it fails, preserve Markdown and surface the PDF error.
16. Set chat output to report body plus generated artifact names.

- [ ] Step 4: Add small Lua helper tests where feasible.

If the Lua runtime supports unit loading individual scripts, add tests for pure helper functions:

- `rating_for_score(89.9) == "A 优秀"`;
- `rating_for_score(90.0) == "A+ 卓越"`;
- invalid score string is nil;
- aggregate excludes failed evaluators.

If direct Lua helper tests are not supported, keep helper functions small and verify through an integration run in Task 7.

---

### Task 5: Web Entry Ordering

**Files:**
- Inspect/modify: `src/apps/web/src/personaInvocation.ts`
- Inspect/modify: `src/apps/web/src/__tests__/personaInvocation.test.ts`
- Possibly modify: `src/apps/web/src/components/ChatInput.tsx`

- [ ] Step 1: Inspect existing untracked user files.

Run:

```bash
git status --short src/apps/web/src/personaInvocation.ts src/apps/web/src/__tests__/personaInvocation.test.ts
sed -n '1,220p' src/apps/web/src/personaInvocation.ts
sed -n '1,260p' src/apps/web/src/__tests__/personaInvocation.test.ts
```

Expected: understand user changes before editing. Do not overwrite unrelated work.

- [ ] Step 2: Add/adjust test for first entry.

Expected behavior:

```ts
expect(entries[0]).toMatchObject({
  id: 'industry-education-index',
  label: '双高产教融合评估',
})
```

- [ ] Step 3: Implement ordering.

Use `selector_order: 0` from persona metadata if the frontend already sorts by selector order. If frontend has hardcoded entries, add this persona first and keep existing entries order after it.

- [ ] Step 4: Run focused frontend tests.

Run:

```bash
cd src/apps/web && pnpm test -- personaInvocation
```

Expected: PASS.

---

### Task 6: Report Artifact And PDF Presentation In Chat

**Files:**
- Inspect/modify: `src/apps/web/src/hooks/useThreadSseEffect.ts`
- Inspect/modify: `src/apps/web/src/lib/chat-helpers.ts`
- Inspect/modify: `src/apps/web/src/components/ArtifactDownload.tsx`
- Test: existing artifact tests in `src/apps/web/src/__tests__/documentPanel.test.ts` and related tests

- [ ] Step 1: Confirm PDF artifacts render as downloads.

Search for MIME handling:

```bash
rg -n "application/pdf|mime_type|ArtifactDownload|document_write|create_artifact" src/apps/web/src
```

- [ ] Step 2: Add PDF artifact test if missing.

Test that an artifact result with:

```ts
{ filename: '深圳职业技术大学_产教融合指数报告_2026.pdf', mime_type: 'application/pdf' }
```

appears as a downloadable artifact row and does not try to inline-render as HTML/SVG/Markdown.

- [ ] Step 3: Implement only if needed.

If the existing artifact renderer already treats unknown MIME types as downloads, no code change is required. If not, add `application/pdf` to the download-safe MIME handling.

- [ ] Step 4: Run focused tests.

Run:

```bash
cd src/apps/web && pnpm test -- documentPanel artifact
```

Expected: PASS.

---

### Task 7: End-To-End Verification

**Files:**
- No new files unless tests reveal missing coverage.

- [ ] Step 1: Backend tests.

Run:

```bash
GOCACHE=/private/tmp/arkloop-go-build go test ./src/services/worker/internal/executor ./src/services/worker/internal/tools/builtin/spawn_agent ./src/services/worker/internal/tools/builtin/markdown_to_pdf ./src/services/worker/internal/subagentctl ./src/services/worker/internal/app ./src/services/worker/internal/pipeline
```

Expected: PASS.

- [ ] Step 2: Frontend tests.

Run:

```bash
cd src/apps/web && pnpm test -- personaInvocation documentPanel artifact
```

Expected: PASS.

- [ ] Step 3: Build checks.

Run:

```bash
cd src/apps/web && pnpm build
```

Expected: PASS.

- [ ] Step 4: Desktop/manual smoke.

Run:

```bash
pnpm dev
```

Manual checks:

- “双高产教融合评估” appears first in the chat agent entry list.
- Input `评估深圳职业技术大学`.
- The agent asks a mode form and model tree form.
- It rejects a clearly non 高职/双高 school after search verification.
- With at least one configured DeepSeek/Qwen/Doubao model, it generates Markdown and PDF artifact names.
- PDF artifact opens/downloads and has formal A4 report formatting.

---

## Self-Review

- Spec coverage: the plan covers form interaction, model tree selection, multi-subagent orchestration, source search, eligibility gate, scoring/aggregation/rating, Markdown/PDF output, re-evaluation, and chat entry placement.
- Risk area: `markdown_to_pdf` renderer library choice must be finalized by inspecting existing Go dependencies during Task 2. The rule is explicit: prefer existing dependencies; if absent, add one small Go dependency and keep the tool pure Go.
- Dirty worktree: `src/apps/web/src/personaInvocation.ts` and `src/apps/web/src/__tests__/personaInvocation.test.ts` are currently untracked. Task 5 explicitly starts by inspecting and preserving those changes.

## Post-Review Adjustments

- The orchestrator form flow is two-step for the current v1 UX: first choose mode, then choose models. School name is parsed from the user message and the report year defaults to 2026.
- Each evaluator subagent is given a 5 minute wait timeout. The Lua script avoids `os` APIs because the executor sandbox blocks them.
- `分别评估` now receives `per_model_computed`, a system-computed per-model score/rating payload, so the synthesizer can split the report by model without recalculating.
- Lua `agent.spawn` and `agent.wait` now emit `tool.call` / `tool.result` events for observability in run details.
