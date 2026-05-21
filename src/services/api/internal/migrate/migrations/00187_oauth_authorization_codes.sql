-- +goose Up

-- One-shot authorization codes for the OIDC Authorization Code flow.
-- The plaintext code is never stored; only its SHA256 hash is the PK.
-- consumed_at acts as the single-use marker (idempotent token exchange).
CREATE TABLE oauth_authorization_codes (
    code_hash             TEXT        PRIMARY KEY,
    client_id             TEXT        NOT NULL REFERENCES oauth_clients(client_id) ON DELETE CASCADE,
    user_id               UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    redirect_uri          TEXT        NOT NULL,
    scopes                TEXT[]      NOT NULL,
    code_challenge        TEXT        NOT NULL,
    code_challenge_method TEXT        NOT NULL DEFAULT 'S256',
    nonce                 TEXT,
    expires_at            TIMESTAMPTZ NOT NULL,
    consumed_at           TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT oauth_authorization_codes_method_check
        CHECK (code_challenge_method IN ('S256'))
);

CREATE INDEX ix_oauth_authorization_codes_expires_at
    ON oauth_authorization_codes(expires_at);

CREATE INDEX ix_oauth_authorization_codes_user_id
    ON oauth_authorization_codes(user_id);

-- +goose Down

DROP INDEX IF EXISTS ix_oauth_authorization_codes_user_id;
DROP INDEX IF EXISTS ix_oauth_authorization_codes_expires_at;
DROP TABLE IF EXISTS oauth_authorization_codes;
