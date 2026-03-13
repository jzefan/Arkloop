-- +goose Up
UPDATE llm_credentials SET scope = 'project' WHERE scope = 'org';
DROP INDEX IF EXISTS llm_credentials_org_name_idx;
CREATE UNIQUE INDEX IF NOT EXISTS llm_credentials_project_name_idx
    ON llm_credentials (org_id, name) WHERE scope = 'project';

-- +goose Down
DROP INDEX IF EXISTS llm_credentials_project_name_idx;
CREATE UNIQUE INDEX IF NOT EXISTS llm_credentials_org_name_idx
    ON llm_credentials (org_id, name) WHERE scope = 'org';
UPDATE llm_credentials SET scope = 'org' WHERE scope = 'project';
