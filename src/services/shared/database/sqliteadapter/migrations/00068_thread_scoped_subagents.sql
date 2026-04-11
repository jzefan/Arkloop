-- +goose Up

ALTER TABLE sub_agents RENAME TO sub_agents_old_00068;

CREATE TABLE sub_agents (
    id                    TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id            TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    owner_thread_id       TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    agent_thread_id       TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    origin_run_id         TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    parent_sub_agent_id   TEXT REFERENCES sub_agents(id) ON DELETE SET NULL,
    depth                 INTEGER NOT NULL CHECK (depth >= 0),
    role                  TEXT,
    persona_id            TEXT,
    nickname              TEXT,
    source_type           TEXT NOT NULL,
    context_mode          TEXT NOT NULL,
    status                TEXT NOT NULL CHECK (
        status IN (
            'created',
            'queued',
            'running',
            'waiting_input',
            'completed',
            'failed',
            'cancelled',
            'closed',
            'resumable'
        )
    ),
    current_run_id        TEXT REFERENCES runs(id) ON DELETE SET NULL,
    last_completed_run_id TEXT REFERENCES runs(id) ON DELETE SET NULL,
    last_output_ref       TEXT,
    last_error            TEXT,
    created_at            TEXT NOT NULL DEFAULT (datetime('now')),
    started_at            TEXT,
    completed_at          TEXT,
    closed_at             TEXT
);

INSERT INTO sub_agents (
    id, account_id, owner_thread_id, agent_thread_id, origin_run_id, parent_sub_agent_id,
    depth, role, persona_id, nickname, source_type, context_mode, status,
    current_run_id, last_completed_run_id, last_output_ref, last_error,
    created_at, started_at, completed_at, closed_at
)
SELECT
    old.id,
    old.account_id,
    COALESCE(old.parent_thread_id, old.root_thread_id),
    COALESCE(
        (SELECT r.thread_id FROM runs r WHERE r.id = old.current_run_id),
        (SELECT r.thread_id FROM runs r WHERE r.id = old.last_completed_run_id),
        old.parent_thread_id,
        old.root_thread_id
    ),
    old.parent_run_id,
    NULL,
    old.depth,
    old.role,
    old.persona_id,
    old.nickname,
    old.source_type,
    old.context_mode,
    old.status,
    old.current_run_id,
    old.last_completed_run_id,
    old.last_output_ref,
    old.last_error,
    old.created_at,
    old.started_at,
    old.completed_at,
    old.closed_at
FROM sub_agents_old_00068 old;

DROP TABLE sub_agents_old_00068;

CREATE INDEX idx_sub_agents_account_id ON sub_agents(account_id);
CREATE INDEX idx_sub_agents_owner_thread_id ON sub_agents(owner_thread_id);
CREATE INDEX idx_sub_agents_parent_sub_agent_id ON sub_agents(parent_sub_agent_id) WHERE parent_sub_agent_id IS NOT NULL;
CREATE INDEX idx_sub_agents_current_run_id ON sub_agents(current_run_id) WHERE current_run_id IS NOT NULL;
CREATE INDEX idx_sub_agents_status ON sub_agents(status);

ALTER TABLE sub_agent_events RENAME TO sub_agent_events_old_00068;
CREATE TABLE sub_agent_events (
    event_id      TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    sub_agent_id  TEXT NOT NULL REFERENCES sub_agents(id) ON DELETE CASCADE,
    run_id        TEXT REFERENCES runs(id) ON DELETE SET NULL,
    seq           INTEGER NOT NULL,
    ts            TEXT NOT NULL DEFAULT (datetime('now')),
    type          TEXT NOT NULL,
    data_json     TEXT NOT NULL DEFAULT '{}',
    error_class   TEXT,
    UNIQUE (sub_agent_id, seq)
);
INSERT INTO sub_agent_events (event_id, sub_agent_id, run_id, seq, ts, type, data_json, error_class)
SELECT event_id, sub_agent_id, run_id, seq, ts, type, data_json, error_class FROM sub_agent_events_old_00068;
DROP TABLE sub_agent_events_old_00068;
CREATE INDEX idx_sub_agent_events_sub_agent_id_ts ON sub_agent_events(sub_agent_id, ts);
CREATE INDEX idx_sub_agent_events_type ON sub_agent_events(type);
CREATE INDEX idx_sub_agent_events_run_id ON sub_agent_events(run_id) WHERE run_id IS NOT NULL;

ALTER TABLE sub_agent_pending_inputs RENAME TO sub_agent_pending_inputs_old_00068;
CREATE TABLE sub_agent_pending_inputs (
    id           TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    sub_agent_id TEXT NOT NULL REFERENCES sub_agents(id) ON DELETE CASCADE,
    seq          INTEGER NOT NULL,
    input        TEXT NOT NULL,
    priority     INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (sub_agent_id, seq)
);
INSERT INTO sub_agent_pending_inputs (id, sub_agent_id, seq, input, priority, created_at)
SELECT id, sub_agent_id, seq, input, priority, created_at FROM sub_agent_pending_inputs_old_00068;
DROP TABLE sub_agent_pending_inputs_old_00068;
CREATE INDEX idx_sub_agent_pending_inputs_sub_agent_id_seq
    ON sub_agent_pending_inputs(sub_agent_id, priority DESC, seq ASC);

ALTER TABLE sub_agent_context_snapshots RENAME TO sub_agent_context_snapshots_old_00068;
CREATE TABLE sub_agent_context_snapshots (
    sub_agent_id  TEXT PRIMARY KEY REFERENCES sub_agents(id) ON DELETE CASCADE,
    snapshot_json TEXT NOT NULL,
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO sub_agent_context_snapshots (sub_agent_id, snapshot_json, created_at, updated_at)
SELECT sub_agent_id, snapshot_json, created_at, updated_at FROM sub_agent_context_snapshots_old_00068;
DROP TABLE sub_agent_context_snapshots_old_00068;
CREATE INDEX idx_sub_agent_context_snapshots_updated_at
    ON sub_agent_context_snapshots(updated_at);

CREATE TABLE thread_subagent_callbacks (
    id                 TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id         TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    thread_id          TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    sub_agent_id       TEXT NOT NULL REFERENCES sub_agents(id) ON DELETE CASCADE,
    source_run_id      TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    status             TEXT NOT NULL,
    payload_json       TEXT NOT NULL DEFAULT '{}',
    created_at         TEXT NOT NULL DEFAULT (datetime('now')),
    consumed_at        TEXT,
    consumed_by_run_id TEXT REFERENCES runs(id) ON DELETE SET NULL
);
CREATE INDEX idx_thread_subagent_callbacks_thread_pending
    ON thread_subagent_callbacks(thread_id, created_at);

-- +goose Down

DROP INDEX IF EXISTS idx_thread_subagent_callbacks_thread_pending;
DROP TABLE IF EXISTS thread_subagent_callbacks;

DROP INDEX IF EXISTS idx_sub_agent_context_snapshots_updated_at;
DROP TABLE IF EXISTS sub_agent_context_snapshots;

DROP INDEX IF EXISTS idx_sub_agent_pending_inputs_sub_agent_id_seq;
DROP TABLE IF EXISTS sub_agent_pending_inputs;

DROP INDEX IF EXISTS idx_sub_agent_events_run_id;
DROP INDEX IF EXISTS idx_sub_agent_events_type;
DROP INDEX IF EXISTS idx_sub_agent_events_sub_agent_id_ts;
DROP TABLE IF EXISTS sub_agent_events;

DROP INDEX IF EXISTS idx_sub_agents_status;
DROP INDEX IF EXISTS idx_sub_agents_current_run_id;
DROP INDEX IF EXISTS idx_sub_agents_parent_sub_agent_id;
DROP INDEX IF EXISTS idx_sub_agents_owner_thread_id;
DROP INDEX IF EXISTS idx_sub_agents_account_id;
DROP TABLE IF EXISTS sub_agents;
