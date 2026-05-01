package webfetch

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const (
	minReadableCandidateRunes = 40
	minReadableScore          = 80
	structuredDataMaxDepth    = 12
	structuredDataMaxValues   = 24
)

var structuredFieldOrder = []string{"headline", "name", "title", "description", "articleBody", "text"}

func extractHTMLContent(htmlText string, pageURL string) (string, string, string) {
	doc, err := html.Parse(strings.NewReader(htmlText))
	if err != nil {
		content := normalizeBlockWhitespace(htmlToPlainTextFallback(stripUnsafeRawHTMLBlocks(htmlText)))
		extractor := "html-text"
		if content == "" {
			extractor = "html-empty"
		}
		return content, extractTitleFallback(htmlText), extractor
	}

	title := extractDocumentTitle(doc)
	body := findFirstElement(doc, atom.Body)
	if body == nil {
		body = doc
	}

	best := bestReadableNode(body)
	extractor := "html-readability"
	if best == nil {
		best = body
		extractor = "html"
	} else if best.DataAtom == atom.Body {
		extractor = "html"
	}

	content := renderHTMLBlocks(best, pageURL)
	if content == "" && best != body {
		content = renderHTMLBlocks(body, pageURL)
		extractor = "html"
	}
	if content == "" {
		content = normalizeBlockWhitespace(visibleText(body))
		extractor = "html-text"
	}
	if content == "" {
		structured := extractStructuredHTMLData(doc)
		if structured.content != "" {
			if title == "" {
				title = structured.title
			}
			return structured.content, title, structured.extractor
		}
		if description := extractDocumentDescription(doc); description != "" {
			return description, title, "html-metadata"
		}
		extractor = "html-empty"
	}
	return content, title, extractor
}

func bestReadableNode(root *html.Node) *html.Node {
	var best *html.Node
	bestScore := 0
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil || shouldSkipNode(node) {
			return
		}
		if isReadableCandidate(node) {
			score := readabilityScore(node)
			if score > bestScore {
				bestScore = score
				best = node
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	if bestScore < minReadableScore {
		return nil
	}
	return best
}

func isReadableCandidate(node *html.Node) bool {
	if node.Type != html.ElementNode {
		return false
	}
	switch node.DataAtom {
	case atom.Article, atom.Main, atom.Section, atom.Div, atom.Td, atom.Body:
		return true
	default:
		return false
	}
}

func readabilityScore(node *html.Node) int {
	text := visibleText(node)
	textRunes := utf8.RuneCountInString(text)
	if textRunes < minReadableCandidateRunes {
		return 0
	}
	linkRunes := linkTextRunes(node)
	linkPenalty := 0
	if textRunes > 0 {
		linkPenalty = (linkRunes * 100) / textRunes
	}

	score := textRunes + countDescendants(node, atom.P)*120 + countHeadingDescendants(node)*80
	score += classIDScore(node)
	score -= linkPenalty * 6
	if node.DataAtom == atom.Body {
		score -= 250
	}
	if score < 0 {
		return 0
	}
	return score
}

func classIDScore(node *html.Node) int {
	value := strings.ToLower(attrValue(node, "class") + " " + attrValue(node, "id") + " " + attrValue(node, "role"))
	score := 0
	positive := []string{"article", "body", "content", "entry", "main", "post", "story", "document", "markdown", "readme"}
	negative := []string{"ad", "advert", "banner", "breadcrumb", "comment", "cookie", "footer", "header", "menu", "modal", "nav", "popup", "promo", "related", "share", "sidebar", "social", "subscribe"}
	for _, word := range positive {
		if strings.Contains(value, word) {
			score += 180
		}
	}
	for _, word := range negative {
		if strings.Contains(value, word) {
			score -= 220
		}
	}
	return score
}

func countDescendants(node *html.Node, target atom.Atom) int {
	count := 0
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current == nil || shouldSkipNode(current) {
			return
		}
		if current.Type == html.ElementNode && current.DataAtom == target {
			count++
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return count
}

func countHeadingDescendants(node *html.Node) int {
	count := 0
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current == nil || shouldSkipNode(current) {
			return
		}
		if current.Type == html.ElementNode && headingLevel(current) > 0 {
			count++
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return count
}

func linkTextRunes(node *html.Node) int {
	count := 0
	var walk func(*html.Node, bool)
	walk = func(current *html.Node, inLink bool) {
		if current == nil || shouldSkipNode(current) {
			return
		}
		if current.Type == html.ElementNode && current.DataAtom == atom.A {
			inLink = true
		}
		if inLink && current.Type == html.TextNode {
			count += utf8.RuneCountInString(normalizeInlineWhitespace(current.Data))
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child, inLink)
		}
	}
	walk(node, false)
	return count
}

func renderHTMLBlocks(root *html.Node, pageURL string) string {
	var lines []string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil || shouldSkipNode(node) {
			return
		}
		if node.Type != html.ElementNode {
			return
		}
		switch node.DataAtom {
		case atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
			level := headingLevel(node)
			line := normalizeInlineWhitespace(renderHTMLInline(node, pageURL))
			if line != "" {
				lines = appendBlockLine(lines, strings.Repeat("#", level)+" "+line)
			}
			return
		case atom.P:
			if line := normalizeInlineWhitespace(renderHTMLInline(node, pageURL)); line != "" {
				lines = appendBlockLine(lines, line)
			}
			return
		case atom.Li:
			if line := normalizeInlineWhitespace(renderHTMLInline(node, pageURL)); line != "" {
				lines = appendBlockLine(lines, "- "+line)
			}
			return
		case atom.Blockquote:
			if line := normalizeInlineWhitespace(renderHTMLInline(node, pageURL)); line != "" {
				lines = appendBlockLine(lines, "> "+line)
			}
			return
		case atom.Pre:
			if line := strings.TrimSpace(visibleText(node)); line != "" {
				lines = appendBlockLine(lines, line)
			}
			return
		case atom.Tr:
			cells := tableCells(node)
			if len(cells) > 0 {
				lines = appendBlockLine(lines, strings.Join(cells, " | "))
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return normalizeBlockWhitespace(strings.Join(lines, "\n\n"))
}

func renderHTMLInline(node *html.Node, pageURL string) string {
	if node == nil || shouldSkipNode(node) {
		return ""
	}
	if node.Type == html.TextNode {
		return node.Data
	}
	if node.Type != html.ElementNode && node.Type != html.DocumentNode {
		return ""
	}
	switch node.DataAtom {
	case atom.Br:
		return "\n"
	case atom.Script, atom.Style, atom.Noscript, atom.Template, atom.Svg, atom.Canvas:
		return ""
	case atom.A:
		label := renderChildrenInline(node, pageURL)
		href := absoluteURL(pageURL, attrValue(node, "href"))
		label = normalizeInlineWhitespace(label)
		if label == "" {
			return href
		}
		if href == "" || strings.HasPrefix(strings.ToLower(href), "javascript:") {
			return label
		}
		return "[" + label + "](" + href + ")"
	case atom.Img:
		return attrValue(node, "alt")
	default:
		return renderChildrenInline(node, pageURL)
	}
}

func renderChildrenInline(node *html.Node, pageURL string) string {
	var builder strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		part := renderHTMLInline(child, pageURL)
		if part == "" {
			continue
		}
		if builder.Len() > 0 && needsInlineSpace(builder.String(), part) {
			builder.WriteByte(' ')
		}
		builder.WriteString(part)
	}
	return builder.String()
}

func tableCells(row *html.Node) []string {
	var cells []string
	for child := row.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || (child.DataAtom != atom.Td && child.DataAtom != atom.Th) {
			continue
		}
		cell := normalizeInlineWhitespace(renderHTMLInline(child, ""))
		if cell != "" {
			cells = append(cells, cell)
		}
	}
	return cells
}

func appendBlockLine(lines []string, line string) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return lines
	}
	if len(lines) > 0 && lines[len(lines)-1] == line {
		return lines
	}
	return append(lines, line)
}

func needsInlineSpace(left string, right string) bool {
	if left == "" || right == "" {
		return false
	}
	l, _ := utf8.DecodeLastRuneInString(left)
	r, _ := utf8.DecodeRuneInString(right)
	if l == '\n' || r == '\n' {
		return false
	}
	return !strings.ContainsRune(" ([{/", l) && !strings.ContainsRune(" .,;:!?)]}/", r)
}

func visibleText(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current == nil || shouldSkipNode(current) {
			return
		}
		if current.Type == html.TextNode {
			text := normalizeInlineWhitespace(current.Data)
			if text != "" {
				if builder.Len() > 0 {
					builder.WriteByte(' ')
				}
				builder.WriteString(text)
			}
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return normalizeInlineWhitespace(builder.String())
}

func shouldSkipNode(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}
	switch node.DataAtom {
	case atom.Script, atom.Style, atom.Noscript, atom.Template, atom.Svg, atom.Canvas, atom.Iframe,
		atom.Nav, atom.Header, atom.Footer:
		return true
	}
	if _, ok := attrLookup(node, "hidden"); ok {
		return true
	}
	if strings.EqualFold(attrValue(node, "aria-hidden"), "true") {
		return true
	}
	style := strings.ToLower(attrValue(node, "style"))
	return strings.Contains(style, "display:none") || strings.Contains(style, "display: none") ||
		strings.Contains(style, "visibility:hidden") || strings.Contains(style, "visibility: hidden")
}

func headingLevel(node *html.Node) int {
	if node == nil || node.Type != html.ElementNode {
		return 0
	}
	switch node.DataAtom {
	case atom.H1:
		return 1
	case atom.H2:
		return 2
	case atom.H3:
		return 3
	case atom.H4:
		return 4
	case atom.H5:
		return 5
	case atom.H6:
		return 6
	default:
		return 0
	}
}

func extractDocumentTitle(doc *html.Node) string {
	if title := normalizeInlineWhitespace(textFromFirstElement(doc, atom.Title)); title != "" {
		return title
	}
	for _, attrName := range []string{"property", "name"} {
		for _, attrValue := range []string{"og:title", "twitter:title"} {
			if title := metaContent(doc, attrName, attrValue); title != "" {
				return title
			}
		}
	}
	return ""
}

func extractDocumentDescription(doc *html.Node) string {
	for _, candidate := range [][2]string{
		{"name", "description"},
		{"property", "og:description"},
		{"name", "twitter:description"},
	} {
		if description := metaContent(doc, candidate[0], candidate[1]); description != "" {
			return description
		}
	}
	return ""
}

type structuredHTMLData struct {
	content   string
	title     string
	extractor string
}

func extractStructuredHTMLData(root *html.Node) structuredHTMLData {
	if data := extractStructuredScripts(root, isJSONLDScript, "html-jsonld"); data.content != "" {
		return data
	}
	return extractStructuredScripts(root, isNextDataScript, "html-next-data")
}

func extractStructuredScripts(root *html.Node, match func(*html.Node) bool, extractor string) structuredHTMLData {
	var found structuredHTMLData
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil || found.content != "" {
			return
		}
		if node.Type == html.ElementNode && node.DataAtom == atom.Script && match(node) {
			content, title := extractReadableJSONFields(scriptText(node))
			if content != "" {
				found = structuredHTMLData{content: content, title: title, extractor: extractor}
			}
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return found
}

func isJSONLDScript(node *html.Node) bool {
	return strings.Contains(strings.ToLower(attrValue(node, "type")), "ld+json")
}

func isNextDataScript(node *html.Node) bool {
	return attrValue(node, "id") == "__NEXT_DATA__"
}

func scriptText(node *html.Node) string {
	var builder strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			builder.WriteString(child.Data)
		}
	}
	return strings.TrimSpace(builder.String())
}

func extractReadableJSONFields(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	var parsed any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&parsed); err != nil {
		return "", ""
	}
	collector := newStructuredFieldCollector()
	collector.collect(parsed, 0)
	return collector.content(), collector.title()
}

type structuredFieldCollector struct {
	values map[string][]string
	seen   map[string]bool
	count  int
}

func newStructuredFieldCollector() *structuredFieldCollector {
	return &structuredFieldCollector{
		values: map[string][]string{},
		seen:   map[string]bool{},
	}
}

func (c *structuredFieldCollector) collect(value any, depth int) {
	if c == nil || depth > structuredDataMaxDepth || c.count >= structuredDataMaxValues {
		return
	}
	switch typed := value.(type) {
	case map[string]any:
		for _, field := range structuredFieldOrder {
			if raw, ok := lookupCaseInsensitive(typed, field); ok {
				c.add(field, raw)
			}
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			c.collect(typed[key], depth+1)
		}
	case []any:
		for _, item := range typed {
			c.collect(item, depth+1)
		}
	}
}

func (c *structuredFieldCollector) add(field string, value any) {
	if c == nil || c.count >= structuredDataMaxValues {
		return
	}
	raw, ok := value.(string)
	if !ok {
		return
	}
	cleaned := normalizeBlockWhitespace(stripInvisibleUnicode(strings.TrimSpace(strings.ToValidUTF8(raw, ""))))
	if cleaned == "" || c.seen[cleaned] {
		return
	}
	c.values[field] = append(c.values[field], cleaned)
	c.seen[cleaned] = true
	c.count++
}

func (c *structuredFieldCollector) content() string {
	if c == nil {
		return ""
	}
	lines := make([]string, 0, c.count)
	for _, field := range structuredFieldOrder {
		lines = append(lines, c.values[field]...)
	}
	return normalizeBlockWhitespace(strings.Join(lines, "\n\n"))
}

func (c *structuredFieldCollector) title() string {
	if c == nil {
		return ""
	}
	for _, field := range []string{"headline", "name", "title"} {
		if values := c.values[field]; len(values) > 0 {
			return normalizeInlineWhitespace(values[0])
		}
	}
	if values := c.values["description"]; len(values) > 0 {
		return normalizeInlineWhitespace(values[0])
	}
	return ""
}

func lookupCaseInsensitive(object map[string]any, expected string) (any, bool) {
	if value, ok := object[expected]; ok {
		return value, true
	}
	for key, value := range object {
		if strings.EqualFold(key, expected) {
			return value, true
		}
	}
	return nil, false
}

func metaContent(root *html.Node, attrName string, attrExpected string) string {
	var found string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil || found != "" {
			return
		}
		if node.Type == html.ElementNode && node.DataAtom == atom.Meta &&
			strings.EqualFold(attrValue(node, attrName), attrExpected) {
			found = normalizeInlineWhitespace(attrValue(node, "content"))
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return found
}

func textFromFirstElement(root *html.Node, target atom.Atom) string {
	node := findFirstElement(root, target)
	if node == nil {
		return ""
	}
	return visibleText(node)
}

func findFirstElement(root *html.Node, target atom.Atom) *html.Node {
	if root == nil {
		return nil
	}
	if root.Type == html.ElementNode && root.DataAtom == target {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if found := findFirstElement(child, target); found != nil {
			return found
		}
	}
	return nil
}

func attrValue(node *html.Node, key string) string {
	value, _ := attrLookup(node, key)
	return value
}

func attrLookup(node *html.Node, key string) (string, bool) {
	if node == nil {
		return "", false
	}
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, key) {
			return strings.TrimSpace(attr.Val), true
		}
	}
	return "", false
}

func htmlToPlainTextFallback(value string) string {
	var builder strings.Builder
	inTag := false
	for _, r := range value {
		switch r {
		case '<':
			inTag = true
			builder.WriteByte(' ')
		case '>':
			inTag = false
			builder.WriteByte(' ')
		default:
			if !inTag {
				builder.WriteRune(r)
			}
		}
	}
	return builder.String()
}

func stripUnsafeRawHTMLBlocks(value string) string {
	cleaned := value
	for _, tag := range []string{"script", "style", "noscript", "template"} {
		cleaned = stripRawHTMLBlock(cleaned, tag)
	}
	return cleaned
}

func stripRawHTMLBlock(value string, tag string) string {
	openMarker := "<" + tag
	closeMarker := "</" + tag + ">"
	for {
		lowered := strings.ToLower(value)
		start := strings.Index(lowered, openMarker)
		if start < 0 {
			return value
		}
		openEnd := strings.Index(lowered[start:], ">")
		if openEnd < 0 {
			return strings.TrimSpace(value[:start])
		}
		contentStart := start + openEnd + 1
		closeStart := strings.Index(lowered[contentStart:], closeMarker)
		if closeStart < 0 {
			return strings.TrimSpace(value[:start])
		}
		closeEnd := contentStart + closeStart + len(closeMarker)
		value = value[:start] + " " + value[closeEnd:]
	}
}

func extractTitleFallback(value string) string {
	lowered := strings.ToLower(value)
	start := strings.Index(lowered, "<title")
	if start < 0 {
		return ""
	}
	openEnd := strings.Index(lowered[start:], ">")
	if openEnd < 0 {
		return ""
	}
	contentStart := start + openEnd + 1
	closeStart := strings.Index(lowered[contentStart:], "</title>")
	if closeStart < 0 {
		return ""
	}
	return normalizeInlineWhitespace(htmlToPlainTextFallback(value[contentStart : contentStart+closeStart]))
}
