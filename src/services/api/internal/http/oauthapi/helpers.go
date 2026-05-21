package oauthapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	nethttp "net/http"
	"net/url"
	"strings"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// ─── OAuth 标准错误（RFC 6749 §5.2）。authorize 端点重定向回 redirect_uri 时传递；
//      token 端点以 400 JSON 形式返回。 ──────────────────────────────────────────

const (
	errInvalidRequest          = "invalid_request"
	errInvalidGrant            = "invalid_grant"
	errInvalidClient           = "invalid_client"
	errInvalidScope            = "invalid_scope"
	errUnauthorizedClient      = "unauthorized_client"
	errUnsupportedGrantType    = "unsupported_grant_type"
	errUnsupportedResponseType = "unsupported_response_type"
	errAccessDenied            = "access_denied"
	errServerError             = "server_error"
)

// ScopesForbiddenForInternal 是 /internal/oauth/issue 永远拒绝的 scope 黑名单。
// 智能体即使受 prompt injection 攻击也越不过这条边界。
var ScopesForbiddenForInternal = map[string]bool{
	"exam:admin": true,
}

func writeJSON(w nethttp.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeOAuthError(w nethttp.ResponseWriter, status int, code, description string) {
	writeJSON(w, status, map[string]string{
		"error":             code,
		"error_description": description,
	})
}

// redirectOAuthError 把错误回传到 client.redirect_uri（仅当 redirect_uri 已校验通过）。
// 当 redirect_uri 本身无效时不能调用，必须直接写 400 防止开放重定向。
func redirectOAuthError(w nethttp.ResponseWriter, r *nethttp.Request, redirectURI, state, code, description string) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		writeOAuthError(w, nethttp.StatusBadRequest, errInvalidRequest, "malformed redirect_uri")
		return
	}
	q := u.Query()
	q.Set("error", code)
	if description != "" {
		q.Set("error_description", description)
	}
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	nethttp.Redirect(w, r, u.String(), nethttp.StatusFound)
}

// parseScopes 把"openid profile email"切成 []string，去重保序，过滤空串。
func parseScopes(raw string) []string {
	out := make([]string, 0, 4)
	seen := make(map[string]bool, 4)
	for _, p := range strings.Fields(raw) {
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func joinScopes(scopes []string) string {
	return strings.Join(scopes, " ")
}

// scopesSubset 判断 want ⊆ have（顺序无关）。
func scopesSubset(want, have []string) bool {
	haveSet := make(map[string]bool, len(have))
	for _, s := range have {
		haveSet[s] = true
	}
	for _, s := range want {
		if !haveSet[s] {
			return false
		}
	}
	return true
}

// validateRedirectURI 精确匹配 client.redirect_uris 中的某一项（scheme/host/port/path/query 全部一致）。
func validateRedirectURI(client *data.OAuthClient, candidate string) bool {
	for _, allowed := range client.RedirectURIs {
		if allowed == candidate {
			return true
		}
	}
	return false
}

// parseClientCredentials 同时支持 client_secret_basic（HTTP Basic Auth）
// 和 client_secret_post（form body）；post 优先于 basic（OAuth 2.1 推荐）。
func parseClientCredentials(r *nethttp.Request) (clientID, clientSecret string) {
	clientID = strings.TrimSpace(r.FormValue("client_id"))
	clientSecret = strings.TrimSpace(r.FormValue("client_secret"))
	if clientID != "" && clientSecret != "" {
		return
	}
	if username, password, ok := r.BasicAuth(); ok {
		if clientID == "" {
			clientID = strings.TrimSpace(username)
		}
		if clientSecret == "" {
			clientSecret = password
		}
	}
	return
}

// authenticateClient 查找 client 并对比 secret hash。返回 nil, nil 表示
// 任一不匹配（不区分原因，防 client_id enumeration）。
func authenticateClient(
	ctx context.Context,
	repo *data.OAuthClientRepository,
	clientID, clientSecret string,
) (*data.OAuthClient, error) {
	if clientID == "" || clientSecret == "" {
		return nil, nil
	}
	c, err := repo.GetByClientID(ctx, clientID)
	if err != nil || c == nil {
		return nil, err
	}
	if c.ClientType == "confidential" {
		if err := bcrypt.CompareHashAndPassword([]byte(c.ClientSecretHash), []byte(clientSecret)); err != nil {
			return nil, nil
		}
	}
	return c, nil
}

// verifyPKCES256 校验 BASE64URL(SHA256(verifier)) == challenge。
// 使用 constant-time compare 防 timing。
func verifyPKCES256(codeVerifier, codeChallenge string) bool {
	sum := sha256.Sum256([]byte(codeVerifier))
	derived := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(derived), []byte(codeChallenge)) == 1
}

// hashCode 用于把"明文 code / refresh_token"映射到数据库主键。
func hashCode(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// randomURLToken 生成 32 字节随机串的 base64url 表示，作为 plain code / refresh_token。
func randomURLToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// currentUserFromSessionCookie 解析 first-party refresh_token cookie，
// 返回当前已登录的 ArkLoop user_id。Cookie 不存在或 token 已失效返回 uuid.Nil。
// authorize 端点用此函数确认"用户是否登录"。
func currentUserFromSessionCookie(
	ctx context.Context,
	r *nethttp.Request,
	repo *data.RefreshTokenRepository,
) uuid.UUID {
	if repo == nil {
		return uuid.Nil
	}
	cookie, err := r.Cookie("arkloop_refresh_token")
	if err != nil || cookie == nil || cookie.Value == "" {
		return uuid.Nil
	}
	hash := hashCode(cookie.Value)
	t, err := repo.GetByHash(ctx, hash)
	if err != nil || t == nil {
		return uuid.Nil
	}
	return t.UserID
}

// validateServiceToken constant-time 对比 worker 提供的 Bearer token。
func validateServiceToken(r *nethttp.Request, expected string) bool {
	if expected == "" {
		return false
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

// scopeContains 是 strings.Contains for []string 的便捷封装。
func scopeContains(scopes []string, target string) bool {
	for _, s := range scopes {
		if s == target {
			return true
		}
	}
	return false
}

// errPKCEMismatch 标识 PKCE verifier 校验失败。
var errPKCEMismatch = errors.New("pkce verifier does not match challenge")
