-- +goose Up
ALTER TABLE threads ADD COLUMN plan_mode INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE threads DROP COLUMN plan_mode;
