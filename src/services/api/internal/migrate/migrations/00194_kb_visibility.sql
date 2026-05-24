-- +goose Up
-- +goose StatementBegin
ALTER TABLE knowledge_bases
    ADD COLUMN visibility TEXT NOT NULL DEFAULT 'workspace_member';

ALTER TABLE knowledge_bases
    ADD CONSTRAINT knowledge_bases_visibility_check
    CHECK (visibility IN ('workspace_member', 'private'));

ALTER TABLE knowledge_bases
    ADD CONSTRAINT knowledge_bases_exam_mode_requires_course
    CHECK (
        integration_mode = 'standalone'
        OR (integration_mode = 'exam' AND exam_course_id IS NOT NULL AND exam_course_id <> '')
    );
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE knowledge_bases
    DROP CONSTRAINT IF EXISTS knowledge_bases_exam_mode_requires_course;
ALTER TABLE knowledge_bases
    DROP CONSTRAINT IF EXISTS knowledge_bases_visibility_check;
ALTER TABLE knowledge_bases
    DROP COLUMN IF EXISTS visibility;
-- +goose StatementEnd
