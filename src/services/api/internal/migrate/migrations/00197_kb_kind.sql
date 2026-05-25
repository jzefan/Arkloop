-- +goose Up
-- +goose StatementBegin

-- M1.2-bridge follow-up: distinguish user-built KBs from the system
-- paper-building bank ("组卷题库"). The system bank is account-level
-- (one per account, regardless of workspace), so callers can find it
-- by (account_id, kb_kind='system_paper_bank') without needing a
-- workspace_ref. The kb_kind column also lets the admin KB list hide
-- the system bank by default.

ALTER TABLE knowledge_bases
    ADD COLUMN kb_kind TEXT NOT NULL DEFAULT 'user';

ALTER TABLE knowledge_bases
    ADD CONSTRAINT knowledge_bases_kb_kind_check
    CHECK (kb_kind IN ('user', 'system_paper_bank'));

-- Mark any existing "组卷题库" rows so the new uniqueness rule and the
-- worker's ensurePaperBankKB lookup find them in place.
UPDATE knowledge_bases
SET    kb_kind = 'system_paper_bank'
WHERE  name = '组卷题库';

CREATE INDEX knowledge_bases_kb_kind_idx
    ON knowledge_bases(account_id, kb_kind);

-- Enforce one system bank per account. Existing accounts with a single
-- "组卷题库" pass; if any account ended up with multiple banks (one per
-- workspace) before this migration, this index will fail loudly so an
-- operator can dedupe before retrying.
CREATE UNIQUE INDEX knowledge_bases_one_system_bank_per_account
    ON knowledge_bases(account_id)
    WHERE kb_kind = 'system_paper_bank';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS knowledge_bases_one_system_bank_per_account;
DROP INDEX IF EXISTS knowledge_bases_kb_kind_idx;

ALTER TABLE knowledge_bases
    DROP CONSTRAINT IF EXISTS knowledge_bases_kb_kind_check;

ALTER TABLE knowledge_bases
    DROP COLUMN IF EXISTS kb_kind;

-- +goose StatementEnd
