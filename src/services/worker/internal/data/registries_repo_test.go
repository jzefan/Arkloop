package data

import (
	"context"
	"testing"
	"time"

	"arkloop/services/worker/internal/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProfileRegistriesRepository_GetOrCreateAndTransitions(t *testing.T) {
	db := testutil.SetupPostgresDatabase(t, "worker_profile_registries")
	pool, err := pgxpool.New(context.Background(), db.DSN)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	orgID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	repo := ProfileRegistriesRepository{}
	record, err := repo.GetOrCreate(context.Background(), pool, RegistryRecord{
		Ref:          "pref_test",
		OrgID:        orgID,
		MetadataJSON: map[string]any{"source": "test"},
	})
	if err != nil {
		t.Fatalf("get or create: %v", err)
	}
	if record.Ref != "pref_test" || record.FlushState != FlushStateIdle {
		t.Fatalf("unexpected record: %#v", record)
	}

	record2, err := repo.GetOrCreate(context.Background(), pool, RegistryRecord{Ref: "pref_test", OrgID: orgID})
	if err != nil {
		t.Fatalf("get or create twice: %v", err)
	}
	if !record.CreatedAt.Equal(record2.CreatedAt) {
		t.Fatalf("expected idempotent create, got %v and %v", record.CreatedAt, record2.CreatedAt)
	}

	if err := repo.MarkFlushPending(context.Background(), pool, "pref_test"); err != nil {
		t.Fatalf("mark pending: %v", err)
	}
	if err := repo.MarkFlushRunning(context.Background(), pool, "pref_test"); err != nil {
		t.Fatalf("mark running: %v", err)
	}
	failedAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.MarkFlushFailed(context.Background(), pool, "pref_test", failedAt); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	stored, err := repo.Get(context.Background(), pool, "pref_test")
	if err != nil {
		t.Fatalf("get after fail: %v", err)
	}
	if stored.FlushState != FlushStateFailed || stored.FlushRetryCount != 1 || stored.LastFlushFailedAt == nil {
		t.Fatalf("unexpected failed record: %#v", stored)
	}

	succeededAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.MarkFlushSucceeded(context.Background(), pool, "pref_test", "rev-1", succeededAt); err != nil {
		t.Fatalf("mark succeeded: %v", err)
	}
	stored, err = repo.Get(context.Background(), pool, "pref_test")
	if err != nil {
		t.Fatalf("get after success: %v", err)
	}
	if stored.FlushState != FlushStateIdle || stored.FlushRetryCount != 0 {
		t.Fatalf("unexpected success state: %#v", stored)
	}
	if stored.LatestManifestRev == nil || *stored.LatestManifestRev != "rev-1" {
		t.Fatalf("unexpected latest manifest: %#v", stored.LatestManifestRev)
	}
	if stored.LastFlushSucceededAt == nil {
		t.Fatalf("expected success timestamp")
	}
}

func TestWorkspaceRegistriesRepository_GetOrCreate(t *testing.T) {
	db := testutil.SetupPostgresDatabase(t, "worker_workspace_registries")
	pool, err := pgxpool.New(context.Background(), db.DSN)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	orgID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	repo := WorkspaceRegistriesRepository{}
	record, err := repo.GetOrCreate(context.Background(), pool, RegistryRecord{Ref: "wsref_test", OrgID: orgID})
	if err != nil {
		t.Fatalf("get or create: %v", err)
	}
	if record.Ref != "wsref_test" || record.OrgID != orgID {
		t.Fatalf("unexpected record: %#v", record)
	}
}
