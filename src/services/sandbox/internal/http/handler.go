package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"arkloop/services/sandbox/internal/logging"
	"arkloop/services/sandbox/internal/session"
)

// NewHandler 注册所有路由并返回 HTTP handler。
func NewHandler(mgr *session.Manager, logger *logging.JSONLogger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthz(mgr))
	mux.HandleFunc("POST /v1/exec", handleExec(mgr, logger))
	mux.HandleFunc("DELETE /v1/sessions/", handleDeleteSession(mgr, logger))
	return recoverMiddleware(mux, logger)
}

func healthz(mgr *session.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":   "ok",
			"sessions": mgr.ActiveCount(),
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	payload, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"code":"internal.error","message":"marshal failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"code":    code,
		"message": message,
	})
}

func recoverMiddleware(next http.Handler, logger *logging.JSONLogger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				logger.Error("panic recovered", logging.LogFields{}, map[string]any{"panic": fmt.Sprintf("%v", v)})
				writeError(w, http.StatusInternalServerError, "internal.panic", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
