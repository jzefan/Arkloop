package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sharedconfig "arkloop/services/shared/config"
	sharedoutbound "arkloop/services/shared/outboundurl"
	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/memory"
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

type nowledgeSnapshotStoreCapture struct {
	block string
	hits  []data.MemoryHitCache
	done  chan struct{}
}

func (s *nowledgeSnapshotStoreCapture) Get(context.Context, uuid.UUID, uuid.UUID, string) (string, bool, error) {
	return "", false, nil
}

func (s *nowledgeSnapshotStoreCapture) UpsertWithHits(_ context.Context, _, _ uuid.UUID, _ string, block string, hits []data.MemoryHitCache) error {
	s.block = block
	s.hits = append([]data.MemoryHitCache(nil), hits...)
	if s.done != nil {
		select {
		case s.done <- struct{}{}:
		default:
		}
	}
	return nil
}

type nowledgeImpressionStoreStub struct {
	score      int
	resetCalls int
}

func (s *nowledgeImpressionStoreStub) Get(context.Context, uuid.UUID, uuid.UUID, string) (string, bool, error) {
	return "", false, nil
}

func (s *nowledgeImpressionStoreStub) Upsert(context.Context, uuid.UUID, uuid.UUID, string, string) error {
	return nil
}

func (s *nowledgeImpressionStoreStub) AddScore(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ string, delta int) (int, error) {
	s.score += delta
	return s.score, nil
}

func (s *nowledgeImpressionStoreStub) ResetScore(context.Context, uuid.UUID, uuid.UUID, string) error {
	s.score = 0
	s.resetCalls++
	return nil
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
	}, nil, nil, nil, nil)
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
	observer := NewNowledgeDistillObserver(provider, nowledgeLinkStoreStub{}, nil, nil, nil, nil, nil)
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

func TestNowledgeDistillObserverRefreshesSnapshotAndImpressionWithoutCreatedCount(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")

	prevWindow := snapshotRefreshWindow
	prevInterval := snapshotRefreshRetryInterval
	prevAttempts := snapshotRefreshMaxAttempts
	snapshotRefreshWindow = 200 * time.Millisecond
	snapshotRefreshRetryInterval = 10 * time.Millisecond
	snapshotRefreshMaxAttempts = 3
	defer func() {
		snapshotRefreshWindow = prevWindow
		snapshotRefreshRetryInterval = prevInterval
		snapshotRefreshMaxAttempts = prevAttempts
	}()

	var triageCalled bool
	var distillCalled bool
	var listCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/memories/distill/triage":
			triageCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"should_distill": true})
		case "/memories/distill":
			distillCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"memories_created": 0})
		case "/memories":
			listCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memories": []map[string]any{{
					"id":         "mem-1",
					"title":      "迁移决策",
					"content":    "团队决定切到 nowledge knowledge 链路。",
					"confidence": 0.91,
				}},
			})
		default:
			t.Fatalf("unexpected nowledge request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	provider := nowledge.NewClient(nowledge.Config{BaseURL: srv.URL})
	snapshotStore := &nowledgeSnapshotStoreCapture{done: make(chan struct{}, 1)}
	impressionStore := &nowledgeImpressionStoreStub{score: 2}
	refreshTriggered := make(chan struct{}, 1)
	observer := NewNowledgeDistillObserver(
		provider,
		nowledgeLinkStoreStub{},
		nowledgeResolverStub{values: map[string]string{"memory.impression_score_threshold": "3"}},
		snapshotStore,
		nil,
		impressionStore,
		func(context.Context, memory.MemoryIdentity, uuid.UUID, uuid.UUID) {
			select {
			case refreshTriggered <- struct{}{}:
			default:
			}
		},
	)

	userID := uuid.New()
	rc := &RunContext{
		Run: data.Run{
			ID:        uuid.New(),
			AccountID: uuid.New(),
			ThreadID:  uuid.New(),
		},
		UserID:  &userID,
		TraceID: "trace-nowledge-refresh",
	}
	delta := ThreadDelta{
		AccountID:       rc.Run.AccountID,
		ThreadID:        rc.Run.ThreadID,
		UserID:          userID,
		AgentID:         "user_" + userID.String(),
		Messages:        []ThreadDeltaMessage{{Role: "user", Content: "今天决定切到 nowledge knowledge"}, {Role: "assistant", Content: "我记住这个迁移决策。"}},
		AssistantOutput: "我记住这个迁移决策。",
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

	select {
	case <-snapshotStore.done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("timeout waiting for snapshot refresh")
	}

	if !triageCalled || !distillCalled || !listCalled {
		t.Fatalf("expected full nowledge flow, triage=%v distill=%v list=%v", triageCalled, distillCalled, listCalled)
	}
	if !strings.Contains(snapshotStore.block, "迁移决策") {
		t.Fatalf("expected snapshot block to include latest memory, got %q", snapshotStore.block)
	}
	if len(snapshotStore.hits) != 1 || snapshotStore.hits[0].URI != "nowledge://memory/mem-1" {
		t.Fatalf("unexpected snapshot hits: %#v", snapshotStore.hits)
	}
	if impressionStore.resetCalls != 1 {
		t.Fatalf("expected impression score reset once, got %d", impressionStore.resetCalls)
	}

	select {
	case <-refreshTriggered:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected impression refresh trigger")
	}
}

func TestLegacyMemoryDistillObserverRefreshesNowledgeSnapshotWithoutCreatedCount(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")

	prevWindow := snapshotRefreshWindow
	prevInterval := snapshotRefreshRetryInterval
	prevAttempts := snapshotRefreshMaxAttempts
	snapshotRefreshWindow = 200 * time.Millisecond
	snapshotRefreshRetryInterval = 10 * time.Millisecond
	snapshotRefreshMaxAttempts = 3
	defer func() {
		snapshotRefreshWindow = prevWindow
		snapshotRefreshRetryInterval = prevInterval
		snapshotRefreshMaxAttempts = prevAttempts
	}()

	var triageCalled bool
	var distillCalled bool
	var listCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/memories/distill/triage":
			triageCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"should_distill": true})
		case "/memories/distill":
			distillCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{"memories_created": 0})
		case "/memories":
			listCalled = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memories": []map[string]any{{
					"id":         "mem-legacy",
					"title":      "上一轮同步",
					"content":    "Nowledge 已经有可投影到 Settings 的记忆。",
					"confidence": 0.88,
				}},
			})
		default:
			t.Fatalf("unexpected nowledge request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	provider := nowledge.NewClient(nowledge.Config{BaseURL: srv.URL})
	snapshotStore := &nowledgeSnapshotStoreCapture{done: make(chan struct{}, 1)}
	userID := uuid.New()
	rc := &RunContext{
		Run: data.Run{
			ID:        uuid.New(),
			AccountID: uuid.New(),
			ThreadID:  uuid.New(),
		},
		UserID:         &userID,
		MemoryProvider: provider,
		TraceID:        "trace-nowledge-legacy-refresh",
	}
	delta := ThreadDelta{
		AccountID:       rc.Run.AccountID,
		ThreadID:        rc.Run.ThreadID,
		UserID:          userID,
		AgentID:         "user_" + userID.String(),
		Messages:        []ThreadDeltaMessage{{Role: "user", Content: "同步上一轮消息"}, {Role: "assistant", Content: "我会更新记忆。"}},
		AssistantOutput: "我会更新记忆。",
	}
	result := ThreadPersistResult{
		Handled:          true,
		Committed:        true,
		ExternalThreadID: "thread-legacy",
		Provider:         "nowledge",
	}
	observer := NewLegacyMemoryDistillObserver(snapshotStore, nil, nil, nil, nil)

	if _, err := observer.AfterThreadPersist(context.Background(), rc, delta, result); err != nil {
		t.Fatalf("AfterThreadPersist: %v", err)
	}

	select {
	case <-snapshotStore.done:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("timeout waiting for snapshot refresh")
	}

	if !triageCalled || !distillCalled || !listCalled {
		t.Fatalf("expected nowledge refresh flow, triage=%v distill=%v list=%v", triageCalled, distillCalled, listCalled)
	}
	if !strings.Contains(snapshotStore.block, "上一轮同步") {
		t.Fatalf("expected snapshot block to include listed memory, got %q", snapshotStore.block)
	}
	if len(snapshotStore.hits) != 1 || snapshotStore.hits[0].URI != "nowledge://memory/mem-legacy" {
		t.Fatalf("unexpected snapshot hits: %#v", snapshotStore.hits)
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

	segments, err := contributor.BeforePromptSegments(context.Background(), rc)
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

	segments, err := contributor.BeforePromptSegments(context.Background(), rc)
	if err != nil {
		t.Fatalf("BeforePromptSegments: %v", err)
	}
	keys := map[string]PromptSegment{}
	for _, segment := range segments {
		keys[segment.Name] = segment
	}
	if _, ok := keys["hook.before.nowledge.working_memory"]; !ok {
		t.Fatal("expected working memory segment")
	}
	if _, ok := keys["hook.before.nowledge.recalled_memories"]; !ok {
		t.Fatal("expected recalled memories segment")
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

	segments, err := contributor.BeforePromptSegments(context.Background(), rc)
	if err != nil {
		t.Fatalf("BeforePromptSegments: %v", err)
	}
	if len(segments) == 0 || segments[0].Name != "hook.before.nowledge.working_memory" {
		t.Fatalf("unexpected segments on recall failure: %#v", segments)
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
