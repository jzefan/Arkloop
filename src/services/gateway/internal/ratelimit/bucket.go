package ratelimit

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter 是限流器的抽象接口，方便测试时替换。
type Limiter interface {
	Consume(ctx context.Context, key string) (ConsumeResult, error)
}

// ConsumeResult 是单次消费的结果。
type ConsumeResult struct {
	Allowed        bool
	Remaining      int64
	RetryAfterSecs int64
}

// tokenBucketScript 是原子 token bucket 的 Lua 实现。
// KEYS[1] = bucket key
// ARGV[1] = capacity（float）
// ARGV[2] = rate（tokens/second，float）
// ARGV[3] = now（Unix timestamp，float）
// 返回：{allowed(0/1), remaining_floor, retry_after_secs}
var tokenBucketScript = redis.NewScript(`
local key      = KEYS[1]
local capacity = tonumber(ARGV[1])
local rate     = tonumber(ARGV[2])
local now      = tonumber(ARGV[3])

local data   = redis.call("HMGET", key, "tokens", "ts")
local tokens = tonumber(data[1]) or capacity
local ts     = tonumber(data[2]) or now

local elapsed = math.max(0.0, now - ts)
tokens = math.min(capacity, tokens + elapsed * rate)

local allowed = tokens >= 1.0
if allowed then
    tokens = tokens - 1.0
end

-- TTL：填满桶所需时间的两倍，保证自动清理
local ttl = math.ceil(capacity / rate) * 2 + 1
redis.call("HSET", key, "tokens", tostring(tokens), "ts", tostring(now))
redis.call("EXPIRE", key, ttl)

local retry_after = 0
if not allowed then
    retry_after = math.ceil((1.0 - tokens) / rate)
end

return {allowed and 1 or 0, math.floor(tokens), retry_after}
`)

// TokenBucket 是基于 Redis 的 token bucket 限流器。
type TokenBucket struct {
	rdb      *redis.Client
	capacity float64
	rate     float64        // tokens per second
	now      func() float64 // 返回 Unix 时间（秒，浮点），可注入以便测试
}

// NewTokenBucket 创建一个 token bucket。cfg.Capacity 和 cfg.RatePerSecond() 决定限流参数。
func NewTokenBucket(rdb *redis.Client, cfg Config) (*TokenBucket, error) {
	if rdb == nil {
		return nil, fmt.Errorf("redis client must not be nil")
	}
	if cfg.Capacity <= 0 {
		return nil, fmt.Errorf("capacity must be positive")
	}
	if cfg.RatePerMinute <= 0 {
		return nil, fmt.Errorf("rate_per_minute must be positive")
	}
	return &TokenBucket{
		rdb:      rdb,
		capacity: cfg.Capacity,
		rate:     cfg.RatePerSecond(),
		now:      func() float64 { return float64(time.Now().UnixNano()) / 1e9 },
	}, nil
}

// Consume 从指定 key 的 bucket 消耗一个 token。
func (b *TokenBucket) Consume(ctx context.Context, key string) (ConsumeResult, error) {
	now := b.now()

	result, err := tokenBucketScript.Run(ctx, b.rdb, []string{key},
		fmt.Sprintf("%g", b.capacity),
		fmt.Sprintf("%g", b.rate),
		fmt.Sprintf("%.6f", now),
	).Slice()
	if err != nil {
		return ConsumeResult{}, fmt.Errorf("token bucket script: %w", err)
	}
	if len(result) < 3 {
		return ConsumeResult{}, fmt.Errorf("unexpected script result length: %d", len(result))
	}

	allowedInt, _ := toInt64(result[0])
	remaining, _ := toInt64(result[1])
	retryAfter, _ := toInt64(result[2])

	return ConsumeResult{
		Allowed:        allowedInt == 1,
		Remaining:      remaining,
		RetryAfterSecs: max64(retryAfter, 0),
	}, nil
}

func toInt64(v any) (int64, bool) {
	switch val := v.(type) {
	case int64:
		return val, true
	case float64:
		return int64(math.Round(val)), true
	}
	return 0, false
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
