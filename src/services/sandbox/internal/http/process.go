package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	processapi "arkloop/services/sandbox/internal/process"
)

func handleProcessExec(svc processapi.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			writeError(w, http.StatusServiceUnavailable, processapi.CodeProcessNotFound, "process service not configured")
			return
		}
		var req processapi.ExecCommandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "sandbox.invalid_request", "invalid JSON body")
			return
		}
		req.SessionID = strings.TrimSpace(req.SessionID)
		req.AccountID = strings.TrimSpace(req.AccountID)
		req.ProfileRef = strings.TrimSpace(req.ProfileRef)
		req.WorkspaceRef = strings.TrimSpace(req.WorkspaceRef)
		req.Tier = strings.TrimSpace(req.Tier)
		req.Command = strings.TrimSpace(req.Command)
		req.Mode = strings.TrimSpace(req.Mode)
		req.Cwd = strings.TrimSpace(req.Cwd)
		if req.SessionID == "" {
			writeError(w, http.StatusBadRequest, "sandbox.missing_session_id", "session_id is required")
			return
		}
		if err := processapi.ValidateExecRequest(req); err != nil {
			writeProcessError(w, err)
			return
		}
		resp, err := svc.ExecCommand(r.Context(), req)
		if err != nil {
			writeProcessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func handleProcessContinue(svc processapi.Service) http.HandlerFunc {
	return handleProcessAction(svc, func(ctx context.Context, req processapi.ContinueProcessRequest) (*processapi.Response, error) {
		return svc.ContinueProcess(ctx, req)
	})
}

func handleProcessTerminate(svc processapi.Service) http.HandlerFunc {
	return handleProcessAction(svc, func(ctx context.Context, req processapi.TerminateProcessRequest) (*processapi.Response, error) {
		return svc.TerminateProcess(ctx, req)
	})
}

func handleProcessAction[T any](svc processapi.Service, fn func(context.Context, T) (*processapi.Response, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			writeError(w, http.StatusServiceUnavailable, processapi.CodeProcessNotFound, "process service not configured")
			return
		}
		var req T
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "sandbox.invalid_request", "invalid JSON body")
			return
		}
		switch typed := any(&req).(type) {
		case *processapi.ContinueProcessRequest:
			typed.SessionID = strings.TrimSpace(typed.SessionID)
			typed.AccountID = strings.TrimSpace(typed.AccountID)
			typed.ProcessRef = strings.TrimSpace(typed.ProcessRef)
			typed.Cursor = strings.TrimSpace(typed.Cursor)
			if err := processapi.ValidateContinueRequest(*typed); err != nil {
				writeProcessError(w, err)
				return
			}
		case *processapi.TerminateProcessRequest:
			typed.SessionID = strings.TrimSpace(typed.SessionID)
			typed.AccountID = strings.TrimSpace(typed.AccountID)
			typed.ProcessRef = strings.TrimSpace(typed.ProcessRef)
		}
		resp, err := fn(r.Context(), req)
		if err != nil {
			writeProcessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func handleProcessResize(svc processapi.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svc == nil {
			writeError(w, http.StatusServiceUnavailable, processapi.CodeProcessNotFound, "process service not configured")
			return
		}
		var req processapi.ResizeProcessRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "sandbox.invalid_request", "invalid JSON body")
			return
		}
		req.SessionID = strings.TrimSpace(req.SessionID)
		req.AccountID = strings.TrimSpace(req.AccountID)
		req.ProcessRef = strings.TrimSpace(req.ProcessRef)
		if err := processapi.ValidateResizeRequest(req); err != nil {
			writeProcessError(w, err)
			return
		}
		resp, err := svc.ResizeProcess(r.Context(), req)
		if err != nil {
			writeProcessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func writeProcessError(w http.ResponseWriter, err error) {
	if procErr, ok := err.(*processapi.Error); ok {
		status := procErr.HTTPStatus
		if status == 0 {
			status = http.StatusInternalServerError
		}
		writeError(w, status, procErr.Code, procErr.Message)
		return
	}
	writeError(w, http.StatusInternalServerError, "sandbox.process_error", err.Error())
}
