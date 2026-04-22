-- +goose Up
ALTER TABLE runs ADD COLUMN updated_at TEXT;

-- +goose Down
-- SQLite does not support DROP COLUMN; intentionally empty.
