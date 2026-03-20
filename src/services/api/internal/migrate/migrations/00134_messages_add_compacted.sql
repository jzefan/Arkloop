-- +goose Up
ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS compacted BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS ix_messages_thread_compacted
    ON messages (thread_id, compacted)
    WHERE deleted_at IS NULL AND compacted = TRUE;

-- +goose Down
DROP INDEX IF EXISTS ix_messages_thread_compacted;
ALTER TABLE messages DROP COLUMN IF EXISTS compacted;
