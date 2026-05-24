package examstore

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_ListQuestions_TransmitsPatternTagFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pattern_tag") != "A2" {
			t.Errorf("missing pattern_tag: %s", r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("bad auth header: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-ArkLoop-API-Version") != "1" {
			t.Errorf("missing version header")
		}
		json.NewEncoder(w).Encode(QListResp{Items: []QItem{{ID: "q1", PatternTag: "A2"}}, Total: 1})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	resp, err := c.ListQuestions(context.Background(), "tok", "kp1", ListFilter{PatternTag: "A2"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 || resp.Items[0].ID != "q1" {
		t.Errorf("unexpected resp: %+v", resp)
	}
}

func TestClient_CreateQuestionsBatch_PartialSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(BatchResp{
			Created: []BatchCreated{{Index: 0, ID: "new-1"}},
			Failed:  []BatchFailed{{Index: 1, ErrorCode: "validation_error", ErrorMessage: "missing answer"}},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	resp, err := c.CreateQuestionsBatch(context.Background(), "tok", []DraftReq{{Stem: "q1"}, {Stem: "q2"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Created) != 1 || resp.Created[0].ID != "new-1" {
		t.Errorf("created: %+v", resp.Created)
	}
	if len(resp.Failed) != 1 || resp.Failed[0].ErrorCode != "validation_error" {
		t.Errorf("failed: %+v", resp.Failed)
	}
}

func TestClient_5xx_RetriesThenFails(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(500)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.retry = RetryPolicy{MaxAttempts: 3, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}
	_, err := c.ListExamScopes(context.Background(), "tok")
	if err == nil {
		t.Fatal("expected error")
	}
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
	if _, ok := err.(*ServerError); !ok {
		t.Errorf("expected ServerError, got %T: %v", err, err)
	}
}

func TestClient_5xx_RetriesAndSucceeds(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 2 {
			w.WriteHeader(500)
			return
		}
		json.NewEncoder(w).Encode(ExamScopeListResp{Items: []ExamScopeItem{
			{ID: "s1", Type: "major", Code: "100201", DisplayName: "临床医学"},
		}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.retry = RetryPolicy{MaxAttempts: 3, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}
	resp, err := c.ListExamScopes(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 1 || resp.Items[0].ID != "s1" || resp.Items[0].DisplayName != "临床医学" {
		t.Errorf("unexpected: %+v", resp)
	}
}

func TestClient_401_NoRetry(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(401)
		w.Write([]byte("unauthorized"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	c.retry = RetryPolicy{MaxAttempts: 3, BaseDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond}
	_, err := c.ListExamScopes(context.Background(), "tok")
	if err == nil {
		t.Fatal("expected error")
	}
	if atomic.LoadInt32(&attempts) != 1 {
		t.Errorf("expected 1 attempt (no retry on 401), got %d", attempts)
	}
	if _, ok := err.(*AuthError); !ok {
		t.Errorf("expected AuthError, got %T", err)
	}
}

func TestClient_ConcurrencyLimit(t *testing.T) {
	var maxInFlight int32
	var current int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&current, 1)
		for {
			old := atomic.LoadInt32(&maxInFlight)
			if n <= old {
				break
			}
			if atomic.CompareAndSwapInt32(&maxInFlight, old, n) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt32(&current, -1)
		json.NewEncoder(w).Encode(ExamScopeListResp{})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	// sema is 4, fire 8 concurrent requests
	done := make(chan struct{}, 8)
	for i := 0; i < 8; i++ {
		go func() {
			c.ListExamScopes(context.Background(), "tok")
			done <- struct{}{}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
	if atomic.LoadInt32(&maxInFlight) > 4 {
		t.Errorf("max in-flight %d exceeded semaphore limit 4", maxInFlight)
	}
}

// TestClient_ListExamScopes_MultiLevelShape verifies that GET /api/exam-scopes
// returns the new 3-level hierarchy shape (major / direction / topic) per Q9.
func TestClient_ListExamScopes_MultiLevelShape(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		majorID := "scope-major-1"
		dirID := "scope-dir-1"
		json.NewEncoder(w).Encode(ExamScopeListResp{Items: []ExamScopeItem{
			{ID: majorID, Type: "major", Code: "100201", DisplayName: "临床医学", ParentID: nil},
			{ID: dirID, Type: "direction", Code: "0201-1", DisplayName: "妇产科方向", ParentID: &majorID},
			{ID: "scope-topic-1", Type: "topic", Code: "0201", DisplayName: "妇产科学", ParentID: &dirID},
		}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	resp, err := c.ListExamScopes(context.Background(), "tok")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/exam-scopes" {
		t.Errorf("want path /api/exam-scopes, got %s", gotPath)
	}
	if len(resp.Items) != 3 {
		t.Fatalf("want 3 items, got %d", len(resp.Items))
	}
	if resp.Items[0].Type != "major" || resp.Items[0].ParentID != nil {
		t.Errorf("major mismatch: %+v", resp.Items[0])
	}
	if resp.Items[1].Type != "direction" || resp.Items[1].ParentID == nil || *resp.Items[1].ParentID != "scope-major-1" {
		t.Errorf("direction mismatch: %+v", resp.Items[1])
	}
	if resp.Items[2].Type != "topic" || resp.Items[2].DisplayName != "妇产科学" {
		t.Errorf("topic mismatch: %+v", resp.Items[2])
	}
}

// TestClient_ListKnowledgePoints_UsesExamScopeIDParam verifies the renamed query param.
func TestClient_ListKnowledgePoints_UsesExamScopeIDParam(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode(KPListResp{Items: []KPItem{
			{ID: "kp1", ExamScopeID: "scope-1", Code: "K-3.2-INTF", DisplayName: "光的干涉"},
		}, Total: 1})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	resp, err := c.ListKnowledgePoints(context.Background(), "tok", "scope-1", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if gotQuery != "exam_scope_id=scope-1" {
		t.Errorf("want query exam_scope_id=scope-1, got %s", gotQuery)
	}
	if resp.Items[0].Code != "K-3.2-INTF" || resp.Items[0].DisplayName != "光的干涉" {
		t.Errorf("KP fields mismatch: %+v", resp.Items[0])
	}
}
