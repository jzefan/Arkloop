-- +goose Up

ALTER TABLE llm_routes
    ADD COLUMN IF NOT EXISTS show_in_picker BOOLEAN NOT NULL DEFAULT TRUE;

-- +goose Down

ALTER TABLE llm_routes
    DROP COLUMN IF EXISTS show_in_picker;
