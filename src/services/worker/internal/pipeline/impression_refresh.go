package pipeline

import (
	"context"
	"encoding/json"
	"log/slog"

	"arkloop/services/shared/runkind"
	"arkloop/services/worker/internal/memory"

	"github.com/google/uuid"
)

// ImpressionRefreshFunc 创建 thread + run + 入队 job 以触发 impression 生成。
// 由 composition 层注入，适配 cloud / desktop 不同的基础设施。
type ImpressionRefreshFunc func(ctx context.Context, ident memory.MemoryIdentity, accountID uuid.UUID, userID uuid.UUID)

// ImpressionRefreshDeps 封装创建 impression run 所需的数据库和队列接口。
type ImpressionRefreshDeps struct {
	// ExecSQL 执行单条 SQL（INSERT/UPDATE），由 pool.Exec 或 db.Exec 包装。
	ExecSQL func(ctx context.Context, sql string, args ...any) error
	// QueryRowScan 执行查询并扫描单行结果。
	QueryRowScan func(ctx context.Context, sql string, args []any, dest ...any) error
	// EnqueueRun 入队一个 run.execute job。
	EnqueueRun func(ctx context.Context, accountID, runID uuid.UUID, traceID, jobType string, payload map[string]any) error
}

// NewImpressionRefreshFunc 构建一个通用的 ImpressionRefreshFunc。
func NewImpressionRefreshFunc(deps ImpressionRefreshDeps) ImpressionRefreshFunc {
	return func(ctx context.Context, ident memory.MemoryIdentity, accountID uuid.UUID, userID uuid.UUID) {
		go doImpressionRefresh(context.Background(), deps, ident, accountID, userID)
	}
}

func doImpressionRefresh(ctx context.Context, deps ImpressionRefreshDeps, ident memory.MemoryIdentity, accountID uuid.UUID, userID uuid.UUID) {
	threadID := uuid.New()
	runID := uuid.New()
	traceID := uuid.NewString()

	// 查找 default project
	var projectID uuid.UUID
	if err := deps.QueryRowScan(ctx,
		`SELECT id FROM projects WHERE account_id = $1 ORDER BY created_at ASC LIMIT 1`,
		[]any{accountID}, &projectID,
	); err != nil {
		slog.WarnContext(ctx, "impression: project lookup failed", "err", err.Error())
		return
	}

	// 创建 thread
	if err := deps.ExecSQL(ctx,
		`INSERT INTO threads (id, account_id, project_id, is_private) VALUES ($1, $2, $3, TRUE)`,
		threadID, accountID, projectID,
	); err != nil {
		slog.WarnContext(ctx, "impression: create thread failed", "err", err.Error())
		return
	}

	// 创建 run
	startedData := map[string]any{
		"run_kind":   runkind.Impression,
		"persona_id": "impression-builder",
	}
	startedJSON, _ := json.Marshal(startedData)
	if err := deps.ExecSQL(ctx,
		`INSERT INTO runs (id, account_id, thread_id, status, created_by_user_id) VALUES ($1, $2, $3, 'running', $4)`,
		runID, accountID, threadID, userID,
	); err != nil {
		slog.WarnContext(ctx, "impression: create run failed", "err", err.Error())
		return
	}

	if err := deps.ExecSQL(ctx,
		`INSERT INTO run_events (run_id, seq, type, data_json) VALUES ($1, 1, 'run.started', $2)`,
		runID, string(startedJSON),
	); err != nil {
		slog.WarnContext(ctx, "impression: create run event failed", "err", err.Error())
		return
	}

	// 入队 job
	payload := map[string]any{
		"source":   "impression_refresh",
		"run_kind": runkind.Impression,
	}
	if err := deps.EnqueueRun(ctx, accountID, runID, traceID, "run.execute", payload); err != nil {
		slog.WarnContext(ctx, "impression: enqueue job failed", "err", err.Error())
		return
	}

	slog.InfoContext(ctx, "impression: refresh triggered",
		"account_id", accountID.String(),
		"user_id", userID.String(),
		"run_id", runID.String(),
	)
}
