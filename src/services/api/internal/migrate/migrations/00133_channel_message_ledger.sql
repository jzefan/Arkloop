-- +goose Up

CREATE TABLE channel_message_ledger (
    id                          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id                  UUID        NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    channel_type                TEXT        NOT NULL,
    direction                   TEXT        NOT NULL,
    thread_id                   UUID        REFERENCES threads(id) ON DELETE SET NULL,
    run_id                      UUID        REFERENCES runs(id) ON DELETE SET NULL,
    platform_conversation_id    TEXT        NOT NULL,
    platform_message_id         TEXT        NOT NULL,
    platform_parent_message_id  TEXT,
    platform_thread_id          TEXT,
    sender_channel_identity_id  UUID        REFERENCES channel_identities(id) ON DELETE SET NULL,
    metadata_json               JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_channel_message_ledger_direction
        CHECK (direction IN ('inbound', 'outbound')),
    CONSTRAINT uq_channel_message_ledger_entry
        UNIQUE (channel_id, direction, platform_conversation_id, platform_message_id)
);

CREATE INDEX ix_channel_message_ledger_channel_id ON channel_message_ledger(channel_id);
CREATE INDEX ix_channel_message_ledger_thread_id ON channel_message_ledger(thread_id);
CREATE INDEX ix_channel_message_ledger_run_id ON channel_message_ledger(run_id);
CREATE INDEX ix_channel_message_ledger_sender_identity_id ON channel_message_ledger(sender_channel_identity_id);

-- +goose Down

DROP INDEX IF EXISTS ix_channel_message_ledger_sender_identity_id;
DROP INDEX IF EXISTS ix_channel_message_ledger_run_id;
DROP INDEX IF EXISTS ix_channel_message_ledger_thread_id;
DROP INDEX IF EXISTS ix_channel_message_ledger_channel_id;
DROP TABLE IF EXISTS channel_message_ledger;
