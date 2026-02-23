-- +goose Up

CREATE TABLE notifications (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id       UUID        NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    type         TEXT        NOT NULL,
    title        TEXT        NOT NULL,
    body         TEXT        NOT NULL DEFAULT '',
    payload_json JSONB       NOT NULL DEFAULT '{}'::jsonb,
    read_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_notifications_user_id_unread ON notifications(user_id) WHERE read_at IS NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_notifications_user_id_unread;
DROP TABLE IF EXISTS notifications;
