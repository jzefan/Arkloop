package toolprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	workerCrypto "arkloop/services/worker/internal/crypto"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ActiveProviderConfig struct {
	GroupName    string
	ProviderName string
	APIKeyValue  *string
	KeyPrefix    *string
	BaseURL      *string
	ConfigJSON   map[string]any
}

func LoadActiveProviders(ctx context.Context, pool *pgxpool.Pool, orgID uuid.UUID) ([]ActiveProviderConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if pool == nil {
		return nil, nil
	}
	if orgID == uuid.Nil {
		return nil, fmt.Errorf("org_id must not be empty")
	}

	rows, err := pool.Query(ctx, `
		SELECT c.group_name, c.provider_name, c.key_prefix, c.base_url, c.config_json,
		       s.encrypted_value, s.key_version
		FROM tool_provider_configs c
		LEFT JOIN secrets s ON s.id = c.secret_id AND s.org_id = c.org_id
		WHERE c.org_id = $1 AND c.is_active = TRUE
		ORDER BY c.updated_at DESC
	`, orgID)
	if err != nil {
		return nil, fmt.Errorf("tool_provider_configs query: %w", err)
	}
	defer rows.Close()

	out := []ActiveProviderConfig{}
	for rows.Next() {
		var (
			groupName    string
			providerName string
			keyPrefix    *string
			baseURL      *string
			configJSON   []byte
			encrypted    *string
			keyVersion   *int
		)
		if err := rows.Scan(&groupName, &providerName, &keyPrefix, &baseURL, &configJSON, &encrypted, &keyVersion); err != nil {
			return nil, fmt.Errorf("tool_provider_configs scan: %w", err)
		}
		_ = keyVersion

		cfg := ActiveProviderConfig{
			GroupName:    strings.TrimSpace(groupName),
			ProviderName: strings.TrimSpace(providerName),
			KeyPrefix:    keyPrefix,
			BaseURL:      baseURL,
			ConfigJSON:   map[string]any{},
		}

		if len(configJSON) > 0 {
			_ = json.Unmarshal(configJSON, &cfg.ConfigJSON)
		}

		if encrypted != nil && strings.TrimSpace(*encrypted) != "" {
			plainBytes, err := workerCrypto.DecryptGCM(*encrypted)
			if err != nil {
				return nil, fmt.Errorf("tool_provider_configs decrypt: %w", err)
			}
			plaintext := string(plainBytes)
			cfg.APIKeyValue = &plaintext
		}

		out = append(out, cfg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tool_provider_configs rows: %w", err)
	}

	return out, nil
}
