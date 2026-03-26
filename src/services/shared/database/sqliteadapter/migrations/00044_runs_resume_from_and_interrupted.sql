-- +goose Up
PRAGMA foreign_keys = OFF;

CREATE TABLE runs_new (
    id                  TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id          TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    thread_id           TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    created_by_user_id  TEXT REFERENCES users(id) ON DELETE SET NULL,
    status              TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'completed', 'failed', 'cancelled', 'cancelling', 'interrupted')),
    next_event_seq      INTEGER NOT NULL DEFAULT 1,
    parent_run_id       TEXT REFERENCES runs(id) ON DELETE SET NULL,
    resume_from_run_id  TEXT REFERENCES runs(id) ON DELETE SET NULL,
    status_updated_at   TEXT,
    completed_at        TEXT,
    failed_at           TEXT,
    duration_ms         INTEGER,
    total_input_tokens  INTEGER,
    total_output_tokens INTEGER,
    total_cost_usd      TEXT,
    model               TEXT,
    persona_id          TEXT,
    deleted_at          TEXT,
    profile_ref         TEXT,
    workspace_ref       TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO runs_new (
    id, account_id, thread_id, created_by_user_id, status, next_event_seq,
    parent_run_id, resume_from_run_id, status_updated_at, completed_at, failed_at,
    duration_ms, total_input_tokens, total_output_tokens, total_cost_usd,
    model, persona_id, deleted_at, profile_ref, workspace_ref, created_at
)
SELECT
    id, account_id, thread_id, created_by_user_id, status, next_event_seq,
    parent_run_id, NULL, status_updated_at, completed_at, failed_at,
    duration_ms, total_input_tokens, total_output_tokens, total_cost_usd,
    model, persona_id, deleted_at, profile_ref, workspace_ref, created_at
FROM runs;

DROP TABLE runs;
ALTER TABLE runs_new RENAME TO runs;

CREATE INDEX ix_runs_org_id ON runs(account_id);
CREATE INDEX ix_runs_thread_id ON runs(thread_id);

PRAGMA foreign_keys = ON;

-- +goose Down
PRAGMA foreign_keys = OFF;

CREATE TABLE runs_old (
    id                  TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id          TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    thread_id           TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    created_by_user_id  TEXT REFERENCES users(id) ON DELETE SET NULL,
    status              TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'completed', 'failed', 'cancelled', 'cancelling')),
    next_event_seq      INTEGER NOT NULL DEFAULT 1,
    parent_run_id       TEXT REFERENCES runs(id) ON DELETE SET NULL,
    status_updated_at   TEXT,
    completed_at        TEXT,
    failed_at           TEXT,
    duration_ms         INTEGER,
    total_input_tokens  INTEGER,
    total_output_tokens INTEGER,
    total_cost_usd      TEXT,
    model               TEXT,
    persona_id          TEXT,
    deleted_at          TEXT,
    profile_ref         TEXT,
    workspace_ref       TEXT,
    created_at          TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT INTO runs_old (
    id, account_id, thread_id, created_by_user_id, status, next_event_seq,
    parent_run_id, status_updated_at, completed_at, failed_at,
    duration_ms, total_input_tokens, total_output_tokens, total_cost_usd,
    model, persona_id, deleted_at, profile_ref, workspace_ref, created_at
)
SELECT
    id, account_id, thread_id, created_by_user_id,
    CASE WHEN status = 'interrupted' THEN 'failed' ELSE status END,
    next_event_seq, parent_run_id, status_updated_at, completed_at, failed_at,
    duration_ms, total_input_tokens, total_output_tokens, total_cost_usd,
    model, persona_id, deleted_at, profile_ref, workspace_ref, created_at
FROM runs;

DROP TABLE runs;
ALTER TABLE runs_old RENAME TO runs;

CREATE INDEX ix_runs_org_id ON runs(account_id);
CREATE INDEX ix_runs_thread_id ON runs(thread_id);

PRAGMA foreign_keys = ON;
