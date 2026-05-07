-- +goose Up
CREATE TABLE ip_rules (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID        NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    type       TEXT        NOT NULL,
    cidr       CIDR        NOT NULL,
    note       TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_ip_rules_type CHECK (type IN ('allowlist', 'blocklist'))
);

CREATE INDEX idx_ip_rules_org_id ON ip_rules (org_id);

-- +goose Down
DROP TABLE IF EXISTS ip_rules;
