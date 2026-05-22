//go:build !desktop

package data

import (
	"context"
	"testing"

	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"
)

func setupDocsRepo(t *testing.T) (*KBDocumentsRepository, *KnowledgeBasesRepository, *AccountRepository, context.Context) {
	t.Helper()
	db := testutil.SetupPostgresDatabase(t, "api_go_kb_docs")
	ctx := context.Background()
	if _, err := migrate.Up(ctx, db.DSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := NewPool(ctx, db.DSN, PoolLimits{MaxConns: 8})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	docs, _ := NewKBDocumentsRepository(pool)
	kbRepo, _ := NewKnowledgeBasesRepository(pool)
	accountRepo, _ := NewAccountRepository(pool)
	return docs, kbRepo, accountRepo, ctx
}

func TestDocCreateAndStatusTransitions(t *testing.T) {
	docs, kbs, accts, ctx := setupDocsRepo(t)
	acc, _ := accts.Create(ctx, "u", "U", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "w", Name: "n"})
	doc, err := docs.Create(ctx, DocCreate{
		KBID: kb.ID, OriginalFilename: "x.txt", MimeType: "text/plain",
		BlobSHA256: "abc", SizeBytes: 100,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if doc.Status != "queued" {
		t.Errorf("initial status: got %q, want queued", doc.Status)
	}
	if err := docs.UpdateStatus(ctx, doc.ID, "parsing", "", nil); err != nil {
		t.Fatalf("update parsing: %v", err)
	}
	got, _ := docs.GetByID(ctx, doc.ID)
	if got.Status != "parsing" {
		t.Errorf("status after update: got %q, want parsing", got.Status)
	}
	if err := docs.UpdateStatus(ctx, doc.ID, "failed", "OCR error", nil); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	got, _ = docs.GetByID(ctx, doc.ID)
	if got.ErrorMessage != "OCR error" {
		t.Errorf("error_message: got %q", got.ErrorMessage)
	}
}

func TestDocListByKB(t *testing.T) {
	docs, kbs, accts, ctx := setupDocsRepo(t)
	acc, _ := accts.Create(ctx, "u", "U", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "w", Name: "n"})
	for i, name := range []string{"a.txt", "b.txt", "c.txt"} {
		_, err := docs.Create(ctx, DocCreate{KBID: kb.ID, OriginalFilename: name, MimeType: "text/plain", BlobSHA256: "sha", SizeBytes: int64(i + 1)})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	out, err := docs.ListByKB(ctx, kb.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out) != 3 {
		t.Errorf("got %d docs, want 3", len(out))
	}
}

func TestDocDelete(t *testing.T) {
	docs, kbs, accts, ctx := setupDocsRepo(t)
	acc, _ := accts.Create(ctx, "u", "U", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "w", Name: "n"})
	doc, _ := docs.Create(ctx, DocCreate{KBID: kb.ID, OriginalFilename: "a.txt", MimeType: "text/plain", BlobSHA256: "s", SizeBytes: 1})
	if err := docs.Delete(ctx, doc.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ := docs.GetByID(ctx, doc.ID)
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}
