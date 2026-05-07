-- Align desktop platform_settings with the repository contract.

-- +goose Up

ALTER TABLE platform_settings RENAME TO platform_settings_legacy_00029;

CREATE TABLE platform_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO platform_settings (key, value, updated_at)
SELECT key, value, datetime('now')
FROM platform_settings_legacy_00029;

DROP TABLE platform_settings_legacy_00029;

-- +goose Down

ALTER TABLE platform_settings RENAME TO platform_settings_with_updated_at_00029;

CREATE TABLE platform_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO platform_settings (key, value)
SELECT key, value
FROM platform_settings_with_updated_at_00029;

DROP TABLE platform_settings_with_updated_at_00029;
