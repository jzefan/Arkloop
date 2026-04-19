//go:build !desktop

package scheduled_job_manage

import (
	"context"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/tools"

	"github.com/jackc/pgx/v5/pgxpool"
)

type executor struct {
	common *executorCommon
}

// New 返回 scheduled_job_manage executor。
func New(pool *pgxpool.Pool) tools.Executor {
	return &executor{
		common: &executorCommon{
			db:   pool,
			repo: data.ScheduledJobsRepository{},
		},
	}
}

func (e *executor) Execute(
	ctx context.Context,
	toolName string,
	args map[string]any,
	execCtx tools.ExecutionContext,
	rawInput string,
) tools.ExecutionResult {
	return e.common.Execute(ctx, toolName, args, execCtx, rawInput)
}
