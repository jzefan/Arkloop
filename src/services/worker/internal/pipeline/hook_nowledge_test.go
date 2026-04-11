package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	sharedconfig "arkloop/services/shared/config"
	sharedoutbound "arkloop/services/shared/outboundurl"
	"arkloop/services/worker/internal/data"
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
