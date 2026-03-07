-- +goose Up
CREATE TABLE IF NOT EXISTS tool_description_overrides (
    org_id      uuid        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    scope       text        NOT NULL DEFAULT 'platform',
    tool_name   text        NOT NULL,
    description text        NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, scope, tool_name)
);

-- +goose Down
DROP TABLE IF EXISTS tool_description_overrides;
