package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// WithTx 返回一个使用给定事务的 ToolProviderConfigsRepository 副本。
func (r *ToolProviderConfigsRepository) WithTx(tx pgx.Tx) *ToolProviderConfigsRepository {
	return &ToolProviderConfigsRepository{db: tx}
}

type ToolProviderConfig struct {
	ID           uuid.UUID
	OrgID        uuid.UUID
	GroupName    string
	ProviderName string
	IsActive     bool
	SecretID     *uuid.UUID
	KeyPrefix    *string
	BaseURL      *string
	ConfigJSON   json.RawMessage
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ToolProviderConfigsRepository struct {
	db Querier
}

func NewToolProviderConfigsRepository(db Querier) (*ToolProviderConfigsRepository, error) {
	if db == nil {
		return nil, errors.New("db must not be nil")
	}
	return &ToolProviderConfigsRepository{db: db}, nil
}

func (r *ToolProviderConfigsRepository) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]ToolProviderConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if orgID == uuid.Nil {
		return nil, fmt.Errorf("org_id must not be empty")
	}

	rows, err := r.db.Query(ctx, `
		SELECT id, org_id, group_name, provider_name, is_active, secret_id, key_prefix, base_url, config_json, created_at, updated_at
		FROM tool_provider_configs
		WHERE org_id = $1
		ORDER BY group_name ASC, provider_name ASC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ToolProviderConfig{}
	for rows.Next() {
		var cfg ToolProviderConfig
		if err := rows.Scan(
			&cfg.ID,
			&cfg.OrgID,
			&cfg.GroupName,
			&cfg.ProviderName,
			&cfg.IsActive,
			&cfg.SecretID,
			&cfg.KeyPrefix,
			&cfg.BaseURL,
			&cfg.ConfigJSON,
			&cfg.CreatedAt,
			&cfg.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, cfg)
	}
	return out, rows.Err()
}

func (r *ToolProviderConfigsRepository) UpsertConfig(
	ctx context.Context,
	orgID uuid.UUID,
	groupName string,
	providerName string,
	secretID *uuid.UUID,
	keyPrefix *string,
	baseURL *string,
	configJSON []byte,
) (ToolProviderConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if orgID == uuid.Nil {
		return ToolProviderConfig{}, fmt.Errorf("org_id must not be empty")
	}

	group := strings.TrimSpace(groupName)
	provider := strings.TrimSpace(providerName)
	if group == "" {
		return ToolProviderConfig{}, fmt.Errorf("group_name must not be empty")
	}
	if provider == "" {
		return ToolProviderConfig{}, fmt.Errorf("provider_name must not be empty")
	}

	if configJSON != nil && len(configJSON) == 0 {
		configJSON = []byte("{}")
	}

	var cfg ToolProviderConfig
	err := r.db.QueryRow(ctx, `
		INSERT INTO tool_provider_configs (org_id, group_name, provider_name, secret_id, key_prefix, base_url, config_json)
		VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, '{}'::jsonb))
		ON CONFLICT (org_id, provider_name)
		DO UPDATE SET
			group_name  = EXCLUDED.group_name,
			secret_id   = COALESCE(EXCLUDED.secret_id, tool_provider_configs.secret_id),
			key_prefix  = COALESCE(EXCLUDED.key_prefix, tool_provider_configs.key_prefix),
			base_url    = COALESCE(EXCLUDED.base_url, tool_provider_configs.base_url),
			config_json = CASE WHEN $7 IS NULL THEN tool_provider_configs.config_json ELSE EXCLUDED.config_json END,
			updated_at  = now()
		RETURNING id, org_id, group_name, provider_name, is_active, secret_id, key_prefix, base_url, config_json, created_at, updated_at
	`, orgID, group, provider, secretID, keyPrefix, baseURL, configJSON).Scan(
		&cfg.ID,
		&cfg.OrgID,
		&cfg.GroupName,
		&cfg.ProviderName,
		&cfg.IsActive,
		&cfg.SecretID,
		&cfg.KeyPrefix,
		&cfg.BaseURL,
		&cfg.ConfigJSON,
		&cfg.CreatedAt,
		&cfg.UpdatedAt,
	)
	if err != nil {
		return ToolProviderConfig{}, err
	}
	return cfg, nil
}

// Activate 事务内调用：同组全关 + 指定 provider 打开。
func (r *ToolProviderConfigsRepository) Activate(ctx context.Context, orgID uuid.UUID, groupName string, providerName string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if orgID == uuid.Nil {
		return fmt.Errorf("org_id must not be empty")
	}
	group := strings.TrimSpace(groupName)
	provider := strings.TrimSpace(providerName)
	if group == "" || provider == "" {
		return fmt.Errorf("group_name and provider_name must not be empty")
	}

	if _, err := r.db.Exec(ctx, `
		UPDATE tool_provider_configs
		SET is_active = FALSE, updated_at = now()
		WHERE org_id = $1 AND group_name = $2 AND is_active = TRUE
	`, orgID, group); err != nil {
		return err
	}

	_, err := r.db.Exec(ctx, `
		INSERT INTO tool_provider_configs (org_id, group_name, provider_name, is_active)
		VALUES ($1, $2, $3, TRUE)
		ON CONFLICT (org_id, provider_name)
		DO UPDATE SET
			group_name = EXCLUDED.group_name,
			is_active  = TRUE,
			updated_at = now()
	`, orgID, group, provider)
	return err
}

func (r *ToolProviderConfigsRepository) Deactivate(ctx context.Context, orgID uuid.UUID, groupName string, providerName string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if orgID == uuid.Nil {
		return fmt.Errorf("org_id must not be empty")
	}
	group := strings.TrimSpace(groupName)
	provider := strings.TrimSpace(providerName)
	if group == "" || provider == "" {
		return fmt.Errorf("group_name and provider_name must not be empty")
	}

	_, err := r.db.Exec(ctx, `
		UPDATE tool_provider_configs
		SET is_active = FALSE, updated_at = now()
		WHERE org_id = $1 AND group_name = $2 AND provider_name = $3
	`, orgID, group, provider)
	return err
}

func (r *ToolProviderConfigsRepository) ClearCredential(ctx context.Context, orgID uuid.UUID, providerName string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if orgID == uuid.Nil {
		return fmt.Errorf("org_id must not be empty")
	}
	provider := strings.TrimSpace(providerName)
	if provider == "" {
		return fmt.Errorf("provider_name must not be empty")
	}

	_, err := r.db.Exec(ctx, `
		UPDATE tool_provider_configs
		SET is_active = FALSE,
		    secret_id = NULL,
		    key_prefix = NULL,
		    base_url = NULL,
		    config_json = '{}'::jsonb,
		    updated_at = now()
		WHERE org_id = $1 AND provider_name = $2
	`, orgID, provider)
	return err
}
