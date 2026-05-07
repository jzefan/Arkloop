-- +goose Up
ALTER TABLE thread_reports ALTER COLUMN thread_id DROP NOT NULL;

-- +goose Down
DELETE FROM thread_reports WHERE thread_id IS NULL;
ALTER TABLE thread_reports ALTER COLUMN thread_id SET NOT NULL;
