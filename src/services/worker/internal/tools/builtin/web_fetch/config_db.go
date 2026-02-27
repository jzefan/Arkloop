package webfetch

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	settingProvider       = "web_fetch.provider"
	settingFirecrawlKey   = "web_fetch.firecrawl_api_key"
	settingFirecrawlURL   = "web_fetch.firecrawl_base_url"
	settingJinaKey        = "web_fetch.jina_api_key"
)

// LoadConfigFromDB 从 platform_settings 读取 web_fetch.* 配置。
// 返回 (cfg, true, nil) 当 DB 中存在 provider 配置；(nil, false, nil) 时调用方应回退到 ENV。
func LoadConfigFromDB(ctx context.Context, pool *pgxpool.Pool) (*Config, bool, error) {
	rows, err := pool.Query(ctx,
		`SELECT key, value FROM platform_settings WHERE key LIKE 'web_fetch.%'`)
	if err != nil {
		return nil, false, fmt.Errorf("query web_fetch config: %w", err)
	}
	defer rows.Close()

	m := make(map[string]string, 4)
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
		ProviderKind:     kind,
		FirecrawlAPIKey:  strings.TrimSpace(m[settingFirecrawlKey]),
		FirecrawlBaseURL: strings.TrimRight(strings.TrimSpace(m[settingFirecrawlURL]), "/"),
		JinaAPIKey:       strings.TrimSpace(m[settingJinaKey]),
	}
	return cfg, true, nil
}
