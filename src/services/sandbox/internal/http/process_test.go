package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	processsvc "arkloop/services/sandbox/internal/process"
)

type stubProcessService struct {
	execReq        *processsvc.ExecCommandRequest
	continueReq    *processsvc.ContinueProcessRequest
	terminateReq   *processsvc.TerminateProcessRequest
	resizeReq      *processsvc.ResizeProcessRequest
	execResp       *processsvc.Response
	continueResp   *processsvc.Response
	terminateResp  *processsvc.Response
	resizeResp     *processsvc.Response
	resizeErr      error
	closeSessionID string
	closeAccountID string
}

func (s *stubProcessService) ExecCommand(_ context.Context, req processsvc.ExecCommandRequest) (*processsvc.Response, error) {
	copied := req
	s.execReq = &copied
	if s.execResp != nil {
		return s.execResp, nil
	}
	return &processsvc.Response{Status: processsvc.StatusRunning, ProcessRef: "proc_1", Cursor: "0", NextCursor: "1"}, nil
}

func (s *stubProcessService) ContinueProcess(_ context.Context, req processsvc.ContinueProcessRequest) (*processsvc.Response, error) {
	copied := req
	s.continueReq = &copied
	if s.continueResp != nil {
		return s.continueResp, nil
	}
	return &processsvc.Response{Status: processsvc.StatusExited, ProcessRef: req.ProcessRef, Cursor: req.Cursor, NextCursor: "2"}, nil
}

func (s *stubProcessService) TerminateProcess(_ context.Context, req processsvc.TerminateProcessRequest) (*processsvc.Response, error) {
	copied := req
	s.terminateReq = &copied
	if s.terminateResp != nil {
		return s.terminateResp, nil
	}
	return &processsvc.Response{Status: processsvc.StatusTerminated, ProcessRef: req.ProcessRef, Cursor: "0", NextCursor: "0"}, nil
}

func (s *stubProcessService) ResizeProcess(_ context.Context, req processsvc.ResizeProcessRequest) (*processsvc.Response, error) {
	copied := req
	s.resizeReq = &copied
	if s.resizeErr != nil {
		return nil, s.resizeErr
	}
	if s.resizeResp != nil {
		return s.resizeResp, nil
	}
	return &processsvc.Response{Status: processsvc.StatusRunning, ProcessRef: req.ProcessRef}, nil
}

func (s *stubProcessService) CloseSession(_ context.Context, sessionID, accountID string) error {
	s.closeSessionID = sessionID
	s.closeAccountID = accountID
	return nil
}

func TestProcessRoutesAcceptNewProtocol(t *testing.T) {
	svc := &stubProcessService{}
	handler := NewHandler(nil, nil, nil, nil, svc, nil, nil, newTestLogger(), "")

	execBody, _ := json.Marshal(map[string]any{
		"session_id": "run_1",
		"command":    "echo hi",
		"mode":       "follow",
		"timeout_ms": 5000,
	})
	execReq := httptest.NewRequest(http.MethodPost, "/v1/process/exec", bytes.NewReader(execBody))
	execRec := httptest.NewRecorder()
	handler.ServeHTTP(execRec, execReq)
	if execRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for exec, got %d", execRec.Code)
	}
	if svc.execReq == nil || svc.execReq.Mode != "follow" {
		t.Fatalf("unexpected exec request: %#v", svc.execReq)
	}

	continueBody, _ := json.Marshal(map[string]any{
		"session_id":  "run_1",
		"process_ref": "proc_1",
		"cursor":      "1",
		"wait_ms":     1200,
	})
	continueReq := httptest.NewRequest(http.MethodPost, "/v1/process/continue", bytes.NewReader(continueBody))
	continueRec := httptest.NewRecorder()
	handler.ServeHTTP(continueRec, continueReq)
	if continueRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for continue, got %d", continueRec.Code)
	}
	if svc.continueReq == nil || svc.continueReq.ProcessRef != "proc_1" || svc.continueReq.Cursor != "1" {
		t.Fatalf("unexpected continue request: %#v", svc.continueReq)
	}

	terminateBody, _ := json.Marshal(map[string]any{
		"session_id":  "run_1",
		"process_ref": "proc_1",
	})
	terminateReq := httptest.NewRequest(http.MethodPost, "/v1/process/terminate", bytes.NewReader(terminateBody))
	terminateRec := httptest.NewRecorder()
	handler.ServeHTTP(terminateRec, terminateReq)
	if terminateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for terminate, got %d", terminateRec.Code)
	}
	if svc.terminateReq == nil || svc.terminateReq.ProcessRef != "proc_1" {
		t.Fatalf("unexpected terminate request: %#v", svc.terminateReq)
	}

	resizeBody, _ := json.Marshal(map[string]any{
		"session_id":  "run_1",
		"process_ref": "proc_1",
		"rows":        50,
		"cols":        120,
	})
	resizeReq := httptest.NewRequest(http.MethodPost, "/v1/process/resize", bytes.NewReader(resizeBody))
	resizeRec := httptest.NewRecorder()
	handler.ServeHTTP(resizeRec, resizeReq)
	if resizeRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for resize, got %d", resizeRec.Code)
	}
	if svc.resizeReq == nil || svc.resizeReq.Rows != 50 || svc.resizeReq.Cols != 120 {
		t.Fatalf("unexpected resize request: %#v", svc.resizeReq)
	}
}

func TestProcessExecRouteRejectsInvalidMode(t *testing.T) {
	svc := &stubProcessService{}
	handler := NewHandler(nil, nil, nil, nil, svc, nil, nil, newTestLogger(), "")

	body, _ := json.Marshal(map[string]any{
		"session_id": "run_1",
		"command":    "echo hi",
		"mode":       "bogus",
		"timeout_ms": 5000,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/process/exec", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.execReq != nil {
		t.Fatalf("service should not be called for invalid mode: %#v", svc.execReq)
	}
}

func TestLegacyProcessExecCommandRouteNotFound(t *testing.T) {
	handler := NewHandler(nil, nil, nil, nil, &stubProcessService{}, nil, nil, newTestLogger(), "")

	req := httptest.NewRequest(http.MethodPost, "/v1/process/exec_command", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestProcessContinueRouteRejectsBadJSON(t *testing.T) {
	handler := handleProcessContinue(&stubProcessService{})
	req := httptest.NewRequest(http.MethodPost, "/v1/process/continue", bytes.NewBufferString("{"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
