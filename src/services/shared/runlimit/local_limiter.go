package runlimit

import (
	"context"
	"sync"
	"time"
)

type localEntry struct {
	count     int64
	expiresAt time.Time
}

// LocalConcurrencyLimiter 纯进程内并发限制器，不依赖 Redis。
type LocalConcurrencyLimiter struct {
	mu      sync.Mutex
	entries map[string]*localEntry
	now     func() time.Time
	ttl     time.Duration
}

// NewLocalConcurrencyLimiter 创建默认 TTL（24h）的本地限制器。
func NewLocalConcurrencyLimiter() *LocalConcurrencyLimiter {
	return &LocalConcurrencyLimiter{
		entries: make(map[string]*localEntry),
		now:     time.Now,
		ttl:     defaultTTL,
	}
}

// NewLocalConcurrencyLimiterWithTTL 创建自定义 TTL 的本地限制器（测试用）。
func NewLocalConcurrencyLimiterWithTTL(ttl time.Duration, clock func() time.Time) *LocalConcurrencyLimiter {
	if clock == nil {
		clock = time.Now
	}
	return &LocalConcurrencyLimiter{
		entries: make(map[string]*localEntry),
		now:     clock,
		ttl:     ttl,
	}
}

func (l *LocalConcurrencyLimiter) TryAcquire(_ context.Context, key string, maxRuns int64) bool {
	if maxRuns <= 0 {
		return false
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry := l.current(key)
	if entry.count >= maxRuns {
		return false
	}
	entry.count++
	entry.expiresAt = l.now().Add(l.ttl)
	l.entries[key] = entry
	return true
}

func (l *LocalConcurrencyLimiter) Release(_ context.Context, key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := l.current(key)
	if entry.count <= 1 {
		delete(l.entries, key)
		return
	}
	entry.count--
	entry.expiresAt = l.now().Add(l.ttl)
	l.entries[key] = entry
}

func (l *LocalConcurrencyLimiter) Set(_ context.Context, key string, count int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if count <= 0 {
		delete(l.entries, key)
		return nil
	}
	l.entries[key] = &localEntry{count: count, expiresAt: l.now().Add(l.ttl)}
	return nil
}

// current 返回 key 的当前条目，过期则视为零值。
func (l *LocalConcurrencyLimiter) current(key string) *localEntry {
	entry, ok := l.entries[key]
	if !ok {
		return &localEntry{}
	}
	if !entry.expiresAt.IsZero() && !entry.expiresAt.After(l.now()) {
		delete(l.entries, key)
		return &localEntry{}
	}
	return entry
}
