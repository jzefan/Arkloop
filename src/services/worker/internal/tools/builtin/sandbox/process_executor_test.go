package sandbox

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExecuteProcessCommandUsesProcessEndpoint(t *testing.T) {
	var gotPath string
	var gotBody processExecRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(processResponse{
			Status:     "running",
			ProcessRef: "proc_1",
			Cursor:     "0",
			NextCursor: "1",
			Stdout:     "hello",
			Items:      []processOutputItem{{Seq: 0, Stream: "stdout", Text: "hello"}},
		})
	}))
	defer server.Close()

	exec := NewToolExecutor(server.URL, "")
	result := exec.Execute(t.Context(), "exec_command", map[string]any{
		"command":    "echo hello",
		"mode":       "follow",
		"timeout_ms": 5000,
	}, testContext(), "")
	if result.Error != nil {
		t.Fatalf("unexpected error: %+v", result.Error)
	}
	if gotPath != "/v1/process/exec" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotBody.Mode != "follow" || gotBody.Command != "echo hello" {
		t.Fatalf("unexpected request body: %#v", gotBody)
	}
	if result.ResultJSON["process_ref"] != "proc_1" {
		t.Fatalf("unexpected result: %#v", result.ResultJSON)
	}
}

func TestExecuteContinueProcessUsesProcessRefAndCursor(t *testing.T) {
	var gotPath string
	var gotBody continueProcessRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(processResponse{
			Status:           "exited",
			ProcessRef:       "proc_1",
			Cursor:           "1",
			NextCursor:       "2",
			Stdout:           "done",
			ExitCode:         intPtr(0),
			AcceptedInputSeq: int64Ptr(7),
			Items:            []processOutputItem{{Seq: 1, Stream: "stdout", Text: "done"}},
		})
	}))
	defer server.Close()

	exec := NewToolExecutor(server.URL, "")
	result := exec.Execute(t.Context(), "continue_process", map[string]any{
		"process_ref": "proc_1",
		"cursor":      "1",
		"stdin_text":  "yes\n",
		"input_seq":   7,
	}, testContext(), "")
	if result.Error != nil {
		t.Fatalf("unexpected error: %+v", result.Error)
	}
	if gotPath != "/v1/process/continue" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotBody.ProcessRef != "proc_1" || gotBody.Cursor != "1" {
		t.Fatalf("unexpected request body: %#v", gotBody)
	}
	if gotBody.InputSeq == nil || *gotBody.InputSeq != 7 {
		t.Fatalf("unexpected input seq: %#v", gotBody.InputSeq)
	}
	if result.ResultJSON["accepted_input_seq"] != int64(7) {
		t.Fatalf("unexpected result: %#v", result.ResultJSON)
	}
}

func int64Ptr(value int64) *int64 {
	return &value
}
