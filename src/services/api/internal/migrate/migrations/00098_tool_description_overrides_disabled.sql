-- +goose Up
ALTER TABLE tool_description_overrides
    ADD COLUMN IF NOT EXISTS is_disabled boolean NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE tool_description_overrides
    DROP COLUMN IF EXISTS is_disabled;
