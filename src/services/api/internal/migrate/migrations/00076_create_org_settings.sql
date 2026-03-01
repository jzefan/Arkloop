-- +goose Up

CREATE TABLE org_settings (
    org_id     uuid        NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    key        text        NOT NULL,
    value      text        NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, key)
);

CREATE INDEX ix_org_settings_key ON org_settings(key);

-- +goose Down

DROP INDEX IF EXISTS ix_org_settings_key;
DROP TABLE IF EXISTS org_settings;

