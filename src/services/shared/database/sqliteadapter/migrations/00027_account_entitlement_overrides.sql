-- +goose Up

CREATE TABLE IF NOT EXISTS plan_entitlements (
    id         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    plan_id    TEXT NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    value      TEXT NOT NULL,
    value_type TEXT NOT NULL CHECK (value_type IN ('int', 'bool', 'string')),
    UNIQUE (plan_id, key)
);

CREATE INDEX IF NOT EXISTS idx_plan_entitlements_plan_id ON plan_entitlements(plan_id);

CREATE TABLE IF NOT EXISTS account_entitlement_overrides (
    id                 TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id         TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    key                TEXT NOT NULL,
    value              TEXT NOT NULL,
    value_type         TEXT NOT NULL CHECK (value_type IN ('int', 'bool', 'string')),
    reason             TEXT,
    expires_at         TEXT,
    created_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at         TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (account_id, key)
);

CREATE INDEX IF NOT EXISTS idx_account_entitlement_overrides_account_id
    ON account_entitlement_overrides(account_id);

-- +goose Down

DROP INDEX IF EXISTS idx_account_entitlement_overrides_account_id;
DROP TABLE IF EXISTS account_entitlement_overrides;
DROP INDEX IF EXISTS idx_plan_entitlements_plan_id;
DROP TABLE IF EXISTS plan_entitlements;
