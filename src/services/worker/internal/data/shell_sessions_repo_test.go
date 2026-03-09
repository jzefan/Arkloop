package data

import (
	"context"
	"testing"

	"arkloop/services/worker/internal/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestShellSessionsRepository_UpsertAndGet(t *testing.T) {
	db := testutil.SetupPostgresDatabase(t, "worker_shell_sessions")
	pool, err := pgxpool.New(context.Background(), db.DSN)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	orgID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	threadID := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	runID := uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")
	repo := ShellSessionsRepository{}
	liveSessionID := "live-1"
	restoreRev := "restore-1"
	record := ShellSessionRecord{
		SessionRef:       "shref_test",
		OrgID:            orgID,
		ProfileRef:       "pref_test",
		WorkspaceRef:     "wsref_test",
		ThreadID:         &threadID,
		RunID:            &runID,
		ShareScope:       ShellShareScopeThread,
		State:            ShellSessionStateBusy,
		LiveSessionID:    &liveSessionID,
		LatestRestoreRev: &restoreRev,
		MetadataJSON:     map[string]any{"source": "test"},
	}
	if err := repo.Upsert(context.Background(), pool, record); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	stored, err := repo.GetBySessionRef(context.Background(), pool, orgID, "shref_test")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.SessionRef != record.SessionRef || stored.WorkspaceRef != record.WorkspaceRef {
		t.Fatalf("unexpected stored record: %#v", stored)
	}
	if stored.LiveSessionID == nil || *stored.LiveSessionID != liveSessionID {
		t.Fatalf("unexpected live_session_id: %#v", stored.LiveSessionID)
	}
	if stored.State != ShellSessionStateBusy {
		t.Fatalf("unexpected state: %s", stored.State)
	}
	if stored.LatestRestoreRev == nil || *stored.LatestRestoreRev != restoreRev {
		t.Fatalf("unexpected latest_restore_rev: %#v", stored.LatestRestoreRev)
	}
}

func TestShellSessionsRepository_UpdateRestoreRevision(t *testing.T) {
	db := testutil.SetupPostgresDatabase(t, "worker_shell_sessions_restore")
	pool, err := pgxpool.New(context.Background(), db.DSN)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	orgID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	repo := ShellSessionsRepository{}
	if err := repo.Upsert(context.Background(), pool, ShellSessionRecord{
		SessionRef:   "shref_test",
		OrgID:        orgID,
		ProfileRef:   "pref_test",
		WorkspaceRef: "wsref_test",
		ShareScope:   ShellShareScopeThread,
		State:        ShellSessionStateReady,
		MetadataJSON: map[string]any{},
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := repo.UpdateRestoreRevision(context.Background(), pool, orgID, "shref_test", "restore-2"); err != nil {
		t.Fatalf("update restore revision: %v", err)
	}

	stored, err := repo.GetBySessionRef(context.Background(), pool, orgID, "shref_test")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.LatestRestoreRev == nil || *stored.LatestRestoreRev != "restore-2" {
		t.Fatalf("unexpected latest_restore_rev: %#v", stored.LatestRestoreRev)
	}
}

func TestDefaultShellSessionBindingsRepository_UpsertAndGet(t *testing.T) {
	db := testutil.SetupPostgresDatabase(t, "worker_shell_session_bindings")
	pool, err := pgxpool.New(context.Background(), db.DSN)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()

	orgID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	repo := DefaultShellSessionBindingsRepository{}
	if err := repo.Upsert(context.Background(), pool, orgID, "pref_test", ShellBindingScopeThread, "thread-1", "shref_a"); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	ref, err := repo.Get(context.Background(), pool, orgID, "pref_test", ShellBindingScopeThread, "thread-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ref != "shref_a" {
		t.Fatalf("unexpected session_ref: %s", ref)
	}
}
