-- +goose Up

ALTER TABLE personas
    ADD COLUMN model TEXT,
    ADD COLUMN reasoning_mode TEXT NOT NULL DEFAULT 'auto',
    ADD COLUMN prompt_cache_control TEXT NOT NULL DEFAULT 'none';

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION arkloop_merge_persona_budgets(
    base JSONB,
    cfg_temperature DOUBLE PRECISION,
    cfg_max_output_tokens INT,
    cfg_top_p DOUBLE PRECISION
) RETURNS JSONB
LANGUAGE plpgsql
AS $$
DECLARE
    result JSONB := COALESCE(base, '{}'::jsonb);
    existing_max INT;
BEGIN
    IF jsonb_typeof(result) IS DISTINCT FROM 'object' THEN
        result := '{}'::jsonb;
    END IF;

    IF cfg_temperature IS NOT NULL AND NOT (result ? 'temperature') THEN
        result := result || jsonb_build_object('temperature', cfg_temperature);
    END IF;

    IF cfg_top_p IS NOT NULL AND NOT (result ? 'top_p') THEN
        result := result || jsonb_build_object('top_p', cfg_top_p);
    END IF;

    IF cfg_max_output_tokens IS NOT NULL THEN
        IF result ? 'max_output_tokens' THEN
            BEGIN
                existing_max := floor((result ->> 'max_output_tokens')::numeric)::INT;
            EXCEPTION WHEN OTHERS THEN
                existing_max := NULL;
            END;

            IF existing_max IS NULL THEN
                result := result || jsonb_build_object('max_output_tokens', cfg_max_output_tokens);
            ELSE
                result := result || jsonb_build_object('max_output_tokens', LEAST(existing_max, cfg_max_output_tokens));
            END IF;
        ELSE
            result := result || jsonb_build_object('max_output_tokens', cfg_max_output_tokens);
        END IF;
    END IF;

    RETURN result;
END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION arkloop_intersect_text_arrays(left_arr TEXT[], right_arr TEXT[])
RETURNS TEXT[]
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT COALESCE(array_agg(value ORDER BY value), '{}'::TEXT[])
    FROM (
        SELECT DISTINCT l.value
        FROM unnest(COALESCE(left_arr, '{}'::TEXT[])) AS l(value)
        INNER JOIN unnest(COALESCE(right_arr, '{}'::TEXT[])) AS r(value)
            ON l.value = r.value
    ) AS matched(value);
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION arkloop_union_text_arrays(left_arr TEXT[], right_arr TEXT[])
RETURNS TEXT[]
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT COALESCE(array_agg(value ORDER BY value), '{}'::TEXT[])
    FROM (
        SELECT DISTINCT value
        FROM unnest(COALESCE(left_arr, '{}'::TEXT[]) || COALESCE(right_arr, '{}'::TEXT[])) AS merged(value)
    ) AS deduped;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION arkloop_merge_prompt(prefix TEXT, prompt TEXT)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT CASE
        WHEN NULLIF(BTRIM(COALESCE(prefix, '')), '') IS NOT NULL
         AND NULLIF(BTRIM(COALESCE(prompt, '')), '') IS NOT NULL
            THEN BTRIM(prefix) || E'\n\n' || BTRIM(prompt)
        WHEN NULLIF(BTRIM(COALESCE(prefix, '')), '') IS NOT NULL
            THEN BTRIM(prefix)
        ELSE BTRIM(COALESCE(prompt, ''))
    END;
$$;
-- +goose StatementEnd

WITH explicit_match AS (
    SELECT
        p.id AS persona_id,
        ac.id,
        ac.scope,
        ac.org_id,
        ac.model,
        ac.reasoning_mode,
        ac.prompt_cache_control,
        ac.temperature,
        ac.max_output_tokens,
        ac.top_p,
        ac.tool_policy,
        ac.tool_allowlist,
        ac.tool_denylist,
        ac.system_prompt_override,
        ac.system_prompt_template_id,
        ac.created_at,
        1 AS match_priority,
        CASE WHEN ac.scope = 'org' AND ac.org_id = p.org_id THEN 0 ELSE 1 END AS scope_priority
    FROM personas p
    JOIN agent_configs ac
      ON p.agent_config_name IS NOT NULL
     AND LOWER(ac.name) = LOWER(p.agent_config_name)
     AND ((ac.scope = 'org' AND ac.org_id = p.org_id) OR ac.scope = 'platform')
),
persona_match AS (
    SELECT
        p.id AS persona_id,
        ac.id,
        ac.scope,
        ac.org_id,
        ac.model,
        ac.reasoning_mode,
        ac.prompt_cache_control,
        ac.temperature,
        ac.max_output_tokens,
        ac.top_p,
        ac.tool_policy,
        ac.tool_allowlist,
        ac.tool_denylist,
        ac.system_prompt_override,
        ac.system_prompt_template_id,
        ac.created_at,
        2 AS match_priority,
        CASE WHEN ac.scope = 'org' AND ac.org_id = p.org_id THEN 0 ELSE 1 END AS scope_priority
    FROM personas p
    JOIN agent_configs ac
      ON ac.persona_id = p.id
),
ranked_match AS (
    SELECT *, ROW_NUMBER() OVER (
        PARTITION BY persona_id
        ORDER BY match_priority ASC, scope_priority ASC, created_at DESC, id DESC
    ) AS row_num
    FROM (
        SELECT * FROM explicit_match
        UNION ALL
        SELECT * FROM persona_match
    ) AS merged
),
selected_match AS (
    SELECT
        rm.persona_id,
        rm.model,
        rm.reasoning_mode,
        rm.prompt_cache_control,
        rm.temperature,
        rm.max_output_tokens,
        rm.top_p,
        rm.tool_policy,
        rm.tool_allowlist,
        rm.tool_denylist,
        COALESCE(NULLIF(BTRIM(rm.system_prompt_override), ''), NULLIF(BTRIM(pt.content), '')) AS resolved_prompt_prefix
    FROM ranked_match rm
    LEFT JOIN prompt_templates pt
      ON pt.id = rm.system_prompt_template_id
    WHERE rm.row_num = 1
)
UPDATE personas p
SET model = sm.model,
    reasoning_mode = COALESCE(NULLIF(BTRIM(sm.reasoning_mode), ''), 'auto'),
    prompt_cache_control = COALESCE(NULLIF(BTRIM(sm.prompt_cache_control), ''), 'none'),
    prompt_md = arkloop_merge_prompt(sm.resolved_prompt_prefix, p.prompt_md),
    budgets_json = arkloop_merge_persona_budgets(p.budgets_json, sm.temperature, sm.max_output_tokens, sm.top_p),
    tool_allowlist = CASE
        WHEN sm.tool_policy = 'allowlist' THEN
            CASE
                WHEN COALESCE(array_length(p.tool_allowlist, 1), 0) > 0
                    THEN arkloop_intersect_text_arrays(p.tool_allowlist, sm.tool_allowlist)
                ELSE COALESCE(sm.tool_allowlist, '{}'::TEXT[])
            END
        ELSE p.tool_allowlist
    END,
    tool_denylist = CASE
        WHEN sm.tool_policy = 'denylist'
            THEN arkloop_union_text_arrays(p.tool_denylist, sm.tool_denylist)
        ELSE p.tool_denylist
    END
FROM selected_match sm
WHERE sm.persona_id = p.id;

DROP FUNCTION IF EXISTS arkloop_merge_persona_budgets(JSONB, DOUBLE PRECISION, INT, DOUBLE PRECISION);
DROP FUNCTION IF EXISTS arkloop_intersect_text_arrays(TEXT[], TEXT[]);
DROP FUNCTION IF EXISTS arkloop_union_text_arrays(TEXT[], TEXT[]);
DROP FUNCTION IF EXISTS arkloop_merge_prompt(TEXT, TEXT);

UPDATE rbac_roles
SET permissions = array_remove(array_remove(permissions, 'data.agent_configs.read'), 'data.agent_configs.manage')
WHERE permissions && ARRAY['data.agent_configs.read', 'data.agent_configs.manage'];

ALTER TABLE threads DROP COLUMN IF EXISTS agent_config_id;
ALTER TABLE personas DROP COLUMN IF EXISTS agent_config_name;

DROP TABLE IF EXISTS agent_configs;
DROP TABLE IF EXISTS prompt_templates;

-- +goose Down

CREATE TABLE prompt_templates (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID        NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name         TEXT        NOT NULL,
    content      TEXT        NOT NULL,
    variables    JSONB       NOT NULL DEFAULT '[]',
    is_default   BOOLEAN     NOT NULL DEFAULT false,
    version      INT         NOT NULL DEFAULT 1,
    published_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_prompt_templates_org_id ON prompt_templates(org_id);

CREATE TABLE agent_configs (
    id                        UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id                    UUID        REFERENCES orgs(id) ON DELETE CASCADE,
    scope                     TEXT        NOT NULL DEFAULT 'org',
    name                      TEXT        NOT NULL,
    system_prompt_template_id UUID        REFERENCES prompt_templates(id) ON DELETE SET NULL,
    system_prompt_override    TEXT,
    model                     TEXT,
    temperature               DOUBLE PRECISION,
    max_output_tokens         INT,
    top_p                     DOUBLE PRECISION,
    context_window_limit      INT,
    tool_policy               TEXT        NOT NULL DEFAULT 'allowlist'
                              CHECK (tool_policy IN ('allowlist', 'denylist', 'none')),
    tool_allowlist            TEXT[]      NOT NULL DEFAULT '{}',
    tool_denylist             TEXT[]      NOT NULL DEFAULT '{}',
    content_filter_level      TEXT        NOT NULL DEFAULT 'standard',
    safety_rules_json         JSONB       NOT NULL DEFAULT '{}',
    project_id                UUID        REFERENCES projects(id) ON DELETE CASCADE,
    persona_id                UUID        REFERENCES personas(id) ON DELETE SET NULL,
    is_default                BOOLEAN     NOT NULL DEFAULT false,
    prompt_cache_control      TEXT        NOT NULL DEFAULT 'none',
    reasoning_mode            TEXT        NOT NULL DEFAULT 'auto',
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_configs_org_id ON agent_configs(org_id);
CREATE INDEX idx_agent_configs_project_id ON agent_configs(project_id) WHERE project_id IS NOT NULL;

ALTER TABLE personas ADD COLUMN agent_config_name TEXT;
ALTER TABLE threads ADD COLUMN agent_config_id UUID REFERENCES agent_configs(id) ON DELETE SET NULL;

ALTER TABLE personas
    DROP COLUMN IF EXISTS model,
    DROP COLUMN IF EXISTS reasoning_mode,
    DROP COLUMN IF EXISTS prompt_cache_control;

UPDATE rbac_roles
SET permissions = CASE
    WHEN NOT ('data.agent_configs.read' = ANY(permissions))
        THEN permissions || ARRAY['data.agent_configs.read']
    ELSE permissions
END;

UPDATE rbac_roles
SET permissions = CASE
    WHEN NOT ('data.agent_configs.manage' = ANY(permissions))
        THEN permissions || ARRAY['data.agent_configs.manage']
    ELSE permissions
END;
