-- +goose Up

CREATE TABLE profile_registries (
    profile_ref             TEXT        PRIMARY KEY,
    org_id                  UUID        NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    latest_manifest_rev     TEXT        NULL,
    flush_state             TEXT        NOT NULL DEFAULT 'idle' CHECK (flush_state IN ('idle', 'pending', 'running', 'failed')),
    flush_retry_count       INTEGER     NOT NULL DEFAULT 0,
    last_flush_failed_at    TIMESTAMPTZ NULL,
    last_flush_succeeded_at TIMESTAMPTZ NULL,
    metadata_json           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_profile_registries_org_id
    ON profile_registries (org_id);

CREATE TABLE workspace_registries (
    workspace_ref           TEXT        PRIMARY KEY,
    org_id                  UUID        NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    latest_manifest_rev     TEXT        NULL,
    flush_state             TEXT        NOT NULL DEFAULT 'idle' CHECK (flush_state IN ('idle', 'pending', 'running', 'failed')),
    flush_retry_count       INTEGER     NOT NULL DEFAULT 0,
    last_flush_failed_at    TIMESTAMPTZ NULL,
    last_flush_succeeded_at TIMESTAMPTZ NULL,
    metadata_json           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_workspace_registries_org_id
    ON workspace_registries (org_id);

ALTER TABLE shell_sessions
    ADD COLUMN IF NOT EXISTS latest_restore_rev TEXT NULL;

-- +goose Down

ALTER TABLE shell_sessions
    DROP COLUMN IF EXISTS latest_restore_rev;

DROP INDEX IF EXISTS idx_workspace_registries_org_id;
DROP TABLE IF EXISTS workspace_registries;

DROP INDEX IF EXISTS idx_profile_registries_org_id;
DROP TABLE IF EXISTS profile_registries;
