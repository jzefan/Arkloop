-- +goose Up
ALTER TABLE threads ADD COLUMN sidebar_work_folder TEXT NULL;
ALTER TABLE threads ADD COLUMN sidebar_pinned_at TEXT NULL;
ALTER TABLE threads ADD COLUMN sidebar_gtd_bucket TEXT NULL CHECK (sidebar_gtd_bucket IS NULL OR sidebar_gtd_bucket IN ('inbox', 'todo', 'waiting', 'someday', 'archived'));

CREATE INDEX idx_threads_owner_sidebar_pinned
    ON threads (account_id, created_by_user_id, sidebar_pinned_at DESC)
    WHERE deleted_at IS NULL AND sidebar_pinned_at IS NOT NULL;

CREATE INDEX idx_threads_owner_sidebar_gtd
    ON threads (account_id, created_by_user_id, sidebar_gtd_bucket)
    WHERE deleted_at IS NULL AND sidebar_gtd_bucket IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_threads_owner_sidebar_gtd;
DROP INDEX IF EXISTS idx_threads_owner_sidebar_pinned;
ALTER TABLE threads DROP COLUMN sidebar_gtd_bucket;
ALTER TABLE threads DROP COLUMN sidebar_pinned_at;
ALTER TABLE threads DROP COLUMN sidebar_work_folder;
