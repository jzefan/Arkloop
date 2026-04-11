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

func testIdent() memory.MemoryIdentity {
	return memory.MemoryIdentity{
		AccountID: uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001"),
		UserID:    uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000002"),
		AgentID:   "nowledge-agent",
	}
}

func TestClientSearchRich(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/memories/search":
			if got := r.URL.Query().Get("query"); got != "deploy notes" {
				t.Fatalf("unexpected query: %q", got)
			}
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Fatalf("missing auth header")
			}
			if got := r.Header.Get("X-Arkloop-App"); got != "arkloop" {
				t.Fatalf("missing app header: %q", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memories": []map[string]any{
					{
						"id":               "mem-1",
						"title":            "Deploy decision",
						"content":          "Use blue/green",
						"score":            0.82,
						"labels":           []string{"decisions"},
						"relevance_reason": "keyword",
						"metadata": map[string]any{
							"source_thread_id": "thread-9",
						},
					},
				},
			})
		case "/threads/search":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"threads": []map[string]any{
					{
						"thread_id":     "thread-9",
						"title":         "Deploy chat",
						"source":        "arkloop",
						"message_count": 3,
						"score":         0.7,
						"snippets":      []string{"Use blue/green"},
					},
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, APIKey: "test-key"})
	results, err := c.SearchRich(context.Background(), testIdent(), "deploy notes", 3)
	if err != nil {
		t.Fatalf("SearchRich failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "mem-1" || results[0].Title != "Deploy decision" {
		t.Fatalf("unexpected result: %#v", results[0])
	}
	if results[0].SourceThreadID != "thread-9" {
		t.Fatalf("unexpected source thread id: %#v", results[0])
	}
	if len(results[0].RelatedThreads) != 1 || results[0].RelatedThreads[0].ThreadID != "thread-9" {
		t.Fatalf("unexpected related threads: %#v", results[0].RelatedThreads)
	}
}

func TestClientContentParsesNowledgeURI(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/memories/mem-9" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "mem-9",
			"title":   "Preference",
			"content": "Prefers concise updates",
		})
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	content, err := c.Content(context.Background(), testIdent(), "nowledge://memory/mem-9", memory.MemoryLayerOverview)
	if err != nil {
		t.Fatalf("Content failed: %v", err)
	}
	if !strings.Contains(content, "Preference") {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestClientMemoryDetailIncludesSourceThreadID(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/memories/mem-9" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "mem-9",
			"title":   "Preference",
			"content": "Prefers concise updates",
			"metadata": map[string]any{
				"source_thread_id": "thread-42",
			},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	detail, err := c.MemoryDetail(context.Background(), testIdent(), "nowledge://memory/mem-9")
	if err != nil {
		t.Fatalf("MemoryDetail failed: %v", err)
	}
	if detail.SourceThreadID != "thread-42" {
		t.Fatalf("unexpected detail: %#v", detail)
	}
}

func TestClientListMemories(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/memories" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("limit"); got != "7" {
			t.Fatalf("unexpected limit: %q", got)
		}
		sawAuth = r.Header.Get("Authorization") != ""
		_ = json.NewEncoder(w).Encode(map[string]any{
			"memories": []map[string]any{
				{
					"id":         "mem-3",
					"title":      "Deploy note",
					"content":    "Use canary rollout",
					"rating":     0.6,
					"confidence": 0.9,
					"time":       "2026-04-10T12:00:00Z",
					"label_ids":  []string{"ops", "deploy"},
					"source":     "arkloop",
				},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	memories, err := c.ListMemories(context.Background(), testIdent(), 7)
	if err != nil {
		t.Fatalf("ListMemories failed: %v", err)
	}
	if sawAuth {
		t.Fatal("did not expect auth header when api key is empty")
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}
	if memories[0].ID != "mem-3" || memories[0].Confidence != 0.9 {
		t.Fatalf("unexpected memory: %#v", memories[0])
	}
}

func TestClientListMemoriesReturnsEmptySlice(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"memories": []map[string]any{}})
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL, APIKey: "test-key"})
	memories, err := c.ListMemories(context.Background(), testIdent(), 5)
	if err != nil {
		t.Fatalf("ListMemories failed: %v", err)
	}
	if len(memories) != 0 {
		t.Fatalf("expected empty memories, got %#v", memories)
	}
}

func TestClientWriteParsesWritableEnvelope(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/memories" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "mem-2"})
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	err := c.Write(context.Background(), testIdent(), memory.MemoryScopeUser, memory.MemoryEntry{
		Content: "[user/preferences/summary] prefers short summaries",
	})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if got["title"] != "summary" {
		t.Fatalf("unexpected title: %#v", got["title"])
	}
	if got["unit_type"] != "preference" {
		t.Fatalf("unexpected unit type: %#v", got["unit_type"])
	}
}

func TestClientReadWorkingMemory(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent/working-memory" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"exists":  true,
			"content": "Today: finish hook integration",
		})
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	wm, err := c.ReadWorkingMemory(context.Background(), testIdent())
	if err != nil {
		t.Fatalf("ReadWorkingMemory failed: %v", err)
	}
	if !wm.Available || !strings.Contains(wm.Content, "hook integration") {
		t.Fatalf("unexpected wm: %#v", wm)
	}
}

func TestClientThreadEndpoints(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")
	var createBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/threads":
			if err := json.NewDecoder(r.Body).Decode(&createBody); err != nil {
				t.Fatalf("decode create body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "thread-created"})
		case r.Method == http.MethodPost && r.URL.Path == "/threads/thread-created/append":
			_ = json.NewEncoder(w).Encode(map[string]any{"messages_added": 2})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/threads/search"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"threads": []map[string]any{{
					"id":            "thread-created",
					"title":         "Deploy chat",
					"message_count": 4,
					"snippets":      []string{"Use blue green"},
				}},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/threads/thread-created":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"thread_id":     "thread-created",
				"title":         "Deploy chat",
				"message_count": 4,
				"messages": []map[string]any{
					{"role": "user", "content": "Deploy it", "timestamp": "2026-01-01T00:00:00Z"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	threadID, err := c.CreateThread(context.Background(), testIdent(), "ark-thread", "Deploy chat", "arkloop", []ThreadMessage{{Role: "user", Content: "Deploy it"}})
	if err != nil || threadID != "thread-created" {
		t.Fatalf("CreateThread failed: %v %q", err, threadID)
	}
	if got := createBody["source"]; got != "arkloop" {
		t.Fatalf("unexpected source: %#v", got)
	}
	appended, err := c.AppendThread(context.Background(), testIdent(), threadID, []ThreadMessage{{Role: "assistant", Content: "Done"}}, "idem-1")
	if err != nil || appended != 2 {
		t.Fatalf("AppendThread failed: %v %d", err, appended)
	}
	searchResults, err := c.SearchThreads(context.Background(), testIdent(), "deploy", 3)
	if err != nil {
		t.Fatalf("SearchThreads failed: %v %#v", err, searchResults)
	}
	if got := len(searchResults["threads"].([]map[string]any)); got != 1 {
		t.Fatalf("unexpected search results: %#v", searchResults)
	}
	fetched, err := c.FetchThread(context.Background(), testIdent(), threadID, 0, 20)
	if err != nil {
		t.Fatalf("FetchThread failed: %v %#v", err, fetched)
	}
	if got := len(fetched["messages"].([]map[string]any)); got != 1 {
		t.Fatalf("unexpected fetched thread: %#v", fetched)
	}
}

func TestBuildThreadMessageMetadata(t *testing.T) {
	metadata := BuildThreadMessageMetadata("arkloop", "session-1", "run-1", "thread-1", "user", "hello world", 0, "trace-1")
	if metadata["source"] != "arkloop" {
		t.Fatalf("unexpected source: %#v", metadata["source"])
	}
	if metadata["session_key"] != "session-1" {
		t.Fatalf("unexpected session key: %#v", metadata["session_key"])
	}
	if metadata["session_id"] != "run-1" {
		t.Fatalf("unexpected session id: %#v", metadata["session_id"])
	}
	if metadata["trace_id"] != "trace-1" {
		t.Fatalf("unexpected trace id: %#v", metadata["trace_id"])
	}
	if got, _ := metadata["external_id"].(string); !strings.HasPrefix(got, "arkloop_") || got == "" {
		t.Fatalf("unexpected external id: %#v", metadata["external_id"])
	}
}

func TestThreadMessageJSONUsesLowercaseKeys(t *testing.T) {
	payload, err := json.Marshal(ThreadMessage{
		Role:    "user",
		Content: "hello",
		Metadata: map[string]any{
			"source": "arkloop",
		},
	})
	if err != nil {
		t.Fatalf("marshal thread message: %v", err)
	}
	text := string(payload)
	if !strings.Contains(text, `"role":"user"`) || !strings.Contains(text, `"content":"hello"`) {
		t.Fatalf("unexpected payload: %s", text)
	}
	if strings.Contains(text, `"Role"`) || strings.Contains(text, `"Content"`) {
		t.Fatalf("expected lowercase keys, got: %s", text)
	}
}

func TestClientDistillEndpoints(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/memories/distill/triage":
			_ = json.NewEncoder(w).Encode(map[string]any{"should_distill": true, "reason": "contains decisions"})
		case "/memories/distill":
			_ = json.NewEncoder(w).Encode(map[string]any{"memories_created": 3})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	triage, err := c.TriageConversation(context.Background(), testIdent(), "user: choose postgres\nassistant: use pgx")
	if err != nil || !triage.ShouldDistill {
		t.Fatalf("TriageConversation failed: %v %#v", err, triage)
	}
	distill, err := c.DistillThread(context.Background(), testIdent(), "thread-1", "Decision", "content")
	if err != nil || distill.MemoriesCreated != 3 {
		t.Fatalf("DistillThread failed: %v %#v", err, distill)
	}
}

func TestClientConnections(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graph/expand/mem-1" {
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"neighbors": []map[string]any{
				{"id": "node-1", "label": "SeaweedFS", "node_type": "entity", "description": "storage backend"},
			},
			"edges": []map[string]any{
				{"source": "mem-1", "target": "node-1", "edge_type": "MENTIONS", "weight": 0.8},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	connections, err := c.Connections(context.Background(), testIdent(), "mem-1", 1, 20)
	if err != nil {
		t.Fatalf("Connections failed: %v", err)
	}
	if len(connections) != 1 || connections[0].NodeID != "node-1" || connections[0].EdgeType != "MENTIONS" {
		t.Fatalf("unexpected connections: %#v", connections)
	}
}

func TestClientTimeline(t *testing.T) {
	t.Setenv(sharedoutbound.AllowLoopbackHTTPEnv, "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent/feed/events" {
			t.Fatalf("unexpected path: %s", r.URL.String())
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"events": []map[string]any{
				{
					"id":                 "evt-1",
					"event_type":         "memory_created",
					"title":              "Deploy decision",
					"created_at":         "2026-04-12T10:00:00Z",
					"memory_id":          "mem-1",
					"related_memory_ids": []string{"mem-2"},
				},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{BaseURL: srv.URL})
	events, err := c.Timeline(context.Background(), testIdent(), 7, "", "", "", true, 100)
	if err != nil {
		t.Fatalf("Timeline failed: %v", err)
	}
	if len(events) != 1 || events[0].Label != "Memory saved" || events[0].MemoryID != "mem-1" {
		t.Fatalf("unexpected events: %#v", events)
	}
}
