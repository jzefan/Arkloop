//go:build !desktop

package pipeline

import (
	"context"

	"arkloop/services/worker/internal/data"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgxMemorySnapshotStore struct {
	pool *pgxpool.Pool
}

// NewPgxMemorySnapshotStore 基于 Postgres 池；pool 为 nil 时 Get 恒未命中、Upsert 无操作。
func NewPgxMemorySnapshotStore(pool *pgxpool.Pool) MemorySnapshotStore {
	return pgxMemorySnapshotStore{pool: pool}
}

func (s pgxMemorySnapshotStore) Get(ctx context.Context, accountID, userID uuid.UUID, agentID string) (string, bool, error) {
	if s.pool == nil {
		return "", false, nil
	}
	return data.MemorySnapshotRepository{}.Get(ctx, s.pool, accountID, userID, agentID)
}

func (s pgxMemorySnapshotStore) UpsertWithHits(ctx context.Context, accountID, userID uuid.UUID, agentID, memoryBlock string, hits []data.MemoryHitCache) error {
	if s.pool == nil {
		return nil
	}
	return data.MemorySnapshotRepository{}.UpsertWithHits(ctx, s.pool, accountID, userID, agentID, memoryBlock, hits)
}
