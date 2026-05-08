package markdowntopdf

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"arkloop/services/shared/objectstore"
	"arkloop/services/worker/internal/tools"
)

const (
	errorArgsInvalid  = "tool.args_invalid"
	errorUploadFailed = "tool.upload_failed"

	pdfMimeType = "application/pdf"
)

type ToolExecutor struct {
	store interface {
		PutObject(ctx context.Context, key string, data []byte, options objectstore.PutOptions) error
	}
}

func NewToolExecutor(store interface {
	PutObject(ctx context.Context, key string, data []byte, options objectstore.PutOptions) error
}) *ToolExecutor {
	return &ToolExecutor{store: store}
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

	pdfBytes := renderFormalReportPDF(title, content)

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

func renderFormalReportPDF(title, markdown string) []byte {
	lines := markdownToReportLines(title, markdown)
	if len(lines) == 0 {
		lines = []reportLine{{Text: title, FontSize: 18, Leading: 22}}
	}

	const linesPerPage = 42
	var pages [][]reportLine
	for len(lines) > 0 {
		n := linesPerPage
		if len(lines) < n {
			n = len(lines)
		}
		pages = append(pages, lines[:n])
		lines = lines[n:]
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
		fmt.Sprintf("<< /Type /Font /Subtype /Type0 /BaseFont /STSong-Light /Encoding /UniGB-UCS2-H /DescendantFonts [%d 0 R] >>", fontObj+1),
		"<< /Type /Font /Subtype /CIDFontType0 /BaseFont /STSong-Light /CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 2 >> >>",
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
}

func buildPageContentStream(lines []reportLine, pageNumber, pageCount int) string {
	var b strings.Builder
	b.WriteString("BT\n/F1 12 Tf\n72 770 Td\n")
	for _, line := range lines {
		fontSize := line.FontSize
		if fontSize <= 0 {
			fontSize = 12
		}
		leading := line.Leading
		if leading <= 0 {
			leading = fontSize + 4
		}
		fmt.Fprintf(&b, "/F1 %d Tf\n%d TL\n", fontSize, leading)
		if strings.TrimSpace(line.Text) == "" {
			fmt.Fprintf(&b, "0 -%d Td\n", leading/2)
			continue
		}
		fmt.Fprintf(&b, "%% raw:%s\n<%s> Tj\nT*\n", escapePDFComment(line.Text), encodePDFTextHex(line.Text))
	}
	b.WriteString("/F1 10 Tf\n")
	pageText := fmt.Sprintf("Page %d of %d", pageNumber, pageCount)
	fmt.Fprintf(&b, "0 -20 Td\n<%s> Tj\n", encodePDFTextHex(pageText))
	b.WriteString("ET")
	return b.String()
}

var markdownLinkPattern = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

func markdownToReportLines(title, markdown string) []reportLine {
	rawLines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	lines := make([]reportLine, 0, len(rawLines)+2)
	if strings.TrimSpace(title) != "" {
		lines = append(lines,
			reportLine{Text: title, FontSize: 18, Leading: 24},
			reportLine{},
		)
	}
	for _, raw := range rawLines {
		line := strings.TrimSpace(raw)
		fontSize := 12
		leading := 16
		switch {
		case strings.HasPrefix(line, "# "):
			fontSize = 17
			leading = 23
		case strings.HasPrefix(line, "## "):
			fontSize = 15
			leading = 21
		case strings.HasPrefix(line, "### "):
			fontSize = 13
			leading = 18
		case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "):
			line = "· " + strings.TrimSpace(line[2:])
		}
		line = markdownLinkPattern.ReplaceAllString(line, "$1 ($2)")
		line = strings.TrimPrefix(line, "> ")
		line = strings.TrimLeft(line, "#")
		line = strings.TrimSpace(line)
		line = strings.Trim(line, "`")
		line = strings.ReplaceAll(line, "**", "")
		line = strings.ReplaceAll(line, "__", "")
		line = strings.ReplaceAll(line, "*", "")
		line = strings.ReplaceAll(line, "_", "")
		lines = appendWrappedLines(lines, line, 86, fontSize, leading)
	}
	return lines
}

func appendWrappedLines(lines []reportLine, line string, maxRunes int, fontSize int, leading int) []reportLine {
	if utf8.RuneCountInString(line) <= maxRunes {
		return append(lines, reportLine{Text: line, FontSize: fontSize, Leading: leading})
	}
	runes := []rune(line)
	for len(runes) > maxRunes {
		cut := maxRunes
		for cut > maxRunes/2 && runes[cut] != ' ' {
			cut--
		}
		if cut <= maxRunes/2 {
			cut = maxRunes
		}
		lines = append(lines, reportLine{Text: strings.TrimSpace(string(runes[:cut])), FontSize: fontSize, Leading: leading})
		runes = runes[cut:]
	}
	if len(runes) > 0 {
		lines = append(lines, reportLine{Text: strings.TrimSpace(string(runes)), FontSize: fontSize, Leading: leading})
	}
	return lines
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
