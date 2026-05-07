-- +goose Up

WITH ranked AS (
    SELECT session_ref,
           row_number() OVER (
               PARTITION BY org_id, profile_ref, session_type, default_binding_key
               ORDER BY updated_at DESC, created_at DESC, session_ref DESC
           ) AS rank_order
      FROM shell_sessions
     WHERE default_binding_key IS NOT NULL
       AND state <> 'closed'
)
UPDATE shell_sessions AS sessions
   SET default_binding_key = NULL,
       updated_at = now(),
       last_used_at = now()
  FROM ranked
 WHERE sessions.session_ref = ranked.session_ref
   AND ranked.rank_order > 1;

DROP INDEX IF EXISTS idx_shell_sessions_org_profile_binding_type_updated;

CREATE UNIQUE INDEX IF NOT EXISTS idx_shell_sessions_org_profile_binding_type_unique
    ON shell_sessions (org_id, profile_ref, session_type, default_binding_key)
    WHERE default_binding_key IS NOT NULL
      AND state <> 'closed';

-- +goose Down

DROP INDEX IF EXISTS idx_shell_sessions_org_profile_binding_type_unique;

CREATE INDEX IF NOT EXISTS idx_shell_sessions_org_profile_binding_type_updated
    ON shell_sessions (org_id, profile_ref, session_type, default_binding_key, updated_at DESC)
    WHERE default_binding_key IS NOT NULL;
