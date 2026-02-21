-- +goose Up
ALTER TABLE audit_logs
    ADD COLUMN ip_address        INET,
    ADD COLUMN user_agent        TEXT,
    ADD COLUMN api_key_id        UUID,
    ADD COLUMN before_state_json JSONB,
    ADD COLUMN after_state_json  JSONB;

-- +goose Down
ALTER TABLE audit_logs
    DROP COLUMN IF EXISTS after_state_json,
    DROP COLUMN IF EXISTS before_state_json,
    DROP COLUMN IF EXISTS api_key_id,
    DROP COLUMN IF EXISTS user_agent,
    DROP COLUMN IF EXISTS ip_address;
