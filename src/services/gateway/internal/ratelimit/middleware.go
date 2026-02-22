package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)



const (
	rateLimitOrgKeyPrefix = "arkloop:ratelimit:org:"
	rateLimitIPKeyPrefix  = "arkloop:ratelimit:ip:"
)

// NewRateLimitMiddleware 返回限流中间件。
// SSE 请求（Accept: text/event-stream 或路径后缀 /events）跳过限流。
// 有效 JWT 或 API Key 按 org_id 限流；否则按客户端 IP 限流。
// Redis 不可用时 fail-open：放行请求，不阻断流量。
func NewRateLimitMiddleware(next http.Handler, limiter Limiter, jwtSecret string, rdb ...*redis.Client) http.Handler {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}))
	secretBytes := []byte(jwtSecret)

	var redisClient *redis.Client
	if len(rdb) > 0 {
		redisClient = rdb[0]
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSSE(r) {
			next.ServeHTTP(w, r)
			return
		}

		key := rateLimitKeyFromRequest(r, parser, secretBytes, redisClient)

		result, err := limiter.Consume(r.Context(), key)
		if err != nil {
			// Redis 不可用时放行，避免限流器故障阻断所有流量
			next.ServeHTTP(w, r)
			return
		}

		if !result.Allowed {
			writeRateLimitExceeded(w, result.RetryAfterSecs)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// rateLimitKeyFromRequest 提取限流 key。
// 优先从 JWT 或 API Key（Redis 缓存）取 org_id；失败时降级到客户端 IP。
func rateLimitKeyFromRequest(r *http.Request, parser *jwt.Parser, secret []byte, rdb *redis.Client) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	bearer, ok := strings.CutPrefix(auth, "Bearer ")
	if ok && strings.HasPrefix(bearer, "ak-") {
		if orgID := extractOrgIDFromAPIKey(r.Context(), bearer, rdb); orgID != uuid.Nil {
			return rateLimitOrgKeyPrefix + orgID.String()
		}
		return rateLimitIPKeyPrefix + clientIP(r)
	}

	if orgID := extractOrgIDFromBearer(r, parser, secret); orgID != uuid.Nil {
		return rateLimitOrgKeyPrefix + orgID.String()
	}
	return rateLimitIPKeyPrefix + clientIP(r)
}

type apiKeyCacheEntry struct {
	OrgID   string `json:"org_id"`
	Revoked bool   `json:"revoked"`
}

// extractOrgIDFromAPIKey 从 Redis 缓存中取 API Key 对应的 org_id。
// 缓存 miss 或 Redis 不可用时返回 uuid.Nil（降级为 IP 限流）。
func extractOrgIDFromAPIKey(ctx context.Context, rawKey string, rdb *redis.Client) uuid.UUID {
	if rdb == nil {
		return uuid.Nil
	}

	digest := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(digest[:])
	redisKey := fmt.Sprintf("arkloop:api_keys:%s", keyHash)

	raw, err := rdb.Get(ctx, redisKey).Bytes()
	if err != nil {
		return uuid.Nil
	}

	var entry apiKeyCacheEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return uuid.Nil
	}
	if entry.Revoked {
		return uuid.Nil
	}

	parsed, err := uuid.Parse(entry.OrgID)
	if err != nil {
		return uuid.Nil
	}
	return parsed
}

// extractOrgIDFromBearer 验证 Bearer JWT 并提取 org claim。
// 验证失败（无 token、签名错误、已过期）均返回 uuid.Nil。
func extractOrgIDFromBearer(r *http.Request, parser *jwt.Parser, secret []byte) uuid.UUID {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Bearer ") {
		return uuid.Nil
	}
	raw := strings.TrimPrefix(auth, "Bearer ")

	// 仅解码 payload 以提取 org claim；签名验证由 API 负责。
	// Gateway 这里验证签名是为了防止客户端伪造 org_id 逃脱限流。
	if len(secret) == 0 {
		return extractOrgClaimUnsafe(raw)
	}

	token, err := parser.Parse(raw, func(t *jwt.Token) (any, error) {
		return secret, nil
	})
	if err != nil || !token.Valid {
		return uuid.Nil
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return uuid.Nil
	}
	orgRaw, exists := claims["org"]
	if !exists {
		return uuid.Nil
	}
	orgStr, ok := orgRaw.(string)
	if !ok {
		return uuid.Nil
	}
	parsed, err := uuid.Parse(orgStr)
	if err != nil {
		return uuid.Nil
	}
	return parsed
}

// extractOrgClaimUnsafe 不验证签名，仅 base64 解码 payload 取 org claim。
// 仅在 jwtSecret 未配置时使用（降级模式）。
func extractOrgClaimUnsafe(raw string) uuid.UUID {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return uuid.Nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return uuid.Nil
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return uuid.Nil
	}
	orgRaw, ok := claims["org"]
	if !ok {
		return uuid.Nil
	}
	orgStr, ok := orgRaw.(string)
	if !ok {
		return uuid.Nil
	}
	parsed, err := uuid.Parse(orgStr)
	if err != nil {
		return uuid.Nil
	}
	return parsed
}

// isSSE 判断请求是否为 SSE 长连接。
func isSSE(r *http.Request) bool {
	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		return true
	}
	return strings.HasSuffix(r.URL.Path, "/events")
}

// clientIP 从 X-Forwarded-For 或 RemoteAddr 提取客户端 IP。
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first, _, ok := strings.Cut(xff, ","); ok {
			if ip := net.ParseIP(strings.TrimSpace(first)); ip != nil {
				return ip.String()
			}
		}
		if ip := net.ParseIP(strings.TrimSpace(xff)); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeRateLimitExceeded(w http.ResponseWriter, retryAfterSecs int64) {
	if retryAfterSecs > 0 {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfterSecs))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(`{"code":"ratelimit.exceeded","message":"rate limit exceeded"}`))
}
