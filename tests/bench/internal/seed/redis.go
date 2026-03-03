package seed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const apiKeyCacheKeyPrefix = "arkloop:api_keys:"

type apiKeyCacheEntry struct {
	OrgID   string `json:"org_id"`
	Revoked bool   `json:"revoked"`
}

// SeedAPIKey 向 Redis 写入一条模拟的 API Key 缓存，格式与 identity 包一致。
func SeedAPIKey(ctx context.Context, rdb *redis.Client, rawKey string, orgID string) error {
	digest := sha256.Sum256([]byte(rawKey))
	redisKey := apiKeyCacheKeyPrefix + hex.EncodeToString(digest[:])

	entry := apiKeyCacheEntry{OrgID: orgID, Revoked: false}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal api key entry: %w", err)
	}

	// TTL 设长一些，benchmark 结束后会主动清理
	if err := rdb.Set(ctx, redisKey, data, 10*time.Minute).Err(); err != nil {
		return fmt.Errorf("seed api key: %w", err)
	}
	return nil
}

// CleanupAPIKey 删除 seed 写入的 API Key 缓存。
func CleanupAPIKey(ctx context.Context, rdb *redis.Client, rawKey string) {
	digest := sha256.Sum256([]byte(rawKey))
	redisKey := apiKeyCacheKeyPrefix + hex.EncodeToString(digest[:])
	_ = rdb.Del(ctx, redisKey)
}

// ConnectRedis 从 URL 创建 Redis 客户端并验证连通性。
func ConnectRedis(ctx context.Context, redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	}
	return client, nil
}
