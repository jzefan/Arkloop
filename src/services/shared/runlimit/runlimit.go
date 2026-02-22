package runlimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const KeyPrefix = "arkloop:org:active_runs:"

const defaultTTL = 24 * time.Hour

// tryAcquireScript 原子 check-and-increment。
// KEYS[1] = active runs key
// ARGV[1] = max concurrent runs
// ARGV[2] = TTL seconds
// 返回 1 表示获取成功，0 表示已达上限。
var tryAcquireScript = redis.NewScript(`
local key = KEYS[1]
local max = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])
local cur = tonumber(redis.call("GET", key) or "0")
if cur >= max then
    return 0
end
redis.call("INCR", key)
if ttl > 0 then
    redis.call("EXPIRE", key, ttl)
end
return 1
`)

// releaseScript 原子 decrement，不低于 0。
// KEYS[1] = active runs key
// 返回 decrement 后的值。
var releaseScript = redis.NewScript(`
local key = KEYS[1]
local cur = tonumber(redis.call("GET", key) or "0")
if cur <= 0 then
    return 0
end
return redis.call("DECR", key)
`)

// TryAcquire 为 org 原子地获取一个并发 run 槽。
// Redis 不可用时 fail-open 返回 true。
func TryAcquire(ctx context.Context, rdb *redis.Client, key string, maxRuns int64) bool {
	if rdb == nil {
		return true
	}
	ttlSecs := int64(defaultTTL.Seconds())
	result, err := tryAcquireScript.Run(ctx, rdb, []string{key}, maxRuns, ttlSecs).Int64()
	if err != nil {
		return true
	}
	return result == 1
}

// Release 为 org 原子地释放一个并发 run 槽，计数不低于 0。
func Release(ctx context.Context, rdb *redis.Client, key string) {
	if rdb == nil {
		return
	}
	_ = releaseScript.Run(ctx, rdb, []string{key}).Err()
}

// Key 根据 orgID 字符串构建 Redis key。
func Key(orgID string) string {
	return KeyPrefix + orgID
}

// Set 直接设置 org 的活跃 run 计数（用于 SyncFromDB 修正漂移）。
func Set(ctx context.Context, rdb *redis.Client, key string, count int64) error {
	if rdb == nil {
		return nil
	}
	return rdb.Set(ctx, key, count, defaultTTL).Err()
}
