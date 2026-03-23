-- +goose Up
DROP TABLE IF EXISTS scheduled_triggers;

CREATE TABLE scheduled_triggers (
    id                    TEXT PRIMARY KEY,
    channel_identity_id   TEXT NOT NULL UNIQUE,
    persona_key           TEXT NOT NULL,
    account_id            TEXT NOT NULL,
    model                 TEXT NOT NULL DEFAULT '',
    interval_min          INTEGER NOT NULL DEFAULT 30,
    next_fire_at          TEXT NOT NULL,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS scheduled_triggers_next_fire_at_idx
    ON scheduled_triggers (next_fire_at);

-- +goose Down
DROP TABLE IF EXISTS scheduled_triggers;
