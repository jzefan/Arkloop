-- +goose Up
UPDATE tool_provider_configs
SET group_name = 'read'
WHERE group_name = 'image_understanding';

UPDATE tool_provider_configs
SET provider_name = 'read.minimax'
WHERE provider_name = 'image_understanding.minimax';

-- personas.conditional_tools_json is created in migration 00153. On a fresh
-- database 146 runs before 153 so the column is not yet present; in that case
-- there is no data to backfill and we skip the UPDATE safely.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
          FROM information_schema.columns
         WHERE table_name = 'personas'
           AND column_name = 'conditional_tools_json'
    ) THEN
        UPDATE personas
        SET conditional_tools_json = REPLACE(conditional_tools_json::text, '"understand_image"', '"read"')::jsonb
        WHERE conditional_tools_json IS NOT NULL
          AND conditional_tools_json::text LIKE '%"understand_image"%';
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
          FROM information_schema.columns
         WHERE table_name = 'personas'
           AND column_name = 'conditional_tools_json'
    ) THEN
        UPDATE personas
        SET conditional_tools_json = REPLACE(conditional_tools_json::text, '"read"', '"understand_image"')::jsonb
        WHERE conditional_tools_json IS NOT NULL
          AND conditional_tools_json::text LIKE '%"read"%';
    END IF;
END $$;
-- +goose StatementEnd

UPDATE tool_provider_configs
SET provider_name = 'image_understanding.minimax'
WHERE provider_name = 'read.minimax';

UPDATE tool_provider_configs
SET group_name = 'image_understanding'
WHERE group_name = 'read';
