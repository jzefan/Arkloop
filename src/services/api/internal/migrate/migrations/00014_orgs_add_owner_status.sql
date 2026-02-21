-- +goose Up
ALTER TABLE orgs
    ADD COLUMN owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN status        TEXT NOT NULL DEFAULT 'active'
                             CHECK (status IN ('active', 'suspended')),
    ADD COLUMN country       TEXT,
    ADD COLUMN timezone      TEXT,
    ADD COLUMN logo_url      TEXT,
    ADD COLUMN settings_json JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN deleted_at    TIMESTAMP WITH TIME ZONE;

-- +goose Down
ALTER TABLE orgs
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS settings_json,
    DROP COLUMN IF EXISTS logo_url,
    DROP COLUMN IF EXISTS timezone,
    DROP COLUMN IF EXISTS country,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS owner_user_id;
