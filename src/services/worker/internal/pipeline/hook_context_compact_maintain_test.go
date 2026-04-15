package pipeline

import (
	"context"
	"testing"
	"time"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/queue"
	"arkloop/services/worker/internal/routing"
	"arkloop/services/worker/internal/testutil"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type compactEnqueueCall struct {
	accountID uuid.UUID
	runID     uuid.UUID
	traceID   string
	jobType   string
	payload   map[string]any
}

type compactJobQueueSpy struct {
	calls []compactEnqueueCall
}

func (s *compactJobQueueSpy) EnqueueRun(ctx context.Context, accountID uuid.UUID, runID uuid.UUID, traceID string, queueJobType string, payload map[string]any, availableAt *time.Time) (uuid.UUID, error) {
	cloned := map[string]any{}
	for key, value := range payload {
		cloned[key] = value
	}
	s.calls = append(s.calls, compactEnqueueCall{
		accountID: accountID,
		runID:     runID,
		traceID:   traceID,
		jobType:   queueJobType,
		payload:   cloned,
	})
	return uuid.New(), nil
}

func (*compactJobQueueSpy) Lease(context.Context, int, []string) (*queue.JobLease, error) {
	return nil, nil
}
func (*compactJobQueueSpy) Heartbeat(context.Context, queue.JobLease, int) error { return nil }
func (*compactJobQueueSpy) Ack(context.Context, queue.JobLease) error            { return nil }
func (*compactJobQueueSpy) Nack(context.Context, queue.JobLease, *int) error     { return nil }
func (*compactJobQueueSpy) QueueDepth(context.Context, []string) (int, error)    { return 0, nil }

func TestContextCompactMaintenanceObserverEnqueuesJobAfterThreadPersist(t *testing.T) {
	db := testutil.SetupPostgresDatabase(t, "hook_context_compact_maintain")
	pool, err := pgxpool.New(context.Background(), db.DSN)
	if err != nil {
		t.Fatalf("pgxpool.New failed: %v", err)
	}
	t.Cleanup(pool.Close)

	accountID := uuid.New()
	threadID := uuid.New()
	runID := uuid.New()
	traceID := "0123456789abcdef0123456789abcdef"

	if _, err := pool.Exec(context.Background(), `
		INSERT INTO messages (id, account_id, thread_id, thread_seq, role, content, metadata_json, hidden)
		VALUES
			($1, $2, $3, 1, 'user', 'one', '{}'::jsonb, false),
			($4, $2, $3, 2, 'assistant', 'two', '{}'::jsonb, false),
			($5, $2, $3, 3, 'user', 'three', '{}'::jsonb, false)
	`, uuid.New(), accountID, threadID, uuid.New(), uuid.New()); err != nil {
		t.Fatalf("seed messages failed: %v", err)
	}

	jobQueue := &compactJobQueueSpy{}
	observer := NewContextCompactMaintenanceObserver(jobQueue)
	rc := &RunContext{
		Run: data.Run{
			ID:        runID,
			AccountID: accountID,
			ThreadID:  threadID,
		},
		Pool:               pool,
		TraceID:            traceID,
		ThreadPersistReady: true,
		ContextCompact: ContextCompactSettings{
			PersistEnabled:           true,
			PersistTriggerContextPct: 85,
			TargetContextPct:         65,
		},
		ContextWindowTokens: 64000,
		SelectedRoute: &routing.SelectedProviderRoute{
			Route: routing.ProviderRouteRule{
				ID:           "default",
				Model:        "stub",
				CredentialID: "stub_default",
			},
		},
	}

	if _, err := observer.AfterThreadPersist(context.Background(), rc, ThreadDelta{}, ThreadPersistResult{}); err != nil {
		t.Fatalf("AfterThreadPersist failed: %v", err)
	}
	if len(jobQueue.calls) != 1 {
		t.Fatalf("expected 1 enqueue call, got %d", len(jobQueue.calls))
	}
	call := jobQueue.calls[0]
	if call.jobType != queue.ContextCompactMaintainJobType {
		t.Fatalf("unexpected job type: %s", call.jobType)
	}
	if call.accountID != accountID || call.runID != runID || call.traceID != traceID {
		t.Fatalf("unexpected enqueue envelope: %+v", call)
	}
	if got := call.payload["thread_id"]; got != threadID.String() {
		t.Fatalf("unexpected thread_id payload: %#v", got)
	}
	if got := call.payload["upper_bound_thread_seq"]; got != int64(3) {
		t.Fatalf("unexpected upper_bound_thread_seq: %#v", got)
	}
	if got := call.payload["route_id"]; got != "default" {
		t.Fatalf("unexpected route_id: %#v", got)
	}
	if got := call.payload["context_window_tokens"]; got != 64000 {
		t.Fatalf("unexpected context_window_tokens: %#v", got)
	}
}
