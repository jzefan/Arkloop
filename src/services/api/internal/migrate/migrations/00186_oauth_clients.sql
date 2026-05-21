-- +goose Up

CREATE TABLE oauth_clients (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id           TEXT        NOT NULL UNIQUE,
    client_secret_hash  TEXT        NOT NULL,
    client_type         TEXT        NOT NULL DEFAULT 'confidential',
    name                TEXT        NOT NULL,
    redirect_uris       TEXT[]      NOT NULL,
    allowed_scopes      TEXT[]      NOT NULL,
    require_pkce        BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ,
    CONSTRAINT oauth_clients_client_type_check CHECK (client_type IN ('confidential', 'public'))
);

CREATE UNIQUE INDEX ix_oauth_clients_client_id_active
    ON oauth_clients(client_id)
    WHERE deleted_at IS NULL;

-- +goose Down

DROP INDEX IF EXISTS ix_oauth_clients_client_id_active;
DROP TABLE IF EXISTS oauth_clients;
