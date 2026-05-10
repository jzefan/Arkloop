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

	pdfMimeType = "application/pdf"

	pdfPageWidth    = 595.0
	pdfPageHeight   = 842.0
	pdfMarginLeft   = 72.0
	pdfMarginRight  = 72.0
	pdfContentTop   = 770.0
	pdfFooterY      = 50.0
	pdfContentWidth = pdfPageWidth - pdfMarginLeft - pdfMarginRight
	pdfTableGap     = 12.0
)

var (
	markdownLinkPattern = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reportMarkdown      = goldmark.New(goldmark.WithExtensions(extension.GFM))
)

type ToolExecutor struct {
	store interface {
		PutObject(ctx context.Context, key string, data []byte, options objectstore.PutOptions) error
	}
}

func NewToolExecutor(store interface {
	PutObject(ctx context.Context, key string, data []byte, options objectstore.PutOptions) error
}, _ ...tools.Executor) *ToolExecutor {
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

	pdfBytes, err := renderStandardFormalReportPDF(title, content)
	if err != nil {
		return errResult(tools.ErrorClassToolExecutionFailed, fmt.Sprintf("render pdf failed: %s", err.Error()), started)
	}

	return e.uploadLocalPDF(ctx, execCtx, filename, title, pdfBytes, started)
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

func renderStandardFormalReportPDF(title, markdown string) ([]byte, error) {
	blocks := markdownToReportBlocks(title, markdown)
	if len(blocks) == 0 {
		blocks = []reportBlock{{Line: reportLine{Text: title, FontSize: 18, Leading: 22, IsBold: true}}}
	}

	pages := paginateReportBlocks(blocks)

	var objects []string
	objects = append(objects, "<< /Type /Catalog /Pages 2 0 R >>")
	var kids []string
	for i := range pages {
		pageObj := 3 + i*2
		kids = append(kids, fmt.Sprintf("%d 0 R", pageObj))
	}
	objects = append(objects, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages)))
	cjkFontObj := 3 + len(pages)*2
	latinFontObj := cjkFontObj + 2

	for i, pageBlocks := range pages {
		pageObj := 3 + i*2
		contentObj := pageObj + 1
		objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] /Resources << /Font << /F1 %d 0 R /F2 %d 0 R >> >> /Contents %d 0 R >>", pdfPageWidth, pdfPageHeight, cjkFontObj, latinFontObj, contentObj))
		stream := buildPageContentStream(pageBlocks, i+1, len(pages))
		objects = append(objects, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	}
	objects = append(objects,
		fmt.Sprintf("<< /Type /Font /Subtype /Type0 /BaseFont /STSong-Light /Encoding /UniGB-UCS2-H /DescendantFonts [%d 0 R] >>", cjkFontObj+1),
		"<< /Type /Font /Subtype /CIDFontType0 /BaseFont /STSong-Light /CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 2 >> >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
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
	pdfBytes := out.Bytes()
	if !bytes.HasPrefix(pdfBytes, []byte("%PDF-1.4")) {
		return nil, fmt.Errorf("invalid pdf header")
	}
	return pdfBytes, nil
}

func paginateReportBlocks(blocks []reportBlock) [][]reportBlock {
	const pageBottomPadding = 24.0
	maxHeight := pdfContentTop - pdfFooterY - pageBottomPadding
	var pages [][]reportBlock
	var page []reportBlock
	usedHeight := 0.0
	for _, block := range blocks {
		blockHeight := reportBlockHeight(block)
		if len(page) > 0 && usedHeight+blockHeight > maxHeight {
			pages = append(pages, page)
			page = nil
			usedHeight = 0
			if block.Table == nil && strings.TrimSpace(block.Line.Text) == "" {
				continue
			}
		}
		page = append(page, block)
		usedHeight += blockHeight
	}
	if len(page) > 0 {
		pages = append(pages, page)
	}
	if len(pages) == 0 {
		return [][]reportBlock{{}}
	}
	return pages
}

func reportBlockHeight(block reportBlock) float64 {
	if block.Table != nil {
		return block.Table.Height + pdfTableGap
	}
	return reportLineHeight(block.Line)
}

func reportLineHeight(line reportLine) float64 {
	fontSize := line.FontSize
	if fontSize <= 0 {
		fontSize = 12
	}
	leading := line.Leading
	if leading <= 0 {
		leading = fontSize + 4
	}
	if strings.TrimSpace(line.Text) == "" {
		return float64(leading) / 2.0
	}
	return float64(leading)
}

type reportBlock struct {
	Line  reportLine
	Table *reportTable
}

type reportLine struct {
	Text     string
	FontSize int
	Leading  int
	IsBold   bool
	Indent   int
	IsBullet bool
}

type reportTable struct {
	Rows         []reportTableRow
	ColumnWidths []float64
	Height       float64
}

type reportTableRow struct {
	Cells    []reportTableCell
	Height   float64
	IsHeader bool
}

type reportTableCell struct {
	Lines []string
	Bold  bool
}

func buildPageContentStream(blocks []reportBlock, pageNumber, pageCount int) string {
	var b strings.Builder
	b.WriteString("BT\n")
	lastX := pdfMarginLeft
	lastY := pdfContentTop
	fmt.Fprintf(&b, "%0.2f %0.2f Td\n", lastX, lastY)

	for _, block := range blocks {
		if block.Table != nil {
			b.WriteString("ET\n")
			writePDFTable(&b, block.Table, lastX, lastY)
			lastY -= block.Table.Height + pdfTableGap
			lastX = pdfMarginLeft
			b.WriteString("BT\n")
			fmt.Fprintf(&b, "%0.2f %0.2f Td\n", lastX, lastY)
			continue
		}

		line := block.Line
		fontSize := line.FontSize
		if fontSize <= 0 {
			fontSize = 12
		}
		leading := line.Leading
		if leading <= 0 {
			leading = fontSize + 4
		}

		targetX := pdfMarginLeft + float64(line.Indent)*12.0
		if targetX != lastX {
			fmt.Fprintf(&b, "%0.2f %0.2f Td\n", targetX-lastX, 0.0)
			lastX = targetX
		}

		fmt.Fprintf(&b, "%d TL\n", leading)
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
			blankHeight := reportLineHeight(line)
			fmt.Fprintf(&b, "0 -%0.2f Td\n", blankHeight)
			lastY -= blankHeight
			continue
		}

		fmt.Fprintf(&b, "%% raw:%s\n", escapePDFComment(text))
		writePDFTextRuns(&b, text, fontSize)
		b.WriteString("\nT*\n")
		lastY -= float64(leading)
	}

	b.WriteString("0 Tr 0 w\n")
	fmt.Fprintf(&b, "%0.2f %0.2f Td\n", pdfMarginLeft-lastX, pdfFooterY-lastY)
	pageText := fmt.Sprintf("Page %d of %d", pageNumber, pageCount)
	fmt.Fprintf(&b, "%% raw:%s\n", escapePDFComment(pageText))
	writePDFTextRuns(&b, pageText, 10)
	b.WriteString("ET")
	return b.String()
}

func writePDFTextRuns(b *strings.Builder, text string, fontSize int) {
	for _, run := range splitPDFTextRuns(text) {
		if run.Text == "" {
			continue
		}
		if run.CJK {
			fmt.Fprintf(b, "/F1 %d Tf <%s> Tj\n", fontSize, encodePDFTextHex(run.Text))
			continue
		}
		fmt.Fprintf(b, "/F2 %d Tf (%s) Tj\n", fontSize, escapePDFLiteral(run.Text))
	}
}

func writePDFTable(b *strings.Builder, table *reportTable, x float64, y float64) {
	if table == nil {
		return
	}
	currentY := y
	for _, row := range table.Rows {
		currentX := x
		for col, cell := range row.Cells {
			width := table.ColumnWidths[col]
			fmt.Fprintf(b, "0 Tr 0 w\n%0.2f %0.2f %0.2f %0.2f re S\n", currentX, currentY-row.Height, width, row.Height)
			textY := currentY - 15
			for lineIndex, line := range cell.Lines {
				if lineIndex == 0 {
					fmt.Fprintf(b, "%% raw:table-cell:%s\n", escapePDFComment(line))
				}
				b.WriteString("BT\n")
				fmt.Fprintf(b, "%0.2f %0.2f Td\n", currentX+6, textY-float64(lineIndex)*13)
				if cell.Bold {
					b.WriteString("2 Tr 0.35 w\n")
				} else {
					b.WriteString("0 Tr 0 w\n")
				}
				writePDFTextRuns(b, line, 9)
				b.WriteString("ET\n")
			}
			currentX += width
		}
		currentY -= row.Height
	}
}

type pdfTextRun struct {
	Text string
	CJK  bool
}

func splitPDFTextRuns(text string) []pdfTextRun {
	var runs []pdfTextRun
	var current strings.Builder
	currentCJK := false
	started := false
	for _, r := range text {
		runeUsesCIDFont := usesCIDFont(r)
		if started && runeUsesCIDFont != currentCJK {
			runs = append(runs, pdfTextRun{Text: current.String(), CJK: currentCJK})
			current.Reset()
		}
		current.WriteRune(r)
		currentCJK = runeUsesCIDFont
		started = true
	}
	if current.Len() > 0 {
		runs = append(runs, pdfTextRun{Text: current.String(), CJK: currentCJK})
	}
	return runs
}

func markdownToReportLines(title, markdown string) []reportLine {
	blocks := markdownToReportBlocks(title, markdown)
	lines := make([]reportLine, 0, len(blocks))
	for _, block := range blocks {
		if block.Table == nil {
			lines = append(lines, block.Line)
			continue
		}
		lines = append(lines, reportLine{Text: "[表格数据]", FontSize: 10, Leading: 14, IsBold: true})
		for _, row := range block.Table.Rows {
			var cells []string
			for _, cell := range row.Cells {
				cells = append(cells, strings.Join(cell.Lines, " "))
			}
			lines = append(lines, reportLine{Text: strings.Join(cells, "    "), FontSize: 10, Leading: 14})
		}
	}
	return lines
}

func markdownToReportBlocks(title, markdown string) []reportBlock {
	reader := text.NewReader([]byte(markdown))
	parser := reportMarkdown.Parser()
	doc := parser.Parse(reader)

	var blocks []reportBlock
	if shouldInjectTitle(title, markdown) {
		blocks = appendLineBlocks(blocks, title, 0, 18, 24, true, 0, false)
		blocks = append(blocks, lineBlock(reportLine{}))
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
			text := inlineText(node, reader.Source())
			blocks = appendLineBlocks(blocks, text, 0, fontSize, fontSize+6, true, 0, false)
			blocks = append(blocks, lineBlock(reportLine{}))
			return ast.WalkSkipChildren, nil

		case *ast.Paragraph:
			content := inlineText(n, reader.Source())
			blocks = appendLineBlocks(blocks, content, 80, 12, 18, false, 0, false)
			blocks = append(blocks, lineBlock(reportLine{}))
			return ast.WalkSkipChildren, nil

		case *ast.ListItem:
			content := inlineText(n, reader.Source())
			blocks = appendLineBlocks(blocks, content, 75, 12, 18, false, 1, true)
			return ast.WalkSkipChildren, nil

		case *ast.FencedCodeBlock, *ast.CodeBlock:
			blocks = append(blocks, lineBlock(reportLine{Text: "--- Code ---", FontSize: 10, Leading: 14, IsBold: true}))
			for i := 0; i < node.Lines().Len(); i++ {
				line := node.Lines().At(i)
				blocks = append(blocks, lineBlock(reportLine{Text: string(line.Value(reader.Source())), FontSize: 10, Leading: 14}))
			}
			blocks = append(blocks, lineBlock(reportLine{}))
			return ast.WalkSkipChildren, nil

		case *ast.Blockquote:
			content := inlineText(n, reader.Source())
			blocks = appendLineBlocks(blocks, "> "+content, 75, 12, 18, false, 1, false)
			blocks = append(blocks, lineBlock(reportLine{}))
			return ast.WalkSkipChildren, nil

		case *ast.ThematicBreak:
			blocks = append(blocks, lineBlock(reportLine{Text: "________________________________________________________________________________", FontSize: 10, Leading: 12}))
			blocks = append(blocks, lineBlock(reportLine{}))
			return ast.WalkSkipChildren, nil

		case *extast.Table:
			blocks = append(blocks, buildTableBlocks(node, reader.Source())...)
			blocks = append(blocks, lineBlock(reportLine{}))
			return ast.WalkSkipChildren, nil
		}

		return ast.WalkContinue, nil
	})

	return blocks
}

func lineBlock(line reportLine) reportBlock {
	return reportBlock{Line: line}
}

func appendLineBlocks(blocks []reportBlock, line string, maxRunes int, fontSize int, leading int, isBold bool, indent int, isBullet bool) []reportBlock {
	for _, reportLine := range wrapReportLines(line, maxRunes, fontSize, leading, isBold, indent, isBullet) {
		blocks = append(blocks, lineBlock(reportLine))
	}
	return blocks
}

func buildTableBlocks(table *extast.Table, source []byte) []reportBlock {
	rows := extractTableRows(table, source)
	if len(rows) == 0 {
		return nil
	}
	return []reportBlock{{
		Table: buildReportTable(rows),
	}}
}

func extractTableRows(table *extast.Table, source []byte) [][]string {
	var rows [][]string
	for row := table.FirstChild(); row != nil; row = row.NextSibling() {
		var cells []string
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			cells = append(cells, nodeText(cell, source))
		}
		if len(cells) > 0 {
			rows = append(rows, cells)
		}
	}
	return rows
}

func buildReportTable(rows [][]string) *reportTable {
	columnCount := 0
	for _, row := range rows {
		if len(row) > columnCount {
			columnCount = len(row)
		}
	}
	if columnCount == 0 {
		return nil
	}

	columnWidths := tableColumnWidths(rows, columnCount)
	reportRows := make([]reportTableRow, 0, len(rows))
	totalHeight := 0.0
	for rowIndex, row := range rows {
		cells := make([]reportTableCell, 0, columnCount)
		rowLineCount := 1
		for col := 0; col < columnCount; col++ {
			text := ""
			if col < len(row) {
				text = row[col]
			}
			lines := wrapTextByWidth(cleanInlineMarkup(text), 9, columnWidths[col]-12)
			if len(lines) == 0 {
				lines = []string{""}
			}
			if len(lines) > rowLineCount {
				rowLineCount = len(lines)
			}
			cells = append(cells, reportTableCell{Lines: lines, Bold: rowIndex == 0})
		}
		rowHeight := float64(rowLineCount)*13.0 + 10.0
		if rowHeight < 24 {
			rowHeight = 24
		}
		reportRows = append(reportRows, reportTableRow{Cells: cells, Height: rowHeight, IsHeader: rowIndex == 0})
		totalHeight += rowHeight
	}
	return &reportTable{Rows: reportRows, ColumnWidths: columnWidths, Height: totalHeight}
}

func tableColumnWidths(rows [][]string, columnCount int) []float64 {
	weights := make([]float64, columnCount)
	for col := 0; col < columnCount; col++ {
		weights[col] = 1
	}
	for _, row := range rows {
		for col, cell := range row {
			if col >= columnCount {
				continue
			}
			width := textWidth(cleanInlineMarkup(cell), 9)
			if width > weights[col] {
				weights[col] = width
			}
		}
	}

	minWidth := 72.0
	totalMinWidth := minWidth * float64(columnCount)
	availableWidth := pdfContentWidth
	if totalMinWidth > availableWidth {
		minWidth = availableWidth / float64(columnCount)
	}

	totalWeight := 0.0
	for _, weight := range weights {
		totalWeight += weight
	}
	if totalWeight <= 0 {
		totalWeight = float64(columnCount)
	}

	widths := make([]float64, columnCount)
	remaining := availableWidth
	for col := 0; col < columnCount; col++ {
		width := availableWidth * weights[col] / totalWeight
		if width < minWidth {
			width = minWidth
		}
		widths[col] = width
		remaining -= width
	}
	if remaining < 0 {
		scale := availableWidth / (availableWidth - remaining)
		for col := range widths {
			widths[col] *= scale
		}
	}
	return widths
}

func cleanInlineMarkup(line string) string {
	line = markdownLinkPattern.ReplaceAllString(line, "$1 ($2)")
	line = strings.ReplaceAll(line, "**", "")
	line = strings.ReplaceAll(line, "__", "")
	line = strings.ReplaceAll(line, "*", "")
	line = strings.ReplaceAll(line, "_", "")
	line = strings.ReplaceAll(line, "`", "")
	return strings.TrimSpace(line)
}

func nodeText(n ast.Node, source []byte) string {
	value := inlineText(n, source)
	if value != "" {
		return value
	}
	return normalizeInlineText(string(n.Text(source)))
}

func inlineText(n ast.Node, source []byte) string {
	var parts []string
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		appendInlineText(&parts, child, source)
	}
	return normalizeInlineText(strings.Join(parts, ""))
}

func appendInlineText(parts *[]string, n ast.Node, source []byte) {
	switch node := n.(type) {
	case *ast.Text:
		*parts = append(*parts, string(node.Value(source)))
		if node.SoftLineBreak() || node.HardLineBreak() {
			*parts = append(*parts, " ")
		}
		return
	case *ast.String:
		*parts = append(*parts, string(node.Value))
		return
	case *ast.CodeSpan:
		*parts = append(*parts, inlineText(node, source))
		return
	case *ast.AutoLink:
		*parts = append(*parts, string(node.URL(source)))
		return
	case *ast.Link:
		label := inlineText(node, source)
		destination := strings.TrimSpace(string(node.Destination))
		if destination != "" {
			*parts = append(*parts, fmt.Sprintf("%s (%s)", label, destination))
			return
		}
		*parts = append(*parts, label)
		return
	}

	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		appendInlineText(parts, child, source)
	}
}

func normalizeInlineText(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func appendWrappedLines(lines []reportLine, line string, maxRunes int, fontSize int, leading int, isBold bool, indent int, isBullet bool) []reportLine {
	return append(lines, wrapReportLines(line, maxRunes, fontSize, leading, isBold, indent, isBullet)...)
}

func wrapReportLines(line string, maxRunes int, fontSize int, leading int, isBold bool, indent int, isBullet bool) []reportLine {
	line = strings.ReplaceAll(line, "\n", " ")
	line = markdownLinkPattern.ReplaceAllString(line, "$1 ($2)")
	line = strings.ReplaceAll(line, "**", "")
	line = strings.ReplaceAll(line, "__", "")
	line = strings.ReplaceAll(line, "*", "")
	line = strings.ReplaceAll(line, "_", "")
	line = strings.ReplaceAll(line, "`", "")

	availableWidth := pdfContentWidth - float64(indent)*12.0
	if isBullet {
		availableWidth -= textWidth("· ", fontSize)
	}
	if availableWidth < 120 {
		availableWidth = 120
	}
	if maxRunes > 0 {
		maxWidthByRunes := float64(maxRunes) * float64(fontSize) * 0.5
		if maxWidthByRunes < availableWidth {
			availableWidth = maxWidthByRunes
		}
	}
	wrapped := wrapTextByWidth(line, fontSize, availableWidth)
	if len(wrapped) == 0 {
		return []reportLine{{Text: line, FontSize: fontSize, Leading: leading, IsBold: isBold, Indent: indent, IsBullet: isBullet}}
	}
	lines := make([]reportLine, 0, len(wrapped))
	for i, wrappedLine := range wrapped {
		lines = append(lines, reportLine{Text: wrappedLine, FontSize: fontSize, Leading: leading, IsBold: isBold, Indent: indent, IsBullet: isBullet && i == 0})
	}
	return lines
}

func wrapTextByWidth(text string, fontSize int, maxWidth float64) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{text}
	}

	var lines []string
	var current []rune
	currentWidth := 0.0
	lastBreak := -1
	widthAtBreak := 0.0

	flush := func() {
		value := strings.TrimSpace(string(current))
		if value != "" {
			lines = append(lines, value)
		}
		current = current[:0]
		currentWidth = 0
		lastBreak = -1
		widthAtBreak = 0
	}

	for _, r := range []rune(text) {
		if r == '\n' {
			flush()
			continue
		}
		width := runeWidth(r, fontSize)
		if currentWidth+width > maxWidth && len(current) > 0 {
			if lastBreak > 0 {
				lines = append(lines, strings.TrimSpace(string(current[:lastBreak])))
				remainder := append([]rune(nil), current[lastBreak:]...)
				current = []rune(strings.TrimLeftFunc(string(remainder), func(r rune) bool { return r == ' ' || r == '\t' }))
				currentWidth = textWidth(string(current), fontSize)
				lastBreak = -1
				widthAtBreak = 0
			} else {
				flush()
			}
		}
		current = append(current, r)
		currentWidth += width
		if isBreakableRune(r) {
			lastBreak = len(current)
			widthAtBreak = currentWidth
		}
		if widthAtBreak > maxWidth {
			flush()
		}
	}
	flush()
	return lines
}

func textWidth(text string, fontSize int) float64 {
	width := 0.0
	for _, r := range text {
		width += runeWidth(r, fontSize)
	}
	return width
}

func runeWidth(r rune, fontSize int) float64 {
	if r == '\t' {
		return float64(fontSize) * 1.2
	}
	if r == ' ' {
		return float64(fontSize) * 0.28
	}
	if usesCIDFont(r) {
		return float64(fontSize)
	}
	if strings.ContainsRune("ilI.,:;!|", r) {
		return float64(fontSize) * 0.28
	}
	if strings.ContainsRune("mwMW@#%&", r) {
		return float64(fontSize) * 0.85
	}
	return float64(fontSize) * 0.52
}

func isBreakableRune(r rune) bool {
	return r == ' ' || r == '\t' || r == '/' || r == '-' || r == '_' || r == '，' || r == '。' || r == '、' || r == '；' || r == '：'
}

func isCJKLikeRune(r rune) bool {
	return (r >= 0x2E80 && r <= 0x9FFF) ||
		(r >= 0xAC00 && r <= 0xD7AF) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0xFE30 && r <= 0xFE4F) ||
		(r >= 0xFF00 && r <= 0xFFEF)
}

func usesCIDFont(r rune) bool {
	return r > 126 || isCJKLikeRune(r)
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

func escapePDFLiteral(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\', '(', ')':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\r', '\n':
			b.WriteByte(' ')
		default:
			if r >= 32 && r <= 126 {
				b.WriteRune(r)
				continue
			}
			fmt.Fprintf(&b, "\\%03o", r)
		}
	}
	return b.String()
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
