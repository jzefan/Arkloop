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

type stubSandboxExecutor struct {
	result      tools.ExecutionResult
	args        map[string]any
	ctx         tools.ExecutionContext
	executeHook func(args map[string]any)
}

func (s *stubSandboxExecutor) Execute(_ context.Context, _ string, args map[string]any, execCtx tools.ExecutionContext, _ string) tools.ExecutionResult {
	s.args = args
	s.ctx = execCtx
	if s.executeHook != nil {
		s.executeHook(args)
	}
	return s.result
}

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

func TestExecuteGeneratesStandardPDFWithoutSandbox(t *testing.T) {
	store := &recordingStore{}
	sandboxCalled := false
	sandbox := &stubSandboxExecutor{
		result: tools.ExecutionResult{Error: &tools.ExecutionError{ErrorClass: tools.ErrorClassToolExecutionFailed, Message: "sandbox should not be used"}},
		executeHook: func(map[string]any) {
			sandboxCalled = true
		},
	}
	res := NewToolExecutor(store, sandbox).Execute(context.Background(), ToolName, map[string]any{
		"filename": "report.pdf",
		"title":    "正式报告",
		"content":  "# 正式报告\n\n> **综合评级：A 优秀**\n\n## 数据来源\n\n- [学校官网](https://example.edu.cn/report)\n\n| 荣誉 / 排名 | 级别 / 来源 |\n| --- | --- |\n| 双高计划 | 教育部 |\n\n正文包含中文、English 和 2026 年度指标。",
	}, tools.ExecutionContext{RunID: uuid.New()}, "call_1")

	if res.Error != nil {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	if sandboxCalled {
		t.Fatal("expected standard Go PDF renderer to avoid sandbox/browser dependency")
	}
	artifact := singleArtifact(t, res)
	if artifact["filename"] != "report.pdf" {
		t.Fatalf("unexpected filename: %#v", artifact["filename"])
	}
	for _, snippet := range [][]byte{
		[]byte("%PDF-1.4"),
		[]byte("/MediaBox [0 0 595 842]"),
		[]byte("/BaseFont /STSong-Light"),
		[]byte("raw:正式报告"),
		[]byte("raw:· 学校官网 (https://example.edu.cn/report)"),
		[]byte("raw:table-cell:双高计划"),
		[]byte("raw:Page 1 of"),
	} {
		if !bytes.Contains(store.data, snippet) {
			t.Fatalf("expected standard PDF to contain %q", snippet)
		}
	}
}

func TestExecuteDoesNotDependOnBrowserRuntime(t *testing.T) {
	store := &recordingStore{}
	sandbox := &stubSandboxExecutor{result: tools.ExecutionResult{Error: &tools.ExecutionError{ErrorClass: tools.ErrorClassToolExecutionFailed, Message: "Cannot find module 'playwright'"}}}
	res := NewToolExecutor(store, sandbox).Execute(context.Background(), ToolName, map[string]any{
		"filename": "report.pdf",
		"content":  "# 报告\n\n正文。",
	}, tools.ExecutionContext{RunID: uuid.New()}, "call_1")

	if res.Error != nil {
		t.Fatalf("expected standard renderer to work without browser runtime, got %v", res.Error)
	}
	if !bytes.HasPrefix(store.data, []byte("%PDF-1.4")) {
		t.Fatalf("expected standard PDF bytes")
	}
}

func TestExecuteIgnoresBrowserRendererFailures(t *testing.T) {
	store := &recordingStore{}
	sandbox := &stubSandboxExecutor{result: tools.ExecutionResult{Error: &tools.ExecutionError{ErrorClass: tools.ErrorClassToolExecutionFailed, Message: "page.pdf failed: navigation crashed"}}}
	res := NewToolExecutor(store, sandbox).Execute(context.Background(), ToolName, map[string]any{
		"filename": "report.pdf",
		"content":  "# Report",
	}, tools.ExecutionContext{RunID: uuid.New()}, "call_1")

	if res.Error != nil {
		t.Fatalf("expected standard renderer to ignore browser renderer failures, got %v", res.Error)
	}
	if !bytes.HasPrefix(store.data, []byte("%PDF-1.4")) {
		t.Fatalf("expected standard PDF bytes")
	}
}

func TestRenderStandardFormalReportPDFIncludesHeadingListTableAndFooter(t *testing.T) {
	pdf, err := renderStandardFormalReportPDF("正式报告", "## 数据来源\n\n- [学校官网](https://example.edu.cn)\n\n| 荣誉 / 排名 | 级别 / 来源 |\n| --- | --- |\n| 双高计划 | 教育部 |")
	if err != nil {
		t.Fatalf("render pdf: %v", err)
	}
	if !bytes.Contains(pdf, []byte("/F1 15")) {
		t.Fatalf("expected second-level heading font size in PDF stream")
	}
	if !bytes.Contains(pdf, []byte("raw:· 学校官网 (https://example.edu.cn)")) {
		t.Fatalf("expected bullet and link text in PDF stream")
	}
	if !bytes.Contains(pdf, []byte("raw:table-cell:双高计划")) {
		t.Fatalf("expected table cell text in PDF stream")
	}
	if !bytes.Contains(pdf, []byte("raw:Page 1 of")) {
		t.Fatalf("expected page footer in PDF stream")
	}
}

func TestRenderStandardFormalReportPDFDrawsTableGridAndCells(t *testing.T) {
	pdf, err := renderStandardFormalReportPDF("正式报告", "| 荣誉 / 排名 | 级别 / 来源 | 详情 |\n| --- | --- | --- |\n| 双高高职相关公开信息 | 苏州工业职业技术学院 | http://www.siit.cn |\n| 产教融合、校企合作或专业群建设线索 | 公开资料待复核 | 需后续核验 |")
	if err != nil {
		t.Fatalf("render pdf: %v", err)
	}
	for _, snippet := range [][]byte{
		[]byte(" re S\n"),
		[]byte("raw:table-cell:荣誉 / 排名"),
		[]byte("raw:table-cell:苏州工业职业技术学院"),
		[]byte("raw:table-cell:http://www.siit.cn"),
	} {
		if !bytes.Contains(pdf, snippet) {
			t.Fatalf("expected table PDF to contain %q", snippet)
		}
	}
	if bytes.Contains(pdf, []byte("raw:荣誉 / 排名    级别 / 来源")) {
		t.Fatalf("expected table to be rendered as cells, not a joined text row")
	}
}

func TestRenderStandardFormalReportPDFPaginatesLongReports(t *testing.T) {
	longBody := strings.Repeat("这是用于验证标准 PDF 分页能力的长段落，包含产教融合、校企合作、实训基地、人才培养质量和数据来源等信息。\n\n", 80)
	pdf, err := renderStandardFormalReportPDF("长报告", "# 长报告\n\n"+longBody)
	if err != nil {
		t.Fatalf("render pdf: %v", err)
	}
	if bytes.Count(pdf, []byte("/Type /Page ")) < 2 {
		t.Fatalf("expected multiple PDF pages")
	}
	if !bytes.Contains(pdf, []byte("raw:Page 2 of")) {
		t.Fatalf("expected second page footer")
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
