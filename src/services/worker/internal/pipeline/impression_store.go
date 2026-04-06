package pipeline

import (
	"context"

	"github.com/google/uuid"
)

// ImpressionStore 抽象 user_impression_snapshots 读写，统一云端与桌面。
type ImpressionStore interface {
	Get(ctx context.Context, accountID, userID uuid.UUID, agentID string) (impression string, found bool, err error)
	Upsert(ctx context.Context, accountID, userID uuid.UUID, agentID, impression string) error
	AddScore(ctx context.Context, accountID, userID uuid.UUID, agentID string, delta int) (newScore int, err error)
	ResetScore(ctx context.Context, accountID, userID uuid.UUID, agentID string) error
}
