-- +goose Up
ALTER TABLE llm_credentials RENAME TO llm_credentials_legacy_00093;

DROP INDEX IF EXISTS ix_llm_credentials_account_id;
DROP INDEX IF EXISTS llm_credentials_platform_name_idx;
DROP INDEX IF EXISTS llm_credentials_user_name_idx;

CREATE TABLE llm_credentials (
    id              TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id      TEXT NOT NULL DEFAULT '00000000-0000-4000-8000-000000000002' REFERENCES accounts(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL CHECK (provider IN ('openai', 'anthropic', 'gemini', 'deepseek', 'doubao', 'qwen', 'zenmax')),
    name            TEXT NOT NULL,
    secret_id       TEXT,
    key_prefix      TEXT,
    base_url        TEXT,
    openai_api_mode TEXT,
    advanced_json   TEXT NOT NULL DEFAULT '{}',
    revoked_at      TEXT,
    last_used_at    TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
    owner_kind      TEXT NOT NULL DEFAULT 'platform',
    owner_user_id   TEXT REFERENCES users(id) ON DELETE CASCADE
);

INSERT INTO llm_credentials (
    id,
    account_id,
    provider,
    name,
    secret_id,
    key_prefix,
    base_url,
    openai_api_mode,
    advanced_json,
    revoked_at,
    last_used_at,
    created_at,
    updated_at,
    owner_kind,
    owner_user_id
)
SELECT
    id,
    account_id,
    provider,
    name,
    secret_id,
    key_prefix,
    base_url,
    openai_api_mode,
    COALESCE(advanced_json, '{}'),
    revoked_at,
    last_used_at,
    COALESCE(created_at, datetime('now')),
    COALESCE(updated_at, COALESCE(created_at, datetime('now'))),
    COALESCE(owner_kind, 'platform'),
    owner_user_id
FROM llm_credentials_legacy_00093;

CREATE INDEX ix_llm_credentials_account_id ON llm_credentials(account_id);

CREATE UNIQUE INDEX llm_credentials_platform_name_idx
    ON llm_credentials (name)
    WHERE owner_kind = 'platform';

CREATE UNIQUE INDEX llm_credentials_user_name_idx
    ON llm_credentials (owner_user_id, name)
    WHERE owner_kind = 'user' AND owner_user_id IS NOT NULL;

DROP TABLE llm_credentials_legacy_00093;

ALTER TABLE llm_routes RENAME TO llm_routes_legacy_00093;

DROP INDEX IF EXISTS ix_llm_routes_account_id;
DROP INDEX IF EXISTS ix_llm_routes_credential_id;
DROP INDEX IF EXISTS ix_llm_routes_project_id;
DROP INDEX IF EXISTS ux_llm_routes_credential_model_lower;
DROP INDEX IF EXISTS ux_llm_routes_credential_default;
DROP INDEX IF EXISTS ux_llm_routes_route_key;

CREATE TABLE llm_routes (
    id                     TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id             TEXT NOT NULL DEFAULT '00000000-0000-4000-8000-000000000002' REFERENCES accounts(id) ON DELETE CASCADE,
    project_id             TEXT REFERENCES projects(id) ON DELETE CASCADE,
    credential_id          TEXT NOT NULL REFERENCES llm_credentials(id) ON DELETE CASCADE,
    model                  TEXT NOT NULL,
    priority               INTEGER NOT NULL DEFAULT 0,
    is_default             INTEGER NOT NULL DEFAULT 0,
    tags                   TEXT NOT NULL DEFAULT '[]',
    when_json              TEXT NOT NULL DEFAULT '{}',
    advanced_json          TEXT NOT NULL DEFAULT '{}',
    multiplier             REAL NOT NULL DEFAULT 1.0,
    cost_per_1k_input      REAL,
    cost_per_1k_output     REAL,
    cost_per_1k_cache_write REAL,
    cost_per_1k_cache_read REAL,
    created_at             TEXT NOT NULL DEFAULT (datetime('now')),
    route_key              TEXT,
    show_in_picker         INTEGER NOT NULL DEFAULT 1
);

INSERT INTO llm_routes (
    id,
    account_id,
    project_id,
    credential_id,
    model,
    priority,
    is_default,
    tags,
    when_json,
    advanced_json,
    multiplier,
    cost_per_1k_input,
    cost_per_1k_output,
    cost_per_1k_cache_write,
    cost_per_1k_cache_read,
    created_at,
    route_key,
    show_in_picker
)
SELECT
    id,
    account_id,
    project_id,
    credential_id,
    model,
    priority,
    is_default,
    COALESCE(tags, '[]'),
    COALESCE(when_json, '{}'),
    COALESCE(advanced_json, '{}'),
    COALESCE(multiplier, 1.0),
    cost_per_1k_input,
    cost_per_1k_output,
    cost_per_1k_cache_write,
    cost_per_1k_cache_read,
    COALESCE(created_at, datetime('now')),
    route_key,
    COALESCE(show_in_picker, 1)
FROM llm_routes_legacy_00093;

CREATE INDEX ix_llm_routes_account_id ON llm_routes(account_id);
CREATE INDEX ix_llm_routes_credential_id ON llm_routes(credential_id);
CREATE INDEX ix_llm_routes_project_id
    ON llm_routes(project_id)
    WHERE project_id IS NOT NULL;
CREATE UNIQUE INDEX ux_llm_routes_credential_model_lower
    ON llm_routes (credential_id, lower(model));
CREATE UNIQUE INDEX ux_llm_routes_credential_default
    ON llm_routes (credential_id)
    WHERE is_default = 1;
CREATE UNIQUE INDEX ux_llm_routes_route_key
    ON llm_routes (lower(route_key))
    WHERE route_key IS NOT NULL;

DROP TABLE llm_routes_legacy_00093;

-- +goose Down
ALTER TABLE llm_routes RENAME TO llm_routes_rollback_00093;

DROP INDEX IF EXISTS ix_llm_routes_account_id;
DROP INDEX IF EXISTS ix_llm_routes_credential_id;
DROP INDEX IF EXISTS ix_llm_routes_project_id;
DROP INDEX IF EXISTS ux_llm_routes_credential_model_lower;
DROP INDEX IF EXISTS ux_llm_routes_credential_default;
DROP INDEX IF EXISTS ux_llm_routes_route_key;

CREATE TABLE llm_routes (
    id                     TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id             TEXT NOT NULL DEFAULT '00000000-0000-4000-8000-000000000002' REFERENCES accounts(id) ON DELETE CASCADE,
    project_id             TEXT REFERENCES projects(id) ON DELETE CASCADE,
    credential_id          TEXT NOT NULL REFERENCES llm_credentials(id) ON DELETE CASCADE,
    model                  TEXT NOT NULL,
    priority               INTEGER NOT NULL DEFAULT 0,
    is_default             INTEGER NOT NULL DEFAULT 0,
    tags                   TEXT NOT NULL DEFAULT '[]',
    when_json              TEXT NOT NULL DEFAULT '{}',
    advanced_json          TEXT NOT NULL DEFAULT '{}',
    multiplier             REAL NOT NULL DEFAULT 1.0,
    cost_per_1k_input      REAL,
    cost_per_1k_output     REAL,
    cost_per_1k_cache_write REAL,
    cost_per_1k_cache_read REAL,
    created_at             TEXT NOT NULL DEFAULT (datetime('now')),
    route_key              TEXT,
    show_in_picker         INTEGER NOT NULL DEFAULT 1
);

INSERT INTO llm_routes (
    id,
    account_id,
    project_id,
    credential_id,
    model,
    priority,
    is_default,
    tags,
    when_json,
    advanced_json,
    multiplier,
    cost_per_1k_input,
    cost_per_1k_output,
    cost_per_1k_cache_write,
    cost_per_1k_cache_read,
    created_at,
    route_key,
    show_in_picker
)
SELECT
    id,
    account_id,
    project_id,
    credential_id,
    model,
    priority,
    is_default,
    COALESCE(tags, '[]'),
    COALESCE(when_json, '{}'),
    COALESCE(advanced_json, '{}'),
    COALESCE(multiplier, 1.0),
    cost_per_1k_input,
    cost_per_1k_output,
    cost_per_1k_cache_write,
    cost_per_1k_cache_read,
    COALESCE(created_at, datetime('now')),
    route_key,
    COALESCE(show_in_picker, 1)
FROM llm_routes_rollback_00093;

CREATE INDEX ix_llm_routes_account_id ON llm_routes(account_id);
CREATE INDEX ix_llm_routes_credential_id ON llm_routes(credential_id);
CREATE INDEX ix_llm_routes_project_id
    ON llm_routes(project_id)
    WHERE project_id IS NOT NULL;
CREATE UNIQUE INDEX ux_llm_routes_credential_model
    ON llm_routes (credential_id, model);
CREATE UNIQUE INDEX ux_llm_routes_credential_default
    ON llm_routes (credential_id)
    WHERE is_default = 1;
CREATE UNIQUE INDEX ux_llm_routes_route_key
    ON llm_routes (lower(route_key))
    WHERE route_key IS NOT NULL;

DROP TABLE llm_routes_rollback_00093;

ALTER TABLE llm_credentials RENAME TO llm_credentials_rollback_00093;

DROP INDEX IF EXISTS ix_llm_credentials_account_id;
DROP INDEX IF EXISTS llm_credentials_platform_name_idx;
DROP INDEX IF EXISTS llm_credentials_user_name_idx;

CREATE TABLE llm_credentials (
    id              TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id      TEXT NOT NULL DEFAULT '00000000-0000-4000-8000-000000000002' REFERENCES accounts(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL CHECK (provider IN ('openai', 'anthropic', 'gemini', 'deepseek', 'zenmax')),
    name            TEXT NOT NULL,
    secret_id       TEXT,
    key_prefix      TEXT,
    base_url        TEXT,
    openai_api_mode TEXT,
    advanced_json   TEXT NOT NULL DEFAULT '{}',
    revoked_at      TEXT,
    last_used_at    TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now')),
    owner_kind      TEXT NOT NULL DEFAULT 'platform',
    owner_user_id   TEXT REFERENCES users(id) ON DELETE CASCADE
);

INSERT INTO llm_credentials (
    id,
    account_id,
    provider,
    name,
    secret_id,
    key_prefix,
    base_url,
    openai_api_mode,
    advanced_json,
    revoked_at,
    last_used_at,
    created_at,
    updated_at,
    owner_kind,
    owner_user_id
)
SELECT
    id,
    account_id,
    provider,
    name,
    secret_id,
    key_prefix,
    base_url,
    openai_api_mode,
    COALESCE(advanced_json, '{}'),
    revoked_at,
    last_used_at,
    COALESCE(created_at, datetime('now')),
    COALESCE(updated_at, COALESCE(created_at, datetime('now'))),
    COALESCE(owner_kind, 'platform'),
    owner_user_id
FROM llm_credentials_rollback_00093
WHERE provider IN ('openai', 'anthropic', 'gemini', 'deepseek', 'zenmax');

CREATE INDEX ix_llm_credentials_account_id ON llm_credentials(account_id);

CREATE UNIQUE INDEX llm_credentials_platform_name_idx
    ON llm_credentials (name)
    WHERE owner_kind = 'platform';

CREATE UNIQUE INDEX llm_credentials_user_name_idx
    ON llm_credentials (owner_user_id, name)
    WHERE owner_kind = 'user' AND owner_user_id IS NOT NULL;

DROP TABLE llm_credentials_rollback_00093;
