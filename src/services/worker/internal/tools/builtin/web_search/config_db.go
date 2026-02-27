package websearch

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	settingProvider      = "web_search.provider"
	settingSearxngURL    = "web_search.searxng_base_url"
	settingTavilyKey     = "web_search.tavily_api_key"
)

// LoadConfigFromDB 从 platform_settings 读取 web_search.* 配置。
// 返回 (cfg, true, nil) 当 DB 中存在 provider 配置；(nil, false, nil) 时调用方应回退到 ENV。
func LoadConfigFromDB(ctx context.Context, pool *pgxpool.Pool) (*Config, bool, error) {
	rows, err := pool.Query(ctx,
		`SELECT key, value FROM platform_settings WHERE key LIKE 'web_search.%'`)
	if err != nil {
		return nil, false, fmt.Errorf("query web_search config: %w", err)
	}
	defer rows.Close()

	m := make(map[string]string, 3)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, false, err
		}
		m[k] = v
	}
	if rows.Err() != nil {
		return nil, false, rows.Err()
	}

	raw := strings.TrimSpace(m[settingProvider])
	if raw == "" {
		return nil, false, nil
	}

	kind, err := parseProviderKind(raw)
	if err != nil {
		return nil, false, err
	}

	cfg := &Config{
		ProviderKind:   kind,
		SearxngBaseURL: strings.TrimRight(strings.TrimSpace(m[settingSearxngURL]), "/"),
		TavilyAPIKey:   strings.TrimSpace(m[settingTavilyKey]),
	}
	return cfg, true, nil
}
