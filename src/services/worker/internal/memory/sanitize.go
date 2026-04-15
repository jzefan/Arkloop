package memory

import "strings"

// xmlBoundaryTags 是可能突破 <memory>/<notebook> XML 块边界的标签。
// 攻击者可通过 memory_write/notebook_write 注入这些标签来逃逸 XML 块，
// 伪造 system prompt 或注入任意指令。
var xmlBoundaryTags = []string{
	"</memory>",
	"</notebook>",
	"<memory>",
	"<memory ",
	"<notebook>",
	"<notebook ",
	"<system>",
	"</system>",
	"<system ",
	"<tool_result>",
	"</tool_result>",
	"<tool_result ",
	"<function_results>",
	"</function_results>",
	"<function_results ",
}

// SanitizeBlockContent 过滤内容中可能突破 XML 块边界的标签。
// 采用大小写不敏感匹配，将匹配到的标签中的 < 替换为全角 ＜ 以保留可读性。
func SanitizeBlockContent(s string) string {
	lower := strings.ToLower(s)
	for _, tag := range xmlBoundaryTags {
		if !strings.Contains(lower, tag) {
			continue
		}
		// 逐个替换，保留原始大小写但中和标签
		result := make([]byte, 0, len(s))
		src := s
		srcLower := lower
		tagLen := len(tag)
		for {
			idx := strings.Index(srcLower, tag)
			if idx < 0 {
				result = append(result, src...)
				break
			}
			result = append(result, src[:idx]...)
			// 将 '<' 替换为全角 '＜'，'>' 替换为全角 '＞'
			original := src[idx : idx+tagLen]
			neutralized := strings.ReplaceAll(strings.ReplaceAll(original, "<", "\uFF1C"), ">", "\uFF1E")
			result = append(result, neutralized...)
			src = src[idx+tagLen:]
			srcLower = srcLower[idx+tagLen:]
		}
		s = string(result)
		lower = strings.ToLower(s)
	}
	return s
}
