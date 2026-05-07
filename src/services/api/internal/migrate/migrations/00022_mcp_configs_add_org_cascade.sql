-- +goose Up
ALTER TABLE mcp_configs
    DROP CONSTRAINT mcp_configs_org_id_fkey,
    ADD CONSTRAINT mcp_configs_org_id_fkey FOREIGN KEY (org_id) REFERENCES orgs(id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE mcp_configs
    DROP CONSTRAINT mcp_configs_org_id_fkey,
    ADD CONSTRAINT mcp_configs_org_id_fkey FOREIGN KEY (org_id) REFERENCES orgs(id);
