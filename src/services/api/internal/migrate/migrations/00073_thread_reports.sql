-- +goose Up
CREATE TABLE thread_reports (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id   UUID        NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    reporter_id UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    categories  TEXT[]      NOT NULL,
    feedback    TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_thread_reports_thread_id ON thread_reports(thread_id);
CREATE INDEX idx_thread_reports_created_at ON thread_reports(created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS thread_reports;
