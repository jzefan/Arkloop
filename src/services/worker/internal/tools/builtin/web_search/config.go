package websearch

import (
	"fmt"
	"os"
	"strings"
)

const (
	webSearchProviderEnv = "ARKLOOP_WEB_SEARCH_PROVIDER"
	searxngBaseURLEnv    = "ARKLOOP_WEB_SEARCH_SEARXNG_BASE_URL"
	tavilyAPIKeyEnv      = "ARKLOOP_WEB_SEARCH_TAVILY_API_KEY"
)

const (
	settingProvider    = "web_search.provider"
	settingSearxngURL  = "web_search.searxng_base_url"
	settingTavilyKey   = "web_search.tavily_api_key"
)

type ProviderKind string

const (
	ProviderKindSearxng     ProviderKind = "searxng"
	ProviderKindTavily      ProviderKind = "tavily"
	ProviderKindSerper      ProviderKind = "serper"
	ProviderKindDuckduckgo  ProviderKind = "duckduckgo"
)

type Config struct {
	ProviderKind   ProviderKind
	SearxngBaseURL string
	TavilyAPIKey   string
}

func ConfigFromEnv(required bool) (*Config, error) {
	raw := strings.TrimSpace(os.Getenv(webSearchProviderEnv))
	if raw == "" {
		if required {
			return nil, fmt.Errorf("missing environment variable %s", webSearchProviderEnv)
		}
		return nil, nil
	}

	kind, err := parseProviderKind(raw)
	if err != nil {
		return nil, err
	}

	switch kind {
	case ProviderKindSearxng:
		baseURL := strings.TrimSpace(os.Getenv(searxngBaseURLEnv))
		if baseURL == "" {
			return nil, fmt.Errorf("missing environment variable %s", searxngBaseURLEnv)
		}
		baseURL = strings.TrimRight(baseURL, "/")
		return &Config{
			ProviderKind:   kind,
			SearxngBaseURL: baseURL,
		}, nil
	case ProviderKindTavily:
		apiKey := strings.TrimSpace(os.Getenv(tavilyAPIKeyEnv))
		if apiKey == "" {
			return nil, fmt.Errorf("missing environment variable %s", tavilyAPIKeyEnv)
		}
		return &Config{
			ProviderKind: kind,
			TavilyAPIKey: apiKey,
		}, nil
	case ProviderKindSerper:
		return &Config{ProviderKind: kind}, nil
	case ProviderKindDuckduckgo:
		return &Config{ProviderKind: kind}, nil
	default:
		return nil, fmt.Errorf("%s must be searxng/tavily/serper/duckduckgo", webSearchProviderEnv)
	}
}

func parseProviderKind(raw string) (ProviderKind, error) {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(raw), "-", "_"))
	switch normalized {
	case "searxng":
		return ProviderKindSearxng, nil
	case "tavily":
		return ProviderKindTavily, nil
	case "serper":
		return ProviderKindSerper, nil
	case "duckduckgo":
		return ProviderKindDuckduckgo, nil
	case "browser":
		// 旧桌面内置搜索已移除；曾写入 platform_settings / 环境变量者仍解析为 DuckDuckGo
		return ProviderKindDuckduckgo, nil
	default:
		return "", fmt.Errorf("%s must be searxng/tavily/serper/duckduckgo", webSearchProviderEnv)
	}
}
