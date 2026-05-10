package markdowntopdf

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"arkloop/services/shared/objectstore"
	sharedtoolruntime "arkloop/services/shared/toolruntime"
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

func TestExecuteUsesSandboxArtifactWhenAvailable(t *testing.T) {
	store := &recordingStore{}
	sandbox := &stubSandboxExecutor{result: tools.ExecutionResult{ResultJSON: map[string]any{
		"artifacts": []map[string]any{{
			"key":       "acc/run/1/rendered.pdf",
			"filename":  "rendered.pdf",
			"size":      int64(4096),
			"mime_type": "application/pdf",
		}},
	}}}
	res := NewToolExecutor(store, sandbox).Execute(context.Background(), ToolName, map[string]any{
		"filename": "report.pdf",
		"title":    "正式报告",
		"content":  "# 正式报告\n\nSee [学校官网](https://example.edu.cn/report) for details.",
	}, tools.ExecutionContext{RunID: uuid.New()}, "call_1")

	if res.Error != nil {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	if store.key != "" {
		t.Fatalf("expected sandbox path to skip local upload, got %s", store.key)
	}
	artifact := singleArtifact(t, res)
	if artifact["key"] != "acc/run/1/rendered.pdf" {
		t.Fatalf("unexpected artifact key: %#v", artifact["key"])
	}
	env, ok := sandbox.args["env"].(map[string]any)
	if !ok {
		t.Fatalf("expected sandbox env args, got %#v", sandbox.args["env"])
	}
	if _, ok := env[sandboxHTMLBase64EnvKey].(string); !ok {
		t.Fatalf("expected html payload env var, got %#v", env)
	}
	if _, ok := env[hostOutputEnvKey].(string); !ok {
		t.Fatalf("expected host output env var, got %#v", env)
	}
	profiles, ok := sandbox.ctx.Budget["sandbox_profiles"].(map[string]any)
	if !ok || profiles[sandboxExecToolName] != sandboxExecTier {
		t.Fatalf("expected browser sandbox profile override, got %#v", sandbox.ctx.Budget)
	}
}

func TestExecuteUploadsHostRenderedPDFWhenLocalFileExists(t *testing.T) {
	store := &recordingStore{}
	sandbox := &stubSandboxExecutor{
		result: tools.ExecutionResult{ResultJSON: map[string]any{"exit_code": 0}},
		executeHook: func(args map[string]any) {
			env, ok := args["env"].(map[string]any)
			if !ok {
				return
			}
			hostPath, _ := env[hostOutputEnvKey].(string)
			if hostPath == "" {
				return
			}
			_ = os.WriteFile(hostPath, []byte("%PDF-1.4 host"), 0o600)
		},
	}
	res := NewToolExecutor(store, sandbox).Execute(context.Background(), ToolName, map[string]any{
		"filename": "report.pdf",
		"title":    "正式报告",
		"content":  "# 正式报告\n\n正文。",
	}, tools.ExecutionContext{RunID: uuid.New(), RuntimeSnapshot: &sharedtoolruntime.RuntimeSnapshot{}}, "call_1")

	if res.Error != nil {
		t.Fatalf("unexpected error: %v", res.Error)
	}
	if !bytes.HasPrefix(store.data, []byte("%PDF-1.4 host")) {
		t.Fatalf("expected uploaded host PDF bytes, got %q", store.data)
	}
}

func TestExecuteFallsBackOnlyWhenRuntimeUnavailable(t *testing.T) {
	store := &recordingStore{}
	sandbox := &stubSandboxExecutor{result: tools.ExecutionResult{Error: &tools.ExecutionError{ErrorClass: tools.ErrorClassToolExecutionFailed, Message: "Cannot find module 'playwright'"}}}
	res := NewToolExecutor(store, sandbox).Execute(context.Background(), ToolName, map[string]any{
		"filename": "report.pdf",
		"content":  "# Report",
	}, tools.ExecutionContext{RunID: uuid.New()}, "call_1")

	if res.Error != nil {
		t.Fatalf("expected legacy fallback on runtime unavailable, got %v", res.Error)
	}
	if !bytes.HasPrefix(store.data, []byte("%PDF-1.4")) {
		t.Fatalf("expected fallback legacy PDF bytes")
	}
}

func TestExecuteReturnsErrorWhenRendererFailsButRuntimeExists(t *testing.T) {
	store := &recordingStore{}
	sandbox := &stubSandboxExecutor{result: tools.ExecutionResult{Error: &tools.ExecutionError{ErrorClass: tools.ErrorClassToolExecutionFailed, Message: "page.pdf failed: navigation crashed"}}}
	res := NewToolExecutor(store, sandbox).Execute(context.Background(), ToolName, map[string]any{
		"filename": "report.pdf",
		"content":  "# Report",
	}, tools.ExecutionContext{RunID: uuid.New()}, "call_1")

	if res.Error == nil {
		t.Fatal("expected render error")
	}
	if store.key != "" {
		t.Fatalf("expected no fallback upload on runtime-present failure, got %s", store.key)
	}
}

func TestRenderFormalReportHTMLUsesRequestedFonts(t *testing.T) {
	html, err := renderFormalReportHTML("正式报告", "# 正式报告\n\n正文\n\n```go\nfmt.Println(\"hi\")\n```")
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	if !strings.Contains(html, `"STFangsong", "FangSong", "仿宋", "STFangsong-Light", "Songti SC", serif`) {
		t.Fatalf("expected chinese body font stack in html: %s", html)
	}
	if !strings.Contains(html, `Menlo, Monaco, "Courier New", monospace`) {
		t.Fatalf("expected code font stack in html: %s", html)
	}
	if !strings.Contains(html, "@page") || !strings.Contains(html, "size: A4") {
		t.Fatalf("expected print page css in html: %s", html)
	}
}

func TestRenderFormalReportHTMLRendersMarkdownStructure(t *testing.T) {
	html, err := renderFormalReportHTML("正式报告", "## 数据来源\n\n- [学校官网](https://example.edu.cn)\n\n| 列 | 值 |\n| --- | --- |\n| A | B |")
	if err != nil {
		t.Fatalf("render html: %v", err)
	}
	for _, snippet := range []string{"<h2>数据来源</h2>", "<li><a href=\"https://example.edu.cn\">学校官网</a></li>", "<table>"} {
		if !strings.Contains(html, snippet) {
			t.Fatalf("expected snippet %q in html: %s", snippet, html)
		}
	}
}

func TestRenderLegacyFormalReportPDFIncludesHeadingAndBulletStyling(t *testing.T) {
	pdf := renderLegacyFormalReportPDF("正式报告", "## 数据来源\n\n- [学校官网](https://example.edu.cn)")
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
