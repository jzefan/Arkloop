# 产教融合评估搜索超时修复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reduce timeout failures for the industry-education evaluation persona under `web_search.basic`, and improve the timeout message so users can immediately see the active provider and the next corrective step.

**Architecture:** Keep the fix inside the `industry-education-index` Lua persona and its executor tests. Shrink the web search query fan-out from 5 to 3, surface the active web search provider name in timeout errors, and add a provider-specific hint when the active provider is `web_search.basic`.

**Tech Stack:** Lua persona runtime, Go executor tests, Vitest-free Go test workflow, existing `web_search` and `web_fetch` stubs in `src/services/worker/internal/executor/lua_test.go`.

---

## Files And Responsibilities

- Modify `src/personas/industry-education-index/agent.lua`: reduce search query count, derive active web_search provider name from runtime context, and improve timeout diagnostics.
- Modify `src/services/worker/internal/executor/lua_test.go`: add focused timeout-diagnostic regressions and tighten the existing happy-path assertion around emitted queries.

---

### Task 1: Prove the persona currently over-fans search queries

**Files:**
- Modify: `src/services/worker/internal/executor/lua_test.go`
- Test: `src/services/worker/internal/executor/lua_test.go`

- [ ] **Step 1: Add the failing query-count regression**

In `src/services/worker/internal/executor/lua_test.go`, extend `TestIndustryEducationIndexAgentLuaRunsWithSubAgentsAndArtifacts` with this assertion immediately after `if len(searchQueries) == 0 { t.Fatal(...) }`:

```go
	if len(searchQueries) != 3 {
		t.Fatalf("expected 3 search queries, got %d: %v", len(searchQueries), searchQueries)
	}
```

Keep the existing UTF-8 and school-name assertions below it.

- [ ] **Step 2: Run the focused test to verify it fails**

Run:

```bash
go test ./src/services/worker/internal/executor -run TestIndustryEducationIndexAgentLuaRunsWithSubAgentsAndArtifacts -count=1
```

Expected: FAIL because the current persona still emits 5 search queries.

- [ ] **Step 3: Do not change production code yet**

Leave `src/personas/industry-education-index/agent.lua` untouched until Task 2.

---

### Task 2: Reduce query fan-out and add provider-aware timeout messaging

**Files:**
- Modify: `src/personas/industry-education-index/agent.lua`
- Test: `src/services/worker/internal/executor/lua_test.go`

- [ ] **Step 1: Add provider-name helpers in Lua**

In `src/personas/industry-education-index/agent.lua`, insert these helpers above `search_sources`:

```lua
local function active_tool_provider_configs_by_group()
  local raw = context.get("active_tool_provider_configs_by_group")
  if raw == nil or raw == "" then return {} end
  local parsed, err = json.decode(raw)
  if err ~= nil or parsed == nil then return {} end
  return parsed
end

local function active_web_search_provider_name()
  local configs = active_tool_provider_configs_by_group()
  local cfg = configs.web_search
  if type(cfg) == "table" then
    local provider_name = trim(cfg.provider_name)
    if provider_name ~= "" then return provider_name end
  end
  return "web_search"
end

local function search_error_message(provider_name, search_err)
  local raw = tostring(search_err or "empty_results")
  local lines = {
    "当前联网搜索 provider：" .. provider_name,
  }
  if string.find(raw, "timed out", 1, true) ~= nil then
    table.insert(lines, "搜索在工具超时窗口内未完成。")
  else
    table.insert(lines, "当前搜索提供商未返回可用于核验院校身份的公开结果。")
  end
  if provider_name == "web_search.basic" then
    table.insert(lines, "web_search.basic 依赖本地桌面 browser-search 端点；本地代理不可用或响应慢时容易超时。")
  end
  table.insert(lines, "请检查“接入”中的联网搜索配置，建议切换到 Tavily 或 SearXNG 后重试。")
  table.insert(lines, "原始信息：" .. raw)
  return table.concat(lines, "\n")
end
```

- [ ] **Step 2: Reduce the query list from 5 to 3**

Replace the query construction in `search_sources` from:

```lua
  local queries = {
    school_name,
    school_name .. " 官网",
    school_name .. " 学校官网 高职 院校 简介",
    school_name .. " 双高计划 高水平高职学校 专业群",
    school_name .. " 产教融合 校企合作 实训基地 " .. year,
  }
  if trim(analysis_focus) ~= "" then
    queries[#queries] = school_name .. " " .. analysis_focus
  end
```

to:

```lua
  local combined_focus = school_name .. " 高职 双高 产教融合 校企合作 " .. year
  if trim(analysis_focus) ~= "" then
    combined_focus = combined_focus .. " " .. trim(analysis_focus)
  end
  local queries = {
    school_name,
    school_name .. " 官网",
    combined_focus,
  }
```

- [ ] **Step 3: Replace the generic search failure message**

Change the failure branch in `search_sources` from:

```lua
  if search_err ~= nil or search_result == nil or search_result.results == nil or #search_result.results == 0 then
    return nil, "当前搜索提供商未返回可用于核验院校身份的公开结果。请检查“接入”中的联网搜索配置，建议切换到 Tavily 或 SearXNG 后重试。原始信息：" .. tostring(search_err or "empty_results")
  end
```

to:

```lua
  if search_err ~= nil or search_result == nil or search_result.results == nil or #search_result.results == 0 then
    return nil, search_error_message(active_web_search_provider_name(), search_err or "empty_results")
  end
```

- [ ] **Step 4: Re-run the happy-path test to verify the query-count regression passes**

Run:

```bash
go test ./src/services/worker/internal/executor -run TestIndustryEducationIndexAgentLuaRunsWithSubAgentsAndArtifacts -count=1
```

Expected: PASS.

---

### Task 3: Prove timeout diagnostics include provider context

**Files:**
- Modify: `src/services/worker/internal/executor/lua_test.go`
- Test: `src/services/worker/internal/executor/lua_test.go`

- [ ] **Step 1: Add a failing basic-provider timeout regression**

Add this new test in `src/services/worker/internal/executor/lua_test.go` near the other industry-education tests:

```go
func TestIndustryEducationIndexAgentLuaSearchTimeoutMentionsBasicProvider(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"available_models": []map[string]any{{
			"selector": "deepseek official^deepseek-chat",
			"model": "deepseek-chat",
			"provider_kind": "deepseek",
			"credential_name": "deepseek official",
		}},
		"active_tool_provider_configs_by_group": map[string]any{
			"web_search": map[string]any{"provider_name": "web_search.basic"},
		},
	}
	answers := []string{
		`{"mode":"综合评估"}`,
		`{"models":["deepseek official^deepseek-chat"]}`,
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
	if err := reg.Register(tools.AgentToolSpec{Name: "web_search", Version: "1", Description: "search", RiskLevel: tools.RiskLevelLow}); err != nil {
		t.Fatalf("register web_search: %v", err)
	}
	dispatch := tools.NewDispatchingExecutor(reg, tools.NewPolicyEnforcer(reg, tools.AllowlistFromNames([]string{"web_search"})))
	if err := dispatch.Bind("web_search", stubLuaToolExecutor{
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{Error: &tools.ExecutionError{ErrorClass: "tool.timeout", Message: "web_search timed out"}}
		},
	}); err != nil {
		t.Fatalf("bind web_search: %v", err)
	}
	rc.ToolExecutor = dispatch

	evs := runLuaScript(t, string(scriptBytes), rc)
	texts := strings.Join(deltaTexts(evs), "")
	if !strings.Contains(texts, "当前联网搜索 provider：web_search.basic") {
		t.Fatalf("expected basic provider in timeout message, got %q", texts)
	}
	if !strings.Contains(texts, "搜索在工具超时窗口内未完成") {
		t.Fatalf("expected timeout explanation, got %q", texts)
	}
	if !strings.Contains(texts, "browser-search 端点") {
		t.Fatalf("expected desktop browser-search hint, got %q", texts)
	}
	if !strings.Contains(texts, "Tavily") || !strings.Contains(texts, "SearXNG") {
		t.Fatalf("expected provider switch guidance, got %q", texts)
	}
}
```

- [ ] **Step 2: Run the new test to verify it fails first**

Run:

```bash
go test ./src/services/worker/internal/executor -run TestIndustryEducationIndexAgentLuaSearchTimeoutMentionsBasicProvider -count=1
```

Expected: FAIL before Task 2 is implemented.

- [ ] **Step 3: Re-run after Task 2 and confirm pass**

Run the same command again after Task 2 changes are in place.

Expected: PASS.

---

### Task 4: Prove non-basic provider timeout does not claim desktop proxy dependency

**Files:**
- Modify: `src/services/worker/internal/executor/lua_test.go`
- Test: `src/services/worker/internal/executor/lua_test.go`

- [ ] **Step 1: Add a Tavily timeout regression**

Add this test after the basic-provider timeout regression:

```go
func TestIndustryEducationIndexAgentLuaSearchTimeoutMentionsProviderWithoutBasicHint(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"available_models": []map[string]any{{
			"selector": "deepseek official^deepseek-chat",
			"model": "deepseek-chat",
			"provider_kind": "deepseek",
			"credential_name": "deepseek official",
		}},
		"active_tool_provider_configs_by_group": map[string]any{
			"web_search": map[string]any{"provider_name": "web_search.tavily"},
		},
	}
	answers := []string{
		`{"mode":"综合评估"}`,
		`{"models":["deepseek official^deepseek-chat"]}`,
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
	if err := reg.Register(tools.AgentToolSpec{Name: "web_search", Version: "1", Description: "search", RiskLevel: tools.RiskLevelLow}); err != nil {
		t.Fatalf("register web_search: %v", err)
	}
	dispatch := tools.NewDispatchingExecutor(reg, tools.NewPolicyEnforcer(reg, tools.AllowlistFromNames([]string{"web_search"})))
	if err := dispatch.Bind("web_search", stubLuaToolExecutor{
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{Error: &tools.ExecutionError{ErrorClass: "tool.timeout", Message: "web_search timed out"}}
		},
	}); err != nil {
		t.Fatalf("bind web_search: %v", err)
	}
	rc.ToolExecutor = dispatch

	evs := runLuaScript(t, string(scriptBytes), rc)
	texts := strings.Join(deltaTexts(evs), "")
	if !strings.Contains(texts, "当前联网搜索 provider：web_search.tavily") {
		t.Fatalf("expected tavily provider in timeout message, got %q", texts)
	}
	if strings.Contains(texts, "browser-search 端点") {
		t.Fatalf("did not expect basic-provider desktop hint, got %q", texts)
	}
}
```

- [ ] **Step 2: Run the Tavily timeout test**

Run:

```bash
go test ./src/services/worker/internal/executor -run TestIndustryEducationIndexAgentLuaSearchTimeoutMentionsProviderWithoutBasicHint -count=1
```

Expected: PASS after Task 2 changes are in place.

---

### Task 5: Run final focused verification

**Files:**
- Modify: `src/personas/industry-education-index/agent.lua`
- Modify: `src/services/worker/internal/executor/lua_test.go`

- [ ] **Step 1: Run all search-timeout focused tests together**

Run:

```bash
go test ./src/services/worker/internal/executor -run 'TestIndustryEducationIndexAgentLua(RunsWithSubAgentsAndArtifacts|SearchTimeoutMentionsBasicProvider|SearchTimeoutMentionsProviderWithoutBasicHint)' -count=1
```

Expected: PASS.

- [ ] **Step 2: Inspect the final diff**

Run:

```bash
git -C "/Users/jzefan/work/proj/ArkLoop" diff -- src/personas/industry-education-index/agent.lua src/services/worker/internal/executor/lua_test.go
```

Confirm the diff is limited to:

- reducing search query fan-out
- deriving provider name from runtime context
- improving search failure diagnostics
- the new executor regressions and tighter happy-path assertion

- [ ] **Step 3: Commit the verified patch**

Run:

```bash
git -C "/Users/jzefan/work/proj/ArkLoop" add src/personas/industry-education-index/agent.lua src/services/worker/internal/executor/lua_test.go
git -C "/Users/jzefan/work/proj/ArkLoop" commit -m "fix: reduce evaluation search timeout risk"
```

---

## Self-Review

- **Spec coverage:** Task 1 proves the old search fan-out is too large for the expected contract. Task 2 reduces query count and adds provider-aware timeout messaging. Tasks 3 and 4 prove timeout diagnostics differ correctly for `web_search.basic` vs. non-basic providers. Task 5 verifies the focused path end-to-end and checks diff scope.
- **Placeholder scan:** No TODO/TBD markers remain. Every task lists exact files, code blocks, commands, and expected outcomes.
- **Type consistency:** The plan consistently uses `active_tool_provider_configs_by_group`, `provider_name`, and `search_error_message(provider_name, search_err)` across the implementation and tests.
