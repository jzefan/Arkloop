-- +goose Up

CREATE TABLE redemption_codes (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    code               TEXT        NOT NULL UNIQUE,
    type               TEXT        NOT NULL CHECK (type IN ('credit', 'feature')),
    value              TEXT        NOT NULL,
    max_uses           INT         NOT NULL DEFAULT 1,
    use_count          INT         NOT NULL DEFAULT 0,
    expires_at         TIMESTAMPTZ,
    is_active          BOOLEAN     NOT NULL DEFAULT true,
    batch_id           TEXT,
    created_by_user_id UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_redemption_codes_batch_id ON redemption_codes(batch_id) WHERE batch_id IS NOT NULL;
CREATE INDEX idx_redemption_codes_created_at ON redemption_codes(created_at DESC, id DESC);

CREATE TABLE redemption_records (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    code_id     UUID        NOT NULL REFERENCES redemption_codes(id) ON DELETE CASCADE,
    user_id     UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id      UUID        NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    redeemed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(code_id, user_id)
);

CREATE INDEX idx_redemption_records_user_id ON redemption_records(user_id);

-- +goose Down

DROP TABLE IF EXISTS redemption_records;
DROP TABLE IF EXISTS redemption_codes;
