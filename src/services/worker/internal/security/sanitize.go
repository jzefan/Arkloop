package security

import (
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

var (
	// 方向控制字符 (U+202A-U+202E, U+2066-U+2069)
	dirControlRe = regexp.MustCompile("[\u202A-\u202E\u2066-\u2069]")
	// 连续空白，含 Unicode 空格 (\p{Z} 覆盖 Zs/Zl/Zp)
	multiSpaceRe = regexp.MustCompile(`[\s\p{Z}]+`)
	// 零宽字符替换器
	zeroWidthReplacer = strings.NewReplacer(
		"\u200B", "", // zero width space
		"\u200C", "", // zero width non-joiner
		"\u200D", "", // zero width joiner
		"\uFEFF", "", // BOM / zero width no-break space
		"\u2060", "", // word joiner
		"\u2061", "", // function application
		"\u2062", "", // invisible times
		"\u2063", "", // invisible separator
		"\u2064", "", // invisible plus
	)
)

// sanitizeInput 对输入做 Unicode 预处理，防止字符混淆绕过正则检测。
// 处理顺序: NFKC 规范化 -> 移除零宽字符 -> 移除方向控制符 -> 归一化空白。
func sanitizeInput(text string) string {
	text = norm.NFKC.String(text)
	text = zeroWidthReplacer.Replace(text)
	text = dirControlRe.ReplaceAllString(text, "")
	text = multiSpaceRe.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}
