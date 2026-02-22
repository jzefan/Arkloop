-- +goose Up
CREATE TABLE worker_registrations (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    worker_id       UUID        NOT NULL UNIQUE,
    hostname        TEXT        NOT NULL,
    version         TEXT        NOT NULL DEFAULT 'unknown',
    status          TEXT        NOT NULL DEFAULT 'active',
    capabilities    JSONB       NOT NULL DEFAULT '[]',
    current_load    INT         NOT NULL DEFAULT 0,
    max_concurrency INT         NOT NULL DEFAULT 4,
    heartbeat_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    registered_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_worker_status CHECK (status IN ('active', 'draining', 'dead'))
);

CREATE INDEX idx_worker_registrations_status    ON worker_registrations (status);
CREATE INDEX idx_worker_registrations_heartbeat ON worker_registrations (heartbeat_at);

-- +goose Down
DROP TABLE IF EXISTS worker_registrations;
