package data

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MemoryHitCache 是 memory.MemoryHit 的存储形式，避免 data 包依赖 memory 包。
type MemoryHitCache struct {
	URI         string  `json:"uri"`
	Abstract    string  `json:"abstract"`
	Score       float64 `json:"score"`
	MatchReason string  `json:"match_reason"`
	IsLeaf      bool    `json:"is_leaf"`
}

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

// GetHits 读取缓存的 raw hits JSON。未找到或列为空时返回 (nil, false, nil)。
func (MemorySnapshotRepository) GetHits(ctx context.Context, pool *pgxpool.Pool, orgID, userID uuid.UUID, agentID string) ([]MemoryHitCache, bool, error) {
	var raw []byte
	err := pool.QueryRow(ctx,
		`SELECT hits_json FROM user_memory_snapshots
		 WHERE org_id = $1 AND user_id = $2 AND agent_id = $3 AND hits_json IS NOT NULL`,
		orgID, userID, agentID,
	).Scan(&raw)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return nil, false, nil
		}
		return nil, false, err
	}
	var hits []MemoryHitCache
	if err := json.Unmarshal(raw, &hits); err != nil {
		return nil, false, nil
	}
	return hits, len(hits) > 0, nil
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

// UpsertWithHits 同时写入渲染后的 memory_block 和原始 hits JSON。
func (MemorySnapshotRepository) UpsertWithHits(ctx context.Context, pool *pgxpool.Pool, orgID, userID uuid.UUID, agentID, memoryBlock string, hits []MemoryHitCache) error {
	hitsJSON, err := json.Marshal(hits)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO user_memory_snapshots (org_id, user_id, agent_id, memory_block, hits_json, updated_at)
		 VALUES ($1, $2, $3, $4, $5, now())
		 ON CONFLICT (org_id, user_id, agent_id)
		 DO UPDATE SET memory_block = EXCLUDED.memory_block, hits_json = EXCLUDED.hits_json, updated_at = now()`,
		orgID, userID, agentID, memoryBlock, hitsJSON,
	)
	return err
}
