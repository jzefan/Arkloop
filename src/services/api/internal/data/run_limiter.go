package data

import (
	"context"
	"fmt"

	"arkloop/services/shared/runlimit"

	"github.com/google/uuid"
)

type RunLimiter struct {
	limiter runlimit.ConcurrencyLimiter
	maxRuns int64
}

func NewRunLimiter(limiter runlimit.ConcurrencyLimiter, maxRuns int64) (*RunLimiter, error) {
	if limiter == nil {
		return nil, fmt.Errorf("concurrency limiter must not be nil")
	}
	if maxRuns <= 0 {
		return nil, fmt.Errorf("max_runs must be positive")
	}
	return &RunLimiter{limiter: limiter, maxRuns: maxRuns}, nil
}

// TryAcquire 为 org 原子地获取一个并发 run 槽。
// Redis 不可用时 fail-open 返回 true。
func (l *RunLimiter) TryAcquire(ctx context.Context, orgID uuid.UUID) bool {
	key := runlimit.Key(orgID.String())
	return l.limiter.TryAcquire(ctx, key, l.maxRuns)
}

// Release 原子地释放 org 的一个并发 run 槽，计数不低于 0。
func (l *RunLimiter) Release(ctx context.Context, orgID uuid.UUID) {
	key := runlimit.Key(orgID.String())
	l.limiter.Release(ctx, key)
}

// SyncFromDB 从数据库查询 org 实际活跃 run 数量并重置 Redis 计数器。
func (l *RunLimiter) SyncFromDB(ctx context.Context, q Querier, orgID uuid.UUID) error {
	var count int64
	err := q.QueryRow(
		ctx,
		`SELECT COUNT(*) FROM runs WHERE org_id = $1 AND status = 'running'`,
		orgID,
	).Scan(&count)
	if err != nil {
		return err
	}
	key := runlimit.Key(orgID.String())
	return l.limiter.Set(ctx, key, count)
}
