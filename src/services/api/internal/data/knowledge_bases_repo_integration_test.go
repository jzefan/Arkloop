//go:build !desktop

package data

import (
	"context"
	"testing"

	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"

	"github.com/google/uuid"
)

func setupKBRepo(t *testing.T) (*KnowledgeBasesRepository, *AccountRepository, context.Context) {
	t.Helper()
	db := testutil.SetupPostgresDatabase(t, "api_go_kb_bases")
	ctx := context.Background()
	if _, err := migrate.Up(ctx, db.DSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := NewPool(ctx, db.DSN, PoolLimits{MaxConns: 8})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	repo, err := NewKnowledgeBasesRepository(pool)
	if err != nil {
		t.Fatalf("kb repo: %v", err)
	}
	accountRepo, err := NewAccountRepository(pool)
	if err != nil {
		t.Fatalf("account repo: %v", err)
	}
	return repo, accountRepo, ctx
}

func TestKBCreateAndGet(t *testing.T) {
	repo, accounts, ctx := setupKBRepo(t)
	acc, err := accounts.Create(ctx, "alice", "Alice", "personal")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	kb, err := repo.Create(ctx, KBCreate{
		AccountID: acc.ID, WorkspaceRef: "ws-1", Name: "大学物理", Description: "test",
	})
	if err != nil {
		t.Fatalf("create kb: %v", err)
	}
	if kb.ID == uuid.Nil {
		t.Fatal("expected non-nil id")
	}
	got, err := repo.GetByID(ctx, kb.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v / %v", err, got)
	}
	if got.Name != "大学物理" || got.WorkspaceRef != "ws-1" {
		t.Errorf("mismatch: %+v", got)
	}
}

func TestKBListByWorkspace(t *testing.T) {
	repo, accounts, ctx := setupKBRepo(t)
	acc, _ := accounts.Create(ctx, "bob", "Bob", "personal")
	_, _ = repo.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "ws-a", Name: "k1"})
	_, _ = repo.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "ws-a", Name: "k2"})
	_, _ = repo.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "ws-b", Name: "k3"})

	got, err := repo.ListByWorkspace(ctx, acc.ID, "ws-a")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d, want 2", len(got))
	}
}

func TestKBUniqueNamePerWorkspace(t *testing.T) {
	repo, accounts, ctx := setupKBRepo(t)
	acc, _ := accounts.Create(ctx, "carol", "Carol", "personal")
	if _, err := repo.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "ws-x", Name: "dup"}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := repo.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "ws-x", Name: "dup"})
	if err == nil {
		t.Fatal("expected duplicate name error")
	}
}

func TestKBDelete(t *testing.T) {
	repo, accounts, ctx := setupKBRepo(t)
	acc, _ := accounts.Create(ctx, "dave", "Dave", "personal")
	kb, _ := repo.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "ws-1", Name: "to-delete"})
	if err := repo.Delete(ctx, kb.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ := repo.GetByID(ctx, kb.ID)
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}
