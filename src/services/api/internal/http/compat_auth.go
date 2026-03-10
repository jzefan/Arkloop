package http

import (
	"encoding/json"
	"net/mail"
	"strings"

	nethttp "net/http"

	httpkit "arkloop/services/api/internal/http/httpkit"
)

const (
	refreshTokenCookieName = "arkloop_refresh_token"
	refreshTokenCookiePath = "/v1/auth"
	clientAppHeader        = "X-Client-App"
)

var allowedClientApps = map[string]string{
	"web":          "arkloop_rt_web",
	"console":      "arkloop_rt_console",
	"console-lite": "arkloop_rt_console_lite",
}

type captchaConfigResponse struct {
	Enabled bool   `json:"enabled"`
	SiteKey string `json:"site_key"`
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

type logoutResponse struct {
	OK bool `json:"ok"`
}

type registerResponse struct {
	UserID      string  `json:"user_id"`
	AccessToken string  `json:"access_token"`
	TokenType   string  `json:"token_type"`
	Warning     *string `json:"warning,omitempty"`
}

type registrationModeResponse struct {
	Mode string `json:"mode"`
}

type meResponse struct {
	ID                        string   `json:"id"`
	Username                  string   `json:"username"`
	Email                     *string  `json:"email,omitempty"`
	EmailVerified             bool     `json:"email_verified"`
	EmailVerificationRequired bool     `json:"email_verification_required"`
	CreatedAt                 string   `json:"created_at"`
	OrgID                     string   `json:"org_id,omitempty"`
	OrgName                   string   `json:"org_name,omitempty"`
	Role                      string   `json:"role,omitempty"`
	Permissions               []string `json:"permissions"`
}

type updateMeResponse struct {
	Username string `json:"username"`
}

func parseBearerToken(w nethttp.ResponseWriter, r *nethttp.Request, traceID string) (string, bool) {
	authorization := r.Header.Get("Authorization")
	if strings.TrimSpace(authorization) == "" {
		WriteError(w, nethttp.StatusUnauthorized, "auth.missing_token", "missing Authorization Bearer token", traceID, nil)
		return "", false
	}
	scheme, rest, ok := strings.Cut(authorization, " ")
	if !ok || strings.TrimSpace(rest) == "" || strings.ToLower(scheme) != "bearer" {
		WriteError(w, nethttp.StatusUnauthorized, "auth.invalid_authorization", "Authorization header must be: Bearer <token>", traceID, nil)
		return "", false
	}
	return strings.TrimSpace(rest), true
}

func writeAuthNotConfigured(w nethttp.ResponseWriter, traceID string) {
	httpkit.WriteAuthNotConfigured(w, traceID)
}

func isValidEmail(value string) bool {
	if strings.ContainsAny(value, "\r\n") {
		return false
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	addr, err := mail.ParseAddress(trimmed)
	if err != nil || addr == nil {
		return false
	}
	return addr.Address == trimmed
}

const maxJSONBodySize = 1 << 20

func decodeJSON(r *nethttp.Request, dst any) error {
	reader := nethttp.MaxBytesReader(nil, r.Body, maxJSONBodySize)
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func writeJSON(w nethttp.ResponseWriter, traceID string, statusCode int, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		WriteError(w, nethttp.StatusInternalServerError, "internal.error", "internal error", traceID, nil)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_, _ = w.Write(raw)
}
