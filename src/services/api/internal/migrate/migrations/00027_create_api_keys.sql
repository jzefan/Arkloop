-- +goose Up
CREATE TABLE api_keys (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID        NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    user_id      UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    key_prefix   TEXT        NOT NULL,
    key_hash     TEXT        NOT NULL UNIQUE,
    scopes       TEXT[]      NOT NULL DEFAULT '{}',
    revoked_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_api_keys_org_id ON api_keys (org_id);
CREATE INDEX idx_api_keys_key_hash ON api_keys (key_hash);

-- +goose Down
DROP TABLE IF EXISTS api_keys;
