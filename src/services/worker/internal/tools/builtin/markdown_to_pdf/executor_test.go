package markdowntopdf

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"arkloop/services/shared/objectstore"
	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
)

type recordingStore struct {
	key     string
	data    []byte
	options objectstore.PutOptions
}

func (s *recordingStore) PutObject(_ context.Context, key string, data []byte, options objectstore.PutOptions) error {
	s.key = key
	s.data = append([]byte(nil), data...)
	s.options = options
	return nil
}

func TestExecuteRequiresFilename(t *testing.T) {
	store := &recordingStore{}
	res := NewToolExecutor(store).Execute(context.Background(), ToolName, map[string]any{
		"content": "# Report",
	}, tools.ExecutionContext{RunID: uuid.New()}, "call_1")

	if res.Error == nil {
		t.Fatal("expected error")
	}
	if res.Error.ErrorClass != "tool.args_invalid" {
		t.Fatalf("unexpected error class: %s", res.Error.ErrorClass)
	}
}

func TestExecuteNormalizesFilenameToPDF(t *testing.T) {
	store := &recordingStore{}
	runID := uuid.New()
	accountID := uuid.New()
	res := NewToolExecutor(store).Execute(context.Background(), ToolName, map[string]any{
		"filename": "formal-report.md",
		"content":  "# Formal Report",
		"template": "formal_report",
	}, tools.ExecutionContext{RunID: runID, AccountID: &accountID}, "call_1")

	if res.Error != nil {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	artifact := singleArtifact(t, res)
	if artifact["filename"] != "formal-report.pdf" {
		t.Fatalf("unexpected filename: %#v", artifact["filename"])
	}
	if !strings.HasSuffix(store.key, "/formal-report.pdf") {
		t.Fatalf("unexpected object key: %s", store.key)
	}
}

func TestExecuteRejectsUnsupportedTemplate(t *testing.T) {
	store := &recordingStore{}
	res := NewToolExecutor(store).Execute(context.Background(), ToolName, map[string]any{
		"filename": "report.pdf",
		"content":  "# Report",
		"template": "poster",
	}, tools.ExecutionContext{RunID: uuid.New()}, "call_1")

	if res.Error == nil {
		t.Fatal("expected error")
	}
	if res.Error.ErrorClass != "tool.args_invalid" {
		t.Fatalf("unexpected error class: %s", res.Error.ErrorClass)
	}
}

func TestExecuteStoresPDFArtifactMetadata(t *testing.T) {
	store := &recordingStore{}
	runID := uuid.New()
	accountID := uuid.New()
	threadID := uuid.New()
	res := NewToolExecutor(store).Execute(context.Background(), ToolName, map[string]any{
		"filename": "report.pdf",
		"title":    "Formal Report",
		"content":  "# Report\n\nBody text.",
	}, tools.ExecutionContext{RunID: runID, AccountID: &accountID, ThreadID: &threadID}, "call_1")

	if res.Error != nil {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	artifact := singleArtifact(t, res)
	if artifact["mime_type"] != "application/pdf" {
		t.Fatalf("unexpected mime_type: %#v", artifact["mime_type"])
	}
	if artifact["title"] != "Formal Report" {
		t.Fatalf("unexpected title: %#v", artifact["title"])
	}
	if artifact["display"] != "download" {
		t.Fatalf("unexpected display: %#v", artifact["display"])
	}
	if store.options.ContentType != "application/pdf" {
		t.Fatalf("unexpected store content type: %s", store.options.ContentType)
	}
	if !bytes.HasPrefix(store.data, []byte("%PDF-1.4")) {
		t.Fatalf("expected PDF header, got %q", store.data[:min(len(store.data), 16)])
	}
	if store.options.Metadata[objectstore.ArtifactMetaThreadID] != threadID.String() {
		t.Fatalf("expected thread metadata, got %#v", store.options.Metadata)
	}
}

func TestExecuteRepresentsMarkdownLinksInPDFBytes(t *testing.T) {
	store := &recordingStore{}
	res := NewToolExecutor(store).Execute(context.Background(), ToolName, map[string]any{
		"filename": "report.pdf",
		"content":  "See [学校官网](https://example.edu.cn/report) for details.",
	}, tools.ExecutionContext{RunID: uuid.New()}, "call_1")

	if res.Error != nil {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	if !bytes.Contains(store.data, []byte("https://example.edu.cn/report")) {
		t.Fatalf("expected link URL to be represented in PDF bytes")
	}
}

func TestRenderFormalReportPDFIncludesHeadingAndBulletStyling(t *testing.T) {
	pdf := renderFormalReportPDF("正式报告", "## 数据来源\n\n- [学校官网](https://example.edu.cn)")
	if !bytes.Contains(pdf, []byte("/F1 15 Tf")) {
		t.Fatalf("expected second-level heading font size in PDF stream")
	}
	if !bytes.Contains(pdf, []byte("raw:· 学校官网 (https://example.edu.cn)")) {
		t.Fatalf("expected bullet and link text in PDF stream")
	}
}

func singleArtifact(t *testing.T, res tools.ExecutionResult) map[string]any {
	t.Helper()
	artifacts, ok := res.ResultJSON["artifacts"].([]map[string]any)
	if !ok || len(artifacts) != 1 {
		t.Fatalf("unexpected artifacts: %#v", res.ResultJSON["artifacts"])
	}
	return artifacts[0]
}
