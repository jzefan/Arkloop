package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"arkloop/services/sandbox/internal/logging"
	"arkloop/services/sandbox/internal/shell"
)

func handleShellOpen(svc shell.Service, _ *logging.JSONLogger) http.HandlerFunc {
	return handleShellAction(svc, func(ctx context.Context, req shell.Request, svc shell.Service) (*shell.Response, error) {
		return svc.Open(ctx, req)
	})
}

func handleShellExec(svc shell.Service, _ *logging.JSONLogger) http.HandlerFunc {
	return handleShellAction(svc, func(ctx context.Context, req shell.Request, svc shell.Service) (*shell.Response, error) {
		return svc.Exec(ctx, req)
	})
}

func handleShellRead(svc shell.Service, _ *logging.JSONLogger) http.HandlerFunc {
	return handleShellAction(svc, func(ctx context.Context, req shell.Request, svc shell.Service) (*shell.Response, error) {
		return svc.Read(ctx, req)
	})
}

func handleShellWrite(svc shell.Service, _ *logging.JSONLogger) http.HandlerFunc {
	return handleShellAction(svc, func(ctx context.Context, req shell.Request, svc shell.Service) (*shell.Response, error) {
		return svc.Write(ctx, req)
	})
}

func handleShellSignal(svc shell.Service, _ *logging.JSONLogger) http.HandlerFunc {
	return handleShellAction(svc, func(ctx context.Context, req shell.Request, svc shell.Service) (*shell.Response, error) {
		return svc.Signal(ctx, req)
	})
}

func handleShellClose(svc shell.Service, _ *logging.JSONLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			writeError(w, http.StatusServiceUnavailable, shell.CodeSessionNotFound, "shell service not configured")
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/v1/shell/session/")
		id = strings.TrimSpace(id)
		if id == "" {
			writeError(w, http.StatusBadRequest, "sandbox.missing_session_id", "session id is required")
			return
		}
		orgID := strings.TrimSpace(r.Header.Get("X-Org-ID"))
		if err := svc.Close(r.Context(), id, orgID); err != nil {
			writeShellError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type shellActionFunc func(ctx context.Context, req shell.Request, svc shell.Service) (*shell.Response, error)

func handleShellAction(svc shell.Service, fn shellActionFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			writeError(w, http.StatusServiceUnavailable, shell.CodeSessionNotFound, "shell service not configured")
			return
		}

		var req shell.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "sandbox.invalid_request", "invalid JSON body")
			return
		}

		req.SessionID = strings.TrimSpace(req.SessionID)
		req.OrgID = strings.TrimSpace(req.OrgID)
		req.Tier = strings.TrimSpace(req.Tier)
		req.Cwd = strings.TrimSpace(req.Cwd)
		req.Signal = strings.TrimSpace(req.Signal)
		if req.SessionID == "" {
			writeError(w, http.StatusBadRequest, "sandbox.missing_session_id", "session_id is required")
			return
		}
		if req.Tier == "" {
			req.Tier = "lite"
		}
		if err := shell.ValidateTimeoutMs(req.TimeoutMs); err != nil {
			writeShellError(w, err)
			return
		}

		resp, err := fn(r.Context(), req, svc)
		if err != nil {
			writeShellError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func writeShellError(w http.ResponseWriter, err error) {
	if shellErr, ok := err.(*shell.Error); ok {
		status := shellErr.HTTPStatus
		if status == 0 {
			status = http.StatusInternalServerError
		}
		writeError(w, status, shellErr.Code, shellErr.Message)
		return
	}
	writeError(w, http.StatusInternalServerError, "sandbox.shell_error", err.Error())
}
