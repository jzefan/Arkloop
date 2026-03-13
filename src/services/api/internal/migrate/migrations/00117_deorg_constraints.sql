-- +goose Up

-- ============================================================
-- Phase 1: Backfill project_id from org_id -> default project
-- ============================================================

UPDATE personas p
SET project_id = (
    SELECT pr.id FROM projects pr
    WHERE pr.org_id = p.org_id AND pr.deleted_at IS NULL
    ORDER BY pr.created_at ASC LIMIT 1
)
WHERE p.org_id IS NOT NULL AND p.project_id IS NULL;

UPDATE tool_provider_configs tpc
SET project_id = (
    SELECT pr.id FROM projects pr
    WHERE pr.org_id = tpc.org_id AND pr.deleted_at IS NULL
    ORDER BY pr.created_at ASC LIMIT 1
)
WHERE tpc.scope = 'org' AND tpc.org_id IS NOT NULL AND tpc.project_id IS NULL;

UPDATE tool_description_overrides tdo
SET project_id = (
    SELECT pr.id FROM projects pr
    WHERE pr.org_id = tdo.org_id AND pr.deleted_at IS NULL
    ORDER BY pr.created_at ASC LIMIT 1
)
WHERE tdo.scope = 'org' AND tdo.org_id != '00000000-0000-0000-0000-000000000000' AND tdo.project_id IS NULL;

-- ============================================================
-- Phase 2: Normalize scope values org -> project
-- ============================================================

UPDATE tool_provider_configs SET scope = 'project' WHERE scope = 'org';
UPDATE tool_description_overrides SET scope = 'project' WHERE scope = 'org';

-- ============================================================
-- Phase 3: Personas constraints
-- ============================================================

ALTER TABLE personas DROP CONSTRAINT IF EXISTS uq_personas_org_key_version;

CREATE UNIQUE INDEX IF NOT EXISTS uq_personas_platform_key_version
    ON personas (persona_key, version)
    WHERE project_id IS NULL;

-- ============================================================
-- Phase 4: tool_provider_configs constraints
-- ============================================================

DROP INDEX IF EXISTS tool_provider_configs_org_provider_idx;
DROP INDEX IF EXISTS ix_tool_provider_configs_org_group_active;

DROP INDEX IF EXISTS ix_tool_provider_configs_project_group_active;
CREATE UNIQUE INDEX ix_tool_provider_configs_project_group_active
    ON tool_provider_configs (project_id, group_name)
    WHERE project_id IS NOT NULL AND is_active = TRUE;

CREATE UNIQUE INDEX IF NOT EXISTS tool_provider_configs_project_provider_idx
    ON tool_provider_configs (project_id, provider_name)
    WHERE project_id IS NOT NULL;

-- ============================================================
-- Phase 5: tool_description_overrides restructure PK
-- ============================================================

ALTER TABLE tool_description_overrides
    ADD COLUMN IF NOT EXISTS id UUID DEFAULT gen_random_uuid();

UPDATE tool_description_overrides SET id = gen_random_uuid() WHERE id IS NULL;

ALTER TABLE tool_description_overrides ALTER COLUMN id SET NOT NULL;

ALTER TABLE tool_description_overrides DROP CONSTRAINT IF EXISTS tool_description_overrides_pkey;

ALTER TABLE tool_description_overrides
    ADD CONSTRAINT tool_description_overrides_pkey PRIMARY KEY (id);

DROP INDEX IF EXISTS ix_tool_description_overrides_project_tool;

CREATE UNIQUE INDEX uq_tool_description_overrides_platform_tool
    ON tool_description_overrides (tool_name)
    WHERE scope = 'platform';

CREATE UNIQUE INDEX uq_tool_description_overrides_project_tool
    ON tool_description_overrides (project_id, tool_name)
    WHERE project_id IS NOT NULL;

-- +goose Down

DROP INDEX IF EXISTS uq_tool_description_overrides_project_tool;
DROP INDEX IF EXISTS uq_tool_description_overrides_platform_tool;

ALTER TABLE tool_description_overrides DROP CONSTRAINT IF EXISTS tool_description_overrides_pkey;
ALTER TABLE tool_description_overrides
    ADD CONSTRAINT tool_description_overrides_pkey PRIMARY KEY (org_id, scope, tool_name);

ALTER TABLE tool_description_overrides DROP COLUMN IF EXISTS id;

CREATE INDEX IF NOT EXISTS ix_tool_description_overrides_project_tool
    ON tool_description_overrides (project_id, tool_name)
    WHERE project_id IS NOT NULL;

DROP INDEX IF EXISTS tool_provider_configs_project_provider_idx;

DROP INDEX IF EXISTS ix_tool_provider_configs_project_group_active;
CREATE INDEX ix_tool_provider_configs_project_group_active
    ON tool_provider_configs (project_id, group_name)
    WHERE project_id IS NOT NULL AND is_active = TRUE;

CREATE UNIQUE INDEX tool_provider_configs_org_provider_idx
    ON tool_provider_configs (org_id, provider_name)
    WHERE scope = 'org';

CREATE UNIQUE INDEX ix_tool_provider_configs_org_group_active
    ON tool_provider_configs (org_id, group_name)
    WHERE scope = 'org' AND is_active = true;

DROP INDEX IF EXISTS uq_personas_platform_key_version;

ALTER TABLE personas
    ADD CONSTRAINT uq_personas_org_key_version UNIQUE NULLS NOT DISTINCT (org_id, persona_key, version);

UPDATE tool_description_overrides SET scope = 'org' WHERE scope = 'project';
UPDATE tool_provider_configs SET scope = 'org' WHERE scope = 'project';
