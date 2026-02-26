package ua

import (
	"net/http"
	"strings"
)

// Type 表示 User-Agent 的分类。
type Type string

const (
	TypeHuman   Type = "human"   // 普通浏览器
	TypeBot     Type = "bot"     // 爬虫 / 搜索引擎
	TypeTool    Type = "tool"    // HTTP 调试工具（curl/httpie/Postman 等）或 SDK
	TypeUnknown Type = "unknown" // 空 UA 或无法识别
)

// Info 是 UA 解析结果。
type Info struct {
	Raw  string
	Type Type
}

// Parse 解析请求的 User-Agent header。
func Parse(r *http.Request) Info {
	raw := r.Header.Get("User-Agent")
	return Info{Raw: raw, Type: classify(raw)}
}

func classify(ua string) Type {
	if strings.TrimSpace(ua) == "" {
		return TypeUnknown
	}

	lower := strings.ToLower(ua)

	// 已知爬虫/机器人特征
	for _, pattern := range botPatterns {
		if strings.Contains(lower, pattern) {
			return TypeBot
		}
	}

	// HTTP 工具 / SDK 特征
	for _, pattern := range toolPatterns {
		if strings.Contains(lower, pattern) {
			return TypeTool
		}
	}

	// 包含浏览器特征字符串则视为人类用户
	for _, pattern := range humanPatterns {
		if strings.Contains(lower, pattern) {
			return TypeHuman
		}
	}

	// 无法归类的 UA（可能是自定义 SDK 等）归为 tool
	return TypeTool
}

var botPatterns = []string{
	"bot", "crawler", "spider", "scraper", "slurp",
	"facebookexternalhit", "twitterbot", "linkedinbot",
	"googlebot", "bingbot", "yandex", "duckduckbot",
	"semrushbot", "ahrefsbot", "mj12bot",
	"wget", "python-requests", "python-urllib", "java/",
	"go-http-client",
}

var toolPatterns = []string{
	"curl/", "httpie/", "insomnia/", "postman",
	"axios/", "node-fetch", "got/", "ky/", "superagent",
	"okhttp/", "apache-httpclient", "unirest",
	"dart:", "cfnetwork/", "nsurlsession",
}

var humanPatterns = []string{
	"mozilla/", "chrome/", "safari/", "firefox/",
	"edge/", "opera/", "webkit/",
}
