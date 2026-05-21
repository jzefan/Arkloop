//go:build !desktop

package data

import (
	"context"
	"testing"

	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"
	"arkloop/services/shared/runkind"

	"github.com/google/uuid"
)

func setupRunsTestRepo(t *testing.T) (*RunEventRepository, *AccountRepository, context.Context) {
	t.Helper()

	db := testutil.SetupPostgresDatabase(t, "api_go_runs")
	ctx := context.Background()

	if _, err := migrate.Up(ctx, db.DSN); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	pool, err := NewPool(ctx, db.DSN, PoolLimits{MaxConns: 32, MinConns: 0})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(pool.Close)

	runRepo, err := NewRunEventRepository(pool)
	if err != nil {
		t.Fatalf("new run repo: %v", err)
	}

	orgRepo, err := NewAccountRepository(pool)
	if err != nil {
		t.Fatalf("new org repo: %v", err)
	}

	return runRepo, orgRepo, ctx
}

func TestListRunsAggregatesJoinedUsageAndCredits(t *testing.T) {
	repo, orgRepo, ctx := setupRunsTestRepo(t)

	org, err := orgRepo.Create(ctx, "runs-join-test", "Runs Join Test Org", "personal")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	threadID := uuid.New()
	_, err = repo.db.Exec(ctx,
		`INSERT INTO threads (id, account_id, title)
		 VALUES ($1, $2, $3)`,
		threadID, org.ID, "runs-join-test",
	)
	if err != nil {
		t.Fatalf("insert thread: %v", err)
	}

	runID := uuid.New()
	_, err = repo.db.Exec(ctx,
		`INSERT INTO runs (id, account_id, thread_id, total_input_tokens, total_output_tokens, total_cost_usd)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		runID, org.ID, threadID, 120, 60, 0.12,
	)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}

	_, err = repo.db.Exec(ctx,
		`INSERT INTO usage_records (account_id, run_id, usage_type, cache_read_tokens, cache_creation_tokens, cached_tokens)
		 VALUES ($1, $2, 'llm', $3, $4, $5),
		        ($1, $2, 'embedding', $6, $7, $8)`,
		org.ID, runID, 10, 20, 30, 1, 2, 3,
	)
	if err != nil {
		t.Fatalf("insert usage records: %v", err)
	}

	_, err = repo.db.Exec(ctx,
		`INSERT INTO credit_transactions (account_id, amount, type, reference_type, reference_id)
		 VALUES ($1, $2, 'consumption', 'run', $3),
		        ($1, $4, 'consumption', 'run', $3)`,
		org.ID, -5, runID, -7,
	)
	if err != nil {
		t.Fatalf("insert credit transactions: %v", err)
	}

	runs, total, err := repo.ListRuns(ctx, ListRunsParams{RunID: &runID, Limit: 10})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}

	if total != 1 {
		t.Fatalf("expected total 1, got %d", total)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run row, got %d", len(runs))
	}

	row := runs[0]
	if row.CacheReadTokens == nil || *row.CacheReadTokens != 11 {
		t.Fatalf("expected cache_read_tokens 11, got %+v", row.CacheReadTokens)
	}
	if row.CacheCreationTokens == nil || *row.CacheCreationTokens != 22 {
		t.Fatalf("expected cache_creation_tokens 22, got %+v", row.CacheCreationTokens)
	}
	if row.CachedTokens == nil || *row.CachedTokens != 33 {
		t.Fatalf("expected cached_tokens 33, got %+v", row.CachedTokens)
	}
	if row.CreditsUsed == nil || *row.CreditsUsed != 12 {
		t.Fatalf("expected credits_used 12, got %+v", row.CreditsUsed)
	}
}

// 回归测试：subagent_callback 类型的 active root run 必须被 user-initiated run 顶掉，
// 永不返回 ErrThreadBusy。这是 thread_busy 用户报障的核心修复路径。
func TestCreateRootRun_SupersedesSubagentCallback(t *testing.T) {
	repo, accountRepo, ctx := setupRunsTestRepo(t)

	account, err := accountRepo.Create(ctx, "callback-supersede", "Callback Supersede Test", "personal")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	userID := uuid.New()
	threadID := uuid.New()
	if _, err := repo.db.Exec(ctx,
		`INSERT INTO threads (id, account_id, title) VALUES ($1, $2, 'callback-supersede')`,
		threadID, account.ID,
	); err != nil {
		t.Fatalf("insert thread: %v", err)
	}

	// 模拟 projector 留下的 callback root run + 对应的 pending 回调记录。
	callbackRunID := uuid.New()
	subAgentID := uuid.New()
	sourceRunID := uuid.New()
	if _, err := repo.db.Exec(ctx,
		`INSERT INTO runs (id, account_id, thread_id, status) VALUES ($1, $2, $3, 'running')`,
		callbackRunID, account.ID, threadID,
	); err != nil {
		t.Fatalf("insert callback run: %v", err)
	}
	if _, err := repo.db.Exec(ctx,
		`INSERT INTO run_events (run_id, seq, type, data_json)
		 VALUES ($1, 1, 'run.started', $2::jsonb)`,
		callbackRunID,
		`{"run_kind":"`+runkind.SubagentCallback+`","callback_id":"`+uuid.NewString()+`"}`,
	); err != nil {
		t.Fatalf("insert run.started: %v", err)
	}
	// 准备一个 source run + sub_agent，以满足 thread_subagent_callbacks 的 FK。
	if _, err := repo.db.Exec(ctx,
		`INSERT INTO runs (id, account_id, thread_id, status) VALUES ($1, $2, $3, 'completed')`,
		sourceRunID, account.ID, threadID,
	); err != nil {
		t.Fatalf("insert source run: %v", err)
	}
	if _, err := repo.db.Exec(ctx,
		`INSERT INTO sub_agents (id, account_id, origin_run_id, owner_thread_id, agent_thread_id, depth, source_type, context_mode, status)
		 VALUES ($1, $2, $3, $4, $5, 1, 'thread_spawn', 'isolated', 'completed')`,
		subAgentID, account.ID, sourceRunID, threadID, uuid.New(),
	); err != nil {
		t.Fatalf("insert sub_agent: %v", err)
	}
	callbackID := uuid.New()
	if _, err := repo.db.Exec(ctx,
		`INSERT INTO thread_subagent_callbacks (id, account_id, thread_id, sub_agent_id, source_run_id, status)
		 VALUES ($1, $2, $3, $4, $5, 'completed')`,
		callbackID, account.ID, threadID, subAgentID, sourceRunID,
	); err != nil {
		t.Fatalf("insert callback record: %v", err)
	}

	// 用户发起新 run —— 不能再返回 thread_busy。
	newRun, _, err := repo.CreateRootRunWithClaim(ctx, account.ID, threadID, &userID, "run.started", map[string]any{})
	if err != nil {
		t.Fatalf("expected callback to be superseded, got: %v", err)
	}
	if newRun.ID == uuid.Nil {
		t.Fatalf("expected new run to be created")
	}

	// 老的 callback run 必须被强制终态化。
	var oldStatus string
	if err := repo.db.QueryRow(ctx, `SELECT status FROM runs WHERE id = $1`, callbackRunID).Scan(&oldStatus); err != nil {
		t.Fatalf("query old run status: %v", err)
	}
	if oldStatus != "cancelled" {
		t.Fatalf("expected old callback run cancelled, got %s", oldStatus)
	}

	var terminalCount int
	if err := repo.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM run_events WHERE run_id = $1 AND type = 'run.cancelled'`,
		callbackRunID,
	).Scan(&terminalCount); err != nil {
		t.Fatalf("query terminal event count: %v", err)
	}
	if terminalCount != 1 {
		t.Fatalf("expected exactly 1 run.cancelled event, got %d", terminalCount)
	}

	// pending callback 必须一并被标记 consumed，避免 worker 重复唤起。
	var pending int
	if err := repo.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM thread_subagent_callbacks WHERE thread_id = $1 AND consumed_at IS NULL`,
		threadID,
	).Scan(&pending); err != nil {
		t.Fatalf("query pending callbacks: %v", err)
	}
	if pending != 0 {
		t.Fatalf("expected all callbacks consumed, %d still pending", pending)
	}
}

// 回归测试：normal vs normal 仍然必须返回 ErrThreadBusy（确保没有把正常并发也放过）。
func TestCreateRootRun_NormalVsNormalStillRejected(t *testing.T) {
	repo, accountRepo, ctx := setupRunsTestRepo(t)

	account, err := accountRepo.Create(ctx, "normal-conflict", "Normal Conflict Test", "personal")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	userID := uuid.New()
	threadID := uuid.New()
	if _, err := repo.db.Exec(ctx,
		`INSERT INTO threads (id, account_id, title) VALUES ($1, $2, 'normal-conflict')`,
		threadID, account.ID,
	); err != nil {
		t.Fatalf("insert thread: %v", err)
	}

	// 先放一条普通 running run（没有 run_kind = subagent_callback）。
	existingRunID := uuid.New()
	if _, err := repo.db.Exec(ctx,
		`INSERT INTO runs (id, account_id, thread_id, status) VALUES ($1, $2, $3, 'running')`,
		existingRunID, account.ID, threadID,
	); err != nil {
		t.Fatalf("insert existing run: %v", err)
	}
	if _, err := repo.db.Exec(ctx,
		`INSERT INTO run_events (run_id, seq, type, data_json)
		 VALUES ($1, 1, 'run.started', '{}'::jsonb)`,
		existingRunID,
	); err != nil {
		t.Fatalf("insert run.started: %v", err)
	}

	_, _, err = repo.CreateRootRunWithClaim(ctx, account.ID, threadID, &userID, "run.started", map[string]any{})
	if err != ErrThreadBusy {
		t.Fatalf("expected ErrThreadBusy, got: %v", err)
	}
}
