-- +goose Up
ALTER TABLE messages ADD COLUMN compacted INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS ix_messages_thread_compacted
    ON messages (thread_id, compacted)
    WHERE deleted_at IS NULL AND compacted = 1;

-- +goose Down
DROP INDEX IF EXISTS ix_messages_thread_compacted;
