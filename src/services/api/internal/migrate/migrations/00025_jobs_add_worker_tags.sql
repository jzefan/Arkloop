-- +goose Up
ALTER TABLE jobs ADD COLUMN worker_tags TEXT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE jobs DROP COLUMN IF EXISTS worker_tags;
