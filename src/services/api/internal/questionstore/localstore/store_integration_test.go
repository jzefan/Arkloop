//go:build !desktop

package localstore

import (
	"context"
	"encoding/json"
	"testing"

	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"
	"arkloop/services/shared/questionstore"

	"github.com/google/uuid"
)

// testFixture bundles repos + a freshly-created account/KB for each test.
type testFixture struct {
	ctx     context.Context
	deps    Dependencies
	account data.Account
	kb      *data.KnowledgeBase
}

func setupFixture(t *testing.T) *testFixture {
	t.Helper()
	db := testutil.SetupPostgresDatabase(t, "api_go_questionstore_localstore")
	ctx := context.Background()
	if _, err := migrate.Up(ctx, db.DSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := data.NewPool(ctx, db.DSN, data.PoolLimits{MaxConns: 8})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)

	kbs, _ := data.NewKnowledgeBasesRepository(pool)
	kp, _ := data.NewKBKnowledgePointsRepository(pool)
	q, _ := data.NewKBQuestionsRepository(pool)
	p, _ := data.NewKBPapersRepository(pool)
	accts, _ := data.NewAccountRepository(pool)

	suffix := uuid.New().String()[:8]
	acc, err := accts.Create(ctx, "lsacc-"+suffix, "LSAcc", "personal")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	kb, err := kbs.Create(ctx, data.KBCreate{
		AccountID:    acc.ID,
		WorkspaceRef: "ws-" + suffix,
		Name:         "kb-" + suffix,
	})
	if err != nil {
		t.Fatalf("create kb: %v", err)
	}

	return &testFixture{
		ctx: ctx,
		deps: Dependencies{
			KnowledgeBases:  kbs,
			KnowledgePoints: kp,
			Questions:       q,
			Papers:          p,
		},
		account: acc,
		kb:      kb,
	}
}

func (f *testFixture) store() *Store {
	return New(f.deps, f.kb.ID.String())
}

// --- tests ---

func TestLocalStore_SaveQuestions_PartialFailure(t *testing.T) {
	f := setupFixture(t)
	s := f.store()
	// draft[1] references a non-existent knowledge_point_id (well-formed UUID
	// that does not exist in kb_knowledge_points) -> FK violation.
	bogusKP := uuid.New().String()
	drafts := []questionstore.QuestionDraft{
		{Type: "single_choice", Difficulty: "easy", Stem: "Q1", Answer: "A"},
		{Type: "single_choice", Difficulty: "easy", Stem: "Q2", Answer: "B", KnowledgePointID: bogusKP},
		{Type: "short_answer", Difficulty: "medium", Stem: "Q3", Answer: "C"},
	}
	res, err := s.SaveQuestions(f.ctx, drafts)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(res.Created) != 2 {
		t.Errorf("Created len = %d, want 2: %+v", len(res.Created), res.Created)
	}
	if len(res.Failed) != 1 {
		t.Fatalf("Failed len = %d, want 1: %+v", len(res.Failed), res.Failed)
	}
	if res.Failed[0].Index != 1 {
		t.Errorf("Failed[0].Index = %d, want 1", res.Failed[0].Index)
	}
	if res.Failed[0].ErrorCode != "knowledge_point_not_found" {
		t.Errorf("Failed[0].ErrorCode = %q, want knowledge_point_not_found", res.Failed[0].ErrorCode)
	}
}

func TestLocalStore_SaveQuestions_ValidationError(t *testing.T) {
	f := setupFixture(t)
	s := f.store()
	drafts := []questionstore.QuestionDraft{
		{Type: "not_a_type", Difficulty: "easy", Stem: "Q1", Answer: "A"},
		{Type: "single_choice", Difficulty: "easy", Stem: "", Answer: "B"}, // empty stem
		{Type: "single_choice", Difficulty: "spicy", Stem: "Q3", Answer: "C"},
	}
	res, _ := s.SaveQuestions(f.ctx, drafts)
	if len(res.Created) != 0 {
		t.Errorf("Created len = %d, want 0", len(res.Created))
	}
	if len(res.Failed) != 3 {
		t.Fatalf("Failed len = %d, want 3", len(res.Failed))
	}
	for i, fail := range res.Failed {
		if fail.ErrorCode != "validation_error" {
			t.Errorf("Failed[%d].ErrorCode = %q, want validation_error", i, fail.ErrorCode)
		}
	}
}

func TestLocalStore_SaveQuestions_PatternTagIgnored(t *testing.T) {
	f := setupFixture(t)
	s := f.store()
	drafts := []questionstore.QuestionDraft{
		{
			Type: "single_choice", Difficulty: "easy", Stem: "tagged Q", Answer: "A",
			PatternTag:      "A1",
			CreatedBySource: "ai",
		},
	}
	res, err := s.SaveQuestions(f.ctx, drafts)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(res.Created) != 1 || len(res.Failed) != 0 {
		t.Fatalf("unexpected result: created=%d failed=%d", len(res.Created), len(res.Failed))
	}
	// Read back via the DB layer to confirm the question landed without any
	// schema-level rejection (the kb_questions table has no pattern_tag column
	// at all, so the only way "drop PatternTag" can break is by crashing the
	// save path). The save succeeded -> contract holds.
	qID, _ := uuid.Parse(res.Created[0].ID)
	got, err := f.deps.Questions.GetByID(f.ctx, qID)
	if err != nil || got == nil {
		t.Fatalf("read back: %v / %v", err, got)
	}
	if got.Stem != "tagged Q" {
		t.Errorf("Stem = %q, want %q", got.Stem, "tagged Q")
	}
}

func TestLocalStore_ListReferenceQuestions_FiltersByTypeAndDifficulty(t *testing.T) {
	f := setupFixture(t)
	s := f.store()
	kp, _ := f.deps.KnowledgePoints.Create(f.ctx, data.KBKnowledgePointCreate{KBID: f.kb.ID, Name: "topic"})
	kpID := kp.ID

	seeds := []data.KBQuestionCreate{
		{KBID: f.kb.ID, KnowledgePointID: &kpID, QuestionType: "single_choice", Difficulty: "medium", Stem: "match", Answer: "A"},
		{KBID: f.kb.ID, KnowledgePointID: &kpID, QuestionType: "single_choice", Difficulty: "easy", Stem: "wrong-diff", Answer: "B"},
		{KBID: f.kb.ID, KnowledgePointID: &kpID, QuestionType: "multi_choice", Difficulty: "medium", Stem: "wrong-type", Answer: "C"},
		{KBID: f.kb.ID, KnowledgePointID: &kpID, QuestionType: "short_answer", Difficulty: "hard", Stem: "neither", Answer: "D"},
	}
	for _, sd := range seeds {
		if _, err := f.deps.Questions.Create(f.ctx, sd); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	items, total, err := s.ListReferenceQuestions(f.ctx, kp.ID.String(), questionstore.ListFilter{
		Type:       "single_choice",
		Difficulty: "medium",
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("total=%d items=%d, want 1/1", total, len(items))
	}
	if items[0].Stem != "match" {
		t.Errorf("Stem = %q, want match", items[0].Stem)
	}
}

func TestLocalStore_ListReferenceQuestions_ReturnsTotal(t *testing.T) {
	f := setupFixture(t)
	s := f.store()
	kp, _ := f.deps.KnowledgePoints.Create(f.ctx, data.KBKnowledgePointCreate{KBID: f.kb.ID, Name: "rt"})
	kpID := kp.ID
	for i := 0; i < 12; i++ {
		if _, err := f.deps.Questions.Create(f.ctx, data.KBQuestionCreate{
			KBID: f.kb.ID, KnowledgePointID: &kpID,
			QuestionType: "single_choice", Difficulty: "easy",
			Stem: "Q", Answer: "A",
		}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	items, total, err := s.ListReferenceQuestions(f.ctx, kp.ID.String(), questionstore.ListFilter{Limit: 5})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 12 {
		t.Errorf("total = %d, want 12", total)
	}
	if len(items) != 5 {
		t.Errorf("len(items) = %d, want 5", len(items))
	}
}

func TestLocalStore_ListKnowledgePoints_Mapping(t *testing.T) {
	f := setupFixture(t)
	s := f.store()

	root1, _ := f.deps.KnowledgePoints.Create(f.ctx, data.KBKnowledgePointCreate{KBID: f.kb.ID, Name: "root1", SortOrder: 1})
	_, _ = f.deps.KnowledgePoints.Create(f.ctx, data.KBKnowledgePointCreate{KBID: f.kb.ID, Name: "root2", SortOrder: 2})
	parentID := root1.ID
	child, _ := f.deps.KnowledgePoints.Create(f.ctx, data.KBKnowledgePointCreate{KBID: f.kb.ID, Name: "child", ParentID: &parentID, SortOrder: 3})

	got, err := s.ListKnowledgePoints(f.ctx, f.kb.ID.String())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	// Build map for assertion.
	byName := map[string]questionstore.KnowledgePoint{}
	for _, k := range got {
		byName[k.Name] = k
	}
	if byName["root1"].ParentID != nil {
		t.Errorf("root1 ParentID = %v, want nil", byName["root1"].ParentID)
	}
	if byName["root2"].ParentID != nil {
		t.Errorf("root2 ParentID = %v, want nil", byName["root2"].ParentID)
	}
	if byName["child"].ParentID == nil || *byName["child"].ParentID != root1.ID.String() {
		t.Errorf("child ParentID = %v, want %s", byName["child"].ParentID, root1.ID)
	}
	if byName["child"].ID != child.ID.String() {
		t.Errorf("child ID mismatch")
	}
}

func TestLocalStore_SavePaper_PreservesQuestionIDOrdering(t *testing.T) {
	f := setupFixture(t)
	s := f.store()

	ids := []string{
		uuid.New().String(),
		uuid.New().String(),
		uuid.New().String(),
	}
	// Submit with deliberate duplicate of the first id.
	ordered := []string{ids[0], ids[1], ids[2], ids[0]}

	paperID, err := s.SavePaper(f.ctx, "midterm", f.kb.ID.String(), questionstore.PaperSpec{
		TotalCount: 4,
		Seed:       42,
	}, ordered)
	if err != nil {
		t.Fatalf("save paper: %v", err)
	}
	pid, _ := uuid.Parse(paperID)
	got, err := f.deps.Papers.GetByID(f.ctx, pid)
	if err != nil || got == nil {
		t.Fatalf("paper read: %v / %v", err, got)
	}
	if got.Seed != 42 {
		t.Errorf("seed = %d, want 42", got.Seed)
	}
	var roundtrip []string
	if err := json.Unmarshal(got.QuestionIDsJSON, &roundtrip); err != nil {
		t.Fatalf("unmarshal qids: %v (raw=%s)", err, got.QuestionIDsJSON)
	}
	if len(roundtrip) != len(ordered) {
		t.Fatalf("roundtrip len = %d, want %d", len(roundtrip), len(ordered))
	}
	for i, id := range ordered {
		if roundtrip[i] != id {
			t.Errorf("roundtrip[%d] = %q, want %q", i, roundtrip[i], id)
		}
	}
}

func TestLocalStore_ListQuestionsForPaperPool_AcrossMultipleKPs(t *testing.T) {
	f := setupFixture(t)
	s := f.store()

	kps := make([]uuid.UUID, 0, 3)
	for i := 0; i < 3; i++ {
		kp, err := f.deps.KnowledgePoints.Create(f.ctx, data.KBKnowledgePointCreate{KBID: f.kb.ID, Name: "kp"})
		if err != nil {
			t.Fatalf("kp seed: %v", err)
		}
		kps = append(kps, kp.ID)
	}
	for _, kpID := range kps {
		kpIDCopy := kpID
		for j := 0; j < 2; j++ {
			if _, err := f.deps.Questions.Create(f.ctx, data.KBQuestionCreate{
				KBID: f.kb.ID, KnowledgePointID: &kpIDCopy,
				QuestionType: "single_choice", Difficulty: "easy",
				Stem: "Q", Answer: "A",
			}); err != nil {
				t.Fatalf("q seed: %v", err)
			}
		}
	}

	ids := []string{kps[0].String(), kps[1].String(), kps[2].String()}
	got, err := s.ListQuestionsForPaperPool(f.ctx, ids, questionstore.ListFilter{})
	if err != nil {
		t.Fatalf("pool list: %v", err)
	}
	if len(got) != 6 {
		t.Errorf("len = %d, want 6", len(got))
	}
}

func TestLocalStore_For_StandaloneMode_DelegatesToLocalStore(t *testing.T) {
	f := setupFixture(t)
	Register(f.deps)
	t.Cleanup(func() { questionstore.NewLocalStoreFunc = nil })

	got, err := questionstore.For(questionstore.KBDescriptor{
		IntegrationMode: "standalone",
		ID:              f.kb.ID.String(),
	}, true)
	if err != nil {
		t.Fatalf("For: %v", err)
	}
	if got == nil {
		t.Fatal("got nil store")
	}
	if _, ok := got.(*Store); !ok {
		t.Errorf("got %T, want *Store", got)
	}
}
