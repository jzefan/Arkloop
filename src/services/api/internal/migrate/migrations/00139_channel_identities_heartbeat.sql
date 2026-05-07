-- +goose Up
ALTER TABLE channel_identities
    ADD COLUMN IF NOT EXISTS heartbeat_enabled INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS heartbeat_interval_minutes INTEGER NOT NULL DEFAULT 30,
    ADD COLUMN IF NOT EXISTS heartbeat_model TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE channel_identities
    DROP COLUMN IF EXISTS heartbeat_enabled,
    DROP COLUMN IF EXISTS heartbeat_interval_minutes,
    DROP COLUMN IF EXISTS heartbeat_model;
