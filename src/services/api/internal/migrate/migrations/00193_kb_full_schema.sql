-- +goose Up
-- M1.0 of book-kb-rag: rebuild kb_chunks under a proper schema with FK
-- to knowledge_bases and kb_documents. Drops M0's flat table and creates
-- the real shape. Also lays down placeholder tables for knowledge points.

DROP TABLE IF EXISTS kb_chunks;

CREATE TABLE knowledge_bases (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_ref    TEXT NOT NULL,
    account_id       UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name             TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    integration_mode TEXT NOT NULL DEFAULT 'standalone',
    exam_course_id   TEXT,
    created_by       UUID REFERENCES users(id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_ref, name),
    CHECK (integration_mode IN ('standalone', 'exam'))
);
CREATE INDEX knowledge_bases_workspace_idx ON knowledge_bases(workspace_ref);
CREATE INDEX knowledge_bases_account_idx ON knowledge_bases(account_id);

CREATE TABLE kb_documents (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id             UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    original_filename TEXT NOT NULL,
    mime_type         TEXT NOT NULL,
    blob_sha256       TEXT NOT NULL,
    size_bytes        BIGINT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'queued',
    error_message     TEXT NOT NULL DEFAULT '',
    parse_meta_json   JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by        UUID REFERENCES users(id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (status IN ('queued','parsing','chunking','embedding','upserting','ready','failed'))
);
CREATE INDEX kb_documents_kb_idx ON kb_documents(kb_id);
CREATE INDEX kb_documents_status_idx ON kb_documents(status);

CREATE TABLE kb_chunks (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id         UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    document_id   UUID NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
    ordinal       INTEGER NOT NULL,
    heading_path  TEXT[] NOT NULL DEFAULT '{}',
    chunk_type    TEXT NOT NULL DEFAULT 'paragraph',
    text          TEXT NOT NULL,
    token_count   INTEGER NOT NULL,
    embedding     vector(1024) NOT NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (kb_id, document_id, ordinal),
    CHECK (chunk_type IN ('paragraph','heading','image','table','formula'))
);
CREATE INDEX kb_chunks_kb_idx ON kb_chunks(kb_id);
CREATE INDEX kb_chunks_document_idx ON kb_chunks(document_id);
CREATE INDEX kb_chunks_embedding_hnsw_idx
    ON kb_chunks
    USING hnsw (embedding vector_cosine_ops);

CREATE TABLE kb_knowledge_points (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id                   UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    name                    TEXT NOT NULL,
    parent_id               UUID REFERENCES kb_knowledge_points(id) ON DELETE CASCADE,
    exam_knowledge_point_id TEXT,
    sort_order              INTEGER NOT NULL DEFAULT 0,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX kb_knowledge_points_kb_idx ON kb_knowledge_points(kb_id);

CREATE TABLE kb_document_knowledge_points (
    kb_id              UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    document_id        UUID NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
    knowledge_point_id UUID NOT NULL REFERENCES kb_knowledge_points(id) ON DELETE CASCADE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (document_id, knowledge_point_id)
);

-- +goose Down
DROP TABLE IF EXISTS kb_document_knowledge_points;
DROP TABLE IF EXISTS kb_knowledge_points;
DROP INDEX IF EXISTS kb_chunks_embedding_hnsw_idx;
DROP INDEX IF EXISTS kb_chunks_document_idx;
DROP INDEX IF EXISTS kb_chunks_kb_idx;
DROP TABLE IF EXISTS kb_chunks;
DROP INDEX IF EXISTS kb_documents_status_idx;
DROP INDEX IF EXISTS kb_documents_kb_idx;
DROP TABLE IF EXISTS kb_documents;
DROP INDEX IF EXISTS knowledge_bases_account_idx;
DROP INDEX IF EXISTS knowledge_bases_workspace_idx;
DROP TABLE IF EXISTS knowledge_bases;
