-- +goose Up

CREATE TABLE skill_packages (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           UUID        NOT NULL,
    skill_key        TEXT        NOT NULL,
    version          TEXT        NOT NULL,
    display_name     TEXT        NOT NULL,
    description      TEXT        NULL,
    instruction_path TEXT        NOT NULL,
    manifest_key     TEXT        NOT NULL,
    bundle_key       TEXT        NOT NULL,
    files_prefix     TEXT        NOT NULL,
    platforms        TEXT[]      NOT NULL DEFAULT '{}',
    is_active        BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_skill_packages_org_key_version UNIQUE (org_id, skill_key, version),
    CONSTRAINT chk_skill_packages_key_format CHECK (skill_key ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$'),
    CONSTRAINT chk_skill_packages_version_format CHECK (version ~ '^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$')
);

CREATE TABLE profile_skill_installs (
    profile_ref       TEXT        NOT NULL,
    org_id            UUID        NOT NULL,
    owner_user_id     UUID        NOT NULL,
    skill_key         TEXT        NOT NULL,
    version           TEXT        NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (profile_ref, skill_key, version),
    CONSTRAINT fk_profile_skill_installs_package FOREIGN KEY (org_id, skill_key, version)
        REFERENCES skill_packages (org_id, skill_key, version)
        ON DELETE CASCADE
);

CREATE INDEX idx_profile_skill_installs_profile_ref
    ON profile_skill_installs (org_id, profile_ref);

CREATE TABLE workspace_skill_enablements (
    workspace_ref       TEXT        NOT NULL,
    org_id              UUID        NOT NULL,
    enabled_by_user_id  UUID        NOT NULL,
    skill_key           TEXT        NOT NULL,
    version             TEXT        NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_ref, skill_key),
    CONSTRAINT fk_workspace_skill_enablements_package FOREIGN KEY (org_id, skill_key, version)
        REFERENCES skill_packages (org_id, skill_key, version)
        ON DELETE CASCADE
);

CREATE INDEX idx_workspace_skill_enablements_workspace_ref
    ON workspace_skill_enablements (org_id, workspace_ref);

-- +goose Down

DROP TABLE IF EXISTS workspace_skill_enablements;
DROP TABLE IF EXISTS profile_skill_installs;
DROP TABLE IF EXISTS skill_packages;
