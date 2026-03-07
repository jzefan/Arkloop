-- +goose Up
TRUNCATE TABLE tool_description_overrides;

-- +goose Down
SELECT 1;
