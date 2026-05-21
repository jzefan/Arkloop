package oauthapi

import (
	nethttp "net/http"
	"strings"

	"github.com/google/uuid"
)

// userinfo 实现 GET /v1/auth/oauth/userinfo。
// 验签 access_token 后按 scope 返回字段子集（OIDC Core §5.3）。
func userinfo(deps Deps) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			w.Header().Set("WWW-Authenticate", `Bearer realm="userinfo"`)
			writeOAuthError(w, nethttp.StatusUnauthorized, errInvalidRequest, "Bearer token required")
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))

		claims, err := deps.OIDCService.VerifyToken(r.Context(), token)
		if err != nil {
			writeOAuthError(w, nethttp.StatusUnauthorized, errInvalidGrant, err.Error())
			return
		}
		subStr, _ := claims["sub"].(string)
		userID, err := uuid.Parse(subStr)
		if err != nil {
			writeOAuthError(w, nethttp.StatusUnauthorized, errInvalidGrant, "invalid sub claim")
			return
		}
		scopeStr, _ := claims["scope"].(string)
		scopes := parseScopes(scopeStr)

		user, err := deps.UsersRepo.GetByID(r.Context(), userID)
		if err != nil || user == nil {
			writeOAuthError(w, nethttp.StatusUnauthorized, errInvalidGrant, "user not found")
			return
		}

		out := map[string]any{"sub": user.ID.String()}
		if scopeContains(scopes, "email") && user.Email != nil {
			out["email"] = *user.Email
			out["email_verified"] = user.EmailVerifiedAt != nil
		}
		if scopeContains(scopes, "profile") {
			if user.Username != "" {
				out["name"] = user.Username
				out["preferred_username"] = user.Username
			}
			if user.AvatarURL != nil {
				out["picture"] = *user.AvatarURL
			}
			if user.Locale != nil {
				out["locale"] = *user.Locale
			}
			if user.Timezone != nil {
				out["zoneinfo"] = *user.Timezone
			}
		}
		writeJSON(w, nethttp.StatusOK, out)
	}
}
