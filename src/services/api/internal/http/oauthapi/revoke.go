package oauthapi

import (
	nethttp "net/http"
)

// revoke 实现 RFC 7009 token revocation。仅支持 token_type_hint=refresh_token
// （access_token 是无状态 JWT，不能主动撤销，靠短 TTL 兜底）。
//
// 为防 client_id 枚举攻击，无论 token 是否存在都返回 200。
func revoke(deps Deps) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if err := r.ParseForm(); err != nil {
			writeOAuthError(w, nethttp.StatusBadRequest, errInvalidRequest, "malformed form body")
			return
		}
		clientID, clientSecret := parseClientCredentials(r)
		client, err := authenticateClient(r.Context(), deps.ClientsRepo, clientID, clientSecret)
		if err != nil || client == nil {
			writeOAuthError(w, nethttp.StatusUnauthorized, errInvalidClient, "client authentication failed")
			return
		}

		token := r.FormValue("token")
		if token == "" {
			// RFC 7009 §2.2: 即使缺 token 也返回 200
			w.WriteHeader(nethttp.StatusOK)
			return
		}
		hash := hashCode(token)
		existing, err := deps.RefreshTokensRepo.GetByHash(r.Context(), hash)
		if err != nil {
			writeOAuthError(w, nethttp.StatusInternalServerError, errServerError, "lookup failed")
			return
		}
		if existing == nil || existing.ClientID != client.ClientID || existing.RevokedAt != nil {
			// 静默成功（防枚举）
			w.WriteHeader(nethttp.StatusOK)
			return
		}
		// 撤销整条 rotation 链，避免某节点遗漏
		_ = deps.RefreshTokensRepo.RevokeChain(r.Context(), hash)
		w.WriteHeader(nethttp.StatusOK)
	}
}
