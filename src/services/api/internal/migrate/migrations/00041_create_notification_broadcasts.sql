-- +goose Up

CREATE TABLE notification_broadcasts (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    type            TEXT        NOT NULL,
    title           TEXT        NOT NULL,
    body            TEXT        NOT NULL DEFAULT '',
    target_type     TEXT        NOT NULL DEFAULT 'all',
    target_id       UUID,
    payload_json    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    status          TEXT        NOT NULL DEFAULT 'pending',
    sent_count      INT         NOT NULL DEFAULT 0,
    created_by      UUID        NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_notification_broadcasts_created_at ON notification_broadcasts(created_at DESC);

ALTER TABLE notifications ADD COLUMN broadcast_id UUID REFERENCES notification_broadcasts(id);
CREATE INDEX idx_notifications_broadcast_id ON notifications(broadcast_id) WHERE broadcast_id IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_notifications_broadcast_id;
ALTER TABLE notifications DROP COLUMN IF EXISTS broadcast_id;
DROP INDEX IF EXISTS idx_notification_broadcasts_created_at;
DROP TABLE IF EXISTS notification_broadcasts;
