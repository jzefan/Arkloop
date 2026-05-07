-- +goose Up
ALTER TABLE thread_context_replacements ADD COLUMN start_context_seq INTEGER;
ALTER TABLE thread_context_replacements ADD COLUMN end_context_seq INTEGER;

UPDATE thread_context_replacements
   SET start_context_seq = start_thread_seq
 WHERE start_context_seq IS NULL;

UPDATE thread_context_replacements
   SET end_context_seq = end_thread_seq
 WHERE end_context_seq IS NULL;

CREATE INDEX IF NOT EXISTS idx_thread_context_replacements_thread_active_context
    ON thread_context_replacements (
        thread_id,
        start_context_seq,
        end_context_seq,
        layer DESC,
        created_at DESC
    )
    WHERE superseded_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_thread_context_replacements_account_thread_id
    ON thread_context_replacements (account_id, thread_id, id);

CREATE TABLE IF NOT EXISTS thread_context_atoms (
    id                       TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id               TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    thread_id                TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    atom_seq                 INTEGER NOT NULL,
    atom_kind                TEXT NOT NULL,
    role                     TEXT NOT NULL,
    source_message_start_seq INTEGER NOT NULL,
    source_message_end_seq   INTEGER NOT NULL,
    payload_text             TEXT NOT NULL DEFAULT '',
    payload_json             TEXT NOT NULL DEFAULT '{}',
    metadata_json            TEXT NOT NULL DEFAULT '{}',
    created_at               TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK (source_message_start_seq <= source_message_end_seq),
    CHECK (atom_kind IN ('user_text_atom', 'assistant_text_atom', 'tool_episode_atom')),
    UNIQUE (thread_id, atom_seq),
    UNIQUE (account_id, thread_id, id)
);

CREATE INDEX IF NOT EXISTS idx_thread_context_atoms_thread_atom_seq
    ON thread_context_atoms (thread_id, atom_seq);

CREATE TABLE IF NOT EXISTS thread_context_chunks (
    id            TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id    TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    thread_id     TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    atom_id       TEXT NOT NULL,
    chunk_seq     INTEGER NOT NULL,
    context_seq   INTEGER NOT NULL,
    chunk_kind    TEXT NOT NULL DEFAULT 'payload',
    payload_text  TEXT NOT NULL DEFAULT '',
    payload_json  TEXT NOT NULL DEFAULT '{}',
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK (chunk_seq > 0 AND context_seq > 0),
    UNIQUE (thread_id, context_seq),
    UNIQUE (atom_id, chunk_seq),
    UNIQUE (account_id, thread_id, id),
    FOREIGN KEY (account_id, thread_id, atom_id)
        REFERENCES thread_context_atoms(account_id, thread_id, id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_thread_context_chunks_thread_atom_chunk
    ON thread_context_chunks (thread_id, atom_id, chunk_seq);

CREATE INDEX IF NOT EXISTS idx_thread_context_chunks_thread_context_seq
    ON thread_context_chunks (thread_id, context_seq);

CREATE TABLE IF NOT EXISTS replacement_supersession_edges (
    id                        TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id                TEXT NOT NULL,
    thread_id                 TEXT NOT NULL,
    replacement_id            TEXT NOT NULL,
    superseded_replacement_id TEXT NULL,
    superseded_chunk_id       TEXT NULL,
    created_at                TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK (
        (superseded_replacement_id IS NOT NULL AND superseded_chunk_id IS NULL) OR
        (superseded_replacement_id IS NULL AND superseded_chunk_id IS NOT NULL)
    ),
    CHECK (superseded_replacement_id IS NULL OR superseded_replacement_id <> replacement_id),
    FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE,
    FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE CASCADE,
    FOREIGN KEY (account_id, thread_id, replacement_id)
        REFERENCES thread_context_replacements(account_id, thread_id, id)
        ON DELETE CASCADE,
    FOREIGN KEY (account_id, thread_id, superseded_replacement_id)
        REFERENCES thread_context_replacements(account_id, thread_id, id)
        ON DELETE CASCADE,
    FOREIGN KEY (account_id, thread_id, superseded_chunk_id)
        REFERENCES thread_context_chunks(account_id, thread_id, id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_replacement_supersession_edges_replacement
    ON replacement_supersession_edges (replacement_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_replacement_supersession_edges_thread
    ON replacement_supersession_edges (thread_id, created_at DESC);

-- +goose Down
PRAGMA foreign_keys = OFF;

DROP INDEX IF EXISTS idx_replacement_supersession_edges_thread;
DROP INDEX IF EXISTS idx_replacement_supersession_edges_replacement;
DROP TABLE IF EXISTS replacement_supersession_edges;

DROP INDEX IF EXISTS idx_thread_context_chunks_thread_context_seq;
DROP INDEX IF EXISTS idx_thread_context_chunks_thread_atom_chunk;
DROP TABLE IF EXISTS thread_context_chunks;

DROP INDEX IF EXISTS idx_thread_context_atoms_thread_atom_seq;
DROP TABLE IF EXISTS thread_context_atoms;

DROP INDEX IF EXISTS uq_thread_context_replacements_account_thread_id;
DROP INDEX IF EXISTS idx_thread_context_replacements_thread_active_context;
DROP INDEX IF EXISTS idx_thread_context_replacements_thread_created;
DROP INDEX IF EXISTS idx_thread_context_replacements_thread_active;

ALTER TABLE thread_context_replacements RENAME TO thread_context_replacements_new;

CREATE TABLE thread_context_replacements (
    id               TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    account_id       TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    thread_id        TEXT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    start_thread_seq INTEGER NOT NULL,
    end_thread_seq   INTEGER NOT NULL,
    summary_text     TEXT NOT NULL,
    layer            INTEGER NOT NULL DEFAULT 1,
    metadata_json    TEXT NOT NULL DEFAULT '{}',
    superseded_at    TEXT NULL,
    created_at       TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK (start_thread_seq <= end_thread_seq)
);

INSERT INTO thread_context_replacements (
    id, account_id, thread_id, start_thread_seq, end_thread_seq,
    summary_text, layer, metadata_json, superseded_at, created_at
)
SELECT
    id, account_id, thread_id, start_thread_seq, end_thread_seq,
    summary_text, layer, metadata_json, superseded_at, created_at
FROM thread_context_replacements_new;

DROP TABLE thread_context_replacements_new;

CREATE INDEX IF NOT EXISTS idx_thread_context_replacements_thread_active
    ON thread_context_replacements(thread_id, start_thread_seq, end_thread_seq, layer DESC, created_at DESC)
    WHERE superseded_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_thread_context_replacements_thread_created
    ON thread_context_replacements(thread_id, created_at DESC);

PRAGMA foreign_keys = ON;
