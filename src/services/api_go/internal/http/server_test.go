package http

import (
	"encoding/json"
	"io"
	"regexp"
	"testing"

	nethttp "net/http"
	"net/http/httptest"

	"arkloop/services/api_go/internal/observability"
)

func TestHealthz(t *testing.T) {
	logger := observability.NewJSONLogger("test", io.Discard)
	handler := NewHandler(HandlerConfig{Logger: logger})

	req := httptest.NewRequest(nethttp.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != nethttp.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}

	traceID := recorder.Header().Get(observability.TraceIDHeader)
	if traceID == "" {
		t.Fatalf("missing %s header", observability.TraceIDHeader)
	}

	var payload map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["status"] != "ok" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestNotFoundReturnsEnvelope(t *testing.T) {
	logger := observability.NewJSONLogger("test", io.Discard)
	handler := NewHandler(HandlerConfig{Logger: logger})

	req := httptest.NewRequest(nethttp.MethodGet, "/nope", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != nethttp.StatusNotFound {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}

	traceID := recorder.Header().Get(observability.TraceIDHeader)
	if traceID == "" {
		t.Fatalf("missing %s header", observability.TraceIDHeader)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(traceID) {
		t.Fatalf("invalid trace id: %q", traceID)
	}

	var payload ErrorEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.TraceID != traceID {
		t.Fatalf("trace_id mismatch: header=%q payload=%q", traceID, payload.TraceID)
	}
	if payload.Code != "http_error" {
		t.Fatalf("unexpected code: %q", payload.Code)
	}
	if payload.Message == "" {
		t.Fatalf("missing message")
	}
}

func TestReadyzRequiresDatabase(t *testing.T) {
	logger := observability.NewJSONLogger("test", io.Discard)
	handler := NewHandler(HandlerConfig{Logger: logger})

	req := httptest.NewRequest(nethttp.MethodGet, "/readyz", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != nethttp.StatusServiceUnavailable {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}

	traceID := recorder.Header().Get(observability.TraceIDHeader)
	if traceID == "" {
		t.Fatalf("missing %s header", observability.TraceIDHeader)
	}

	var payload ErrorEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != "not_ready" {
		t.Fatalf("unexpected code: %q", payload.Code)
	}
	if payload.TraceID != traceID {
		t.Fatalf("trace_id mismatch: header=%q payload=%q", traceID, payload.TraceID)
	}
}
