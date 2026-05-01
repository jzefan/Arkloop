package websearch

import (
	"context"
	"encoding/base64"
	"fmt"
	stdhtml "html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	sharedoutbound "arkloop/services/shared/outboundurl"

	xhtml "golang.org/x/net/html"
)

const (
	defaultBasicSearchBaseURL = "https://www.bing.com"
	basicSearchMaxBytes       = 2_000_000
	basicSearchMaxCountParam  = 50
	basicSearchUserAgent      = "Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko) ArkloopWebSearch/1.0 Safari/537.36"
)

type BasicProvider struct {
	baseURL string
	client  *http.Client
}

func NewBasicProvider() *BasicProvider {
	return NewBasicProviderWithBaseURL(defaultBasicSearchBaseURL)
}

func NewBasicProviderWithBaseURL(baseURL string) *BasicProvider {
	cleaned := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if cleaned == "" {
		cleaned = defaultBasicSearchBaseURL
	}
	return &BasicProvider{
		baseURL: cleaned,
		client:  sharedoutbound.DefaultPolicy().NewHTTPClient(15 * time.Second),
	}
}

func (p *BasicProvider) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query must not be empty")
	}
	if maxResults <= 0 {
		return nil, fmt.Errorf("maxResults must be a positive integer")
	}

	reqURL, err := p.searchURL(query, maxResults)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.1")
	market := basicSearchMarket(query)
	req.Header.Set("Accept-Language", market.acceptLanguage)
	req.Header.Set("User-Agent", basicSearchUserAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, HttpError{StatusCode: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, basicSearchMaxBytes))
	if err != nil {
		return nil, err
	}

	rawBody := string(body)
	var finalURL *url.URL
	if resp.Request != nil {
		finalURL = resp.Request.URL
	}
	results := parseBasicSearchHTML(rawBody, finalURL, maxResults)
	if len(results) == 0 {
		if isBasicSearchBlockedPage(rawBody) {
			return nil, fmt.Errorf("search provider returned a challenge page")
		}
		return nil, fmt.Errorf("search returned no usable results")
	}
	return results, nil
}

func (p *BasicProvider) searchURL(query string, maxResults int) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(p.baseURL))
	if err != nil {
		return "", err
	}
	if !parsed.IsAbs() || strings.TrimSpace(parsed.Hostname()) == "" {
		return "", fmt.Errorf("invalid search base url")
	}

	count := maxResults
	if count > basicSearchMaxCountParam {
		count = basicSearchMaxCountParam
	}
	parsed.Path = "/search"
	parsed.RawQuery = ""
	parsed.Fragment = ""

	q := parsed.Query()
	q.Set("q", query)
	market := basicSearchMarket(query)
	q.Set("count", strconv.Itoa(count))
	q.Set("mkt", market.mkt)
	q.Set("setlang", market.setLang)
	parsed.RawQuery = q.Encode()
	return parsed.String(), nil
}

func parseBasicSearchHTML(body string, baseURL *url.URL, maxResults int) []Result {
	if maxResults <= 0 || strings.TrimSpace(body) == "" {
		return nil
	}
	doc, err := xhtml.Parse(strings.NewReader(body))
	if err != nil {
		return nil
	}

	out := make([]Result, 0, maxResults)
	seen := map[string]struct{}{}
	add := func(title, href, snippet string) {
		if len(out) >= maxResults {
			return
		}
		cleanURL := cleanBasicResultURL(href, baseURL)
		if cleanURL == "" {
			return
		}
		key := normalizeURL(cleanURL)
		if key == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		title = normalizeInlineText(title, 160)
		if title == "" {
			title = titleFromURL(cleanURL)
		}
		if title == "" {
			return
		}
		seen[key] = struct{}{}
		out = append(out, Result{
			Title:   title,
			URL:     cleanURL,
			Snippet: normalizeInlineText(snippet, 320),
		})
	}

	walkNodes(doc, func(n *xhtml.Node) bool {
		if len(out) >= maxResults {
			return false
		}
		if !hasHTMLClass(n, "b_algo") {
			return true
		}
		title, href, snippet := basicResultFromAlgoNode(n)
		add(title, href, snippet)
		return true
	})

	if len(out) < maxResults {
		if resultsRoot := firstDescendant(doc, isBasicResultsRoot); resultsRoot != nil {
			walkNodes(resultsRoot, func(n *xhtml.Node) bool {
				if len(out) >= maxResults {
					return false
				}
				if !isHTMLElement(n, "li") && !isHTMLElement(n, "article") {
					return true
				}
				anchor := firstDescendant(n, func(candidate *xhtml.Node) bool {
					return isHTMLElement(candidate, "a") && strings.TrimSpace(htmlAttr(candidate, "href")) != ""
				})
				if anchor == nil {
					return true
				}
				add(nodeText(anchor), htmlAttr(anchor, "href"), firstParagraphText(n))
				return true
			})
		}
	}

	return out
}

func basicResultFromAlgoNode(n *xhtml.Node) (string, string, string) {
	heading := firstDescendant(n, func(candidate *xhtml.Node) bool {
		return isHTMLElement(candidate, "h2")
	})
	anchorRoot := n
	if heading != nil {
		anchorRoot = heading
	}
	anchor := firstDescendant(anchorRoot, func(candidate *xhtml.Node) bool {
		return isHTMLElement(candidate, "a") && strings.TrimSpace(htmlAttr(candidate, "href")) != ""
	})
	if anchor == nil && heading != nil {
		anchor = firstDescendant(n, func(candidate *xhtml.Node) bool {
			return isHTMLElement(candidate, "a") && strings.TrimSpace(htmlAttr(candidate, "href")) != ""
		})
	}
	if anchor == nil {
		return "", "", ""
	}

	snippet := ""
	caption := firstDescendant(n, func(candidate *xhtml.Node) bool {
		return hasHTMLClass(candidate, "b_caption") || hasHTMLClass(candidate, "b_snippet")
	})
	if caption != nil {
		if p := firstDescendant(caption, func(candidate *xhtml.Node) bool {
			return isHTMLElement(candidate, "p")
		}); p != nil {
			snippet = nodeText(p)
		}
		if snippet == "" {
			snippet = nodeText(caption)
		}
	}
	if snippet == "" {
		if p := firstDescendant(n, func(candidate *xhtml.Node) bool {
			return isHTMLElement(candidate, "p")
		}); p != nil {
			snippet = nodeText(p)
		}
	}

	return nodeText(anchor), htmlAttr(anchor, "href"), snippet
}

func firstParagraphText(n *xhtml.Node) string {
	if p := firstDescendant(n, func(candidate *xhtml.Node) bool {
		return isHTMLElement(candidate, "p")
	}); p != nil {
		return nodeText(p)
	}
	return ""
}

func cleanBasicResultURL(raw string, baseURL *url.URL) string {
	cleaned := strings.TrimSpace(stdhtml.UnescapeString(raw))
	if cleaned == "" {
		return ""
	}
	parsed, err := url.Parse(cleaned)
	if err != nil {
		return ""
	}
	if !parsed.IsAbs() {
		if baseURL == nil {
			return ""
		}
		parsed = baseURL.ResolveReference(parsed)
	}
	if decoded := decodeBingRedirectURL(parsed); decoded != "" {
		return decoded
	}
	if !isPublicSearchResultURL(parsed) || isBingHost(parsed.Hostname()) {
		return ""
	}
	parsed.Fragment = ""
	parsed.RawFragment = ""
	return parsed.String()
}

func decodeBingRedirectURL(parsed *url.URL) string {
	if parsed == nil || !isBingHost(parsed.Hostname()) {
		return ""
	}
	raw := strings.TrimSpace(parsed.Query().Get("u"))
	if raw == "" {
		return ""
	}
	candidate := raw
	if strings.HasPrefix(candidate, "a1") {
		if decoded := decodeBase64URL(candidate[2:]); decoded != "" {
			candidate = decoded
		}
	}
	target, err := url.Parse(strings.TrimSpace(candidate))
	if err != nil || !isPublicSearchResultURL(target) || isBingHost(target.Hostname()) {
		return ""
	}
	target.Fragment = ""
	target.RawFragment = ""
	return target.String()
}

func decodeBase64URL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	encodings := []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.StdEncoding,
	}
	for _, enc := range encodings {
		decoded, err := enc.DecodeString(value)
		if err == nil {
			return strings.TrimSpace(string(decoded))
		}
	}
	return ""
}

func isPublicSearchResultURL(parsed *url.URL) bool {
	if parsed == nil || parsed.User != nil {
		return false
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return false
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return false
	}
	lowered := strings.ToLower(strings.Trim(host, "."))
	if lowered == "localhost" || strings.HasSuffix(lowered, ".localhost") {
		return false
	}
	if ip := sharedoutbound.ParseIP(host); ip.IsValid() {
		return sharedoutbound.DefaultPolicy().EnsureIPAllowed(ip) == nil
	}
	return true
}

func isBingHost(host string) bool {
	lowered := strings.ToLower(strings.Trim(strings.TrimSpace(host), "."))
	return lowered == "bing.com" || strings.HasSuffix(lowered, ".bing.com")
}

func walkNodes(n *xhtml.Node, visit func(*xhtml.Node) bool) bool {
	if n == nil {
		return true
	}
	if !visit(n) {
		return false
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if !walkNodes(child, visit) {
			return false
		}
	}
	return true
}

func firstDescendant(n *xhtml.Node, match func(*xhtml.Node) bool) *xhtml.Node {
	var found *xhtml.Node
	walkNodes(n, func(candidate *xhtml.Node) bool {
		if candidate != n && match(candidate) {
			found = candidate
			return false
		}
		return true
	})
	return found
}

func nodeText(n *xhtml.Node) string {
	var b strings.Builder
	var walk func(*xhtml.Node)
	walk = func(candidate *xhtml.Node) {
		if candidate == nil || isIgnoredTextElement(candidate) {
			return
		}
		if candidate.Type == xhtml.TextNode {
			b.WriteString(candidate.Data)
			b.WriteByte(' ')
			return
		}
		for child := candidate.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return normalizeInlineText(b.String(), 500)
}

func isIgnoredTextElement(n *xhtml.Node) bool {
	return isHTMLElement(n, "script") ||
		isHTMLElement(n, "style") ||
		isHTMLElement(n, "noscript") ||
		isHTMLElement(n, "svg")
}

func isHTMLElement(n *xhtml.Node, name string) bool {
	return n != nil && n.Type == xhtml.ElementNode && strings.EqualFold(n.Data, name)
}

func htmlAttr(n *xhtml.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) {
			return attr.Val
		}
	}
	return ""
}

func hasHTMLClass(n *xhtml.Node, className string) bool {
	if n == nil || n.Type != xhtml.ElementNode {
		return false
	}
	for _, item := range strings.Fields(htmlAttr(n, "class")) {
		if item == className {
			return true
		}
	}
	return false
}

func isBasicResultsRoot(n *xhtml.Node) bool {
	return isHTMLElement(n, "ol") && strings.EqualFold(htmlAttr(n, "id"), "b_results")
}

func isBasicSearchBlockedPage(body string) bool {
	lowered := strings.ToLower(body)
	if strings.Contains(lowered, `id="b_results"`) ||
		strings.Contains(lowered, `id='b_results'`) ||
		strings.Contains(lowered, "b_algo") {
		return false
	}
	return strings.Contains(lowered, "cfconfig") ||
		strings.Contains(lowered, "challenges.cloudflare.com") ||
		strings.Contains(lowered, "/challenge/verify") ||
		strings.Contains(lowered, "turnstile")
}

type basicSearchLocale struct {
	mkt            string
	setLang        string
	acceptLanguage string
}

func basicSearchMarket(query string) basicSearchLocale {
	for _, r := range query {
		switch {
		case unicode.Is(unicode.Han, r):
			return basicSearchLocale{
				mkt:            "zh-CN",
				setLang:        "zh-CN",
				acceptLanguage: "zh-CN,zh;q=0.9,en-US;q=0.7,en;q=0.6",
			}
		case unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
			return basicSearchLocale{
				mkt:            "ja-JP",
				setLang:        "ja",
				acceptLanguage: "ja-JP,ja;q=0.9,en-US;q=0.7,en;q=0.6",
			}
		case unicode.Is(unicode.Hangul, r):
			return basicSearchLocale{
				mkt:            "ko-KR",
				setLang:        "ko",
				acceptLanguage: "ko-KR,ko;q=0.9,en-US;q=0.7,en;q=0.6",
			}
		}
	}
	return basicSearchLocale{
		mkt:            "en-US",
		setLang:        "en-US",
		acceptLanguage: "en-US,en;q=0.9",
	}
}
