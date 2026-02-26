-- +goose Up
ALTER TABLE threads
    ADD COLUMN is_private BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN expires_at TIMESTAMPTZ;

CREATE INDEX ix_threads_private_expires ON threads(expires_at) WHERE is_private = TRUE;

-- +goose Down
DROP INDEX IF EXISTS ix_threads_private_expires;

ALTER TABLE threads
    DROP COLUMN expires_at,
    DROP COLUMN is_private;
