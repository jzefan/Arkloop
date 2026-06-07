package exam

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
)

// minimalExam is a httptest fake covering only the endpoints exam_build/save_questions hits.
func minimalExam(t *testing.T, behavior func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(behavior))
	t.Cleanup(srv.Close)
	return srv
}

// minimalArkLoop returns a httptest server that mints OIDC tokens for the worker.
func minimalArkLoop(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/internal/oauth/issue") {
			json.NewEncoder(w).Encode(map[string]string{"access_token": "test-tok"})
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func setupBuilderExec(t *testing.T, examURL string) *BuilderExecutor {
	arkSrv := minimalArkLoop(t)
	t.Setenv("EXAM_BASE_URL", examURL)
	t.Setenv("ARKLOOP_API_INTERNAL_URL", arkSrv.URL)
	t.Setenv("ARKLOOP_INTERNAL_SERVICE_TOKEN", "fake-service-token")
	client, err := NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return NewBuilderExecutor(client)
}

func TestBuildQuestions_RejectsEmptySeedIDs(t *testing.T) {
	exec := setupBuilderExec(t, "http://unused")
	res := exec.Execute(context.Background(), ToolNameBuildQuestions, map[string]any{
		"seed_question_ids": []any{}, // empty
		"skill_key":         "gyn-medical-exam",
	}, tools.ExecutionContext{UserID: ptrUUID(uuid.New())}, "")
	if res.Error == nil || res.Error.ErrorClass != "exam.seed_required" {
		t.Errorf("expected exam.seed_required, got: %+v", res.Error)
	}
}

func TestBuildQuestions_FetchesSeedPatternTag(t *testing.T) {
	srv := minimalExam(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/questions/") {
			id := strings.TrimPrefix(r.URL.Path, "/api/questions/")
			json.NewEncoder(w).Encode(map[string]any{
				"id":          id,
				"pattern_tag": "A2",
				"type":        "single_choice",
				"difficulty":  "medium",
				"stem":        "stem for " + id,
			})
			return
		}
		w.WriteHeader(404)
	})
	exec := setupBuilderExec(t, srv.URL)
	uid := uuid.New()
	res := exec.Execute(context.Background(), ToolNameBuildQuestions, map[string]any{
		"seed_question_ids": []any{"q-1", "q-2"},
		"skill_key":         "gyn-medical-exam",
		"count":             3.0, // float64 from JSON
	}, tools.ExecutionContext{UserID: &uid}, "")
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
	got := res.ResultJSON
	if got["expected_pattern_tag"] != "A2" {
		t.Errorf("expected_pattern_tag: got %v", got["expected_pattern_tag"])
	}
	summaries, ok := got["seed_summaries"].([]map[string]any)
	if !ok || len(summaries) != 2 {
		t.Errorf("seed_summaries: got %v", got["seed_summaries"])
	}
	instr, _ := got["instruction"].(string)
	if !strings.Contains(instr, "A2") {
		t.Errorf("instruction missing pattern_tag: %s", instr)
	}
}

func TestBuildQuestions_RejectsMixedSeedPatternTags(t *testing.T) {
	srv := minimalExam(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/questions/q-A2" {
			json.NewEncoder(w).Encode(map[string]any{"id": "q-A2", "pattern_tag": "A2"})
			return
		}
		if r.URL.Path == "/api/questions/q-A3" {
			json.NewEncoder(w).Encode(map[string]any{"id": "q-A3", "pattern_tag": "A3"})
			return
		}
		w.WriteHeader(404)
	})
	exec := setupBuilderExec(t, srv.URL)
	uid := uuid.New()
	res := exec.Execute(context.Background(), ToolNameBuildQuestions, map[string]any{
		"seed_question_ids": []any{"q-A2", "q-A3"},
		"skill_key":         "gyn-medical-exam",
	}, tools.ExecutionContext{UserID: &uid}, "")
	if res.Error == nil || res.Error.ErrorClass != "exam.seed_pattern_mismatch" {
		t.Errorf("expected exam.seed_pattern_mismatch, got: %+v", res.Error)
	}
}

func TestSaveQuestions_EnforcesExpectedPatternTag(t *testing.T) {
	calls := 0
	srv := minimalExam(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/questions/batch" {
			calls++
			var body struct {
				Questions []map[string]any `json:"questions"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			created := make([]map[string]any, len(body.Questions))
			for i := range body.Questions {
				created[i] = map[string]any{"index": float64(i), "id": "id-x"}
			}
			json.NewEncoder(w).Encode(map[string]any{"created": created, "failed": []any{}})
			return
		}
		w.WriteHeader(404)
	})
	exec := setupBuilderExec(t, srv.URL)
	uid := uuid.New()
	res := exec.Execute(context.Background(), ToolNameSaveQuestions, map[string]any{
		"expected_pattern_tag": "A2",
		"questions": []any{
			map[string]any{"stem": "ok", "pattern_tag": "A2", "type": "single_choice", "answer": "A"},
			map[string]any{"stem": "wrong tag", "pattern_tag": "A1", "type": "single_choice", "answer": "B"},
			map[string]any{"stem": "missing tag", "type": "single_choice", "answer": "C"},
		},
	}, tools.ExecutionContext{UserID: &uid}, "")
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
	got := res.ResultJSON
	createdCount, _ := got["created_count"].(int)
	failedCount, _ := got["failed_count"].(int)
	if createdCount != 1 {
		t.Errorf("expected 1 created, got %d", createdCount)
	}
	if failedCount != 2 {
		t.Errorf("expected 2 failed (pattern_tag_mismatch x2), got %d", failedCount)
	}
	failed, _ := got["failed"].([]map[string]any)
	for _, f := range failed {
		if f["error_code"] != "pattern_tag_mismatch" {
			t.Errorf("expected error_code=pattern_tag_mismatch, got %v", f["error_code"])
		}
	}
	if calls != 1 {
		t.Errorf("expected 1 call to exam (only good question), got %d", calls)
	}
}

func TestSaveQuestions_NoExpectedTag_PassThrough(t *testing.T) {
	srv := minimalExam(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.Path == "/api/questions/batch" {
			var body struct {
				Questions []map[string]any `json:"questions"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			// No client-side filtering when expected_pattern_tag is missing.
			created := make([]map[string]any, len(body.Questions))
			for i := range body.Questions {
				created[i] = map[string]any{"index": float64(i), "id": "id-x"}
			}
			json.NewEncoder(w).Encode(map[string]any{"created": created, "failed": []any{}})
			return
		}
		w.WriteHeader(404)
	})
	exec := setupBuilderExec(t, srv.URL)
	uid := uuid.New()
	res := exec.Execute(context.Background(), ToolNameSaveQuestions, map[string]any{
		"questions": []any{
			map[string]any{"stem": "any tag", "pattern_tag": "A1", "type": "single_choice", "answer": "A"},
			map[string]any{"stem": "no tag", "type": "single_choice", "answer": "B"},
		},
	}, tools.ExecutionContext{UserID: &uid}, "")
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
	got := res.ResultJSON
	if got["created_count"].(int) != 2 {
		t.Errorf("expected 2 created (no filter), got %v", got["created_count"])
	}
	if got["failed_count"].(int) != 0 {
		t.Errorf("expected 0 failed, got %v", got["failed_count"])
	}
}

func TestBuildPaper_HonorsDistribution(t *testing.T) {
	pool := []map[string]any{
		{"id": "sc-1", "type": "single_choice", "difficulty": "easy", "knowledge_point_id": "kp-1"},
		{"id": "sc-2", "type": "single_choice", "difficulty": "medium", "knowledge_point_id": "kp-1"},
		{"id": "fi-1", "type": "fill_in", "difficulty": "easy", "knowledge_point_id": "kp-1"},
		{"id": "fi-2", "type": "fill_in", "difficulty": "medium", "knowledge_point_id": "kp-1"},
	}
	var posted []string
	paperCalls := 0
	srv := minimalExam(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/questions":
			json.NewEncoder(w).Encode(map[string]any{"items": pool, "total": len(pool)})
		case r.Method == "POST" && r.URL.Path == "/api/papers":
			paperCalls++
			var body struct {
				QuestionIDs []string `json:"question_ids"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			posted = body.QuestionIDs
			json.NewEncoder(w).Encode(map[string]any{"id": "paper-1", "question_count": len(body.QuestionIDs)})
		default:
			w.WriteHeader(404)
		}
	})
	exec := setupBuilderExec(t, srv.URL)
	uid := uuid.New()
	res := exec.Execute(context.Background(), ToolNameBuildPaper, map[string]any{
		"exam_scope_id":       "scope-1",
		"name":                "妇产科测验",
		"knowledge_point_ids": []any{"kp-1"},
		"total_count":         3.0,
		"type_distribution":   map[string]any{"single_choice": 2.0, "fill_in": 1.0},
	}, tools.ExecutionContext{UserID: &uid}, "")
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
	if paperCalls != 1 {
		t.Fatalf("expected 1 POST /api/papers, got %d", paperCalls)
	}
	if got, _ := res.ResultJSON["question_count"].(int); got != 3 {
		t.Errorf("question_count: want 3 got %v", res.ResultJSON["question_count"])
	}
	if len(posted) != 3 {
		t.Fatalf("posted ids: want 3 got %v", posted)
	}
	sc, fi := 0, 0
	for _, id := range posted {
		switch {
		case strings.HasPrefix(id, "sc-"):
			sc++
		case strings.HasPrefix(id, "fi-"):
			fi++
		}
	}
	if sc != 2 || fi != 1 {
		t.Errorf("distribution not honored: single_choice=%d fill_in=%d (want 2/1), ids=%v", sc, fi, posted)
	}
}

func TestBuildPaper_HonorsKnowledgePointDistribution(t *testing.T) {
	// Pool spans two knowledge points; spec asks for 2 from kp-A and 1 from kp-B.
	poolA := []map[string]any{
		{"id": "a-1", "type": "single_choice", "difficulty": "easy", "knowledge_point_id": "kp-A"},
		{"id": "a-2", "type": "single_choice", "difficulty": "easy", "knowledge_point_id": "kp-A"},
		{"id": "a-3", "type": "single_choice", "difficulty": "easy", "knowledge_point_id": "kp-A"},
	}
	poolB := []map[string]any{
		{"id": "b-1", "type": "single_choice", "difficulty": "easy", "knowledge_point_id": "kp-B"},
		{"id": "b-2", "type": "single_choice", "difficulty": "easy", "knowledge_point_id": "kp-B"},
	}
	var posted []string
	srv := minimalExam(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/questions":
			items := poolA
			if r.URL.Query().Get("knowledge_point_id") == "kp-B" {
				items = poolB
			}
			json.NewEncoder(w).Encode(map[string]any{"items": items, "total": len(items)})
		case r.Method == "POST" && r.URL.Path == "/api/papers":
			var body struct {
				QuestionIDs []string `json:"question_ids"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			posted = body.QuestionIDs
			json.NewEncoder(w).Encode(map[string]any{"id": "paper-1"})
		default:
			w.WriteHeader(404)
		}
	})
	exec := setupBuilderExec(t, srv.URL)
	uid := uuid.New()
	res := exec.Execute(context.Background(), ToolNameBuildPaper, map[string]any{
		"exam_scope_id":                "scope-1",
		"name":                         "跨知识点卷",
		"knowledge_point_ids":          []any{"kp-A", "kp-B"},
		"total_count":                  3.0,
		"knowledge_point_distribution": map[string]any{"kp-A": 2.0, "kp-B": 1.0},
	}, tools.ExecutionContext{UserID: &uid}, "")
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
	if len(posted) != 3 {
		t.Fatalf("posted ids: want 3 got %v", posted)
	}
	a, b := 0, 0
	for _, id := range posted {
		switch {
		case strings.HasPrefix(id, "a-"):
			a++
		case strings.HasPrefix(id, "b-"):
			b++
		}
	}
	if a != 2 || b != 1 {
		t.Errorf("kp distribution not honored: kp-A=%d kp-B=%d (want 2/1), ids=%v", a, b, posted)
	}
}

func TestBuildPaper_ReturnsShortageWithoutSaving(t *testing.T) {
	pool := []map[string]any{
		{"id": "q-1", "type": "single_choice", "difficulty": "easy", "knowledge_point_id": "kp-1"},
		{"id": "q-2", "type": "single_choice", "difficulty": "easy", "knowledge_point_id": "kp-1"},
	}
	paperCalls := 0
	srv := minimalExam(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/questions":
			json.NewEncoder(w).Encode(map[string]any{"items": pool, "total": len(pool)})
		case r.Method == "POST" && r.URL.Path == "/api/papers":
			paperCalls++
			json.NewEncoder(w).Encode(map[string]any{"id": "paper-x"})
		default:
			w.WriteHeader(404)
		}
	})
	exec := setupBuilderExec(t, srv.URL)
	uid := uuid.New()
	res := exec.Execute(context.Background(), ToolNameBuildPaper, map[string]any{
		"exam_scope_id":       "scope-1",
		"name":                "测验",
		"knowledge_point_ids": []any{"kp-1"},
		"total_count":         5.0,
	}, tools.ExecutionContext{UserID: &uid}, "")
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
	warnings, ok := res.ResultJSON["shortage_warnings"].([]map[string]any)
	if !ok || len(warnings) == 0 {
		t.Fatalf("expected shortage_warnings, got %v", res.ResultJSON["shortage_warnings"])
	}
	if paperCalls != 0 {
		t.Errorf("expected no POST /api/papers on shortage, got %d", paperCalls)
	}
}

func TestMain(m *testing.M) {
	// Ensure tests don't pick up real env from shell
	os.Unsetenv("EXAM_BASE_URL")
	os.Unsetenv("ARKLOOP_API_INTERNAL_URL")
	os.Unsetenv("ARKLOOP_INTERNAL_SERVICE_TOKEN")
	os.Exit(m.Run())
}

func ptrUUID(u uuid.UUID) *uuid.UUID { return &u }
