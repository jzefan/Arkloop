-- +goose Up
CREATE TABLE llm_credentials (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID        NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    provider        TEXT        NOT NULL
                                CHECK (provider IN ('openai', 'anthropic', 'gemini', 'deepseek', 'zenmax')),
    name            TEXT        NOT NULL,
    secret_id       UUID        REFERENCES secrets(id) ON DELETE SET NULL,
    key_prefix      TEXT,
    base_url        TEXT,
    openai_api_mode TEXT,
    revoked_at      TIMESTAMPTZ,
    last_used_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_llm_credentials_org_name UNIQUE (org_id, name)
);

CREATE TABLE llm_routes (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID        NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    credential_id UUID        NOT NULL REFERENCES llm_credentials(id) ON DELETE CASCADE,
    model         TEXT        NOT NULL,
    priority      INT         NOT NULL DEFAULT 0,
    is_default    BOOLEAN     NOT NULL DEFAULT false,
    when_json     JSONB       NOT NULL DEFAULT '{}',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_llm_credentials_org_id ON llm_credentials(org_id);
CREATE INDEX ix_llm_routes_org_id ON llm_routes(org_id);
CREATE INDEX ix_llm_routes_credential_id ON llm_routes(credential_id);

-- +goose Down
DROP INDEX IF EXISTS ix_llm_routes_credential_id;
DROP INDEX IF EXISTS ix_llm_routes_org_id;
DROP INDEX IF EXISTS ix_llm_credentials_org_id;
DROP TABLE IF EXISTS llm_routes;
DROP TABLE IF EXISTS llm_credentials;
