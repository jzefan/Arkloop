-- +goose Up

ALTER TABLE asr_credentials
    ADD COLUMN scope TEXT NOT NULL DEFAULT 'org';

ALTER TABLE asr_credentials
    ALTER COLUMN org_id DROP NOT NULL;

-- 替换表级 UNIQUE 约束为 scope 感知的部分索引
ALTER TABLE asr_credentials
    DROP CONSTRAINT asr_credentials_org_id_name_key;

CREATE UNIQUE INDEX asr_credentials_org_name_idx
    ON asr_credentials (org_id, name)
    WHERE scope = 'org';

CREATE UNIQUE INDEX asr_credentials_platform_name_idx
    ON asr_credentials (name)
    WHERE scope = 'platform';

-- 替换 default 唯一索引
DROP INDEX asr_credentials_org_default_idx;

CREATE UNIQUE INDEX asr_credentials_org_default_idx
    ON asr_credentials (org_id)
    WHERE scope = 'org' AND is_default = true AND revoked_at IS NULL;

-- platform 全局最多 1 个 default
CREATE UNIQUE INDEX asr_credentials_platform_default_idx
    ON asr_credentials (is_default)
    WHERE scope = 'platform' AND is_default = true AND revoked_at IS NULL;

-- +goose Down

DROP INDEX IF EXISTS asr_credentials_platform_default_idx;
DROP INDEX IF EXISTS asr_credentials_org_default_idx;
DROP INDEX IF EXISTS asr_credentials_platform_name_idx;
DROP INDEX IF EXISTS asr_credentials_org_name_idx;

CREATE UNIQUE INDEX asr_credentials_org_default_idx
    ON asr_credentials (org_id)
    WHERE is_default = true AND revoked_at IS NULL;

ALTER TABLE asr_credentials
    ADD CONSTRAINT asr_credentials_org_id_name_key UNIQUE (org_id, name);

ALTER TABLE asr_credentials
    ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE asr_credentials
    DROP COLUMN IF EXISTS scope;
