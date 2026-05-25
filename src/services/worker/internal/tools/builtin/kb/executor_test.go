package kb

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	wdata "arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
)

type fakeChunksRepo struct{ hits []wdata.KBChunkHit }

func (f *fakeChunksRepo) Search(ctx context.Context, kbID uuid.UUID, q []float32, k int) ([]wdata.KBChunkHit, error) {
	return f.hits, nil
}

func (f *fakeChunksRepo) ListHeadings(ctx context.Context, kbID, docID uuid.UUID) ([]wdata.KBChunkHit, error) {
	return nil, nil
}

func (f *fakeChunksRepo) GetByIDs(ctx context.Context, kbID uuid.UUID, ids []uuid.UUID) ([]wdata.KBChunkHit, error) {
	byID := make(map[uuid.UUID]wdata.KBChunkHit, len(f.hits))
	for _, hit := range f.hits {
		byID[hit.ID] = hit
	}
	out := make([]wdata.KBChunkHit, 0, len(ids))
	for _, id := range ids {
		if hit, ok := byID[id]; ok && hit.KBID == kbID {
			out = append(out, hit)
		}
	}
	return out, nil
}

func (f *fakeChunksRepo) Dim() int { return 8 }

// adminProvider is a ProviderClient fake that records every CallExamAsAdmin
// invocation. The plain CallExam path is intentionally not used by linked-
// mode read flows after the admin-credentials migration; if the executor
// ever falls back to it, that's a regression and the test should fail
// loudly.
type adminProvider struct {
	calls            []string
	bodies           []any
	bankResponses    []map[string]any
	questionResponse map[string]any
	kpResponse       map[string]any
}

func (p *adminProvider) CallExam(ctx context.Context, userID uuid.UUID, scopes []string, method, path string, body any, out any) error {
	p.calls = append(p.calls, "USER "+method+" "+path)
	encodeInto(map[string]any{}, out)
	return nil
}

func (p *adminProvider) CallExamAsAdmin(ctx context.Context, method, path string, body any, out any) error {
	p.calls = append(p.calls, "ADMIN "+method+" "+path)
	p.bodies = append(p.bodies, body)
	switch {
	case method == "GET" && path == "/api/question-banks":
		encodeInto(p.bankResponses, out)
	case method == "GET" && strings.HasPrefix(path, "/api/knowledge-points?"):
		encodeInto(p.kpResponse, out)
	case method == "GET" && strings.HasPrefix(path, "/api/questions?"):
		encodeInto(p.questionResponse, out)
	default:
		encodeInto(map[string]any{}, out)
	}
	return nil
}

func encodeInto(v any, out any) {
	b, _ := json.Marshal(v)
	_ = json.Unmarshal(b, out)
}

type fakeEmbedder struct{ dim int }

func (e fakeEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		v := make([]float32, e.dim)
		v[0] = 1
		out[i] = v
	}
	return out, nil
}

func (e fakeEmbedder) Dim() int { return e.dim }

type fakeAccess struct{ allow bool }

func (f fakeAccess) IsActorWorkspaceMember(ctx context.Context, accountID, kbID uuid.UUID) (bool, error) {
	return f.allow, nil
}

func TestKBSearchHappyPath(t *testing.T) {
	kbID := uuid.New()
	exe := NewExecutor(&fakeChunksRepo{hits: []wdata.KBChunkHit{
		{DocumentRef: "physics.txt", Ordinal: 3, Text: "光的干涉", Score: 0.91, Metadata: map[string]any{"page": 2}},
		{DocumentRef: "physics.txt", Ordinal: 4, Text: "杨氏双缝", Score: 0.74},
	}}, fakeEmbedder{dim: 8}, fakeAccess{allow: true})
	accountID := uuid.New()

	result := exe.Execute(context.Background(), ToolNameSearch, map[string]any{"kb_id": kbID.String(), "query": "干涉", "k": 5}, tools.ExecutionContext{AccountID: &accountID}, "")
	if result.Error != nil {
		t.Fatalf("execute error: %v", result.Error)
	}
	hits, _ := result.ResultJSON["hits"].([]map[string]any)
	if len(hits) != 2 {
		t.Errorf("got %d hits", len(hits))
	}
	meta, _ := hits[0]["metadata"].(map[string]any)
	if meta["page"] != 2 {
		t.Fatalf("metadata not returned: %+v", hits[0])
	}
}

func TestKBSearchDeniesNonMember(t *testing.T) {
	exe := NewExecutor(&fakeChunksRepo{}, fakeEmbedder{dim: 8}, fakeAccess{allow: false})
	accountID := uuid.New()
	result := exe.Execute(context.Background(), ToolNameSearch, map[string]any{"kb_id": uuid.New().String(), "query": "x"}, tools.ExecutionContext{AccountID: &accountID}, "")
	if result.Error == nil || result.Error.ErrorClass != errorPermissionDenied {
		t.Fatalf("expected permission denied, got %+v", result.Error)
	}
}

func TestKBSearchValidatesInput(t *testing.T) {
	exe := NewExecutor(&fakeChunksRepo{}, fakeEmbedder{dim: 8}, fakeAccess{allow: true})
	accountID := uuid.New()
	result := exe.Execute(context.Background(), ToolNameSearch, map[string]any{"query": "x"}, tools.ExecutionContext{AccountID: &accountID}, "")
	if result.Error == nil || result.Error.ErrorClass != errorArgsInvalid {
		t.Fatalf("expected args error for missing kb_id, got %+v", result.Error)
	}
	result = exe.Execute(context.Background(), ToolNameSearch, map[string]any{"kb_id": uuid.New().String()}, tools.ExecutionContext{AccountID: &accountID}, "")
	if result.Error == nil || result.Error.ErrorClass != errorArgsInvalid {
		t.Fatalf("expected args error for missing query, got %+v", result.Error)
	}
}

func TestKBSaveQuestionsRequiresConfirmation(t *testing.T) {
	exe := NewExecutor(&fakeChunksRepo{}, fakeEmbedder{dim: 8}, fakeAccess{allow: true})
	accountID := uuid.New()
	userID := uuid.New()

	result := exe.Execute(context.Background(), ToolNameSaveQuestions, map[string]any{
		"kb_id": uuid.New().String(),
		"questions": []any{
			map[string]any{
				"knowledge_point_id": uuid.New().String(),
				"type":               "single_choice",
				"difficulty":         "medium",
				"stem":               "题干",
				"answer":             "A",
			},
		},
	}, tools.ExecutionContext{AccountID: &accountID, UserID: &userID}, "")

	if result.Error == nil || result.Error.ErrorClass != errorConfirmation {
		t.Fatalf("expected confirmation error, got %+v", result.Error)
	}
}

func TestSelectQuestionsReportsTypeShortage(t *testing.T) {
	selected, warnings := selectQuestions([]questionRow{
		{ID: uuid.New(), Type: "single_choice"},
	}, 2, map[string]int{"single_choice": 2}, nil, nil, 0)
	if len(selected) != 0 {
		t.Fatalf("expected no selected questions, got %d", len(selected))
	}
	if len(warnings) != 1 || warnings[0]["type"] != "single_choice" {
		t.Fatalf("expected single_choice shortage, got %+v", warnings)
	}
}

func TestSelectQuestionsSatisfiesDifficultyDistribution(t *testing.T) {
	pool := []questionRow{
		{ID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), Type: "single_choice", Difficulty: "easy"},
		{ID: uuid.MustParse("00000000-0000-0000-0000-000000000002"), Type: "single_choice", Difficulty: "medium"},
		{ID: uuid.MustParse("00000000-0000-0000-0000-000000000003"), Type: "single_choice", Difficulty: "medium"},
		{ID: uuid.MustParse("00000000-0000-0000-0000-000000000004"), Type: "single_choice", Difficulty: "hard"},
	}

	selected, warnings := selectQuestions(pool, 3, nil, map[string]int{"easy": 1, "medium": 2}, nil, 0)

	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %+v", warnings)
	}
	counts := map[string]int{}
	for _, q := range selected {
		counts[q.Difficulty]++
	}
	if counts["easy"] != 1 || counts["medium"] != 2 {
		t.Fatalf("unexpected difficulty distribution: selected=%+v counts=%+v", selected, counts)
	}
}

func TestSelectQuestionsReportsKnowledgePointShortage(t *testing.T) {
	kpID := uuid.New()
	otherID := uuid.New()
	selected, warnings := selectQuestions([]questionRow{
		{ID: uuid.New(), KnowledgePointID: &kpID, Type: "single_choice", Difficulty: "medium"},
		{ID: uuid.New(), KnowledgePointID: &otherID, Type: "single_choice", Difficulty: "medium"},
	}, 2, nil, nil, map[string]int{kpID.String(): 2}, 0)

	if len(selected) != 0 {
		t.Fatalf("expected no selected questions, got %d", len(selected))
	}
	if len(warnings) != 1 || warnings[0]["knowledge_point_id"] != kpID.String() {
		t.Fatalf("expected knowledge point shortage, got %+v", warnings)
	}
}

func TestPaperPreviewPanelContainsConfirmationPrompt(t *testing.T) {
	panel := paperPreviewPanel("期中卷", []questionRow{
		{ID: uuid.New(), Stem: "光的干涉题", Type: "single_choice", Difficulty: "medium"},
	}, "# 期中卷", false)
	code, _ := panel["widget_code"].(string)
	if !strings.Contains(code, "sendPrompt('确认保存这份试卷')") {
		t.Fatalf("expected confirmation sendPrompt in widget code: %s", code)
	}
	if panel["kind"] != "paper_preview" {
		t.Fatalf("unexpected panel kind: %+v", panel)
	}
}

func TestQuestionPanelsUseChineseLabelsForTypeAndDifficulty(t *testing.T) {
	draft := questionDraftPanel("第二章", 3, "single_choice", "medium", 2, 1)
	draftCode, _ := draft["widget_code"].(string)
	if !strings.Contains(draftCode, "单选题") || !strings.Contains(draftCode, "中等") {
		t.Fatalf("draft panel should display Chinese type/difficulty labels: %s", draftCode)
	}
	if strings.Contains(draftCode, "single_choice") || strings.Contains(draftCode, "medium") {
		t.Fatalf("draft panel leaked internal enum labels: %s", draftCode)
	}

	preview := paperPreviewPanel("妇产科小测", []questionRow{
		{ID: uuid.New(), Stem: "女性生殖系统发育题", Type: "single_choice", Difficulty: "medium"},
	}, "# 妇产科小测", false)
	previewCode, _ := preview["widget_code"].(string)
	if !strings.Contains(previewCode, "单选题 / 中等") {
		t.Fatalf("preview panel should display Chinese type/difficulty labels: %s", previewCode)
	}
	if strings.Contains(previewCode, "single_choice / medium") {
		t.Fatalf("preview panel leaked internal enum labels: %s", previewCode)
	}

	shortage := shortagePanel([]map[string]any{
		{"type": "single_choice", "requested": 3, "available": 1},
		{"difficulty": "medium", "requested": 3, "available": 1},
	})
	shortageCode, _ := shortage["widget_code"].(string)
	if !strings.Contains(shortageCode, "题型 单选题") || !strings.Contains(shortageCode, "难度 中等") {
		t.Fatalf("shortage panel should display Chinese type/difficulty labels: %s", shortageCode)
	}
	if strings.Contains(shortageCode, "single_choice") || strings.Contains(shortageCode, "medium") {
		t.Fatalf("shortage panel leaked internal enum labels: %s", shortageCode)
	}
}

func TestBookTutorPromptDefaultsGynSkillForGynecologyExamWork(t *testing.T) {
	content, err := os.ReadFile("../../../../../../personas/book-tutor-agent/prompt.md")
	if err != nil {
		t.Fatalf("read book tutor prompt: %v", err)
	}
	prompt := string(content)
	for _, required := range []string{"妇产科", "妇产科学", "gyn-medical-exam", "load_skill"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("prompt missing %q", required)
		}
	}
}

func TestProviderUnavailableIsTeacherFriendly(t *testing.T) {
	result := providerUnavailable("当前课程资料绑定的保存目标暂时不可用，暂不能保存试卷。请稍后重试。")
	if result.Error == nil {
		t.Fatal("expected error")
	}
	if result.Error.ErrorClass != "kb.provider_unavailable" {
		t.Fatalf("unexpected error class: %+v", result.Error)
	}
	if strings.Contains(strings.ToLower(result.Error.Message), "exam") || strings.Contains(strings.ToLower(result.Error.Message), "provider") {
		t.Fatalf("message should not expose provider internals: %q", result.Error.Message)
	}
}

// TestLinkedReadPathsUseAdminCredentials enforces the post-admin-migration
// invariant: linked-mode KP listing and draft-questions reference fetch
// MUST go through CallExamAsAdmin (not the per-user CallExam). Ordinary
// teacher accounts cannot see exam's platform-administrator question bank
// (e.g. 国考医学), so falling back to user tokens here would silently
// return empty references.
func TestLinkedReadPathsUseAdminCredentials(t *testing.T) {
	provider := &adminProvider{
		bankResponses: []map[string]any{
			{"id": "bank-paper", "name": "组卷题库"},
			{"id": "bank-national-medical", "name": "国考医学"},
		},
		kpResponse: map[string]any{
			"items": []map[string]any{{"id": "kp-1", "display_name": "妇产科基础"}},
			"total": 1,
		},
		questionResponse: map[string]any{
			"items": []map[string]any{{
				"id":                 "q-admin-1",
				"knowledge_point_id": "kp-1",
				"type":               "single_choice",
				"difficulty":         "medium",
				"stem":               "国考医学题",
				"answer":             "A",
			}},
		},
	}
	exe := NewExecutor(&fakeChunksRepo{}, fakeEmbedder{dim: 8}, fakeAccess{allow: true})
	exe.provider = provider

	scope := "scope-physics"
	kb := kbDescriptor{ID: uuid.New(), IntegrationMode: "exam", ExamScopeID: &scope}
	execCtx := tools.ExecutionContext{}

	kpRes := exe.executeProviderListKnowledgePoints(context.Background(), kb, execCtx)
	if kpRes.Error != nil {
		t.Fatalf("list kp error: %+v", kpRes.Error)
	}

	draftRes := exe.executeProviderDraftQuestions(context.Background(), kb, map[string]any{
		"knowledge_point_id": "kp-1",
		"count":              float64(3),
		"type":               "single_choice",
		"difficulty":         "medium",
	}, execCtx)
	if draftRes.Error != nil {
		t.Fatalf("draft questions error: %+v", draftRes.Error)
	}
	refs, _ := draftRes.ResultJSON["reference_questions"].([]map[string]any)
	if len(refs) != 1 || refs[0]["id"] != "q-admin-1" {
		t.Fatalf("expected admin-only reference question to be returned, got %+v", refs)
	}

	for _, c := range provider.calls {
		if strings.HasPrefix(c, "USER ") {
			t.Fatalf("linked-mode read path leaked into per-user CallExam: %s (calls=%v)",
				c, provider.calls)
		}
	}
	wantFragments := []string{
		"ADMIN GET /api/knowledge-points?",
		"ADMIN GET /api/question-banks",
		"ADMIN GET /api/questions?",
	}
	for _, want := range wantFragments {
		found := false
		for _, c := range provider.calls {
			if strings.HasPrefix(c, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing expected admin call %q in %v", want, provider.calls)
		}
	}
}

func TestListKnowledgeBasesDoesNotImplicitlyUseRunWorkspace(t *testing.T) {
	got := listKnowledgeBasesWorkspaceRef(map[string]any{})
	if got != "" {
		t.Fatalf("expected empty workspace filter when teacher did not choose one, got %q", got)
	}

	got = listKnowledgeBasesWorkspaceRef(map[string]any{"workspace_ref": "  wsref_course  "})
	if got != "wsref_course" {
		t.Fatalf("expected explicit workspace_ref to be preserved, got %q", got)
	}
}

func TestDeriveChapterKnowledgePointNamesFromHeadings(t *testing.T) {
	names := deriveChapterKnowledgePointNames([]headingCandidate{
		{Text: "10", Ordinal: 1},
		{Text: "第二章", Ordinal: 100},
		{Text: "女性生殖系统发育与解剖", Ordinal: 101},
		{Text: "第三章", Ordinal: 150},
		{Text: "女性生殖系统生理", Ordinal: 151},
		{Text: "第三章", Ordinal: 152},
		{Text: "女性生殖系统生理", Ordinal: 153},
	})

	want := []string{"第二章 女性生殖系统发育与解剖", "第三章 女性生殖系统生理"}
	if len(names) != len(want) {
		t.Fatalf("expected %d names, got %d: %+v", len(want), len(names), names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("name[%d] = %q, want %q (all=%+v)", i, names[i], want[i], names)
		}
	}
}
