-- +goose Up

-- RSA keypairs used to sign OIDC tokens (access_token / id_token).
-- Multiple keys can coexist to support rotation:
--   active    — currently used for signing new tokens
--   retired   — no longer signs new tokens, but still appears in JWKS
--               so tokens issued before the rotation can still be verified
--   compromised — emergency revocation; removed from JWKS immediately
-- The private key is envelope-encrypted via sharedencryption.KeyRing and
-- stored together with its key_version so it can be decrypted on rotation
-- of the master key.
CREATE TABLE oidc_signing_keys (
    kid                          TEXT        PRIMARY KEY,
    algorithm                    TEXT        NOT NULL DEFAULT 'RS256',
    public_key_pem               TEXT        NOT NULL,
    private_key_encrypted        TEXT        NOT NULL,
    private_key_encryption_keyver INTEGER    NOT NULL,
    status                       TEXT        NOT NULL DEFAULT 'active',
    created_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
    activated_at                 TIMESTAMPTZ,
    retired_at                   TIMESTAMPTZ,
    CONSTRAINT oidc_signing_keys_algorithm_check
        CHECK (algorithm IN ('RS256')),
    CONSTRAINT oidc_signing_keys_status_check
        CHECK (status IN ('active', 'retired', 'compromised'))
);

CREATE INDEX ix_oidc_signing_keys_status_created
    ON oidc_signing_keys(status, created_at DESC);

-- +goose Down

DROP INDEX IF EXISTS ix_oidc_signing_keys_status_created;
DROP TABLE IF EXISTS oidc_signing_keys;
