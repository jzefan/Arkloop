-- +goose Up
ALTER TABLE thread_context_replacements
    ADD COLUMN IF NOT EXISTS start_context_seq BIGINT,
    ADD COLUMN IF NOT EXISTS end_context_seq BIGINT;

UPDATE thread_context_replacements
   SET start_context_seq = start_thread_seq
 WHERE start_context_seq IS NULL;

UPDATE thread_context_replacements
   SET end_context_seq = end_thread_seq
 WHERE end_context_seq IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
          FROM pg_constraint
         WHERE conname = 'chk_thread_context_replacements_context_range'
    ) THEN
        ALTER TABLE thread_context_replacements
            ADD CONSTRAINT chk_thread_context_replacements_context_range
            CHECK (
                COALESCE(start_context_seq, start_thread_seq) <= COALESCE(end_context_seq, end_thread_seq)
            );
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_thread_context_replacements_thread_active_context
    ON thread_context_replacements (
        thread_id,
        COALESCE(start_context_seq, start_thread_seq),
        COALESCE(end_context_seq, end_thread_seq),
        layer DESC,
        created_at DESC
    )
    WHERE superseded_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_thread_context_replacements_account_thread_id
    ON thread_context_replacements (account_id, thread_id, id);

CREATE TABLE IF NOT EXISTS thread_context_atoms (
    id                         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id                 UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    thread_id                  UUID NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    atom_seq                   BIGINT NOT NULL,
    atom_kind                  TEXT NOT NULL,
    role                       TEXT NOT NULL,
    source_message_start_seq   BIGINT NOT NULL,
    source_message_end_seq     BIGINT NOT NULL,
    payload_text               TEXT NOT NULL DEFAULT '',
    payload_json               JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata_json              JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_thread_context_atoms_seq_range CHECK (source_message_start_seq <= source_message_end_seq),
    CONSTRAINT chk_thread_context_atoms_kind CHECK (
        atom_kind IN ('user_text_atom', 'assistant_text_atom', 'tool_episode_atom')
    ),
    CONSTRAINT uq_thread_context_atoms_thread_atom_seq UNIQUE (thread_id, atom_seq),
    CONSTRAINT uq_thread_context_atoms_account_thread_id UNIQUE (account_id, thread_id, id)
);

CREATE INDEX IF NOT EXISTS idx_thread_context_atoms_thread_atom_seq
    ON thread_context_atoms (thread_id, atom_seq);

CREATE TABLE IF NOT EXISTS thread_context_chunks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    thread_id       UUID NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    atom_id         UUID NOT NULL,
    chunk_seq       BIGINT NOT NULL,
    context_seq     BIGINT NOT NULL,
    chunk_kind      TEXT NOT NULL DEFAULT 'payload',
    payload_text    TEXT NOT NULL DEFAULT '',
    payload_json    JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata_json   JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_thread_context_chunks_seq_positive CHECK (chunk_seq > 0 AND context_seq > 0),
    CONSTRAINT uq_thread_context_chunks_thread_context_seq UNIQUE (thread_id, context_seq),
    CONSTRAINT uq_thread_context_chunks_atom_chunk_seq UNIQUE (atom_id, chunk_seq),
    CONSTRAINT uq_thread_context_chunks_account_thread_id UNIQUE (account_id, thread_id, id),
    CONSTRAINT fk_thread_context_chunks_atom
        FOREIGN KEY (account_id, thread_id, atom_id)
        REFERENCES thread_context_atoms(account_id, thread_id, id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_thread_context_chunks_thread_atom_chunk
    ON thread_context_chunks (thread_id, atom_id, chunk_seq);

CREATE INDEX IF NOT EXISTS idx_thread_context_chunks_thread_context_seq
    ON thread_context_chunks (thread_id, context_seq);

CREATE TABLE IF NOT EXISTS replacement_supersession_edges (
    id                        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id                UUID NOT NULL,
    thread_id                 UUID NOT NULL,
    replacement_id            UUID NOT NULL,
    superseded_replacement_id UUID NULL,
    superseded_chunk_id       UUID NULL,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_replacement_supersession_edges_target
        CHECK (num_nonnulls(superseded_replacement_id, superseded_chunk_id) = 1),
    CONSTRAINT chk_replacement_supersession_edges_no_self
        CHECK (superseded_replacement_id IS NULL OR superseded_replacement_id <> replacement_id),
    CONSTRAINT fk_replacement_supersession_edges_account
        FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE,
    CONSTRAINT fk_replacement_supersession_edges_thread
        FOREIGN KEY (thread_id) REFERENCES threads(id) ON DELETE CASCADE,
    CONSTRAINT fk_replacement_supersession_edges_replacement
        FOREIGN KEY (account_id, thread_id, replacement_id)
        REFERENCES thread_context_replacements(account_id, thread_id, id)
        ON DELETE CASCADE,
    CONSTRAINT fk_replacement_supersession_edges_superseded_replacement
        FOREIGN KEY (account_id, thread_id, superseded_replacement_id)
        REFERENCES thread_context_replacements(account_id, thread_id, id)
        ON DELETE CASCADE,
    CONSTRAINT fk_replacement_supersession_edges_superseded_chunk
        FOREIGN KEY (account_id, thread_id, superseded_chunk_id)
        REFERENCES thread_context_chunks(account_id, thread_id, id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_replacement_supersession_edges_replacement
    ON replacement_supersession_edges (replacement_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_replacement_supersession_edges_thread
    ON replacement_supersession_edges (thread_id, created_at DESC);

-- +goose Down
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

ALTER TABLE thread_context_replacements
    DROP CONSTRAINT IF EXISTS chk_thread_context_replacements_context_range;

ALTER TABLE thread_context_replacements
    DROP COLUMN IF EXISTS end_context_seq;

ALTER TABLE thread_context_replacements
    DROP COLUMN IF EXISTS start_context_seq;
