//go:build !desktop

package data

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"
)

func setupPapersRepo(t *testing.T) (*KBPapersRepository, *KnowledgeBasesRepository, *AccountRepository, context.Context) {
	t.Helper()
	db := testutil.SetupPostgresDatabase(t, "api_go_kb_papers")
	ctx := context.Background()
	if _, err := migrate.Up(ctx, db.DSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := NewPool(ctx, db.DSN, PoolLimits{MaxConns: 8})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	p, err := NewKBPapersRepository(pool)
	if err != nil {
		t.Fatalf("p repo: %v", err)
	}
	kbs, _ := NewKnowledgeBasesRepository(pool)
	accts, _ := NewAccountRepository(pool)
	return p, kbs, accts, ctx
}

func TestKBPapersCreateAndGet(t *testing.T) {
	p, kbs, accts, ctx := setupPapersRepo(t)
	acc, _ := accts.Create(ctx, "pcreate", "PCreate", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "wpc", Name: "kpc"})

	created, err := p.Create(ctx, KBPaperCreate{
		KBID:            kb.ID,
		Name:            "midterm",
		SpecJSON:        []byte(`{"sections":[{"name":"single","count":5}]}`),
		Seed:            12345,
		QuestionIDsJSON: []byte(`["q-1","q-2","q-3"]`),
		Markdown:        "# Midterm",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := p.GetByID(ctx, created.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v / %v", err, got)
	}
	if got.Name != "midterm" || got.Seed != 12345 || got.Markdown != "# Midterm" {
		t.Errorf("got %+v", got)
	}
	if string(got.SpecJSON) == "" || string(got.QuestionIDsJSON) == "" {
		t.Errorf("jsonb roundtrip lost: spec=%s qids=%s", got.SpecJSON, got.QuestionIDsJSON)
	}
}

func TestKBPapersListByKB(t *testing.T) {
	p, kbs, accts, ctx := setupPapersRepo(t)
	acc, _ := accts.Create(ctx, "plist", "PList", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "wpl", Name: "kpl"})

	for _, n := range []string{"a", "b", "c"} {
		if _, err := p.Create(ctx, KBPaperCreate{KBID: kb.ID, Name: n}); err != nil {
			t.Fatalf("create %s: %v", n, err)
		}
	}
	list, err := p.ListByKB(ctx, kb.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}
}

func TestKBPapersCascadeOnKBDelete(t *testing.T) {
	p, kbs, accts, ctx := setupPapersRepo(t)
	acc, _ := accts.Create(ctx, "pcasc", "PCasc", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "wpz", Name: "kpz"})

	created, _ := p.Create(ctx, KBPaperCreate{KBID: kb.ID, Name: "doomed"})
	if err := kbs.Delete(ctx, kb.ID); err != nil {
		t.Fatalf("delete kb: %v", err)
	}
	got, err := p.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get after cascade: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil after cascade, got %+v", got)
	}
}

func TestKBPapersPreserveQuestionIDsOrder(t *testing.T) {
	p, kbs, accts, ctx := setupPapersRepo(t)
	acc, _ := accts.Create(ctx, "pord", "POrd", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "wpo", Name: "kpo"})

	// Intentionally non-sorted to ensure JSONB stores exactly what was given.
	raw := []byte(`["q-9","q-2","q-7","q-1","q-3"]`)
	created, err := p.Create(ctx, KBPaperCreate{
		KBID:            kb.ID,
		Name:            "ordering",
		QuestionIDsJSON: raw,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := p.GetByID(ctx, created.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v / %v", err, got)
	}
	// JSONB normalises whitespace, so compare parsed values element-by-element
	// to assert ordering (not bytewise) preservation.
	var gotIDs, wantIDs []string
	if err := json.Unmarshal(got.QuestionIDsJSON, &gotIDs); err != nil {
		t.Fatalf("unmarshal got: %v (raw=%s)", err, got.QuestionIDsJSON)
	}
	if err := json.Unmarshal(raw, &wantIDs); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Errorf("question_ids ordering changed:\n got  %v\n want %v", gotIDs, wantIDs)
	}
}
