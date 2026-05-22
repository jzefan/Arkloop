-- +goose Up

-- M0 of book-kb-rag: minimal schema to validate the chunker -> embedder ->
-- pgvector -> retrieval pipeline end-to-end. M1 will introduce
-- knowledge_bases, kb_documents, etc. and rebuild this table with
-- foreign keys; that migration will copy/transform existing rows.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE kb_chunks (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_name       TEXT         NOT NULL,
    document_ref  TEXT         NOT NULL,
    ordinal       INTEGER      NOT NULL,
    text          TEXT         NOT NULL,
    token_count   INTEGER      NOT NULL,
    embedding     vector(1024) NOT NULL,
    metadata_json JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),

    UNIQUE (kb_name, document_ref, ordinal)
);

CREATE INDEX kb_chunks_kb_name_idx ON kb_chunks (kb_name);

-- hnsw with cosine ops. ef_construction and m are pgvector defaults;
-- M0 data volume is small so we don't tune these.
CREATE INDEX kb_chunks_embedding_hnsw_idx
    ON kb_chunks
    USING hnsw (embedding vector_cosine_ops);

-- +goose Down

DROP INDEX IF EXISTS kb_chunks_embedding_hnsw_idx;
DROP INDEX IF EXISTS kb_chunks_kb_name_idx;
DROP TABLE IF EXISTS kb_chunks;
-- Note: we do NOT drop the vector extension on rollback; other migrations
-- in the future may depend on it.
