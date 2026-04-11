package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sharedconfig "arkloop/services/shared/config"
	sharedoutbound "arkloop/services/shared/outboundurl"
	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/memory/nowledge"

	"github.com/google/uuid"
)

type nowledgeLinkStoreStub struct{}

func (nowledgeLinkStoreStub) Get(context.Context, uuid.UUID, uuid.UUID, string) (string, bool, error) {
	return "", false, fmt.Errorf("unexpected link lookup")
}

func (nowledgeLinkStoreStub) Upsert(context.Context, uuid.UUID, uuid.UUID, string, string) error {
	return fmt.Errorf("unexpected link upsert")
}

type nowledgeResolverStub struct {
	values map[string]string
}

func (s nowledgeResolverStub) Resolve(_ context.Context, key string, _ sharedconfig.Scope) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", fmt.Errorf("not found")
	}
	return value, nil
}

func (s nowledgeResolverStub) ResolvePrefix(context.Context, string, sharedconfig.Scope) (map[string]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestNowledgeDistillObserverSkipsWhenDisabled(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected nowledge request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	provider := nowledge.NewClient(nowledge.Config{BaseURL: srv.URL})
	observer := NewNowledgeDistillObserver(provider, nowledgeLinkStoreStub{}, nowledgeResolverStub{
		values: map[string]string{"memory.distill_enabled": "false"},
	})
	if observer == nil {
		t.Fatal("expected observer")
	}

	userID := uuid.New()
	rc := &RunContext{
		Run: data.Run{
			ID:        uuid.New(),
			AccountID: uuid.New(),
			ThreadID:  uuid.New(),
		},
		UserID: &userID,
	}

	delta := ThreadDelta{
		AccountID:       rc.Run.AccountID,
		ThreadID:        rc.Run.ThreadID,
		UserID:          userID,
		AgentID:         "user_" + userID.String(),
		Messages:        []ThreadDeltaMessage{{Role: "user", Content: "你好"}, {Role: "assistant", Content: "你好，我记住了。"}},
		AssistantOutput: "你好，我记住了。",
	}
	result := ThreadPersistResult{
		Handled:          true,
		Committed:        true,
		ExternalThreadID: "thread-1",
		Provider:         "nowledge",
	}

	if _, err := observer.AfterThreadPersist(context.Background(), rc, delta, result); err != nil {
		t.Fatalf("AfterThreadPersist: %v", err)
	}
}

func TestNowledgeDistillObserverRunsWhenEnabled(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")
	var triageCalled bool
	var distillCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/memories/distill/triage":
			triageCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"should_distill": true})
		case "/memories/distill":
			distillCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"memories_created": 1})
		default:
			t.Fatalf("unexpected nowledge request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	provider := nowledge.NewClient(nowledge.Config{BaseURL: srv.URL})
	observer := NewNowledgeDistillObserver(provider, nowledgeLinkStoreStub{}, nil)
	userID := uuid.New()
	rc := &RunContext{
		Run: data.Run{
			ID:        uuid.New(),
			AccountID: uuid.New(),
			ThreadID:  uuid.New(),
		},
		UserID: &userID,
	}
	delta := ThreadDelta{
		AccountID:       rc.Run.AccountID,
		ThreadID:        rc.Run.ThreadID,
		UserID:          userID,
		AgentID:         "user_" + userID.String(),
		Messages:        []ThreadDeltaMessage{{Role: "user", Content: "今天决定切到 nowledge"}, {Role: "assistant", Content: "我会记住这个决定。"}},
		AssistantOutput: "我会记住这个决定。",
	}
	result := ThreadPersistResult{
		Handled:          true,
		Committed:        true,
		ExternalThreadID: "thread-1",
		Provider:         "nowledge",
	}

	if _, err := observer.AfterThreadPersist(context.Background(), rc, delta, result); err != nil {
		t.Fatalf("AfterThreadPersist: %v", err)
	}
	if !triageCalled || !distillCalled {
		t.Fatalf("expected nowledge distill flow to run, triage=%v distill=%v", triageCalled, distillCalled)
	}
}

func TestBuildNowledgeThreadPayloadCarriesOpenClawStyleMetadata(t *testing.T) {
	delta := ThreadDelta{
		RunID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		ThreadID:        uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		TraceID:         "trace-1",
		Messages:        []ThreadDeltaMessage{{Role: "user", Content: "第一句"}, {Role: "assistant", Content: "第二句"}},
		AssistantOutput: "最终回复",
	}

	payload := buildNowledgeThreadPayload(delta)
	if len(payload) != 3 {
		t.Fatalf("unexpected payload length: %d", len(payload))
	}
	for index, message := range payload {
		if message.Metadata["source"] != nowledgeThreadSource {
			t.Fatalf("unexpected source at %d: %#v", index, message.Metadata)
		}
		if message.Metadata["session_key"] != delta.ThreadID.String() {
			t.Fatalf("unexpected session_key at %d: %#v", index, message.Metadata)
		}
		if message.Metadata["session_id"] != delta.RunID.String() {
			t.Fatalf("unexpected session_id at %d: %#v", index, message.Metadata)
		}
		if message.Metadata["trace_id"] != delta.TraceID {
			t.Fatalf("unexpected trace_id at %d: %#v", index, message.Metadata)
		}
		if externalID, _ := message.Metadata["external_id"].(string); externalID == "" {
			t.Fatalf("missing external_id at %d: %#v", index, message.Metadata)
		}
	}
}

func TestNowledgeContextContributorInjectsBehavioralGuidanceWithoutRecall(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/agent/working-memory":
			_ = json.NewEncoder(w).Encode(map[string]any{"exists": false, "content": ""})
		default:
			t.Fatalf("unexpected nowledge request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	provider := nowledge.NewClient(nowledge.Config{BaseURL: srv.URL})
	contributor := NewNowledgeContextContributor(provider)
	userID := uuid.New()
	rc := &RunContext{
		Run: data.Run{
			ID:        uuid.New(),
			AccountID: uuid.New(),
			ThreadID:  uuid.New(),
		},
		UserID: &userID,
		Messages: []llm.Message{
			{Role: "user", Content: []llm.ContentPart{{Type: "text", Text: "hi"}}},
		},
	}

	fragments, err := contributor.BeforePromptAssemble(context.Background(), rc)
	if err != nil {
		t.Fatalf("BeforePromptAssemble: %v", err)
	}
	for _, fragment := range fragments {
		if fragment.Key == nowledgeGuidanceTag {
			t.Fatalf("guidance should not be emitted as prompt fragment: %#v", fragment)
		}
	}
	segmentHook, ok := contributor.(BeforePromptSegmentsHook)
	if !ok {
		t.Fatal("expected before prompt segments hook")
	}
	segments, err := segmentHook.BeforePromptSegments(context.Background(), rc)
	if err != nil {
		t.Fatalf("BeforePromptSegments: %v", err)
	}
	guidance := ""
	for _, segment := range segments {
		if segment.Name == "hook.before.nowledge.guidance" {
			guidance = segment.Text
			break
		}
	}
	if guidance == "" {
		t.Fatal("expected guidance segment")
	}
	if !strings.Contains(guidance, "memory_search") || !strings.Contains(guidance, "memory_connections") || !strings.Contains(guidance, "memory_timeline") {
		t.Fatalf("guidance missing tool references: %q", guidance)
	}
	if strings.Contains(guidance, "已注入") {
		t.Fatalf("guidance should not claim injected context: %q", guidance)
	}
}

func TestNowledgeContextContributorAdjustsBehavioralGuidanceWhenContextInjected(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/agent/working-memory":
			_ = json.NewEncoder(w).Encode(map[string]any{"exists": true, "content": "今天聚焦 memory 系统"})
		case "/memories/search":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memories": []map[string]any{{
					"id":               "mem-1",
					"title":            "最近决策",
					"content":          "统一走 SeaweedFS",
					"score":            0.91,
					"relevance_reason": "topic match",
				}},
			})
		case "/threads/search":
			_ = json.NewEncoder(w).Encode(map[string]any{"threads": []map[string]any{}})
		default:
			t.Fatalf("unexpected nowledge request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	provider := nowledge.NewClient(nowledge.Config{BaseURL: srv.URL})
	contributor := NewNowledgeContextContributor(provider)
	userID := uuid.New()
	rc := &RunContext{
		Run: data.Run{
			ID:        uuid.New(),
			AccountID: uuid.New(),
			ThreadID:  uuid.New(),
		},
		UserID: &userID,
		Messages: []llm.Message{
			{Role: "user", Content: []llm.ContentPart{{Type: "text", Text: "附件存储方案最后定了吗"}}},
		},
	}

	fragments, err := contributor.BeforePromptAssemble(context.Background(), rc)
	if err != nil {
		t.Fatalf("BeforePromptAssemble: %v", err)
	}
	keys := map[string]PromptFragment{}
	for _, fragment := range fragments {
		keys[fragment.Key] = fragment
	}
	if _, ok := keys["nowledge_working_memory"]; !ok {
		t.Fatal("expected working memory fragment")
	}
	if _, ok := keys["nowledge_recalled_memories"]; !ok {
		t.Fatal("expected recalled memories fragment")
	}
	if _, ok := keys[nowledgeGuidanceTag]; ok {
		t.Fatal("guidance should not be emitted as prompt fragment")
	}
	segmentHook, ok := contributor.(BeforePromptSegmentsHook)
	if !ok {
		t.Fatal("expected before prompt segments hook")
	}
	segments, err := segmentHook.BeforePromptSegments(context.Background(), rc)
	if err != nil {
		t.Fatalf("BeforePromptSegments: %v", err)
	}
	guidance := ""
	for _, segment := range segments {
		if segment.Name == "hook.before.nowledge.guidance" {
			guidance = segment.Text
			break
		}
	}
	if !strings.Contains(guidance, "已注入 Working Memory和相关记忆") {
		t.Fatalf("guidance should acknowledge injected context: %q", guidance)
	}
}

func TestNowledgeContextContributorKeepsWorkingMemoryWhenRecallFails(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/agent/working-memory":
			_ = json.NewEncoder(w).Encode(map[string]any{"exists": true, "content": "今天聚焦 memory 系统"})
		case "/memories/search":
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected nowledge request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	provider := nowledge.NewClient(nowledge.Config{BaseURL: srv.URL})
	contributor := NewNowledgeContextContributor(provider)
	userID := uuid.New()
	rc := &RunContext{
		Run: data.Run{
			ID:        uuid.New(),
			AccountID: uuid.New(),
			ThreadID:  uuid.New(),
		},
		UserID: &userID,
		Messages: []llm.Message{
			{Role: "user", Content: []llm.ContentPart{{Type: "text", Text: "附件存储方案最后定了吗"}}},
		},
	}

	fragments, err := contributor.BeforePromptAssemble(context.Background(), rc)
	if err != nil {
		t.Fatalf("BeforePromptAssemble: %v", err)
	}
	if len(fragments) != 1 || fragments[0].Key != "nowledge_working_memory" {
		t.Fatalf("unexpected fragments on recall failure: %#v", fragments)
	}
	segmentHook, ok := contributor.(BeforePromptSegmentsHook)
	if !ok {
		t.Fatal("expected before prompt segments hook")
	}
	segments, err := segmentHook.BeforePromptSegments(context.Background(), rc)
	if err != nil {
		t.Fatalf("BeforePromptSegments: %v", err)
	}
	foundGuidance := false
	for _, segment := range segments {
		if segment.Name == "hook.before.nowledge.guidance" {
			foundGuidance = true
			break
		}
	}
	if !foundGuidance {
		t.Fatal("expected guidance segment when recall fails")
	}
}

func TestNowledgeContextContributorBeforePromptSegmentsIncludesGuidanceSegment(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/agent/working-memory":
			_ = json.NewEncoder(w).Encode(map[string]any{"exists": false, "content": ""})
		default:
			t.Fatalf("unexpected nowledge request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	provider := nowledge.NewClient(nowledge.Config{BaseURL: srv.URL})
	contributor := NewNowledgeContextContributor(provider)
	userID := uuid.New()
	rc := &RunContext{
		Run: data.Run{
			ID:        uuid.New(),
			AccountID: uuid.New(),
			ThreadID:  uuid.New(),
		},
		UserID: &userID,
		Messages: []llm.Message{
			{Role: "user", Content: []llm.ContentPart{{Type: "text", Text: "hi"}}},
		},
	}

	segmentHook, ok := contributor.(BeforePromptSegmentsHook)
	if !ok {
		t.Fatal("expected before prompt segments hook")
	}
	segments, err := segmentHook.BeforePromptSegments(context.Background(), rc)
	if err != nil {
		t.Fatalf("BeforePromptSegments: %v", err)
	}
	found := false
	for _, segment := range segments {
		if segment.Name != "hook.before.nowledge.guidance" {
			continue
		}
		found = true
		if segment.Target != PromptTargetSystemPrefix || segment.Role != "system" {
			t.Fatalf("unexpected guidance segment placement: %#v", segment)
		}
		if !strings.Contains(segment.Text, "memory_search") {
			t.Fatalf("unexpected guidance text: %#v", segment)
		}
	}
	if !found {
		t.Fatal("expected guidance segment")
	}
}
