package config

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrCacheMiss = errors.New("cache miss")

type Cache interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
}

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(client *redis.Client) *RedisCache {
	if client == nil {
		return nil
	}
	return &RedisCache{client: client}
}

func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	if c == nil || c.client == nil {
		return nil, ErrCacheMiss
	}
	val, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, err
	}
	return val, nil
}

func (c *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *RedisCache) Del(ctx context.Context, keys ...string) error {
	if c == nil || c.client == nil || len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}

type MemoryCache struct {
	mu    sync.Mutex
	items map[string][]byte
}

func NewMemoryCache() *MemoryCache {
	return &MemoryCache{items: map[string][]byte{}}
}

func (c *MemoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	_ = ctx
	if c == nil {
		return nil, ErrCacheMiss
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	val, ok := c.items[key]
	if !ok {
		return nil, ErrCacheMiss
	}
	out := make([]byte, len(val))
	copy(out, val)
	return out, nil
}

func (c *MemoryCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	_ = ctx
	_ = ttl
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cp := make([]byte, len(value))
	copy(cp, value)
	c.items[key] = cp
	return nil
}

func (c *MemoryCache) Del(ctx context.Context, keys ...string) error {
	_ = ctx
	if c == nil || len(keys) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		delete(c.items, key)
	}
	return nil
}
