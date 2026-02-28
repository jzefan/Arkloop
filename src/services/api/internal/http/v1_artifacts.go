package http

import (
	nethttp "net/http"
	"strings"

	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/observability"
	"arkloop/services/shared/objectstore"
)

func artifactsEntry(
	authService *auth.Service,
	store *objectstore.Store,
) func(nethttp.ResponseWriter, *nethttp.Request) {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())

		if r.Method != nethttp.MethodGet {
			writeMethodNotAllowed(w, r)
			return
		}

		if store == nil {
			WriteError(w, nethttp.StatusServiceUnavailable, "artifacts.not_configured", "artifact storage not configured", traceID, nil)
			return
		}

		_, ok := authenticateUser(w, r, traceID, authService)
		if !ok {
			return
		}

		// 从 URL 路径中提取 key: /v1/artifacts/{session_id}/{filename}
		key := strings.TrimPrefix(r.URL.Path, "/v1/artifacts/")
		if key == "" || strings.Contains(key, "..") {
			WriteError(w, nethttp.StatusBadRequest, "artifacts.invalid_key", "invalid artifact key", traceID, nil)
			return
		}

		data, contentType, err := store.GetWithContentType(r.Context(), key)
		if err != nil {
			WriteError(w, nethttp.StatusNotFound, "artifacts.not_found", "artifact not found", traceID, nil)
			return
		}

		if contentType == "" {
			contentType = "application/octet-stream"
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.WriteHeader(nethttp.StatusOK)
		_, _ = w.Write(data)
	}
}
