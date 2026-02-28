package data

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MemorySnapshotRepository struct{}

// Get 读取用户记忆快照。未找到时返回 ("", false, nil)。
func (MemorySnapshotRepository) Get(ctx context.Context, pool *pgxpool.Pool, orgID, userID uuid.UUID, agentID string) (string, bool, error) {
	var block string
	err := pool.QueryRow(ctx,
		`SELECT memory_block FROM user_memory_snapshots
		 WHERE org_id = $1 AND user_id = $2 AND agent_id = $3`,
		orgID, userID, agentID,
	).Scan(&block)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return "", false, nil
		}
		return "", false, err
	}
	return block, true, nil
}

// Upsert 写入或覆盖用户记忆快照。
func (MemorySnapshotRepository) Upsert(ctx context.Context, pool *pgxpool.Pool, orgID, userID uuid.UUID, agentID, memoryBlock string) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO user_memory_snapshots (org_id, user_id, agent_id, memory_block, updated_at)
		 VALUES ($1, $2, $3, $4, now())
		 ON CONFLICT (org_id, user_id, agent_id)
		 DO UPDATE SET memory_block = EXCLUDED.memory_block, updated_at = now()`,
		orgID, userID, agentID, memoryBlock,
	)
	return err
}
