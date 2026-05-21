-- +goose Up

-- OIDC standard profile claims. email_verified is derived from the existing
-- email_verified_at timestamp at the application layer; no boolean column
-- is added here to avoid two-source-of-truth drift.
-- IF NOT EXISTS so the migration is re-runnable on a database where a
-- previous failed attempt already added some columns (goose marks the
-- whole migration as not-applied on error, then re-runs from the top).
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS given_name  TEXT,
    ADD COLUMN IF NOT EXISTS family_name TEXT,
    ADD COLUMN IF NOT EXISTS picture_url TEXT,
    ADD COLUMN IF NOT EXISTS updated_at  TIMESTAMPTZ NOT NULL DEFAULT now();

-- +goose StatementBegin
-- goose tokenises by ';' which collides with the embedded semicolons inside
-- a PL/pgSQL function body. The StatementBegin/End markers tell goose to
-- forward the entire block to the server as one statement.
CREATE OR REPLACE FUNCTION users_set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_users_set_updated_at ON users;
CREATE TRIGGER trg_users_set_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION users_set_updated_at();

-- +goose Down

DROP TRIGGER IF EXISTS trg_users_set_updated_at ON users;
DROP FUNCTION IF EXISTS users_set_updated_at();
ALTER TABLE users
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS picture_url,
    DROP COLUMN IF EXISTS family_name,
    DROP COLUMN IF EXISTS given_name;
