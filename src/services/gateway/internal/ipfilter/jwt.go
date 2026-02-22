package ipfilter

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

// extractOrgID 从 Bearer token 中解析 org_id。
// JWT token：解析 payload 中的 org claim（不做签名验证）。
// API Key（ak- 前缀）：从 Redis 缓存中查取。
// 返回 org_id 字符串；获取失败时返回空字符串（fail-open）。
func extractOrgID(authHeader string) string {
	return extractOrgIDWithRedis(authHeader, nil, context.Background())
}

func extractOrgIDWithRedis(authHeader string, rdb *redis.Client, ctx context.Context) string {
	token, ok := strings.CutPrefix(authHeader, "Bearer ")
	if !ok || token == "" {
		return ""
	}

	if strings.HasPrefix(token, "ak-") {
		return extractOrgIDFromAPIKeyCache(ctx, token, rdb)
	}

	return extractOrgIDFromJWT(token)
}

func extractOrgIDFromJWT(token string) string {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return ""
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}

	var claims struct {
		Org string `json:"org"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}

	return strings.TrimSpace(claims.Org)
}

type ipFilterAPICacheEntry struct {
	OrgID   string `json:"org_id"`
	Revoked bool   `json:"revoked"`
}

func extractOrgIDFromAPIKeyCache(ctx context.Context, rawKey string, rdb *redis.Client) string {
	if rdb == nil {
		return ""
	}

	digest := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(digest[:])
	redisKey := fmt.Sprintf("arkloop:api_keys:%s", keyHash)

	raw, err := rdb.Get(ctx, redisKey).Bytes()
	if err != nil {
		return ""
	}

	var entry ipFilterAPICacheEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return ""
	}
	if entry.Revoked {
		return ""
	}

	return strings.TrimSpace(entry.OrgID)
}
