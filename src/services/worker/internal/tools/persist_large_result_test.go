package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestPersistLargeResult_UnderThreshold(t *testing.T) {
	dir := t.TempDir()
	execCtx := ExecutionContext{
		RunID:   uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		WorkDir: dir,
	}
	result := ExecutionResult{
		ResultJSON: map[string]any{"output": "small"},
	}
	out := PersistLargeResult(context.Background(), execCtx, "tc1", "test", result)
	if out.ResultJSON["persisted"] != nil {
		t.Fatalf("expected no persistence for small result")
	}
}

func TestPersistLargeResult_OverThreshold(t *testing.T) {
	dir := t.TempDir()
	execCtx := ExecutionContext{
		RunID:   uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		WorkDir: dir,
	}
	large := map[string]any{
		"output": string(make([]byte, PersistThreshold+1)),
	}
	result := ExecutionResult{
		ResultJSON: large,
	}
	out := PersistLargeResult(context.Background(), execCtx, "tc2", "test", result)
	if out.ResultJSON["persisted"] != true {
		t.Fatalf("expected persistence for large result")
	}
	filePath, _ := out.ResultJSON["filepath"].(string)
	if filePath != ".tool-outputs/tc2.txt" {
		t.Fatalf("unexpected filepath: %s", filePath)
	}
	ob, _ := out.ResultJSON["original_bytes"].(int)
	if ob <= PersistThreshold {
		t.Fatalf("expected original_bytes > threshold, got %d", ob)
	}

	data, err := os.ReadFile(filepath.Join(dir, filePath))
	if err != nil {
		t.Fatalf("read persisted file failed: %v", err)
	}
	var recovered map[string]any
	if err := json.Unmarshal(data, &recovered); err != nil {
		t.Fatalf("unmarshal persisted file failed: %v", err)
	}
	if recovered["output"] == nil {
		t.Fatalf("expected output in persisted file")
	}
}

func TestPersistLargeResult_KeepsMetadata(t *testing.T) {
	dir := t.TempDir()
	execCtx := ExecutionContext{
		RunID:   uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		WorkDir: dir,
	}
	large := map[string]any{
		"output":   string(make([]byte, PersistThreshold+1)),
		"exit_code": 42,
		"cwd":      "/tmp",
	}
	result := ExecutionResult{
		ResultJSON: large,
	}
	out := PersistLargeResult(context.Background(), execCtx, "tc3", "test", result)
	if out.ResultJSON["exit_code"] != 42 {
		t.Fatalf("expected exit_code preserved, got %v", out.ResultJSON["exit_code"])
	}
	if out.ResultJSON["cwd"] != "/tmp" {
		t.Fatalf("expected cwd preserved, got %v", out.ResultJSON["cwd"])
	}
}

func TestPersistLargeResult_Preview(t *testing.T) {
	tests := []struct {
		name   string
		raw    []byte
		budget int
		want   string
	}{
		{
			name:   "fits budget",
			raw:    []byte("hello world"),
			budget: 100,
			want:   "hello world",
		},
		{
			name:   "truncates at newline",
			raw:    []byte("line one\nline two\nline three\nline four"),
			budget: 25,
			want:   "line one\nline two\n...[truncated]",
		},
		{
			name:   "truncates hard when no newline in second half",
			raw:    []byte("abcdefghijklmnopqrstuvwxyz"),
			budget: 20,
			want:   "abcdefghijklmnopqrst\n...[truncated]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generatePreview(tt.raw, tt.budget)
			if got != tt.want {
				t.Fatalf("generatePreview() = %q, want %q", got, tt.want)
			}
		})
	}
}
