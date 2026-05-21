package oauthapi

import (
	"encoding/json"
	nethttp "net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// internalIssueRequest 是 worker 调用 /internal/oauth/issue 的请求体。
type internalIssueRequest struct {
	UserID   string   `json:"user_id"`
	ClientID string   `json:"client_id"`
	Scopes   []string `json:"scopes"`
}

// internalIssue 实现 POST /internal/oauth/issue（T10 任务核心）。
//
// 关键安全约束（与 design doc §6.4 一致）：
//   1. 必须用 service token 鉴权（仅 worker 持有）
//   2. 申请的 scopes 不允许包含 exam:admin 等高权 scope
//   3. 颁发的 access_token TTL 强制为 60s
//   4. 永远不颁发 refresh_token（每次按需重新铸）
//
// 这是智能体能"以用户身份调 exam"的唯一桥梁；其失守 == 全平台失守。
func internalIssue(deps Deps) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if !validateServiceToken(r, deps.InternalServiceToken) {
			writeOAuthError(w, nethttp.StatusUnauthorized, errInvalidClient, "service token required")
			return
		}

		var req internalIssueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeOAuthError(w, nethttp.StatusBadRequest, errInvalidRequest, "malformed JSON")
			return
		}
		userID, err := uuid.Parse(req.UserID)
		if err != nil {
			writeOAuthError(w, nethttp.StatusBadRequest, errInvalidRequest, "invalid user_id")
			return
		}
		if req.ClientID == "" {
			writeOAuthError(w, nethttp.StatusBadRequest, errInvalidRequest, "client_id is required")
			return
		}
		if len(req.Scopes) == 0 {
			writeOAuthError(w, nethttp.StatusBadRequest, errInvalidScope, "at least one scope is required")
			return
		}

		// 白名单：拒绝任何高敏 scope。
		for _, s := range req.Scopes {
			if ScopesForbiddenForInternal[s] {
				writeOAuthError(w, nethttp.StatusForbidden, errInvalidScope, "scope "+s+" not allowed for internal issuance")
				return
			}
		}

		client, err := deps.ClientsRepo.GetByClientID(r.Context(), req.ClientID)
		if err != nil {
			writeOAuthError(w, nethttp.StatusInternalServerError, errServerError, "client lookup failed")
			return
		}
		if client == nil {
			writeOAuthError(w, nethttp.StatusBadRequest, errInvalidClient, "unknown client_id")
			return
		}
		// 必须落在 client.allowed_scopes 内
		if !scopesSubset(req.Scopes, client.AllowedScopes) {
			writeOAuthError(w, nethttp.StatusBadRequest, errInvalidScope, "scopes exceed client allow list")
			return
		}

		const internalTTL = 60 // seconds — 远短于浏览器流的 15min
		now := time.Now().UTC()
		claims := jwt.MapClaims{
			"iss":       deps.Issuer,
			"sub":       userID.String(),
			"aud":       client.ClientID,
			"azp":       client.ClientID,
			"client_id": client.ClientID,
			"scope":     joinScopes(req.Scopes),
			"iat":       now.Unix(),
			"exp":       now.Add(internalTTL * time.Second).Unix(),
			"jti":       uuid.New().String(),
			// Marker so downstream audit can distinguish internal-issued tokens
			"internal_issue": true,
		}

		// Enrich claims with the user's email / name so downstream services
		// (exam) can auto-provision a sensible account on first contact —
		// the user has never visited exam in a browser, so this is their
		// only chance to surface a human-readable identity.
		if deps.UsersRepo != nil {
			if user, uerr := deps.UsersRepo.GetByID(r.Context(), userID); uerr == nil && user != nil {
				if user.Email != nil {
					claims["email"] = *user.Email
					claims["email_verified"] = user.EmailVerifiedAt != nil
				}
				if user.Username != "" {
					claims["name"] = user.Username
					claims["preferred_username"] = user.Username
				}
				if user.AvatarURL != nil {
					claims["picture"] = *user.AvatarURL
				}
			}
		}
		accessToken, err := deps.OIDCService.IssueToken(r.Context(), claims)
		if err != nil {
			writeOAuthError(w, nethttp.StatusInternalServerError, errServerError, "sign failed")
			return
		}

		writeJSON(w, nethttp.StatusOK, map[string]any{
			"access_token": accessToken,
			"token_type":   "Bearer",
			"expires_in":   internalTTL,
			"scope":        joinScopes(req.Scopes),
		})
	}
}
