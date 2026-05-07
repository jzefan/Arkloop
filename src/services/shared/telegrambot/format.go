package telegrambot

import (
	"strings"
)

// ParseModeHTML 供 sendMessage / editMessageText：助手常用 Markdown 子集转 Telegram 允许的 HTML。
const ParseModeHTML = "HTML"

// FormatAssistantMarkdownAsHTML 将模型输出的常见 Markdown 安全转为 Telegram HTML（goldmark AST + Telegram 子集标签）。
func FormatAssistantMarkdownAsHTML(raw string) string {
	return formatAssistantMarkdownAsHTMLGoldmark(raw)
}

func telegramEscapeHTML(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}
