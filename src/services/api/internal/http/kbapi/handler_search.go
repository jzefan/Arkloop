package kbapi

import (
	nethttp "net/http"
	"strconv"
	"strings"
)

func handleSearch(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, nethttp.StatusUnauthorized, "auth.unauthenticated", "unauthenticated")
			return
		}
		kb, ok := loadAuthorizedKB(w, r, h, a)
		if !ok {
			return
		}
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			writeErr(w, nethttp.StatusBadRequest, "kb.missing_query", "q query param is required")
			return
		}
		if h.embedder == nil {
			writeErr(w, nethttp.StatusServiceUnavailable, "kb.embedding_not_configured", "embedding provider is not configured")
			return
		}
		k := 8
		if v := r.URL.Query().Get("k"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
				k = n
			}
		}
		vecs, err := h.embedder.Embed(r.Context(), []string{q})
		if err != nil || len(vecs) != 1 {
			writeErr(w, nethttp.StatusInternalServerError, "internal.embed_failed", "could not embed query")
			return
		}
		hits, err := h.chunksRepo.Search(r.Context(), kb.ID, vecs[0], k)
		if err != nil {
			writeErr(w, nethttp.StatusInternalServerError, "internal.search_failed", "search failed")
			return
		}
		items := make([]map[string]any, 0, len(hits))
		for _, hit := range hits {
			items = append(items, map[string]any{
				"document_ref": hit.DocumentRef,
				"ordinal":      hit.Ordinal,
				"heading_path": hit.HeadingPath,
				"chunk_type":   hit.ChunkType,
				"text":         hit.Text,
				"score":        hit.Score,
				"metadata":     hit.Metadata,
			})
		}
		writeJSON(w, nethttp.StatusOK, map[string]any{"hits": items})
	}
}
