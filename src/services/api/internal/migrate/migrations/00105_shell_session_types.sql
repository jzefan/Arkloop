-- +goose Up

ALTER TABLE shell_sessions
    ADD COLUMN IF NOT EXISTS session_type TEXT NOT NULL DEFAULT 'shell';

ALTER TABLE shell_sessions
    DROP CONSTRAINT IF EXISTS shell_sessions_session_type_check;

ALTER TABLE shell_sessions
    ADD CONSTRAINT shell_sessions_session_type_check CHECK (session_type IN ('shell', 'browser'));

CREATE INDEX IF NOT EXISTS idx_shell_sessions_org_run_type
    ON shell_sessions (org_id, run_id, session_type);

CREATE INDEX IF NOT EXISTS idx_shell_sessions_org_profile_binding_type_updated
    ON shell_sessions (org_id, profile_ref, session_type, default_binding_key, updated_at DESC)
    WHERE default_binding_key IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS idx_shell_sessions_org_profile_binding_type_updated;
DROP INDEX IF EXISTS idx_shell_sessions_org_run_type;

ALTER TABLE shell_sessions
    DROP CONSTRAINT IF EXISTS shell_sessions_session_type_check;

ALTER TABLE shell_sessions
    DROP COLUMN IF EXISTS session_type;
