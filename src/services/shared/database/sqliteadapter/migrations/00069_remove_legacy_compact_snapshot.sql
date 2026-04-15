-- +goose NO TRANSACTION
-- +goose Up

PRAGMA foreign_keys = OFF;

DROP INDEX IF EXISTS ix_messages_thread_compacted;

ALTER TABLE messages RENAME TO messages_old_00069;

CREATE TABLE messages (
    id                 TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    thread_id          TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    account_id         TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    thread_seq         INTEGER NOT NULL,
    created_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    role               TEXT NOT NULL,
    content            TEXT NOT NULL,
    content_json       TEXT,
    metadata_json      TEXT NOT NULL DEFAULT '{}',
    hidden             INTEGER NOT NULL DEFAULT 0,
    deleted_at         TEXT,
    token_count        INTEGER,
    created_at         TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO messages (
    id, thread_id, account_id, thread_seq, created_by_user_id, role, content, content_json,
    metadata_json, hidden, deleted_at, token_count, created_at
)
SELECT
    id, thread_id, account_id, thread_seq, created_by_user_id, role, content, content_json,
    metadata_json, hidden, deleted_at, token_count, created_at
FROM messages_old_00069;

ALTER TABLE channel_message_ledger RENAME TO channel_message_ledger_old_00069;

CREATE TABLE channel_message_ledger (
    id                         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    channel_id                 TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    channel_type               TEXT NOT NULL,
    direction                  TEXT NOT NULL,
    thread_id                  TEXT REFERENCES threads(id) ON DELETE SET NULL,
    run_id                     TEXT REFERENCES runs(id) ON DELETE SET NULL,
    platform_conversation_id   TEXT NOT NULL,
    platform_message_id        TEXT NOT NULL,
    platform_parent_message_id TEXT,
    platform_thread_id         TEXT,
    sender_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
    metadata_json              TEXT NOT NULL DEFAULT '{}',
    created_at                 TEXT NOT NULL DEFAULT (datetime('now')),
    message_id                 TEXT REFERENCES messages(id) ON DELETE SET NULL,
    CHECK (direction IN ('inbound', 'outbound')),
    UNIQUE (channel_id, direction, platform_conversation_id, platform_message_id)
);

INSERT INTO channel_message_ledger (
    id, channel_id, channel_type, direction, thread_id, run_id, platform_conversation_id,
    platform_message_id, platform_parent_message_id, platform_thread_id,
    sender_channel_identity_id, metadata_json, created_at, message_id
)
SELECT
    id, channel_id, channel_type, direction, thread_id, run_id, platform_conversation_id,
    platform_message_id, platform_parent_message_id, platform_thread_id,
    sender_channel_identity_id, metadata_json, created_at, message_id
FROM channel_message_ledger_old_00069;

DROP TABLE messages_old_00069;
DROP TABLE channel_message_ledger_old_00069;

CREATE INDEX ix_messages_thread_id ON messages(thread_id);
CREATE INDEX ix_messages_org_id_thread_id_created_at ON messages(account_id, thread_id, created_at);
CREATE INDEX ix_messages_account_id_thread_id_thread_seq ON messages(account_id, thread_id, thread_seq);
CREATE INDEX ix_messages_thread_id_thread_seq ON messages(thread_id, thread_seq);
CREATE UNIQUE INDEX uq_messages_thread_id_thread_seq ON messages(thread_id, thread_seq);
CREATE INDEX idx_channel_message_ledger_channel_id ON channel_message_ledger(channel_id);
CREATE INDEX idx_channel_message_ledger_thread_id ON channel_message_ledger(thread_id);
CREATE INDEX idx_channel_message_ledger_run_id ON channel_message_ledger(run_id);
CREATE INDEX idx_channel_message_ledger_sender_identity_id ON channel_message_ledger(sender_channel_identity_id);
CREATE INDEX idx_channel_message_ledger_message_id ON channel_message_ledger(message_id);

DROP INDEX IF EXISTS ix_thread_compaction_snapshots_thread_created_at;
DROP INDEX IF EXISTS uq_thread_compaction_snapshots_active_thread;
DROP TABLE IF EXISTS thread_compaction_snapshots;

PRAGMA foreign_keys = ON;

-- +goose Down

PRAGMA foreign_keys = OFF;

ALTER TABLE messages RENAME TO messages_old_00069_down;

CREATE TABLE messages (
    id                 TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    thread_id          TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    account_id         TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    thread_seq         INTEGER NOT NULL,
    created_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    role               TEXT NOT NULL,
    content            TEXT NOT NULL,
    content_json       TEXT,
    metadata_json      TEXT NOT NULL DEFAULT '{}',
    hidden             INTEGER NOT NULL DEFAULT 0,
    deleted_at         TEXT,
    token_count        INTEGER,
    created_at         TEXT NOT NULL DEFAULT (datetime('now')),
    compacted          INTEGER NOT NULL DEFAULT 0
);

INSERT INTO messages (
    id, thread_id, account_id, thread_seq, created_by_user_id, role, content, content_json,
    metadata_json, hidden, deleted_at, token_count, created_at, compacted
)
SELECT
    id, thread_id, account_id, thread_seq, created_by_user_id, role, content, content_json,
    metadata_json, hidden, deleted_at, token_count, created_at, 0
FROM messages_old_00069_down;

ALTER TABLE channel_message_ledger RENAME TO channel_message_ledger_old_00069_down;

CREATE TABLE channel_message_ledger (
    id                         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    channel_id                 TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    channel_type               TEXT NOT NULL,
    direction                  TEXT NOT NULL,
    thread_id                  TEXT REFERENCES threads(id) ON DELETE SET NULL,
    run_id                     TEXT REFERENCES runs(id) ON DELETE SET NULL,
    platform_conversation_id   TEXT NOT NULL,
    platform_message_id        TEXT NOT NULL,
    platform_parent_message_id TEXT,
    platform_thread_id         TEXT,
    sender_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
    metadata_json              TEXT NOT NULL DEFAULT '{}',
    created_at                 TEXT NOT NULL DEFAULT (datetime('now')),
    message_id                 TEXT REFERENCES messages(id) ON DELETE SET NULL,
    CHECK (direction IN ('inbound', 'outbound')),
    UNIQUE (channel_id, direction, platform_conversation_id, platform_message_id)
);

INSERT INTO channel_message_ledger (
    id, channel_id, channel_type, direction, thread_id, run_id, platform_conversation_id,
    platform_message_id, platform_parent_message_id, platform_thread_id,
    sender_channel_identity_id, metadata_json, created_at, message_id
)
SELECT
    id, channel_id, channel_type, direction, thread_id, run_id, platform_conversation_id,
    platform_message_id, platform_parent_message_id, platform_thread_id,
    sender_channel_identity_id, metadata_json, created_at, message_id
FROM channel_message_ledger_old_00069_down;

DROP TABLE messages_old_00069_down;
DROP TABLE channel_message_ledger_old_00069_down;

CREATE INDEX ix_messages_thread_id ON messages(thread_id);
CREATE INDEX ix_messages_org_id_thread_id_created_at ON messages(account_id, thread_id, created_at);
CREATE INDEX ix_messages_account_id_thread_id_thread_seq ON messages(account_id, thread_id, thread_seq);
CREATE INDEX ix_messages_thread_id_thread_seq ON messages(thread_id, thread_seq);
CREATE UNIQUE INDEX uq_messages_thread_id_thread_seq ON messages(thread_id, thread_seq);
CREATE INDEX idx_channel_message_ledger_channel_id ON channel_message_ledger(channel_id);
CREATE INDEX idx_channel_message_ledger_thread_id ON channel_message_ledger(thread_id);
CREATE INDEX idx_channel_message_ledger_run_id ON channel_message_ledger(run_id);
CREATE INDEX idx_channel_message_ledger_sender_identity_id ON channel_message_ledger(sender_channel_identity_id);
CREATE INDEX idx_channel_message_ledger_message_id ON channel_message_ledger(message_id);
CREATE INDEX ix_messages_thread_compacted
    ON messages (thread_id, compacted)
    WHERE deleted_at IS NULL AND compacted = 1;

CREATE TABLE thread_compaction_snapshots (
    id                    TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id            TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    thread_id             TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    summary_text          TEXT NOT NULL,
    metadata_json         TEXT NOT NULL DEFAULT '{}',
    supersedes_snapshot_id TEXT REFERENCES thread_compaction_snapshots(id) ON DELETE SET NULL,
    is_active             INTEGER NOT NULL DEFAULT 1,
    created_at            TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX uq_thread_compaction_snapshots_active_thread
    ON thread_compaction_snapshots(thread_id)
    WHERE is_active = 1;

CREATE INDEX ix_thread_compaction_snapshots_thread_created_at
    ON thread_compaction_snapshots(thread_id, created_at DESC);

PRAGMA foreign_keys = ON;
