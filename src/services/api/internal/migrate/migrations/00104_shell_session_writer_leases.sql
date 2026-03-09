-- +goose Up

ALTER TABLE shell_sessions
    ADD COLUMN IF NOT EXISTS lease_owner_id TEXT NULL,
    ADD COLUMN IF NOT EXISTS lease_until TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS lease_epoch BIGINT NOT NULL DEFAULT 0;

ALTER TABLE shell_sessions
    DROP CONSTRAINT IF EXISTS shell_sessions_lease_consistency;

ALTER TABLE shell_sessions
    ADD CONSTRAINT shell_sessions_lease_consistency CHECK (
        (lease_owner_id IS NULL AND lease_until IS NULL)
        OR (lease_owner_id IS NOT NULL AND lease_until IS NOT NULL)
    );

CREATE INDEX IF NOT EXISTS idx_shell_sessions_org_lease_until
    ON shell_sessions (org_id, lease_until)
    WHERE lease_until IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_shell_sessions_org_lease_until;

ALTER TABLE shell_sessions
    DROP CONSTRAINT IF EXISTS shell_sessions_lease_consistency;

ALTER TABLE shell_sessions
    DROP COLUMN IF EXISTS lease_epoch,
    DROP COLUMN IF EXISTS lease_until,
    DROP COLUMN IF EXISTS lease_owner_id;
