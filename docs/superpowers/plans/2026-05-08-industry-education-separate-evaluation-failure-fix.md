# 分别评估失败摘要修复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the industry-education persona so `分别评估` continues when at least one evaluator returns valid JSON, and shows model-specific failure reasons when all evaluators fail.

**Architecture:** Keep the change inside the Lua orchestrator and its executor tests. Extend `validate_evaluation` to return structured failure reasons, preserve per-model failure summaries in `wait_for_evaluators`, and format the final `#evaluations == 0` error from those summaries without changing frontend, API, or persona product flow.

**Tech Stack:** Lua persona runtime, Go executor tests, `go test`, existing `agent.spawn`, `agent.wait`, `context.set_output`, and JSON helpers.

---

## Files And Responsibilities

- Modify `src/personas/industry-education-index/agent.lua`: preserve evaluator validation reasons, collect per-model failure summaries, and format the final all-failed message.
- Modify `src/services/worker/internal/executor/lua_test.go`: add regression tests for partial success in `分别评估`, all-invalid JSON failures, and all-timeout/no-output failures.
- Reference `docs/superpowers/specs/2026-05-08-industry-education-separate-evaluation-failure-design.md`: source of truth for scope and acceptance criteria.

---

### Task 1: Prove `分别评估` survives partial evaluator failure

**Files:**
- Modify: `src/services/worker/internal/executor/lua_test.go`
- Test: `src/services/worker/internal/executor/lua_test.go`

- [ ] **Step 1: Write the failing regression test**

Add this test after `TestIndustryEducationIndexAgentLuaRunsWithSubAgentsAndArtifacts` in `src/services/worker/internal/executor/lua_test.go`:

```go
func TestIndustryEducationIndexAgentLuaSeparateModeContinuesWithOneValidEvaluation(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../../../personas/industry-education-index/agent.lua")
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"available_models": []map[string]any{
			{"selector": "deepseek official^deepseek-chat", "model": "deepseek-chat", "provider_kind": "deepseek", "credential_name": "deepseek official"},
			{"selector": "qwen proxy^qwen-max", "model": "qwen-max", "provider_kind": "openai", "credential_name": "qwen proxy"},
		},
	}
	answers := []string{
		`{"mode":"分别评估"}`,
		`{"models":["deepseek official^deepseek-chat","qwen proxy^qwen-max"]}`,
	}
	rc.WaitForInput = func(_ context.Context) (string, bool) {
		if len(answers) == 0 {
			return "", false
		}
		answer := answers[0]
		answers = answers[1:]
		return answer, true
	}

	reg := tools.NewRegistry()
	for _, spec := range []tools.AgentToolSpec{
		{Name: "web_search", Version: "1", Description: "search", RiskLevel: tools.RiskLevelLow},
		{Name: "web_fetch", Version: "1", Description: "fetch", RiskLevel: tools.RiskLevelLow},
		{Name: "document_write", Version: "1", Description: "doc", RiskLevel: tools.RiskLevelLow},
		{Name: "markdown_to_pdf", Version: "1", Description: "pdf", RiskLevel: tools.RiskLevelLow},
	} {
		if err := reg.Register(spec); err != nil {
			t.Fatalf("register %s: %v", spec.Name, err)
		}
	}
	dispatch := tools.NewDispatchingExecutor(reg, tools.NewPolicyEnforcer(reg, tools.AllowlistFromNames([]string{
		"web_search",
		"web_fetch",
		"document_write",
		"markdown_to_pdf",
	})))
	if err := dispatch.Bind("web_search", stubLuaToolExecutor{
		execute: func(_ context.Context, _ string, _ map[string]any, _ tools.ExecutionContext, _ string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"results": []map[string]any{{
					"title":   "深圳职业技术大学官网",
					"url":     "https://www.szpu.edu.cn/",
					"snippet": "学校官网、院系设置、人才培养与校园新闻。",
				}},
			}}
		},
	}); err != nil {
		t.Fatalf("bind web_search: %v", err)
	}
	if err := dispatch.Bind("web_fetch", stubLuaToolExecutor{
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"content": "学校围绕产教融合、校企合作和专业群建设开展人才培养。",
			}}
		},
	}); err != nil {
		t.Fatalf("bind web_fetch: %v", err)
	}
	if err := dispatch.Bind("document_write", stubLuaToolExecutor{
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"artifacts": []map[string]any{{"filename": "report.md", "mime_type": "text/markdown"}},
			}}
		},
	}); err != nil {
		t.Fatalf("bind document_write: %v", err)
	}
	if err := dispatch.Bind("markdown_to_pdf", stubLuaToolExecutor{
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"artifacts": []map[string]any{{"filename": "report.pdf", "mime_type": "application/pdf", "display": "download"}},
			}}
		},
	}); err != nil {
		t.Fatalf("bind markdown_to_pdf: %v", err)
	}
	rc.ToolExecutor = dispatch

	outputs := map[uuid.UUID]string{}
	errs := map[uuid.UUID]error{}
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			personaID := req.PersonaID
			switch req.PersonaID {
			case "industry-education-evaluator":
				if req.Model == "deepseek official^deepseek-chat" {
					outputs[subAgentID] = `{"eligible":true,"model_label":"DeepSeek","dimensions":[{"name":"基础与机制","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"资源共建共享","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"产学建设与服务","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"人才培养质量","weight":25,"score":80.0,"data_confidence":"medium"}],"core_honors":[],"highlights":[],"improvements":[],"missing":[]}`
				} else {
					outputs[subAgentID] = `{"eligible":true,"model_label":"Qwen","dimensions":[{"name":"基础与机制","weight":25,"score":81.0,"data_confidence":"medium"}]}`
				}
			case "industry-education-synthesizer":
				outputs[subAgentID] = "# 深圳职业技术大学 · 产教融合指数报告（2026年）\n\n## 综合评级与产教融合指数得分"
			default:
				errs[subAgentID] = errors.New("unexpected persona: " + req.PersonaID)
			}
			return subagentctl.StatusSnapshot{
				SubAgentID:  subAgentID,
				Status:      data.SubAgentStatusQueued,
				PersonaID:   &personaID,
				ContextMode: req.ContextMode,
			}, nil
		},
		wait: func(_ context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := req.SubAgentIDs[0]
			if err := errs[subAgentID]; err != nil {
				return subagentctl.StatusSnapshot{}, err
			}
			output := outputs[subAgentID]
			return subagentctl.StatusSnapshot{SubAgentID: subAgentID, Status: data.SubAgentStatusCompleted, LastOutput: &output}, nil
		},
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	texts := strings.Join(deltaTexts(evs), "")
	if strings.Contains(texts, "所有评估模型均未成功返回有效结果") {
		t.Fatalf("expected separate mode to continue with one valid evaluation, got %q", texts)
	}
	if !strings.Contains(texts, "深圳职业技术大学 · 产教融合指数报告（2026年）") {
		t.Fatalf("expected report output, got %q", texts)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run:

```bash
go test ./src/services/worker/internal/executor -run TestIndustryEducationIndexAgentLuaSeparateModeContinuesWithOneValidEvaluation -count=1
```

Expected: FAIL because the current Lua flow does not yet preserve detailed evaluator validation reasons and this regression test is not implemented yet.

- [ ] **Step 3: Keep the test in place and inspect the failure output**

Confirm the failure is from the new regression test itself, not from unrelated setup errors. The key assertion should fail on the all-failed message or missing report output.

- [ ] **Step 4: Do not change implementation yet**

Leave `src/personas/industry-education-index/agent.lua` untouched until the next task adds the minimal behavior needed to satisfy this regression.

---

### Task 2: Add evaluator validation reasons and all-failed summaries

**Files:**
- Modify: `src/personas/industry-education-index/agent.lua`
- Test: `src/services/worker/internal/executor/lua_test.go`

- [ ] **Step 1: Change `validate_evaluation` to return a reason**

Replace the existing `validate_evaluation` function in `src/personas/industry-education-index/agent.lua` with:

```lua
local function validate_evaluation(raw_text)
  local text = trim(raw_text)
  if text == "" then
    return nil, "未返回内容"
  end
  local decoded, err = json.decode(text)
  if err ~= nil or decoded == nil then
    return nil, "返回格式不是有效 JSON"
  end
  if decoded.eligible ~= true then
    return nil, "未返回 eligible=true 的评估结果"
  end
  if decoded.dimensions == nil then
    return nil, "缺少 dimensions 字段"
  end
  local by_name = {}
  local count = 0
  for _, dim in ipairs(decoded.dimensions) do
    if dim ~= nil and dim.name ~= nil then
      by_name[dim.name] = dim
      count = count + 1
    end
  end
  if count ~= #DIMENSIONS then
    return nil, "维度数量不完整"
  end
  for _, name in ipairs(DIMENSIONS) do
    if by_name[name] == nil then
      return nil, "维度顺序或名称不符合要求"
    end
    local raw_score = by_name[name].score
    if raw_score ~= nil and valid_score(raw_score) == nil then
      return nil, "维度分数超出允许范围"
    end
    by_name[name].score = valid_score(raw_score)
  end
  decoded.dimensions = {
    by_name[DIMENSIONS[1]],
    by_name[DIMENSIONS[2]],
    by_name[DIMENSIONS[3]],
    by_name[DIMENSIONS[4]],
  }
  return decoded, nil
end
```

- [ ] **Step 2: Add failure formatting helpers**

Insert these helpers above `wait_for_evaluators` in `src/personas/industry-education-index/agent.lua`:

```lua
local function failure_reason_text(reason)
  if reason == nil or trim(reason) == "" then
    return "未知错误"
  end
  return trim(reason)
end

local function format_failures(failures)
  if failures == nil or #failures == 0 then
    return "所有评估模型均未成功返回有效结果，请重试或更换模型。"
  end
  local lines = {
    "所有评估模型均未成功返回有效结果。",
    "",
    "失败摘要：",
  }
  for _, failure in ipairs(failures) do
    local label = model_label(failure.model)
    table.insert(lines, "- " .. label .. "：" .. failure_reason_text(failure.error))
  end
  table.insert(lines, "")
  table.insert(lines, "请重试或更换模型。")
  return table.concat(lines, "\n")
end
```

- [ ] **Step 3: Preserve model-specific failure reasons in `wait_for_evaluators`**

Replace the existing `wait_for_evaluators` implementation in `src/personas/industry-education-index/agent.lua` with:

```lua
local function wait_for_evaluators(children, single_model)
  local evaluations = {}
  local failures = {}
  for _, item in ipairs(children) do
    local resolved, wait_err = agent.wait(item.child.id, WAIT_MS)
    if wait_err ~= nil then
      table.insert(failures, { model = item.model, error = "执行失败或超时：" .. tostring(wait_err) })
      if single_model then return evaluations, failures end
    elseif resolved == nil or resolved.output == nil then
      table.insert(failures, { model = item.model, error = "未返回内容" })
      if single_model then return evaluations, failures end
    else
      local evaluation, reason = validate_evaluation(resolved.output)
      if evaluation == nil then
        table.insert(failures, { model = item.model, error = reason or "返回内容不符合评估格式" })
        if single_model then return evaluations, failures end
      else
        evaluation.model_label = evaluation.model_label or model_label(item.model)
        table.insert(evaluations, evaluation)
      end
    end
  end
  return evaluations, failures
end
```

- [ ] **Step 4: Use failure summaries in the all-failed branch**

Change the `#evaluations == 0` branch in `src/personas/industry-education-index/agent.lua` from:

```lua
if #evaluations == 0 then
  context.set_output("所有评估模型均未成功返回有效结果，请重试或更换模型。")
  return
end
```

to:

```lua
if #evaluations == 0 then
  context.set_output(format_failures(failures))
  return
end
```

- [ ] **Step 5: Run the partial-success regression test to verify it passes**

Run:

```bash
go test ./src/services/worker/internal/executor -run TestIndustryEducationIndexAgentLuaSeparateModeContinuesWithOneValidEvaluation -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the behavior change**

Run:

```bash
git add src/personas/industry-education-index/agent.lua src/services/worker/internal/executor/lua_test.go
git commit -m "fix: preserve separate evaluation results"
```

---

### Task 3: Prove all-invalid JSON failures include model-specific reasons

**Files:**
- Modify: `src/services/worker/internal/executor/lua_test.go`
- Test: `src/services/worker/internal/executor/lua_test.go`

- [ ] **Step 1: Write the failing all-invalid JSON test**

Add this test after the partial-success regression in `src/services/worker/internal/executor/lua_test.go`:

```go
func TestIndustryEducationIndexAgentLuaSeparateModeSummarizesInvalidEvaluatorOutputs(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../../../personas/industry-education-index/agent.lua")
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"available_models": []map[string]any{
			{"selector": "deepseek official^deepseek-chat", "model": "deepseek-chat", "provider_kind": "deepseek", "credential_name": "deepseek official"},
			{"selector": "qwen proxy^qwen-max", "model": "qwen-max", "provider_kind": "openai", "credential_name": "qwen proxy"},
		},
	}
	answers := []string{
		`{"mode":"分别评估"}`,
		`{"models":["deepseek official^deepseek-chat","qwen proxy^qwen-max"]}`,
	}
	rc.WaitForInput = func(_ context.Context) (string, bool) {
		if len(answers) == 0 {
			return "", false
		}
		answer := answers[0]
		answers = answers[1:]
		return answer, true
	}

	reg := tools.NewRegistry()
	for _, spec := range []tools.AgentToolSpec{
		{Name: "web_search", Version: "1", Description: "search", RiskLevel: tools.RiskLevelLow},
		{Name: "web_fetch", Version: "1", Description: "fetch", RiskLevel: tools.RiskLevelLow},
	} {
		if err := reg.Register(spec); err != nil {
			t.Fatalf("register %s: %v", spec.Name, err)
		}
	}
	dispatch := tools.NewDispatchingExecutor(reg, tools.NewPolicyEnforcer(reg, tools.AllowlistFromNames([]string{"web_search", "web_fetch"})))
	if err := dispatch.Bind("web_search", stubLuaToolExecutor{
		execute: func(_ context.Context, _ string, _ map[string]any, _ tools.ExecutionContext, _ string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"results": []map[string]any{{
					"title":   "深圳职业技术大学官网",
					"url":     "https://www.szpu.edu.cn/",
					"snippet": "学校官网、院系设置、人才培养与校园新闻。",
				}},
			}}
		},
	}); err != nil {
		t.Fatalf("bind web_search: %v", err)
	}
	if err := dispatch.Bind("web_fetch", stubLuaToolExecutor{
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"content": "学校围绕产教融合、校企合作和专业群建设开展人才培养。",
			}}
		},
	}); err != nil {
		t.Fatalf("bind web_fetch: %v", err)
	}
	rc.ToolExecutor = dispatch

	outputs := map[uuid.UUID]string{}
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			personaID := req.PersonaID
			if req.PersonaID == "industry-education-evaluator" {
				if req.Model == "deepseek official^deepseek-chat" {
					outputs[subAgentID] = `{"eligible":true,"model_label":"DeepSeek","dimensions":[{"name":"基础与机制","weight":25,"score":88.0,"data_confidence":"medium"}]}`
				} else {
					outputs[subAgentID] = `not-json`
				}
			}
			return subagentctl.StatusSnapshot{SubAgentID: subAgentID, Status: data.SubAgentStatusQueued, PersonaID: &personaID, ContextMode: req.ContextMode}, nil
		},
		wait: func(_ context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := req.SubAgentIDs[0]
			output := outputs[subAgentID]
			return subagentctl.StatusSnapshot{SubAgentID: subAgentID, Status: data.SubAgentStatusCompleted, LastOutput: &output}, nil
		},
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	texts := strings.Join(deltaTexts(evs), "")
	if !strings.Contains(texts, "失败摘要") {
		t.Fatalf("expected failure summary output, got %q", texts)
	}
	if !strings.Contains(texts, "DeepSeek") {
		t.Fatalf("expected model label in failure summary, got %q", texts)
	}
	if !strings.Contains(texts, "维度数量不完整") {
		t.Fatalf("expected dimensions reason, got %q", texts)
	}
	if !strings.Contains(texts, "Qwen") {
		t.Fatalf("expected second model label in failure summary, got %q", texts)
	}
	if !strings.Contains(texts, "返回格式不是有效 JSON") {
		t.Fatalf("expected invalid json reason, got %q", texts)
	}
}
```

- [ ] **Step 2: Run the new test to verify it fails first**

Run:

```bash
go test ./src/services/worker/internal/executor -run TestIndustryEducationIndexAgentLuaSeparateModeSummarizesInvalidEvaluatorOutputs -count=1
```

Expected: FAIL before the summary assertions pass.

- [ ] **Step 3: Keep the implementation from Task 2 and re-run the test**

Run the same command again after Task 2 is implemented.

Expected: PASS.

- [ ] **Step 4: Commit the invalid-output regression**

Run:

```bash
git add src/services/worker/internal/executor/lua_test.go
git commit -m "test: cover invalid evaluator summaries"
```

---

### Task 4: Prove timeout and empty-output failures are summarized

**Files:**
- Modify: `src/services/worker/internal/executor/lua_test.go`
- Test: `src/services/worker/internal/executor/lua_test.go`

- [ ] **Step 1: Write the failing timeout/empty-output test**

Add this test after the invalid-output regression in `src/services/worker/internal/executor/lua_test.go`:

```go
func TestIndustryEducationIndexAgentLuaSeparateModeSummarizesExecutionFailures(t *testing.T) {
	scriptBytes, err := os.ReadFile("../../../../personas/industry-education-index/agent.lua")
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"available_models": []map[string]any{
			{"selector": "deepseek official^deepseek-chat", "model": "deepseek-chat", "provider_kind": "deepseek", "credential_name": "deepseek official"},
			{"selector": "qwen proxy^qwen-max", "model": "qwen-max", "provider_kind": "openai", "credential_name": "qwen proxy"},
		},
	}
	answers := []string{
		`{"mode":"分别评估"}`,
		`{"models":["deepseek official^deepseek-chat","qwen proxy^qwen-max"]}`,
	}
	rc.WaitForInput = func(_ context.Context) (string, bool) {
		if len(answers) == 0 {
			return "", false
		}
		answer := answers[0]
		answers = answers[1:]
		return answer, true
	}

	reg := tools.NewRegistry()
	for _, spec := range []tools.AgentToolSpec{
		{Name: "web_search", Version: "1", Description: "search", RiskLevel: tools.RiskLevelLow},
		{Name: "web_fetch", Version: "1", Description: "fetch", RiskLevel: tools.RiskLevelLow},
	} {
		if err := reg.Register(spec); err != nil {
			t.Fatalf("register %s: %v", spec.Name, err)
		}
	}
	dispatch := tools.NewDispatchingExecutor(reg, tools.NewPolicyEnforcer(reg, tools.AllowlistFromNames([]string{"web_search", "web_fetch"})))
	if err := dispatch.Bind("web_search", stubLuaToolExecutor{
		execute: func(_ context.Context, _ string, _ map[string]any, _ tools.ExecutionContext, _ string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"results": []map[string]any{{
					"title":   "深圳职业技术大学官网",
					"url":     "https://www.szpu.edu.cn/",
					"snippet": "学校官网、院系设置、人才培养与校园新闻。",
				}},
			}}
		},
	}); err != nil {
		t.Fatalf("bind web_search: %v", err)
	}
	if err := dispatch.Bind("web_fetch", stubLuaToolExecutor{
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"content": "学校围绕产教融合、校企合作和专业群建设开展人才培养。",
			}}
		},
	}); err != nil {
		t.Fatalf("bind web_fetch: %v", err)
	}
	rc.ToolExecutor = dispatch

	outputs := map[uuid.UUID]string{}
	errs := map[uuid.UUID]error{}
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			personaID := req.PersonaID
			if req.PersonaID == "industry-education-evaluator" {
				if req.Model == "deepseek official^deepseek-chat" {
					errs[subAgentID] = errors.New("deadline exceeded")
				} else {
					outputs[subAgentID] = ""
				}
			}
			return subagentctl.StatusSnapshot{SubAgentID: subAgentID, Status: data.SubAgentStatusQueued, PersonaID: &personaID, ContextMode: req.ContextMode}, nil
		},
		wait: func(_ context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := req.SubAgentIDs[0]
			if err := errs[subAgentID]; err != nil {
				return subagentctl.StatusSnapshot{}, err
			}
			output := outputs[subAgentID]
			return subagentctl.StatusSnapshot{SubAgentID: subAgentID, Status: data.SubAgentStatusCompleted, LastOutput: &output}, nil
		},
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	texts := strings.Join(deltaTexts(evs), "")
	if !strings.Contains(texts, "失败摘要") {
		t.Fatalf("expected failure summary output, got %q", texts)
	}
	if !strings.Contains(texts, "DeepSeek") || !strings.Contains(texts, "执行失败或超时") {
		t.Fatalf("expected timeout reason for DeepSeek, got %q", texts)
	}
	if !strings.Contains(texts, "Qwen") || !strings.Contains(texts, "未返回内容") {
		t.Fatalf("expected empty-output reason for Qwen, got %q", texts)
	}
}
```

- [ ] **Step 2: Run the new test to verify it fails first**

Run:

```bash
go test ./src/services/worker/internal/executor -run TestIndustryEducationIndexAgentLuaSeparateModeSummarizesExecutionFailures -count=1
```

Expected: FAIL before the summary assertions pass.

- [ ] **Step 3: Re-run the test after Task 2 implementation**

Run the same command again.

Expected: PASS.

- [ ] **Step 4: Commit the execution-failure regression**

Run:

```bash
git add src/services/worker/internal/executor/lua_test.go
git commit -m "test: cover evaluator execution summaries"
```

---

### Task 5: Run the focused verification suite and inspect regression safety

**Files:**
- Modify: `src/personas/industry-education-index/agent.lua`
- Modify: `src/services/worker/internal/executor/lua_test.go`
- Test: `src/services/worker/internal/executor/lua_test.go`

- [ ] **Step 1: Run all three new regressions together**

Run:

```bash
go test ./src/services/worker/internal/executor -run 'TestIndustryEducationIndexAgentLuaSeparateMode(ContinuesWithOneValidEvaluation|SummarizesInvalidEvaluatorOutputs|SummarizesExecutionFailures)' -count=1
```

Expected: PASS.

- [ ] **Step 2: Run the existing industry-education Lua test**

Run:

```bash
go test ./src/services/worker/internal/executor -run TestIndustryEducationIndexAgentLuaRunsWithSubAgentsAndArtifacts -count=1
```

Expected: PASS.

- [ ] **Step 3: Run the full executor package tests**

Run:

```bash
go test ./src/services/worker/internal/executor -count=1
```

Expected: PASS.

- [ ] **Step 4: Review the final diff before handing off**

Run:

```bash
git diff -- src/personas/industry-education-index/agent.lua src/services/worker/internal/executor/lua_test.go
```

Confirm the diff is limited to:

- returning validation reasons from `validate_evaluation`
- summarizing failures in `wait_for_evaluators`
- formatting the all-failed message from collected failures
- the three new regression tests

- [ ] **Step 5: Commit the final verified patch**

Run:

```bash
git add src/personas/industry-education-index/agent.lua src/services/worker/internal/executor/lua_test.go
git commit -m "fix: summarize separate evaluation failures"
```

---

## Self-Review

- **Spec coverage:** Task 1 proves partial success keeps `分别评估` alive. Task 2 implements validation reasons and all-failed summaries in Lua. Tasks 3 and 4 prove invalid JSON, timeout, and empty-output failure summaries. Task 5 verifies no regression in the existing industry-education flow and the executor package.
- **Placeholder scan:** No TODO/TBD markers remain. Every task lists exact files, test code, shell commands, and expected outcomes.
- **Type consistency:** The plan consistently uses `validate_evaluation(raw_text) -> evaluation, reason`, `failures = { model = item.model, error = ... }`, and `format_failures(failures)` across implementation and tests.
