package runlimit

import "context"

// ConcurrencyLimiter manages concurrent run slots.
// Implementations include RedisConcurrencyLimiter (distributed) and
// LocalConcurrencyLimiter (in-process, for Desktop mode).
type ConcurrencyLimiter interface {
	// TryAcquire atomically acquires a concurrent run slot for the given key.
	// Returns true if the slot was acquired, false if the limit is reached.
	TryAcquire(ctx context.Context, key string, maxRuns int64) bool

	// Release atomically releases a concurrent run slot. The count never goes below 0.
	Release(ctx context.Context, key string)

	// Set directly sets the active run count for a key (used for drift correction via SyncFromDB).
	Set(ctx context.Context, key string, count int64) error
}
