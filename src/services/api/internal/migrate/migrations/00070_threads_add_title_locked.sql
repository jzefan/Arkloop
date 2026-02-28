-- +goose Up
ALTER TABLE threads ADD COLUMN title_locked boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE threads DROP COLUMN title_locked;
