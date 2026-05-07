-- +goose Up

ALTER TABLE tool_provider_configs
    ADD COLUMN scope TEXT NOT NULL DEFAULT 'org';

ALTER TABLE tool_provider_configs
    ALTER COLUMN org_id DROP NOT NULL;

-- 替换表级 UNIQUE 约束为 scope 感知的部分索引
ALTER TABLE tool_provider_configs
    DROP CONSTRAINT tool_provider_configs_org_id_provider_name_key;

CREATE UNIQUE INDEX tool_provider_configs_org_provider_idx
    ON tool_provider_configs (org_id, provider_name)
    WHERE scope = 'org';

CREATE UNIQUE INDEX tool_provider_configs_platform_provider_idx
    ON tool_provider_configs (provider_name)
    WHERE scope = 'platform';

-- 同 scope+group 只允许一个 active
DROP INDEX IF EXISTS ix_tool_provider_configs_org_group_active;

CREATE UNIQUE INDEX ix_tool_provider_configs_org_group_active
    ON tool_provider_configs (org_id, group_name)
    WHERE scope = 'org' AND is_active = true;

CREATE UNIQUE INDEX ix_tool_provider_configs_platform_group_active
    ON tool_provider_configs (group_name)
    WHERE scope = 'platform' AND is_active = true;

-- 迁移：若已有 org 级 active，选最新的作为 platform 默认
WITH ranked AS (
    SELECT
        group_name,
        provider_name,
        secret_id,
        key_prefix,
        base_url,
        config_json,
        ROW_NUMBER() OVER (
            PARTITION BY group_name
            ORDER BY updated_at DESC, created_at DESC, id DESC
        ) AS rn
    FROM tool_provider_configs
    WHERE scope = 'org' AND is_active = true
)
INSERT INTO tool_provider_configs (
    org_id,
    scope,
    group_name,
    provider_name,
    is_active,
    secret_id,
    key_prefix,
    base_url,
    config_json,
    created_at,
    updated_at
)
SELECT
    NULL,
    'platform',
    r.group_name,
    r.provider_name,
    TRUE,
    r.secret_id,
    r.key_prefix,
    r.base_url,
    r.config_json,
    now(),
    now()
FROM ranked r
WHERE r.rn = 1
  AND NOT EXISTS (
      SELECT 1
      FROM tool_provider_configs p
      WHERE p.scope = 'platform' AND p.group_name = r.group_name
  );

-- 迁移：将被 platform 引用的 tool_provider secret 提升为 platform scope
UPDATE secrets s
SET scope = 'platform',
    org_id = NULL,
    updated_at = now()
WHERE s.id IN (
    SELECT secret_id
    FROM tool_provider_configs
    WHERE scope = 'platform' AND secret_id IS NOT NULL
)
  AND s.scope = 'org'
  AND s.name LIKE 'tool_provider:%';

-- +goose Down

-- 回滚时丢弃 platform 级记录
DELETE FROM tool_provider_configs WHERE scope = 'platform';

DROP INDEX IF EXISTS ix_tool_provider_configs_platform_group_active;
DROP INDEX IF EXISTS ix_tool_provider_configs_org_group_active;
DROP INDEX IF EXISTS tool_provider_configs_platform_provider_idx;
DROP INDEX IF EXISTS tool_provider_configs_org_provider_idx;

ALTER TABLE tool_provider_configs
    ADD CONSTRAINT tool_provider_configs_org_id_provider_name_key UNIQUE (org_id, provider_name);

ALTER TABLE tool_provider_configs
    ALTER COLUMN org_id SET NOT NULL;

ALTER TABLE tool_provider_configs
    DROP COLUMN IF EXISTS scope;

CREATE UNIQUE INDEX ix_tool_provider_configs_org_group_active
    ON tool_provider_configs (org_id, group_name)
    WHERE is_active = true;

