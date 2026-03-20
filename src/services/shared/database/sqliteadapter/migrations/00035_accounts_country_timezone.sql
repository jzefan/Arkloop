-- Align accounts with PG (00014_orgs_add_owner_status): columns used by AccountRepository.

-- +goose Up

ALTER TABLE accounts ADD COLUMN country TEXT;
ALTER TABLE accounts ADD COLUMN timezone TEXT;
ALTER TABLE accounts ADD COLUMN logo_url TEXT;
ALTER TABLE accounts ADD COLUMN settings_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE accounts ADD COLUMN deleted_at TEXT;

-- +goose Down

ALTER TABLE accounts DROP COLUMN deleted_at;
ALTER TABLE accounts DROP COLUMN settings_json;
ALTER TABLE accounts DROP COLUMN logo_url;
ALTER TABLE accounts DROP COLUMN timezone;
ALTER TABLE accounts DROP COLUMN country;
