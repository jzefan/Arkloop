package pipeline

import (
	"context"
	"testing"
	"time"

	"arkloop/services/shared/pgnotify"
	"arkloop/services/worker/internal/testutil"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRunControlHubFanout(t *testing.T) {
	db := testutil.SetupPostgresDatabase(t, "arkloop_wg_run_control_hub")

	ctx, cancel := context.WithCancel(context.Background())

	poolCfg, err := pgxpool.ParseConfig(db.DSN)
	if err != nil {
		t.Fatalf("pgxpool.ParseConfig failed: %v", err)
	}
	poolCfg.MaxConns = 4

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		cancel()
		t.Fatalf("pgxpool.NewWithConfig failed: %v", err)
	}
	t.Cleanup(pool.Close)
	t.Cleanup(cancel)

	hub := NewRunControlHub()
	hub.Start(ctx, pool)

	runID := uuid.New()
	cancelCh, inputCh, unsubscribe := hub.Subscribe(runID)
	t.Cleanup(unsubscribe)

	if _, err := pool.Exec(ctx, "SELECT pg_notify($1, $2)", pgnotify.ChannelRunCancel, runID.String()); err != nil {
		t.Fatalf("notify cancel failed: %v", err)
	}
	select {
	case <-cancelCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected cancel signal received")
	}

	if _, err := pool.Exec(ctx, "SELECT pg_notify($1, $2)", pgnotify.ChannelRunInput, runID.String()); err != nil {
		t.Fatalf("notify input failed: %v", err)
	}
	select {
	case <-inputCh:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected input signal received")
	}
}
