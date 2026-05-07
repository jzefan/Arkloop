-- +goose Up

ALTER TABLE projects
    ADD COLUMN owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN is_default BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE INDEX idx_projects_owner_user_id
    ON projects(owner_user_id)
    WHERE owner_user_id IS NOT NULL;

CREATE UNIQUE INDEX uq_projects_default_per_owner
    ON projects(org_id, owner_user_id)
    WHERE deleted_at IS NULL AND is_default = true;

ALTER TABLE threads
    ALTER COLUMN project_id SET NOT NULL;

-- +goose Down

ALTER TABLE threads
    ALTER COLUMN project_id DROP NOT NULL;

DROP INDEX IF EXISTS uq_projects_default_per_owner;
DROP INDEX IF EXISTS idx_projects_owner_user_id;

ALTER TABLE projects
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS is_default,
    DROP COLUMN IF EXISTS owner_user_id;
