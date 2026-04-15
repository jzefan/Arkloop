package pipeline

import (
	"context"
	"log/slog"
	"strings"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/llm"
	"arkloop/services/worker/internal/queue"
	"arkloop/services/worker/internal/routing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type contextCompactMaintenanceObserver struct {
	jobQueue queue.JobQueue
}

func NewContextCompactMaintenanceObserver(jobQueue queue.JobQueue) AfterThreadPersistHook {
	if jobQueue == nil {
		return nil
	}
	return &contextCompactMaintenanceObserver{jobQueue: jobQueue}
}

func (o *contextCompactMaintenanceObserver) HookProviderName() string {
	return "context_compact_maintain"
}

func (o *contextCompactMaintenanceObserver) AfterThreadPersist(
	ctx context.Context,
	rc *RunContext,
	_ ThreadDelta,
	_ ThreadPersistResult,
) (PersistObservers, error) {
	if o == nil || o.jobQueue == nil || rc == nil || rc.Pool == nil {
		return nil, nil
	}
	if !rc.ThreadPersistReady || rc.ImpressionRun || isImpressionRun(rc) {
		return nil, nil
	}
	if !rc.ContextCompact.PersistEnabled || rc.Run.AccountID == uuid.Nil || rc.Run.ThreadID == uuid.Nil || rc.Run.ID == uuid.Nil {
		return nil, nil
	}

	upperBoundThreadSeq, err := queryThreadUpperBound(ctx, rc)
	if err != nil {
		slog.WarnContext(ctx, "context_compact_maintain_enqueue_failed",
			"run_id", rc.Run.ID.String(),
			"thread_id", rc.Run.ThreadID.String(),
			"error", err.Error(),
		)
		return nil, nil
	}
	if upperBoundThreadSeq <= 0 {
		return nil, nil
	}
	shouldEnqueue, err := shouldEnqueueContextCompactMaintenance(ctx, rc)
	if err != nil {
		slog.WarnContext(ctx, "context_compact_maintain_gate_failed",
			"run_id", rc.Run.ID.String(),
			"thread_id", rc.Run.ThreadID.String(),
			"error", err.Error(),
		)
		return nil, nil
	}
	if !shouldEnqueue {
		return nil, nil
	}

	payload := map[string]any{
		"thread_id":              rc.Run.ThreadID.String(),
		"upper_bound_thread_seq": upperBoundThreadSeq,
		"route_id":               compactMaintenanceRouteID(rc),
		"context_window_tokens":  compactMaintenanceContextWindow(rc),
		"trigger_context_pct":    rc.ContextCompact.PersistTriggerContextPct,
		"target_context_pct":     rc.ContextCompact.TargetContextPct,
	}
	if err := enqueueCompactMaintenanceJob(ctx, o.jobQueue, rc, payload); err != nil {
		slog.WarnContext(ctx, "context_compact_maintain_enqueue_failed",
			"run_id", rc.Run.ID.String(),
			"thread_id", rc.Run.ThreadID.String(),
			"error", err.Error(),
		)
	}
	return nil, nil
}

func shouldEnqueueContextCompactMaintenance(ctx context.Context, rc *RunContext) (bool, error) {
	if rc == nil || rc.Pool == nil {
		return false, nil
	}
	triggerTokens, _ := compactPersistTriggerTokens(rc.ContextCompact, compactMaintenanceContextWindow(rc))
	if triggerTokens <= 0 {
		return false, nil
	}
	tx, err := rc.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	canonical, err := buildCanonicalThreadContext(ctx, tx, rc.Run, data.MessagesRepository{}, nil, nil, 0)
	if err != nil {
		return false, err
	}
	var anchorPtr *ContextCompactPressureAnchor
	if anchor, ok := resolveContextCompactPressureAnchor(ctx, rc.Pool, rc); ok {
		anchorCopy := anchor
		anchorPtr = &anchorCopy
	}
	stats := ComputeContextCompactPressure(
		EstimateRequestContextTokens(rc, llm.Request{Messages: canonical.Messages}),
		anchorPtr,
	)
	return stats.ContextPressureTokens > triggerTokens, nil
}

func queryThreadUpperBound(ctx context.Context, rc *RunContext) (int64, error) {
	if rc == nil || rc.Pool == nil {
		return 0, nil
	}
	var threadSeq int64
	err := rc.Pool.QueryRow(
		ctx,
		`SELECT COALESCE(MAX(thread_seq), 0)
		   FROM messages
		  WHERE account_id = $1
		    AND thread_id = $2
		    AND deleted_at IS NULL`,
		rc.Run.AccountID,
		rc.Run.ThreadID,
	).Scan(&threadSeq)
	return threadSeq, err
}

func compactMaintenanceRouteID(rc *RunContext) string {
	if rc == nil {
		return ""
	}
	if rc.SelectedRoute != nil && strings.TrimSpace(rc.SelectedRoute.Route.ID) != "" {
		return strings.TrimSpace(rc.SelectedRoute.Route.ID)
	}
	if raw, ok := rc.InputJSON["route_id"].(string); ok {
		return strings.TrimSpace(raw)
	}
	return ""
}

func compactMaintenanceContextWindow(rc *RunContext) int {
	if rc == nil {
		return 0
	}
	if rc.ContextWindowTokens > 0 {
		return rc.ContextWindowTokens
	}
	if rc.SelectedRoute != nil {
		return routing.RouteContextWindowTokens(rc.SelectedRoute.Route)
	}
	return 0
}

func enqueueCompactMaintenanceJob(ctx context.Context, jobQueue queue.JobQueue, rc *RunContext, payload map[string]any) error {
	if jobQueue == nil || rc == nil {
		return nil
	}
	_, err := jobQueue.EnqueueRun(
		ctx,
		rc.Run.AccountID,
		rc.Run.ID,
		rc.TraceID,
		queue.ContextCompactMaintainJobType,
		payload,
		nil,
	)
	return err
}
