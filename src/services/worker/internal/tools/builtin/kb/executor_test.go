package kb

import (
	"context"
	"encoding/json"
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

type captureProvider struct {
	body any
	resp any
}

func (p *captureProvider) CallExam(ctx context.Context, userID uuid.UUID, scopes []string, method, path string, body any, out any) error {
	p.body = body
	b, _ := json.Marshal(p.resp)
	_ = json.Unmarshal(b, out)
	return nil
}

type routeProvider struct {
	calls       []string
	bodies      []any
	paperBody   map[string]any
	questionReq map[string]any
}

func (p *routeProvider) CallExam(ctx context.Context, userID uuid.UUID, scopes []string, method, path string, body any, out any) error {
	p.calls = append(p.calls, method+" "+path)
	p.bodies = append(p.bodies, body)
	switch {
	case method == "POST" && path == "/api/questions/batch":
		p.questionReq, _ = body.(map[string]any)
		encodeInto(map[string]any{
			"created": []map[string]any{{"index": 0, "id": "q-new-1"}},
			"failed":  []map[string]any{},
		}, out)
	case method == "GET" && strings.HasPrefix(path, "/api/questions?"):
		encodeInto(map[string]any{
			"items": []map[string]any{{
				"id":                 "q-new-1",
				"knowledge_point_id": "kp-interference",
				"type":               "single_choice",
				"difficulty":         "medium",
				"stem":               "干涉题",
				"answer":             "A",
				"explanation":        "根据教材干涉实验可得。",
				"source_snippets":    []map[string]any{{"snippet": "课程资料快照"}},
			}},
		}, out)
	case method == "POST" && path == "/api/papers":
		p.paperBody, _ = body.(map[string]any)
		encodeInto(map[string]any{"id": "paper-1", "name": p.paperBody["name"]}, out)
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

func TestProviderSaveQuestionsAutofillsSourceSnippetsFromChunkIDs(t *testing.T) {
	kbID := uuid.New()
	chunkID := uuid.New()
	sourceText := strings.Repeat("课程资料中的物理光学干涉实验依据。", 18)
	provider := &captureProvider{resp: map[string]any{
		"created": []map[string]any{{"index": 0, "id": "q-new-1"}},
		"failed":  []map[string]any{},
	}}
	exe := NewExecutor(&fakeChunksRepo{hits: []wdata.KBChunkHit{{
		ID:          chunkID,
		KBID:        kbID,
		DocumentRef: "physics.pdf",
		Ordinal:     7,
		HeadingPath: []string{"第三章", "物理光学"},
		Text:        sourceText,
	}}}, fakeEmbedder{dim: 8}, fakeAccess{allow: true})
	exe.provider = provider
	userID := uuid.New()
	scopeID := "scope-physics"

	result := exe.executeProviderSaveQuestions(context.Background(), kbDescriptor{
		ID:              kbID,
		IntegrationMode: "exam",
		ExamScopeID:     &scopeID,
	}, map[string]any{
		"questions": []any{
			map[string]any{
				"knowledge_point_id": "kp-interference",
				"type":               "single_choice",
				"difficulty":         "medium",
				"stem":               "干涉题",
				"answer":             "A",
				"source_chunk_ids":   []any{chunkID.String()},
			},
		},
	}, tools.ExecutionContext{UserID: &userID})

	if result.Error != nil {
		t.Fatalf("execute error: %+v", result.Error)
	}
	body, _ := provider.body.(map[string]any)
	questions, _ := body["questions"].([]map[string]any)
	if len(questions) != 1 {
		t.Fatalf("provider body missing questions: %#v", provider.body)
	}
	snippets, _ := questions[0]["source_snippets"].([]map[string]any)
	if len(snippets) != 1 {
		t.Fatalf("expected one auto-filled source snippet, got %#v", questions[0]["source_snippets"])
	}
	if snippets[0]["chunk_id"] != chunkID.String() || snippets[0]["document_ref"] != "physics.pdf" || snippets[0]["ordinal"] != 7 {
		t.Fatalf("unexpected snippet metadata: %+v", snippets[0])
	}
	text, _ := snippets[0]["snippet"].(string)
	if len([]rune(text)) > 500 || len([]rune(text)) < 200 {
		t.Fatalf("snippet length = %d, want 200..500", len([]rune(text)))
	}
}

func TestProviderSaveQuestionsRejectsQuestionsWithoutSource(t *testing.T) {
	provider := &captureProvider{resp: map[string]any{"created": []map[string]any{}, "failed": []map[string]any{}}}
	exe := NewExecutor(&fakeChunksRepo{}, fakeEmbedder{dim: 8}, fakeAccess{allow: true})
	exe.provider = provider
	userID := uuid.New()
	scopeID := "scope-physics"

	result := exe.executeProviderSaveQuestions(context.Background(), kbDescriptor{
		ID:              uuid.New(),
		IntegrationMode: "exam",
		ExamScopeID:     &scopeID,
	}, map[string]any{
		"questions": []any{
			map[string]any{
				"knowledge_point_id": "kp-interference",
				"type":               "single_choice",
				"difficulty":         "medium",
				"stem":               "干涉题",
				"answer":             "A",
			},
		},
	}, tools.ExecutionContext{UserID: &userID})

	if result.Error != nil {
		t.Fatalf("execute error: %+v", result.Error)
	}
	if provider.body != nil {
		t.Fatalf("provider should not be called when all questions fail validation")
	}
	failed, _ := result.ResultJSON["failed"].([]map[string]any)
	if len(failed) != 1 || failed[0]["error_code"] != "source_required" {
		t.Fatalf("expected source_required failure, got %+v", result.ResultJSON)
	}
}

func TestLinkedPaperBuilderE2E_SaveQuestionThenComposePaper(t *testing.T) {
	kbID := uuid.New()
	chunkID := uuid.New()
	provider := &routeProvider{}
	exe := NewExecutor(&fakeChunksRepo{hits: []wdata.KBChunkHit{{
		ID:          chunkID,
		KBID:        kbID,
		DocumentRef: "physics.pdf",
		Ordinal:     3,
		HeadingPath: []string{"第三章", "3.2 干涉"},
		Text:        strings.Repeat("杨氏双缝干涉实验说明。", 30),
	}}}, fakeEmbedder{dim: 8}, fakeAccess{allow: true})
	exe.provider = provider
	userID := uuid.New()
	scopeID := "scope-physics"
	kb := kbDescriptor{ID: kbID, IntegrationMode: "exam", ExamScopeID: &scopeID}
	execCtx := tools.ExecutionContext{UserID: &userID}

	save := exe.executeProviderSaveQuestions(context.Background(), kb, map[string]any{
		"questions": []any{map[string]any{
			"knowledge_point_id": "kp-interference",
			"type":               "single_choice",
			"difficulty":         "medium",
			"stem":               "干涉题",
			"answer":             "A",
			"source_chunk_ids":   []any{chunkID.String()},
		}},
	}, execCtx)
	if save.Error != nil || save.ResultJSON["created_count"] != 1 {
		t.Fatalf("save failed: err=%+v result=%+v", save.Error, save.ResultJSON)
	}

	compose := exe.executeProviderComposePaper(context.Background(), kb, map[string]any{
		"name":                    "物理光学小测",
		"knowledge_point_ids":     []any{"kp-interference"},
		"total_count":             1,
		"type_distribution":       map[string]any{"single_choice": 1},
		"difficulty_distribution": map[string]any{"medium": 1},
		"confirmed":               true,
	}, execCtx)
	if compose.Error != nil {
		t.Fatalf("compose failed: %+v", compose.Error)
	}
	if compose.ResultJSON["saved"] != true || compose.ResultJSON["paper_id"] != "paper-1" {
		t.Fatalf("paper not saved: %+v", compose.ResultJSON)
	}
	if provider.paperBody["exam_scope_id"] != scopeID {
		t.Fatalf("paper save did not target scope: %+v", provider.paperBody)
	}
	var q map[string]any
	switch questions := provider.questionReq["questions"].(type) {
	case []map[string]any:
		if len(questions) == 1 {
			q = questions[0]
		}
	case []any:
		if len(questions) == 1 {
			q, _ = questions[0].(map[string]any)
		}
	}
	if q == nil {
		t.Fatalf("question save body missing: %+v", provider.questionReq)
	}
	if len(normalizeSourceSnippets(q["source_snippets"])) != 1 {
		t.Fatalf("question source snapshot not sent: %+v", q)
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

func TestProviderPaperPreviewPanelContainsConfirmationPrompt(t *testing.T) {
	panel := paperPreviewPanelFromMaps("期中卷", []map[string]any{
		{"id": "q-1", "stem": "光的干涉题", "type": "single_choice", "difficulty": "medium"},
	}, "# 期中卷", false)
	code, _ := panel["widget_code"].(string)
	if !strings.Contains(code, "sendPrompt('确认保存这份试卷')") {
		t.Fatalf("expected confirmation sendPrompt in widget code: %s", code)
	}
	if panel["kind"] != "paper_preview" {
		t.Fatalf("unexpected panel kind: %+v", panel)
	}
}

func TestSelectProviderQuestionsReportsTypeShortage(t *testing.T) {
	selected, warnings := selectProviderQuestions([]map[string]any{
		{"id": "q-1", "type": "single_choice"},
	}, 2, map[string]int{"single_choice": 2}, nil, nil, 0)
	if len(selected) != 0 {
		t.Fatalf("expected no selected questions, got %d", len(selected))
	}
	if len(warnings) != 1 || warnings[0]["type"] != "single_choice" {
		t.Fatalf("expected single_choice shortage, got %+v", warnings)
	}
}

func TestSelectProviderQuestionsSatisfiesDifficultyDistribution(t *testing.T) {
	selected, warnings := selectProviderQuestions([]map[string]any{
		{"id": "q-1", "type": "single_choice", "difficulty": "easy", "knowledge_point_id": "kp-1"},
		{"id": "q-2", "type": "single_choice", "difficulty": "medium", "knowledge_point_id": "kp-1"},
		{"id": "q-3", "type": "single_choice", "difficulty": "medium", "knowledge_point_id": "kp-2"},
	}, 2, nil, map[string]int{"medium": 2}, nil, 0)

	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %+v", warnings)
	}
	if len(selected) != 2 {
		t.Fatalf("expected two selected questions, got %d", len(selected))
	}
	for _, q := range selected {
		if q["difficulty"] != "medium" {
			t.Fatalf("expected only medium questions, got %+v", selected)
		}
	}
}

func TestRemapProviderIndicesPreservesOriginalPositions(t *testing.T) {
	items := remapProviderIndices([]map[string]any{
		{"index": float64(1), "id": "q-new"},
	}, []int{0, 2})
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d", len(items))
	}
	if items[0]["index"] != 2 {
		t.Fatalf("expected provider index 1 to map back to original index 2, got %+v", items[0])
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
