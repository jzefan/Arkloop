package executor

import (
	"context"
	"errors"
	"testing"

	"arkloop/services/worker/internal/events"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/pipeline"
	"arkloop/services/worker/internal/routing"
)

// funcGateway 允许每个测试用例提供自己的 Stream 逻辑。
type funcGateway struct {
	calls []llm.Request
	fn    func(callIdx int, req llm.Request, yield func(llm.StreamEvent) error) error
}

func (g *funcGateway) Stream(_ context.Context, req llm.Request, yield func(llm.StreamEvent) error) error {
	idx := len(g.calls)
	g.calls = append(g.calls, req)
	return g.fn(idx, req, yield)
}

// twoPhaseStream 构造一个双阶段 gateway：第 0 次调用返回分类文本，第 1 次调用返回内容 + completed。
func twoPhaseStream(category string, responseText string) *funcGateway {
	return &funcGateway{
		fn: func(idx int, _ llm.Request, yield func(llm.StreamEvent) error) error {
			if idx == 0 {
				if err := yield(llm.StreamMessageDelta{ContentDelta: category, Role: "assistant"}); err != nil {
					return err
				}
				return yield(llm.StreamRunCompleted{})
			}
			if err := yield(llm.StreamMessageDelta{ContentDelta: responseText, Role: "assistant"}); err != nil {
				return err
			}
			return yield(llm.StreamRunCompleted{})
		},
	}
}

func buildClassifyRC(gw llm.Gateway) *pipeline.RunContext {
	return &pipeline.RunContext{
		Gateway:  gw,
		Messages: []llm.Message{{Role: "user", Content: []llm.TextPart{{Text: "hello"}}}},
		SelectedRoute: &routing.SelectedProviderRoute{
			Route: routing.ProviderRouteRule{
				ID:    "default",
				Model: "default-model",
			},
		},
	}
}

func minimalClassifyConfig() map[string]any {
	return map[string]any{
		"classify_prompt": "Classify as technical or general.",
		"routes": map[string]any{
			"technical": map[string]any{"prompt_override": "You are a technical expert."},
			"general":   map[string]any{"prompt_override": "You are a general assistant."},
		},
	}
}

// --- 配置校验测试 ---

func TestNewClassifyRouteExecutor_NilConfig(t *testing.T) {
	_, err := NewClassifyRouteExecutor(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestNewClassifyRouteExecutor_MissingClassifyPrompt(t *testing.T) {
	cfg := map[string]any{
		"routes": map[string]any{
			"a": map[string]any{"prompt_override": "p"},
		},
	}
	_, err := NewClassifyRouteExecutor(cfg)
	if err == nil {
		t.Fatal("expected error for missing classify_prompt")
	}
}

func TestNewClassifyRouteExecutor_MissingRoutes(t *testing.T) {
	cfg := map[string]any{"classify_prompt": "classify"}
	_, err := NewClassifyRouteExecutor(cfg)
	if err == nil {
		t.Fatal("expected error for missing routes")
	}
}

func TestNewClassifyRouteExecutor_EmptyRoutes(t *testing.T) {
	cfg := map[string]any{
		"classify_prompt": "classify",
		"routes":          map[string]any{},
	}
	_, err := NewClassifyRouteExecutor(cfg)
	if err == nil {
		t.Fatal("expected error for empty routes")
	}
}

func TestNewClassifyRouteExecutor_InvalidDefaultRoute(t *testing.T) {
	cfg := map[string]any{
		"classify_prompt": "classify",
		"default_route":   "nonexistent",
		"routes": map[string]any{
			"a": map[string]any{"prompt_override": "p"},
		},
	}
	_, err := NewClassifyRouteExecutor(cfg)
	if err == nil {
		t.Fatal("expected error for invalid default_route")
	}
}

func TestNewClassifyRouteExecutor_MissingPromptOverride(t *testing.T) {
	cfg := map[string]any{
		"classify_prompt": "classify",
		"routes": map[string]any{
			"a": map[string]any{"other_field": "x"},
		},
	}
	_, err := NewClassifyRouteExecutor(cfg)
	if err == nil {
		t.Fatal("expected error for missing prompt_override in route")
	}
}

// --- 执行路径测试 ---

func TestClassifyRouteExecutor_HappyPath(t *testing.T) {
	gw := twoPhaseStream("technical", "technical response")
	ex, err := NewClassifyRouteExecutor(minimalClassifyConfig())
	if err != nil {
		t.Fatalf("factory failed: %v", err)
	}

	rc := buildClassifyRC(gw)
	emitter := events.NewEmitter("trace")

	var collected []events.RunEvent
	if err := ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error {
		collected = append(collected, ev)
		return nil
	}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// 验证 classify call 的 system prompt
	if len(gw.calls) != 2 {
		t.Fatalf("expected 2 gateway calls, got %d", len(gw.calls))
	}
	classifyMsg := gw.calls[0].Messages[0]
	if classifyMsg.Role != "system" || classifyMsg.Content[0].Text != "Classify as technical or general." {
		t.Fatalf("unexpected classify system prompt: %+v", classifyMsg)
	}

	// 验证 execute call 的 system prompt = routes["technical"].prompt_override
	executeMsg := gw.calls[1].Messages[0]
	if executeMsg.Role != "system" || executeMsg.Content[0].Text != "You are a technical expert." {
		t.Fatalf("unexpected execute system prompt: %+v", executeMsg)
	}

	// 验证事件
	eventTypes := make([]string, 0, len(collected))
	for _, ev := range collected {
		eventTypes = append(eventTypes, ev.Type)
	}
	if len(collected) != 2 {
		t.Fatalf("expected 2 events, got %v", eventTypes)
	}
	if collected[0].Type != "message.delta" {
		t.Fatalf("expected message.delta, got %q", collected[0].Type)
	}
	if collected[1].Type != "run.completed" {
		t.Fatalf("expected run.completed, got %q", collected[1].Type)
	}
}

func TestClassifyRouteExecutor_DefaultRouteFallback(t *testing.T) {
	gw := twoPhaseStream("unknown_category", "fallback response")
	cfg := map[string]any{
		"classify_prompt": "classify",
		"default_route":   "general",
		"routes": map[string]any{
			"technical": map[string]any{"prompt_override": "tech prompt"},
			"general":   map[string]any{"prompt_override": "general prompt"},
		},
	}
	ex, err := NewClassifyRouteExecutor(cfg)
	if err != nil {
		t.Fatalf("factory failed: %v", err)
	}

	rc := buildClassifyRC(gw)
	emitter := events.NewEmitter("trace")

	var collected []events.RunEvent
	if err := ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error {
		collected = append(collected, ev)
		return nil
	}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// execute 应使用 default_route="general" 的 prompt_override
	executeMsg := gw.calls[1].Messages[0]
	if executeMsg.Content[0].Text != "general prompt" {
		t.Fatalf("expected general prompt, got %q", executeMsg.Content[0].Text)
	}
	if len(collected) < 1 || collected[len(collected)-1].Type != "run.completed" {
		t.Fatalf("expected run.completed as last event")
	}
}

func TestClassifyRouteExecutor_NoMatchNoDefault(t *testing.T) {
	gw := twoPhaseStream("unknown_category", "")
	ex, err := NewClassifyRouteExecutor(minimalClassifyConfig())
	if err != nil {
		t.Fatalf("factory failed: %v", err)
	}

	rc := buildClassifyRC(gw)
	emitter := events.NewEmitter("trace")

	var collected []events.RunEvent
	if err := ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error {
		collected = append(collected, ev)
		return nil
	}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// 应只调用 1 次 gateway（classify），不进入 execute
	if len(gw.calls) != 1 {
		t.Fatalf("expected 1 gateway call, got %d", len(gw.calls))
	}
	if len(collected) != 1 || collected[0].Type != "run.failed" {
		t.Fatalf("expected run.failed, got %+v", collected)
	}
	if collected[0].ErrorClass == nil || *collected[0].ErrorClass != "task.classify_route.no_match" {
		t.Fatalf("unexpected error_class: %v", collected[0].ErrorClass)
	}
}

func TestClassifyRouteExecutor_ModelOverride(t *testing.T) {
	gw := twoPhaseStream("technical", "response")
	cfg := map[string]any{
		"classify_prompt": "classify",
		"routes": map[string]any{
			"technical": map[string]any{
				"prompt_override": "tech prompt",
				"model_override":  "claude-3-5-sonnet",
			},
		},
	}
	ex, err := NewClassifyRouteExecutor(cfg)
	if err != nil {
		t.Fatalf("factory failed: %v", err)
	}

	rc := buildClassifyRC(gw)
	emitter := events.NewEmitter("trace")
	_ = ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error { return nil })

	if len(gw.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(gw.calls))
	}
	if gw.calls[1].Model != "claude-3-5-sonnet" {
		t.Fatalf("expected model=claude-3-5-sonnet, got %q", gw.calls[1].Model)
	}
	// classify call 应沿用默认 model
	if gw.calls[0].Model != "default-model" {
		t.Fatalf("expected classify model=default-model, got %q", gw.calls[0].Model)
	}
}

func TestClassifyRouteExecutor_ClassifyPhaseFailed(t *testing.T) {
	gw := &funcGateway{
		fn: func(idx int, _ llm.Request, yield func(llm.StreamEvent) error) error {
			return yield(llm.StreamRunFailed{
				Error: llm.GatewayError{
					ErrorClass: llm.ErrorClassProviderNonRetryable,
					Message:    "provider error",
				},
			})
		},
	}
	ex, err := NewClassifyRouteExecutor(minimalClassifyConfig())
	if err != nil {
		t.Fatalf("factory failed: %v", err)
	}

	rc := buildClassifyRC(gw)
	emitter := events.NewEmitter("trace")

	var collected []events.RunEvent
	if err := ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error {
		collected = append(collected, ev)
		return nil
	}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// 应只调用 1 次（classify 失败），不进入 execute
	if len(gw.calls) != 1 {
		t.Fatalf("expected 1 gateway call, got %d", len(gw.calls))
	}
	if len(collected) != 1 || collected[0].Type != "run.failed" {
		t.Fatalf("expected run.failed, got %+v", collected)
	}
}

func TestClassifyRouteExecutor_ExecutePhaseFailed(t *testing.T) {
	gw := &funcGateway{
		fn: func(idx int, _ llm.Request, yield func(llm.StreamEvent) error) error {
			if idx == 0 {
				// classify: 返回有效 category
				_ = yield(llm.StreamMessageDelta{ContentDelta: "technical", Role: "assistant"})
				return yield(llm.StreamRunCompleted{})
			}
			// execute: 失败
			return yield(llm.StreamRunFailed{
				Error: llm.GatewayError{
					ErrorClass: llm.ErrorClassProviderRetryable,
					Message:    "timeout",
				},
			})
		},
	}
	ex, err := NewClassifyRouteExecutor(minimalClassifyConfig())
	if err != nil {
		t.Fatalf("factory failed: %v", err)
	}

	rc := buildClassifyRC(gw)
	emitter := events.NewEmitter("trace")

	var collected []events.RunEvent
	if err := ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error {
		collected = append(collected, ev)
		return nil
	}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(gw.calls) != 2 {
		t.Fatalf("expected 2 gateway calls, got %d", len(gw.calls))
	}
	if len(collected) != 1 || collected[0].Type != "run.failed" {
		t.Fatalf("expected run.failed, got %+v", collected)
	}
}

func TestClassifyRouteExecutor_YieldErrorPropagated(t *testing.T) {
	gw := twoPhaseStream("technical", "response text")
	ex, err := NewClassifyRouteExecutor(minimalClassifyConfig())
	if err != nil {
		t.Fatalf("factory failed: %v", err)
	}

	rc := buildClassifyRC(gw)
	emitter := events.NewEmitter("trace")
	yieldErr := errors.New("downstream closed")

	err = ex.Execute(context.Background(), rc, emitter, func(ev events.RunEvent) error {
		return yieldErr
	})
	if !errors.Is(err, yieldErr) {
		t.Fatalf("expected yieldErr to propagate, got: %v", err)
	}
}
