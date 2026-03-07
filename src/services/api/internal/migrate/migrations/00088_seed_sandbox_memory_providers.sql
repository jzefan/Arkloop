-- +goose Up

-- Seed sandbox provider from platform_settings if configured
INSERT INTO tool_provider_configs (
    org_id, scope, group_name, provider_name, is_active, base_url, config_json
)
SELECT
    NULL,
    'platform',
    'sandbox',
    CASE
        WHEN value LIKE '%firecracker%' THEN 'sandbox.firecracker'
        ELSE 'sandbox.docker'
    END,
    true,
    value,
    '{}'::jsonb
FROM platform_settings
WHERE key = 'sandbox.base_url' AND TRIM(value) != ''
ON CONFLICT DO NOTHING;

-- Seed memory (openviking) provider from platform_settings if configured
-- api_key cannot be migrated here (requires encryption); configure via Console UI
INSERT INTO tool_provider_configs (
    org_id, scope, group_name, provider_name, is_active, base_url, config_json
)
SELECT
    NULL,
    'platform',
    'memory',
    'memory.openviking',
    true,
    value,
    '{}'::jsonb
FROM platform_settings
WHERE key = 'openviking.base_url' AND TRIM(value) != ''
ON CONFLICT DO NOTHING;

-- +goose Down
DELETE FROM tool_provider_configs
WHERE scope = 'platform'
  AND provider_name IN ('sandbox.docker', 'sandbox.firecracker', 'memory.openviking')
  AND secret_id IS NULL;
