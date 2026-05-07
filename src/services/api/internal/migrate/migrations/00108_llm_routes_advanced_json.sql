-- +goose Up

ALTER TABLE llm_routes
    ADD COLUMN advanced_json JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down

ALTER TABLE llm_routes
    DROP COLUMN IF EXISTS advanced_json;
