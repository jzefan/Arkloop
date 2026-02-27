package http

import (
	"net/http"
	"strings"

	"arkloop/services/sandbox/internal/logging"
	"arkloop/services/sandbox/internal/session"
)

func handleDeleteSession(mgr *session.Manager, logger *logging.JSONLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 路由: DELETE /v1/sessions/{id}
		id := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
		id = strings.TrimSpace(id)
		if id == "" {
			writeError(w, http.StatusBadRequest, "sandbox.missing_session_id", "session id is required")
			return
		}

		if err := mgr.Delete(r.Context(), id); err != nil {
			logger.Warn("delete session not found", logging.LogFields{SessionID: &id}, nil)
			writeError(w, http.StatusNotFound, "sandbox.session_not_found", err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
