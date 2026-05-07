package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSMiddlewareAllowsDesktopPackagedOrigins(t *testing.T) {
	handler := corsMiddleware([]string{"http://localhost:19080"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	// file:// origins should be allowed
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "file://")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "file://" {
		t.Fatalf("expected origin %q to be allowed, got %q", "file://", got)
	}
}

func TestCORSMiddlewareRejectsNullOrigin(t *testing.T) {
	handler := corsMiddleware([]string{"http://localhost:19080"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "null")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected null origin to be rejected, got %q", got)
	}
}

func TestAuthMiddlewareAllowsHealthz(t *testing.T) {
	handler := authMiddleware("test-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected healthz without auth to return 200, got %d", rec.Code)
	}
}

func TestAuthMiddlewareRejectsWithoutToken(t *testing.T) {
	handler := authMiddleware("test-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/modules", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing auth to return 401, got %d", rec.Code)
	}
}

func TestBridgeHandlerAddsCORSHeadersToUnauthorizedResponse(t *testing.T) {
	handler := bridgeHandler("test-token", []string{"http://localhost:19080"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/modules", nil)
	req.Header.Set("Origin", "http://localhost:19080")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing auth to return 401, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:19080" {
		t.Fatalf("expected allowed origin header, got %q", got)
	}
}

func TestBridgeHandlerHandlesPreflightWithoutAuth(t *testing.T) {
	handler := bridgeHandler("test-token", []string{"http://localhost:19080"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("preflight should not reach next handler")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/v1/modules", nil)
	req.Header.Set("Origin", "http://localhost:19080")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected preflight to return 204, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:19080" {
		t.Fatalf("expected allowed origin header, got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization" {
		t.Fatalf("expected allowed headers, got %q", got)
	}
}

func TestAuthMiddlewareRejectsInvalidToken(t *testing.T) {
	handler := authMiddleware("test-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/modules", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid token to return 401, got %d", rec.Code)
	}
}

func TestAuthMiddlewareAcceptsValidToken(t *testing.T) {
	handler := authMiddleware("test-token", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/modules", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected valid token to return 200, got %d", rec.Code)
	}
}
