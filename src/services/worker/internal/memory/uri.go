package memory

import (
	"fmt"
	"regexp"
	"strings"
)

// reInvalidKey 过滤 URI key 中非法字符，防止路径注入。
var reInvalidKey = regexp.MustCompile(`[^a-zA-Z0-9_\-.]`)

func sanitizeKey(key string) string {
	return reInvalidKey.ReplaceAllString(key, "_")
}

// BuildURI 构造标准 memory URI，与 OpenViking 内部存储路径保持一致。
// 多租户隔离由请求头（X-OpenViking-User/Agent）处理，不体现在 URI 路径中。
//
// 例：BuildURI(MemoryScopeUser, MemoryCategoryPreference, "language")
//
//	→ "viking://user/memories/preferences/language"
func BuildURI(scope MemoryScope, category MemoryCategory, key string) string {
	key = sanitizeKey(strings.TrimSpace(key))
	switch scope {
	case MemoryScopeAgent:
		return fmt.Sprintf("viking://agent/memories/%s/%s", string(category), key)
	default:
		return fmt.Sprintf("viking://user/memories/%s/%s", string(category), key)
	}
}

func SelfURI(userID string) string {
	return fmt.Sprintf("viking://user/%s/memories/", userID)
}

func PeerExternalURI(id string) string {
	safe := sanitizeKey(strings.ReplaceAll(id, "-", ""))
	return fmt.Sprintf("viking://user/tg_%s/memories/", safe)
}

func SpaceURI(chatID string) string {
	safe := sanitizeKey(strings.TrimSpace(chatID))
	return fmt.Sprintf("viking://user/tgchat_%s/memories/", safe)
}
