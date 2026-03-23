-- Migrate asr_credentials to match PG schema post-migration-00118.
-- The old table (00016) only had: account_id, provider, api_key, created_at, updated_at.
-- The new schema needs: owner_kind, owner_user_id, name, secret_id, key_prefix,
-- base_url, model, is_default, revoked_at.

-- +goose Up

PRAGMA foreign_keys = OFF;

ALTER TABLE asr_credentials RENAME TO asr_credentials_legacy_00042;

CREATE TABLE asr_credentials (
    id            TEXT PRIMARY KEY DEFAULT (
        lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' ||
        substr(lower(hex(randomblob(2))),2) || '-' ||
        substr('89ab',abs(random()) % 4 + 1, 1) ||
        substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id    TEXT NOT NULL DEFAULT '00000000-0000-4000-8000-000000000002' REFERENCES accounts(id) ON DELETE CASCADE,
    owner_kind    TEXT NOT NULL DEFAULT 'user' CHECK (owner_kind IN ('platform', 'user')),
    owner_user_id TEXT REFERENCES users(id) ON DELETE CASCADE,
    provider      TEXT NOT NULL,
    name          TEXT NOT NULL,
    secret_id     TEXT REFERENCES secrets(id) ON DELETE SET NULL,
    key_prefix    TEXT,
    base_url      TEXT,
    model         TEXT NOT NULL,
    is_default    INTEGER NOT NULL DEFAULT 0,
    revoked_at    TEXT,
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now')),
    -- legacy column kept for data safety on upgrade; not read by new app code
    api_key_legacy TEXT
);

INSERT INTO asr_credentials (id, account_id, owner_kind, owner_user_id, provider, name, created_at, updated_at, api_key_legacy)
SELECT
    id,
    account_id,
    'user' AS owner_kind,
    (SELECT id FROM users LIMIT 1) AS owner_user_id,
    provider,
    provider || '-' || id AS name,
    created_at,
    updated_at,
    api_key
FROM asr_credentials_legacy_00042;

DROP TABLE asr_credentials_legacy_00042;

CREATE UNIQUE INDEX IF NOT EXISTS asr_credentials_user_name_idx
    ON asr_credentials (owner_user_id, name)
    WHERE owner_kind = 'user' AND owner_user_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS asr_credentials_platform_name_idx
    ON asr_credentials (name)
    WHERE owner_kind = 'platform';

CREATE UNIQUE INDEX IF NOT EXISTS asr_credentials_user_default_idx
    ON asr_credentials (owner_user_id)
    WHERE owner_kind = 'user' AND is_default = 1 AND revoked_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS asr_credentials_platform_default_idx
    ON asr_credentials (is_default)
    WHERE owner_kind = 'platform' AND is_default = 1 AND revoked_at IS NULL;

PRAGMA foreign_keys = ON;

-- +goose Down

PRAGMA foreign_keys = OFF;

ALTER TABLE asr_credentials RENAME TO asr_credentials_new_00042;

CREATE TABLE asr_credentials (
    id         TEXT PRIMARY KEY DEFAULT (
        lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' ||
        substr(lower(hex(randomblob(2))),2) || '-' ||
        substr('89ab',abs(random()) % 4 + 1, 1) ||
        substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id TEXT NOT NULL,
    provider  TEXT NOT NULL,
    api_key   TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO asr_credentials (id, account_id, provider, created_at, updated_at)
SELECT id, account_id, provider, created_at, updated_at
FROM asr_credentials_new_00042;

DROP TABLE asr_credentials_new_00042;

PRAGMA foreign_keys = ON;
