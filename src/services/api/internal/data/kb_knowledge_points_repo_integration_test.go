//go:build !desktop

package data

import (
	"context"
	"testing"

	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"

	"github.com/google/uuid"
)

func setupKPRepo(t *testing.T) (*KBKnowledgePointsRepository, *KBDocumentsRepository, *KnowledgeBasesRepository, *AccountRepository, context.Context) {
	t.Helper()
	db := testutil.SetupPostgresDatabase(t, "api_go_kb_kp")
	ctx := context.Background()
	if _, err := migrate.Up(ctx, db.DSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := NewPool(ctx, db.DSN, PoolLimits{MaxConns: 8})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	kp, err := NewKBKnowledgePointsRepository(pool)
	if err != nil {
		t.Fatalf("kp repo: %v", err)
	}
	docs, _ := NewKBDocumentsRepository(pool)
	kbs, _ := NewKnowledgeBasesRepository(pool)
	accts, _ := NewAccountRepository(pool)
	return kp, docs, kbs, accts, ctx
}

func TestKBKnowledgePointsCreateAndListOrdered(t *testing.T) {
	kp, _, kbs, accts, ctx := setupKPRepo(t)
	acc, _ := accts.Create(ctx, "kp-owner", "KP Owner", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "ws-kp", Name: "kp-kb"})

	// Insert in non-sorted order; expect ListByKB to return by sort_order ASC.
	b, err := kp.Create(ctx, KBKnowledgePointCreate{KBID: kb.ID, Name: "B", SortOrder: 20})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}
	a, err := kp.Create(ctx, KBKnowledgePointCreate{KBID: kb.ID, Name: "A", SortOrder: 10})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	c, err := kp.Create(ctx, KBKnowledgePointCreate{KBID: kb.ID, Name: "C", SortOrder: 30})
	if err != nil {
		t.Fatalf("create C: %v", err)
	}

	got, err := kp.GetByID(ctx, a.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v / %v", err, got)
	}
	if got.Name != "A" || got.SortOrder != 10 {
		t.Errorf("got %+v", got)
	}

	list, err := kp.ListByKB(ctx, kb.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}
	if list[0].ID != a.ID || list[1].ID != b.ID || list[2].ID != c.ID {
		t.Errorf("order = %s,%s,%s; want %s,%s,%s",
			list[0].Name, list[1].Name, list[2].Name, a.Name, b.Name, c.Name)
	}
}

func TestKBKnowledgePointsAssociateAndListByDocument(t *testing.T) {
	kp, docs, kbs, accts, ctx := setupKPRepo(t)
	acc, _ := accts.Create(ctx, "assoc-owner", "Assoc Owner", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "ws-assoc", Name: "assoc-kb"})

	doc, err := docs.Create(ctx, DocCreate{KBID: kb.ID, OriginalFilename: "x.pdf", MimeType: "application/pdf", BlobSHA256: "sha-1", SizeBytes: 1})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}
	k1, _ := kp.Create(ctx, KBKnowledgePointCreate{KBID: kb.ID, Name: "K1", SortOrder: 1})
	k2, _ := kp.Create(ctx, KBKnowledgePointCreate{KBID: kb.ID, Name: "K2", SortOrder: 2})

	if err := kp.AssociateDocument(ctx, kb.ID, doc.ID, k1.ID); err != nil {
		t.Fatalf("associate k1: %v", err)
	}
	if err := kp.AssociateDocument(ctx, kb.ID, doc.ID, k2.ID); err != nil {
		t.Fatalf("associate k2: %v", err)
	}
	// Idempotency: associating again must not error or duplicate.
	if err := kp.AssociateDocument(ctx, kb.ID, doc.ID, k1.ID); err != nil {
		t.Fatalf("re-associate k1: %v", err)
	}

	list, err := kp.ListByDocument(ctx, doc.ID)
	if err != nil {
		t.Fatalf("list by doc: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	if list[0].ID != k1.ID || list[1].ID != k2.ID {
		t.Errorf("order = %s,%s; want %s,%s", list[0].Name, list[1].Name, k1.Name, k2.Name)
	}
}

func TestKBKnowledgePointsCascadeOnKBDelete(t *testing.T) {
	kp, docs, kbs, accts, ctx := setupKPRepo(t)
	acc, _ := accts.Create(ctx, "cascade-owner", "Cascade Owner", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "ws-cascade", Name: "cascade-kb"})

	doc, _ := docs.Create(ctx, DocCreate{KBID: kb.ID, OriginalFilename: "x", MimeType: "text/plain", BlobSHA256: "s", SizeBytes: 1})
	point, _ := kp.Create(ctx, KBKnowledgePointCreate{KBID: kb.ID, Name: "doomed"})
	if err := kp.AssociateDocument(ctx, kb.ID, doc.ID, point.ID); err != nil {
		t.Fatalf("associate: %v", err)
	}

	if err := kbs.Delete(ctx, kb.ID); err != nil {
		t.Fatalf("delete kb: %v", err)
	}

	got, err := kp.GetByID(ctx, point.ID)
	if err != nil {
		t.Fatalf("get after cascade: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after KB cascade, got %+v", got)
	}
	// And the association table should also be empty for this doc.
	listed, err := kp.ListByDocument(ctx, doc.ID)
	if err != nil {
		t.Fatalf("list after cascade: %v", err)
	}
	if len(listed) != 0 {
		t.Errorf("expected 0 associations, got %d", len(listed))
	}
}

func TestKBKnowledgePointsDelete(t *testing.T) {
	kp, _, kbs, accts, ctx := setupKPRepo(t)
	acc, _ := accts.Create(ctx, "del-owner", "Del Owner", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "ws-del", Name: "del-kb"})
	point, _ := kp.Create(ctx, KBKnowledgePointCreate{KBID: kb.ID, Name: "to-delete"})
	if err := kp.Delete(ctx, point.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// Double delete returns ErrKBKnowledgePointNotFound.
	if err := kp.Delete(ctx, point.ID); err != ErrKBKnowledgePointNotFound {
		t.Errorf("expected ErrKBKnowledgePointNotFound, got %v", err)
	}
	// Sanity: deleting an unknown id also returns the sentinel.
	if err := kp.Delete(ctx, uuid.New()); err != ErrKBKnowledgePointNotFound {
		t.Errorf("expected ErrKBKnowledgePointNotFound for unknown id, got %v", err)
	}
}
