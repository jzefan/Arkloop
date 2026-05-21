-- +goose Up

-- Records the union of scopes a user has consented to for a given client.
-- A second authorize call is silent as long as requested scopes ⊆ scopes here.
-- Requesting a wider scope re-prompts the consent screen.
CREATE TABLE oauth_consents (
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id   TEXT        NOT NULL REFERENCES oauth_clients(client_id) ON DELETE CASCADE,
    scopes      TEXT[]      NOT NULL,
    granted_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at  TIMESTAMPTZ,
    PRIMARY KEY (user_id, client_id)
);

CREATE INDEX ix_oauth_consents_client_id
    ON oauth_consents(client_id)
    WHERE revoked_at IS NULL;

-- +goose Down

DROP INDEX IF EXISTS ix_oauth_consents_client_id;
DROP TABLE IF EXISTS oauth_consents;
