package markdowntopdf

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"arkloop/services/shared/objectstore"
	"arkloop/services/worker/internal/tools"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

const (
	errorArgsInvalid  = "tool.args_invalid"
	errorUploadFailed = "tool.upload_failed"

	pdfMimeType             = "application/pdf"
	sandboxExecToolName     = "exec_command"
	sandboxExecTimeoutMs    = 120000
	sandboxExecTier         = "browser"
	sandboxHTMLBase64EnvKey = "ARKLOOP_PDF_HTML_BASE64"
	sandboxOutputEnvKey     = "ARKLOOP_PDF_OUTPUT"
	hostOutputEnvKey        = "ARKLOOP_PDF_HOST_OUTPUT"
)

const sandboxRenderCommand = `node -e 'const fs = require("fs"); const path = require("path"); const { chromium } = require("playwright"); (async()=>{ const html = Buffer.from(process.env.ARKLOOP_PDF_HTML_BASE64 || "", "base64").toString("utf8"); const output = process.env.ARKLOOP_PDF_OUTPUT || "/tmp/output/report.pdf"; const hostOutput = process.env.ARKLOOP_PDF_HOST_OUTPUT || ""; fs.mkdirSync(path.dirname(output), { recursive: true }); if (hostOutput) fs.mkdirSync(path.dirname(hostOutput), { recursive: true }); const browser = await chromium.launch({ headless: true, executablePath: process.env.AGENT_BROWSER_CHROME_PATH || "/usr/bin/chromium", args: ["--no-sandbox", "--disable-dev-shm-usage"] }); try { const page = await browser.newPage(); await page.setContent(html, { waitUntil: "load" }); await page.emulateMedia({ media: "print" }); await page.pdf({ path: output, format: "A4", printBackground: true, preferCSSPageSize: true, margin: { top: "18mm", right: "16mm", bottom: "20mm", left: "16mm" } }); if (hostOutput) fs.copyFileSync(output, hostOutput); } finally { await browser.close(); } })().catch((err)=>{ console.error(err && err.stack ? err.stack : String(err)); process.exit(1); });'`

var (
	markdownLinkPattern = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reportMarkdown      = goldmark.New(goldmark.WithExtensions(extension.GFM))
)

type ToolExecutor struct {
	store interface {
		PutObject(ctx context.Context, key string, data []byte, options objectstore.PutOptions) error
	}
	sandboxExecutor tools.Executor
}

type artifactResult struct {
	Key      string `json:"key"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
}

func NewToolExecutor(store interface {
	PutObject(ctx context.Context, key string, data []byte, options objectstore.PutOptions) error
}, sandboxExecutor ...tools.Executor) *ToolExecutor {
	executor := &ToolExecutor{store: store}
	if len(sandboxExecutor) > 0 {
		executor.sandboxExecutor = sandboxExecutor[0]
	}
	return executor
}

func (e *ToolExecutor) Execute(
	ctx context.Context,
	_ string,
	args map[string]any,
	execCtx tools.ExecutionContext,
	_ string,
) tools.ExecutionResult {
	started := time.Now()

	if blocked, isBlocked := tools.PlanModeBlocked(execCtx.PipelineRC, started); isBlocked {
		return blocked
	}

	filename, _ := args["filename"].(string)
	filename = normalizePDFFilename(filename)
	if filename == "" {
		return errResult(errorArgsInvalid, "parameter filename is required", started)
	}

	content, _ := args["content"].(string)
	if strings.TrimSpace(content) == "" {
		return errResult(errorArgsInvalid, "parameter content is required", started)
	}

	template, _ := args["template"].(string)
	template = strings.TrimSpace(template)
	if template != "" && template != "formal_report" {
		return errResult(errorArgsInvalid, "parameter template must be formal_report", started)
	}

	title, _ := args["title"].(string)
	title = strings.TrimSpace(title)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	}

	htmlDoc, err := renderFormalReportHTML(title, content)
	if err != nil {
		return errResult(tools.ErrorClassToolExecutionFailed, fmt.Sprintf("render html failed: %s", err.Error()), started)
	}

	if sandboxResult, ok := e.tryRenderWithSandbox(ctx, execCtx, filename, title, htmlDoc, started); ok {
		return sandboxResult
	}

	pdfBytes := renderLegacyFormalReportPDF(title, content)
	return e.uploadLocalPDF(ctx, execCtx, filename, title, pdfBytes, started)
}

func (e *ToolExecutor) tryRenderWithSandbox(
	ctx context.Context,
	execCtx tools.ExecutionContext,
	filename string,
	title string,
	htmlDoc string,
	started time.Time,
) (tools.ExecutionResult, bool) {
	if e == nil || e.sandboxExecutor == nil {
		return tools.ExecutionResult{}, false
	}

	outputFilename := sandboxOutputFilename(filename, started)
	sandboxOutputPath := filepath.ToSlash(filepath.Join("/tmp/output", outputFilename))
	hostOutputPath := filepath.Join(os.TempDir(), outputFilename)
	sandboxArgs := map[string]any{
		"command":    sandboxRenderCommand,
		"mode":       "buffered",
		"timeout_ms": sandboxExecTimeoutMs,
		"env": map[string]any{
			sandboxHTMLBase64EnvKey: base64.StdEncoding.EncodeToString([]byte(htmlDoc)),
			sandboxOutputEnvKey:     sandboxOutputPath,
			hostOutputEnvKey:        hostOutputPath,
		},
	}
	sandboxCtx := execCtx
	sandboxCtx.Budget = cloneBudgetWithSandboxProfile(execCtx.Budget, sandboxExecToolName, sandboxExecTier)
	result := e.sandboxExecutor.Execute(ctx, sandboxExecToolName, sandboxArgs, sandboxCtx, "")
	if result.Error != nil {
		if isRuntimeUnavailable(result.Error, result.ResultJSON) {
			return tools.ExecutionResult{}, false
		}
		return result, true
	}

	if hostResult, ok := e.tryReadHostRenderedPDF(ctx, execCtx, filename, title, hostOutputPath, started); ok {
		return hostResult, true
	}

	artifacts, ok := decodeArtifactResults(result.ResultJSON["artifacts"])
	if !ok || len(artifacts) == 0 {
		exitCode := intValue(result.ResultJSON["exit_code"])
		if exitCode != 0 {
			message := renderFailureMessage(result.ResultJSON, exitCode)
			err := &tools.ExecutionError{ErrorClass: tools.ErrorClassToolExecutionFailed, Message: message}
			if isRuntimeUnavailable(err, result.ResultJSON) {
				return tools.ExecutionResult{}, false
			}
			return tools.ExecutionResult{Error: err, DurationMs: durationMs(started)}, true
		}
		return errResult(tools.ErrorClassToolExecutionFailed, "PDF render produced no artifact", started), true
	}

	normalized := normalizeArtifactResults(artifacts, filename, title)
	if len(normalized) == 0 {
		return errResult(tools.ErrorClassToolExecutionFailed, "PDF render produced no downloadable artifact", started), true
	}

	return tools.ExecutionResult{
		ResultJSON: map[string]any{
			"artifacts": normalized,
		},
		DurationMs: durationMs(started),
	}, true
}

func (e *ToolExecutor) tryReadHostRenderedPDF(
	ctx context.Context,
	execCtx tools.ExecutionContext,
	filename string,
	title string,
	hostOutputPath string,
	started time.Time,
) (tools.ExecutionResult, bool) {
	data, err := os.ReadFile(hostOutputPath)
	if err != nil {
		return tools.ExecutionResult{}, false
	}
	defer func() { _ = os.Remove(hostOutputPath) }()
	if len(data) == 0 {
		return errResult(tools.ErrorClassToolExecutionFailed, "desktop PDF render produced empty output", started), true
	}
	return e.uploadLocalPDF(ctx, execCtx, filename, title, data, started), true
}

func (e *ToolExecutor) uploadLocalPDF(
	ctx context.Context,
	execCtx tools.ExecutionContext,
	filename string,
	title string,
	pdfBytes []byte,
	started time.Time,
) tools.ExecutionResult {
	accountPrefix := "_anonymous"
	if execCtx.AccountID != nil {
		accountPrefix = execCtx.AccountID.String()
	}
	key := fmt.Sprintf("%s/%s/%s", accountPrefix, execCtx.RunID.String(), filename)
	var threadID *string
	if execCtx.ThreadID != nil {
		value := execCtx.ThreadID.String()
		threadID = &value
	}
	metadata := objectstore.ArtifactMetadata(objectstore.ArtifactOwnerKindRun, execCtx.RunID.String(), accountPrefix, threadID)

	if err := e.store.PutObject(ctx, key, pdfBytes, objectstore.PutOptions{ContentType: pdfMimeType, Metadata: metadata}); err != nil {
		return errResult(errorUploadFailed, fmt.Sprintf("upload failed: %s", err.Error()), started)
	}

	return tools.ExecutionResult{
		ResultJSON: map[string]any{
			"artifacts": []map[string]any{
				{
					"key":       key,
					"filename":  filename,
					"size":      len(pdfBytes),
					"mime_type": pdfMimeType,
					"title":     title,
					"display":   "download",
				},
			},
		},
		DurationMs: durationMs(started),
	}
}

func normalizePDFFilename(filename string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return ""
	}
	ext := filepath.Ext(filename)
	if strings.EqualFold(ext, ".pdf") {
		return filename
	}
	if ext == "" {
		return filename + ".pdf"
	}
	return strings.TrimSuffix(filename, ext) + ".pdf"
}

func renderFormalReportHTML(title, markdown string) (string, error) {
	var body bytes.Buffer
	if err := reportMarkdown.Convert([]byte(markdown), &body); err != nil {
		return "", err
	}

	safeTitle := stdhtml.EscapeString(strings.TrimSpace(title))
	var titleBlock strings.Builder
	if shouldInjectTitle(title, markdown) {
		titleBlock.WriteString(`<header class="report-header"><h1 class="report-title">`)
		titleBlock.WriteString(safeTitle)
		titleBlock.WriteString(`</h1></header>`)
	}

	return `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>` + safeTitle + `</title>
  <style>
    :root {
      color-scheme: light;
      --report-text: #24292f;
      --report-muted: #59636e;
      --report-border: #d0d7de;
      --report-border-muted: #eaeef2;
      --report-surface: #ffffff;
      --report-surface-subtle: #f6f8fa;
      --report-link: #0969da;
      --report-code-bg: rgba(175, 184, 193, 0.2);
      --report-shadow: 0 1px 2px rgba(31, 35, 40, 0.04);
      --report-font-body: "STFangsong", "FangSong", "仿宋", "STFangsong-Light", "Songti SC", serif;
      --report-font-code: Menlo, Monaco, "Courier New", monospace;
    }
    @page {
      size: A4;
      margin: 18mm 16mm 20mm;
    }
    * {
      box-sizing: border-box;
    }
    html {
      background: var(--report-surface);
    }
    body {
      margin: 0;
      color: var(--report-text);
      background: var(--report-surface);
      font-family: var(--report-font-body);
      font-size: 12pt;
      line-height: 1.75;
      -webkit-font-smoothing: antialiased;
      text-rendering: optimizeLegibility;
      word-break: break-word;
    }
    main {
      width: 100%;
      margin: 0 auto;
    }
    .report-header {
      margin: 0 0 18pt;
      padding-bottom: 10pt;
      border-bottom: 1px solid var(--report-border);
      page-break-after: avoid;
    }
    .report-title {
      margin: 0;
      font-size: 22pt;
      font-weight: 700;
      line-height: 1.3;
      letter-spacing: 0;
    }
    .markdown-body {
      color: var(--report-text);
      font-family: var(--report-font-body);
      font-size: 12pt;
      line-height: 1.75;
    }
    .markdown-body > *:first-child {
      margin-top: 0;
    }
    .markdown-body > *:last-child {
      margin-bottom: 0;
    }
    .markdown-body h1,
    .markdown-body h2,
    .markdown-body h3,
    .markdown-body h4,
    .markdown-body h5,
    .markdown-body h6 {
      margin-top: 1.6em;
      margin-bottom: 0.7em;
      font-weight: 700;
      line-height: 1.35;
      page-break-after: avoid;
      page-break-inside: avoid;
      letter-spacing: 0;
    }
    .markdown-body h1 {
      font-size: 20pt;
      border-bottom: 1px solid var(--report-border);
      padding-bottom: 0.3em;
    }
    .markdown-body h2 {
      font-size: 16pt;
      border-bottom: 1px solid var(--report-border-muted);
      padding-bottom: 0.25em;
    }
    .markdown-body h3 {
      font-size: 14pt;
    }
    .markdown-body p,
    .markdown-body ul,
    .markdown-body ol,
    .markdown-body blockquote,
    .markdown-body pre,
    .markdown-body table {
      margin-top: 0;
      margin-bottom: 1em;
    }
    .markdown-body ul,
    .markdown-body ol {
      padding-left: 1.5em;
    }
    .markdown-body li + li {
      margin-top: 0.3em;
    }
    .markdown-body li > ul,
    .markdown-body li > ol {
      margin-top: 0.35em;
      margin-bottom: 0.35em;
    }
    .markdown-body a {
      color: var(--report-link);
      text-decoration: none;
      overflow-wrap: anywhere;
    }
    .markdown-body strong {
      font-weight: 700;
    }
    .markdown-body em {
      font-style: italic;
    }
    .markdown-body code,
    .markdown-body pre,
    .markdown-body kbd,
    .markdown-body samp {
      font-family: var(--report-font-code);
    }
    .markdown-body code {
      padding: 0.14em 0.35em;
      border-radius: 6px;
      background: var(--report-code-bg);
      font-size: 0.92em;
    }
    .markdown-body pre {
      padding: 14px 16px;
      overflow: hidden;
      background: var(--report-surface-subtle);
      border: 1px solid var(--report-border-muted);
      border-radius: 10px;
      box-shadow: var(--report-shadow);
      page-break-inside: avoid;
      white-space: pre-wrap;
      word-break: break-word;
    }
    .markdown-body pre code {
      padding: 0;
      background: transparent;
      border-radius: 0;
      font-size: 0.9em;
      line-height: 1.65;
    }
    .markdown-body blockquote {
      margin-left: 0;
      padding: 0.2em 1em;
      color: var(--report-muted);
      border-left: 4px solid var(--report-border);
      background: var(--report-surface-subtle);
    }
    .markdown-body table {
      width: 100%;
      border-collapse: collapse;
      font-size: 10.5pt;
      page-break-inside: avoid;
    }
    .markdown-body th,
    .markdown-body td {
      padding: 8px 10px;
      border: 1px solid var(--report-border);
      text-align: left;
      vertical-align: top;
    }
    .markdown-body th {
      background: var(--report-surface-subtle);
      font-weight: 700;
    }
    .markdown-body tr:nth-child(even) td {
      background: #fbfcfd;
    }
    .markdown-body hr {
      border: 0;
      border-top: 1px solid var(--report-border);
      margin: 1.5em 0;
    }
    .markdown-body img,
    .markdown-body svg {
      max-width: 100%;
      page-break-inside: avoid;
    }
  </style>
</head>
<body>
  <main>
    ` + titleBlock.String() + `
    <article class="markdown-body">` + body.String() + `</article>
  </main>
</body>
</html>`, nil
}

func shouldInjectTitle(title, markdown string) bool {
	title = normalizeHeadingText(title)
	if title == "" {
		return false
	}
	for _, rawLine := range strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# ") && normalizeHeadingText(strings.TrimSpace(line[2:])) == title {
			return false
		}
		return true
	}
	return true
}

func normalizeHeadingText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func sandboxOutputFilename(filename string, started time.Time) string {
	base := filepath.Base(filename)
	if base == "." || base == "" {
		base = "report.pdf"
	}
	return fmt.Sprintf("%d-%s", started.UnixNano(), base)
}

func cloneBudgetWithSandboxProfile(budget map[string]any, key string, tier string) map[string]any {
	cloned := map[string]any{}
	for existingKey, value := range budget {
		cloned[existingKey] = value
	}
	profiles := map[string]any{}
	if rawProfiles, ok := cloned["sandbox_profiles"].(map[string]any); ok {
		for existingKey, value := range rawProfiles {
			profiles[existingKey] = value
		}
	}
	profiles[key] = tier
	cloned["sandbox_profiles"] = profiles
	return cloned
}

func isSandboxUnavailable(err *tools.ExecutionError) bool {
	if err == nil {
		return false
	}
	switch strings.TrimSpace(err.ErrorClass) {
	case "config.missing", "tool.unavailable", "tool.not_registered":
		return true
	default:
		return false
	}
}

func isRuntimeUnavailable(err *tools.ExecutionError, resultJSON map[string]any) bool {
	if isSandboxUnavailable(err) {
		return true
	}
	message := ""
	if err != nil {
		message = err.Message
	}
	if strings.TrimSpace(message) == "" {
		message = strings.TrimSpace(strings.Join([]string{stringValue(resultJSON["stderr"]), stringValue(resultJSON["stdout"])}, "\n"))
	}
	lower := strings.ToLower(message)
	for _, marker := range []string{
		"cannot find module 'playwright'",
		"cannot find module \"playwright\"",
		"playwright",
		"chromium",
		"agent_browser_chrome_path",
		"node: not found",
		"node.exe",
		"is not recognized as an internal or external command",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func renderFailureMessage(resultJSON map[string]any, exitCode int) string {
	message := strings.TrimSpace(strings.Join([]string{stringValue(resultJSON["stderr"]), stringValue(resultJSON["stdout"])}, "\n"))
	if message == "" {
		message = fmt.Sprintf("PDF render failed with exit code %d", exitCode)
	}
	return message
}

func decodeArtifactResults(raw any) ([]artifactResult, bool) {
	if raw == nil {
		return nil, false
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var artifacts []artifactResult
	if err := json.Unmarshal(payload, &artifacts); err != nil {
		return nil, false
	}
	return artifacts, true
}

func normalizeArtifactResults(artifacts []artifactResult, filename string, title string) []map[string]any {
	out := make([]map[string]any, 0, len(artifacts))
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.Key) == "" {
			continue
		}
		mimeType := strings.TrimSpace(artifact.MimeType)
		if mimeType == "" {
			mimeType = pdfMimeType
		}
		out = append(out, map[string]any{
			"key":       artifact.Key,
			"filename":  filename,
			"size":      artifact.Size,
			"mime_type": mimeType,
			"title":     title,
			"display":   "download",
		})
	}
	return out
}

func stringValue(raw any) string {
	value, _ := raw.(string)
	return value
}

func intValue(raw any) int {
	switch value := raw.(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func renderLegacyFormalReportPDF(title, markdown string) []byte {
	lines := markdownToReportLines(title, markdown)
	if len(lines) == 0 {
		lines = []reportLine{{Text: title, FontSize: 18, Leading: 22, IsBold: true}}
	}

	const linesPerPage = 42
	var pages [][]reportLine
	for i := 0; i < len(lines); {
		end := i + linesPerPage
		if end > len(lines) {
			end = len(lines)
		}
		// Try not to split tables or lists if possible (simple heuristic)
		pages = append(pages, lines[i:end])
		i = end
	}

	var objects []string
	objects = append(objects, "<< /Type /Catalog /Pages 2 0 R >>")
	var kids []string
	for i := range pages {
		pageObj := 3 + i*2
		kids = append(kids, fmt.Sprintf("%d 0 R", pageObj))
	}
	objects = append(objects, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages)))
	fontObj := 3 + len(pages)*2

	for i, pageLines := range pages {
		pageObj := 3 + i*2
		contentObj := pageObj + 1
		objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", fontObj, contentObj))
		stream := buildPageContentStream(pageLines, i+1, len(pages))
		objects = append(objects, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	}
	objects = append(objects,
		fmt.Sprintf("<< /Type /Font /Subtype /Type0 /BaseFont /STFangsong-Light /Encoding /UniGB-UCS2-H /DescendantFonts [%d 0 R] >>", fontObj+1),
		"<< /Type /Font /Subtype /CIDFontType0 /BaseFont /STFangsong-Light /CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 2 >> >>",
	)

	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for i, obj := range objects {
		offsets[i+1] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xrefOffset := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n", len(objects)+1)
	out.WriteString("0000000000 65535 f \n")
	for i := 1; i < len(offsets); i++ {
		fmt.Fprintf(&out, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset)
	return out.Bytes()
}

type reportLine struct {
	Text     string
	FontSize int
	Leading  int
	IsBold   bool
	Indent   int
	IsBullet bool
}

func buildPageContentStream(lines []reportLine, pageNumber, pageCount int) string {
	var b strings.Builder
	b.WriteString("BT\n/F1 12 Tf\n72 770 Td\n")
	lastX := 72.0
	lastY := 770.0

	for _, line := range lines {
		fontSize := line.FontSize
		if fontSize <= 0 {
			fontSize = 12
		}
		leading := line.Leading
		if leading <= 0 {
			leading = fontSize + 4
		}

		// Reset X if indented
		targetX := 72.0 + float64(line.Indent)*12.0
		if targetX != lastX {
			fmt.Fprintf(&b, "%f %f Td\n", targetX-lastX, 0.0)
			lastX = targetX
		}

		fmt.Fprintf(&b, "/F1 %d Tf\n%d TL\n", fontSize, leading)
		if line.IsBold {
			b.WriteString("2 Tr 0.4 w\n")
		} else {
			b.WriteString("0 Tr 0 w\n")
		}

		text := line.Text
		if line.IsBullet {
			text = "· " + text
		}

		if strings.TrimSpace(text) == "" {
			fmt.Fprintf(&b, "0 -%d Td\n", leading/2)
			lastY -= float64(leading) / 2.0
			continue
		}

		fmt.Fprintf(&b, "%% raw:%s\n<%s> Tj\nT*\n", escapePDFComment(text), encodePDFTextHex(text))
		lastY -= float64(leading)
	}

	// Footer
	b.WriteString("0 Tr 0 w\n/F1 10 Tf\n")
	footerY := 50.0
	fmt.Fprintf(&b, "%f %f Td\n", 72.0-lastX, footerY-lastY)
	pageText := fmt.Sprintf("Page %d of %d", pageNumber, pageCount)
	fmt.Fprintf(&b, "<%s> Tj\n", encodePDFTextHex(pageText))
	b.WriteString("ET")
	return b.String()
}

func markdownToReportLines(title, markdown string) []reportLine {
	reader := text.NewReader([]byte(markdown))
	parser := reportMarkdown.Parser()
	doc := parser.Parse(reader)

	var lines []reportLine
	if shouldInjectTitle(title, markdown) {
		lines = append(lines,
			reportLine{Text: title, FontSize: 18, Leading: 24, IsBold: true},
			reportLine{},
		)
	}

	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := n.(type) {
		case *ast.Heading:
			level := node.Level
			fontSize := 17
			if level == 2 {
				fontSize = 15
			} else if level >= 3 {
				fontSize = 13
			}
			text := string(node.Text(reader.Source()))
			lines = append(lines, reportLine{Text: text, FontSize: fontSize, Leading: fontSize + 6, IsBold: true})
			lines = append(lines, reportLine{})
			return ast.WalkSkipChildren, nil

		case *ast.Paragraph:
			content := string(n.Text(reader.Source()))
			lines = appendWrappedLines(lines, content, 80, 12, 18, false, 0, false)
			lines = append(lines, reportLine{})
			return ast.WalkSkipChildren, nil

		case *ast.ListItem:
			// Simple list handling
			content := string(n.Text(reader.Source()))
			lines = appendWrappedLines(lines, content, 75, 12, 18, false, 1, true)
			return ast.WalkSkipChildren, nil

		case *ast.FencedCodeBlock, *ast.CodeBlock:
			lines = append(lines, reportLine{Text: "--- Code ---", FontSize: 10, Leading: 14, IsBold: true})
			for i := 0; i < node.Lines().Len(); i++ {
				line := node.Lines().At(i)
				lines = append(lines, reportLine{Text: string(line.Value(reader.Source())), FontSize: 10, Leading: 14})
			}
			lines = append(lines, reportLine{})
			return ast.WalkSkipChildren, nil

		case *ast.Blockquote:
			content := string(n.Text(reader.Source()))
			lines = appendWrappedLines(lines, "> "+content, 75, 12, 18, false, 1, false)
			lines = append(lines, reportLine{})
			return ast.WalkSkipChildren, nil

		case *ast.ThematicBreak:
			lines = append(lines, reportLine{Text: "________________________________________________________________________________", FontSize: 10, Leading: 12})
			lines = append(lines, reportLine{})
			return ast.WalkSkipChildren, nil

		case *extast.Table:
			// Simple table rendering: row by row with separator
			lines = append(lines, reportLine{Text: "[表格数据]", FontSize: 10, Leading: 14, IsBold: true})
			for row := node.FirstChild(); row != nil; row = row.NextSibling() {
				var cells []string
				for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
					cells = append(cells, string(cell.Text(reader.Source())))
				}
				rowText := "| " + strings.Join(cells, " | ") + " |"
				lines = appendWrappedLines(lines, rowText, 80, 10, 14, false, 0, false)
			}
			lines = append(lines, reportLine{})
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})

	return lines
}

func appendWrappedLines(lines []reportLine, line string, maxRunes int, fontSize int, leading int, isBold bool, indent int, isBullet bool) []reportLine {
	line = strings.ReplaceAll(line, "\n", " ")
	line = markdownLinkPattern.ReplaceAllString(line, "$1 ($2)")
	line = strings.ReplaceAll(line, "**", "")
	line = strings.ReplaceAll(line, "__", "")
	line = strings.ReplaceAll(line, "*", "")
	line = strings.ReplaceAll(line, "_", "")
	line = strings.ReplaceAll(line, "`", "")
	line = insertCJKLatinSpacing(line)

	// CJK 文本每行容纳字符数约为拉丁文本的 55%
	if hasCJK(line) {
		maxRunes = int(float64(maxRunes) * 0.55)
		if maxRunes < 20 {
			maxRunes = 20
		}
	}

	if utf8.RuneCountInString(line) <= maxRunes {
		return append(lines, reportLine{Text: line, FontSize: fontSize, Leading: leading, IsBold: isBold, Indent: indent, IsBullet: isBullet})
	}

	runes := []rune(line)
	cjk := hasCJK(line)
	first := true
	for len(runes) > 0 {
		cut := maxRunes
		if len(runes) < cut {
			cut = len(runes)
		} else if !cjk {
			// 拉丁文本优先在空格处断行
			for cut > maxRunes/2 && cut < len(runes) && runes[cut] != ' ' {
				cut--
			}
			if cut <= maxRunes/2 {
				cut = maxRunes
			}
		}
		// CJK 文本在任意字符处断行，无需搜索空格

		bullet := isBullet && first
		lines = append(lines, reportLine{Text: strings.TrimSpace(string(runes[:cut])), FontSize: fontSize, Leading: leading, IsBold: isBold, Indent: indent, IsBullet: bullet})
		runes = runes[cut:]
		first = false
	}
	return lines
}

func hasCJK(s string) bool {
	for _, r := range s {
		if r >= 0x2E80 && r <= 0x9FFF {
			return true
		}
		if r >= 0xAC00 && r <= 0xD7AF {
			return true
		} // Hangul
		if r >= 0xF900 && r <= 0xFAFF {
			return true
		} // CJK Compatibility
		if r >= 0xFE30 && r <= 0xFE4F {
			return true
		} // CJK Compatibility Forms
		if r >= 0xFF00 && r <= 0xFFEF {
			return true
		} // Halfwidth/Fullwidth
	}
	return false
}

func isCJK(r rune) bool {
	return (r >= 0x2E80 && r <= 0x9FFF) ||
		(r >= 0xAC00 && r <= 0xD7AF) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0xFE30 && r <= 0xFE4F) ||
		(r >= 0xFF00 && r <= 0xFFEF)
}

func insertCJKLatinSpacing(s string) string {
	if !hasCJK(s) {
		return s
	}
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(runes) + len(runes)/5)
	for i, r := range runes {
		b.WriteRune(r)
		if i < len(runes)-1 {
			next := runes[i+1]
			cjkCur := isCJK(r)
			cjkNext := isCJK(next)
			// CJK ↔ 拉丁字符之间插入微间距
			if cjkCur != cjkNext && r != ' ' && next != ' ' && r != ' ' && next != ' ' {
				b.WriteRune(' ')
			}
		}
	}
	return b.String()
}

func encodePDFTextHex(s string) string {
	units := utf16.Encode([]rune(s))
	var b strings.Builder
	for _, unit := range units {
		fmt.Fprintf(&b, "%04X", unit)
	}
	return b.String()
}

func escapePDFComment(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func errResult(errorClass, message string, started time.Time) tools.ExecutionResult {
	return tools.ExecutionResult{
		Error: &tools.ExecutionError{
			ErrorClass: errorClass,
			Message:    message,
		},
		DurationMs: durationMs(started),
	}
}

func durationMs(started time.Time) int {
	elapsed := time.Since(started)
	millis := int(elapsed / time.Millisecond)
	if millis < 0 {
		return 0
	}
	return millis
}
