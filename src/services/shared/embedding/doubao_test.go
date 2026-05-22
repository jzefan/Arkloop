package embedding

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newFakeArk(t *testing.T, dim int, failuresBeforeSuccess int32) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	var failsLeft = failuresBeforeSuccess
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Path != "/embeddings" {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			http.Error(w, "no auth", http.StatusUnauthorized)
			return
		}
		if atomic.LoadInt32(&failsLeft) > 0 {
			atomic.AddInt32(&failsLeft, -1)
			http.Error(w, "transient", http.StatusServiceUnavailable)
			return
		}
		var body struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		resp := map[string]any{
			"model": "doubao-embedding-text-240715",
			"data":  []any{},
		}
		data := make([]any, len(body.Input))
		for i := range body.Input {
			vec := make([]float32, dim)
			vec[0] = float32(i + 1)
			data[i] = map[string]any{"index": i, "embedding": vec}
		}
		resp["data"] = data
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestDoubaoEmbedSingleBatch(t *testing.T) {
	srv, _ := newFakeArk(t, 1024, 0)
	emb := NewDoubao(DoubaoConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "doubao-embedding-text-240715",
		BatchSize: 32, MaxRetries: 0, Dim: 1024,
	})
	vecs, err := emb.Embed(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(vecs) != 3 {
		t.Fatalf("got %d vecs, want 3", len(vecs))
	}
	if len(vecs[0]) != 1024 {
		t.Fatalf("dim mismatch: %d", len(vecs[0]))
	}
}

func TestDoubaoEmbedBatchesWhenOverBatchSize(t *testing.T) {
	srv, calls := newFakeArk(t, 16, 0)
	emb := NewDoubao(DoubaoConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		BatchSize: 2, MaxRetries: 0, Dim: 16,
	})
	in := []string{"a", "b", "c", "d", "e"} // 5 inputs, BatchSize 2 => 3 calls
	vecs, err := emb.Embed(context.Background(), in)
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(vecs) != 5 {
		t.Fatalf("got %d vecs, want 5", len(vecs))
	}
	if got := atomic.LoadInt32(calls); got != 3 {
		t.Fatalf("expected 3 HTTP calls, got %d", got)
	}
}

func TestDoubaoEmbedRetriesOnTransient(t *testing.T) {
	srv, calls := newFakeArk(t, 8, 2) // 2 transient failures, then success
	emb := NewDoubao(DoubaoConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		BatchSize: 32, MaxRetries: 3, BaseBackoff: 1 * time.Millisecond, Dim: 8,
	})
	_, err := emb.Embed(context.Background(), []string{"a"})
	if err != nil {
		t.Fatalf("expected retry success, got %v", err)
	}
	if got := atomic.LoadInt32(calls); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestDoubaoEmbedReturnsErrAfterMaxRetries(t *testing.T) {
	srv, _ := newFakeArk(t, 8, 99) // always fail
	emb := NewDoubao(DoubaoConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		BatchSize: 32, MaxRetries: 2, BaseBackoff: 1 * time.Millisecond, Dim: 8,
	})
	_, err := emb.Embed(context.Background(), []string{"a"})
	if err == nil {
		t.Fatal("expected error after retry budget")
	}
	if !errors.Is(err, ErrUpstream) {
		t.Errorf("expected ErrUpstream, got %v", err)
	}
}

func TestDoubaoEmbedEmptyInputReturnsEmpty(t *testing.T) {
	emb := NewDoubao(DoubaoConfig{BaseURL: "http://unused", APIKey: "k", Model: "m", BatchSize: 32, Dim: 8})
	vecs, err := emb.Embed(context.Background(), nil)
	if err != nil || vecs != nil {
		t.Fatalf("empty input: got vecs=%v err=%v", vecs, err)
	}
}

func TestDoubaoRejectsDimMismatch(t *testing.T) {
	srv, _ := newFakeArk(t, 1024, 0) // server returns 1024
	emb := NewDoubao(DoubaoConfig{
		BaseURL: srv.URL, APIKey: "k", Model: "m",
		BatchSize: 32, MaxRetries: 0, Dim: 2560, // expect 2560
	})
	_, err := emb.Embed(context.Background(), []string{"a"})
	if err == nil {
		t.Fatal("expected dim-mismatch error")
	}
	if !errors.Is(err, ErrDimMismatch) {
		t.Errorf("expected ErrDimMismatch, got %v", err)
	}
}
