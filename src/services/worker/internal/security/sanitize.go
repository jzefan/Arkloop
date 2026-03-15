package security

import (
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

var (
	// 方向控制字符 (U+202A-U+202E, U+2066-U+2069)
	dirControlRe = regexp.MustCompile("[\u202A-\u202E\u2066-\u2069]")
	// 零宽 / 不可见控制字符，含 soft hyphen、RTL mark、word joiner 等。
	zeroWidthRe = regexp.MustCompile("[\u00AD\u200B-\u200F\u2060-\u2064\uFEFF]")
	// 连续空白，含 Unicode 空格 (\p{Z} 覆盖 Zs/Zl/Zp)
	multiSpaceRe = regexp.MustCompile(`[\s\p{Z}]+`)
)

// sanitizeInput 对输入做 Unicode 预处理，防止字符混淆绕过正则检测。
// 处理顺序: NFKC 规范化 -> 移除零宽字符 -> 移除方向控制符 -> 归一化空白。
func sanitizeInput(text string) string {
	text = norm.NFKC.String(text)
	text = zeroWidthRe.ReplaceAllString(text, "")
	text = dirControlRe.ReplaceAllString(text, "")
	text = multiSpaceRe.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}
