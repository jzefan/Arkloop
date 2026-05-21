package markdowntopdf

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"arkloop/services/shared/objectstore"
	"arkloop/services/worker/internal/tools"

	"github.com/google/uuid"
)

// -----------------------------------------------------------------------------
// Test doubles
// -----------------------------------------------------------------------------

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

// requireChrome skips the test if no Chromium/Chrome binary can be found in PATH.
func requireChrome(t *testing.T) {
	t.Helper()
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome-stable", "google-chrome"} {
		if _, err := exec.LookPath(name); err == nil {
			return
		}
	}
	// Also try ARK_CHROMIUM_PATH env override.
	if p := strings.TrimSpace(os.Getenv("ARK_CHROMIUM_PATH")); p != "" {
		if _, err := os.Stat(p); err == nil {
			return
		}
	}
	t.Skip("chromium/google-chrome not found; install Chromium or set ARK_CHROMIUM_PATH")
}

// -----------------------------------------------------------------------------
// Execute: argument validation
// -----------------------------------------------------------------------------

func TestExecuteRequiresFilename(t *testing.T) {
	store := &recordingStore{}
	res := NewToolExecutor(store).Execute(context.Background(), ToolName, map[string]any{
		"content": "# Report",
	}, tools.ExecutionContext{RunID: uuid.New()}, "call_1")

	if res.Error == nil {
		t.Fatal("expected error")
	}
	if res.Error.ErrorClass != errorArgsInvalid {
		t.Fatalf("unexpected error class: %s", res.Error.ErrorClass)
	}
}

func TestExecuteRequiresContent(t *testing.T) {
	store := &recordingStore{}
	res := NewToolExecutor(store).Execute(context.Background(), ToolName, map[string]any{
		"filename": "report.pdf",
	}, tools.ExecutionContext{RunID: uuid.New()}, "call_1")

	if res.Error == nil {
		t.Fatal("expected error")
	}
	if res.Error.ErrorClass != errorArgsInvalid {
		t.Fatalf("unexpected error class: %s", res.Error.ErrorClass)
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
	if res.Error.ErrorClass != errorArgsInvalid {
		t.Fatalf("unexpected error class: %s", res.Error.ErrorClass)
	}
}

// -----------------------------------------------------------------------------
// Filename normalisation
// -----------------------------------------------------------------------------

func TestNormalizePDFFilename(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"report", "report.pdf"},
		{"report.pdf", "report.pdf"},
		{"Report.PDF", "Report.PDF"},
		{"report.md", "report.pdf"},
		{"  foo bar  ", "foo bar.pdf"},
		{"", ""},
	}
	for _, c := range cases {
		if got := normalizePDFFilename(c.in); got != c.want {
			t.Errorf("normalizePDFFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExecuteNormalizesFilenameToPDF(t *testing.T) {
	requireChrome(t)
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

// -----------------------------------------------------------------------------
// Execute: happy path + artifact metadata
// -----------------------------------------------------------------------------

func TestExecuteStoresPDFArtifactMetadata(t *testing.T) {
	requireChrome(t)
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
	if artifact["mime_type"] != pdfMimeType {
		t.Fatalf("unexpected mime_type: %#v", artifact["mime_type"])
	}
	if artifact["title"] != "Formal Report" {
		t.Fatalf("unexpected title: %#v", artifact["title"])
	}
	if artifact["display"] != pdfDisplayDownload {
		t.Fatalf("unexpected display: %#v", artifact["display"])
	}
	if store.options.ContentType != pdfMimeType {
		t.Fatalf("unexpected store content type: %s", store.options.ContentType)
	}
	if !looksLikePDF(store.data) {
		t.Fatalf("expected PDF magic header, got %q", store.data[:min(len(store.data), 16)])
	}
	if store.options.Metadata[objectstore.ArtifactMetaThreadID] != threadID.String() {
		t.Fatalf("expected thread metadata, got %#v", store.options.Metadata)
	}
}

// -----------------------------------------------------------------------------
// Render: semantic feature coverage
// -----------------------------------------------------------------------------

func TestRenderProducesValidPDF(t *testing.T) {
	requireChrome(t)

	pdf, err := Render(RenderOptions{
		Title: "示例报告",
		Markdown: `# 示例报告

正文段落包含中文、English 混排，以及 **强调**、*斜体* 和 ` + "`inline code`" + `。

## 数据来源

- [学校官网](https://example.edu.cn/report)
- 公开年鉴

## 主要指标

| 维度 | 评分 | 备注 |
| --- | --- | --- |
| 产教融合 | A | 标杆 |
| 校企合作 | B+ | 稳步提升 |

### 说明

1. 第一条说明
2. 第二条说明
   1. 嵌套子项
   2. 另一个嵌套子项

> 引用块：需进一步核实。

---

` + "```" + `go
func main() {
    fmt.Println("Hello, 世界")
}
` + "```" + `
`,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !looksLikePDF(pdf) {
		t.Fatal("output is not a valid PDF")
	}
	if !bytes.Contains(pdf[max(0, len(pdf)-128):], []byte("%%EOF")) {
		t.Fatalf("expected trailing %%EOF marker")
	}
	if len(pdf) < 4096 {
		t.Fatalf("PDF unexpectedly small: %d bytes", len(pdf))
	}
}

func TestRenderPaginatesLongReports(t *testing.T) {
	requireChrome(t)
	var body strings.Builder
	for i := 0; i < 30; i++ {
		body.WriteString("这是第 ")
		body.WriteString(strings.Repeat("段落 ", 30))
		body.WriteString("。\n\n")
	}
	pdf, err := Render(RenderOptions{
		Title:    "长报告",
		Markdown: "# 长报告\n\n" + body.String(),
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	// Chrome embeds /Type /Page for each page in the PDF cross-reference table.
	pageObjCount := bytes.Count(pdf, []byte("/Type /Page\n")) +
		bytes.Count(pdf, []byte("/Type /Page ")) +
		bytes.Count(pdf, []byte("/Type /Page>"))
	if pageObjCount < 2 {
		t.Fatalf("expected multiple pages, got %d page objects; pdf len=%d", pageObjCount, len(pdf))
	}
}

// -----------------------------------------------------------------------------
// Image loading
// -----------------------------------------------------------------------------

func TestLoadImageFromDataURI(t *testing.T) {
	// 1x1 transparent PNG (base64).
	ref := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="
	img, err := LoadImage(context.Background(), ref, nil)
	if err != nil {
		t.Fatalf("LoadImage: %v", err)
	}
	if img.Format != "png" {
		t.Fatalf("expected png, got %s", img.Format)
	}
	if img.Width != 1 || img.Height != 1 {
		t.Fatalf("expected 1x1, got %dx%d", img.Width, img.Height)
	}
}

func TestLoadImageRejectsUnsupportedFormat(t *testing.T) {
	// Bogus data URI with text payload.
	_, err := LoadImage(context.Background(), "data:text/plain;base64,aGVsbG8=", nil)
	if err == nil {
		t.Fatal("expected error for non-image data URI")
	}
}

func TestLoadImageRejectsLocalOutsideAllowedRoots(t *testing.T) {
	dir := t.TempDir()
	png := dir + "/dummy.png"
	// Write a valid 1x1 PNG.
	bs, _ := base64DecodeStd("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=")
	if err := os.WriteFile(png, bs, 0o600); err != nil {
		t.Fatal(err)
	}
	// With nil allowedRoots, local reads are unconstrained.
	if _, err := LoadImage(context.Background(), png, nil); err != nil {
		t.Fatalf("LoadImage local with nil roots: %v", err)
	}
	otherRoot := t.TempDir()
	// Provide a root that does not contain the image: should be rejected.
	if _, err := LoadImage(context.Background(), png, []string{otherRoot}); err == nil {
		t.Fatalf("expected path containment error")
	}
	// Provide the correct root: should load successfully.
	img, err := LoadImage(context.Background(), png, []string{filepath.Dir(png)})
	if err != nil {
		t.Fatalf("LoadImage with correct root: %v", err)
	}
	if img.Format != "png" {
		t.Fatalf("expected png, got %s", img.Format)
	}
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

func looksLikePDF(data []byte) bool {
	return bytes.HasPrefix(data, []byte("%PDF-"))
}

func singleArtifact(t *testing.T, res tools.ExecutionResult) map[string]any {
	t.Helper()
	artifacts, ok := res.ResultJSON["artifacts"].([]map[string]any)
	if !ok || len(artifacts) != 1 {
		t.Fatalf("unexpected artifacts: %#v", res.ResultJSON["artifacts"])
	}
	return artifacts[0]
}

func base64DecodeStd(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
