package http

import (
	"fmt"
	"runtime/debug"
	"time"

	nethttp "net/http"

	"arkloop/services/api/internal/observability"
)

type statusRecorder struct {
	nethttp.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	if r.wroteHeader {
		return
	}
	r.wroteHeader = true
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *statusRecorder) Write(payload []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(nethttp.StatusOK)
	}
	return r.ResponseWriter.Write(payload)
}

// Flush forwards to the underlying ResponseWriter; required for SSE/streaming.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(nethttp.Flusher); ok {
		f.Flush()
	}
}

func TraceMiddleware(next nethttp.Handler, logger *observability.JSONLogger, trustIncomingTraceID bool) nethttp.Handler {
	if next == nil {
		return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
			WriteError(w, nethttp.StatusInternalServerError, "internal_error", "internal error", "", nil)
		})
	}

	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		start := time.Now()

		traceID := observability.NewTraceID()
		if trustIncomingTraceID {
			if incoming := observability.NormalizeTraceID(r.Header.Get(observability.TraceIDHeader)); incoming != "" {
				traceID = incoming
			}
		}

		ctx := observability.WithTraceID(r.Context(), traceID)
		r = r.WithContext(ctx)

		recorder := &statusRecorder{ResponseWriter: w, statusCode: nethttp.StatusOK}
		recorder.Header().Set(observability.TraceIDHeader, traceID)

		next.ServeHTTP(recorder, r)

		if logger == nil {
			return
		}

		durationMs := time.Since(start).Milliseconds()
		logger.Info(
			"http request",
			observability.LogFields{TraceID: &traceID},
			map[string]any{
				"method":      r.Method,
				"path":        r.URL.Path,
				"status_code": recorder.statusCode,
				"duration_ms": durationMs,
			},
		)
	})
}

func RecoverMiddleware(next nethttp.Handler, logger *observability.JSONLogger) nethttp.Handler {
	if next == nil {
		return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
			WriteError(w, nethttp.StatusInternalServerError, "internal_error", "internal error", "", nil)
		})
	}

	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		defer func() {
			if value := recover(); value != nil {
				traceID := observability.TraceIDFromContext(r.Context())
				if traceID == "" {
					traceID = observability.NewTraceID()
				}

				if logger != nil {
					logger.Error(
						"panic",
						observability.LogFields{TraceID: &traceID},
						map[string]any{
							"panic": fmt.Sprint(value),
							"stack": string(debug.Stack()),
						},
					)
				}

				WriteError(w, nethttp.StatusInternalServerError, "internal_error", "internal error", traceID, nil)
			}
		}()

		next.ServeHTTP(w, r)
	})
}
