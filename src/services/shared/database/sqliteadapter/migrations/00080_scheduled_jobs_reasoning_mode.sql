-- +goose Up
ALTER TABLE scheduled_jobs ADD COLUMN reasoning_mode TEXT NOT NULL DEFAULT '';
ALTER TABLE scheduled_jobs DROP COLUMN thinking;

-- +goose Down
ALTER TABLE scheduled_jobs ADD COLUMN thinking INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scheduled_jobs DROP COLUMN reasoning_mode;
