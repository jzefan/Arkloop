-- +goose Up
CREATE TABLE IF NOT EXISTS scheduled_triggers (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_identity_id   UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    persona_key           TEXT NOT NULL,
    account_id            UUID NOT NULL,
    model                 TEXT NOT NULL DEFAULT '',
    interval_min          INT NOT NULL DEFAULT 30,
    next_fire_at          TIMESTAMPTZ NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS scheduled_triggers_channel_identity_id_idx
    ON scheduled_triggers (channel_identity_id);

CREATE INDEX IF NOT EXISTS scheduled_triggers_next_fire_at_idx
    ON scheduled_triggers (next_fire_at);

-- +goose Down
DROP TABLE IF EXISTS scheduled_triggers;
