-- +goose Up
ALTER TABLE scheduled_jobs DROP COLUMN light_context;
ALTER TABLE scheduled_jobs DROP COLUMN tools_allow;

-- +goose Down
ALTER TABLE scheduled_jobs ADD COLUMN light_context INTEGER NOT NULL DEFAULT 0;
ALTER TABLE scheduled_jobs ADD COLUMN tools_allow TEXT NOT NULL DEFAULT '';
