-- +goose Up

-- OIDC refresh tokens, distinct from the existing /v1/auth refresh_tokens
-- table (which serves the first-party web session). These are scoped per
-- (user, client) and support rotation-on-use: rotated_to points at the
-- successor's hash so we can detect replay of a revoked token and revoke
-- the entire chain (assume the token was stolen).
CREATE TABLE oauth_refresh_tokens (
    token_hash      TEXT        PRIMARY KEY,
    client_id       TEXT        NOT NULL REFERENCES oauth_clients(client_id) ON DELETE CASCADE,
    user_id         UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scopes          TEXT[]      NOT NULL,
    issued_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL,
    rotated_to      TEXT        REFERENCES oauth_refresh_tokens(token_hash) ON DELETE SET NULL,
    revoked_at      TIMESTAMPTZ
);

CREATE INDEX ix_oauth_refresh_tokens_active
    ON oauth_refresh_tokens(user_id, client_id)
    WHERE revoked_at IS NULL;

CREATE INDEX ix_oauth_refresh_tokens_expires_at
    ON oauth_refresh_tokens(expires_at)
    WHERE revoked_at IS NULL;

-- +goose Down

DROP INDEX IF EXISTS ix_oauth_refresh_tokens_expires_at;
DROP INDEX IF EXISTS ix_oauth_refresh_tokens_active;
DROP TABLE IF EXISTS oauth_refresh_tokens;
