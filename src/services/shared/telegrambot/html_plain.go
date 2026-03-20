package telegrambot

import (
	"strings"

	"golang.org/x/net/html"
)

// IsTelegramEntityParseError 判断是否为 Telegram HTML parse_mode 实体解析失败（可降级为纯文本重试）。
func IsTelegramEntityParseError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "parse") && strings.Contains(s, "entit")
}

// StripTelegramHTMLToPlain 去掉 Telegram HTML 子集标签并还原实体，供发送降级。
func StripTelegramHTMLToPlain(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	nodes, err := html.ParseFragment(strings.NewReader(s), nil)
	if err != nil || len(nodes) == 0 {
		return s
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		switch n.Type {
		case html.TextNode:
			b.WriteString(n.Data)
		case html.ElementNode:
			tag := strings.ToLower(n.Data)
			if tag == "br" {
				b.WriteByte('\n')
				return
			}
			block := tag == "p" || tag == "div" || tag == "blockquote" || tag == "pre"
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
			if block {
				b.WriteByte('\n')
			}
		}
	}
	for _, root := range nodes {
		walk(root)
	}
	return strings.TrimSpace(html.UnescapeString(b.String()))
}
