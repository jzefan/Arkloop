-- +goose Up
DROP INDEX IF EXISTS idx_thread_shares_thread_id;
ALTER TABLE thread_shares ADD COLUMN live_update BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE thread_shares ADD COLUMN snapshot_turn_count INT NOT NULL DEFAULT 0;
CREATE INDEX idx_thread_shares_thread_id ON thread_shares(thread_id);

-- +goose Down
DROP INDEX IF EXISTS idx_thread_shares_thread_id;
ALTER TABLE thread_shares DROP COLUMN IF EXISTS snapshot_turn_count;
ALTER TABLE thread_shares DROP COLUMN IF EXISTS live_update;
CREATE UNIQUE INDEX idx_thread_shares_thread_id ON thread_shares(thread_id);
