package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewClient(server.URL, "test-token")
}

func TestGetMe(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"u1","username":"qq","account_id":"a1","work_enabled":true}`))
	})

	got, err := client.GetMe(context.Background())
	if err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if got.ID != "u1" || got.Username != "qq" || got.AccountID != "a1" || !got.WorkEnabled {
		t.Fatalf("unexpected me: %#v", got)
	}
}

func TestListSelectablePersonas(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/me/selectable-personas" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"p1","persona_key":"search","display_name":"Search","selector_name":"Search","model":"gpt-4.1","reasoning_mode":"enabled","source":"builtin"}]`))
	})

	got, err := client.ListSelectablePersonas(context.Background())
	if err != nil {
		t.Fatalf("ListSelectablePersonas: %v", err)
	}
	if len(got) != 1 || got[0].PersonaKey != "search" {
		t.Fatalf("unexpected personas: %#v", got)
	}
}

func TestListLlmProviders(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/v1/llm-providers?scope=user" {
			t.Fatalf("unexpected uri: %s", r.URL.RequestURI())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"provider-1","name":"OpenAI","models":[{"id":"m1","provider_id":"provider-1","model":"gpt-4.1","is_default":true,"show_in_picker":true,"tags":["chat"]}]}]`))
	})

	got, err := client.ListLlmProviders(context.Background())
	if err != nil {
		t.Fatalf("ListLlmProviders: %v", err)
	}
	if len(got) != 1 || got[0].Name != "OpenAI" || len(got[0].Models) != 1 {
		t.Fatalf("unexpected providers: %#v", got)
	}
}

func TestListThreads(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/v1/threads?limit=200" {
			t.Fatalf("unexpected uri: %s", r.URL.RequestURI())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"t1","mode":"chat","title":"Hello","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z","active_run_id":"r1","is_private":false}]`))
	})

	got, err := client.ListThreads(context.Background(), 200)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(got) != 1 || got[0].ID != "t1" || got[0].UpdatedAt != "2026-01-02T00:00:00Z" {
		t.Fatalf("unexpected threads: %#v", got)
	}
}

func TestGetMeReturnsHTTPError(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"code":"auth.invalid_token","message":"token invalid"}`, http.StatusUnauthorized)
	})

	if _, err := client.GetMe(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}
