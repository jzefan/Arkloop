-- +goose Up
-- 同 org + group 只允许一个 active，先清理历史脏数据再加 UNIQUE partial index。
WITH ranked AS (
    SELECT
        id,
        ROW_NUMBER() OVER (
            PARTITION BY org_id, group_name
            ORDER BY updated_at DESC, created_at DESC, id DESC
        ) AS rn
    FROM tool_provider_configs
    WHERE is_active = TRUE
)
UPDATE tool_provider_configs c
SET is_active = FALSE,
    updated_at = now()
FROM ranked r
WHERE c.id = r.id
  AND r.rn > 1;

DROP INDEX IF EXISTS ix_tool_provider_configs_org_group_active;
CREATE UNIQUE INDEX ix_tool_provider_configs_org_group_active
    ON tool_provider_configs (org_id, group_name)
    WHERE is_active = true;

-- +goose Down
DROP INDEX IF EXISTS ix_tool_provider_configs_org_group_active;
CREATE INDEX ix_tool_provider_configs_org_group_active
    ON tool_provider_configs (org_id, group_name)
    WHERE is_active = true;

