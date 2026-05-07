-- +goose Up
ALTER TABLE skills ADD COLUMN agent_config_name TEXT;

-- +goose Down
ALTER TABLE skills DROP COLUMN IF EXISTS agent_config_name;
