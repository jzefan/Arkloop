package ipfilter

import (
	"context"

	"arkloop/services/gateway/internal/identity"

	"github.com/redis/go-redis/v9"
)

// extractOrgIDWithRedis 委托给 identity 包提取 org_id。
func extractOrgIDWithRedis(authHeader string, rdb *redis.Client, ctx context.Context) string {
	return identity.ExtractOrgID(ctx, authHeader, rdb)
}
