package proxy

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"arkloop/services/gateway/internal/clientip"
)

// Config holds the reverse proxy configuration.
type Config struct {
	Upstream string
}

// Proxy is a stateless reverse proxy for the API service.
type Proxy struct {
	handler http.Handler
}

// New creates a reverse proxy targeting the given upstream URL.
// FlushInterval=-1 enables immediate flushing, required for SSE streams.
//
// X-Forwarded-For is handled by httputil.ReverseProxy.ServeHTTP automatically
// from req.RemoteAddr. We clear any client-provided XFF in the Director to
// prevent spoofing — ServeHTTP will then set it fresh.
func New(cfg Config) (*Proxy, error) {
	target, err := url.Parse(cfg.Upstream)
	if err != nil || strings.TrimSpace(target.Host) == "" {
		return nil, fmt.Errorf("invalid upstream url: %q", cfg.Upstream)
	}

	rp := httputil.NewSingleHostReverseProxy(target)
	rp.FlushInterval = -1

	original := rp.Director
	rp.Director = func(req *http.Request) {
		originalHost := req.Host
		original(req)
		req.Header.Del("X-Forwarded-For")
		req.Header.Set("X-Forwarded-Host", originalHost)

		// 透传 clientip 中间件解析的真实 IP
		if realIP := clientip.FromContext(req.Context()); realIP != "" {
			req.Header.Set("X-Real-IP", realIP)
		}
	}

	return &Proxy{handler: rp}, nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.handler.ServeHTTP(w, r)
}
