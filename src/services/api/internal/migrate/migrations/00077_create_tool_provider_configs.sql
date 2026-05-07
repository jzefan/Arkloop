-- +goose Up
CREATE TABLE tool_provider_configs (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    group_name    text NOT NULL,
    provider_name text NOT NULL,
    is_active     boolean NOT NULL DEFAULT false,
    secret_id     uuid REFERENCES secrets(id) ON DELETE SET NULL,
    key_prefix    text,
    base_url      text,
    config_json   jsonb NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE(org_id, provider_name)
);

CREATE INDEX ix_tool_provider_configs_org_group_active
    ON tool_provider_configs (org_id, group_name)
    WHERE is_active = true;

-- +goose Down
DROP INDEX IF EXISTS ix_tool_provider_configs_org_group_active;
DROP TABLE IF EXISTS tool_provider_configs;

