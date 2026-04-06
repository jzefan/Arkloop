//go:build !desktop

package data

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ImpressionRepository struct{}

func (ImpressionRepository) Get(ctx context.Context, pool *pgxpool.Pool, accountID, userID uuid.UUID, agentID string) (string, bool, error) {
	var impression string
	err := pool.QueryRow(ctx,
		`SELECT impression FROM user_impression_snapshots
		 WHERE account_id = $1 AND user_id = $2 AND agent_id = $3`,
		accountID, userID, agentID,
	).Scan(&impression)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return impression, true, nil
}

func (ImpressionRepository) Upsert(ctx context.Context, pool *pgxpool.Pool, accountID, userID uuid.UUID, agentID, impression string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO user_impression_snapshots (account_id, user_id, agent_id, impression, updated_at)
		 VALUES ($1, $2, $3, $4, now())
		 ON CONFLICT (account_id, user_id, agent_id)
		 DO UPDATE SET impression = EXCLUDED.impression, updated_at = now()`,
		accountID, userID, agentID, impression,
	)
	return err
}

func (ImpressionRepository) AddScore(ctx context.Context, pool *pgxpool.Pool, accountID, userID uuid.UUID, agentID string, delta int) (int, error) {
	var newScore int
	err := pool.QueryRow(ctx,
		`INSERT INTO user_impression_snapshots (account_id, user_id, agent_id, impression_score, updated_at)
		 VALUES ($1, $2, $3, $4, now())
		 ON CONFLICT (account_id, user_id, agent_id)
		 DO UPDATE SET impression_score = user_impression_snapshots.impression_score + $4, updated_at = now()
		 RETURNING impression_score`,
		accountID, userID, agentID, delta,
	).Scan(&newScore)
	return newScore, err
}

func (ImpressionRepository) ResetScore(ctx context.Context, pool *pgxpool.Pool, accountID, userID uuid.UUID, agentID string) error {
	_, err := pool.Exec(ctx,
		`UPDATE user_impression_snapshots
		 SET impression_score = 0, updated_at = now()
		 WHERE account_id = $1 AND user_id = $2 AND agent_id = $3`,
		accountID, userID, agentID,
	)
	return err
}
