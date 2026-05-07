-- +goose Up
ALTER TABLE threads ADD COLUMN collaboration_mode TEXT NOT NULL DEFAULT 'default' CHECK (collaboration_mode IN ('default', 'plan'));
ALTER TABLE threads ADD COLUMN collaboration_mode_revision INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE threads DROP COLUMN collaboration_mode_revision;
ALTER TABLE threads DROP COLUMN collaboration_mode;
