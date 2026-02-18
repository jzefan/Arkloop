package http

import (
	nethttp "net/http"

	"arkloop/services/api_go/internal/data"
	"arkloop/services/api_go/internal/observability"
)

type HandlerConfig struct {
	Logger               *observability.JSONLogger
	SchemaRepository     *data.SchemaRepository
	TrustIncomingTraceID bool
}

func NewHandler(cfg HandlerConfig) nethttp.Handler {
	mux := nethttp.NewServeMux()
	mux.HandleFunc("/healthz", healthz)
	mux.HandleFunc("/readyz", readyz(cfg.SchemaRepository, cfg.Logger))

	notFound := nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := observability.TraceIDFromContext(r.Context())
		WriteError(w, nethttp.StatusNotFound, "http_error", "Not Found", traceID, nil)
	})

	base := nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		handler, pattern := mux.Handler(r)
		if pattern == "" {
			notFound.ServeHTTP(w, r)
			return
		}
		handler.ServeHTTP(w, r)
	})

	handler := RecoverMiddleware(base, cfg.Logger)
	handler = TraceMiddleware(handler, cfg.Logger, cfg.TrustIncomingTraceID)
	return handler
}
