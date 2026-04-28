-- +goose Up
ALTER TABLE threads ADD COLUMN collaboration_mode TEXT NOT NULL DEFAULT 'default';
ALTER TABLE threads ADD COLUMN collaboration_mode_revision BIGINT NOT NULL DEFAULT 0;
ALTER TABLE threads ADD CONSTRAINT threads_collaboration_mode_check CHECK (collaboration_mode IN ('default', 'plan'));

-- +goose Down
ALTER TABLE threads DROP CONSTRAINT IF EXISTS threads_collaboration_mode_check;
ALTER TABLE threads DROP COLUMN IF EXISTS collaboration_mode_revision;
ALTER TABLE threads DROP COLUMN IF EXISTS collaboration_mode;
