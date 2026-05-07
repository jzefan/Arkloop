package memory

import (
	"fmt"
	"regexp"
	"strings"
)

var reInvalidKey = regexp.MustCompile(`[^a-zA-Z0-9_\-.]`)

func sanitizeKey(key string) string {
	return reInvalidKey.ReplaceAllString(key, "_")
}

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
