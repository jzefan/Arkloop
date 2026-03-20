package telegrambot

import (
	"html"
	"regexp"
	"strings"
)

// ParseModeHTML 供 sendMessage / editMessageText：助手常用 Markdown 子集转 Telegram 允许的 HTML。
const ParseModeHTML = "HTML"

var (
	reFencedCode  = regexp.MustCompile("(?s)```([a-zA-Z0-9]*)\r?\n?(.*?)```")
	reInlineCode  = regexp.MustCompile("`([^`\n]+)`")
	reMDLink      = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	reBoldStars   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reBoldUnder   = regexp.MustCompile(`__([^_]+)__`)
	reStrikeTilde = regexp.MustCompile(`~~([^~]+)~~`)
)

// FormatAssistantMarkdownAsHTML 将模型输出的常见 Markdown 安全转为 Telegram HTML。
// 先做 html.EscapeString，再按顺序替换围栏代码、行内代码、链接、粗体、删除线，避免误伤字面量。
func FormatAssistantMarkdownAsHTML(raw string) string {
	s := html.EscapeString(raw)
	s = reFencedCode.ReplaceAllStringFunc(s, func(m string) string {
		sub := reFencedCode.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		return "<pre>" + sub[2] + "</pre>"
	})
	s = reInlineCode.ReplaceAllStringFunc(s, func(m string) string {
		sub := reInlineCode.FindStringSubmatch(m)
		if len(sub) < 2 {
			return m
		}
		return "<code>" + sub[1] + "</code>"
	})
	s = reMDLink.ReplaceAllStringFunc(s, func(m string) string {
		sub := reMDLink.FindStringSubmatch(m)
		if len(sub) < 3 {
			return m
		}
		href := strings.TrimSpace(sub[2])
		if !strings.HasPrefix(href, "http://") && !strings.HasPrefix(href, "https://") {
			return m
		}
		return `<a href="` + html.EscapeString(href) + `">` + sub[1] + `</a>`
	})
	s = reBoldStars.ReplaceAllString(s, "<b>$1</b>")
	s = reBoldUnder.ReplaceAllString(s, "<b>$1</b>")
	s = reStrikeTilde.ReplaceAllString(s, "<s>$1</s>")
	return s
}
