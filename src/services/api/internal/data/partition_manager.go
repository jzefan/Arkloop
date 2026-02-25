package data

import (
	"context"
	"fmt"
	"time"

	"arkloop/services/api/internal/observability"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PartitionManager 定期检查并创建 run_events 的月分区。
type PartitionManager struct {
	pool     *pgxpool.Pool
	logger   *observability.JSONLogger
	interval time.Duration
}

func NewPartitionManager(pool *pgxpool.Pool, logger *observability.JSONLogger) *PartitionManager {
	return &PartitionManager{
		pool:     pool,
		logger:   logger,
		interval: 24 * time.Hour,
	}
}

// Run 启动后台循环，定期执行 EnsurePartitions。ctx 取消时退出。
func (pm *PartitionManager) Run(ctx context.Context) {
	pm.ensure(ctx)

	ticker := time.NewTicker(pm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pm.ensure(ctx)
		}
	}
}

func (pm *PartitionManager) ensure(ctx context.Context) {
	if err := pm.EnsurePartitions(ctx); err != nil {
		pm.logger.Error("partition check failed", observability.LogFields{}, map[string]any{"error": err.Error()})
	}
}

// EnsurePartitions 检查并创建当前月、下月、下下月的分区（幂等）。
func (pm *PartitionManager) EnsurePartitions(ctx context.Context) error {
	now := time.Now().UTC()
	base := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		start := base.AddDate(0, i, 0)
		end := base.AddDate(0, i+1, 0)
		name := fmt.Sprintf("run_events_p%s", start.Format("2006_01"))

		if err := pm.createPartitionIfNotExists(ctx, name, start, end); err != nil {
			return fmt.Errorf("partition %s: %w", name, err)
		}
	}
	return nil
}

func (pm *PartitionManager) createPartitionIfNotExists(
	ctx context.Context,
	name string,
	start, end time.Time,
) error {
	ddl := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS "%s" PARTITION OF run_events FOR VALUES FROM ('%s') TO ('%s')`,
		name,
		start.Format("2006-01-02"),
		end.Format("2006-01-02"),
	)
	tag, err := pm.pool.Exec(ctx, ddl)
	if err != nil {
		return err
	}
	// CommandTag 为 "CREATE TABLE" 时表示实际创建了新分区
	if tag.String() == "CREATE TABLE" {
		pm.logger.Info("partition created", observability.LogFields{}, map[string]any{"partition": name})
	}
	return nil
}
