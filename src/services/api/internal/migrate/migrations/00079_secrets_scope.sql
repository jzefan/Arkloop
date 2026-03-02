-- +goose Up

ALTER TABLE secrets
    ADD COLUMN scope TEXT NOT NULL DEFAULT 'org';

ALTER TABLE secrets
    ALTER COLUMN org_id DROP NOT NULL;

-- 替换表级 UNIQUE 约束为 scope 感知的部分索引
ALTER TABLE secrets
    DROP CONSTRAINT uq_secrets_org_name;

CREATE UNIQUE INDEX secrets_org_name_idx
    ON secrets (org_id, name)
    WHERE scope = 'org';

CREATE UNIQUE INDEX secrets_platform_name_idx
    ON secrets (name)
    WHERE scope = 'platform';

-- +goose Down

DROP INDEX IF EXISTS secrets_platform_name_idx;
DROP INDEX IF EXISTS secrets_org_name_idx;

ALTER TABLE secrets
    ADD CONSTRAINT uq_secrets_org_name UNIQUE (org_id, name);

ALTER TABLE secrets
    ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE secrets
    DROP COLUMN IF EXISTS scope;

