-- +goose Up

CREATE TEMP TABLE _web_search_provider_00089 AS
SELECT
    old.id AS old_id,
    target.id AS target_id,
    old.is_active AS old_is_active,
    old.secret_id AS old_secret_id,
    old.key_prefix AS old_key_prefix,
    old.base_url AS old_base_url,
    old.config_json AS old_config_json
FROM tool_provider_configs old
JOIN tool_provider_configs target
  ON target.provider_name = 'web_search.basic'
 AND target.owner_kind = old.owner_kind
 AND (
      old.owner_kind = 'platform'
      OR (old.owner_kind = 'user' AND target.owner_user_id = old.owner_user_id)
 )
WHERE old.provider_name = 'web_search.duckduckgo';

UPDATE tool_provider_configs
SET is_active = 0,
    updated_at = datetime('now')
WHERE id IN (SELECT old_id FROM _web_search_provider_00089);

UPDATE tool_provider_configs
SET group_name = 'web_search',
    is_active = CASE
        WHEN is_active = 1
          OR EXISTS (
              SELECT 1 FROM _web_search_provider_00089 m
              WHERE m.target_id = tool_provider_configs.id AND m.old_is_active = 1
          )
        THEN 1 ELSE 0 END,
    secret_id = COALESCE(secret_id, (SELECT old_secret_id FROM _web_search_provider_00089 m WHERE m.target_id = tool_provider_configs.id)),
    key_prefix = COALESCE(key_prefix, (SELECT old_key_prefix FROM _web_search_provider_00089 m WHERE m.target_id = tool_provider_configs.id)),
    base_url = COALESCE(base_url, (SELECT old_base_url FROM _web_search_provider_00089 m WHERE m.target_id = tool_provider_configs.id)),
    config_json = CASE
        WHEN COALESCE(config_json, '{}') = '{}'
        THEN COALESCE((SELECT old_config_json FROM _web_search_provider_00089 m WHERE m.target_id = tool_provider_configs.id), '{}')
        ELSE config_json
    END,
    updated_at = datetime('now')
WHERE id IN (SELECT target_id FROM _web_search_provider_00089);

DELETE FROM tool_provider_configs
WHERE id IN (SELECT old_id FROM _web_search_provider_00089);

UPDATE tool_provider_configs
SET provider_name = 'web_search.basic',
    group_name = 'web_search',
    updated_at = datetime('now')
WHERE provider_name = 'web_search.duckduckgo';

DROP TABLE _web_search_provider_00089;

-- +goose Down

CREATE TEMP TABLE _web_search_provider_00089_down AS
SELECT
    old.id AS old_id,
    target.id AS target_id,
    old.is_active AS old_is_active,
    old.secret_id AS old_secret_id,
    old.key_prefix AS old_key_prefix,
    old.base_url AS old_base_url,
    old.config_json AS old_config_json
FROM tool_provider_configs old
JOIN tool_provider_configs target
  ON target.provider_name = 'web_search.duckduckgo'
 AND target.owner_kind = old.owner_kind
 AND (
      old.owner_kind = 'platform'
      OR (old.owner_kind = 'user' AND target.owner_user_id = old.owner_user_id)
 )
WHERE old.provider_name = 'web_search.basic';

UPDATE tool_provider_configs
SET is_active = 0,
    updated_at = datetime('now')
WHERE id IN (SELECT old_id FROM _web_search_provider_00089_down);

UPDATE tool_provider_configs
SET group_name = 'web_search',
    is_active = CASE
        WHEN is_active = 1
          OR EXISTS (
              SELECT 1 FROM _web_search_provider_00089_down m
              WHERE m.target_id = tool_provider_configs.id AND m.old_is_active = 1
          )
        THEN 1 ELSE 0 END,
    secret_id = COALESCE(secret_id, (SELECT old_secret_id FROM _web_search_provider_00089_down m WHERE m.target_id = tool_provider_configs.id)),
    key_prefix = COALESCE(key_prefix, (SELECT old_key_prefix FROM _web_search_provider_00089_down m WHERE m.target_id = tool_provider_configs.id)),
    base_url = COALESCE(base_url, (SELECT old_base_url FROM _web_search_provider_00089_down m WHERE m.target_id = tool_provider_configs.id)),
    config_json = CASE
        WHEN COALESCE(config_json, '{}') = '{}'
        THEN COALESCE((SELECT old_config_json FROM _web_search_provider_00089_down m WHERE m.target_id = tool_provider_configs.id), '{}')
        ELSE config_json
    END,
    updated_at = datetime('now')
WHERE id IN (SELECT target_id FROM _web_search_provider_00089_down);

DELETE FROM tool_provider_configs
WHERE id IN (SELECT old_id FROM _web_search_provider_00089_down);

UPDATE tool_provider_configs
SET provider_name = 'web_search.duckduckgo',
    group_name = 'web_search',
    updated_at = datetime('now')
WHERE provider_name = 'web_search.basic';

DROP TABLE _web_search_provider_00089_down;
