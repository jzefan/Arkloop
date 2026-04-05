-- +goose Up
CREATE TABLE user_impression_snapshots (
    account_id       TEXT NOT NULL,
    user_id          TEXT NOT NULL,
    agent_id         TEXT NOT NULL DEFAULT 'default',
    impression       TEXT NOT NULL DEFAULT '',
    impression_score INTEGER NOT NULL DEFAULT 0,
    updated_at       TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (account_id, user_id, agent_id)
);

-- +goose Down
DROP TABLE IF EXISTS user_impression_snapshots;
