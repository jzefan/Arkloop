-- Align tool_provider_configs with PostgreSQL owner_kind / partial unique indexes
-- so ToolProviderConfigsRepository and /v1/tool-providers work on desktop.

-- +goose Up

PRAGMA foreign_keys = OFF;

ALTER TABLE tool_provider_configs RENAME TO tool_provider_configs_legacy_00034;

CREATE TABLE tool_provider_configs (
    id              TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id      TEXT NOT NULL DEFAULT '00000000-0000-4000-8000-000000000002' REFERENCES accounts(id) ON DELETE CASCADE,
    owner_kind      TEXT NOT NULL DEFAULT 'platform' CHECK (owner_kind IN ('platform', 'user')),
    owner_user_id   TEXT REFERENCES users(id) ON DELETE CASCADE,
    group_name      TEXT NOT NULL,
    provider_name   TEXT NOT NULL,
    is_active       INTEGER NOT NULL DEFAULT 0,
    secret_id       TEXT,
    key_prefix      TEXT,
    base_url        TEXT,
    config_json     TEXT NOT NULL DEFAULT '{}',
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO tool_provider_configs (
    id, account_id, owner_kind, owner_user_id, group_name, provider_name,
    is_active, secret_id, key_prefix, base_url, config_json, created_at, updated_at
)
SELECT
    id,
    COALESCE(account_id, '00000000-0000-4000-8000-000000000002'),
    'platform',
    NULL,
    group_name,
    provider_name,
    is_active,
    secret_id,
    key_prefix,
    base_url,
    COALESCE(config_json, '{}'),
    COALESCE(created_at, datetime('now')),
    COALESCE(updated_at, datetime('now'))
FROM tool_provider_configs_legacy_00034;

CREATE UNIQUE INDEX tool_provider_configs_platform_provider_idx
    ON tool_provider_configs (provider_name)
    WHERE owner_kind = 'platform';

CREATE UNIQUE INDEX ix_tool_provider_configs_platform_group_active
    ON tool_provider_configs (group_name)
    WHERE owner_kind = 'platform' AND is_active = 1;

CREATE UNIQUE INDEX tool_provider_configs_user_provider_idx
    ON tool_provider_configs (owner_user_id, provider_name)
    WHERE owner_kind = 'user' AND owner_user_id IS NOT NULL;

CREATE UNIQUE INDEX ix_tool_provider_configs_user_group_active
    ON tool_provider_configs (owner_user_id, group_name)
    WHERE owner_kind = 'user' AND owner_user_id IS NOT NULL AND is_active = 1;

DROP TABLE tool_provider_configs_legacy_00034;

PRAGMA foreign_keys = ON;

-- +goose Down

PRAGMA foreign_keys = OFF;

DROP INDEX IF EXISTS ix_tool_provider_configs_user_group_active;
DROP INDEX IF EXISTS tool_provider_configs_user_provider_idx;
DROP INDEX IF EXISTS ix_tool_provider_configs_platform_group_active;
DROP INDEX IF EXISTS tool_provider_configs_platform_provider_idx;

ALTER TABLE tool_provider_configs RENAME TO tool_provider_configs_new_00034;

CREATE TABLE tool_provider_configs (
    id            TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id    TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    project_id    TEXT,
    group_name    TEXT NOT NULL,
    provider_name TEXT NOT NULL,
    is_active     INTEGER NOT NULL DEFAULT 0,
    secret_id     TEXT,
    key_prefix    TEXT,
    base_url      TEXT,
    config_json   TEXT NOT NULL DEFAULT '{}',
    scope         TEXT NOT NULL DEFAULT 'org',
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (account_id, provider_name)
);

CREATE UNIQUE INDEX ix_tool_provider_configs_org_group_active
    ON tool_provider_configs (account_id, group_name)
    WHERE is_active = 1;

INSERT INTO tool_provider_configs (
    id, account_id, project_id, group_name, provider_name, is_active, secret_id, key_prefix, base_url, config_json, scope, created_at, updated_at
)
SELECT
    id,
    account_id,
    NULL,
    group_name,
    provider_name,
    is_active,
    secret_id,
    key_prefix,
    base_url,
    config_json,
    'org',
    created_at,
    updated_at
FROM tool_provider_configs_new_00034;

DROP TABLE tool_provider_configs_new_00034;

PRAGMA foreign_keys = ON;
