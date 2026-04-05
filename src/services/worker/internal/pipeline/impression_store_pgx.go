//go:build !desktop

package pipeline

import (
	"context"

	"arkloop/services/worker/internal/data"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgxImpressionStore struct {
	pool *pgxpool.Pool
}

func NewPgxImpressionStore(pool *pgxpool.Pool) ImpressionStore {
	return pgxImpressionStore{pool: pool}
}

func (s pgxImpressionStore) Get(ctx context.Context, accountID, userID uuid.UUID, agentID string) (string, bool, error) {
	if s.pool == nil {
		return "", false, nil
	}
	return data.ImpressionRepository{}.Get(ctx, s.pool, accountID, userID, agentID)
}

func (s pgxImpressionStore) Upsert(ctx context.Context, accountID, userID uuid.UUID, agentID, impression string) error {
	if s.pool == nil {
		return nil
	}
	return data.ImpressionRepository{}.Upsert(ctx, s.pool, accountID, userID, agentID, impression)
}

func (s pgxImpressionStore) AddScore(ctx context.Context, accountID, userID uuid.UUID, agentID string, delta int) (int, error) {
	if s.pool == nil {
		return 0, nil
	}
	return data.ImpressionRepository{}.AddScore(ctx, s.pool, accountID, userID, agentID, delta)
}

func (s pgxImpressionStore) ResetScore(ctx context.Context, accountID, userID uuid.UUID, agentID string) error {
	if s.pool == nil {
		return nil
	}
	return data.ImpressionRepository{}.ResetScore(ctx, s.pool, accountID, userID, agentID)
}
