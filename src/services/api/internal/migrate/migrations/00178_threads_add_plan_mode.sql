-- +goose Up
ALTER TABLE threads ADD COLUMN plan_mode BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE threads DROP COLUMN IF EXISTS plan_mode;
