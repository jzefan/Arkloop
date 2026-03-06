package http

import (
	"net/http"
	"strings"

	"arkloop/services/sandbox/internal/logging"
	"arkloop/services/sandbox/internal/session"
	"arkloop/services/sandbox/internal/shell"
)

func handleDeleteSession(mgr *session.Manager, shellSvc shell.Service, logger *logging.JSONLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 路由: DELETE /v1/sessions/{id}
		id := strings.TrimPrefix(r.URL.Path, "/v1/sessions/")
		id = strings.TrimSpace(id)
		if id == "" {
			writeError(w, http.StatusBadRequest, "sandbox.missing_session_id", "session id is required")
			return
		}

		orgID := strings.TrimSpace(r.Header.Get("X-Org-ID"))
		if shellSvc != nil {
			if err := shellSvc.Close(r.Context(), id, orgID); err == nil {
				w.WriteHeader(http.StatusNoContent)
				return
			} else if shellErr, ok := err.(*shell.Error); ok && shellErr.Code == shell.CodeOrgMismatch {
				writeError(w, http.StatusForbidden, shellErr.Code, shellErr.Message)
				return
			}
		}

		if err := mgr.Delete(r.Context(), id, orgID); err != nil {
			if strings.Contains(err.Error(), "org mismatch") {
				writeError(w, http.StatusForbidden, "sandbox.org_mismatch", "session belongs to another org")
				return
			}
			logger.Warn("delete session not found", logging.LogFields{SessionID: &id}, nil)
			writeError(w, http.StatusNotFound, "sandbox.session_not_found", err.Error())
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
