package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"

	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/tools"
)

type traceRecord struct {
	middleware string
	event      string
	fields     map[string]any
}

type spyTracer struct {
	records []traceRecord
}

func (s *spyTracer) Event(middleware, event string, fields map[string]any) {
	cloned := map[string]any{}
	for k, v := range fields {
		cloned[k] = v
	}
	s.records = append(s.records, traceRecord{
		middleware: middleware,
		event:      event,
		fields:     cloned,
	})
}

type fakeContextContributor struct {
	name      string
	before    PromptSegments
	after     PromptSegments
	beforeErr error
	afterErr  error
}

func (f fakeContextContributor) HookProviderName() string { return f.name }
func (f fakeContextContributor) BeforePromptSegments(context.Context, *RunContext) (PromptSegments, error) {
	if f.beforeErr != nil {
		return nil, f.beforeErr
	}
	return f.before, nil
}
func (f fakeContextContributor) AfterPromptSegments(context.Context, *RunContext, string) (PromptSegments, error) {
	if f.afterErr != nil {
		return nil, f.afterErr
	}
	return f.after, nil
}

type fakeCompactionAdvisor struct {
	name      string
	before    CompactHints
	after     PostCompactActions
	beforeErr error
	afterErr  error
}

func (f fakeCompactionAdvisor) HookProviderName() string { return f.name }
func (f fakeCompactionAdvisor) BeforeCompact(context.Context, *RunContext, CompactInput) (CompactHints, error) {
	if f.beforeErr != nil {
		return nil, f.beforeErr
	}
	return f.before, nil
}
func (f fakeCompactionAdvisor) AfterCompact(context.Context, *RunContext, CompactOutput) (PostCompactActions, error) {
	if f.afterErr != nil {
		return nil, f.afterErr
	}
	return f.after, nil
}

type fakeThreadProvider struct {
	name   string
	result ThreadPersistResult
}

func (f fakeThreadProvider) HookProviderName() string { return f.name }
func (f fakeThreadProvider) PersistThread(context.Context, *RunContext, ThreadDelta, ThreadPersistHints) ThreadPersistResult {
	return f.result
}

type fakeBeforeThreadPersistHook struct {
	name   string
	before ThreadPersistHints
	err    error
}

func (f fakeBeforeThreadPersistHook) HookProviderName() string { return f.name }
func (f fakeBeforeThreadPersistHook) BeforeThreadPersist(context.Context, *RunContext, ThreadDelta) (ThreadPersistHints, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.before, nil
}

type fakeThreadObserver struct {
	name  string
	after PersistObservers
	err   error
}

func (f fakeThreadObserver) HookProviderName() string { return f.name }
func (f fakeThreadObserver) AfterThreadPersist(context.Context, *RunContext, ThreadDelta, ThreadPersistResult) (PersistObservers, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.after, nil
}

type fakeModelLifecycleHook struct {
	name      string
	before    ModelCallHints
	afterResp PostResponseActions
	afterTool PostToolActions
}

func (f fakeModelLifecycleHook) HookProviderName() string { return f.name }
func (f fakeModelLifecycleHook) BeforeModelCall(context.Context, *RunContext, llm.Request) (ModelCallHints, error) {
	return f.before, nil
}
func (f fakeModelLifecycleHook) AfterModelResponse(context.Context, *RunContext, ModelResponse) (PostResponseActions, error) {
	return f.afterResp, nil
}
func (f fakeModelLifecycleHook) AfterToolCall(context.Context, *RunContext, llm.ToolCall, tools.ExecutionResult) (PostToolActions, error) {
	return f.afterTool, nil
}

func TestHookRegistrySetThreadProviderRejectsSecondProvider(t *testing.T) {
	registry := NewHookRegistry()
	first := fakeThreadProvider{name: "first"}
	second := fakeThreadProvider{name: "second"}

	if err := registry.SetThreadPersistenceProvider(first); err != nil {
		t.Fatalf("first set failed: %v", err)
	}
	if err := registry.SetThreadPersistenceProvider(second); err == nil {
		t.Fatal("expected second thread provider to fail")
	}
}

func TestHookRuntimeBeforePromptAssembleSortsAndIgnoresErrors(t *testing.T) {
	registry := NewHookRegistry()
	registry.RegisterContextContributor(fakeContextContributor{
		name:      "bad",
		beforeErr: errors.New("boom"),
	})
	registry.RegisterContextContributor(fakeContextContributor{
		name: "good-a",
		before: PromptSegments{
			{Name: "a", Target: PromptTargetSystemPrefix, Role: "system", Text: "A"},
		},
	})
	registry.RegisterContextContributor(fakeContextContributor{
		name: "good-b",
		before: PromptSegments{
			{Name: "b", Target: PromptTargetSystemPrefix, Role: "system", Text: "B"},
		},
	})
	rt := NewHookRuntime(registry, NewDefaultHookResultApplier())
	tracer := &spyTracer{}
	rc := &RunContext{Tracer: tracer}

	segments := rt.BeforePromptSegments(context.Background(), rc, "hook.before")
	if len(segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segments))
	}
	if got := segments[0].Name; got != "a" {
		t.Fatalf("expected first normalized segment name a, got %s", got)
	}
	if got := segments[1].Name; got != "b" {
		t.Fatalf("expected second normalized segment name b, got %s", got)
	}

	failed := findTraceEvent(tracer.records, "runtime_hook.failed")
	if failed == nil {
		t.Fatal("expected runtime_hook.failed event")
	}
	if failed.fields["hook_name"] != string(HookBeforePromptAssemble) {
		t.Fatalf("unexpected hook_name: %#v", failed.fields["hook_name"])
	}
	if failed.fields["provider"] != "bad" {
		t.Fatalf("unexpected provider in failed trace: %#v", failed.fields["provider"])
	}
}

func TestHookRuntimeAllNineHooksEmitTraceAndReturnData(t *testing.T) {
	registry := NewHookRegistry()
	registry.RegisterContextContributor(fakeContextContributor{
		name: "ctx",
		before: PromptSegments{
			{Name: "nb", Target: PromptTargetSystemPrefix, Role: "system", Text: "note"},
		},
		after: PromptSegments{
			{Name: "imp", Target: PromptTargetSystemPrefix, Role: "system", Text: "impress"},
		},
	})
	registry.RegisterModelLifecycleHook(fakeModelLifecycleHook{
		name: "model",
		before: ModelCallHints{
			{Key: "route", Value: "main", Priority: 1},
		},
		afterResp: PostResponseActions{
			{Key: "capture", Value: "yes", Priority: 1},
		},
		afterTool: PostToolActions{
			{Key: "audit", Value: "ok", Priority: 1},
		},
	})
	registry.RegisterCompactionAdvisor(fakeCompactionAdvisor{
		name: "compact",
		before: CompactHints{
			{Content: "keep ids", Priority: 1},
		},
		after: PostCompactActions{
			{Key: "post", Value: "ok", Priority: 1},
		},
	})
	registry.RegisterBeforeThreadPersistHook(fakeBeforeThreadPersistHook{
		name: "thread-hints",
		before: ThreadPersistHints{
			{Key: "target", Value: "external", Priority: 1},
		},
	})
	if err := registry.SetThreadPersistenceProvider(fakeThreadProvider{
		name:   "thread",
		result: ThreadPersistResult{Handled: true, Provider: "thread"},
	}); err != nil {
		t.Fatalf("set thread provider: %v", err)
	}
	registry.RegisterAfterThreadPersistHook(fakeThreadObserver{
		name: "observer",
		after: PersistObservers{
			{Key: "observe", Value: "v", Priority: 1},
		},
	})

	rt := NewHookRuntime(registry, NewDefaultHookResultApplier())
	tracer := &spyTracer{}
	rc := &RunContext{Tracer: tracer}

	if got := len(rt.BeforePromptSegments(context.Background(), rc, "hook.before")); got != 1 {
		t.Fatalf("before prompt results = %d, want 1", got)
	}
	if got := len(rt.AfterPromptSegments(context.Background(), rc, "x", "hook.after")); got != 1 {
		t.Fatalf("after prompt results = %d, want 1", got)
	}
	if got := len(rt.BeforeModelCall(context.Background(), rc, llm.Request{})); got != 1 {
		t.Fatalf("before model results = %d, want 1", got)
	}
	if got := len(rt.AfterModelResponse(context.Background(), rc, ModelResponse{})); got != 1 {
		t.Fatalf("after model results = %d, want 1", got)
	}
	if got := len(rt.AfterToolCall(context.Background(), rc, llm.ToolCall{}, tools.ExecutionResult{})); got != 1 {
		t.Fatalf("after tool results = %d, want 1", got)
	}
	if got := len(rt.BeforeCompact(context.Background(), rc, CompactInput{})); got != 1 {
		t.Fatalf("before compact results = %d, want 1", got)
	}
	if got := len(rt.AfterCompact(context.Background(), rc, CompactOutput{})); got != 1 {
		t.Fatalf("after compact results = %d, want 1", got)
	}
	if got := len(rt.BeforeThreadPersist(context.Background(), rc, ThreadDelta{})); got != 1 {
		t.Fatalf("before thread results = %d, want 1", got)
	}
	if got := rt.ExecuteThreadPersist(context.Background(), rc, ThreadDelta{}, ThreadPersistHints{}); !got.Handled {
		t.Fatalf("expected thread provider to handle persist, got %#v", got)
	}
	if got := len(rt.AfterThreadPersist(context.Background(), rc, ThreadDelta{}, ThreadPersistResult{})); got != 1 {
		t.Fatalf("after thread results = %d, want 1", got)
	}

	invoked := countTraceEvents(tracer.records, "runtime_hook.invoked")
	completed := countTraceEvents(tracer.records, "runtime_hook.completed")
	wantEvents := len(allHookNames) + 1 // thread provider execution records the persist stage separately
	if invoked != wantEvents {
		t.Fatalf("invoked events = %d, want %d", invoked, wantEvents)
	}
	if completed != wantEvents {
		t.Fatalf("completed events = %d, want %d", completed, wantEvents)
	}

	for _, record := range tracer.records {
		if !strings.HasPrefix(record.event, "runtime_hook.") {
			continue
		}
		if _, ok := record.fields["hook_name"]; !ok {
			t.Fatalf("trace missing hook_name: %#v", record)
		}
		if _, ok := record.fields["provider"]; !ok {
			t.Fatalf("trace missing provider: %#v", record)
		}
		if _, ok := record.fields["duration_ms"]; !ok {
			t.Fatalf("trace missing duration_ms: %#v", record)
		}
		if _, ok := record.fields["status"]; !ok {
			t.Fatalf("trace missing status: %#v", record)
		}
		if _, ok := record.fields["result_count"]; !ok {
			t.Fatalf("trace missing result_count: %#v", record)
		}
	}
}

func TestDefaultHookResultApplierAppliesCompactHintsInOrder(t *testing.T) {
	applier := NewDefaultHookResultApplier()

	compact := applier.ApplyCompactHints(CompactInput{SystemPrompt: "p"}, CompactHints{
		{Content: "hint-b", Priority: 20},
		{Content: "hint-a", Priority: 10},
	})
	if !strings.Contains(compact.SystemPrompt, "<compact_hints>\nhint-a\nhint-b\n</compact_hints>") {
		t.Fatalf("compact hints block invalid: %s", compact.SystemPrompt)
	}
}

func findTraceEvent(records []traceRecord, event string) *traceRecord {
	for i := range records {
		if records[i].event == event {
			return &records[i]
		}
	}
	return nil
}

func countTraceEvents(records []traceRecord, event string) int {
	count := 0
	for _, record := range records {
		if record.event == event {
			count++
		}
	}
	return count
}
