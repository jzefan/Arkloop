-- Core identity tables: orgs, users, org_memberships
--
-- SQLite UUID default (reused across all migrations):
--   (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' ||
--    substr(lower(hex(randomblob(2))),2) || '-' ||
--    substr('89ab',abs(random()) % 4 + 1, 1) ||
--    substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6))))

-- +goose Up

CREATE TABLE orgs (
    id         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    slug       TEXT NOT NULL UNIQUE,
    name       TEXT NOT NULL,
    type       TEXT NOT NULL DEFAULT 'personal' CHECK (type IN ('personal', 'workspace')),
    owner_user_id TEXT,
    status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE users (
    id                TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    username          TEXT NOT NULL,
    email             TEXT,
    email_verified_at TEXT,
    status            TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleted')),
    deleted_at        TEXT,
    avatar_url        TEXT,
    locale            TEXT,
    timezone          TEXT,
    last_login_at     TEXT,
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX uq_users_email ON users (email) WHERE deleted_at IS NULL;

CREATE TABLE org_memberships (
    id         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    org_id     TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'member',
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (org_id, user_id)
);

CREATE INDEX ix_org_memberships_org_id ON org_memberships(org_id);
CREATE INDEX ix_org_memberships_user_id ON org_memberships(user_id);

-- +goose Down

DROP INDEX IF EXISTS ix_org_memberships_user_id;
DROP INDEX IF EXISTS ix_org_memberships_org_id;
DROP TABLE IF EXISTS org_memberships;
DROP INDEX IF EXISTS uq_users_email;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS orgs;
