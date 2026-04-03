-- +goose Up

ALTER TABLE personas ADD COLUMN heartbeat_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE personas ADD COLUMN heartbeat_interval_minutes INTEGER NOT NULL DEFAULT 30;

-- +goose Down

-- SQLite: leave columns in place.
