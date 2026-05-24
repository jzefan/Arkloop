package examstore_test

// E2E smoke test: simulates the full M2b flow against a mock exam backend
// using httptest. Covers: list exam scopes → list KPs → list seed questions
// → save batch (with pattern_tag mismatch) → create paper.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"arkloop/services/shared/questionstore"
	"arkloop/services/shared/questionstore/examstore"
)

// mockExam is a minimal exam backend for E2E flow.
type mockExam struct {
	// Mock state
	scopes       []map[string]any
	kps          []map[string]any
	questions    []map[string]any
	createdQs    []map[string]any
	createdPaper map[string]any
}

func newMockExam() *mockExam {
	return &mockExam{
		scopes: []map[string]any{
			{"id": "scope-medicine", "type": "major", "name": "临床医学", "code": "med"},
			{"id": "scope-gyn", "type": "topic", "name": "妇产科学", "code": "gyn", "parent_id": "scope-medicine"},
		},
		kps: []map[string]any{
			{"id": "kp-ectopic", "code": "kp_ectopic", "display_name": "异位妊娠", "depth": 0, "sort_order": 0},
		},
		questions: []map[string]any{
			{
				"id":                 "q-seed-1",
				"knowledge_point_id": "kp-ectopic",
				"type":               "single_choice",
				"difficulty":         "medium",
				"pattern_tag":        "A2",
				"stem":               "女，28岁。停经50天，下腹痛伴阴道少量流血2天...",
				"options": []map[string]string{
					{"key": "A", "text": "异位妊娠"},
					{"key": "B", "text": "先兆流产"},
					{"key": "C", "text": "葡萄胎"},
					{"key": "D", "text": "盆腔炎"},
					{"key": "E", "text": "卵巢囊肿"},
				},
				"answer":      "A",
				"explanation": "宫颈举痛 + 单侧附件压痛 + 尿妊娠试验阳性 = 异位妊娠典型表现",
			},
		},
	}
}

func (m *mockExam) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/exam-scopes":
			json.NewEncoder(w).Encode(map[string]any{"items": m.scopes})

		case r.Method == "GET" && r.URL.Path == "/api/knowledge-points":
			json.NewEncoder(w).Encode(map[string]any{"items": m.kps, "total": len(m.kps)})

		case r.Method == "GET" && r.URL.Path == "/api/questions":
			// Optional pattern_tag filter
			pt := r.URL.Query().Get("pattern_tag")
			items := m.questions
			if pt != "" {
				filtered := make([]map[string]any, 0)
				for _, q := range m.questions {
					if q["pattern_tag"] == pt {
						filtered = append(filtered, q)
					}
				}
				items = filtered
			}
			json.NewEncoder(w).Encode(map[string]any{"items": items, "total": len(items)})

		case r.Method == "POST" && r.URL.Path == "/api/questions/batch":
			var body struct {
				Questions []map[string]any `json:"questions"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			created := make([]map[string]any, 0)
			failed := make([]map[string]any, 0)
			for i, q := range body.Questions {
				// Reject questions with empty stem
				if stem, _ := q["stem"].(string); strings.TrimSpace(stem) == "" {
					failed = append(failed, map[string]any{
						"index":         i,
						"error_code":    "validation_error",
						"error_message": "stem is required",
					})
					continue
				}
				m.createdQs = append(m.createdQs, q)
				created = append(created, map[string]any{"index": i, "id": "q-new-" + strings.Repeat("x", i+1)})
			}
			json.NewEncoder(w).Encode(map[string]any{"created": created, "failed": failed})

		case r.Method == "POST" && r.URL.Path == "/api/papers":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			ids, _ := body["question_ids"].([]any)
			m.createdPaper = body
			json.NewEncoder(w).Encode(map[string]any{
				"id":             "paper-1",
				"name":           body["name"],
				"question_count": len(ids),
			})

		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"not found"}`))
		}
	}
}

func TestE2E_FullFlow_ListScopes_ListKP_SaveQuestions_CreatePaper(t *testing.T) {
	mock := newMockExam()
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	c := examstore.NewClient(srv.URL)
	tokenSource := func(ctx context.Context) (string, error) { return "test-tok", nil }
	store := examstore.NewStore(c, tokenSource, "scope-gyn")

	ctx := context.Background()

	// Step 1: list exam scopes
	scopesResp, err := c.ListExamScopes(ctx, "test-tok")
	if err != nil {
		t.Fatalf("list scopes: %v", err)
	}
	if len(scopesResp.Items) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(scopesResp.Items))
	}

	// Step 2: list knowledge points
	kps, err := store.ListKnowledgePoints(ctx, "scope-gyn")
	if err != nil {
		t.Fatalf("list kps: %v", err)
	}
	if len(kps) != 1 || kps[0].ID != "kp-ectopic" {
		t.Errorf("kps: %+v", kps)
	}

	// Step 3: list seed questions for the knowledge point with pattern_tag=A2 filter
	seedQs, _, err := store.ListReferenceQuestions(ctx, "kp-ectopic", questionstore.ListFilter{PatternTag: "A2"})
	if err != nil {
		t.Fatalf("list seed: %v", err)
	}
	if len(seedQs) != 1 || seedQs[0].PatternTag != "A2" {
		t.Errorf("seed: %+v", seedQs)
	}

	// Step 4: save a batch with one valid + one invalid question
	drafts := []questionstore.QuestionDraft{
		{
			KnowledgePointID: "kp-ectopic",
			Type:             "single_choice",
			Difficulty:       "medium",
			Stem:             "女，30岁。停经45天，下腹剧痛...",
			Answer:           "A",
			PatternTag:       "A2",
			Options: []questionstore.QuestionOption{
				{Key: "A", Text: "异位妊娠破裂"},
				{Key: "B", Text: "卵巢囊肿扭转"},
				{Key: "C", Text: "急性阑尾炎"},
				{Key: "D", Text: "盆腔感染"},
				{Key: "E", Text: "肠梗阻"},
			},
		},
		{
			KnowledgePointID: "kp-ectopic",
			Type:             "single_choice",
			Stem:             "", // invalid: empty stem
			Answer:           "X",
			PatternTag:       "A2",
		},
	}
	saved, err := store.SaveQuestions(ctx, drafts)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(saved.Created) != 1 {
		t.Errorf("expected 1 created, got %d", len(saved.Created))
	}
	if len(saved.Failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(saved.Failed))
	}
	if len(saved.Failed) > 0 && saved.Failed[0].ErrorCode != "validation_error" {
		t.Errorf("failed: %+v", saved.Failed)
	}

	// Step 5: build a paper
	paperID, err := store.SavePaper(ctx, "妇产科期中卷",  "scope-gyn", questionstore.PaperSpec{
		TotalCount: 1,
	}, []string{"q-new-x"})
	if err != nil {
		t.Fatalf("save paper: %v", err)
	}
	if paperID != "paper-1" {
		t.Errorf("paper id: %s", paperID)
	}
}
