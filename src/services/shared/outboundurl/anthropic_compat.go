package outboundurl

import (
	"net/url"
	"strings"
)

// NormalizeAnthropicCompatibleBaseURL aligns provider base_url with vendors
// that expose Anthropic-compatible APIs behind an unversioned prefix.
func NormalizeAnthropicCompatibleBaseURL(raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return ""
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}
	if !strings.EqualFold(parsed.Hostname(), "api.minimaxi.com") {
		return trimmed
	}

	path := strings.TrimRight(parsed.Path, "/")
	if path != "/anthropic" {
		return trimmed
	}
	parsed.Path = path + "/v1"
	return strings.TrimRight(parsed.String(), "/")
}
