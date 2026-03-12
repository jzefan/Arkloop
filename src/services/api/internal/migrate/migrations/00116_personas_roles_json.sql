ALTER TABLE personas
    ADD COLUMN roles_json JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE personas
    DROP COLUMN IF EXISTS roles_json;
