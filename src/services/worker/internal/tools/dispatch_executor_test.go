package tools

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"arkloop/services/worker/internal/events"
)

type recordingExecutor struct {
	mu         sync.Mutex
	calledWith string
}

type fixedResultExecutor struct {
	mu      sync.Mutex
	context ExecutionContext
	result  ExecutionResult
}

func (e *recordingExecutor) Execute(
	_ context.Context,
	toolName string,
	_ map[string]any,
	_ ExecutionContext,
	_ string,
) ExecutionResult {
	e.mu.Lock()
	e.calledWith = toolName
	e.mu.Unlock()
	return ExecutionResult{ResultJSON: map[string]any{"ok": true}}
}

func (e *recordingExecutor) CalledWith() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calledWith
}

func (e *fixedResultExecutor) Execute(
	_ context.Context,
	_ string,
	_ map[string]any,
	ctx ExecutionContext,
	_ string,
) ExecutionResult {
	e.mu.Lock()
	e.context = ctx
	e.mu.Unlock()
	return e.result
}

func (e *fixedResultExecutor) Context() ExecutionContext {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.context
}

type blockingExecutor struct {
	started chan struct{}
}

func (e *blockingExecutor) Execute(
	ctx context.Context,
	_ string,
	_ map[string]any,
	_ ExecutionContext,
	_ string,
) ExecutionResult {
	close(e.started)
	<-ctx.Done()
	return ExecutionResult{
		Error: &ExecutionError{
			ErrorClass: "executor.cancelled",
			Message:    ctx.Err().Error(),
		},
	}
}

type panicExecutor struct{}

func (panicExecutor) Execute(
	_ context.Context,
	_ string,
	_ map[string]any,
	_ ExecutionContext,
	_ string,
) ExecutionResult {
	panic("boom")
}

func TestDispatchingExecutorResolvesLlmNameToProvider(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(AgentToolSpec{
		Name:        "web_search.tavily",
		LlmName:     "web_search",
		Version:     "1",
		Description: "x",
		RiskLevel:   RiskLevelLow,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	allowlist := AllowlistFromNames([]string{"web_search.tavily"})
	policy := NewPolicyEnforcer(registry, allowlist)
	dispatch := NewDispatchingExecutor(registry, policy)

	exec := &recordingExecutor{}
	if err := dispatch.Bind("web_search.tavily", exec); err != nil {
		t.Fatalf("bind failed: %v", err)
	}

	ctx := context.Background()
	emit := events.NewEmitter("trace")
	result := dispatch.Execute(ctx, "web_search", map[string]any{"query": "x"}, ExecutionContext{Emitter: emit}, "call1")
	if result.Error != nil {
		t.Fatalf("unexpected error: %+v", result.Error)
	}
	if got := exec.CalledWith(); got != "web_search.tavily" {
		t.Fatalf("expected web_search.tavily, got %q", got)
	}
}

func TestDispatchingExecutorBindsDuckduckgoProviderName(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(AgentToolSpec{
		Name:        "web_search.duckduckgo",
		LlmName:     "web_search",
		Version:     "1",
		Description: "x",
		RiskLevel:   RiskLevelLow,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	allowlist := AllowlistFromNames([]string{"web_search.duckduckgo"})
	policy := NewPolicyEnforcer(registry, allowlist)
	dispatch := NewDispatchingExecutor(registry, policy)

	exec := &recordingExecutor{}
	if err := dispatch.Bind("web_search.duckduckgo", exec); err != nil {
		t.Fatalf("bind failed: %v", err)
	}

	result := dispatch.Execute(
		context.Background(),
		"web_search",
		map[string]any{"query": "x"},
		ExecutionContext{Emitter: events.NewEmitter("trace")},
		"call1",
	)
	if result.Error != nil {
		t.Fatalf("unexpected error: %+v", result.Error)
	}
	if got := exec.CalledWith(); got != "web_search.duckduckgo" {
		t.Fatalf("expected web_search.duckduckgo, got %q", got)
	}
}

func TestDispatchingExecutorUsesLegacyNameWhenBound(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(AgentToolSpec{
		Name:        "web_search",
		Version:     "1",
		Description: "x",
		RiskLevel:   RiskLevelLow,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	allowlist := AllowlistFromNames([]string{"web_search"})
	policy := NewPolicyEnforcer(registry, allowlist)
	dispatch := NewDispatchingExecutor(registry, policy)

	exec := &recordingExecutor{}
	if err := dispatch.Bind("web_search", exec); err != nil {
		t.Fatalf("bind failed: %v", err)
	}

	ctx := context.Background()
	emit := events.NewEmitter("trace")
	result := dispatch.Execute(ctx, "web_search", map[string]any{"query": "x"}, ExecutionContext{Emitter: emit}, "call1")
	if result.Error != nil {
		t.Fatalf("unexpected error: %+v", result.Error)
	}
	if got := exec.CalledWith(); got != "web_search" {
		t.Fatalf("expected web_search, got %q", got)
	}
}

func TestDispatchingExecutorBypassesCompressionAndSummarizationForGenerativeUIBootstrapTools(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(AgentToolSpec{
		Name:        "visualize_read_me",
		Version:     "1",
		Description: "x",
		RiskLevel:   RiskLevelLow,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	allowlist := AllowlistFromNames([]string{"visualize_read_me"})
	policy := NewPolicyEnforcer(registry, allowlist)
	dispatch := NewDispatchingExecutor(registry, policy)
	dispatch.SetSummarizer(NewResultSummarizer(&mockGateway{response: "should not run"}, "test-model", 10, ResultSummarizerConfig{Prompt: "compress", MaxTokens: 32}))

	longGuidelines := strings.Repeat("guideline line\n", 5000)
	exec := &fixedResultExecutor{
		result: ExecutionResult{
			ResultJSON: map[string]any{
				"guidelines": longGuidelines,
			},
		},
	}
	if err := dispatch.Bind("visualize_read_me", exec); err != nil {
		t.Fatalf("bind failed: %v", err)
	}

	result := dispatch.Execute(
		context.Background(),
		"visualize_read_me",
		map[string]any{"modules": []string{"interactive"}},
		ExecutionContext{Emitter: events.NewEmitter("trace")},
		"call_bootstrap",
	)
	if result.Error != nil {
		t.Fatalf("unexpected error: %+v", result.Error)
	}
	if result.ResultJSON["guidelines"] != longGuidelines {
		t.Fatal("guidelines should pass through without compression")
	}
	if _, ok := result.ResultJSON["_compressed"]; ok {
		t.Fatal("bootstrap result should not be compressed")
	}
	if _, ok := result.ResultJSON["_summarized"]; ok {
		t.Fatal("bootstrap result should not be summarized")
	}
	if exec.Context().GenerativeUIReadMeSeen {
		t.Fatal("bootstrap tool should not see read_me flag before it runs")
	}
	if !dispatch.generativeUIReadMeSeen {
		t.Fatal("dispatch should remember read_me at run scope after bootstrap tool succeeds")
	}
}

func TestDispatchingExecutorHardTimeout(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(AgentToolSpec{
		Name:        "slow_tool",
		Version:     "1",
		Description: "slow tool",
		RiskLevel:   RiskLevelLow,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	dispatch := NewDispatchingExecutor(registry, NewPolicyEnforcer(registry, AllowlistFromNames([]string{"slow_tool"})))
	exec := &blockingExecutor{started: make(chan struct{})}
	if err := dispatch.Bind("slow_tool", exec); err != nil {
		t.Fatalf("bind failed: %v", err)
	}

	timeoutMs := 20
	result := dispatch.Execute(
		context.Background(),
		"slow_tool",
		nil,
		ExecutionContext{Emitter: events.NewEmitter("trace"), TimeoutMs: &timeoutMs},
		"call_slow",
	)
	<-exec.started

	if result.Error == nil {
		t.Fatal("expected timeout error")
	}
	if result.Error.ErrorClass != ErrorClassToolHardTimeout {
		t.Fatalf("expected %s, got %s", ErrorClassToolHardTimeout, result.Error.ErrorClass)
	}
	if got, ok := result.Error.Details["timeout_ms"].(int); !ok || got != timeoutMs {
		t.Fatalf("expected timeout_ms=%d, got %#v", timeoutMs, result.Error.Details["timeout_ms"])
	}
}

func TestDispatchingExecutorReturnsCancellationWhenParentContextDone(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(AgentToolSpec{
		Name:        "slow_cancel",
		Version:     "1",
		Description: "slow cancel",
		RiskLevel:   RiskLevelLow,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	dispatch := NewDispatchingExecutor(registry, NewPolicyEnforcer(registry, AllowlistFromNames([]string{"slow_cancel"})))
	exec := &blockingExecutor{started: make(chan struct{})}
	if err := dispatch.Bind("slow_cancel", exec); err != nil {
		t.Fatalf("bind failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-exec.started
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	result := dispatch.Execute(
		ctx,
		"slow_cancel",
		nil,
		ExecutionContext{Emitter: events.NewEmitter("trace")},
		"call_cancel",
	)
	if result.Error == nil {
		t.Fatal("expected cancellation error")
	}
	if result.Error.ErrorClass != ErrorClassToolExecutionFailed {
		t.Fatalf("expected %s, got %s", ErrorClassToolExecutionFailed, result.Error.ErrorClass)
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("expected parent context canceled, got %v", ctx.Err())
	}
}

func TestDispatchingExecutorRecoversPanicsInsideTimeoutWrapper(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(AgentToolSpec{
		Name:        "panic_tool",
		Version:     "1",
		Description: "panic tool",
		RiskLevel:   RiskLevelLow,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	dispatch := NewDispatchingExecutor(registry, NewPolicyEnforcer(registry, AllowlistFromNames([]string{"panic_tool"})))
	if err := dispatch.Bind("panic_tool", panicExecutor{}); err != nil {
		t.Fatalf("bind failed: %v", err)
	}

	result := dispatch.Execute(
		context.Background(),
		"panic_tool",
		nil,
		ExecutionContext{Emitter: events.NewEmitter("trace")},
		"call_panic",
	)
	if result.Error == nil {
		t.Fatal("expected panic to be converted into execution error")
	}
	if result.Error.ErrorClass != ErrorClassToolExecutionFailed {
		t.Fatalf("expected %s, got %s", ErrorClassToolExecutionFailed, result.Error.ErrorClass)
	}
}
