-- +goose Up
ALTER TABLE llm_credentials RENAME TO llm_credentials_legacy_00094;

DROP INDEX IF EXISTS ix_llm_credentials_account_id;
DROP INDEX IF EXISTS llm_credentials_platform_name_idx;
DROP INDEX IF EXISTS llm_credentials_user_name_idx;

CREATE TABLE llm_credentials (
    id              TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id      TEXT NOT NULL DEFAULT '00000000-0000-4000-8000-000000000002' REFERENCES accounts(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL CHECK (provider IN ('openai', 'anthropic', 'gemini', 'deepseek', 'doubao', 'qwen', 'yuanbao', 'kimi', 'zenmax')),
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
FROM llm_credentials_legacy_00094;

CREATE INDEX ix_llm_credentials_account_id ON llm_credentials(account_id);

CREATE UNIQUE INDEX llm_credentials_platform_name_idx
    ON llm_credentials (name)
    WHERE owner_kind = 'platform';

CREATE UNIQUE INDEX llm_credentials_user_name_idx
    ON llm_credentials (owner_user_id, name)
    WHERE owner_kind = 'user' AND owner_user_id IS NOT NULL;

DROP TABLE llm_credentials_legacy_00094;

-- +goose Down
ALTER TABLE llm_credentials RENAME TO llm_credentials_rollback_00094;

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
FROM llm_credentials_rollback_00094
WHERE provider IN ('openai', 'anthropic', 'gemini', 'deepseek', 'doubao', 'qwen', 'zenmax');

CREATE INDEX ix_llm_credentials_account_id ON llm_credentials(account_id);

CREATE UNIQUE INDEX llm_credentials_platform_name_idx
    ON llm_credentials (name)
    WHERE owner_kind = 'platform';

CREATE UNIQUE INDEX llm_credentials_user_name_idx
    ON llm_credentials (owner_user_id, name)
    WHERE owner_kind = 'user' AND owner_user_id IS NOT NULL;

DROP TABLE llm_credentials_rollback_00094;
