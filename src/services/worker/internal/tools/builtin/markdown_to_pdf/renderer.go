package markdowntopdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/signintech/gopdf"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// RenderOptions controls a single Markdown→PDF conversion.
type RenderOptions struct {
	// Ctx is used for image download cancellation.
	Ctx context.Context
	// Title is rendered as the document header on every page and appears
	// as a synthetic H1 at the beginning if the markdown body does not
	// already start with an equivalent H1.
	Title string
	// Markdown is the source content.
	Markdown string
	// Font is the resolved CJK-capable TrueType font (from fonts.go).
	Font *Font
	// AllowedImageRoots limits which local filesystem paths LoadImage may
	// read from. An empty slice means "no local files allowed at all".
	AllowedImageRoots []string
	// Logger, if non-nil, receives warnings (image load failures, etc.).
	Logger *slog.Logger
}

// A4 page geometry in PDF points (1 pt = 1/72 inch).
const (
	pageMarginLeft   = 64.0
	pageMarginRight  = 64.0
	pageMarginTop    = 64.0
	pageMarginBottom = 64.0

	// Font sizes.
	sizeH1    = 22.0
	sizeH2    = 18.0
	sizeH3    = 15.0
	sizeH4    = 13.0
	sizeH5    = 12.0
	sizeH6    = 11.5
	sizeBody  = 11.0
	sizeCode  = 10.0
	sizeTable = 10.0
	sizeFoot  = 9.0

	// Spacing.
	lineLeadingRatio   = 1.55
	paragraphSpacing   = 6.0
	headingTopSpacing  = 12.0
	headingBelowSpace  = 4.0
	listItemIndent     = 18.0
	blockquoteIndent   = 18.0
	codeBlockPadding   = 6.0
	tableCellPadding   = 4.0
	hrThickness        = 0.5
	linkUnderlineOff   = 1.0
	codeBgBorderRadius = 2.0
	tableBorderWidth   = 0.5

	// Placeholder name for total-page-count pattern.
	totalPagesPlaceholder = "ARK_TOTAL_PAGES"
)

// Render converts a Markdown document into a PDF byte stream.
func Render(opts RenderOptions) ([]byte, error) {
	if opts.Font == nil || len(opts.Font.Data) == 0 {
		return nil, errors.New("Render: Font is required")
	}
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}

	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})

	// Register the font under one family; gopdf subsets embedded glyphs
	// automatically, so only characters that appear in the document are
	// actually embedded into the resulting PDF.
	if err := pdf.AddTTFFontByReader(FontFamily, bytes.NewReader(opts.Font.Data)); err != nil {
		return nil, fmt.Errorf("register font: %w", err)
	}

	r := &pdfRenderer{
		opts:       opts,
		pdf:        pdf,
		pageWidth:  gopdf.PageSizeA4.W,
		pageHeight: gopdf.PageSizeA4.H,
		contentTop: pageMarginTop,
	}
	r.contentWidth = r.pageWidth - pageMarginLeft - pageMarginRight
	r.footerY = r.pageHeight - pageMarginBottom + 18

	// Derive a display title for the page header when the caller did not
	// pass one explicitly. We prefer the first H1 in the markdown because
	// that text is almost always author-curated and visually formatted
	// ("江苏农牧科技职业学院 · 产教融合指数报告（2026年）"), whereas the
	// filename passed into the executor often uses underscores and other
	// filesystem-friendly glyphs that look ugly in a page header.
	if strings.TrimSpace(opts.Title) == "" {
		if derived := firstMarkdownHeading(opts.Markdown); derived != "" {
			opts.Title = derived
			r.opts.Title = derived
		}
	}

	// Header / footer run once per page. We use a placeholder for the
	// total-page count so the footer can say "Page X of Y" even before
	// we've rendered all pages.
	pdf.AddHeader(r.drawHeader)
	pdf.AddFooter(r.drawFooter)

	pdf.AddPage()
	if err := r.setFont(sizeBody); err != nil {
		return nil, err
	}
	pdf.SetXY(pageMarginLeft, r.contentTop)

	// Render the markdown as-is. We intentionally do NOT inject a
	// synthetic H1 from opts.Title: the markdown is the source of truth
	// for document content, and the page header at the top of every page
	// already carries the document title. Injecting another H1 caused a
	// duplicate "big title" when a caller passed a filename-derived title
	// that did not textually match the markdown's own H1.
	doc := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader([]byte(opts.Markdown)))
	if err := r.renderBlock(doc, []byte(opts.Markdown)); err != nil {
		return nil, err
	}

	// Fill in the total-page-count placeholder everywhere on every page.
	totalPages := pdf.GetNumberOfPages()
	if err := pdf.FillInPlaceHoldText(totalPagesPlaceholder, fmt.Sprintf("%d", totalPages), gopdf.Left); err != nil {
		// Non-fatal: if the placeholder was never reached, we just end up
		// with blank "of " strings. Log if we have a logger.
		if r.opts.Logger != nil {
			r.opts.Logger.Warn("fill total pages placeholder", "error", err)
		}
	}

	var out bytes.Buffer
	if _, err := pdf.WriteTo(&out); err != nil {
		return nil, fmt.Errorf("write pdf: %w", err)
	}
	return out.Bytes(), nil
}

// pdfRenderer walks the goldmark AST and emits primitives against a gopdf
// instance. Methods are organised by block type.
type pdfRenderer struct {
	opts RenderOptions
	pdf  *gopdf.GoPdf

	pageWidth, pageHeight float64
	contentTop            float64
	contentWidth          float64
	footerY               float64

	curFontSize float64
	// orderedListCounters tracks the current item number for each nested
	// ordered list level (top of stack = innermost list).
	orderedListCounters []int
	orderedListMarkers  []string // "1", "a", "i"
}

// setFont sets the current font family+size. The family is always FontFamily
// since we registered exactly one TTF.
func (r *pdfRenderer) setFont(size float64) error {
	if err := r.pdf.SetFont(FontFamily, "", size); err != nil {
		return err
	}
	r.curFontSize = size
	return nil
}

// lineHeight returns the leading for the current font size.
func (r *pdfRenderer) lineHeight() float64 {
	return r.curFontSize * lineLeadingRatio
}

// bottomLimit is the Y coordinate below which we should trigger a page break.
func (r *pdfRenderer) bottomLimit() float64 {
	return r.pageHeight - pageMarginBottom - 6
}

// ensureSpaceForLine creates a new page if needed to fit `neededHeight`
// more points at the current cursor Y.
func (r *pdfRenderer) ensureSpaceForLine(neededHeight float64) {
	if r.pdf.GetY()+neededHeight > r.bottomLimit() {
		r.pdf.AddPage()
		r.pdf.SetXY(pageMarginLeft, r.contentTop)
	}
}

// -----------------------------------------------------------------------------
// Header / footer
// -----------------------------------------------------------------------------

func (r *pdfRenderer) drawHeader() {
	title := strings.TrimSpace(r.opts.Title)
	if title == "" {
		return
	}
	if err := r.pdf.SetFont(FontFamily, "", sizeFoot); err != nil {
		return
	}
	r.pdf.SetTextColor(128, 128, 128)
	r.pdf.SetXY(pageMarginLeft, pageMarginTop-22)
	_ = r.pdf.Cell(nil, title)
	// thin rule beneath header
	r.pdf.SetStrokeColor(210, 210, 210)
	r.pdf.SetLineWidth(0.3)
	r.pdf.Line(pageMarginLeft, pageMarginTop-8, r.pageWidth-pageMarginRight, pageMarginTop-8)
	r.pdf.SetTextColor(0, 0, 0)
}

func (r *pdfRenderer) drawFooter() {
	if err := r.pdf.SetFont(FontFamily, "", sizeFoot); err != nil {
		return
	}
	r.pdf.SetTextColor(128, 128, 128)
	// "Page X of [placeholder]"
	pageNum := r.pdf.GetNumberOfPages()
	prefix := fmt.Sprintf("Page %d of ", pageNum)
	width, _ := r.pdf.MeasureTextWidth(prefix)
	// We need extra space for the placeholder (roughly 20pt).
	placeholderWidth := 24.0
	totalWidth := width + placeholderWidth
	x := (r.pageWidth - totalWidth) / 2
	// gopdf's Cell / Text / PlaceHolderText treat the cursor Y differently:
	// Cell uses ContentTypeCell, where y is the TOP of the cell (glyphs are
	// drawn at baseline = y + ascender). Text and PlaceHolderText use
	// ContentTypeText, where y is the glyph BASELINE directly. Mixing the
	// two at the same Y produces a ~0.8*fontSize vertical misalignment.
	//
	// We render both parts of the footer with ContentTypeText (Text +
	// PlaceHolderText) so they share a baseline, and bias Y downward by an
	// ascender-sized offset so the footer visually sits where we asked.
	baselineY := r.footerY + sizeFoot*0.85
	r.pdf.SetXY(x, baselineY)
	_ = r.pdf.Text(prefix)
	r.pdf.SetXY(x+width, baselineY)
	_ = r.pdf.PlaceHolderText(totalPagesPlaceholder, placeholderWidth)
	r.pdf.SetTextColor(0, 0, 0)
}

// -----------------------------------------------------------------------------
// Block-level dispatch
// -----------------------------------------------------------------------------

// renderBlock walks direct children of `node` as block-level items.
func (r *pdfRenderer) renderBlock(node ast.Node, source []byte) error {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if err := r.renderNode(child, source); err != nil {
			return err
		}
	}
	return nil
}

func (r *pdfRenderer) renderNode(node ast.Node, source []byte) error {
	switch n := node.(type) {
	case *ast.Heading:
		return r.renderHeading(n.Level, collectInlineText(n, source))

	case *ast.Paragraph:
		return r.renderParagraph(n, source)

	case *ast.List:
		return r.renderList(n, source)

	case *ast.FencedCodeBlock, *ast.CodeBlock:
		return r.renderCodeBlock(node, source)

	case *ast.Blockquote:
		return r.renderBlockquote(n, source)

	case *ast.ThematicBreak:
		return r.renderThematicBreak()

	case *extast.Table:
		return r.renderTable(n, source)

	case *ast.HTMLBlock:
		// Raw HTML: render stripped-tag text rather than embedding HTML.
		raw := collectRawText(n, source)
		return r.renderParagraphText(raw)

	default:
		// Unknown block: recurse so we don't lose content in extensions we
		// haven't explicitly handled.
		return r.renderBlock(node, source)
	}
}

// -----------------------------------------------------------------------------
// Headings
// -----------------------------------------------------------------------------

func (r *pdfRenderer) renderHeading(level int, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	size := sizeBody
	switch level {
	case 1:
		size = sizeH1
	case 2:
		size = sizeH2
	case 3:
		size = sizeH3
	case 4:
		size = sizeH4
	case 5:
		size = sizeH5
	case 6:
		size = sizeH6
	}

	// Top spacing before heading.
	if r.pdf.GetY() > r.contentTop {
		r.pdf.Br(headingTopSpacing)
	}
	if err := r.setFont(size); err != nil {
		return err
	}
	lines := r.wrapPlainText(content, r.contentWidth)
	for _, line := range lines {
		r.ensureSpaceForLine(r.lineHeight())
		r.pdf.SetX(pageMarginLeft)
		r.pdf.SetTextColor(17, 24, 39) // near-black for headings
		// Simulated bold: draw the glyphs twice with a 0.3 pt offset so
		// characters read thicker. Very cheap and works for any font.
		_ = r.pdf.Cell(nil, line)
		curX := r.pdf.GetX()
		curY := r.pdf.GetY()
		r.pdf.SetXY(curX-(mustMeasureWidth(r.pdf, line))+0.3, curY)
		_ = r.pdf.Cell(nil, line)
		r.pdf.SetTextColor(0, 0, 0)
		r.pdf.Br(r.lineHeight())
	}
	r.pdf.Br(headingBelowSpace)
	// Thin rule beneath H1 and H2 for visual hierarchy.
	if level <= 2 {
		r.pdf.SetStrokeColor(210, 210, 210)
		r.pdf.SetLineWidth(0.4)
		y := r.pdf.GetY() - 2
		r.pdf.Line(pageMarginLeft, y, r.pageWidth-pageMarginRight, y)
		r.pdf.Br(4)
	}
	return r.setFont(sizeBody)
}

// mustMeasureWidth returns 0 on error — used only for the bold simulation
// offset, where a small miscalculation is harmless.
func mustMeasureWidth(pdf *gopdf.GoPdf, s string) float64 {
	w, err := pdf.MeasureTextWidth(s)
	if err != nil {
		return 0
	}
	return w
}

// -----------------------------------------------------------------------------
// Paragraphs
// -----------------------------------------------------------------------------

func (r *pdfRenderer) renderParagraph(p *ast.Paragraph, source []byte) error {
	// Paragraph may contain inline images at top level; render them first
	// if they are the entire content, otherwise render text.
	if onlyChildIsImage(p, source) {
		img := firstChildImage(p)
		if img != nil {
			return r.renderImage(img, source)
		}
	}
	segments := collectInlineSegments(p, source)
	if err := r.setFont(sizeBody); err != nil {
		return err
	}
	r.pdf.SetTextColor(33, 37, 41)
	err := r.renderInlineSegments(segments, pageMarginLeft, r.contentWidth)
	r.pdf.Br(paragraphSpacing)
	return err
}

// renderParagraphText is a cheap fallback for HTML blocks or otherwise
// unstructured text.
func (r *pdfRenderer) renderParagraphText(text string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if err := r.setFont(sizeBody); err != nil {
		return err
	}
	for _, line := range r.wrapPlainText(text, r.contentWidth) {
		r.ensureSpaceForLine(r.lineHeight())
		r.pdf.SetX(pageMarginLeft)
		_ = r.pdf.Cell(nil, line)
		r.pdf.Br(r.lineHeight())
	}
	r.pdf.Br(paragraphSpacing)
	return nil
}

// -----------------------------------------------------------------------------
// Lists
// -----------------------------------------------------------------------------

func (r *pdfRenderer) renderList(list *ast.List, source []byte) error {
	depth := len(r.orderedListCounters) // nesting level (0 for outermost)
	indentPx := float64(depth) * listItemIndent

	if list.IsOrdered() {
		r.orderedListCounters = append(r.orderedListCounters, 0)
		r.orderedListMarkers = append(r.orderedListMarkers, orderedMarker(depth))
		defer func() {
			r.orderedListCounters = r.orderedListCounters[:len(r.orderedListCounters)-1]
			r.orderedListMarkers = r.orderedListMarkers[:len(r.orderedListMarkers)-1]
		}()
	}

	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		li, ok := item.(*ast.ListItem)
		if !ok {
			continue
		}

		var marker string
		if list.IsOrdered() {
			r.orderedListCounters[len(r.orderedListCounters)-1]++
			num := r.orderedListCounters[len(r.orderedListCounters)-1]
			marker = fmt.Sprintf("%s.", orderedNumeral(num, r.orderedListMarkers[len(r.orderedListMarkers)-1]))
		} else {
			marker = bulletMarker(depth)
		}

		if err := r.renderListItem(li, source, pageMarginLeft+indentPx, marker); err != nil {
			return err
		}
	}
	if depth == 0 {
		r.pdf.Br(paragraphSpacing)
	}
	return nil
}

func (r *pdfRenderer) renderListItem(li *ast.ListItem, source []byte, itemX float64, marker string) error {
	if err := r.setFont(sizeBody); err != nil {
		return err
	}
	// Width reserved for the marker + gap before text.
	markerReserve := 18.0
	textX := itemX + markerReserve
	textWidth := r.contentWidth - (textX - pageMarginLeft)

	// Render first block of the item inline with the marker.
	firstBlock := li.FirstChild()
	r.ensureSpaceForLine(r.lineHeight())
	r.pdf.SetTextColor(33, 37, 41)
	// Marker
	r.pdf.SetXY(itemX, r.pdf.GetY())
	_ = r.pdf.Cell(nil, marker)

	if firstBlock != nil {
		switch fb := firstBlock.(type) {
		case *ast.TextBlock, *ast.Paragraph:
			segments := collectInlineSegments(fb, source)
			if err := r.renderInlineSegments(segments, textX, textWidth); err != nil {
				return err
			}
		default:
			r.pdf.Br(r.lineHeight())
			if err := r.renderNode(fb, source); err != nil {
				return err
			}
		}
	} else {
		r.pdf.Br(r.lineHeight())
	}

	// Subsequent siblings of the first block are nested content (paragraphs,
	// nested lists) — render indented at textX.
	for sibling := firstBlockOrNil(firstBlock); sibling != nil; sibling = sibling.NextSibling() {
		if sibling == firstBlock {
			continue
		}
		switch sibling.(type) {
		case *ast.List:
			if err := r.renderNode(sibling, source); err != nil {
				return err
			}
		default:
			if err := r.renderNode(sibling, source); err != nil {
				return err
			}
		}
	}
	return nil
}

func firstBlockOrNil(n ast.Node) ast.Node {
	if n == nil {
		return nil
	}
	return n
}

func bulletMarker(depth int) string {
	markers := []string{"•", "◦", "▪", "•"}
	return markers[depth%len(markers)]
}

func orderedMarker(depth int) string {
	markers := []string{"1", "a", "i", "1"}
	return markers[depth%len(markers)]
}

// orderedNumeral formats `n` in the requested style (1, a, i). Supports
// numerals up to 26 in alpha mode (a..z) and roman up to ~40; beyond that
// falls back to decimal.
func orderedNumeral(n int, style string) string {
	switch style {
	case "a":
		if n >= 1 && n <= 26 {
			return string(rune('a' + n - 1))
		}
	case "i":
		if s, ok := toRoman(n); ok {
			return s
		}
	}
	return fmt.Sprintf("%d", n)
}

func toRoman(n int) (string, bool) {
	if n <= 0 || n >= 4000 {
		return "", false
	}
	numerals := []struct {
		val int
		sym string
	}{
		{1000, "m"}, {900, "cm"}, {500, "d"}, {400, "cd"},
		{100, "c"}, {90, "xc"}, {50, "l"}, {40, "xl"},
		{10, "x"}, {9, "ix"}, {5, "v"}, {4, "iv"}, {1, "i"},
	}
	var b strings.Builder
	for _, num := range numerals {
		for n >= num.val {
			b.WriteString(num.sym)
			n -= num.val
		}
	}
	return b.String(), true
}

// -----------------------------------------------------------------------------
// Code blocks
// -----------------------------------------------------------------------------

func (r *pdfRenderer) renderCodeBlock(node ast.Node, source []byte) error {
	if err := r.setFont(sizeCode); err != nil {
		return err
	}
	lines := []string{}
	linesContainer := node.Lines()
	for i := 0; i < linesContainer.Len(); i++ {
		seg := linesContainer.At(i)
		lines = append(lines, string(seg.Value(source)))
	}
	// Strip trailing empty lines but keep leading indentation.
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return nil
	}

	// Measure block height to draw a single background rectangle.
	lineH := sizeCode * 1.35
	blockHeight := float64(len(lines))*lineH + codeBlockPadding*2
	r.ensureSpaceForLine(blockHeight)

	bgX := pageMarginLeft
	bgY := r.pdf.GetY()
	bgW := r.contentWidth
	bgH := blockHeight

	// Background.
	r.pdf.SetFillColor(246, 248, 250)
	r.pdf.SetStrokeColor(223, 227, 230)
	r.pdf.SetLineWidth(0.3)
	r.pdf.RectFromUpperLeftWithStyle(bgX, bgY, bgW, bgH, "FD")

	// Text inside.
	textX := bgX + codeBlockPadding
	textY := bgY + codeBlockPadding
	r.pdf.SetTextColor(36, 41, 47)
	for _, rawLine := range lines {
		// Truncate lines that overflow right edge rather than wrapping
		// (wrapping arbitrary code lines is rarely desirable in a report).
		line := r.truncateToWidth(strings.TrimRight(rawLine, "\n"), bgW-codeBlockPadding*2)
		r.pdf.SetXY(textX, textY)
		_ = r.pdf.Cell(nil, line)
		textY += lineH
	}
	r.pdf.SetTextColor(0, 0, 0)
	r.pdf.SetFillColor(0, 0, 0)
	r.pdf.SetXY(pageMarginLeft, bgY+bgH)
	r.pdf.Br(paragraphSpacing)
	return r.setFont(sizeBody)
}

func (r *pdfRenderer) truncateToWidth(text string, maxWidth float64) string {
	w, err := r.pdf.MeasureTextWidth(text)
	if err != nil {
		return text
	}
	if w <= maxWidth {
		return text
	}
	ellipsis := "…"
	runes := []rune(text)
	for i := len(runes); i > 0; i-- {
		cand := string(runes[:i]) + ellipsis
		w, err := r.pdf.MeasureTextWidth(cand)
		if err == nil && w <= maxWidth {
			return cand
		}
	}
	return ellipsis
}

// -----------------------------------------------------------------------------
// Blockquote
// -----------------------------------------------------------------------------

func (r *pdfRenderer) renderBlockquote(bq *ast.Blockquote, source []byte) error {
	if err := r.setFont(sizeBody); err != nil {
		return err
	}
	content := collectInlineText(bq, source)
	lines := r.wrapPlainText(content, r.contentWidth-blockquoteIndent-6)
	startY := r.pdf.GetY()

	// Left rule: draw before text so it appears behind.
	for _, line := range lines {
		r.ensureSpaceForLine(r.lineHeight())
		y := r.pdf.GetY()
		// rule
		r.pdf.SetStrokeColor(185, 195, 205)
		r.pdf.SetLineWidth(3)
		r.pdf.Line(pageMarginLeft+1, y+2, pageMarginLeft+1, y+r.lineHeight()-2)
		r.pdf.SetLineWidth(0.5)
		// text
		r.pdf.SetTextColor(90, 98, 108)
		r.pdf.SetXY(pageMarginLeft+blockquoteIndent, y)
		_ = r.pdf.Cell(nil, line)
		r.pdf.SetTextColor(0, 0, 0)
		r.pdf.Br(r.lineHeight())
	}
	_ = startY
	r.pdf.Br(paragraphSpacing)
	return nil
}

// -----------------------------------------------------------------------------
// Thematic break (horizontal rule)
// -----------------------------------------------------------------------------

func (r *pdfRenderer) renderThematicBreak() error {
	r.ensureSpaceForLine(8)
	y := r.pdf.GetY() + 4
	r.pdf.SetStrokeColor(200, 200, 200)
	r.pdf.SetLineWidth(hrThickness)
	r.pdf.Line(pageMarginLeft, y, r.pageWidth-pageMarginRight, y)
	r.pdf.SetXY(pageMarginLeft, y+6)
	r.pdf.Br(paragraphSpacing)
	return nil
}

// -----------------------------------------------------------------------------
// Image
// -----------------------------------------------------------------------------

func (r *pdfRenderer) renderImage(img *ast.Image, source []byte) error {
	ref := string(img.Destination)
	loaded, err := LoadImage(r.opts.Ctx, ref, r.opts.AllowedImageRoots)
	if err != nil {
		if r.opts.Logger != nil {
			r.opts.Logger.Warn("markdown_to_pdf: skipping image", "ref", ref, "error", err)
		}
		// Render alt text as a paragraph fallback so the reader knows
		// something was here.
		alt := collectInlineText(img, source)
		if alt == "" {
			alt = "[image: " + ref + "]"
		} else {
			alt = "[image: " + alt + "]"
		}
		return r.renderParagraphText(alt)
	}

	// Compute display size: fit within contentWidth, preserve aspect ratio.
	dispW := float64(loaded.Width)
	dispH := float64(loaded.Height)
	if dispW > r.contentWidth {
		scale := r.contentWidth / dispW
		dispW *= scale
		dispH *= scale
	}
	if dispH > r.pageHeight-pageMarginTop-pageMarginBottom {
		scale := (r.pageHeight - pageMarginTop - pageMarginBottom) / dispH
		dispH *= scale
		dispW *= scale
	}

	r.ensureSpaceForLine(dispH + 4)

	imgHolder, err := gopdf.ImageHolderByBytes(loaded.Data)
	if err != nil {
		if r.opts.Logger != nil {
			r.opts.Logger.Warn("markdown_to_pdf: gopdf image holder error", "ref", ref, "error", err)
		}
		return nil
	}

	x := pageMarginLeft + (r.contentWidth-dispW)/2
	y := r.pdf.GetY()
	if err := r.pdf.ImageByHolder(imgHolder, x, y, &gopdf.Rect{W: dispW, H: dispH}); err != nil {
		if r.opts.Logger != nil {
			r.opts.Logger.Warn("markdown_to_pdf: draw image", "ref", ref, "error", err)
		}
		return nil
	}
	r.pdf.SetXY(pageMarginLeft, y+dispH)
	r.pdf.Br(paragraphSpacing)
	return nil
}

// -----------------------------------------------------------------------------
// Table
// -----------------------------------------------------------------------------

func (r *pdfRenderer) renderTable(table *extast.Table, source []byte) error {
	rows := extractTableRows(table, source)
	if len(rows) == 0 {
		return nil
	}
	colCount := 0
	for _, row := range rows {
		if len(row) > colCount {
			colCount = len(row)
		}
	}
	if colCount == 0 {
		return nil
	}

	// Compute column widths by weighting the longest measured content per
	// column, with a sensible minimum.
	weights := make([]float64, colCount)
	for i := range weights {
		weights[i] = 1
	}
	if err := r.setFont(sizeTable); err != nil {
		return err
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= colCount {
				break
			}
			w, err := r.pdf.MeasureTextWidth(cell)
			if err != nil {
				continue
			}
			if w > weights[i] {
				weights[i] = w
			}
		}
	}
	colWidths := make([]float64, colCount)
	totalWeight := 0.0
	for _, w := range weights {
		totalWeight += w
	}
	available := r.contentWidth
	minCol := 48.0
	for i, w := range weights {
		colWidths[i] = available * w / totalWeight
		if colWidths[i] < minCol {
			colWidths[i] = minCol
		}
	}
	// Normalise so total equals available width.
	sum := 0.0
	for _, w := range colWidths {
		sum += w
	}
	if sum > 0 {
		scale := available / sum
		for i := range colWidths {
			colWidths[i] *= scale
		}
	}

	// Render row by row. First row is treated as header.
	for rowIdx, row := range rows {
		isHeader := rowIdx == 0
		// Wrap each cell into lines based on its column width.
		wrappedCells := make([][]string, colCount)
		maxLines := 1
		for i := 0; i < colCount; i++ {
			var cellText string
			if i < len(row) {
				cellText = row[i]
			}
			innerWidth := colWidths[i] - tableCellPadding*2
			if innerWidth < 10 {
				innerWidth = 10
			}
			lines := r.wrapPlainText(cellText, innerWidth)
			if len(lines) == 0 {
				lines = []string{""}
			}
			if len(lines) > maxLines {
				maxLines = len(lines)
			}
			wrappedCells[i] = lines
		}
		lineH := sizeTable * 1.35
		rowH := float64(maxLines)*lineH + tableCellPadding*2

		r.ensureSpaceForLine(rowH)
		rowY := r.pdf.GetY()

		// Draw cells. NOTE: we must re-set the fill colour before each
		// rectangle because drawing text (Cell) internally emits an `rg`
		// operator to set the text colour, which is the same PDF state as
		// `rg` for fill colour. Without re-setting fill per cell, rectangle
		// #2+ in the row would pick up the previous cell's text colour and
		// render as a near-black block.
		rowFillR, rowFillG, rowFillB := uint8(255), uint8(255), uint8(255)
		if isHeader {
			rowFillR, rowFillG, rowFillB = 235, 239, 244
		} else if rowIdx%2 == 1 {
			rowFillR, rowFillG, rowFillB = 249, 251, 252
		}

		// Draw cells.
		x := pageMarginLeft
		for i := 0; i < colCount; i++ {
			r.pdf.SetFillColor(rowFillR, rowFillG, rowFillB)
			r.pdf.SetStrokeColor(210, 215, 220)
			r.pdf.SetLineWidth(tableBorderWidth)
			r.pdf.RectFromUpperLeftWithStyle(x, rowY, colWidths[i], rowH, "FD")
			// Cell text
			if isHeader {
				r.pdf.SetTextColor(30, 35, 40)
			} else {
				r.pdf.SetTextColor(50, 55, 60)
			}
			textY := rowY + tableCellPadding
			for _, line := range wrappedCells[i] {
				r.pdf.SetXY(x+tableCellPadding, textY)
				_ = r.pdf.Cell(nil, line)
				textY += lineH
			}
			r.pdf.SetTextColor(0, 0, 0)
			x += colWidths[i]
		}
		r.pdf.SetXY(pageMarginLeft, rowY+rowH)
	}
	r.pdf.Br(paragraphSpacing)
	return r.setFont(sizeBody)
}

// -----------------------------------------------------------------------------
// Inline segments
// -----------------------------------------------------------------------------

// inlineSegment is a chunk of inline content with uniform rendering
// attributes. We flatten the inline tree into segments and then perform
// greedy word-wrap by measuring each segment.
type inlineSegment struct {
	text     string
	link     string // non-empty = rendered as a hyperlink with underline
	code     bool   // inline `code` formatting
	kind     string // "text" | "image" — images are currently treated as text (alt only) when inline
	isBold   bool
	isItalic bool
}

func collectInlineSegments(n ast.Node, source []byte) []inlineSegment {
	var segs []inlineSegment
	var visit func(n ast.Node, styles inlineSegment)
	visit = func(n ast.Node, styles inlineSegment) {
		switch node := n.(type) {
		case *ast.Text:
			seg := styles
			seg.text = string(node.Value(source))
			segs = append(segs, seg)
			if node.SoftLineBreak() || node.HardLineBreak() {
				segs = append(segs, inlineSegment{text: " "})
			}
		case *ast.String:
			seg := styles
			seg.text = string(node.Value)
			segs = append(segs, seg)
		case *ast.CodeSpan:
			seg := styles
			seg.code = true
			seg.text = collectRawText(node, source)
			segs = append(segs, seg)
		case *ast.Emphasis:
			child := styles
			if node.Level >= 2 {
				child.isBold = true
			} else {
				child.isItalic = true
			}
			for c := node.FirstChild(); c != nil; c = c.NextSibling() {
				visit(c, child)
			}
		case *ast.Link:
			child := styles
			child.link = string(node.Destination)
			for c := node.FirstChild(); c != nil; c = c.NextSibling() {
				visit(c, child)
			}
		case *ast.AutoLink:
			seg := styles
			url := string(node.URL(source))
			seg.text = url
			seg.link = url
			segs = append(segs, seg)
		case *ast.Image:
			// Inline images are rendered as alt text; true block-level
			// images are handled via onlyChildIsImage before we reach here.
			alt := collectInlineText(node, source)
			seg := styles
			if alt == "" {
				seg.text = "[image]"
			} else {
				seg.text = "[" + alt + "]"
			}
			segs = append(segs, seg)
		default:
			for c := n.FirstChild(); c != nil; c = c.NextSibling() {
				visit(c, styles)
			}
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		visit(c, inlineSegment{})
	}
	return segs
}

// renderInlineSegments lays out a sequence of inline segments with greedy
// wrapping at `startX`, wrapping within `width` points. Honours inline code
// (background tint), links (underline + blue + PDF annotation), and
// SoftLineBreak-injected spaces.
func (r *pdfRenderer) renderInlineSegments(segments []inlineSegment, startX, width float64) error {
	if err := r.setFont(sizeBody); err != nil {
		return err
	}
	// Chunk each segment into runes and accumulate into lines by measuring.
	x := startX
	r.ensureSpaceForLine(r.lineHeight())
	r.pdf.SetXY(x, r.pdf.GetY())

	for _, seg := range segments {
		if seg.text == "" {
			continue
		}
		runes := []rune(seg.text)
		word := strings.Builder{}
		emit := func(s string) {
			if s == "" {
				return
			}
			w, err := r.pdf.MeasureTextWidth(s)
			if err != nil {
				return
			}
			// Wrap if this chunk would overflow the current line.
			if r.pdf.GetX()+w > startX+width && r.pdf.GetX() > startX {
				r.pdf.Br(r.lineHeight())
				r.ensureSpaceForLine(r.lineHeight())
				r.pdf.SetX(startX)
				// drop a leading space after wrap
				s = strings.TrimLeft(s, " \t")
				if s == "" {
					return
				}
				w, _ = r.pdf.MeasureTextWidth(s)
			}
			r.drawSegmentRun(s, seg)
			r.pdf.SetX(r.pdf.GetX() + w)
		}
		for i, rr := range runes {
			// Break word boundary at whitespace or CJK chars so wrap can
			// slot in between runs.
			if isBreakRune(rr) {
				if word.Len() > 0 {
					emit(word.String())
					word.Reset()
				}
				emit(string(rr))
			} else {
				word.WriteRune(rr)
				// If this is a CJK run, emit each char so wrapping is
				// permissive within the run.
				if isCJK(rr) {
					emit(word.String())
					word.Reset()
				}
			}
			_ = i
		}
		if word.Len() > 0 {
			emit(word.String())
		}
	}
	r.pdf.Br(r.lineHeight())
	r.pdf.SetX(pageMarginLeft) // reset for the next block
	return nil
}

// drawSegmentRun writes `s` at the current cursor with the styling implied
// by `seg` (link colour+underline, code background, etc.).
func (r *pdfRenderer) drawSegmentRun(s string, seg inlineSegment) {
	w, err := r.pdf.MeasureTextWidth(s)
	if err != nil {
		w = 0
	}
	x := r.pdf.GetX()
	y := r.pdf.GetY()

	if seg.code {
		r.pdf.SetFillColor(240, 242, 244)
		r.pdf.RectFromUpperLeftWithStyle(x-1, y+1, w+2, r.curFontSize*1.1, "F")
		r.pdf.SetTextColor(52, 58, 64)
		r.pdf.SetXY(x, y)
		_ = r.pdf.Cell(nil, s)
		r.pdf.SetTextColor(0, 0, 0)
		r.pdf.SetFillColor(0, 0, 0)
		r.pdf.SetXY(x, y) // gopdf already advanced x; we'll fix up by callers adding w
		return
	}

	if seg.link != "" {
		r.pdf.SetTextColor(0, 102, 204)
		r.pdf.SetXY(x, y)
		_ = r.pdf.Cell(nil, s)
		// Underline
		r.pdf.SetStrokeColor(0, 102, 204)
		r.pdf.SetLineWidth(0.5)
		r.pdf.Line(x, y+r.curFontSize-linkUnderlineOff, x+w, y+r.curFontSize-linkUnderlineOff)
		// Clickable annotation
		r.pdf.AddExternalLink(seg.link, x, y, w, r.curFontSize)
		r.pdf.SetTextColor(0, 0, 0)
		r.pdf.SetXY(x, y)
		return
	}

	r.pdf.SetXY(x, y)
	_ = r.pdf.Cell(nil, s)
	r.pdf.SetXY(x, y) // callers advance x manually
}

// wrapPlainText is a simpler greedy wrapper for headings, blockquotes and
// table cells. It does not handle links or inline code; use the segment
// renderer for paragraphs.
func (r *pdfRenderer) wrapPlainText(text string, maxWidth float64) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var out []string
	for _, para := range strings.Split(text, "\n") {
		out = append(out, r.wrapSingleLine(para, maxWidth)...)
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func (r *pdfRenderer) wrapSingleLine(text string, maxWidth float64) []string {
	text = strings.TrimRight(text, " \t")
	if text == "" {
		return []string{""}
	}
	var out []string
	var line strings.Builder
	lineWidth := 0.0
	lastBreakLen := -1
	lastBreakWidth := 0.0

	flush := func() {
		if line.Len() > 0 {
			out = append(out, strings.TrimRight(line.String(), " \t"))
		}
		line.Reset()
		lineWidth = 0
		lastBreakLen = -1
		lastBreakWidth = 0
	}

	for _, rr := range text {
		ch := string(rr)
		w, err := r.pdf.MeasureTextWidth(ch)
		if err != nil {
			w = r.curFontSize * 0.5
		}
		if lineWidth+w > maxWidth && line.Len() > 0 {
			if lastBreakLen > 0 {
				wrapped := strings.TrimRight(line.String()[:lastBreakLen], " \t")
				out = append(out, wrapped)
				remainder := line.String()[lastBreakLen:]
				remainder = strings.TrimLeft(remainder, " \t")
				line.Reset()
				line.WriteString(remainder)
				remainderW, _ := r.pdf.MeasureTextWidth(remainder)
				lineWidth = remainderW
				lastBreakLen = -1
				lastBreakWidth = 0
			} else {
				flush()
			}
		}
		line.WriteString(ch)
		lineWidth += w
		if isBreakRune(rr) {
			lastBreakLen = line.Len()
			lastBreakWidth = lineWidth
			_ = lastBreakWidth
		}
		if isCJK(rr) {
			lastBreakLen = line.Len()
			lastBreakWidth = lineWidth
		}
	}
	flush()
	return out
}

func isBreakRune(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '/', '-', '_', '，', '。', '、', '；', '：':
		return true
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

// -----------------------------------------------------------------------------
// AST helpers
// -----------------------------------------------------------------------------

// collectInlineText flattens an inline tree to plain text (no styling,
// no link destinations).
func collectInlineText(n ast.Node, source []byte) string {
	var b strings.Builder
	var visit func(n ast.Node)
	visit = func(n ast.Node) {
		switch node := n.(type) {
		case *ast.Text:
			b.Write(node.Value(source))
			if node.SoftLineBreak() || node.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(node.Value)
		case *ast.AutoLink:
			b.Write(node.URL(source))
		default:
			for c := n.FirstChild(); c != nil; c = c.NextSibling() {
				visit(c)
			}
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		visit(c)
	}
	return strings.TrimSpace(b.String())
}

// collectRawText collects raw text from a node; similar to collectInlineText
// but without whitespace normalisation, for code spans and HTML blocks.
func collectRawText(n ast.Node, source []byte) string {
	if n.Type() == ast.TypeBlock {
		lines := n.Lines()
		var b strings.Builder
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			b.Write(seg.Value(source))
		}
		return b.String()
	}
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch node := c.(type) {
		case *ast.Text:
			b.Write(node.Value(source))
		case *ast.String:
			b.Write(node.Value)
		default:
			b.WriteString(collectRawText(c, source))
		}
	}
	return b.String()
}

// onlyChildIsImage returns true when the paragraph's sole (non-space) child
// is an image, so we should render it as a block-level image.
func onlyChildIsImage(p *ast.Paragraph, source []byte) bool {
	var imageFound bool
	for c := p.FirstChild(); c != nil; c = c.NextSibling() {
		switch node := c.(type) {
		case *ast.Image:
			if imageFound {
				return false
			}
			imageFound = true
		case *ast.Text:
			if strings.TrimSpace(string(node.Value(source))) != "" {
				return false
			}
		default:
			return false
		}
	}
	return imageFound
}

func firstChildImage(p *ast.Paragraph) *ast.Image {
	for c := p.FirstChild(); c != nil; c = c.NextSibling() {
		if img, ok := c.(*ast.Image); ok {
			return img
		}
	}
	return nil
}

func extractTableRows(table *extast.Table, source []byte) [][]string {
	var rows [][]string
	for row := table.FirstChild(); row != nil; row = row.NextSibling() {
		var cells []string
		for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
			cells = append(cells, collectInlineText(cell, source))
		}
		if len(cells) > 0 {
			rows = append(rows, cells)
		}
	}
	return rows
}

// firstMarkdownHeading scans the markdown for the first ATX H1..H6 heading
// and returns its text content. Used to derive a display title for the
// page header when no explicit title is supplied.
func firstMarkdownHeading(markdown string) string {
	for _, line := range strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			// First non-blank line is not a heading → no heading at top.
			return ""
		}
		// Strip up to 6 leading '#' and the required space.
		i := 0
		for i < len(trimmed) && i < 6 && trimmed[i] == '#' {
			i++
		}
		if i == 0 || i >= len(trimmed) || trimmed[i] != ' ' {
			return ""
		}
		return strings.TrimSpace(trimmed[i+1:])
	}
	return ""
}
