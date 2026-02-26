package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"arkloop/services/gateway/internal/clientip"
	"arkloop/services/gateway/internal/geoip"
	"arkloop/services/gateway/internal/ua"
)

const traceIDHeader = "X-Trace-Id"

type statusRecorder struct {
	http.ResponseWriter
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
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(payload)
}

// Flush forwards to the underlying ResponseWriter; required for SSE/streaming.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func traceMiddleware(next http.Handler, logger *JSONLogger, geo geoip.Lookup) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		traceID := newTraceID()
		if incoming := normalizeTraceID(r.Header.Get(traceIDHeader)); incoming != "" {
			traceID = incoming
		}

		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		recorder.Header().Set(traceIDHeader, traceID)

		next.ServeHTTP(recorder, r)

		if logger == nil {
			return
		}

		// 日志字段：clientip 中间件在 next.ServeHTTP 之前已写入 context
		ip := clientip.FromContext(r.Context())
		uaInfo := ua.Parse(r)

		extra := map[string]any{
			"method":      r.Method,
			"path":        r.URL.Path,
			"status_code": recorder.statusCode,
			"duration_ms": time.Since(start).Milliseconds(),
			"client_ip":   ip,
			"user_agent":  uaInfo.Raw,
			"ua_type":     string(uaInfo.Type),
		}

		if geo != nil && ip != "" {
			result := geo.LookupIP(ip)
			if result.Country != "" {
				extra["country"] = result.Country
			}
		}

		// risk_score 由 riskMiddleware 写在 X-Risk-Score 请求头（已转发给上游）
		if score := r.Header.Get("X-Risk-Score"); score != "" {
			extra["risk_score"] = score
		}

		tid := traceID
		logger.Info("request", LogFields{TraceID: &tid}, extra)
	})
}

func recoverMiddleware(next http.Handler, logger *JSONLogger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				// 复用 traceMiddleware 已设置在 response header 上的 traceID
				traceID := w.Header().Get(traceIDHeader)
				if traceID == "" {
					traceID = newTraceID()
				}
				if logger != nil {
					logger.Error("panic", LogFields{TraceID: &traceID}, map[string]any{
						"panic": fmt.Sprint(value),
						"stack": string(debug.Stack()),
					})
				}
				http.Error(w, `{"code":"internal.error","message":"internal error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func newTraceID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(buf)
}

func normalizeTraceID(value string) string {
	candidate := strings.TrimSpace(value)
	if len(candidate) != 32 {
		return ""
	}
	for i := 0; i < len(candidate); i++ {
		ch := candidate[i]
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F') {
			continue
		}
		return ""
	}
	return strings.ToLower(candidate)
}
