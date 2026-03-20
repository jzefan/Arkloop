-- Align audit_logs with PostgreSQL schema and shared DBSink / AuditLogRepository.
-- Legacy desktop table (00016) used user_id, resource_type, resource_id, details, created_at.

-- +goose Up

ALTER TABLE audit_logs ADD COLUMN trace_id TEXT NOT NULL DEFAULT '';
ALTER TABLE audit_logs ADD COLUMN user_agent TEXT;
ALTER TABLE audit_logs ADD COLUMN api_key_id TEXT;
ALTER TABLE audit_logs ADD COLUMN before_state_json TEXT;
ALTER TABLE audit_logs ADD COLUMN after_state_json TEXT;

ALTER TABLE audit_logs RENAME COLUMN user_id TO actor_user_id;
ALTER TABLE audit_logs RENAME COLUMN resource_type TO target_type;
ALTER TABLE audit_logs RENAME COLUMN resource_id TO target_id;
ALTER TABLE audit_logs RENAME COLUMN details TO metadata_json;
ALTER TABLE audit_logs RENAME COLUMN created_at TO ts;

UPDATE audit_logs SET metadata_json = '{}'
WHERE metadata_json IS NULL OR trim(metadata_json) = '';

CREATE INDEX IF NOT EXISTS ix_audit_logs_trace_id ON audit_logs(trace_id);
CREATE INDEX IF NOT EXISTS ix_audit_logs_account_id_ts ON audit_logs(account_id, ts);

-- +goose Down

DROP INDEX IF EXISTS ix_audit_logs_account_id_ts;
DROP INDEX IF EXISTS ix_audit_logs_trace_id;

ALTER TABLE audit_logs RENAME COLUMN ts TO created_at;
ALTER TABLE audit_logs RENAME COLUMN metadata_json TO details;
ALTER TABLE audit_logs RENAME COLUMN target_id TO resource_id;
ALTER TABLE audit_logs RENAME COLUMN target_type TO resource_type;
ALTER TABLE audit_logs RENAME COLUMN actor_user_id TO user_id;

ALTER TABLE audit_logs DROP COLUMN after_state_json;
ALTER TABLE audit_logs DROP COLUMN before_state_json;
ALTER TABLE audit_logs DROP COLUMN api_key_id;
ALTER TABLE audit_logs DROP COLUMN user_agent;
ALTER TABLE audit_logs DROP COLUMN trace_id;
