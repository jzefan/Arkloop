-- +goose Up
CREATE TABLE mcp_configs (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id             UUID        NOT NULL REFERENCES orgs(id),
    name               TEXT        NOT NULL,
    transport          TEXT        NOT NULL,
    url                TEXT,
    auth_secret_id     UUID        REFERENCES secrets(id),
    command            TEXT,
    args_json          JSONB       NOT NULL DEFAULT '[]',
    cwd                TEXT,
    env_json           JSONB       NOT NULL DEFAULT '{}',
    inherit_parent_env BOOLEAN     NOT NULL DEFAULT FALSE,
    call_timeout_ms    INT         NOT NULL DEFAULT 10000,
    is_active          BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_mcp_configs_org_name    UNIQUE (org_id, name),
    CONSTRAINT chk_mcp_configs_transport  CHECK (transport IN ('stdio', 'http_sse', 'streamable_http')),
    CONSTRAINT chk_mcp_configs_timeout    CHECK (call_timeout_ms > 0),
    CONSTRAINT chk_mcp_configs_stdio_cmd  CHECK (transport != 'stdio' OR command IS NOT NULL),
    CONSTRAINT chk_mcp_configs_remote_url CHECK (transport = 'stdio' OR url IS NOT NULL)
);

-- +goose Down
DROP TABLE IF EXISTS mcp_configs;
