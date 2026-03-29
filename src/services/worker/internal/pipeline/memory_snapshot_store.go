package pipeline

import (
	"context"

	"arkloop/services/worker/internal/data"

	"github.com/google/uuid"
)

// MemorySnapshotStore 抽象 user_memory_snapshots 读写，统一云端与桌面。
type MemorySnapshotStore interface {
	Get(ctx context.Context, accountID, userID uuid.UUID, agentID string) (block string, found bool, err error)
	UpsertWithHits(ctx context.Context, accountID, userID uuid.UUID, agentID, memoryBlock string, hits []data.MemoryHitCache) error
}
