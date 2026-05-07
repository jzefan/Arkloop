-- +goose Up
CREATE TABLE skills (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID        REFERENCES orgs(id),
    skill_key      TEXT        NOT NULL,
    version        TEXT        NOT NULL,
    display_name   TEXT        NOT NULL,
    description    TEXT,
    prompt_md      TEXT        NOT NULL,
    tool_allowlist TEXT[]      NOT NULL DEFAULT '{}',
    budgets_json   JSONB       NOT NULL DEFAULT '{}',
    is_active      BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_skills_org_key_version UNIQUE NULLS NOT DISTINCT (org_id, skill_key, version),
    CONSTRAINT chk_skills_key_format     CHECK (skill_key ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$'),
    CONSTRAINT chk_skills_version_format CHECK (version ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$')
);

-- +goose Down
DROP TABLE IF EXISTS skills;
