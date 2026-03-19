-- +goose Up

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
    CHECK (direction IN ('inbound', 'outbound')),
    UNIQUE (channel_id, direction, platform_conversation_id, platform_message_id)
);

CREATE INDEX idx_channel_message_ledger_channel_id ON channel_message_ledger(channel_id);
CREATE INDEX idx_channel_message_ledger_thread_id ON channel_message_ledger(thread_id);
CREATE INDEX idx_channel_message_ledger_run_id ON channel_message_ledger(run_id);
CREATE INDEX idx_channel_message_ledger_sender_identity_id ON channel_message_ledger(sender_channel_identity_id);

-- +goose Down

DROP INDEX IF EXISTS idx_channel_message_ledger_sender_identity_id;
DROP INDEX IF EXISTS idx_channel_message_ledger_run_id;
DROP INDEX IF EXISTS idx_channel_message_ledger_thread_id;
DROP INDEX IF EXISTS idx_channel_message_ledger_channel_id;
DROP TABLE IF EXISTS channel_message_ledger;
