-- +goose Up
ALTER TABLE messages
    ADD COLUMN content_json   JSONB,
    ADD COLUMN metadata_json  JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN deleted_at     TIMESTAMP WITH TIME ZONE,
    ADD COLUMN token_count    INTEGER;

-- +goose Down
ALTER TABLE messages
    DROP COLUMN IF EXISTS token_count,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS metadata_json,
    DROP COLUMN IF EXISTS content_json;
