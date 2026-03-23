package pipeline

import (
	"regexp"
	"strings"
)

// AssistantTextIsHeartbeatACK 检测文本是否仅由 HEARTBEAT_OK token 构成。
// 用于 heartbeat_decision 工具未被调用时的兜底判断。
func AssistantTextIsHeartbeatACK(text string) bool {
	s := strings.TrimSpace(text)
	if s == "" {
		return true
	}
	// 去除 Markdown 强调符和 HTML 标签
	s = regexp.MustCompile(`<[^>]*>`).ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "**")
	s = strings.TrimSuffix(s, "**")
	s = strings.TrimSpace(s)
	up := strings.ToUpper(s)
	if !strings.Contains(up, "HEARTBEAT_OK") {
		return false
	}
	rest := strings.ReplaceAll(up, "HEARTBEAT_OK", "")
	rest = strings.TrimSpace(rest)
	rest = strings.Trim(rest, ".,:;!?*- ")
	return rest == ""
}
