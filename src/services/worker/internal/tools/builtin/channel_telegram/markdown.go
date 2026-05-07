package channel_telegram

import (
	"strings"
	"unicode/utf8"
)

// 长消息分段，与 pipeline 侧 splitTelegramMessage 行为一致（避免 import pipeline）。

func splitTelegramMessage(text string, limit int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	runes := []rune(text)
	if limit <= 0 || len(runes) <= limit {
		return []string{text}
	}

	var segments []string
	remaining := runes
	for len(remaining) > limit {
		cut := chooseTelegramSplitPoint(remaining, limit)
		segment := strings.TrimSpace(string(remaining[:cut]))
		if segment != "" {
			segments = append(segments, segment)
		}
		remaining = []rune(strings.TrimSpace(string(remaining[cut:])))
	}
	if len(remaining) > 0 {
		segments = append(segments, string(remaining))
	}
	return segments
}

func chooseTelegramSplitPoint(text []rune, limit int) int {
	window := string(text[:limit])
	for _, marker := range []string{"\n\n", "\n", "。", ".", "!", "?"} {
		if idx := strings.LastIndex(window, marker); idx > 0 {
			return utf8.RuneCountInString(window[:idx+len(marker)])
		}
	}
	return limit
}
