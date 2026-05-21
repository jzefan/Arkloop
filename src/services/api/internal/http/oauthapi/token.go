package oauthapi

import (
	"context"
	"errors"
	"fmt"
	nethttp "net/http"
	"time"

	"arkloop/services/api/internal/data"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// token 实现 POST /v1/auth/oauth/token。支持两种 grant_type:
//   - authorization_code: 拿 code 换 token，验证 PKCE，颁发 access + id + refresh
//   - refresh_token: rotation-on-use，重放时 revoke 整条 chain
func token(deps Deps) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		if err := r.ParseForm(); err != nil {
			writeOAuthError(w, nethttp.StatusBadRequest, errInvalidRequest, "malformed form body")
			return
		}
		grantType := r.FormValue("grant_type")
		clientID, clientSecret := parseClientCredentials(r)

		client, err := authenticateClient(r.Context(), deps.ClientsRepo, clientID, clientSecret)
		if err != nil {
			writeOAuthError(w, nethttp.StatusInternalServerError, errServerError, "client auth failed")
			return
		}
		if client == nil {
			writeOAuthError(w, nethttp.StatusUnauthorized, errInvalidClient, "client authentication failed")
			return
		}

		switch grantType {
		case "authorization_code":
			tokenAuthorizationCode(deps, w, r, client)
		case "refresh_token":
			tokenRefreshToken(deps, w, r, client)
		default:
			writeOAuthError(w, nethttp.StatusBadRequest, errUnsupportedGrantType, "unsupported grant_type")
		}
	}
}

func tokenAuthorizationCode(deps Deps, w nethttp.ResponseWriter, r *nethttp.Request, client *data.OAuthClient) {
	ctx := r.Context()
	code := r.FormValue("code")
	redirectURI := r.FormValue("redirect_uri")
	codeVerifier := r.FormValue("code_verifier")

	if code == "" || redirectURI == "" || codeVerifier == "" {
		writeOAuthError(w, nethttp.StatusBadRequest, errInvalidRequest, "code, redirect_uri, code_verifier are required")
		return
	}

	codeHash := hashCode(code)
	record, err := deps.AuthCodesRepo.Consume(ctx, codeHash)
	if err != nil {
		writeOAuthError(w, nethttp.StatusInternalServerError, errServerError, "code consume failed")
		return
	}
	if record == nil {
		writeOAuthError(w, nethttp.StatusBadRequest, errInvalidGrant, "code is invalid, expired, or already used")
		return
	}
	if record.ClientID != client.ClientID {
		writeOAuthError(w, nethttp.StatusBadRequest, errInvalidGrant, "code does not belong to this client")
		return
	}
	if record.RedirectURI != redirectURI {
		writeOAuthError(w, nethttp.StatusBadRequest, errInvalidGrant, "redirect_uri does not match the authorize request")
		return
	}
	if !verifyPKCES256(codeVerifier, record.CodeChallenge) {
		writeOAuthError(w, nethttp.StatusBadRequest, errInvalidGrant, "PKCE verifier does not match challenge")
		return
	}

	nonce := ""
	if record.Nonce != nil {
		nonce = *record.Nonce
	}
	resp, err := issueTokenBundle(ctx, deps, client, record.UserID, record.Scopes, nonce, true /*allowRefresh*/)
	if err != nil {
		writeOAuthError(w, nethttp.StatusInternalServerError, errServerError, "token issuance failed: "+err.Error())
		return
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

func tokenRefreshToken(deps Deps, w nethttp.ResponseWriter, r *nethttp.Request, client *data.OAuthClient) {
	ctx := r.Context()
	refreshToken := r.FormValue("refresh_token")
	if refreshToken == "" {
		writeOAuthError(w, nethttp.StatusBadRequest, errInvalidRequest, "refresh_token is required")
		return
	}
	oldHash := hashCode(refreshToken)
	old, err := deps.RefreshTokensRepo.GetByHash(ctx, oldHash)
	if err != nil {
		writeOAuthError(w, nethttp.StatusInternalServerError, errServerError, "refresh lookup failed")
		return
	}
	if old == nil {
		writeOAuthError(w, nethttp.StatusBadRequest, errInvalidGrant, "refresh_token not found")
		return
	}
	if old.ClientID != client.ClientID {
		writeOAuthError(w, nethttp.StatusBadRequest, errInvalidGrant, "refresh_token does not belong to this client")
		return
	}

	// ─── 重放检测：被使用过（revoked）的 token 再次出现 → 假定泄露，撤销整条链 ───
	if old.RevokedAt != nil {
		_ = deps.RefreshTokensRepo.RevokeChain(ctx, oldHash)
		writeOAuthError(w, nethttp.StatusBadRequest, errInvalidGrant, "refresh_token has been revoked (replay detected)")
		return
	}
	if time.Now().After(old.ExpiresAt) {
		writeOAuthError(w, nethttp.StatusBadRequest, errInvalidGrant, "refresh_token expired")
		return
	}

	// 可选 scope 收窄
	requested := parseScopes(r.FormValue("scope"))
	scopes := old.Scopes
	if len(requested) > 0 {
		if !scopesSubset(requested, old.Scopes) {
			writeOAuthError(w, nethttp.StatusBadRequest, errInvalidScope, "requested scope exceeds original grant")
			return
		}
		scopes = requested
	}

	// 生成新 refresh + access；事务内 rotate
	newRefresh, err := randomURLToken()
	if err != nil {
		writeOAuthError(w, nethttp.StatusInternalServerError, errServerError, "random gen failed")
		return
	}
	newHash := hashCode(newRefresh)
	newExpires := time.Now().Add(time.Duration(deps.RefreshTokenTTLSeconds) * time.Second)

	ok, err := deps.RefreshTokensRepo.Rotate(ctx, deps.Pool, oldHash, newHash, scopes, newExpires)
	if err != nil {
		writeOAuthError(w, nethttp.StatusInternalServerError, errServerError, "rotation failed")
		return
	}
	if !ok {
		writeOAuthError(w, nethttp.StatusBadRequest, errInvalidGrant, "refresh_token concurrently revoked")
		return
	}

	// id_token 在 refresh grant 时不重新颁发 nonce（按 OIDC §12.1 可以省略）
	accessToken, err := signAccessToken(ctx, deps, client.ClientID, old.UserID, scopes)
	if err != nil {
		writeOAuthError(w, nethttp.StatusInternalServerError, errServerError, "sign access token failed")
		return
	}
	resp := tokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    deps.AccessTokenTTLSeconds,
		Scope:        joinScopes(scopes),
		RefreshToken: newRefresh,
	}
	// refresh 时 id_token 可选——按 OIDC §12.2 如果 openid scope 还在则返回新 id_token
	if scopeContains(scopes, "openid") {
		idToken, err := signIDToken(ctx, deps, client.ClientID, old.UserID, "", 0)
		if err != nil {
			writeOAuthError(w, nethttp.StatusInternalServerError, errServerError, "sign id token failed")
			return
		}
		resp.IDToken = idToken
	}
	writeJSON(w, nethttp.StatusOK, resp)
}

// tokenResponse 是 /token 端点的 JSON 响应，结构与 RFC 6749 §5.1 + OIDC Core §3.1.3.3 对齐。
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// issueTokenBundle 是 authorization_code grant 颁发完整 token 套件的中枢逻辑。
// allowRefresh 在 authorization_code 流程中始终为 true；refresh_token grant 直接调用 sign* 函数。
func issueTokenBundle(
	ctx context.Context,
	deps Deps,
	client *data.OAuthClient,
	userID uuid.UUID,
	scopes []string,
	nonce string,
	allowRefresh bool,
) (*tokenResponse, error) {
	accessToken, err := signAccessToken(ctx, deps, client.ClientID, userID, scopes)
	if err != nil {
		return nil, fmt.Errorf("sign access: %w", err)
	}
	resp := tokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   deps.AccessTokenTTLSeconds,
		Scope:       joinScopes(scopes),
	}
	if scopeContains(scopes, "openid") {
		idToken, err := signIDToken(ctx, deps, client.ClientID, userID, nonce, 0)
		if err != nil {
			return nil, fmt.Errorf("sign id: %w", err)
		}
		resp.IDToken = idToken
	}
	if allowRefresh && scopeContains(scopes, "offline_access") {
		plain, err := randomURLToken()
		if err != nil {
			return nil, err
		}
		hash := hashCode(plain)
		exp := time.Now().Add(time.Duration(deps.RefreshTokenTTLSeconds) * time.Second)
		if err := deps.RefreshTokensRepo.Create(ctx, hash, client.ClientID, userID, scopes, exp); err != nil {
			return nil, fmt.Errorf("create refresh: %w", err)
		}
		resp.RefreshToken = plain
	}
	return &resp, nil
}

// signAccessToken 颁发 RS256 access_token（JWT）。claims 与 §7.1 一致。
func signAccessToken(
	ctx context.Context,
	deps Deps,
	clientID string,
	userID uuid.UUID,
	scopes []string,
) (string, error) {
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iss":       deps.Issuer,
		"sub":       userID.String(),
		"aud":       clientID,
		"azp":       clientID,
		"client_id": clientID,
		"scope":     joinScopes(scopes),
		"iat":       now.Unix(),
		"exp":       now.Add(time.Duration(deps.AccessTokenTTLSeconds) * time.Second).Unix(),
		"jti":       uuid.New().String(),
	}
	return deps.OIDCService.IssueToken(ctx, claims)
}

// signIDToken 颁发 OIDC id_token。claim 与 §7.2 一致。authTime=0 时省略 auth_time。
func signIDToken(
	ctx context.Context,
	deps Deps,
	clientID string,
	userID uuid.UUID,
	nonce string,
	authTime int64,
) (string, error) {
	user, err := deps.UsersRepo.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}
	if user == nil {
		return "", errors.New("user not found for id_token")
	}
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iss": deps.Issuer,
		"sub": userID.String(),
		"aud": clientID,
		"iat": now.Unix(),
		"exp": now.Add(time.Duration(deps.IDTokenTTLSeconds) * time.Second).Unix(),
	}
	if user.Email != nil {
		claims["email"] = *user.Email
		claims["email_verified"] = user.EmailVerifiedAt != nil
	}
	if user.Username != "" {
		claims["name"] = user.Username
	}
	if user.AvatarURL != nil {
		claims["picture"] = *user.AvatarURL
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	if authTime > 0 {
		claims["auth_time"] = authTime
	}
	return deps.OIDCService.IssueToken(ctx, claims)
}
