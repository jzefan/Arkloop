package oauthapi

import (
	"context"
	nethttp "net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// authorize 实现 OIDC Authorization Code Flow 的起始端点。
//
//   1. 校验 query 参数（response_type、client_id、redirect_uri、scope、state、PKCE）
//   2. 解析当前 ArkLoop 登录用户；未登录则 302 跳到 /login?next=...
//   3. 查 oauth_consents：覆盖性同意已存在则直接颁发 code 并 302 回 redirect_uri
//   4. 否则把请求参数透传到前端 consent 页，由 consentPost 完成最终颁发
//
// 错误处理原则：参数本身有问题（无效 client_id、redirect_uri 不匹配）→ 400 JSON，
// 防开放重定向；参数语义错误（scope 超额、PKCE 缺失）→ 302 重定向带 error 参数。
func authorize(deps Deps) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		ctx := r.Context()
		q := r.URL.Query()

		responseType := q.Get("response_type")
		clientID := q.Get("client_id")
		redirectURI := q.Get("redirect_uri")
		rawScope := q.Get("scope")
		state := q.Get("state")
		codeChallenge := q.Get("code_challenge")
		codeChallengeMethod := q.Get("code_challenge_method")
		nonce := q.Get("nonce")

		// ─── 不可重定向校验 ─────────────────────────────────────────
		if clientID == "" || redirectURI == "" {
			writeOAuthError(w, nethttp.StatusBadRequest, errInvalidRequest, "client_id and redirect_uri are required")
			return
		}
		client, err := deps.ClientsRepo.GetByClientID(ctx, clientID)
		if err != nil {
			writeOAuthError(w, nethttp.StatusInternalServerError, errServerError, "client lookup failed")
			return
		}
		if client == nil {
			writeOAuthError(w, nethttp.StatusBadRequest, errInvalidClient, "unknown client_id")
			return
		}
		if !validateRedirectURI(client, redirectURI) {
			writeOAuthError(w, nethttp.StatusBadRequest, errInvalidRequest, "redirect_uri not registered for this client")
			return
		}

		// ─── 可重定向校验（错误回传到 redirect_uri）─────────────────
		if responseType != "code" {
			redirectOAuthError(w, r, redirectURI, state, errUnsupportedResponseType, "only response_type=code is supported")
			return
		}
		if len(state) < 16 {
			redirectOAuthError(w, r, redirectURI, state, errInvalidRequest, "state must be at least 16 characters")
			return
		}
		if client.RequirePKCE {
			if codeChallenge == "" || codeChallengeMethod != "S256" {
				redirectOAuthError(w, r, redirectURI, state, errInvalidRequest, "PKCE S256 is required")
				return
			}
		}
		scopes := parseScopes(rawScope)
		if !scopeContains(scopes, "openid") {
			redirectOAuthError(w, r, redirectURI, state, errInvalidScope, "scope must include openid")
			return
		}
		if !scopesSubset(scopes, client.AllowedScopes) {
			redirectOAuthError(w, r, redirectURI, state, errInvalidScope, "requested scope exceeds client allowed_scopes")
			return
		}

		// ─── 用户登录态检查 ─────────────────────────────────────────
		userID := currentUserFromSessionCookie(ctx, r, deps.SessionRefreshRepo)
		if userID == uuid.Nil {
			// 把当前完整 URL 作为 next 透传给登录页；登录完成后浏览器再回到这里
			loginURL := "/login?next=" + url.QueryEscape(r.URL.String())
			nethttp.Redirect(w, r, loginURL, nethttp.StatusFound)
			return
		}

		// ─── Consent 检查 ────────────────────────────────────────────
		consent, err := deps.ConsentsRepo.Get(ctx, userID, clientID)
		if err != nil {
			redirectOAuthError(w, r, redirectURI, state, errServerError, "consent lookup failed")
			return
		}
		needsConsent := consent == nil || !scopesSubset(scopes, consent.Scopes)

		if needsConsent {
			// 把 authorize 全套参数转发给前端 consent 页。前端拿用户确认后回调
			// POST /v1/auth/oauth/consent 完成 code 颁发。
			consentURL := strings.TrimRight(deps.FrontendConsentPath, "/") + "?" + r.URL.RawQuery
			nethttp.Redirect(w, r, consentURL, nethttp.StatusFound)
			return
		}

		// ─── 颁发 code 并重定向 ────────────────────────────────────
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

type codeIssueParams struct {
	ClientID            string
	UserID              uuid.UUID
	RedirectURI         string
	Scopes              []string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Nonce               string
}

// issueCodeAndRedirect 把 codeIssueParams 实例化为 oauth_authorization_codes 行并 302 回 redirect_uri。
// 单独成函数是为了让 authorize 和 consentPost 都能调用。
func issueCodeAndRedirect(
	ctx context.Context,
	deps Deps,
	w nethttp.ResponseWriter,
	r *nethttp.Request,
	p codeIssueParams,
) error {
	plainCode, err := randomURLToken()
	if err != nil {
		return err
	}
	codeHash := hashCode(plainCode)
	var noncePtr *string
	if p.Nonce != "" {
		noncePtr = &p.Nonce
	}
	if err := deps.AuthCodesRepo.Create(
		ctx,
		codeHash,
		p.ClientID,
		p.UserID,
		p.RedirectURI,
		p.Scopes,
		p.CodeChallenge,
		p.CodeChallengeMethod,
		noncePtr,
		time.Now().Add(60*time.Second), // 60s TTL per spec
	); err != nil {
		return err
	}

	u, err := url.Parse(p.RedirectURI)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("code", plainCode)
	q.Set("state", p.State)
	u.RawQuery = q.Encode()
	nethttp.Redirect(w, r, u.String(), nethttp.StatusFound)
	return nil
}
