package oauthapi

import (
	nethttp "net/http"
	"strings"
)

// discovery 实现 GET /.well-known/openid-configuration（RFC 8414 metadata）。
// 内容由 Issuer 拼接而成，浏览器/exam 端首次启动会拉取并缓存。
func discovery(deps Deps) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		issuer := strings.TrimRight(deps.Issuer, "/")
		out := map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/v1/auth/oauth/authorize",
			"token_endpoint":                        issuer + "/v1/auth/oauth/token",
			"userinfo_endpoint":                     issuer + "/v1/auth/oauth/userinfo",
			"revocation_endpoint":                   issuer + "/v1/auth/oauth/revoke",
			"jwks_uri":                              issuer + "/.well-known/jwks.json",
			"response_types_supported":              []string{"code"},
			"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
			"scopes_supported":                      []string{"openid", "profile", "email", "offline_access", "exam:read", "exam:write", "exam:admin"},
			"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic"},
			"code_challenge_methods_supported":      []string{"S256"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		writeJSON(w, nethttp.StatusOK, out)
	}
}

// jwks 实现 GET /.well-known/jwks.json。返回 active + retired 的公钥集。
func jwks(deps Deps) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		keys, err := deps.OIDCService.PublishableJWKs(r.Context())
		if err != nil {
			writeOAuthError(w, nethttp.StatusInternalServerError, errServerError, "jwks unavailable")
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=3600")
		writeJSON(w, nethttp.StatusOK, map[string]any{"keys": keys})
	}
}
