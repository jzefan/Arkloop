-- +goose Up
CREATE TEMP TABLE _web_search_provider_00181 AS
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
      OR (old.owner_kind = 'user' AND target.owner_user_id IS NOT DISTINCT FROM old.owner_user_id)
 )
WHERE old.provider_name = 'web_search.duckduckgo';

UPDATE tool_provider_configs
SET is_active = FALSE,
    updated_at = now()
WHERE id IN (SELECT old_id FROM _web_search_provider_00181);

UPDATE tool_provider_configs target
SET group_name = 'web_search',
    is_active = target.is_active OR m.old_is_active,
    secret_id = COALESCE(target.secret_id, m.old_secret_id),
    key_prefix = COALESCE(target.key_prefix, m.old_key_prefix),
    base_url = COALESCE(target.base_url, m.old_base_url),
    config_json = CASE
        WHEN target.config_json = '{}'::jsonb THEN COALESCE(m.old_config_json, '{}'::jsonb)
        ELSE target.config_json
    END,
    updated_at = now()
FROM _web_search_provider_00181 m
WHERE target.id = m.target_id;

DELETE FROM tool_provider_configs
WHERE id IN (SELECT old_id FROM _web_search_provider_00181);

UPDATE tool_provider_configs
SET provider_name = 'web_search.basic',
    group_name = 'web_search',
    updated_at = now()
WHERE provider_name = 'web_search.duckduckgo';

DROP TABLE _web_search_provider_00181;

-- +goose Down
CREATE TEMP TABLE _web_search_provider_00181_down AS
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
      OR (old.owner_kind = 'user' AND target.owner_user_id IS NOT DISTINCT FROM old.owner_user_id)
 )
WHERE old.provider_name = 'web_search.basic';

UPDATE tool_provider_configs
SET is_active = FALSE,
    updated_at = now()
WHERE id IN (SELECT old_id FROM _web_search_provider_00181_down);

UPDATE tool_provider_configs target
SET group_name = 'web_search',
    is_active = target.is_active OR m.old_is_active,
    secret_id = COALESCE(target.secret_id, m.old_secret_id),
    key_prefix = COALESCE(target.key_prefix, m.old_key_prefix),
    base_url = COALESCE(target.base_url, m.old_base_url),
    config_json = CASE
        WHEN target.config_json = '{}'::jsonb THEN COALESCE(m.old_config_json, '{}'::jsonb)
        ELSE target.config_json
    END,
    updated_at = now()
FROM _web_search_provider_00181_down m
WHERE target.id = m.target_id;

DELETE FROM tool_provider_configs
WHERE id IN (SELECT old_id FROM _web_search_provider_00181_down);

UPDATE tool_provider_configs
SET provider_name = 'web_search.duckduckgo',
    group_name = 'web_search',
    updated_at = now()
WHERE provider_name = 'web_search.basic';

DROP TABLE _web_search_provider_00181_down;
