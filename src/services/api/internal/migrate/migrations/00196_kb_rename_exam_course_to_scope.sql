-- +goose Up
-- +goose StatementBegin
ALTER TABLE knowledge_bases
    RENAME COLUMN exam_course_id TO exam_scope_id;

-- Drop and recreate the check constraint with new column name
ALTER TABLE knowledge_bases
    DROP CONSTRAINT IF EXISTS knowledge_bases_exam_mode_requires_course;

ALTER TABLE knowledge_bases
    ADD CONSTRAINT knowledge_bases_exam_mode_requires_scope
    CHECK (
        integration_mode = 'standalone'
        OR (integration_mode = 'exam' AND exam_scope_id IS NOT NULL AND exam_scope_id <> '')
    );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE knowledge_bases
    DROP CONSTRAINT IF EXISTS knowledge_bases_exam_mode_requires_scope;

ALTER TABLE knowledge_bases
    RENAME COLUMN exam_scope_id TO exam_course_id;

ALTER TABLE knowledge_bases
    ADD CONSTRAINT knowledge_bases_exam_mode_requires_course
    CHECK (
        integration_mode = 'standalone'
        OR (integration_mode = 'exam' AND exam_course_id IS NOT NULL AND exam_course_id <> '')
    );
-- +goose StatementEnd
