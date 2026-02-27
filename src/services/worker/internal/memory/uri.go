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

// BuildURI 构造标准的 memory URI，确保格式统一且不会串租。
// 例：BuildURI(MemoryScopeUser, ident, MemoryCategoryPreference, "language")
//
//	→ "viking://user/{user_id}/preferences/language"
func BuildURI(scope MemoryScope, ident MemoryIdentity, category MemoryCategory, key string) string {
	key = sanitizeKey(strings.TrimSpace(key))
	switch scope {
	case MemoryScopeAgent:
		space := fmt.Sprintf("%s_%s", ident.AgentID, ident.UserID.String()[:8])
		return fmt.Sprintf("viking://agent/%s/%s/%s", space, string(category), key)
	default:
		return fmt.Sprintf("viking://user/%s/%s/%s", ident.UserID.String(), string(category), key)
	}
}
