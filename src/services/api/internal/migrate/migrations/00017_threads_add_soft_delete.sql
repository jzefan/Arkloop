-- +goose Up
ALTER TABLE threads
    ADD COLUMN deleted_at  TIMESTAMP WITH TIME ZONE,
    ADD COLUMN project_id  UUID;

-- 仅索引已删除的行，未删除的不占用索引空间
CREATE INDEX ix_threads_deleted_at ON threads(deleted_at) WHERE deleted_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS ix_threads_deleted_at;
ALTER TABLE threads
    DROP COLUMN IF EXISTS project_id,
    DROP COLUMN IF EXISTS deleted_at;
