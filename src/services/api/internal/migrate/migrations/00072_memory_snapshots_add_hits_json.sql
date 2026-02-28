-- +goose Up
ALTER TABLE user_memory_snapshots ADD COLUMN hits_json JSONB;

-- +goose Down
ALTER TABLE user_memory_snapshots DROP COLUMN IF EXISTS hits_json;
