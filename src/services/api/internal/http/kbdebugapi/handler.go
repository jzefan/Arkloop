package kbdebugapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"
)

// Ingester is implemented by composing bookchunker + embedding.Embedder +
// data.KBChunksRepository. Kept as an interface to keep handler tests small.
type Ingester interface {
	Ingest(ctx context.Context, filePath, kbName string) (chunkCount int, err error)
}

// Searcher is the read side of the same composition.
type Searcher interface {
	Search(ctx context.Context, kbName, query string, k int) ([]SearchHit, error)
}

// SearchHit is the JSON shape returned to callers.
type SearchHit struct {
	DocumentRef string  `json:"document_ref"`
	Ordinal     int     `json:"ordinal"`
	Text        string  `json:"text"`
	Score       float32 `json:"score"`
}

type ingestRequest struct {
	FilePath string `json:"file_path"`
	KBName   string `json:"kb_name"`
}

type ingestResponse struct {
	ChunkCount int   `json:"chunk_count"`
	DurationMS int64 `json:"duration_ms"`
}

type searchRequest struct {
	KBName string `json:"kb_name"`
	Query  string `json:"query"`
	K      int    `json:"k"`
}

type searchResponse struct {
	Hits []SearchHit `json:"hits"`
}

func newIngestHandler(ing Ingester) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ingestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.FilePath == "" || req.KBName == "" {
			http.Error(w, "file_path and kb_name required", http.StatusBadRequest)
			return
		}
		if _, err := os.Stat(req.FilePath); err != nil {
			http.Error(w, "file not accessible: "+err.Error(), http.StatusBadRequest)
			return
		}
		start := time.Now()
		count, err := ing.Ingest(r.Context(), req.FilePath, req.KBName)
		if err != nil {
			http.Error(w, "ingest failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ingestResponse{
			ChunkCount: count,
			DurationMS: time.Since(start).Milliseconds(),
		})
	})
}

func newSearchHandler(srch Searcher) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req searchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if req.KBName == "" || req.Query == "" {
			http.Error(w, "kb_name and query required", http.StatusBadRequest)
			return
		}
		if req.K <= 0 || req.K > 50 {
			req.K = 8
		}
		hits, err := srch.Search(r.Context(), req.KBName, req.Query, req.K)
		if err != nil {
			http.Error(w, "search failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(searchResponse{Hits: hits})
	})
}

// Register wires both endpoints onto the supplied mux behind the debug-token
// middleware.
func Register(mux *http.ServeMux, debugToken string, ing Ingester, srch Searcher) {
	guard := RequireDebugToken(debugToken)
	mux.Handle("POST /v1/_debug/kb/ingest", guard(newIngestHandler(ing)))
	mux.Handle("POST /v1/_debug/kb/search", guard(newSearchHandler(srch)))
}
