//go:build !desktop

package data

import (
	"context"
	"errors"
	"testing"
	"time"

	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"

	"github.com/google/uuid"
)

func setupQuestionsRepo(t *testing.T) (*KBQuestionsRepository, *KBKnowledgePointsRepository, *KnowledgeBasesRepository, *AccountRepository, context.Context) {
	t.Helper()
	db := testutil.SetupPostgresDatabase(t, "api_go_kb_questions")
	ctx := context.Background()
	if _, err := migrate.Up(ctx, db.DSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := NewPool(ctx, db.DSN, PoolLimits{MaxConns: 8})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	q, err := NewKBQuestionsRepository(pool)
	if err != nil {
		t.Fatalf("q repo: %v", err)
	}
	kp, _ := NewKBKnowledgePointsRepository(pool)
	kbs, _ := NewKnowledgeBasesRepository(pool)
	accts, _ := NewAccountRepository(pool)
	return q, kp, kbs, accts, ctx
}

func TestKBQuestionsCreateAndGet(t *testing.T) {
	q, _, kbs, accts, ctx := setupQuestionsRepo(t)
	acc, _ := accts.Create(ctx, "qcreate", "QCreate", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "wsq", Name: "kbq"})

	created, err := q.Create(ctx, KBQuestionCreate{
		KBID:               kb.ID,
		QuestionType:       "single_choice",
		Difficulty:         "easy",
		Stem:               "1+1=?",
		OptionsJSON:        []byte(`[{"key":"A","text":"2"},{"key":"B","text":"3"}]`),
		Answer:             "A",
		Explanation:        "basic arithmetic",
		SourceChunkIDsJSON: []byte(`["c-1"]`),
		SourceSnippetsJSON: []byte(`[{"chunk_ref":"c-1","snippet":"1+1=2"}]`),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.QualityFlag != "draft" {
		t.Errorf("default quality = %q, want draft", created.QualityFlag)
	}

	got, err := q.GetByID(ctx, created.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v / %v", err, got)
	}
	if got.Stem != "1+1=?" || got.QuestionType != "single_choice" || got.Difficulty != "easy" {
		t.Errorf("got %+v", got)
	}
	if string(got.OptionsJSON) == "" || string(got.SourceChunkIDsJSON) == "" {
		t.Errorf("jsonb roundtrip lost: opts=%s chunks=%s", got.OptionsJSON, got.SourceChunkIDsJSON)
	}
}

func TestKBQuestionsListWithFilters(t *testing.T) {
	q, kp, kbs, accts, ctx := setupQuestionsRepo(t)
	acc, _ := accts.Create(ctx, "qlist", "QList", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "wsl", Name: "kbl"})
	point, _ := kp.Create(ctx, KBKnowledgePointCreate{KBID: kb.ID, Name: "topic"})

	type seed struct {
		typ, diff string
		withKP    bool
	}
	for _, s := range []seed{
		{"single_choice", "easy", false},
		{"single_choice", "medium", true},
		{"multi_choice", "easy", false},
		{"short_answer", "hard", true},
		{"fill_in", "medium", false},
	} {
		in := KBQuestionCreate{
			KBID: kb.ID, QuestionType: s.typ, Difficulty: s.diff,
			Stem: s.typ + ":" + s.diff, Answer: "x",
		}
		if s.withKP {
			id := point.ID
			in.KnowledgePointID = &id
		}
		if _, err := q.Create(ctx, in); err != nil {
			t.Fatalf("seed %+v: %v", s, err)
		}
	}

	// No filter -> all 5.
	all, total, err := q.ListByKB(ctx, kb.ID, KBQuestionFilter{})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if total != 5 || len(all) != 5 {
		t.Errorf("all: total=%d, len=%d, want 5/5", total, len(all))
	}

	// Type only.
	sc, total, err := q.ListByKB(ctx, kb.ID, KBQuestionFilter{QuestionType: "single_choice"})
	if err != nil {
		t.Fatalf("list type: %v", err)
	}
	if total != 2 || len(sc) != 2 {
		t.Errorf("single_choice: total=%d, len=%d, want 2/2", total, len(sc))
	}

	// Type + difficulty.
	scEasy, total, err := q.ListByKB(ctx, kb.ID, KBQuestionFilter{QuestionType: "single_choice", Difficulty: "easy"})
	if err != nil {
		t.Fatalf("list type+diff: %v", err)
	}
	if total != 1 || len(scEasy) != 1 {
		t.Errorf("single_choice+easy: total=%d, len=%d, want 1/1", total, len(scEasy))
	}

	// KP filter.
	kpFilter, total, err := q.ListByKB(ctx, kb.ID, KBQuestionFilter{KnowledgePointID: &point.ID})
	if err != nil {
		t.Fatalf("list kp: %v", err)
	}
	if total != 2 || len(kpFilter) != 2 {
		t.Errorf("kp filter: total=%d, len=%d, want 2/2", total, len(kpFilter))
	}

	// Limit + offset.
	page, total, err := q.ListByKB(ctx, kb.ID, KBQuestionFilter{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("list paged: %v", err)
	}
	if total != 5 {
		t.Errorf("paged total=%d, want 5", total)
	}
	if len(page) != 2 {
		t.Errorf("paged len=%d, want 2", len(page))
	}
}

func TestKBQuestionsUpdateBumpsUpdatedAt(t *testing.T) {
	q, _, kbs, accts, ctx := setupQuestionsRepo(t)
	acc, _ := accts.Create(ctx, "qupd", "QUpd", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "wsu", Name: "kbu"})
	created, err := q.Create(ctx, KBQuestionCreate{
		KBID: kb.ID, QuestionType: "short_answer", Difficulty: "medium",
		Stem: "old stem", Answer: "old",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Sleep a bit to make sure updated_at can actually advance on systems with
	// coarse timestamp resolution.
	time.Sleep(20 * time.Millisecond)

	newStem := "new stem"
	newQuality := "accepted"
	updated, err := q.Update(ctx, created.ID, KBQuestionPatch{
		Stem:        &newStem,
		QualityFlag: &newQuality,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Stem != newStem {
		t.Errorf("stem = %q, want %q", updated.Stem, newStem)
	}
	if updated.QualityFlag != "accepted" {
		t.Errorf("quality = %q, want accepted", updated.QualityFlag)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Errorf("updated_at not advanced: created=%s, updated=%s", created.UpdatedAt, updated.UpdatedAt)
	}
}

func TestKBQuestionsDelete(t *testing.T) {
	q, _, kbs, accts, ctx := setupQuestionsRepo(t)
	acc, _ := accts.Create(ctx, "qdel", "QDel", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "wsd", Name: "kbd"})
	created, _ := q.Create(ctx, KBQuestionCreate{
		KBID: kb.ID, QuestionType: "essay", Difficulty: "hard",
		Stem: "essay", Answer: "answer",
	})
	if err := q.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ := q.GetByID(ctx, created.ID)
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
	if err := q.Delete(ctx, created.ID); err != ErrKBQuestionNotFound {
		t.Errorf("re-delete: want ErrKBQuestionNotFound, got %v", err)
	}

	// Count for this KB should be 0.
	n, err := q.CountByKB(ctx, kb.ID)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("count = %d, want 0", n)
	}
}

func TestKBQuestion_Update_EmptyPatchOnMissingRow_ReturnsNotFound(t *testing.T) {
	q, _, _, _, ctx := setupQuestionsRepo(t)
	// No-op patch against a non-existent id must surface ErrKBQuestionNotFound,
	// not (nil, nil). Mirrors the non-empty-patch error path.
	got, err := q.Update(ctx, uuid.New(), KBQuestionPatch{})
	if !errors.Is(err, ErrKBQuestionNotFound) {
		t.Errorf("err = %v, want ErrKBQuestionNotFound", err)
	}
	if got != nil {
		t.Errorf("got = %+v, want nil", got)
	}
}

func TestKBQuestion_Update_ClearKnowledgePointID(t *testing.T) {
	q, kp, kbs, accts, ctx := setupQuestionsRepo(t)
	acc, _ := accts.Create(ctx, "qclear", "QClear", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "wscl", Name: "kbcl"})
	point, _ := kp.Create(ctx, KBKnowledgePointCreate{KBID: kb.ID, Name: "kp-clear"})

	// Create question already linked to a knowledge point.
	kpID := point.ID
	created, err := q.Create(ctx, KBQuestionCreate{
		KBID: kb.ID, KnowledgePointID: &kpID,
		QuestionType: "single_choice", Difficulty: "easy",
		Stem: "stem", Answer: "A",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.KnowledgePointID == nil || *created.KnowledgePointID != point.ID {
		t.Fatalf("seed KP not stored: %+v", created.KnowledgePointID)
	}

	// ClearKnowledgePointID=true must NULL the column.
	updated, err := q.Update(ctx, created.ID, KBQuestionPatch{ClearKnowledgePointID: true})
	if err != nil {
		t.Fatalf("update clear: %v", err)
	}
	if updated.KnowledgePointID != nil {
		t.Errorf("KnowledgePointID = %v, want nil after clear", *updated.KnowledgePointID)
	}

	// And ClearKnowledgePointID must be ignored when KnowledgePointID is non-nil.
	newKP, _ := kp.Create(ctx, KBKnowledgePointCreate{KBID: kb.ID, Name: "kp-reset"})
	newID := newKP.ID
	updated2, err := q.Update(ctx, created.ID, KBQuestionPatch{
		KnowledgePointID:      &newID,
		ClearKnowledgePointID: true, // ignored in favour of explicit value
	})
	if err != nil {
		t.Fatalf("update reset: %v", err)
	}
	if updated2.KnowledgePointID == nil || *updated2.KnowledgePointID != newKP.ID {
		t.Errorf("KnowledgePointID = %v, want %s", updated2.KnowledgePointID, newKP.ID)
	}
}

func TestKBQuestionsCheckConstraintRejectsInvalidType(t *testing.T) {
	q, _, kbs, accts, ctx := setupQuestionsRepo(t)
	acc, _ := accts.Create(ctx, "qcheck", "QCheck", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "wsc", Name: "kbc"})
	_, err := q.Create(ctx, KBQuestionCreate{
		KBID: kb.ID, QuestionType: "not_a_type", Difficulty: "easy",
		Stem: "x", Answer: "y",
	})
	if err == nil {
		t.Fatal("expected check-constraint violation, got nil")
	}
}
