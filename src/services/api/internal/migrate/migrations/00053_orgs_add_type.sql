-- +goose Up

ALTER TABLE orgs
    ADD COLUMN type TEXT NOT NULL DEFAULT 'personal'
    CHECK (type IN ('personal', 'workspace'));

-- +goose Down

ALTER TABLE orgs DROP COLUMN IF EXISTS type;
