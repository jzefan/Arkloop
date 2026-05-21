// Package kbdebugapi hosts /v1/_debug/kb/* endpoints used only during M0
// to verify the chunker -> embedder -> pgvector pipeline. These routes
// require a static Bearer token from env ARKLOOP_DEBUG_TOKEN.
//
// M1 retires this package and replaces it with kbapi behind workspace auth.
package kbdebugapi

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// RequireDebugToken returns middleware that enforces Authorization: Bearer <token>
// with a constant-time compare. An empty configured token disables the route
// entirely so misconfiguration fails closed.
func RequireDebugToken(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if expected == "" {
				http.Error(w, "kb debug routes disabled", http.StatusUnauthorized)
				return
			}
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				http.Error(w, "bearer token required", http.StatusUnauthorized)
				return
			}
			provided := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
				http.Error(w, "invalid debug token", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
