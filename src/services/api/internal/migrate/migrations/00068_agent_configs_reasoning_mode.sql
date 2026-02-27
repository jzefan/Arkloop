-- +goose Up

ALTER TABLE agent_configs
    ADD COLUMN reasoning_mode TEXT NOT NULL DEFAULT 'auto';

-- +goose Down

ALTER TABLE agent_configs
    DROP COLUMN IF EXISTS reasoning_mode;
