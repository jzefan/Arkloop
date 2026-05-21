package oauthapi

import (
	nethttp "net/http"

	"github.com/google/uuid"
)

// consentGet 给前端 consent 页提供"该展示哪些 scope、client 是谁"的元数据。
// 前端 GET 这个端点拿 JSON 后渲染同意页。
//
// query 参数同 authorize 端点透传过来，前端不需要解析，原样回 POST consent 提交。
func consentGet(deps Deps) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		ctx := r.Context()
		userID := currentUserFromSessionCookie(ctx, r, deps.SessionRefreshRepo)
		if userID == uuid.Nil {
			writeOAuthError(w, nethttp.StatusUnauthorized, errAccessDenied, "login required")
			return
		}
		clientID := r.URL.Query().Get("client_id")
		client, err := deps.ClientsRepo.GetByClientID(ctx, clientID)
		if err != nil || client == nil {
			writeOAuthError(w, nethttp.StatusBadRequest, errInvalidClient, "unknown client_id")
			return
		}
		scopes := parseScopes(r.URL.Query().Get("scope"))
		writeJSON(w, nethttp.StatusOK, map[string]any{
			"client": map[string]any{
				"client_id": client.ClientID,
				"name":      client.Name,
			},
			"scopes_requested": scopes,
			"scope_descriptions": map[string]string{
				"openid":         "Identify you across applications",
				"profile":        "Read your display name and avatar",
				"email":          "Read your email address",
				"offline_access": "Stay signed in after your session expires",
				"exam:read":      "Read your courses, knowledge points and questions",
				"exam:write":     "Create and update courses and questions on your behalf",
				"exam:admin":     "Perform administrative operations (deletes, cross-user actions)",
			},
		})
	}
}

// consentPost 用户在同意页点"允许"后调用。写入 oauth_consents，然后透传
// 完整 authorize 参数到 issueCodeAndRedirect，让 client 拿到 code。
//
// 拒绝场景：前端应直接 302 回 redirect_uri 带 error=access_denied，不调本端点。
func consentPost(deps Deps) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if err := r.ParseForm(); err != nil {
			writeOAuthError(w, nethttp.StatusBadRequest, errInvalidRequest, "malformed body")
			return
		}
		ctx := r.Context()
		userID := currentUserFromSessionCookie(ctx, r, deps.SessionRefreshRepo)
		if userID == uuid.Nil {
			writeOAuthError(w, nethttp.StatusUnauthorized, errAccessDenied, "login required")
			return
		}

		clientID := r.FormValue("client_id")
		redirectURI := r.FormValue("redirect_uri")
		state := r.FormValue("state")
		scopes := parseScopes(r.FormValue("scope"))
		codeChallenge := r.FormValue("code_challenge")
		codeChallengeMethod := r.FormValue("code_challenge_method")
		nonce := r.FormValue("nonce")

		client, err := deps.ClientsRepo.GetByClientID(ctx, clientID)
		if err != nil || client == nil {
			writeOAuthError(w, nethttp.StatusBadRequest, errInvalidClient, "unknown client_id")
			return
		}
		if !validateRedirectURI(client, redirectURI) {
			writeOAuthError(w, nethttp.StatusBadRequest, errInvalidRequest, "redirect_uri not registered")
			return
		}
		if !scopesSubset(scopes, client.AllowedScopes) {
			redirectOAuthError(w, r, redirectURI, state, errInvalidScope, "scope exceeds client allow list")
			return
		}

		// 持久化 consent。Grant 内部做 scope union。
		if err := deps.ConsentsRepo.Grant(ctx, userID, clientID, scopes); err != nil {
			redirectOAuthError(w, r, redirectURI, state, errServerError, "consent persistence failed")
			return
		}

		if err := issueCodeAndRedirect(ctx, deps, w, r, codeIssueParams{
			ClientID:            client.ClientID,
			UserID:              userID,
			RedirectURI:         redirectURI,
			Scopes:              scopes,
			State:               state,
			CodeChallenge:       codeChallenge,
			CodeChallengeMethod: codeChallengeMethod,
			Nonce:               nonce,
		}); err != nil {
			redirectOAuthError(w, r, redirectURI, state, errServerError, "code issuance failed")
		}
	}
}
