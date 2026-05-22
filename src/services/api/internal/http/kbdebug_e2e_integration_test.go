//go:build !desktop

package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/kbingest"
	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"
	"arkloop/services/shared/embedding"
)

func TestKBDebugHTTPPipelineE2EWithFakeArk(t *testing.T) {
	db := testutil.SetupPostgresDatabase(t, "kbdebug_http_e2e")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := migrate.Up(ctx, db.DSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := data.NewPool(ctx, db.DSN, data.PoolLimits{MaxConns: 4})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	repo, err := data.NewKBChunksRepository(pool)
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	ark := newFakeEmbeddingArk(t, repo.Dim())
	doubao := embedding.NewDoubao(embedding.DoubaoConfig{
		BaseURL:    ark.URL,
		APIKey:     "test-key",
		Model:      "fake-endpoint",
		BatchSize:  16,
		MaxRetries: 1,
		Dim:        repo.Dim(),
	})
	svc, err := kbingest.New(doubao, repo)
	if err != nil {
		t.Fatalf("kbingest: %v", err)
	}

	handler := NewHandler(HandlerConfig{
		KBIngestService: svc,
		KBDebugToken:    "secret",
	})
	fixture := findBookFixture(t)

	ingestBody := strings.NewReader(`{"file_path":"` + fixture + `","kb_name":"e2e"}`)
	ingestReq := httptest.NewRequest(http.MethodPost, "/v1/_debug/kb/ingest", ingestBody)
	ingestReq.Header.Set("Authorization", "Bearer secret")
	ingestReq.Header.Set("Content-Type", "application/json")
	ingestResp := httptest.NewRecorder()
	handler.ServeHTTP(ingestResp, ingestReq)
	if ingestResp.Code != http.StatusOK {
		t.Fatalf("ingest status=%d body=%s", ingestResp.Code, ingestResp.Body.String())
	}
	var ingested struct {
		ChunkCount int `json:"chunk_count"`
	}
	if err := json.NewDecoder(ingestResp.Body).Decode(&ingested); err != nil {
		t.Fatal(err)
	}
	if ingested.ChunkCount == 0 {
		t.Fatal("ingest returned 0 chunks")
	}

	searchReq := httptest.NewRequest(http.MethodPost, "/v1/_debug/kb/search", strings.NewReader(`{"kb_name":"e2e","query":"光的干涉","k":3}`))
	searchReq.Header.Set("Authorization", "Bearer secret")
	searchReq.Header.Set("Content-Type", "application/json")
	searchResp := httptest.NewRecorder()
	handler.ServeHTTP(searchResp, searchReq)
	if searchResp.Code != http.StatusOK {
		t.Fatalf("search status=%d body=%s", searchResp.Code, searchResp.Body.String())
	}
	var searched struct {
		Hits []struct {
			DocumentRef string  `json:"document_ref"`
			Text        string  `json:"text"`
			Score       float32 `json:"score"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(searchResp.Body).Decode(&searched); err != nil {
		t.Fatal(err)
	}
	if len(searched.Hits) == 0 {
		t.Fatal("search returned 0 hits")
	}
	if searched.Hits[0].DocumentRef != filepath.Base(fixture) {
		t.Fatalf("document_ref=%q, want %q", searched.Hits[0].DocumentRef, filepath.Base(fixture))
	}
	if !strings.Contains(searched.Hits[0].Text, "光") {
		t.Fatalf("top hit does not contain 光: %q", searched.Hits[0].Text)
	}
}

func newFakeEmbeddingArk(t *testing.T, dim int) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		type embeddingRow struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}
		rows := make([]embeddingRow, len(req.Input))
		for i, text := range req.Input {
			vec := make([]float32, dim)
			if strings.Contains(text, "光") || strings.Contains(text, "干涉") {
				vec[0] = 1
			} else {
				vec[1] = 1
			}
			rows[i] = embeddingRow{Index: i, Embedding: vec}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "fake-endpoint",
			"data":  rows,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func findBookFixture(t *testing.T) string {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		candidate := filepath.Join(wd, "src", "services", "shared", "bookchunker", "testdata", "cn_textbook_excerpt.txt")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		next := filepath.Dir(wd)
		if next == wd {
			break
		}
		wd = next
	}
	t.Fatal("could not locate cn_textbook_excerpt.txt fixture")
	return ""
}
