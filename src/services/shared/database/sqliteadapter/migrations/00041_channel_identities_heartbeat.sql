-- +goose Up
ALTER TABLE channel_identities ADD COLUMN heartbeat_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE channel_identities ADD COLUMN heartbeat_interval_minutes INTEGER NOT NULL DEFAULT 30;
ALTER TABLE channel_identities ADD COLUMN heartbeat_model TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE channel_identities DROP COLUMN heartbeat_enabled;
ALTER TABLE channel_identities DROP COLUMN heartbeat_interval_minutes;
ALTER TABLE channel_identities DROP COLUMN heartbeat_model;
