package kbdebugapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeIngester struct {
	called     bool
	lastPath   string
	lastKBName string
	chunkCount int
}

func (f *fakeIngester) Ingest(ctx context.Context, filePath, kbName string) (int, error) {
	f.called = true
	f.lastPath = filePath
	f.lastKBName = kbName
	return f.chunkCount, nil
}

type fakeSearcher struct {
	hits []SearchHit
}

func (f *fakeSearcher) Search(ctx context.Context, kbName, query string, k int) ([]SearchHit, error) {
	return f.hits, nil
}

func TestIngestHandlerHappyPath(t *testing.T) {
	tmp := t.TempDir()
	textFile := filepath.Join(tmp, "in.txt")
	if err := os.WriteFile(textFile, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	ing := &fakeIngester{chunkCount: 3}
	h := newIngestHandler(ing)
	body := strings.NewReader(`{"file_path":"` + textFile + `","kb_name":"k"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/_debug/kb/ingest", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		ChunkCount int `json:"chunk_count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.ChunkCount != 3 {
		t.Errorf("chunk_count: got %d, want 3", resp.ChunkCount)
	}
	if !ing.called || ing.lastKBName != "k" || ing.lastPath != textFile {
		t.Errorf("ingester not invoked correctly: %+v", ing)
	}
}

func TestIngestHandlerRejectsMissingFields(t *testing.T) {
	h := newIngestHandler(&fakeIngester{})
	req := httptest.NewRequest(http.MethodPost, "/v1/_debug/kb/ingest", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSearchHandlerHappyPath(t *testing.T) {
	srch := &fakeSearcher{hits: []SearchHit{{DocumentRef: "d", Ordinal: 0, Text: "光的干涉...", Score: 0.97}}}
	h := newSearchHandler(srch)
	body := strings.NewReader(`{"kb_name":"k","query":"光","k":3}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/_debug/kb/search", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Hits []SearchHit `json:"hits"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Hits) != 1 || resp.Hits[0].DocumentRef != "d" {
		t.Errorf("unexpected hits: %+v", resp.Hits)
	}
}
