-- +goose Up

ALTER TABLE notification_broadcasts ADD COLUMN deleted_at TIMESTAMPTZ;
CREATE INDEX idx_notification_broadcasts_active ON notification_broadcasts(created_at DESC) WHERE deleted_at IS NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_notification_broadcasts_active;
ALTER TABLE notification_broadcasts DROP COLUMN IF EXISTS deleted_at;
