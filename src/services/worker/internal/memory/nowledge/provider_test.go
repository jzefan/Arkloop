package nowledge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sharedoutbound "arkloop/services/shared/outboundurl"
	"arkloop/services/worker/internal/memory"

	"github.com/google/uuid"
)

func TestProviderMapsNowledgeMemoryOperations(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/memories/search":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memories": []map[string]any{{
					"id":               "mem-1",
					"title":            "Preference",
					"content":          "Use Chinese responses",
					"score":            0.88,
					"relevance_reason": "topic match",
				}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/threads/search":
			_ = json.NewEncoder(w).Encode(map[string]any{"threads": []map[string]any{}})
		case r.Method == http.MethodGet && r.URL.Path == "/memories/mem-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":      "mem-1",
				"title":   "Preference",
				"content": "Use Chinese responses",
			})
		case r.Method == http.MethodPost && r.URL.Path == "/memories":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "mem-2"})
		case r.Method == http.MethodDelete && r.URL.Path == "/memories/mem-2":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	provider := NewProvider(Config{BaseURL: server.URL, APIKey: "secret"})
	if provider == nil {
		t.Fatal("expected provider")
	}

	ident := memory.MemoryIdentity{
		AccountID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		UserID:    uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		AgentID:   "agent_1",
	}

	hits, err := provider.Find(context.Background(), ident, "", "language", 3)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(hits) != 1 || hits[0].URI != "nowledge://memory/mem-1" {
		t.Fatalf("unexpected hits: %#v", hits)
	}

	content, err := provider.Content(context.Background(), ident, "nowledge://memory/mem-1", memory.MemoryLayerRead)
	if err != nil {
		t.Fatalf("Content: %v", err)
	}
	if content != "Preference\n\nUse Chinese responses" {
		t.Fatalf("unexpected content: %q", content)
	}

	writeProvider, ok := provider.(memory.DesktopLocalMemoryWriteURI)
	if !ok {
		t.Fatal("expected provider to expose WriteReturningURI")
	}
	uri, err := writeProvider.WriteReturningURI(context.Background(), ident, memory.MemoryScopeUser, memory.MemoryEntry{
		Content: "Preference\n\nUse Chinese responses",
	})
	if err != nil {
		t.Fatalf("WriteReturningURI: %v", err)
	}
	if uri != "nowledge://memory/mem-2" {
		t.Fatalf("unexpected uri: %q", uri)
	}

	if err := provider.Delete(context.Background(), ident, uri); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestMemoryIDFromURI(t *testing.T) {
	id, err := MemoryIDFromURI("nowledge://memory/mem-1")
	if err != nil {
		t.Fatalf("MemoryIDFromURI: %v", err)
	}
	if id != "mem-1" {
		t.Fatalf("unexpected id: %q", id)
	}
}

func TestProviderListFragmentsMapsNowledgeMemories(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/memories" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"memories": []map[string]any{
				{
					"id":         "mem-9",
					"title":      "",
					"content":    "这是一段较长的记忆内容，用于摘要回退。",
					"rating":     0.4,
					"confidence": 0,
					"time":       "2026-04-11T01:02:03Z",
					"label_ids":  []string{"profile"},
				},
			},
		})
	}))
	defer server.Close()

	provider := NewClient(Config{BaseURL: server.URL})
	fragments, err := provider.ListFragments(context.Background(), testIdent(), 3)
	if err != nil {
		t.Fatalf("ListFragments: %v", err)
	}
	if len(fragments) != 1 {
		t.Fatalf("expected one fragment, got %#v", fragments)
	}
	fragment := fragments[0]
	if fragment.URI != "nowledge://memory/mem-9" {
		t.Fatalf("unexpected uri: %q", fragment.URI)
	}
	if fragment.Abstract == "" || !strings.Contains(fragment.Abstract, "记忆内容") {
		t.Fatalf("expected abstract fallback from content, got %#v", fragment)
	}
	if fragment.Score != 0.4 {
		t.Fatalf("expected rating fallback, got %#v", fragment)
	}
	if len(fragment.Labels) != 1 || fragment.Labels[0] != "profile" {
		t.Fatalf("unexpected labels: %#v", fragment.Labels)
	}
}
