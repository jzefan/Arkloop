//go:build !desktop

package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKBDebugRoutesFailClosedWhenTokenSetButServiceMissing(t *testing.T) {
	h := NewHandler(HandlerConfig{KBDebugToken: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/v1/_debug/kb/search", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s, want %d", w.Code, w.Body.String(), http.StatusUnauthorized)
	}
}
