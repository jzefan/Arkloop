package executor

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"arkloop/services/shared/rollout"
	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/events"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/memory"
	"arkloop/services/worker/internal/pipeline"
	"arkloop/services/worker/internal/routing"
	"arkloop/services/worker/internal/subagentctl"
	"arkloop/services/worker/internal/tools"
	"github.com/google/uuid"
)

type stubSubAgentControl struct {
	spawn     func(ctx context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error)
	sendInput func(ctx context.Context, req subagentctl.SendInputRequest) (subagentctl.StatusSnapshot, error)
	wait      func(ctx context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error)
	resume    func(ctx context.Context, req subagentctl.ResumeRequest) (subagentctl.StatusSnapshot, error)
	close     func(ctx context.Context, req subagentctl.CloseRequest) (subagentctl.StatusSnapshot, error)
	interrupt func(ctx context.Context, req subagentctl.InterruptRequest) (subagentctl.StatusSnapshot, error)
	getStatus func(ctx context.Context, subAgentID uuid.UUID) (subagentctl.StatusSnapshot, error)
	list      func(ctx context.Context) ([]subagentctl.StatusSnapshot, error)
}

type stubLuaToolExecutor struct {
	execute func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult
}

func (e stubLuaToolExecutor) Execute(ctx context.Context, toolName string, args map[string]any, execCtx tools.ExecutionContext, toolCallID string) tools.ExecutionResult {
	return e.execute(ctx, toolName, args, execCtx, toolCallID)
}

func (s stubSubAgentControl) Spawn(ctx context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
	if s.spawn == nil {
		return subagentctl.StatusSnapshot{}, errors.New("spawn not implemented")
	}
	return s.spawn(ctx, req)
}

func (s stubSubAgentControl) SendInput(ctx context.Context, req subagentctl.SendInputRequest) (subagentctl.StatusSnapshot, error) {
	if s.sendInput == nil {
		return subagentctl.StatusSnapshot{}, errors.New("send_input not implemented")
	}
	return s.sendInput(ctx, req)
}

func (s stubSubAgentControl) Wait(ctx context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
	if s.wait == nil {
		return subagentctl.StatusSnapshot{}, errors.New("wait not implemented")
	}
	return s.wait(ctx, req)
}

func (s stubSubAgentControl) Resume(ctx context.Context, req subagentctl.ResumeRequest) (subagentctl.StatusSnapshot, error) {
	if s.resume == nil {
		return subagentctl.StatusSnapshot{}, errors.New("resume not implemented")
	}
	return s.resume(ctx, req)
}

func (s stubSubAgentControl) Close(ctx context.Context, req subagentctl.CloseRequest) (subagentctl.StatusSnapshot, error) {
	if s.close == nil {
		return subagentctl.StatusSnapshot{}, errors.New("close not implemented")
	}
	return s.close(ctx, req)
}

func (s stubSubAgentControl) Interrupt(ctx context.Context, req subagentctl.InterruptRequest) (subagentctl.StatusSnapshot, error) {
	if s.interrupt == nil {
		return subagentctl.StatusSnapshot{}, errors.New("interrupt not implemented")
	}
	return s.interrupt(ctx, req)
}

func (s stubSubAgentControl) GetStatus(ctx context.Context, subAgentID uuid.UUID) (subagentctl.StatusSnapshot, error) {
	if s.getStatus == nil {
		return subagentctl.StatusSnapshot{}, errors.New("get_status not implemented")
	}
	return s.getStatus(ctx, subAgentID)
}

func (s stubSubAgentControl) ListChildren(ctx context.Context) ([]subagentctl.StatusSnapshot, error) {
	if s.list == nil {
		return nil, errors.New("list not implemented")
	}
	return s.list(ctx)
}

func (s stubSubAgentControl) GetRolloutRecorder(uuid.UUID) (*rollout.Recorder, bool) {
	return nil, false
}

func newOutputControl(run func(personaID string, input string) (string, error)) stubSubAgentControl {
	var (
		mu      sync.Mutex
		outputs = map[uuid.UUID]string{}
		errs    = map[uuid.UUID]error{}
	)
	return stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			output, err := run(req.PersonaID, req.Input)
			mu.Lock()
			outputs[subAgentID] = output
			errs[subAgentID] = err
			mu.Unlock()
			personaID := req.PersonaID
			return subagentctl.StatusSnapshot{
				SubAgentID:  subAgentID,
				Status:      data.SubAgentStatusQueued,
				PersonaID:   &personaID,
				ContextMode: req.ContextMode,
			}, nil
		},
		wait: func(_ context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			if len(req.SubAgentIDs) == 0 {
				return subagentctl.StatusSnapshot{}, errors.New("missing sub_agent_ids")
			}
			subAgentID := req.SubAgentIDs[0]
			mu.Lock()
			output := outputs[subAgentID]
			err := errs[subAgentID]
			mu.Unlock()
			if err != nil {
				return subagentctl.StatusSnapshot{}, err
			}
			return subagentctl.StatusSnapshot{SubAgentID: subAgentID, Status: data.SubAgentStatusCompleted, LastOutput: &output}, nil
		},
	}
}

func TestNewLuaExecutor_MissingScript(t *testing.T) {
	_, err := NewLuaExecutor(map[string]any{})
	if err == nil {
		t.Fatal("expected error when script is missing")
	}
}

func TestNewLuaExecutor_EmptyScript(t *testing.T) {
	_, err := NewLuaExecutor(map[string]any{"script": "   "})
	if err == nil {
		t.Fatal("expected error when script is blank")
	}
}

func TestNewLuaExecutor_ValidScript(t *testing.T) {
	ex, err := NewLuaExecutor(map[string]any{"script": "context.set_output('hello')"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ex == nil {
		t.Fatal("factory returned nil")
	}
}

func TestIndustryEducationEvaluatorPrompt_RequiresEligibleTrueWhenEligibilityVerified(t *testing.T) {
	promptPath := filepath.Join("..", "..", "..", "..", "personas", "industry-education-evaluator", "prompt.md")
	prompt, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}

	const requiredClause = "当 fact_pack.eligibility.verified 为 true 时，必须输出 `eligible: true`"
	if !strings.Contains(string(prompt), requiredClause) {
		t.Fatalf("prompt must contain contract clause %q", requiredClause)
	}
	for _, clause := range []string{
		"可以基于事实包中的公开事实、评分框架和你的专业判断进行分析性评分",
		"不要因为就业率、企业满意度等量化指标缺失就拒绝评分",
		"缺失指标应降低 `data_confidence`",
	} {
		if !strings.Contains(string(prompt), clause) {
			t.Fatalf("prompt must contain analysis clause %q", clause)
		}
	}
}

func TestLuaExecutor_ContextSetOutput(t *testing.T) {
	ex, _ := NewLuaExecutor(map[string]any{
		"script": `context.set_output("hello from lua")`,
	})
	rc := buildLuaRC(nil)
	emitter := events.NewEmitter("trace")

	var got []events.RunEvent
	err := ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error {
		got = append(got, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var deltaTexts []string
	completedCount := 0
	for _, ev := range got {
		switch ev.Type {
		case "message.delta":
			if delta, ok := ev.DataJSON["content_delta"].(string); ok {
				deltaTexts = append(deltaTexts, delta)
			}
		case "run.completed":
			completedCount++
		}
	}
	if len(deltaTexts) == 0 || deltaTexts[0] != "hello from lua" {
		t.Fatalf("expected message.delta with 'hello from lua', got: %v", deltaTexts)
	}
	if completedCount != 1 {
		t.Fatalf("expected 1 run.completed, got %d", completedCount)
	}
}

func TestLuaExecutor_NoOutput_StillCompletes(t *testing.T) {
	ex, _ := NewLuaExecutor(map[string]any{
		"script": `local x = 1 + 1`,
	})
	rc := buildLuaRC(nil)
	emitter := events.NewEmitter("trace")

	var got []events.RunEvent
	err := ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error {
		got = append(got, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	completedCount := 0
	deltaCount := 0
	for _, ev := range got {
		if ev.Type == "run.completed" {
			completedCount++
		}
		if ev.Type == "message.delta" {
			deltaCount++
		}
	}
	if completedCount != 1 {
		t.Fatalf("expected 1 run.completed, got %d", completedCount)
	}
	if deltaCount != 0 {
		t.Fatalf("expected no message.delta when no output set, got %d", deltaCount)
	}
}

func TestLuaExecutor_ScriptError_EmitsRunFailed(t *testing.T) {
	ex, _ := NewLuaExecutor(map[string]any{
		"script": `this is not valid lua @@@@`,
	})
	rc := buildLuaRC(nil)
	emitter := events.NewEmitter("trace")

	var got []events.RunEvent
	err := ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error {
		got = append(got, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("Execute returned unexpected error: %v", err)
	}

	var failedCount int
	for _, ev := range got {
		if ev.Type == "run.failed" {
			failedCount++
			if ec, ok := ev.DataJSON["error_class"].(string); !ok || ec != "agent.lua.script_error" {
				t.Fatalf("expected error_class=agent.lua.script_error, got: %v", ev.DataJSON["error_class"])
			}
		}
	}
	if failedCount != 1 {
		t.Fatalf("expected 1 run.failed, got %d", failedCount)
	}
}

func TestLuaExecutor_ContextGet(t *testing.T) {
	ex, _ := NewLuaExecutor(map[string]any{
		"script": `
local v = context.get("user_prompt")
context.set_output(v)
`,
	})
	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{"user_prompt": "test input"}

	emitter := events.NewEmitter("trace")
	var got []events.RunEvent
	err := ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error {
		got = append(got, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	for _, ev := range got {
		if ev.Type == "message.delta" {
			if delta, ok := ev.DataJSON["content_delta"].(string); ok && delta == "test input" {
				return
			}
		}
	}
	t.Fatal("expected message.delta with 'test input'")
}

func TestLuaExecutor_ContextGet_MissingKey(t *testing.T) {
	ex, _ := NewLuaExecutor(map[string]any{
		"script": `
local v = context.get("nonexistent")
if v == nil then
  context.set_output("nil_ok")
end
`,
	})
	rc := buildLuaRC(nil)
	emitter := events.NewEmitter("trace")
	var got []events.RunEvent
	err := ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error {
		got = append(got, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	for _, ev := range got {
		if ev.Type == "message.delta" {
			if delta, ok := ev.DataJSON["content_delta"].(string); ok && delta == "nil_ok" {
				return
			}
		}
	}
	t.Fatal("expected message.delta with 'nil_ok'")
}

func TestLuaExecutor_MemorySearch_Stub(t *testing.T) {
	ex, _ := NewLuaExecutor(map[string]any{
		"script": `
local results, err = memory.search("test query")
if err then error(err) end
context.set_output(results)
`,
	})
	rc := buildLuaRC(nil)
	emitter := events.NewEmitter("trace")
	var got []events.RunEvent
	err := ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error {
		got = append(got, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	for _, ev := range got {
		if ev.Type == "message.delta" {
			if delta, _ := ev.DataJSON["content_delta"].(string); delta == "[]" {
				return
			}
		}
	}
	t.Fatal("expected memory.search stub to return '[]'")
}

func TestDefaultExecutorRegistry_ContainsAgentLua(t *testing.T) {
	reg := DefaultExecutorRegistry()
	ex, err := reg.Build("agent.lua", map[string]any{"script": "context.set_output('ok')"})
	if err != nil {
		t.Fatalf("Build agent.lua failed: %v", err)
	}
	if ex == nil {
		t.Fatal("Build returned nil")
	}
}

// buildLuaRC 构建适合 LuaExecutor 测试的最小 RunContext。
func buildLuaRC(gateway llm.Gateway) *pipeline.RunContext {
	rc := &pipeline.RunContext{
		Run: data.Run{
			ID:        uuid.New(),
			AccountID: uuid.New(),
			ThreadID:  uuid.New(),
		},
		TraceID:                "lua-test-trace",
		InputJSON:              map[string]any{},
		ReasoningIterations:    10,
		ToolContinuationBudget: 32,
		ToolBudget:             map[string]any{},
		PerToolSoftLimits:      tools.DefaultPerToolSoftLimits(),
	}
	if gateway != nil {
		rc.Gateway = gateway
		rc.SelectedRoute = &routing.SelectedProviderRoute{
			Route: routing.ProviderRouteRule{
				ID:    "default",
				Model: "stub",
			},
		}
	}
	return rc
}

type luaSeqGateway struct {
	events []llm.StreamEvent
}

func (g *luaSeqGateway) Stream(_ context.Context, _ llm.Request, yield func(llm.StreamEvent) error) error {
	for _, event := range g.events {
		if err := yield(event); err != nil {
			return err
		}
	}
	return nil
}

// --- mock MemoryProvider for Lua binding tests ---

type luaMemMock struct {
	findHits    []memory.MemoryHit
	findErr     error
	contentText string
	contentErr  error
	writeErr    error
	deleteErr   error
}

func (m *luaMemMock) Find(_ context.Context, _ memory.MemoryIdentity, _ string, _ string, _ int) ([]memory.MemoryHit, error) {
	return m.findHits, m.findErr
}

func (m *luaMemMock) Content(_ context.Context, _ memory.MemoryIdentity, _ string, _ memory.MemoryLayer) (string, error) {
	return m.contentText, m.contentErr
}

func (m *luaMemMock) AppendSessionMessages(_ context.Context, _ memory.MemoryIdentity, _ string, _ []memory.MemoryMessage) error {
	return nil
}

func (m *luaMemMock) CommitSession(_ context.Context, _ memory.MemoryIdentity, _ string) error {
	return nil
}

func (m *luaMemMock) Write(_ context.Context, _ memory.MemoryIdentity, _ memory.MemoryScope, _ memory.MemoryEntry) error {
	return m.writeErr
}

func (m *luaMemMock) Delete(_ context.Context, _ memory.MemoryIdentity, _ string) error {
	return m.deleteErr
}

func (m *luaMemMock) ListDir(_ context.Context, _ memory.MemoryIdentity, _ string) ([]string, error) {
	return nil, nil
}

// buildLuaRCWithMemory 构造注入了 MemoryProvider 和 UserID 的 RunContext。
func buildLuaRCWithMemory(mp memory.MemoryProvider) *pipeline.RunContext {
	rc := buildLuaRC(nil)
	uid := uuid.New()
	rc.UserID = &uid
	rc.MemoryProvider = mp
	return rc
}

func runLuaScript(t *testing.T, script string, rc *pipeline.RunContext) []events.RunEvent {
	t.Helper()
	ex, err := NewLuaExecutor(map[string]any{"script": script})
	if err != nil {
		t.Fatalf("NewLuaExecutor failed: %v", err)
	}
	emitter := events.NewEmitter("trace")
	var got []events.RunEvent
	if err := ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error {
		got = append(got, ev)
		return nil
	}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	return got
}

func deltaTexts(evs []events.RunEvent) []string {
	var out []string
	for _, ev := range evs {
		if ev.Type == "message.delta" {
			if d, ok := ev.DataJSON["content_delta"].(string); ok {
				out = append(out, d)
			}
		}
	}
	return out
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(encoded)
}

func TestLuaExecutor_SubAgentLegacyBindingsRemoved(t *testing.T) {
	evs := runLuaScript(t, `
if agent.run == nil and agent.run_parallel == nil then
  context.set_output("legacy_removed")
else
  context.set_output("legacy_present")
end
`, buildLuaRC(nil))

	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != "legacy_removed" {
		t.Fatalf("expected legacy bindings removed, got: %v", texts)
	}
}

func TestLuaExecutor_SubAgentSpawn_Unavailable(t *testing.T) {
	evs := runLuaScript(t, `
local status, err = agent.spawn({ persona_id = "lite", input = "hello" })
if status ~= nil then
  error("unexpected status")
end
context.set_output(err or "")
`, buildLuaRC(nil))

	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != "agent.spawn not available: SubAgentControl not initialized" {
		t.Fatalf("unexpected spawn unavailable output: %v", texts)
	}
}

func TestLuaExecutor_SubAgentSpawnWait_Success(t *testing.T) {
	var captured subagentctl.SpawnRequest
	rc := buildLuaRC(nil)
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			captured = req
			subAgentID := uuid.New()
			personaID := req.PersonaID
			return subagentctl.StatusSnapshot{
				SubAgentID:  subAgentID,
				Status:      data.SubAgentStatusQueued,
				PersonaID:   &personaID,
				ContextMode: req.ContextMode,
			}, nil
		},
		wait: func(_ context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			if len(req.SubAgentIDs) == 0 {
				return subagentctl.StatusSnapshot{}, errors.New("missing sub_agent_ids")
			}
			output := "4"
			return subagentctl.StatusSnapshot{
				SubAgentID:  req.SubAgentIDs[0],
				Status:      data.SubAgentStatusCompleted,
				ContextMode: data.SubAgentContextModeIsolated,
				LastOutput:  &output,
			}, nil
		},
	}

	evs := runLuaScript(t, `
local spawned, spawn_err = agent.spawn({ persona_id = "lite", input = "what is 2+2?" })
if spawn_err ~= nil then error(spawn_err) end
if spawned.id == nil or spawned.status ~= "queued" or spawned.context_mode ~= "isolated" then
  error("bad spawn status")
end
local waited, wait_err = agent.wait(spawned.id)
if wait_err ~= nil then error(wait_err) end
context.set_output(waited.output or "")
`, rc)

	if captured.PersonaID != "lite" {
		t.Fatalf("expected persona lite, got %q", captured.PersonaID)
	}
	if captured.Input != "what is 2+2?" {
		t.Fatalf("unexpected input: %q", captured.Input)
	}
	if captured.ContextMode != data.SubAgentContextModeIsolated {
		t.Fatalf("expected isolated context mode, got %q", captured.ContextMode)
	}
	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != "4" {
		t.Fatalf("expected waited output 4, got: %v", texts)
	}
	var spawnCall, spawnResult, waitCall, waitResult bool
	for _, ev := range evs {
		if ev.ToolName == nil {
			continue
		}
		switch ev.Type + ":" + *ev.ToolName {
		case "tool.call:agent.spawn":
			spawnCall = true
		case "tool.result:agent.spawn":
			spawnResult = true
		case "tool.call:agent.wait":
			waitCall = true
		case "tool.result:agent.wait":
			waitResult = true
		}
	}
	if !spawnCall || !spawnResult || !waitCall || !waitResult {
		t.Fatalf("expected agent spawn/wait tool events, got spawnCall=%v spawnResult=%v waitCall=%v waitResult=%v", spawnCall, spawnResult, waitCall, waitResult)
	}
}

func TestLuaAgentSpawnAcceptsModelOverride(t *testing.T) {
	var captured subagentctl.SpawnRequest
	rc := buildLuaRC(nil)
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			captured = req
			subAgentID := uuid.New()
			return subagentctl.StatusSnapshot{
				SubAgentID:  subAgentID,
				Status:      data.SubAgentStatusRunning,
				ContextMode: req.ContextMode,
			}, nil
		},
	}

	runLuaScript(t, `
local child, err = agent.spawn({
  persona_id = "industry-education-evaluator",
  input = "score this school",
  context_mode = "isolated",
  profile = "task",
  model = "deepseek官方^deepseek-chat"
})
if err ~= nil then error(err) end
`, rc)

	if captured.Model != "deepseek官方^deepseek-chat" {
		t.Fatalf("expected model override, got %q", captured.Model)
	}
}

func TestLuaExecutor_SubAgentSend_Interrupt(t *testing.T) {
	subAgentID := uuid.New()
	var captured subagentctl.SendInputRequest
	rc := buildLuaRC(nil)
	rc.SubAgentControl = stubSubAgentControl{
		sendInput: func(_ context.Context, req subagentctl.SendInputRequest) (subagentctl.StatusSnapshot, error) {
			captured = req
			return subagentctl.StatusSnapshot{SubAgentID: req.SubAgentID, Status: data.SubAgentStatusRunning}, nil
		},
	}

	evs := runLuaScript(t, `
local sent, err = agent.send("`+subAgentID.String()+`", "follow-up", { interrupt = true })
if err ~= nil then error(err) end
context.set_output(sent.status)
`, rc)

	if captured.SubAgentID != subAgentID {
		t.Fatalf("unexpected sub_agent_id: %s", captured.SubAgentID)
	}
	if captured.Input != "follow-up" {
		t.Fatalf("unexpected send input: %q", captured.Input)
	}
	if !captured.Interrupt {
		t.Fatal("expected interrupt=true")
	}
	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != data.SubAgentStatusRunning {
		t.Fatalf("expected running status output, got: %v", texts)
	}
}

func TestLuaExecutor_SubAgentInterrupt_ReturnStatus(t *testing.T) {
	subAgentID := uuid.New()
	var captured subagentctl.InterruptRequest
	rc := buildLuaRC(nil)
	rc.SubAgentControl = stubSubAgentControl{
		interrupt: func(_ context.Context, req subagentctl.InterruptRequest) (subagentctl.StatusSnapshot, error) {
			captured = req
			return subagentctl.StatusSnapshot{SubAgentID: req.SubAgentID, Status: data.SubAgentStatusRunning}, nil
		},
	}

	evs := runLuaScript(t, `
local interrupted, err = agent.interrupt("`+subAgentID.String()+`", "timeout")
if err ~= nil then error(err) end
context.set_output(interrupted.status)
	`, rc)

	if captured.SubAgentID != subAgentID {
		t.Fatalf("unexpected sub_agent_id: %s", captured.SubAgentID)
	}
	if captured.Reason != "timeout" {
		t.Fatalf("unexpected reason: %q", captured.Reason)
	}
	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != data.SubAgentStatusRunning {
		t.Fatalf("expected running status output, got: %v", texts)
	}

	var interruptCall, interruptResult bool
	for _, ev := range evs {
		switch ev.Type {
		case "tool.call":
			if ev.ToolName != nil && *ev.ToolName == "agent.interrupt" {
				interruptCall = true
			}
		case "tool.result":
			if ev.ToolName != nil && *ev.ToolName == "agent.interrupt" {
				interruptResult = true
			}
		}
	}
	if !interruptCall || !interruptResult {
		t.Fatalf("expected agent.interrupt tool events, got call=%v result=%v", interruptCall, interruptResult)
	}
}

func TestLuaExecutor_SubAgentWait_TimeoutMs(t *testing.T) {
	subAgentID := uuid.New()
	var captured time.Duration
	rc := buildLuaRC(nil)
	rc.SubAgentControl = stubSubAgentControl{
		wait: func(_ context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			captured = req.Timeout
			if len(req.SubAgentIDs) == 0 {
				return subagentctl.StatusSnapshot{}, errors.New("missing sub_agent_ids")
			}
			return subagentctl.StatusSnapshot{SubAgentID: req.SubAgentIDs[0], Status: data.SubAgentStatusCompleted}, nil
		},
	}

	evs := runLuaScript(t, `
local waited, err = agent.wait("`+subAgentID.String()+`", 2500)
if err ~= nil then error(err) end
context.set_output(waited.status)
`, rc)

	if captured != 2500*time.Millisecond {
		t.Fatalf("expected 2500ms timeout, got %s", captured)
	}
	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != data.SubAgentStatusCompleted {
		t.Fatalf("expected completed status output, got: %v", texts)
	}
}

func TestLuaExecutor_SubAgentWaitEmitsDisplayDescription(t *testing.T) {
	subAgentID := uuid.New()
	rc := buildLuaRC(nil)
	rc.SubAgentControl = stubSubAgentControl{
		wait: func(_ context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			return subagentctl.StatusSnapshot{SubAgentID: req.SubAgentIDs[0], Status: data.SubAgentStatusCompleted}, nil
		},
	}

	evs := runLuaScript(t, `
local waited, err = agent.wait("`+subAgentID.String()+`", 15000, {
  display_description = "等待 QWen / qwen3.5-plus 返回评估结果；每15秒检查一次"
})
if err ~= nil then error(err) end
context.set_output(waited.status)
`, rc)

	for _, ev := range evs {
		if ev.Type != "tool.call" || ev.ToolName == nil || *ev.ToolName != "agent.wait" {
			continue
		}
		if ev.DataJSON["display_description"] != "等待 QWen / qwen3.5-plus 返回评估结果；每15秒检查一次" {
			t.Fatalf("unexpected top-level display_description: %#v", ev.DataJSON["display_description"])
		}
		args, ok := ev.DataJSON["arguments"].(map[string]any)
		if !ok {
			t.Fatalf("expected wait call arguments, got %#v", ev.DataJSON)
		}
		if args["sub_agent_id"] != subAgentID.String() || args["timeout_ms"] != int64(15000) {
			t.Fatalf("expected raw diagnostic fields to remain, got %#v", args)
		}
		return
	}
	t.Fatalf("expected agent.wait tool.call, got %#v", evs)
}

func TestLuaExecutor_SubAgentWaitTimeoutExplainsDisplayDescription(t *testing.T) {
	subAgentID := uuid.New()
	rc := buildLuaRC(nil)
	rc.SubAgentControl = stubSubAgentControl{
		wait: func(context.Context, subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			return subagentctl.StatusSnapshot{}, context.DeadlineExceeded
		},
	}

	evs := runLuaScript(t, `
local waited, err = agent.wait("`+subAgentID.String()+`", 15000, {
  display_description = "等待 DeepSeek / deepseek-v4-flash 返回评估结果（最长8分钟）（已等待19秒/480秒）；每15秒检查一次"
})
if waited ~= nil then error("unexpected wait result") end
context.set_output(err or "")
`, rc)

	for _, ev := range evs {
		if ev.Type != "tool.result" || ev.ToolName == nil || *ev.ToolName != "agent.wait" {
			continue
		}
		errPayload, _ := ev.DataJSON["error"].(map[string]any)
		message, _ := errPayload["message"].(string)
		for _, want := range []string{
			"等待 DeepSeek / deepseek-v4-flash 返回评估结果",
			"15秒",
			"检查窗口",
			"模型仍可能继续运行",
		} {
			if !strings.Contains(message, want) {
				t.Fatalf("expected timeout explanation to contain %q, got %q", want, message)
			}
		}
		return
	}
	t.Fatalf("expected failed agent.wait tool.result, got %#v", evs)
}

func TestLuaExecutor_SubAgentWaitAnyPassesMultipleIDs(t *testing.T) {
	firstID := uuid.New()
	secondID := uuid.New()
	var capturedIDs []uuid.UUID
	var capturedTimeout time.Duration
	rc := buildLuaRC(nil)
	rc.SubAgentControl = stubSubAgentControl{
		wait: func(_ context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			capturedIDs = append([]uuid.UUID(nil), req.SubAgentIDs...)
			capturedTimeout = req.Timeout
			if len(req.SubAgentIDs) != 2 {
				return subagentctl.StatusSnapshot{}, errors.New("expected two sub agent ids")
			}
			output := "second completed first"
			return subagentctl.StatusSnapshot{
				SubAgentID: req.SubAgentIDs[1],
				Status:     data.SubAgentStatusCompleted,
				LastOutput: &output,
			}, nil
		},
	}

	evs := runLuaScript(t, `
local waited, err = agent.wait_any({"`+firstID.String()+`", "`+secondID.String()+`"}, 1500)
if err ~= nil then error(err) end
context.set_output(waited.id .. "|" .. (waited.output or ""))
`, rc)

	if !reflect.DeepEqual(capturedIDs, []uuid.UUID{firstID, secondID}) {
		t.Fatalf("unexpected captured ids: %v", capturedIDs)
	}
	if capturedTimeout != 1500*time.Millisecond {
		t.Fatalf("expected 1500ms timeout, got %s", capturedTimeout)
	}
	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != secondID.String()+"|second completed first" {
		t.Fatalf("expected second sub agent output, got: %v", texts)
	}
}

func TestLuaExecutor_SubAgentWaitAnyEmitsDisplayDescription(t *testing.T) {
	firstID := uuid.New()
	secondID := uuid.New()
	rc := buildLuaRC(nil)
	rc.SubAgentControl = stubSubAgentControl{
		wait: func(_ context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			return subagentctl.StatusSnapshot{SubAgentID: req.SubAgentIDs[1], Status: data.SubAgentStatusCompleted}, nil
		},
	}

	evs := runLuaScript(t, `
local waited, err = agent.wait_any({"`+firstID.String()+`", "`+secondID.String()+`"}, 15000, {
  display_description = "并行等待 2 个评估模型返回结果；每15秒检查一次"
})
if err ~= nil then error(err) end
context.set_output(waited.status)
`, rc)

	for _, ev := range evs {
		if ev.Type != "tool.call" || ev.ToolName == nil || *ev.ToolName != "agent.wait_any" {
			continue
		}
		if ev.DataJSON["display_description"] != "并行等待 2 个评估模型返回结果；每15秒检查一次" {
			t.Fatalf("unexpected top-level display_description: %#v", ev.DataJSON["display_description"])
		}
		args, ok := ev.DataJSON["arguments"].(map[string]any)
		if !ok {
			t.Fatalf("expected wait_any call arguments, got %#v", ev.DataJSON)
		}
		if args["timeout_ms"] != int64(15000) {
			t.Fatalf("expected timeout_ms=15000, got %#v", args)
		}
		return
	}
	t.Fatalf("expected agent.wait_any tool.call, got %#v", evs)
}

func TestLuaExecutor_SubAgentWaitAnyTimeoutExplainsDisplayDescription(t *testing.T) {
	firstID := uuid.New()
	secondID := uuid.New()
	rc := buildLuaRC(nil)
	rc.SubAgentControl = stubSubAgentControl{
		wait: func(context.Context, subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			return subagentctl.StatusSnapshot{}, context.DeadlineExceeded
		},
	}

	evs := runLuaScript(t, `
local waited, err = agent.wait_any({"`+firstID.String()+`", "`+secondID.String()+`"}, 15000, {
  display_description = "并行等待 2 个评估模型返回结果（最长8分钟）（已等待15秒/480秒）；每15秒检查一次"
})
if waited ~= nil then error("unexpected wait_any result") end
context.set_output(err or "")
`, rc)

	for _, ev := range evs {
		if ev.Type != "tool.result" || ev.ToolName == nil || *ev.ToolName != "agent.wait_any" {
			continue
		}
		errPayload, _ := ev.DataJSON["error"].(map[string]any)
		message, _ := errPayload["message"].(string)
		for _, want := range []string{
			"并行等待 2 个评估模型返回结果",
			"15秒",
			"检查窗口",
			"没有任何模型在本轮检查中返回完成、失败或可用输出",
			"模型仍可能继续运行",
		} {
			if !strings.Contains(message, want) {
				t.Fatalf("expected timeout explanation to contain %q, got %q", want, message)
			}
		}
		return
	}
	t.Fatalf("expected failed agent.wait_any tool.result, got %#v", evs)
}

func TestIndustryEducationIndexAgentLuaWaitAnyTimeoutNamesPendingModels(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"available_models": []map[string]any{
			{"selector": "deepseek official^deepseek-v4-flash", "model": "deepseek-v4-flash", "provider_kind": "deepseek", "credential_name": "deepseek official"},
			{"selector": "QWen^qwen3.5-flash-2026-02-23", "model": "qwen3.5-flash-2026-02-23", "provider_kind": "openai", "credential_name": "QWen"},
		},
	}
	answers := []string{
		`{"mode":"分别评估"}`,
		`{"models":["deepseek official^deepseek-v4-flash","QWen^qwen3.5-flash-2026-02-23"]}`,
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
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"results": []map[string]any{{
					"title":   "深圳职业技术大学官网",
					"url":     "https://www.szpu.edu.cn/",
					"snippet": "深圳职业技术大学推进产教融合和校企合作。",
				}},
			}}
		},
	}); err != nil {
		t.Fatalf("bind web_search: %v", err)
	}
	rc.ToolExecutor = dispatch

	waitCalls := 0
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			personaID := req.PersonaID
			return subagentctl.StatusSnapshot{
				SubAgentID:  subAgentID,
				Status:      data.SubAgentStatusQueued,
				PersonaID:   &personaID,
				ContextMode: req.ContextMode,
			}, nil
		},
		wait: func(_ context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			waitCalls++
			if waitCalls > 1 {
				return subagentctl.StatusSnapshot{}, errors.New("stop after first wait_any timeout")
			}
			return subagentctl.StatusSnapshot{}, context.DeadlineExceeded
		},
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	var sawReadableWaitAnyTimeout bool
	for _, ev := range evs {
		if ev.ToolName == nil || *ev.ToolName != "agent.wait_any" {
			continue
		}
		description, _ := ev.DataJSON["display_description"].(string)
		if ev.Type == "tool.call" {
			if strings.Contains(description, "DeepSeek / deepseek-v4-flash") &&
				strings.Contains(description, "Qwen / qwen3.5-flash-2026-02-23") {
				sawReadableWaitAnyTimeout = true
			}
			continue
		}
		if ev.Type != "tool.result" {
			continue
		}
		errPayload, _ := ev.DataJSON["error"].(map[string]any)
		message, _ := errPayload["message"].(string)
		if !strings.Contains(message, "DeepSeek / deepseek-v4-flash") ||
			!strings.Contains(message, "Qwen / qwen3.5-flash-2026-02-23") ||
			!strings.Contains(message, "没有任何模型在本轮检查中返回完成、失败或可用输出") {
			t.Fatalf("expected wait_any timeout to name pending models and detailed reason, got %q", message)
		}
	}
	if !sawReadableWaitAnyTimeout {
		t.Fatalf("expected agent.wait_any call display description to include model names, got events %#v", evs)
	}
}

func TestIndustryEducationIndexAgentLuaSpawnsParallelEvaluatorsWithoutParentStreamAgent(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(&luaSeqGateway{
		events: []llm.StreamEvent{
			llm.StreamRunFailed{Error: llm.GatewayError{Message: "parent stream_agent should not be called", ErrorClass: llm.ErrorClassInternalError}},
		},
	})
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"available_models": []map[string]any{
			{"selector": "deepseek official^deepseek-chat", "model": "deepseek-chat", "provider_kind": "deepseek", "credential_name": "deepseek official"},
			{"selector": "qwen proxy^qwen-max", "model": "qwen-max", "provider_kind": "openai", "credential_name": "qwen proxy"},
		},
	}
	answers := []string{
		`{"mode":"综合评估"}`,
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
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"results": []map[string]any{{
					"title":   "深圳职业技术大学官网",
					"url":     "https://www.szpu.edu.cn/",
					"snippet": "深圳职业技术大学是高职院校，推进双高、产教融合和专业群建设。",
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
	var evaluatorModels []string
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			personaID := req.PersonaID
			switch req.PersonaID {
			case "industry-education-evaluator":
				evaluatorModels = append(evaluatorModels, req.Model)
				outputs[subAgentID] = `{"eligible":true,"model_label":"评估模型","dimensions":[{"name":"基础与机制","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"资源共建共享","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"产学建设与服务","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"人才培养质量","weight":25,"score":80.0,"data_confidence":"medium"}]}`
			case "industry-education-synthesizer":
				outputs[subAgentID] = "# 深圳职业技术大学 · 产教融合指数报告（2026年)"
			default:
				t.Fatalf("unexpected persona: %s", req.PersonaID)
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
			output := outputs[subAgentID]
			return subagentctl.StatusSnapshot{SubAgentID: subAgentID, Status: data.SubAgentStatusCompleted, LastOutput: &output}, nil
		},
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	if got, want := strings.Join(evaluatorModels, ","), "deepseek official^deepseek-chat,qwen proxy^qwen-max"; got != want {
		t.Fatalf("expected parent to spawn both evaluator models in parallel, got %q", got)
	}
	for _, ev := range evs {
		if ev.ToolName != nil && *ev.ToolName == "agent.stream_agent" {
			t.Fatalf("parent index agent must not call stream_agent directly: %#v", ev)
		}
		if ev.Type == "run.failed" {
			t.Fatalf("parent stream_agent fallback was triggered: %#v", ev.DataJSON)
		}
	}
}

func TestIndustryEducationEvaluatorAgentLuaUsesStreamAgent(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationEvaluatorAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education evaluator lua: %v", err)
	}

	gw := &luaSeqGateway{
		events: []llm.StreamEvent{
			llm.StreamMessageDelta{ContentDelta: "基础与机制说明。\n", Role: "assistant"},
			llm.StreamMessageDelta{ContentDelta: `{"eligible":true,"model_label":"DeepSeek","school_name":"深圳职业技术大学","year":"2026","dimensions":[{"name":"基础与机制","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"资源共建共享","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"产学建设与服务","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"人才培养质量","weight":25,"score":80.0,"data_confidence":"medium"}]}`, Role: "assistant"},
			llm.StreamRunCompleted{},
		},
	}
	rc := buildLuaRC(gw)
	rc.Messages = []llm.Message{{
		Role:    "user",
		Content: []llm.TextPart{{Text: `{"task":"evaluate","model_label":"DeepSeek","model_selector":"deepseek official^deepseek-chat","school_name":"深圳职业技术大学","year":"2026","fact_pack":{"eligibility":{"verified":true},"sources":[],"facts":[]}}`}},
	}}
	var resolvedAgentName string
	rc.ResolveGatewayForAgentName = func(_ context.Context, agentName string) (llm.Gateway, *routing.SelectedProviderRoute, error) {
		resolvedAgentName = agentName
		return gw, &routing.SelectedProviderRoute{
			Route: routing.ProviderRouteRule{
				ID:    "deepseek-route",
				Model: "deepseek-chat",
			},
		}, nil
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	if resolvedAgentName != "deepseek official^deepseek-chat" {
		t.Fatalf("expected evaluator to stream through selected model, got %q", resolvedAgentName)
	}
	texts := strings.Join(deltaTexts(evs), "")
	if !strings.Contains(texts, "基础与机制说明") || !strings.Contains(texts, `"eligible":true`) {
		t.Fatalf("expected streamed evaluator output, got %q", texts)
	}
}

func TestLuaExecutor_SubAgentResumeAndClose_ReturnStatus(t *testing.T) {
	resumeID := uuid.New()
	closeID := uuid.New()
	var resumedID uuid.UUID
	var closedID uuid.UUID
	rc := buildLuaRC(nil)
	rc.SubAgentControl = stubSubAgentControl{
		resume: func(_ context.Context, req subagentctl.ResumeRequest) (subagentctl.StatusSnapshot, error) {
			resumedID = req.SubAgentID
			return subagentctl.StatusSnapshot{SubAgentID: req.SubAgentID, Status: data.SubAgentStatusRunning}, nil
		},
		close: func(_ context.Context, req subagentctl.CloseRequest) (subagentctl.StatusSnapshot, error) {
			closedID = req.SubAgentID
			return subagentctl.StatusSnapshot{SubAgentID: req.SubAgentID, Status: data.SubAgentStatusClosed}, nil
		},
	}

	evs := runLuaScript(t, `
local resumed, resume_err = agent.resume("`+resumeID.String()+`")
if resume_err ~= nil then error(resume_err) end
local closed, close_err = agent.close("`+closeID.String()+`")
if close_err ~= nil then error(close_err) end
context.set_output(resumed.status .. "|" .. closed.status)
`, rc)

	if resumedID != resumeID {
		t.Fatalf("unexpected resume id: %s", resumedID)
	}
	if closedID != closeID {
		t.Fatalf("unexpected close id: %s", closedID)
	}
	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != data.SubAgentStatusRunning+"|"+data.SubAgentStatusClosed {
		t.Fatalf("unexpected resume/close output: %v", texts)
	}
}

func TestLuaExecutor_SubAgentContextCancelled(t *testing.T) {
	rc := buildLuaRC(nil)
	rc.SubAgentControl = stubSubAgentControl{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ex, err := NewLuaExecutor(map[string]any{
		"script": `
local status, spawn_err = agent.spawn({ persona_id = "lite", input = "hi" })
if status ~= nil then error("unexpected status") end
context.set_output(spawn_err or "")
`,
	})
	if err != nil {
		t.Fatalf("NewLuaExecutor failed: %v", err)
	}
	emitter := events.NewEmitter("trace")
	var evs []events.RunEvent
	if err := ex.Execute(ctx, rc, emitter, func(ev events.RunEvent) error {
		evs = append(evs, ev)
		return nil
	}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != context.Canceled.Error() {
		t.Fatalf("expected cancelled output, got: %v", texts)
	}
}

func TestLuaExecutor_MemorySearch_WithProvider(t *testing.T) {
	mp := &luaMemMock{
		findHits: []memory.MemoryHit{
			{URI: "viking://user/memories/prefs/lang", Abstract: "Go", Score: 0.9},
		},
	}
	rc := buildLuaRCWithMemory(mp)
	evs := runLuaScript(t, `
local res, err = memory.search("language")
if err then error(err) end
context.set_output(res)
`, rc)

	texts := deltaTexts(evs)
	if len(texts) == 0 {
		t.Fatal("expected message.delta output")
	}
	if !strings.Contains(texts[0], "viking://user/memories/prefs/lang") {
		t.Fatalf("expected URI in output, got: %q", texts[0])
	}
}

func TestLuaExecutor_MemoryRead_WithProvider(t *testing.T) {
	mp := &luaMemMock{contentText: "user prefers Go"}
	rc := buildLuaRCWithMemory(mp)
	evs := runLuaScript(t, `
local content, err = memory.read("viking://user/memories/prefs/lang")
if err then error(err) end
context.set_output(content)
`, rc)

	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != "user prefers Go" {
		t.Fatalf("expected 'user prefers Go', got: %v", texts)
	}
}

func TestLuaExecutor_MemoryWrite_WithProvider(t *testing.T) {
	mp := &luaMemMock{}
	rc := buildLuaRCWithMemory(mp)
	evs := runLuaScript(t, `
local uri, err = memory.write("preferences", "language", "Go")
if err then error(err) end
context.set_output(uri)
`, rc)

	texts := deltaTexts(evs)
	if len(texts) == 0 {
		t.Fatal("expected URI output from memory.write")
	}
	if !strings.HasPrefix(texts[0], "viking://") {
		t.Fatalf("expected viking:// URI, got: %q", texts[0])
	}
}

func TestLuaExecutor_MemoryForget_WithProvider(t *testing.T) {
	mp := &luaMemMock{}
	rc := buildLuaRCWithMemory(mp)
	evs := runLuaScript(t, `
local ok, err = memory.forget("viking://user/memories/prefs/lang")
if err then error(err) end
if ok then context.set_output("deleted") end
`, rc)

	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != "deleted" {
		t.Fatalf("expected 'deleted', got: %v", texts)
	}
}

func TestLuaExecutor_MemoryRead_ProviderNil(t *testing.T) {
	rc := buildLuaRC(nil) // provider nil
	evs := runLuaScript(t, `
local content, err = memory.read("viking://user/memories/prefs/lang")
if err then
  context.set_output("err:" .. err)
else
  context.set_output("ok")
end
`, rc)

	texts := deltaTexts(evs)
	if len(texts) == 0 || !strings.HasPrefix(texts[0], "err:") {
		t.Fatalf("expected error when provider is nil, got: %v", texts)
	}
}

func TestLuaExecutor_MemoryWrite_ProviderNil(t *testing.T) {
	rc := buildLuaRC(nil)
	evs := runLuaScript(t, `
local uri, err = memory.write("preferences", "language", "Go")
if err then
  context.set_output("err:" .. err)
else
  context.set_output("ok")
end
`, rc)

	texts := deltaTexts(evs)
	if len(texts) == 0 || !strings.HasPrefix(texts[0], "err:") {
		t.Fatalf("expected error when provider is nil, got: %v", texts)
	}
}

func TestLuaExecutor_MemoryForget_ProviderNil(t *testing.T) {
	rc := buildLuaRC(nil)
	evs := runLuaScript(t, `
local ok, err = memory.forget("viking://user/memories/prefs/lang")
if err then
  context.set_output("err:" .. err)
else
  context.set_output("ok")
end
`, rc)

	texts := deltaTexts(evs)
	if len(texts) == 0 || !strings.HasPrefix(texts[0], "err:") {
		t.Fatalf("expected error when provider is nil, got: %v", texts)
	}
}

// --- context.emit tests ---

func TestLuaExecutor_ContextEmit_Table(t *testing.T) {
	evs := runLuaScript(t, `
context.emit("run.segment.start", {
  segment_id = "seg1",
  kind = "search_planning",
  display = { mode = "visible", label = "Testing" }
})
`, buildLuaRC(nil))

	for _, ev := range evs {
		if ev.Type == "run.segment.start" {
			if ev.DataJSON["segment_id"] == "seg1" && ev.DataJSON["kind"] == "search_planning" {
				display, ok := ev.DataJSON["display"].(map[string]any)
				if ok && display["label"] == "Testing" {
					return
				}
			}
		}
	}
	t.Fatal("expected run.segment.start with segment_id=seg1")
}

func TestLuaExecutor_ContextEmit_JSONString(t *testing.T) {
	evs := runLuaScript(t, `
context.emit("run.segment.end", '{"segment_id":"seg1"}')
`, buildLuaRC(nil))

	for _, ev := range evs {
		if ev.Type == "run.segment.end" && ev.DataJSON["segment_id"] == "seg1" {
			return
		}
	}
	t.Fatal("expected run.segment.end with segment_id=seg1")
}

func TestLuaExecutor_ContextEmit_InvalidJSON(t *testing.T) {
	evs := runLuaScript(t, `
local ok, err = context.emit("run.segment.start", "not json")
if not ok then
  context.set_output("emit_failed")
end
`, buildLuaRC(nil))

	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != "emit_failed" {
		t.Fatalf("expected emit_failed, got: %v", texts)
	}
}

// --- context.get extensions ---

func TestLuaExecutor_ContextGet_SystemPrompt(t *testing.T) {
	rc := buildLuaRC(nil)
	rc.PromptAssembly.Append(pipeline.PromptSegment{
		Name:          "test.system",
		Target:        pipeline.PromptTargetSystemPrefix,
		Role:          "system",
		Text:          "You are a search assistant.",
		Stability:     pipeline.PromptStabilityStablePrefix,
		CacheEligible: false,
	})
	evs := runLuaScript(t, `
context.set_output(context.get("system_prompt"))
`, rc)

	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != "You are a search assistant." {
		t.Fatalf("expected system_prompt, got: %v", texts)
	}
}

func TestLuaExecutor_ContextGet_Messages(t *testing.T) {
	rc := buildLuaRC(nil)
	rc.Messages = []llm.Message{
		{Role: "user", Content: []llm.TextPart{{Text: "hello"}}},
		{Role: "assistant", Content: []llm.TextPart{{Text: "hi"}}},
	}
	evs := runLuaScript(t, `
local msgs = context.get("messages")
local parsed = json.decode(msgs)
context.set_output(tostring(#parsed) .. ":" .. parsed[1].role)
`, rc)

	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != "2:user" {
		t.Fatalf("expected '2:user', got: %v", texts)
	}
}

// --- agent.generate tests ---

func TestLuaExecutor_AgentGenerate_Basic(t *testing.T) {
	gw := llm.NewAuxGateway(llm.AuxGatewayConfig{Enabled: true, DeltaCount: 1})
	rc := buildLuaRC(gw)
	evs := runLuaScript(t, `
local out, err = agent.generate("system", "user input")
if err then error(err) end
context.set_output(out)
`, rc)

	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != "stub delta 1" {
		t.Fatalf("expected 'stub delta 1', got: %v", texts)
	}
	// agent.generate 不应产生 message.delta 事件（只在最后 set_output 时产生）
	deltasBeforeSetOutput := 0
	for _, ev := range evs {
		if ev.Type == "message.delta" {
			deltasBeforeSetOutput++
		}
	}
	if deltasBeforeSetOutput != 1 {
		t.Fatalf("agent.generate should not yield message.delta, but got %d (1 from set_output)", deltasBeforeSetOutput)
	}
}

func TestLuaExecutor_AgentGenerate_GatewayNil(t *testing.T) {
	rc := buildLuaRC(nil)
	evs := runLuaScript(t, `
local out, err = agent.generate("sys", "msg")
if err then
  context.set_output("err:" .. err)
end
`, rc)

	texts := deltaTexts(evs)
	if len(texts) == 0 || !strings.HasPrefix(texts[0], "err:") {
		t.Fatalf("expected error when gateway is nil, got: %v", texts)
	}
}

func TestLuaExecutor_AgentGenerate_MaxTokens(t *testing.T) {
	gw := llm.NewAuxGateway(llm.AuxGatewayConfig{Enabled: true, DeltaCount: 1})
	rc := buildLuaRC(gw)
	evs := runLuaScript(t, `
local out, err = agent.generate("sys", "msg", {max_tokens = 256})
if err then error(err) end
context.set_output(out)
`, rc)

	texts := deltaTexts(evs)
	if len(texts) == 0 {
		t.Fatal("expected output from agent.generate with max_tokens")
	}
}

// --- agent.stream tests ---

func TestLuaExecutor_AgentStream_StringMessage(t *testing.T) {
	gw := llm.NewAuxGateway(llm.AuxGatewayConfig{Enabled: true, DeltaCount: 3})
	rc := buildLuaRC(gw)
	evs := runLuaScript(t, `
local out, err = agent.stream("system prompt", "user query")
if err then error(err) end
-- out 应包含完整文本，不需要 set_output
`, rc)

	// agent.stream 应产生 message.delta 事件
	var deltas []string
	for _, ev := range evs {
		if ev.Type == "message.delta" {
			if d, ok := ev.DataJSON["content_delta"].(string); ok {
				deltas = append(deltas, d)
			}
		}
	}
	if len(deltas) != 3 {
		t.Fatalf("expected 3 message.delta from stream, got %d", len(deltas))
	}
}

func TestLuaExecutor_AgentStream_MessagesTable(t *testing.T) {
	gw := llm.NewAuxGateway(llm.AuxGatewayConfig{Enabled: true, DeltaCount: 2})
	rc := buildLuaRC(gw)
	evs := runLuaScript(t, `
local msgs = {
  {role = "user", content = "hello"},
  {role = "assistant", content = "hi"},
  {role = "user", content = "how are you"},
}
local out, err = agent.stream("system", msgs)
if err then error(err) end
`, rc)

	var deltaCount int
	for _, ev := range evs {
		if ev.Type == "message.delta" {
			deltaCount++
		}
	}
	if deltaCount != 2 {
		t.Fatalf("expected 2 message.delta, got %d", deltaCount)
	}
}

func TestLuaExecutor_AgentStream_GatewayNil(t *testing.T) {
	rc := buildLuaRC(nil)
	evs := runLuaScript(t, `
local out, err = agent.stream("sys", "msg")
if err then
  context.set_output("err:" .. err)
end
`, rc)

	texts := deltaTexts(evs)
	if len(texts) == 0 || !strings.HasPrefix(texts[0], "err:") {
		t.Fatalf("expected error when gateway is nil, got: %v", texts)
	}
}

func TestLuaExecutor_AgentLoopCapture_CapturesTextWithoutDirectDelta(t *testing.T) {
	inputTokens := 11
	outputTokens := 29
	gw := &luaSeqGateway{
		events: []llm.StreamEvent{
			llm.StreamSegmentStart{
				SegmentID: "s1",
				Kind:      "thinking",
				Display:   llm.SegmentDisplay{Mode: "visible", Label: "Step"},
			},
			llm.StreamMessageDelta{ContentDelta: "captured-", Role: "assistant"},
			llm.StreamMessageDelta{ContentDelta: "result", Role: "assistant"},
			llm.StreamSegmentEnd{SegmentID: "s1"},
			llm.StreamRunCompleted{
				Usage: &llm.Usage{
					InputTokens:  &inputTokens,
					OutputTokens: &outputTokens,
				},
			},
		},
	}
	rc := buildLuaRC(gw)
	evs := runLuaScript(t, `
local out, err = agent.loop_capture("system", "query")
if err then error(err) end
context.set_output(out)
`, rc)

	deltas := deltaTexts(evs)
	if len(deltas) != 1 || deltas[0] != "captured-result" {
		t.Fatalf("expected only set_output delta 'captured-result', got: %v", deltas)
	}

	var hasSegmentStart bool
	var hasSegmentEnd bool
	var usage map[string]any
	for _, ev := range evs {
		switch ev.Type {
		case "run.segment.start":
			hasSegmentStart = true
		case "run.segment.end":
			hasSegmentEnd = true
		case "run.completed":
			raw, ok := ev.DataJSON["usage"].(map[string]any)
			if ok {
				usage = raw
			}
		}
	}
	if !hasSegmentStart || !hasSegmentEnd {
		t.Fatalf("expected run.segment events passthrough, start=%v end=%v", hasSegmentStart, hasSegmentEnd)
	}
	if usage == nil {
		t.Fatal("expected run.completed usage from loop_capture")
	}
	if usage["input_tokens"] != inputTokens || usage["output_tokens"] != outputTokens {
		t.Fatalf("unexpected usage payload: %#v", usage)
	}
}

func TestLuaExecutor_AgentLoop_SteeringInjectedAfterTool(t *testing.T) {
	var secondCallMessages []llm.Message
	var phases []string
	gw := &multiTurnGateway{
		onSecondCall: func(req llm.Request) {
			secondCallMessages = req.Messages
		},
	}

	ex, err := NewLuaExecutor(map[string]any{
		"script": `
local ok, loopErr = agent.loop("system", "query")
if loopErr then error(loopErr) end
`,
	})
	if err != nil {
		t.Fatalf("NewLuaExecutor failed: %v", err)
	}

	rc := buildLuaRC(gw)
	rc.ToolExecutor = buildMinimalToolExecutor()
	rc.UserPromptScanFunc = func(_ context.Context, text string, phase string) error {
		if text != "runtime steering" {
			t.Fatalf("unexpected scan text: %q", text)
		}
		phases = append(phases, phase)
		return nil
	}

	pollCount := 0
	rc.PollSteeringInput = func(_ context.Context) (string, bool) {
		pollCount++
		if pollCount == 2 {
			return "runtime steering", true
		}
		return "", false
	}

	emitter := events.NewEmitter("trace")
	var got []events.RunEvent
	err = ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error {
		got = append(got, ev)
		return nil
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var sawSteering bool
	for _, ev := range got {
		if ev.Type == "run.steering_injected" {
			sawSteering = true
		}
	}
	if !sawSteering {
		t.Fatal("expected run.steering_injected event")
	}
	if len(phases) != 1 || phases[0] != "steering_input" {
		t.Fatalf("unexpected scan phases: %v", phases)
	}

	injectedFound := false
	for _, msg := range secondCallMessages {
		if msg.Role != "user" {
			continue
		}
		for _, part := range msg.Content {
			if part.Text == "runtime steering" {
				injectedFound = true
			}
		}
	}
	if !injectedFound {
		t.Fatalf("steering message not found in second LLM call: %#v", secondCallMessages)
	}
}

func TestLuaExecutor_AgentLoop_AskUserUsesWaitForInputAndPromptScan(t *testing.T) {
	userAnswer := `{"db":"postgres"}`
	gw := &captureGateway{
		events: [][]llm.StreamEvent{
			{
				llm.ToolCall{
					ToolCallID: "call_ask_user",
					ToolName:   "ask_user",
					ArgumentsJSON: map[string]any{
						"message": "Pick a database",
						"fields": []any{
							map[string]any{
								"key":      "db",
								"type":     "string",
								"title":    "Database",
								"enum":     []any{"postgres", "mysql"},
								"required": true,
							},
						},
					},
				},
				llm.StreamRunCompleted{},
			},
			{
				llm.StreamMessageDelta{ContentDelta: "handled", Role: "assistant"},
				llm.StreamRunCompleted{},
			},
		},
	}

	rc := buildLuaRC(gw)
	waitCalls := 0
	var phases []string
	rc.WaitForInput = func(_ context.Context) (string, bool) {
		waitCalls++
		return userAnswer, true
	}
	rc.UserPromptScanFunc = func(_ context.Context, text string, phase string) error {
		if text != userAnswer {
			t.Fatalf("unexpected scan text: %q", text)
		}
		phases = append(phases, phase)
		return nil
	}

	evs := runLuaScript(t, `
local ok, err = agent.loop("system", "query")
if err then error(err) end
`, rc)

	if waitCalls != 1 {
		t.Fatalf("expected WaitForInput called once, got %d", waitCalls)
	}
	if len(phases) != 1 || phases[0] != "ask_user" {
		t.Fatalf("unexpected scan phases: %v", phases)
	}
	if len(gw.requests) != 2 {
		t.Fatalf("expected 2 gateway requests, got %d", len(gw.requests))
	}

	var inputRequested bool
	var askUserToolResultOK bool
	for _, ev := range evs {
		switch ev.Type {
		case "run.input_requested":
			inputRequested = true
		case "tool.result":
			if ev.ToolName != nil && *ev.ToolName == "ask_user" && ev.ErrorClass == nil {
				askUserToolResultOK = true
			}
		}
	}
	if !inputRequested {
		t.Fatal("expected run.input_requested event")
	}
	if !askUserToolResultOK {
		t.Fatalf("expected successful ask_user tool.result, got %#v", evs)
	}

	secondMessages := gw.requests[1].Messages
	answered := false
	for _, msg := range secondMessages {
		if msg.Role != "tool" {
			continue
		}
		for _, part := range msg.Content {
			if strings.Contains(part.Text, `"tool_name":"ask_user"`) && strings.Contains(part.Text, `"db":"postgres"`) {
				answered = true
			}
		}
	}
	if !answered {
		t.Fatalf("ask_user response not forwarded into second LLM call: %#v", secondMessages)
	}

	deltas := deltaTexts(evs)
	if len(deltas) == 0 || deltas[0] != "handled" {
		t.Fatalf("expected follow-up assistant delta, got %v", deltas)
	}
}

func TestLuaContextAskUserUsesWaitForInput(t *testing.T) {
	userAnswer := `{"mode":"综合评估"}`
	rc := buildLuaRC(nil)
	waitCalls := 0
	rc.WaitForInput = func(_ context.Context) (string, bool) {
		waitCalls++
		return userAnswer, true
	}

	evs := runLuaScript(t, `
local result, err = context.ask_user(json.encode({
  message = "选择模式",
  fields = {
    {
      key = "mode",
      type = "string",
      title = "评估模式",
      enum = {"综合评估", "分别评估"},
      required = true
    }
  }
}))
if err then error(err) end
local parsed = json.decode(result)
context.set_output(parsed.user_response.mode)
`, rc)

	if waitCalls != 1 {
		t.Fatalf("expected WaitForInput called once, got %d", waitCalls)
	}
	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != "综合评估" {
		t.Fatalf("unexpected output: %v", texts)
	}
	hasInputRequested := false
	for _, ev := range evs {
		if ev.Type == pipeline.EventTypeInputRequested {
			hasInputRequested = true
		}
	}
	if !hasInputRequested {
		t.Fatal("expected run.input_requested event")
	}
}

func industryEducationIndexAgentLuaPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../personas/industry-education-index/agent.lua"))
}

func industryEducationEvaluatorAgentLuaPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../personas/industry-education-evaluator/agent.lua"))
}

func TestLuaToolsCallEmitsToolEvents(t *testing.T) {
	rc := buildLuaRC(nil)
	reg := tools.NewRegistry()
	if err := reg.Register(tools.AgentToolSpec{Name: "document_write", Version: "1", Description: "doc", RiskLevel: tools.RiskLevelLow}); err != nil {
		t.Fatalf("register tool: %v", err)
	}
	dispatch := tools.NewDispatchingExecutor(reg, tools.NewPolicyEnforcer(reg, tools.AllowlistFromNames([]string{"document_write"})))
	if err := dispatch.Bind("document_write", stubLuaToolExecutor{
		execute: func(_ context.Context, toolName string, _ map[string]any, _ tools.ExecutionContext, _ string) tools.ExecutionResult {
			if toolName != "document_write" {
				t.Fatalf("unexpected tool: %s", toolName)
			}
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"artifacts": []map[string]any{{
					"filename":  "report.md",
					"mime_type": "text/markdown",
				}},
			}}
		},
	}); err != nil {
		t.Fatalf("bind tool: %v", err)
	}
	rc.ToolExecutor = dispatch

	evs := runLuaScript(t, `
local result, err = tools.call("document_write", json.encode({filename="report.md", content="# Report"}))
if err then error(err) end
context.set_output(result)
`, rc)

	var hasToolCall, hasToolResult bool
	for _, ev := range evs {
		switch ev.Type {
		case "tool.call":
			if ev.ToolName != nil && *ev.ToolName == "document_write" {
				hasToolCall = true
			}
		case "tool.result":
			if ev.ToolName != nil && *ev.ToolName == "document_write" {
				hasToolResult = true
				result, _ := ev.DataJSON["result"].(map[string]any)
				if result == nil {
					t.Fatalf("expected tool result payload, got %#v", ev.DataJSON)
				}
			}
		}
	}
	if !hasToolCall || !hasToolResult {
		t.Fatalf("expected tool.call and tool.result, got call=%v result=%v", hasToolCall, hasToolResult)
	}
}

func TestIndustryEducationIndexAgentLuaSeparateModeContinuesWithOneValidEvaluation(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
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
					outputs[subAgentID] = `{"eligible":true,"model_label":"DeepSeek","dimensions":[{"name":"基础与机制","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"资源共建共享","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"产学建设与服务","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"人才培养质量","weight":25,"score":80.0,"data_confidence":"medium"}]}`
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
	if !strings.Contains(texts, "评估完成：") {
		t.Fatalf("expected explicit success hint, got %q", texts)
	}
	if !strings.Contains(texts, "深圳职业技术大学 · 产教融合指数报告（2026年）") {
		t.Fatalf("expected report output, got %q", texts)
	}
}

func TestIndustryEducationIndexAgentLuaContinuePromptDoesNotBecomeSchoolName(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "请继续",
	}
	answers := []string{
		`{"mode":"多模型评估"}`,
	}
	rc.WaitForInput = func(_ context.Context) (string, bool) {
		if len(answers) == 0 {
			return "", false
		}
		answer := answers[0]
		answers = answers[1:]
		return answer, true
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	texts := strings.Join(deltaTexts(evs), "")
	if !strings.Contains(texts, "评估未完成：需要先提供院校名称") {
		t.Fatalf("expected missing school name hint, got %q", texts)
	}
}

func TestIndustryEducationIndexAgentLuaMatchesSchoolNameFromGeneralCatalog(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt":  "帮我看一下北京大学",
		"school_names": `{"source":"test","schools":[{"name":"北京大学","level":"本科"}]}`,
	}
	answers := []string{
		`{"mode":"单模型评估"}`,
		`{"models":["deepseek-chat"]}`,
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
	var searchQueries []string
	if err := dispatch.Bind("web_search", stubLuaToolExecutor{
		execute: func(_ context.Context, _ string, args map[string]any, _ tools.ExecutionContext, _ string) tools.ExecutionResult {
			if rawQueries, ok := args["queries"].([]any); ok {
				for _, rawQuery := range rawQueries {
					if query, ok := rawQuery.(string); ok {
						searchQueries = append(searchQueries, query)
					}
				}
			}
			return tools.ExecutionResult{Error: &tools.ExecutionError{Message: "stop after query capture"}}
		},
	}); err != nil {
		t.Fatalf("bind web_search: %v", err)
	}
	rc.ToolExecutor = dispatch

	_ = runLuaScript(t, string(scriptBytes), rc)
	if len(searchQueries) == 0 {
		t.Fatal("expected web_search queries")
	}
	for _, query := range searchQueries {
		if strings.Contains(query, "一下北京大学") || strings.Contains(query, "帮我看一下北京大学") {
			t.Fatalf("search query kept prompt wording: %q", query)
		}
		if !strings.Contains(query, "北京大学") {
			t.Fatalf("search query lost catalog school name: %q", query)
		}
	}
}

func TestIndustryEducationIndexAgentLuaNormalizesColloquialSchoolNameFromCatalog(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt":  "继续评估一下江苏农牧科技职业学院",
		"school_names": `{"source":"test","schools":[{"name":"江苏农牧科技职业学院","level":"专科"}]}`,
		"available_models": []map[string]any{
			{"selector": "deepseek official^deepseek-chat", "model": "deepseek-chat", "provider_kind": "deepseek", "credential_name": "deepseek official"},
		},
	}
	answers := []string{
		`{"mode":"单模型评估"}`,
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
	var searchQueries []string
	if err := dispatch.Bind("web_search", stubLuaToolExecutor{
		execute: func(_ context.Context, _ string, args map[string]any, _ tools.ExecutionContext, _ string) tools.ExecutionResult {
			if rawQueries, ok := args["queries"].([]any); ok {
				for _, rawQuery := range rawQueries {
					if query, ok := rawQuery.(string); ok {
						searchQueries = append(searchQueries, query)
					}
				}
			}
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"results": []map[string]any{{
					"title":   "江苏农牧科技职业学院官网",
					"url":     "https://www.jsahvc.edu.cn/",
					"snippet": "江苏农牧科技职业学院是高职院校，推进产教融合和校企合作。",
				}},
			}}
		},
	}); err != nil {
		t.Fatalf("bind web_search: %v", err)
	}
	if err := dispatch.Bind("web_fetch", stubLuaToolExecutor{
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"content": "江苏农牧科技职业学院围绕职业教育、产教融合和校企合作开展人才培养。",
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
	var evaluatorInput string
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			personaID := req.PersonaID
			switch req.PersonaID {
			case "industry-education-evaluator":
				evaluatorInput = req.Input
				outputs[subAgentID] = `{"eligible":true,"model_label":"DeepSeek","dimensions":[{"name":"基础与机制","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"资源共建共享","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"产学建设与服务","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"人才培养质量","weight":25,"score":80.0,"data_confidence":"medium"}]}`
			case "industry-education-synthesizer":
				outputs[subAgentID] = "# 江苏农牧科技职业学院 · 产教融合指数报告（2026年）\n\n## 综合评级与产教融合指数得分"
			default:
				t.Fatalf("unexpected persona: %s", req.PersonaID)
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
			output := outputs[subAgentID]
			return subagentctl.StatusSnapshot{SubAgentID: subAgentID, Status: data.SubAgentStatusCompleted, LastOutput: &output}, nil
		},
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	texts := strings.Join(deltaTexts(evs), "")
	if len(searchQueries) == 0 {
		t.Fatal("expected web_search queries")
	}
	for _, query := range searchQueries {
		if strings.Contains(query, "一下江苏农牧科技职业学院") {
			t.Fatalf("search query kept colloquial prefix: %q", query)
		}
		if !strings.Contains(query, "江苏农牧科技职业学院") {
			t.Fatalf("search query lost normalized school name: %q", query)
		}
	}
	if strings.Contains(evaluatorInput, "一下江苏农牧科技职业学院") {
		t.Fatalf("evaluator input kept colloquial prefix: %q", evaluatorInput)
	}
	if !strings.Contains(texts, "江苏农牧科技职业学院 · 产教融合指数报告（2026年）") {
		t.Fatalf("expected normalized report output, got %q", texts)
	}
	if strings.Contains(texts, "一下江苏农牧科技职业学院") {
		t.Fatalf("final output kept colloquial prefix, got %q", texts)
	}
}

func TestIndustryEducationIndexAgentLuaMultiModelWaitsForEvaluatorsTogether(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"available_models": []map[string]any{
			{"selector": "deepseek-官方^deepseek-v4-pro", "model": "deepseek-v4-pro", "provider_kind": "deepseek", "credential_name": "deepseek-官方"},
			{"selector": "QWen^qwen3.6-27b", "model": "qwen3.6-27b", "provider_kind": "openai", "credential_name": "QWen"},
		},
	}
	answers := []string{
		`{"mode":"多模型评估"}`,
		`{"models":["deepseek-官方^deepseek-v4-pro","QWen^qwen3.6-27b"]}`,
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
					"snippet": "深圳职业技术大学是高职院校，推进产教融合、校企合作和专业群建设。",
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
	modelByID := map[uuid.UUID]string{}
	var evaluatorIDs []uuid.UUID
	var sawBatchEvaluatorWait bool
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			personaID := req.PersonaID
			switch req.PersonaID {
			case "industry-education-evaluator":
				evaluatorIDs = append(evaluatorIDs, subAgentID)
				modelByID[subAgentID] = req.Model
				if req.Model == "QWen^qwen3.6-27b" {
					outputs[subAgentID] = `{"eligible":true,"model_label":"Qwen","dimensions":[{"name":"基础与机制","weight":25,"score":82.0,"data_confidence":"medium"},{"name":"资源共建共享","weight":25,"score":83.0,"data_confidence":"medium"},{"name":"产学建设与服务","weight":25,"score":84.0,"data_confidence":"medium"},{"name":"人才培养质量","weight":25,"score":85.0,"data_confidence":"medium"}]}`
				} else {
					outputs[subAgentID] = `{"eligible":true,"model_label":"DeepSeek","dimensions":[{"name":"基础与机制","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"资源共建共享","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"产学建设与服务","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"人才培养质量","weight":25,"score":80.0,"data_confidence":"medium"}]}`
				}
			case "industry-education-synthesizer":
				outputs[subAgentID] = "# 深圳职业技术大学 · 产教融合指数报告（2026年）\n\n## 综合评级与产教融合指数得分"
			default:
				t.Fatalf("unexpected persona: %s", req.PersonaID)
			}
			return subagentctl.StatusSnapshot{
				SubAgentID:  subAgentID,
				Status:      data.SubAgentStatusQueued,
				PersonaID:   &personaID,
				ContextMode: req.ContextMode,
			}, nil
		},
		wait: func(_ context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			if len(req.SubAgentIDs) > 1 {
				sawBatchEvaluatorWait = true
				for _, id := range req.SubAgentIDs {
					if modelByID[id] == "QWen^qwen3.6-27b" {
						output := outputs[id]
						return subagentctl.StatusSnapshot{SubAgentID: id, Status: data.SubAgentStatusCompleted, LastOutput: &output}, nil
					}
				}
			}
			subAgentID := req.SubAgentIDs[0]
			if modelByID[subAgentID] == "deepseek-官方^deepseek-v4-pro" {
				return subagentctl.StatusSnapshot{}, context.DeadlineExceeded
			}
			output := outputs[subAgentID]
			return subagentctl.StatusSnapshot{SubAgentID: subAgentID, Status: data.SubAgentStatusCompleted, LastOutput: &output}, nil
		},
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	texts := strings.Join(deltaTexts(evs), "")
	if len(evaluatorIDs) != 2 {
		t.Fatalf("expected two evaluator spawns, got %d", len(evaluatorIDs))
	}
	if !sawBatchEvaluatorWait {
		t.Fatalf("expected evaluator waits to include multiple sub agent ids")
	}
	if strings.Contains(texts, "所有评估模型均未成功返回有效结果") {
		t.Fatalf("expected fast evaluator result to be used despite slow evaluator, got %q", texts)
	}
	if !strings.Contains(texts, "深圳职业技术大学 · 产教融合指数报告（2026年）") {
		t.Fatalf("expected report output, got %q", texts)
	}
}

func TestIndustryEducationIndexAgentLuaAcceptsEvaluatorJSONWrappedInMarkdown(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"available_models": []map[string]any{
			{"selector": "deepseek official^deepseek-v4-pro", "model": "deepseek-v4-pro", "provider_kind": "deepseek", "credential_name": "deepseek official"},
		},
	}
	answers := []string{
		`{"mode":"单模型评估"}`,
		`{"models":["deepseek official^deepseek-v4-pro"]}`,
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
					"snippet": "深圳职业技术大学是高等职业院校，开展双高专业群和产教融合建设。",
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
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			personaID := req.PersonaID
			switch req.PersonaID {
			case "industry-education-evaluator":
				outputs[subAgentID] = "```json\n{\"eligible\":true,\"model_label\":\"DeepSeek\",\"dimensions\":[{\"name\":\"基础与机制\",\"weight\":25,\"score\":80.0,\"data_confidence\":\"medium\"},{\"name\":\"资源共建共享\",\"weight\":25,\"score\":80.0,\"data_confidence\":\"medium\"},{\"name\":\"产学建设与服务\",\"weight\":25,\"score\":80.0,\"data_confidence\":\"medium\"},{\"name\":\"人才培养质量\",\"weight\":25,\"score\":80.0,\"data_confidence\":\"medium\"}]}\n```"
			case "industry-education-synthesizer":
				outputs[subAgentID] = "# 深圳职业技术大学 · 产教融合指数报告（2026年）\n\n## 综合评级与产教融合指数得分"
			default:
				outputs[subAgentID] = ""
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
			output := outputs[subAgentID]
			return subagentctl.StatusSnapshot{SubAgentID: subAgentID, Status: data.SubAgentStatusCompleted, LastOutput: &output}, nil
		},
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	texts := strings.Join(deltaTexts(evs), "")
	if strings.Contains(texts, "返回格式不是有效 JSON") {
		t.Fatalf("expected markdown-wrapped JSON to be accepted, got %q", texts)
	}
	if !strings.Contains(texts, "深圳职业技术大学 · 产教融合指数报告（2026年）") {
		t.Fatalf("expected report output, got %q", texts)
	}
}

func TestIndustryEducationIndexAgentLuaFinalOutputLinksArtifacts(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"available_models": []map[string]any{
			{"selector": "deepseek official^deepseek-chat", "model": "deepseek-chat", "provider_kind": "deepseek", "credential_name": "deepseek official"},
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
					"snippet": "深圳职业技术大学是高等职业院校，开展双高专业群和产教融合建设。",
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
				"artifacts": []map[string]any{{"key": "acct/run/report.md", "filename": "report.md", "mime_type": "text/markdown"}},
			}}
		},
	}); err != nil {
		t.Fatalf("bind document_write: %v", err)
	}
	if err := dispatch.Bind("markdown_to_pdf", stubLuaToolExecutor{
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"artifacts": []map[string]any{{"key": "acct/run/report.pdf", "filename": "report.pdf", "mime_type": "application/pdf", "display": "download"}},
			}}
		},
	}); err != nil {
		t.Fatalf("bind markdown_to_pdf: %v", err)
	}
	rc.ToolExecutor = dispatch

	outputs := map[uuid.UUID]string{}
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			personaID := req.PersonaID
			switch req.PersonaID {
			case "industry-education-evaluator":
				outputs[subAgentID] = `{"eligible":true,"model_label":"DeepSeek","dimensions":[{"name":"基础与机制","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"资源共建共享","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"产学建设与服务","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"人才培养质量","weight":25,"score":80.0,"data_confidence":"medium"}]}`
			case "industry-education-synthesizer":
				outputs[subAgentID] = "# 深圳职业技术大学 · 产教融合指数报告（2026年）"
			default:
				outputs[subAgentID] = ""
			}
			return subagentctl.StatusSnapshot{
				SubAgentID:  subAgentID,
				Status:      data.SubAgentStatusQueued,
				PersonaID:   &personaID,
				ContextMode: req.ContextMode,
			}, nil
		},
		wait: func(_ context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			output := outputs[req.SubAgentIDs[0]]
			return subagentctl.StatusSnapshot{SubAgentID: req.SubAgentIDs[0], Status: data.SubAgentStatusCompleted, LastOutput: &output}, nil
		},
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	texts := strings.Join(deltaTexts(evs), "")
	if !strings.Contains(texts, "[report.md](artifact:acct/run/report.md)") {
		t.Fatalf("expected markdown artifact link, got %q", texts)
	}
	if !strings.Contains(texts, "[report.pdf](artifact:acct/run/report.pdf)") {
		t.Fatalf("expected pdf artifact link, got %q", texts)
	}
}

func TestIndustryEducationIndexAgentLuaFallbackReportUsesReferenceStyleSections(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"available_models": []map[string]any{
			{"selector": "deepseek official^deepseek-chat", "model": "deepseek-chat", "provider_kind": "deepseek", "credential_name": "deepseek official"},
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
					"snippet": "学校官网显示，学校推进产教融合、校企合作、专业群建设和技术技能人才培养。",
				}},
			}}
		},
	}); err != nil {
		t.Fatalf("bind web_search: %v", err)
	}
	if err := dispatch.Bind("web_fetch", stubLuaToolExecutor{
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"content": "学校围绕现代产业学院、实训基地、校企协同育人、技能竞赛和社会服务开展建设。",
			}}
		},
	}); err != nil {
		t.Fatalf("bind web_fetch: %v", err)
	}
	if err := dispatch.Bind("document_write", stubLuaToolExecutor{
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"artifacts": []map[string]any{{"key": "acct/run/report.md", "filename": "report.md", "mime_type": "text/markdown"}},
			}}
		},
	}); err != nil {
		t.Fatalf("bind document_write: %v", err)
	}
	if err := dispatch.Bind("markdown_to_pdf", stubLuaToolExecutor{
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"artifacts": []map[string]any{{"key": "acct/run/report.pdf", "filename": "report.pdf", "mime_type": "application/pdf", "display": "download"}},
			}}
		},
	}); err != nil {
		t.Fatalf("bind markdown_to_pdf: %v", err)
	}
	rc.ToolExecutor = dispatch

	outputs := map[uuid.UUID]string{}
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			personaID := req.PersonaID
			switch req.PersonaID {
			case "industry-education-evaluator":
				outputs[subAgentID] = `{"eligible":true,"model_label":"DeepSeek","dimensions":[{"name":"基础与机制","weight":25,"score":82.0,"data_confidence":"medium"},{"name":"资源共建共享","weight":25,"score":81.0,"data_confidence":"medium"},{"name":"产学建设与服务","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"人才培养质量","weight":25,"score":79.0,"data_confidence":"low"}],"honors":[{"text":"公开资料显示学校推进专业群建设","source_id":"S1"}],"highlights":[{"name":"校企协同育人","basis":"公开资料显示学校推进校企合作和实训基地建设","source_ids":["S1"]}],"improvements":[{"priority":"中","dimension":"人才培养质量","text":"建议补充就业质量和企业满意度数据","source_ids":["S1"]}],"missing_placeholders":["{{就业率}}","{{企业满意度}}"]}`
			case "industry-education-synthesizer":
				return subagentctl.StatusSnapshot{}, errors.New("force fallback report")
			default:
				outputs[subAgentID] = ""
			}
			return subagentctl.StatusSnapshot{
				SubAgentID:  subAgentID,
				Status:      data.SubAgentStatusQueued,
				PersonaID:   &personaID,
				ContextMode: req.ContextMode,
			}, nil
		},
		wait: func(_ context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			output := outputs[req.SubAgentIDs[0]]
			return subagentctl.StatusSnapshot{SubAgentID: req.SubAgentIDs[0], Status: data.SubAgentStatusCompleted, LastOutput: &output}, nil
		},
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	texts := strings.Join(deltaTexts(evs), "")
	for _, want := range []string{
		"> **综合评级：",
		"> **产教融合指数得分：",
		"## 一、基础与机制",
		"**制度体系**",
		"**治理架构**",
		"## 二、资源共建共享",
		"**实训条件**",
		"**双师队伍**",
		"## 三、产学建设与服务",
		"**人才培养**",
		"**科研与转化**",
		"## 四、人才培养质量",
		"**就业质量**",
		"## 五、核心荣誉与排名",
		"| 荣誉 / 排名 | 级别 / 来源 |",
		"## 六、优势亮点",
		"## 七、提升方向",
	} {
		if !strings.Contains(texts, want) {
			t.Fatalf("expected fallback report to contain %q, got %q", want, texts)
		}
	}
}

func TestIndustryEducationIndexAgentLuaSeparateModeSummarizesInvalidEvaluatorOutputs(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
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
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			personaID := req.PersonaID
			switch req.PersonaID {
			case "industry-education-evaluator":
				if req.Model == "deepseek official^deepseek-chat" {
					outputs[subAgentID] = `{"eligible":true,"model_label":"DeepSeek","dimensions":[{"name":"基础与机制","weight":25,"score":88.0,"data_confidence":"medium"}]}`
				} else {
					outputs[subAgentID] = `not-json`
				}
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
			output := outputs[subAgentID]
			return subagentctl.StatusSnapshot{SubAgentID: subAgentID, Status: data.SubAgentStatusCompleted, LastOutput: &output}, nil
		},
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	texts := strings.Join(deltaTexts(evs), "")
	if !strings.Contains(texts, "评估模型未返回有效评分") {
		t.Fatalf("expected diagnostic report marker, got %q", texts)
	}
	if !strings.Contains(texts, "失败摘要") {
		t.Fatalf("expected failure summary output, got %q", texts)
	}
	if !strings.Contains(texts, "deepseek official / deepseek-chat") {
		t.Fatalf("expected first model label in failure summary, got %q", texts)
	}
	if !strings.Contains(texts, "维度数量不完整") {
		t.Fatalf("expected dimensions reason, got %q", texts)
	}
	if !strings.Contains(texts, "qwen proxy / qwen-max") {
		t.Fatalf("expected second model label in failure summary, got %q", texts)
	}
	if !strings.Contains(texts, "返回格式不是有效 JSON") {
		t.Fatalf("expected invalid json reason, got %q", texts)
	}
}

func TestIndustryEducationIndexAgentLuaSeparateModeSummarizesExecutionFailures(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
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

	var firstModelID, secondModelID uuid.UUID
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			personaID := req.PersonaID
			switch req.Model {
			case "deepseek official^deepseek-chat":
				firstModelID = subAgentID
			case "qwen proxy^qwen-max":
				secondModelID = subAgentID
			}
			return subagentctl.StatusSnapshot{
				SubAgentID:  subAgentID,
				Status:      data.SubAgentStatusQueued,
				PersonaID:   &personaID,
				ContextMode: req.ContextMode,
			}, nil
		},
		wait: func(_ context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			if len(req.SubAgentIDs) == 0 {
				return subagentctl.StatusSnapshot{}, errors.New("missing sub_agent_ids")
			}
			subAgentID := req.SubAgentIDs[0]
			switch subAgentID {
			case firstModelID:
				return subagentctl.StatusSnapshot{}, errors.New("timeout waiting for evaluation")
			case secondModelID:
				lastErr := "local_model_custom: Extra inputs are not permitted"
				return subagentctl.StatusSnapshot{SubAgentID: subAgentID, Status: data.SubAgentStatusFailed, LastError: &lastErr}, nil
			default:
				return subagentctl.StatusSnapshot{}, errors.New("unexpected sub_agent_id")
			}
		},
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	texts := strings.Join(deltaTexts(evs), "")
	if !strings.Contains(texts, "评估模型未返回有效评分") {
		t.Fatalf("expected diagnostic all-failed report, got %q", texts)
	}
	if !strings.Contains(texts, "失败摘要") {
		t.Fatalf("expected failure summary header, got %q", texts)
	}
	if !strings.Contains(texts, "deepseek official / deepseek-chat") {
		t.Fatalf("expected first model label in failure summary, got %q", texts)
	}
	if !strings.Contains(texts, "执行失败或超时：timeout waiting for evaluation") {
		t.Fatalf("expected timeout/execution failure reason, got %q", texts)
	}
	if !strings.Contains(texts, "qwen proxy / qwen-max") {
		t.Fatalf("expected second model label in failure summary, got %q", texts)
	}
	if !strings.Contains(texts, "local_model_custom: Extra inputs are not permitted") {
		t.Fatalf("expected sub-agent last_error reason, got %q", texts)
	}
}

func TestIndustryEducationIndexAgentLuaAllFailedWritesDiagnosticReport(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估苏州职业技术大学",
		"available_models": []map[string]any{
			{"selector": "deepseek-官方^deepseek-v4-pro", "model": "deepseek-v4-pro", "provider_kind": "deepseek", "credential_name": "deepseek-官方"},
			{"selector": "QWen^qwen3.5-plus", "model": "qwen3.5-plus", "provider_kind": "openai", "credential_name": "QWen"},
		},
	}
	answers := []string{
		`{"mode":"分别评估"}`,
		`{"models":["deepseek-官方^deepseek-v4-pro","QWen^qwen3.5-plus"]}`,
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
	var wroteMarkdown bool
	if err := dispatch.Bind("web_search", stubLuaToolExecutor{
		execute: func(_ context.Context, _ string, _ map[string]any, _ tools.ExecutionContext, _ string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"results": []map[string]any{{
					"title":   "苏州职业技术大学官网",
					"url":     "https://www.jssvc.edu.cn/",
					"snippet": "苏州职业技术大学是职业本科院校，围绕高职、产教融合和校企合作开展建设。",
				}},
			}}
		},
	}); err != nil {
		t.Fatalf("bind web_search: %v", err)
	}
	if err := dispatch.Bind("web_fetch", stubLuaToolExecutor{
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"content": "学校推进产教融合、校企合作、专业群建设和技术技能人才培养。",
			}}
		},
	}); err != nil {
		t.Fatalf("bind web_fetch: %v", err)
	}
	if err := dispatch.Bind("document_write", stubLuaToolExecutor{
		execute: func(_ context.Context, _ string, args map[string]any, _ tools.ExecutionContext, _ string) tools.ExecutionResult {
			content, _ := args["content"].(string)
			wroteMarkdown = strings.Contains(content, "评估模型未返回有效评分") && strings.Contains(content, "失败摘要")
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

	var firstModelID, secondModelID uuid.UUID
	var waitTimeouts []time.Duration
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			personaID := req.PersonaID
			switch req.Model {
			case "deepseek-官方^deepseek-v4-pro":
				firstModelID = subAgentID
			case "QWen^qwen3.5-plus":
				secondModelID = subAgentID
			}
			return subagentctl.StatusSnapshot{
				SubAgentID:  subAgentID,
				Status:      data.SubAgentStatusQueued,
				PersonaID:   &personaID,
				ContextMode: req.ContextMode,
			}, nil
		},
		wait: func(_ context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			waitTimeouts = append(waitTimeouts, req.Timeout)
			subAgentID := req.SubAgentIDs[0]
			switch subAgentID {
			case firstModelID:
				output := "模型解释：无法输出 JSON"
				return subagentctl.StatusSnapshot{SubAgentID: subAgentID, Status: data.SubAgentStatusCompleted, LastOutput: &output}, nil
			case secondModelID:
				return subagentctl.StatusSnapshot{}, context.DeadlineExceeded
			default:
				return subagentctl.StatusSnapshot{}, errors.New("unexpected sub_agent_id")
			}
		},
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	texts := strings.Join(deltaTexts(evs), "")
	if !strings.Contains(texts, "评估未完成：") {
		t.Fatalf("expected explicit failure hint, got %q", texts)
	}
	if !strings.Contains(texts, "苏州职业技术大学 · 产教融合指数报告（2026年）") {
		t.Fatalf("expected diagnostic report title, got %q", texts)
	}
	if !strings.Contains(texts, "评估模型未返回有效评分") {
		t.Fatalf("expected visible all-failed diagnostic section, got %q", texts)
	}
	if !strings.Contains(texts, "deepseek-官方 / deepseek-v4-pro") || !strings.Contains(texts, "QWen / qwen3.5-plus") {
		t.Fatalf("expected both model failures in diagnostic report, got %q", texts)
	}
	if !strings.Contains(texts, "生成文件") || !wroteMarkdown {
		t.Fatalf("expected diagnostic report artifacts to be written, wroteMarkdown=%v texts=%q", wroteMarkdown, texts)
	}
	for i, timeout := range waitTimeouts {
		if timeout > 8*time.Minute {
			t.Fatalf("wait %d timeout = %s, want bounded under parent deadline", i, timeout)
		}
	}
}

func TestIndustryEducationIndexAgentLuaTimeoutFallbackWithoutNowMsIsBounded(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}
	script := "context.now_ms = nil\n" + string(scriptBytes)

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估苏州职业技术大学",
		"available_models": []map[string]any{
			{"selector": "QWen^qwen3.5-plus", "model": "qwen3.5-plus", "provider_kind": "openai", "credential_name": "QWen"},
		},
	}
	answers := []string{
		`{"mode":"单模型评估"}`,
		`{"models":["QWen^qwen3.5-plus"]}`,
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
					"title":   "苏州职业技术大学官网",
					"url":     "https://www.jssvc.edu.cn/",
					"snippet": "学校推进产教融合和校企合作。",
				}},
			}}
		},
	}); err != nil {
		t.Fatalf("bind web_search: %v", err)
	}
	if err := dispatch.Bind("web_fetch", stubLuaToolExecutor{
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"content": "学校推进产教融合、校企合作、专业群建设和技术技能人才培养。",
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

	var subAgentID uuid.UUID
	waitCalls := 0
	interruptCalls := 0
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID = uuid.New()
			personaID := req.PersonaID
			return subagentctl.StatusSnapshot{
				SubAgentID:  subAgentID,
				Status:      data.SubAgentStatusQueued,
				PersonaID:   &personaID,
				ContextMode: req.ContextMode,
			}, nil
		},
		wait: func(_ context.Context, req subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			waitCalls++
			if len(req.SubAgentIDs) != 1 || req.SubAgentIDs[0] != subAgentID {
				return subagentctl.StatusSnapshot{}, errors.New("unexpected wait sub agent id")
			}
			return subagentctl.StatusSnapshot{}, context.DeadlineExceeded
		},
		interrupt: func(_ context.Context, req subagentctl.InterruptRequest) (subagentctl.StatusSnapshot, error) {
			interruptCalls++
			if req.SubAgentID != subAgentID {
				return subagentctl.StatusSnapshot{}, errors.New("unexpected interrupt sub agent id")
			}
			status := data.SubAgentStatusRunning
			return subagentctl.StatusSnapshot{SubAgentID: subAgentID, Status: status}, nil
		},
	}

	evs := runLuaScript(t, script, rc)
	texts := strings.Join(deltaTexts(evs), "")
	if !strings.Contains(texts, "评估未完成：") {
		t.Fatalf("expected explicit failure hint, got %q", texts)
	}
	if !strings.Contains(texts, "等待8分钟未完成") {
		t.Fatalf("expected bounded timeout reason, got %q", texts)
	}
	if waitCalls <= 1 || waitCalls > 40 {
		t.Fatalf("expected bounded retry wait calls, got %d", waitCalls)
	}
	if interruptCalls < 1 {
		t.Fatalf("expected pending sub-agent to be interrupted after timeout, got %d", interruptCalls)
	}
}

func TestIndustryEducationIndexAgentLuaSearchTimeoutMentionsBasicProvider(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"active_tool_provider_configs_by_group": map[string]any{
			"web_search": map[string]any{"provider_name": "web_search.basic"},
		},
		"available_models": []map[string]any{
			{"selector": "deepseek official^deepseek-chat", "model": "deepseek-chat", "provider_kind": "deepseek", "credential_name": "deepseek official"},
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
		execute: func(_ context.Context, _ string, _ map[string]any, _ tools.ExecutionContext, _ string) tools.ExecutionResult {
			return tools.ExecutionResult{Error: &tools.ExecutionError{ErrorClass: tools.ErrorClassToolHardTimeout, Message: "timeout: deadline exceeded"}}
		},
	}); err != nil {
		t.Fatalf("bind web_search: %v", err)
	}
	rc.ToolExecutor = dispatch

	evs := runLuaScript(t, string(scriptBytes), rc)
	texts := strings.Join(deltaTexts(evs), "")
	if !strings.Contains(texts, "web_search") || !strings.Contains(texts, "超时") {
		t.Fatalf("expected timeout diagnostics to mention web_search timeout, got %q", texts)
	}
	if !strings.Contains(texts, "web_search.basic") {
		t.Fatalf("expected diagnostics to mention web_search.basic hint, got %q", texts)
	}
	if !strings.Contains(texts, "Tavily") || !strings.Contains(texts, "SearXNG") {
		t.Fatalf("expected diagnostics to mention Tavily/SearXNG switch guidance, got %q", texts)
	}
	if !strings.Contains(texts, "browser-search") {
		t.Fatalf("expected diagnostics to mention browser-search endpoint hint, got %q", texts)
	}
}

func TestIndustryEducationIndexAgentLuaModelPickerFiltersNonEvaluatorModels(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"available_models": []map[string]any{
			{"selector": "qianwen^qwen-plus", "model": "qwen-plus", "provider_kind": "openai", "credential_name": "qianwen"},
			{"selector": "qianwen^wan2.7-image", "model": "wan2.7-image", "provider_kind": "openai", "credential_name": "qianwen"},
			{"selector": "qianwen^yanchin/deepseek-ocr", "model": "yanchin/deepseek-ocr", "provider_kind": "openai", "credential_name": "qianwen"},
		},
	}
	answers := []string{
		`{"mode":"综合评估"}`,
		`{"models":["qianwen^qwen-plus"]}`,
	}
	rc.WaitForInput = func(_ context.Context) (string, bool) {
		if len(answers) == 0 {
			return "", false
		}
		answer := answers[0]
		answers = answers[1:]
		return answer, true
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	var modelEnums []any
	for _, ev := range evs {
		if ev.Type != pipeline.EventTypeInputRequested || ev.DataJSON["message"] != "请选择用于评估的模型。" {
			continue
		}
		schema, _ := ev.DataJSON["requestedSchema"].(map[string]any)
		props, _ := schema["properties"].(map[string]any)
		models, _ := props["models"].(map[string]any)
		items, _ := models["items"].(map[string]any)
		modelEnums, _ = items["enum"].([]any)
		break
	}
	if len(modelEnums) != 1 || modelEnums[0] != "qianwen^qwen-plus" {
		t.Fatalf("expected only text evaluator model in picker, got %#v", modelEnums)
	}
}

func TestIndustryEducationIndexAgentLuaModeAndModelPickerSchemaCopy(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
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
		`{"mode":"多模型评估"}`,
		`{}`,
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
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"results": []map[string]any{{
					"title":   "深圳职业技术大学官网",
					"url":     "https://www.szpu.edu.cn/",
					"snippet": "深圳职业技术大学是高职院校，推进双高、产教融合和专业群建设。",
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

	var evaluatorModels []string
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			evaluatorModels = append(evaluatorModels, req.Model)
			personaID := req.PersonaID
			return subagentctl.StatusSnapshot{
				SubAgentID:  uuid.New(),
				Status:      data.SubAgentStatusQueued,
				PersonaID:   &personaID,
				ContextMode: req.ContextMode,
			}, nil
		},
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	var modeSchema map[string]any
	var modelSchema map[string]any
	for _, ev := range evs {
		if ev.Type != pipeline.EventTypeInputRequested {
			continue
		}
		switch ev.DataJSON["message"] {
		case "请选择评估模式。":
			modeSchema, _ = ev.DataJSON["requestedSchema"].(map[string]any)
		case "请选择用于评估的模型。":
			modelSchema, _ = ev.DataJSON["requestedSchema"].(map[string]any)
		}
	}
	if modeSchema == nil {
		t.Fatal("expected mode input schema")
	}
	modeProps, _ := modeSchema["properties"].(map[string]any)
	modeField, _ := modeProps["mode"].(map[string]any)
	modeEnum, _ := modeField["enum"].([]any)
	wantModeEnum := []any{"多模型评估", "综合评估", "单模型评估"}
	if !reflect.DeepEqual(modeEnum, wantModeEnum) {
		t.Fatalf("unexpected mode enum: got %#v want %#v", modeEnum, wantModeEnum)
	}
	if got := modeField["default"]; got != "多模型评估" {
		t.Fatalf("expected default mode 多模型评估, got %#v", got)
	}
	if desc, _ := modeField["description"].(string); !strings.Contains(desc, "综合评估") || !strings.Contains(desc, "汇总为一份报告") {
		t.Fatalf("expected mode description to explain 综合评估, got %q", desc)
	}
	modeDescriptions, _ := modeField["enumDescriptions"].([]any)
	if len(modeDescriptions) < 2 || !strings.Contains(modeDescriptions[1].(string), "交叉评估") {
		t.Fatalf("expected 综合评估 option description, got %#v", modeDescriptions)
	}
	if modelSchema == nil {
		t.Fatal("expected model picker schema")
	}
	if got := modelSchema["_dismissLabel"]; got != "使用当前模型" {
		t.Fatalf("expected model picker dismiss label 使用当前模型, got %#v", got)
	}
	modelProps, _ := modelSchema["properties"].(map[string]any)
	modelField, _ := modelProps["models"].(map[string]any)
	if _, ok := modelField["default"]; ok {
		t.Fatalf("model picker should not default-select models, got default %#v", modelField["default"])
	}
	if minItems, _ := modelField["minItems"].(float64); minItems != 1 {
		t.Fatalf("expected minItems=1 so submit is disabled when empty, got %#v", modelField["minItems"])
	}
	if desc, _ := modelField["description"].(string); !strings.Contains(desc, "不选择并点击“使用当前模型”") {
		t.Fatalf("expected model picker description to explain current-model skip, got %q", desc)
	}
	if len(evaluatorModels) != 1 || evaluatorModels[0] != "" {
		t.Fatalf("expected skip to spawn one evaluator without model override, got %#v; output=%q", evaluatorModels, strings.Join(deltaTexts(evs), ""))
	}
}

func TestIndustryEducationIndexAgentLuaSearchTimeoutMentionsProviderWithoutBasicHint(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"available_models": []map[string]any{
			{"selector": "qwen proxy^qwen-max", "model": "qwen-max", "provider_kind": "openai", "credential_name": "qwen proxy"},
		},
	}
	answers := []string{
		`{"mode":"综合评估"}`,
		`{"models":["qwen proxy^qwen-max"]}`,
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
		execute: func(_ context.Context, _ string, _ map[string]any, _ tools.ExecutionContext, _ string) tools.ExecutionResult {
			return tools.ExecutionResult{Error: &tools.ExecutionError{ErrorClass: tools.ErrorClassToolHardTimeout, Message: "timeout on upstream search"}}
		},
	}); err != nil {
		t.Fatalf("bind web_search: %v", err)
	}
	rc.ToolExecutor = dispatch

	evs := runLuaScript(t, string(scriptBytes), rc)
	texts := strings.Join(deltaTexts(evs), "")
	if !strings.Contains(texts, "联网搜索超时") {
		t.Fatalf("expected timeout diagnostics to mention search timeout, got %q", texts)
	}
	if !strings.Contains(texts, "当前提供商") {
		t.Fatalf("expected diagnostics to mention provider context, got %q", texts)
	}
	if strings.Contains(texts, "web_search.basic") {
		t.Fatalf("did not expect web_search.basic hint for non-basic provider, got %q", texts)
	}
}

func TestIndustryEducationIndexAgentLuaRunsWithSubAgentsAndArtifacts(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"available_models": []map[string]any{
			{"selector": "deepseek official^deepseek-chat", "model": "deepseek-chat", "provider_kind": "deepseek", "credential_name": "deepseek official"},
			{"selector": "qwen proxy^qwen-max", "model": "qwen-max", "provider_kind": "openai", "credential_name": "qwen proxy"},
			{"selector": "doubao proxy^doubao-seed-1-6", "model": "doubao-seed-1-6", "provider_kind": "openai", "credential_name": "doubao proxy"},
		},
	}
	answers := []string{
		`{"mode":"综合评估"}`,
		`{"models":["deepseek official^deepseek-chat","qwen proxy^qwen-max","doubao proxy^doubao-seed-1-6"]}`,
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
	var searchQueries []string
	var searchMaxResults any
	if err := dispatch.Bind("web_search", stubLuaToolExecutor{
		execute: func(_ context.Context, _ string, args map[string]any, _ tools.ExecutionContext, _ string) tools.ExecutionResult {
			searchMaxResults = args["max_results"]
			if rawQueries, ok := args["queries"].([]any); ok {
				for _, rawQuery := range rawQueries {
					if query, ok := rawQuery.(string); ok {
						searchQueries = append(searchQueries, query)
					}
				}
			}
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
	var evaluatorModels []string
	var synthesizerSpawned bool
	var waitTimeouts []time.Duration
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			personaID := req.PersonaID
			switch req.PersonaID {
			case "industry-education-evaluator":
				evaluatorModels = append(evaluatorModels, req.Model)
				outputs[subAgentID] = `{"eligible":true,"model_label":"评估模型","total_score":80.0,"rating":"A 优秀","dimensions":[{"name":"基础与机制","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"资源共建共享","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"产学建设与服务","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"人才培养质量","weight":25,"score":80.0,"data_confidence":"medium"}],"core_honors":[],"highlights":[],"improvements":[],"missing":[]}`
			case "industry-education-synthesizer":
				synthesizerSpawned = true
				outputs[subAgentID] = "# unexpected synthesizer report"
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
			waitTimeouts = append(waitTimeouts, req.Timeout)
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
	if !strings.Contains(texts, "深圳职业技术大学 · 产教融合指数报告（2026年）") {
		t.Fatalf("expected report output, got %q", texts)
	}
	if !strings.Contains(texts, "[report.md](artifact:") {
		t.Fatalf("expected clickable markdown artifact link, got %q", texts)
	}
	if !strings.Contains(texts, "生成文件") {
		t.Fatalf("expected generated file suffix, got %q", texts)
	}
	if len(searchQueries) == 0 {
		t.Fatal("expected web_search queries")
	}
	if len(searchQueries) != 5 {
		t.Fatalf("expected 5 search queries, got %d: %v", len(searchQueries), searchQueries)
	}
	if searchMaxResults != float64(12) {
		t.Fatalf("expected max_results=12, got %#v", searchMaxResults)
	}
	for _, query := range searchQueries {
		if !utf8.ValidString(query) {
			t.Fatalf("search query is not valid UTF-8: %q", query)
		}
		if !strings.Contains(query, "深圳职业技术大学") {
			t.Fatalf("search query lost full school name: %q", query)
		}
	}
	wantModels := []string{"deepseek official^deepseek-chat", "qwen proxy^qwen-max", "doubao proxy^doubao-seed-1-6"}
	if strings.Join(evaluatorModels, ",") != strings.Join(wantModels, ",") {
		t.Fatalf("unexpected evaluator models: got %v want %v", evaluatorModels, wantModels)
	}
	if synthesizerSpawned {
		t.Fatalf("expected report to be generated by built-in fast renderer without spawning synthesizer")
	}
	if len(waitTimeouts) != 3 {
		t.Fatalf("expected three evaluator waits and no synthesizer wait, got %d", len(waitTimeouts))
	}
	var sawEvaluatorWaitDescription bool
	var sawSynthesizerWaitDescription bool
	for _, ev := range evs {
		if ev.Type != "tool.call" || ev.ToolName == nil {
			continue
		}
		if *ev.ToolName != "agent.wait" && *ev.ToolName != "agent.wait_any" {
			continue
		}
		description, _ := ev.DataJSON["display_description"].(string)
		if strings.Contains(description, "评估") && strings.Contains(description, "每15秒检查一次") {
			sawEvaluatorWaitDescription = true
		}
		if strings.Contains(description, "撰写报告正文") && strings.Contains(description, "每15秒检查一次") {
			sawSynthesizerWaitDescription = true
		}
	}
	if !sawEvaluatorWaitDescription {
		t.Fatalf("expected readable evaluator wait tool description, got events %#v", evs)
	}
	if sawSynthesizerWaitDescription {
		t.Fatalf("did not expect report-writing wait description after fast report generation, got events %#v", evs)
	}
	for i, timeout := range waitTimeouts {
		if timeout > 15*time.Second || timeout < 1*time.Millisecond {
			t.Fatalf("evaluator wait %d timeout = %s, want bounded slice <=15s", i, timeout)
		}
	}
}

func TestIndustryEducationIndexAgentLuaFastReportGenerationDoesNotWaitForSynthesizer(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"available_models": []map[string]any{
			{"selector": "deepseek official^deepseek-chat", "model": "deepseek-chat", "provider_kind": "deepseek", "credential_name": "deepseek official"},
		},
	}
	answers := []string{
		`{"mode":"单模型评估"}`,
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
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"results": []map[string]any{{
					"title":   "深圳职业技术大学官网",
					"url":     "https://www.szpu.edu.cn/",
					"snippet": "深圳职业技术大学是高职院校，推进双高、产教融合和专业群建设。",
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
	var synthesizerSpawned bool
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			personaID := req.PersonaID
			switch req.PersonaID {
			case "industry-education-evaluator":
				outputs[subAgentID] = `{"eligible":true,"model_label":"DeepSeek","dimensions":[{"name":"基础与机制","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"资源共建共享","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"产学建设与服务","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"人才培养质量","weight":25,"score":80.0,"data_confidence":"medium"}]}`
			case "industry-education-synthesizer":
				synthesizerSpawned = true
				outputs[subAgentID] = "# unexpected synthesizer report"
			default:
				t.Fatalf("unexpected persona: %s", req.PersonaID)
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
			output := outputs[subAgentID]
			return subagentctl.StatusSnapshot{SubAgentID: subAgentID, Status: data.SubAgentStatusCompleted, LastOutput: &output}, nil
		},
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	if synthesizerSpawned {
		t.Fatalf("expected fast report generation without spawning synthesizer")
	}
	texts := strings.Join(deltaTexts(evs), "")
	if !strings.Contains(texts, "深圳职业技术大学 · 产教融合指数报告（2026年）") ||
		!strings.Contains(texts, "## 一、基础与机制") ||
		!strings.Contains(texts, "生成文件") {
		t.Fatalf("expected built-in structured report output, got %q", texts)
	}
	var sawWritingProgress bool
	var sawFastReportProgress bool
	for _, ev := range evs {
		if ev.Type != "todo.updated" {
			continue
		}
		progressJSON := mustJSON(t, ev.DataJSON)
		if strings.Contains(progressJSON, "正在撰写报告正文") {
			sawWritingProgress = true
		}
		if strings.Contains(progressJSON, "生成报告文件") || strings.Contains(progressJSON, "报告文件已生成") {
			sawFastReportProgress = true
		}
	}
	if sawWritingProgress {
		t.Fatalf("did not expect long report-writing progress after fast report generation, got events %#v", evs)
	}
	if !sawFastReportProgress {
		t.Fatalf("expected fast report file progress, got events %#v", evs)
	}
	var sawReportWaitDescription bool
	for _, ev := range evs {
		if ev.Type != "tool.call" || ev.ToolName == nil || *ev.ToolName != "agent.wait" {
			continue
		}
		description, _ := ev.DataJSON["display_description"].(string)
		if strings.Contains(description, "正在撰写报告正文") && strings.Contains(description, "每15秒检查一次") {
			sawReportWaitDescription = true
			break
		}
	}
	if sawReportWaitDescription {
		t.Fatalf("did not expect report wait description after fast report generation, got events %#v", evs)
	}
}

func TestIndustryEducationIndexAgentLuaEvaluatorWaitTimeoutExplainsModel(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"available_models": []map[string]any{
			{"selector": "deepseek official^deepseek-v4-flash", "model": "deepseek-v4-flash", "provider_kind": "deepseek", "credential_name": "deepseek official"},
		},
	}
	answers := []string{
		`{"mode":"单模型评估"}`,
		`{"models":["deepseek official^deepseek-v4-flash"]}`,
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
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"results": []map[string]any{{
					"title":   "深圳职业技术大学官网",
					"url":     "https://www.szpu.edu.cn/",
					"snippet": "深圳职业技术大学推进产教融合和校企合作。",
				}},
			}}
		},
	}); err != nil {
		t.Fatalf("bind web_search: %v", err)
	}
	rc.ToolExecutor = dispatch

	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			personaID := req.PersonaID
			return subagentctl.StatusSnapshot{
				SubAgentID:  subAgentID,
				Status:      data.SubAgentStatusQueued,
				PersonaID:   &personaID,
				ContextMode: req.ContextMode,
			}, nil
		},
		wait: func(context.Context, subagentctl.WaitRequest) (subagentctl.StatusSnapshot, error) {
			return subagentctl.StatusSnapshot{}, context.DeadlineExceeded
		},
		interrupt: func(_ context.Context, req subagentctl.InterruptRequest) (subagentctl.StatusSnapshot, error) {
			return subagentctl.StatusSnapshot{SubAgentID: req.SubAgentID, Status: data.SubAgentStatusClosed}, nil
		},
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	var sawReadableTimeout bool
	for _, ev := range evs {
		if ev.Type != "tool.result" || ev.ToolName == nil || *ev.ToolName != "agent.wait" {
			continue
		}
		errPayload, _ := ev.DataJSON["error"].(map[string]any)
		message, _ := errPayload["message"].(string)
		if strings.Contains(message, "DeepSeek / deepseek-v4-flash") &&
			strings.Contains(message, "检查窗口") &&
			strings.Contains(message, "模型仍可能继续运行") {
			sawReadableTimeout = true
			break
		}
	}
	if !sawReadableTimeout {
		t.Fatalf("expected readable evaluator timeout with model name, got events %#v", evs)
	}
}

func TestIndustryEducationIndexAgentLuaEmitsProgressTodos(t *testing.T) {
	scriptBytes, err := os.ReadFile(industryEducationIndexAgentLuaPath(t))
	if err != nil {
		t.Fatalf("read industry education agent lua: %v", err)
	}

	rc := buildLuaRC(nil)
	rc.InputJSON = map[string]any{
		"user_prompt": "评估深圳职业技术大学",
		"available_models": []map[string]any{
			{"selector": "deepseek official^deepseek-chat", "model": "deepseek-chat", "provider_kind": "deepseek", "credential_name": "deepseek official"},
		},
	}
	answers := []string{
		`{"mode":"单模型评估"}`,
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
		execute: func(context.Context, string, map[string]any, tools.ExecutionContext, string) tools.ExecutionResult {
			return tools.ExecutionResult{ResultJSON: map[string]any{
				"results": []map[string]any{{
					"title":   "深圳职业技术大学官网",
					"url":     "https://www.szpu.edu.cn/",
					"snippet": "深圳职业技术大学是高职院校，推进双高、产教融合和专业群建设。",
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
	rc.SubAgentControl = stubSubAgentControl{
		spawn: func(_ context.Context, req subagentctl.SpawnRequest) (subagentctl.StatusSnapshot, error) {
			subAgentID := uuid.New()
			personaID := req.PersonaID
			switch req.PersonaID {
			case "industry-education-evaluator":
				outputs[subAgentID] = `{"eligible":true,"model_label":"DeepSeek","dimensions":[{"name":"基础与机制","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"资源共建共享","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"产学建设与服务","weight":25,"score":80.0,"data_confidence":"medium"},{"name":"人才培养质量","weight":25,"score":80.0,"data_confidence":"medium"}]}`
			case "industry-education-synthesizer":
				outputs[subAgentID] = "# 深圳职业技术大学 · 产教融合指数报告（2026年）\n\n## 综合评级与产教融合指数得分"
			default:
				t.Fatalf("unexpected persona: %s", req.PersonaID)
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
			output := outputs[subAgentID]
			return subagentctl.StatusSnapshot{SubAgentID: subAgentID, Status: data.SubAgentStatusCompleted, LastOutput: &output}, nil
		},
	}

	evs := runLuaScript(t, string(scriptBytes), rc)
	var snapshots [][]map[string]any
	for _, ev := range evs {
		if ev.Type != "todo.updated" {
			continue
		}
		rawTodos, ok := ev.DataJSON["todos"].([]any)
		if !ok {
			t.Fatalf("todo.updated todos should be an array, got %#v", ev.DataJSON["todos"])
		}
		var items []map[string]any
		for _, raw := range rawTodos {
			item, ok := raw.(map[string]any)
			if !ok {
				t.Fatalf("todo item should be an object, got %#v", raw)
			}
			items = append(items, item)
		}
		snapshots = append(snapshots, items)
	}
	if len(snapshots) < 5 {
		t.Fatalf("expected at least 5 progress todo updates, got %d", len(snapshots))
	}
	var progressToolCallIDs []string
	var progressToolResultIDs []string
	for _, ev := range evs {
		if ev.Type != "tool.call" && ev.Type != "tool.result" {
			continue
		}
		toolName, _ := ev.DataJSON["tool_name"].(string)
		if toolName != "todo_write" {
			continue
		}
		toolCallID, _ := ev.DataJSON["tool_call_id"].(string)
		if ev.Type == "tool.call" {
			progressToolCallIDs = append(progressToolCallIDs, toolCallID)
		} else {
			progressToolResultIDs = append(progressToolResultIDs, toolCallID)
		}
	}
	if len(progressToolCallIDs) != 1 || progressToolCallIDs[0] != "industry_education_progress" {
		t.Fatalf("expected one synthetic todo_write tool.call, got %#v", progressToolCallIDs)
	}
	if len(progressToolResultIDs) < 4 {
		t.Fatalf("expected synthetic todo_write tool.result updates, got %#v", progressToolResultIDs)
	}
	finalSnapshot := snapshots[len(snapshots)-1]
	if len(finalSnapshot) != 5 {
		t.Fatalf("expected five progress todos after splitting report and PDF steps, got %d: %#v", len(finalSnapshot), finalSnapshot)
	}
	seenIDs := map[string]bool{}
	for _, item := range finalSnapshot {
		id, _ := item["id"].(string)
		seenIDs[id] = true
	}
	for _, id := range []string{"sources", "evaluate", "score", "report", "pdf"} {
		if !seenIDs[id] {
			t.Fatalf("expected final progress todos to include id %q, got %#v", id, finalSnapshot)
		}
	}
	joined := ""
	for _, snapshot := range snapshots {
		for _, item := range snapshot {
			joined += " " + strings.TrimSpace(item["content"].(string))
			if active, ok := item["active_form"].(string); ok {
				joined += " " + strings.TrimSpace(active)
			}
		}
	}
	for _, want := range []string{
		"检索公开资料",
		"调用 1 个评估模型",
		"DeepSeek / deepseek-chat 评估完成",
		"综合评分",
		"生成报告文件",
		"转化为 PDF 文件",
		"PDF 文件已生成",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected progress todos to contain %q, got %q", want, joined)
		}
	}
}

func TestLuaExecutor_AgentStreamRoute_UsesResolvedRoute(t *testing.T) {
	mainGW := &luaSeqGateway{
		events: []llm.StreamEvent{
			llm.StreamMessageDelta{ContentDelta: "main", Role: "assistant"},
			llm.StreamRunCompleted{},
		},
	}
	routeGW := &luaSeqGateway{
		events: []llm.StreamEvent{
			llm.StreamMessageDelta{ContentDelta: "route", Role: "assistant"},
			llm.StreamRunCompleted{},
		},
	}
	rc := buildLuaRC(mainGW)
	var resolvedRouteID string
	rc.ResolveGatewayForRouteID = func(_ context.Context, routeID string) (llm.Gateway, *routing.SelectedProviderRoute, error) {
		resolvedRouteID = routeID
		return routeGW, &routing.SelectedProviderRoute{
			Route: routing.ProviderRouteRule{
				ID:    routeID,
				Model: "route-model",
			},
		}, nil
	}

	evs := runLuaScript(t, `
local out, err = agent.stream_route("final-route", "sys", "msg")
if err then error(err) end
`, rc)
	if resolvedRouteID != "final-route" {
		t.Fatalf("expected resolver called with final-route, got %q", resolvedRouteID)
	}
	deltas := deltaTexts(evs)
	if len(deltas) == 0 || deltas[0] != "route" {
		t.Fatalf("expected route gateway delta, got %v", deltas)
	}
}

func TestLuaExecutor_AgentStreamRoute_EmptyRouteFallsBackToPrimaryGateway(t *testing.T) {
	mainGW := &luaSeqGateway{
		events: []llm.StreamEvent{
			llm.StreamMessageDelta{ContentDelta: "primary", Role: "assistant"},
			llm.StreamRunCompleted{},
		},
	}
	rc := buildLuaRC(mainGW)
	resolverCalled := false
	rc.ResolveGatewayForRouteID = func(_ context.Context, _ string) (llm.Gateway, *routing.SelectedProviderRoute, error) {
		resolverCalled = true
		return nil, nil, errors.New("should not be called")
	}

	evs := runLuaScript(t, `
local out, err = agent.stream_route("", "sys", "msg")
if err then error(err) end
`, rc)
	if resolverCalled {
		t.Fatal("resolver should not be called when route_id is empty")
	}
	deltas := deltaTexts(evs)
	if len(deltas) == 0 || deltas[0] != "primary" {
		t.Fatalf("expected primary gateway delta, got %v", deltas)
	}
}

func TestLuaExecutor_AgentStreamRoute_RouteResolveFailedCanFallback(t *testing.T) {
	mainGW := &luaSeqGateway{
		events: []llm.StreamEvent{
			llm.StreamMessageDelta{ContentDelta: "fallback", Role: "assistant"},
			llm.StreamRunCompleted{},
		},
	}
	rc := buildLuaRC(mainGW)
	rc.ResolveGatewayForRouteID = func(_ context.Context, _ string) (llm.Gateway, *routing.SelectedProviderRoute, error) {
		return nil, nil, errors.New("route missing")
	}

	evs := runLuaScript(t, `
local out, err = agent.stream_route("missing-route", "sys", "msg")
if err and string.find(err, "route_resolve_failed:", 1, true) == 1 then
  local fb, fbErr = agent.stream("sys", "msg")
  if fbErr then error(fbErr) end
end
`, rc)
	deltas := deltaTexts(evs)
	if len(deltas) == 0 || deltas[0] != "fallback" {
		t.Fatalf("expected fallback gateway delta after resolve error, got %v", deltas)
	}
	for _, ev := range evs {
		if ev.Type == "run.failed" {
			t.Fatalf("did not expect run.failed in resolve fallback path: %#v", ev.DataJSON)
		}
	}
}

func TestLuaExecutor_AgentStreamRoute_StreamStartedFailureEmitsRunFailed(t *testing.T) {
	mainGW := &luaSeqGateway{
		events: []llm.StreamEvent{
			llm.StreamMessageDelta{ContentDelta: "main", Role: "assistant"},
			llm.StreamRunCompleted{},
		},
	}
	routeGW := &luaSeqGateway{
		events: []llm.StreamEvent{
			llm.StreamMessageDelta{ContentDelta: "partial", Role: "assistant"},
			llm.StreamRunFailed{
				Error: llm.GatewayError{
					ErrorClass: llm.ErrorClassProviderNonRetryable,
					Message:    "route stream failed",
				},
			},
		},
	}
	rc := buildLuaRC(mainGW)
	rc.ResolveGatewayForRouteID = func(_ context.Context, routeID string) (llm.Gateway, *routing.SelectedProviderRoute, error) {
		return routeGW, &routing.SelectedProviderRoute{
			Route: routing.ProviderRouteRule{
				ID:    routeID,
				Model: "route-model",
			},
		}, nil
	}

	evs := runLuaScript(t, `
local out, err = agent.stream_route("final-route", "sys", "msg")
if err and string.find(err, "stream_terminal_failed:", 1, true) == 1 then
  return
end
if err then error(err) end
`, rc)

	deltas := deltaTexts(evs)
	if len(deltas) == 0 || deltas[0] != "partial" {
		t.Fatalf("expected partial delta before failure, got %v", deltas)
	}
	runFailedCount := 0
	for _, ev := range evs {
		if ev.Type == "run.failed" {
			runFailedCount++
			if msg, _ := ev.DataJSON["message"].(string); msg != "route stream failed" {
				t.Fatalf("unexpected run.failed message: %#v", ev.DataJSON)
			}
		}
	}
	if runFailedCount != 1 {
		t.Fatalf("expected 1 run.failed event, got %d", runFailedCount)
	}
}

func TestLuaExecutor_AgentStreamAgent_UsesResolvedAgentName(t *testing.T) {
	mainGW := &luaSeqGateway{
		events: []llm.StreamEvent{
			llm.StreamMessageDelta{ContentDelta: "main", Role: "assistant"},
			llm.StreamRunCompleted{},
		},
	}
	agentGW := &luaSeqGateway{
		events: []llm.StreamEvent{
			llm.StreamMessageDelta{ContentDelta: "agent", Role: "assistant"},
			llm.StreamRunCompleted{},
		},
	}
	rc := buildLuaRC(mainGW)
	var resolvedAgentName string
	rc.ResolveGatewayForAgentName = func(_ context.Context, agentName string) (llm.Gateway, *routing.SelectedProviderRoute, error) {
		resolvedAgentName = agentName
		return agentGW, &routing.SelectedProviderRoute{
			Route: routing.ProviderRouteRule{
				ID:    "agent-route",
				Model: "agent-model",
			},
		}, nil
	}

	evs := runLuaScript(t, `
local out, err = agent.stream_agent("sub-haiku-4.5", "sys", "msg")
if err then error(err) end
`, rc)
	if resolvedAgentName != "sub-haiku-4.5" {
		t.Fatalf("expected resolver called with sub-haiku-4.5, got %q", resolvedAgentName)
	}
	deltas := deltaTexts(evs)
	if len(deltas) == 0 || deltas[0] != "agent" {
		t.Fatalf("expected agent gateway delta, got %v", deltas)
	}
}

func TestLuaExecutor_AgentStreamAgent_ResolveFailedCanFallback(t *testing.T) {
	mainGW := &luaSeqGateway{
		events: []llm.StreamEvent{
			llm.StreamMessageDelta{ContentDelta: "fallback", Role: "assistant"},
			llm.StreamRunCompleted{},
		},
	}
	rc := buildLuaRC(mainGW)
	rc.ResolveGatewayForAgentName = func(_ context.Context, _ string) (llm.Gateway, *routing.SelectedProviderRoute, error) {
		return nil, nil, errors.New("agent not found")
	}

	evs := runLuaScript(t, `
local out, err = agent.stream_agent("sub-haiku-4.5", "sys", "msg")
if err and string.find(err, "agent_resolve_failed:", 1, true) == 1 then
  local fb, fbErr = agent.stream("sys", "msg")
  if fbErr then error(fbErr) end
end
`, rc)
	deltas := deltaTexts(evs)
	if len(deltas) == 0 || deltas[0] != "fallback" {
		t.Fatalf("expected fallback delta after agent resolve error, got %v", deltas)
	}
}

// --- tools.call_parallel tests ---

func TestLuaExecutor_ToolsCallParallel_EmptyCalls(t *testing.T) {
	rc := buildLuaRC(nil)
	evs := runLuaScript(t, `
local results, errs = tools.call_parallel({})
context.set_output(tostring(#results) .. ":" .. tostring(#errs))
`, rc)

	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != "0:0" {
		t.Fatalf("expected '0:0', got: %v", texts)
	}
}

func TestLuaExecutor_ToolsCallParallel_ExecutorNil(t *testing.T) {
	rc := buildLuaRC(nil)
	rc.ToolExecutor = nil
	evs := runLuaScript(t, `
local results, errs = tools.call_parallel({{name="web_search", args='{"query":"test"}'}})
if results == nil then
  context.set_output("nil_exec")
end
`, rc)

	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != "nil_exec" {
		t.Fatalf("expected 'nil_exec', got: %v", texts)
	}
}

func TestLuaExecutor_Sandbox_OsBlocked(t *testing.T) {
	evs := runLuaScript(t, `
if os == nil then
  context.set_output("os_blocked")
else
  context.set_output("os_available")
end
`, buildLuaRC(nil))

	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != "os_blocked" {
		t.Fatalf("expected os to be blocked, got: %v", texts)
	}
}

func TestLuaExecutor_Sandbox_IoBlocked(t *testing.T) {
	evs := runLuaScript(t, `
if io == nil then
  context.set_output("io_blocked")
else
  context.set_output("io_available")
end
`, buildLuaRC(nil))

	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != "io_blocked" {
		t.Fatalf("expected io to be blocked, got: %v", texts)
	}
}

func TestLuaExecutor_Sandbox_DebugBlocked(t *testing.T) {
	evs := runLuaScript(t, `
if debug == nil then
  context.set_output("debug_blocked")
else
  context.set_output("debug_available")
end
`, buildLuaRC(nil))

	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != "debug_blocked" {
		t.Fatalf("expected debug to be blocked, got: %v", texts)
	}
}

func TestLuaExecutor_Sandbox_DofileBlocked(t *testing.T) {
	evs := runLuaScript(t, `
if dofile == nil then
  context.set_output("dofile_blocked")
else
  context.set_output("dofile_available")
end
`, buildLuaRC(nil))

	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != "dofile_blocked" {
		t.Fatalf("expected dofile to be blocked, got: %v", texts)
	}
}

func TestLuaExecutor_Sandbox_SafeLibsAvailable(t *testing.T) {
	evs := runLuaScript(t, `
local result = tostring(type(string.len)) .. "," .. tostring(type(math.abs)) .. "," .. tostring(type(table.insert))
context.set_output(result)
`, buildLuaRC(nil))

	texts := deltaTexts(evs)
	if len(texts) == 0 || texts[0] != "function,function,function" {
		t.Fatalf("expected safe libs to be available, got: %v", texts)
	}
}
