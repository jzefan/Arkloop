package app

import (
	"net/http"
	"strings"
)

func routeRequest(apiProxy http.Handler, frontendProxy http.Handler) http.Handler {
	if frontendProxy == nil {
		// Even without a frontend, we must reject /internal/* before it
		// reaches the API: it is an internal-only surface authenticated by
		// a shared service token and must never be exposed publicly.
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isForbiddenPath(r.URL.Path) {
				http.NotFound(w, r)
				return
			}
			apiProxy.ServeHTTP(w, r)
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isForbiddenPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		if shouldProxyToAPI(r.URL.Path) {
			apiProxy.ServeHTTP(w, r)
			return
		}
		frontendProxy.ServeHTTP(w, r)
	})
}

// shouldProxyToAPI routes /v1/* and the OIDC discovery surfaces (/.well-known/*)
// to the API. The latter is required for any OIDC client (e.g. exam) to fetch
// the issuer metadata and JWKS — without this, signature verification fails
// before it even starts.
func shouldProxyToAPI(path string) bool {
	if path == "/v1" || strings.HasPrefix(path, "/v1/") {
		return true
	}
	if strings.HasPrefix(path, "/.well-known/") {
		return true
	}
	return false
}

// isForbiddenPath blocks anything that must never reach external traffic
// regardless of where it would otherwise be routed. /internal/* is the
// worker → api private surface (service-token authenticated); exposing it
// publicly turns a 32-byte shared secret into the only barrier between an
// attacker and impersonating any user.
func isForbiddenPath(path string) bool {
	return strings.HasPrefix(path, "/internal/")
}
