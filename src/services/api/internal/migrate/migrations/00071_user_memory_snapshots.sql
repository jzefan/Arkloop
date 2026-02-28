-- +goose Up
CREATE TABLE user_memory_snapshots (
    org_id       UUID NOT NULL,
    user_id      UUID NOT NULL,
    agent_id     TEXT NOT NULL DEFAULT 'default',
    memory_block TEXT NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, user_id, agent_id)
);

-- +goose Down
DROP TABLE IF EXISTS user_memory_snapshots;
