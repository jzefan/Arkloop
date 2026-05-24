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

func TestMain(m *testing.M) {
	// Ensure tests don't pick up real env from shell
	os.Unsetenv("EXAM_BASE_URL")
	os.Unsetenv("ARKLOOP_API_INTERNAL_URL")
	os.Unsetenv("ARKLOOP_INTERNAL_SERVICE_TOKEN")
	os.Exit(m.Run())
}

func ptrUUID(u uuid.UUID) *uuid.UUID { return &u }
