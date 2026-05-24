-- +goose Up
-- +goose StatementBegin

-- M2a-prereq A of book-kb-rag: complete the schema for knowledge points,
-- many-to-many document associations, standalone-mode questions, and
-- generated papers. 00193 laid down placeholder knowledge_points / m2m
-- tables; this migration rebuilds them with the PRD-spec shape (parent_id
-- ON DELETE SET NULL, m2m PK includes kb_id, indexes on every FK) and
-- adds the two new standalone-only tables.

-- Rebuild kb_knowledge_points with parent_id ON DELETE SET NULL.
-- Placeholder rows from 00193 are not used yet, so drop+recreate is safe.
DROP TABLE IF EXISTS kb_document_knowledge_points;
DROP TABLE IF EXISTS kb_knowledge_points;

CREATE TABLE kb_knowledge_points (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id                    UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    name                     TEXT NOT NULL,
    parent_id                UUID REFERENCES kb_knowledge_points(id) ON DELETE SET NULL,
    exam_knowledge_point_id  TEXT,
    sort_order               INTEGER NOT NULL DEFAULT 0,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX kb_knowledge_points_kb_idx ON kb_knowledge_points(kb_id);
CREATE INDEX kb_knowledge_points_parent_idx ON kb_knowledge_points(parent_id);

CREATE TABLE kb_document_knowledge_points (
    kb_id              UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    document_id        UUID NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
    knowledge_point_id UUID NOT NULL REFERENCES kb_knowledge_points(id) ON DELETE CASCADE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (kb_id, document_id, knowledge_point_id)
);
CREATE INDEX kb_document_knowledge_points_doc_idx ON kb_document_knowledge_points(document_id);
CREATE INDEX kb_document_knowledge_points_kp_idx ON kb_document_knowledge_points(knowledge_point_id);

CREATE TABLE kb_questions (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id                 UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    knowledge_point_id    UUID REFERENCES kb_knowledge_points(id) ON DELETE SET NULL,
    question_type         TEXT NOT NULL,
    difficulty            TEXT NOT NULL,
    stem                  TEXT NOT NULL,
    options_json          JSONB NOT NULL DEFAULT '[]'::jsonb,
    answer                TEXT NOT NULL,
    explanation           TEXT NOT NULL DEFAULT '',
    source_chunk_ids_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    source_snippets_json  JSONB NOT NULL DEFAULT '[]'::jsonb,
    quality_flag          TEXT NOT NULL DEFAULT 'draft',
    created_by            UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (question_type IN ('single_choice','multi_choice','fill_in','short_answer','essay')),
    CHECK (difficulty IN ('easy','medium','hard')),
    CHECK (quality_flag IN ('draft','accepted','needs_review'))
);
CREATE INDEX kb_questions_kb_idx ON kb_questions(kb_id);
CREATE INDEX kb_questions_kp_idx ON kb_questions(knowledge_point_id);
CREATE INDEX kb_questions_type_difficulty_idx ON kb_questions(question_type, difficulty);

CREATE TABLE kb_papers (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id             UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    name              TEXT NOT NULL,
    spec_json         JSONB NOT NULL DEFAULT '{}'::jsonb,
    seed              BIGINT NOT NULL DEFAULT 0,
    question_ids_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    markdown          TEXT NOT NULL DEFAULT '',
    created_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX kb_papers_kb_idx ON kb_papers(kb_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS kb_papers_kb_idx;
DROP TABLE IF EXISTS kb_papers;

DROP INDEX IF EXISTS kb_questions_type_difficulty_idx;
DROP INDEX IF EXISTS kb_questions_kp_idx;
DROP INDEX IF EXISTS kb_questions_kb_idx;
DROP TABLE IF EXISTS kb_questions;

DROP INDEX IF EXISTS kb_document_knowledge_points_kp_idx;
DROP INDEX IF EXISTS kb_document_knowledge_points_doc_idx;
DROP TABLE IF EXISTS kb_document_knowledge_points;

DROP INDEX IF EXISTS kb_knowledge_points_parent_idx;
DROP INDEX IF EXISTS kb_knowledge_points_kb_idx;
DROP TABLE IF EXISTS kb_knowledge_points;

-- Restore the placeholder shapes from 00193 so down-migration leaves the
-- schema as it was after 00194.
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
-- +goose StatementEnd
