# Book-KB-RAG M1.0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver KB infrastructure end-to-end on `.txt` input: a real teacher can create a KB in console-lite, upload a `.txt`, see it processed through worker queue, and search it via `book-tutor-agent` persona — with workspace-level isolation.

**Architecture:** Adds three Postgres tables (`knowledge_bases`, `kb_documents`, `kb_chunks` rebuilt with FK; placeholder `kb_knowledge_points` / `kb_document_knowledge_points`) on top of M0. Introduces `shared/bookparser` parser interface (`.txt` only) and upgrades `shared/bookchunker.Chunk` to consume `ParsedDoc` — this is a breaking change to the M0 chunker signature. New `kb_ingest` worker job type reuses existing `JobQueue.EnqueueRun` with `runID=uuid.Nil` (same pattern as `webhook.deliver`). New `kbapi` HTTP package serves 9 REST endpoints with workspace-membership auth. New `kb_search` worker tool + minimal `book-tutor-agent` persona. New console-lite "知识库" page with list/create/detail/upload/poll/search-debug.

**Tech Stack:** Go 1.26, pgvector, pgx/v5, react-router-dom 7, React 19, Tailwind 4, Vite 8. Existing helpers used: `httpkit.ResolveActor`, `shared/workspaceblob`, `JobQueue.EnqueueRun`, existing webhook delivery handler pattern.

**Reference spec:** `docs/superpowers/specs/2026-05-21-book-kb-rag-m1-decomposition-design.md`

**Out of scope (do NOT implement):** PDF / DOCX parsing, OCR, image/table/formula handling, `kb_knowledge_points` writes, `kb_questions` / `kb_papers` tables, RAG question generation, paper composition, exam-system integration, document batch upload, KB rename, chunk-level browsing, SSE/WebSocket status push, multi-user write conflict resolution.

---

## Task 0: Coordinate with M0 — freeze + tag + branch

**Why:** M0 (committed by Codex) introduces `bookchunker.Chunk(text, opts)`. Task 4 will break that signature to `Chunk(doc ParsedDoc, opts)`. Without coordinating, both branches will fight over the same files. Tag a frozen M0 commit so M1.0 has a clear merge base.

**Files:**
- No code changes; coordination + tagging only.

**Steps:**

- [ ] **Step 1: Verify M0 has fully landed on `main`**

```bash
git checkout main
git pull
# Confirm these files exist and have content:
test -f src/services/shared/bookchunker/chunker.go && echo "chunker present"
test -f src/services/shared/embedding/doubao.go && echo "doubao present"
test -f src/services/api/internal/data/kb_chunks_repo.go && echo "kb_chunks_repo present"
test -f src/services/api/internal/migrate/migrations/00192_kb_chunks.sql && echo "00192 present"
test -f src/services/api/internal/http/kbdebugapi/handler.go && echo "kbdebugapi present"
```

Expected: all 5 lines print.

If any line is missing, stop. M0 isn't complete — sync with whoever owns the Codex implementation before proceeding.

- [ ] **Step 2: Tag the frozen M0 commit**

```bash
git tag -a m0-frozen -m "Book-KB-RAG M0 complete; baseline for M1.0 work"
git push origin m0-frozen
```

- [ ] **Step 3: Create M1.0 branch from the tag**

```bash
git checkout -b feature/book-kb-m1.0 m0-frozen
```

All subsequent tasks happen on this branch.

- [ ] **Step 4: Sanity-build the M0 state once before changing anything**

```bash
cd src/services/api && go build ./...
cd ../shared && go test ./bookchunker/... ./embedding/...
```

Expected: build clean, tests pass.

- [ ] **Step 5: No commit needed**

Tag and branch creation are not commits. Move on.

---

## Task 1: Migration 00193 — full KB schema

**Files:**
- Create: `src/services/api/internal/migrate/migrations/00193_kb_full_schema.sql`

**Steps:**

- [ ] **Step 1: Read the M0 migration to confirm vector dim used**

```bash
grep "vector(" src/services/api/internal/migrate/migrations/00192_kb_chunks.sql
```

Note the dimension number (call it `<DOUBAO_DIM>`). It will be reused verbatim in this migration.

- [ ] **Step 2: Write the migration**

Create `src/services/api/internal/migrate/migrations/00193_kb_full_schema.sql`:

```sql
-- +goose Up
-- M1.0 of book-kb-rag: rebuild kb_chunks under a proper schema with FK
-- to knowledge_bases and kb_documents. Drops M0's flat table and creates
-- the real shape. Also lays down placeholder tables for M1.2 (knowledge
-- points + document↔kp join) so M1.0 schema is the final target shape.

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
    embedding     vector(<DOUBAO_DIM>) NOT NULL,
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
```

Replace `<DOUBAO_DIM>` with the integer from Step 1.

- [ ] **Step 3: Apply migration against a fresh pgvector pg16 container**

```bash
docker run --rm -d --name pgv-m1 -e POSTGRES_PASSWORD=test -p 5599:5432 pgvector/pgvector:pg16
sleep 3
cd src/services/api
ARKLOOP_DATABASE_URL=postgres://postgres:test@localhost:5599/postgres?sslmode=disable \
  go run ./cmd/migrate up
docker exec pgv-m1 psql -U postgres -c "\d knowledge_bases" -c "\d kb_documents" -c "\d kb_chunks" -c "\d kb_knowledge_points"
docker stop pgv-m1
```

Expected: all four `\d` outputs print table definitions; `embedding | vector(<DOUBAO_DIM>)` shown for `kb_chunks`.

- [ ] **Step 4: Commit**

```bash
git add src/services/api/internal/migrate/migrations/00193_kb_full_schema.sql
git commit -m "feat(api): migration 00193 — rebuild kb_chunks under full KB schema

Drop M0 flat kb_chunks. Add knowledge_bases (workspace-scoped),
kb_documents (status state machine), kb_chunks with FK + hnsw,
kb_knowledge_points + join table as placeholders for M1.2."
```

---

## Task 2: Repository layer for knowledge_bases + kb_documents + rebuilt kb_chunks

**Files:**
- Create: `src/services/api/internal/data/knowledge_bases_repo.go`
- Create: `src/services/api/internal/data/kb_documents_repo.go`
- Modify: `src/services/api/internal/data/kb_chunks_repo.go` (rebuild for new schema)
- Modify: `src/services/api/internal/data/kb_chunks_repo_integration_test.go` (rewrite for new schema)
- Create: `src/services/api/internal/data/knowledge_bases_repo_integration_test.go`
- Create: `src/services/api/internal/data/kb_documents_repo_integration_test.go`

**Steps:**

- [ ] **Step 1: Write integration test for knowledge_bases_repo**

Create `src/services/api/internal/data/knowledge_bases_repo_integration_test.go`:

```go
//go:build !desktop

package data

import (
	"context"
	"testing"

	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"

	"github.com/google/uuid"
)

func setupKBRepo(t *testing.T) (*KnowledgeBasesRepository, *AccountRepository, context.Context) {
	t.Helper()
	db := testutil.SetupPostgresDatabase(t, "api_go_kb_bases")
	ctx := context.Background()
	if _, err := migrate.Up(ctx, db.DSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := NewPool(ctx, db.DSN, PoolLimits{MaxConns: 8})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	repo, err := NewKnowledgeBasesRepository(pool)
	if err != nil {
		t.Fatalf("kb repo: %v", err)
	}
	accountRepo, err := NewAccountRepository(pool)
	if err != nil {
		t.Fatalf("account repo: %v", err)
	}
	return repo, accountRepo, ctx
}

func TestKBCreateAndGet(t *testing.T) {
	repo, accounts, ctx := setupKBRepo(t)
	acc, err := accounts.Create(ctx, "alice", "Alice", "personal")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	kb, err := repo.Create(ctx, KBCreate{
		AccountID: acc.ID, WorkspaceRef: "ws-1", Name: "大学物理", Description: "test",
	})
	if err != nil {
		t.Fatalf("create kb: %v", err)
	}
	if kb.ID == uuid.Nil {
		t.Fatal("expected non-nil id")
	}
	got, err := repo.GetByID(ctx, kb.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v / %v", err, got)
	}
	if got.Name != "大学物理" || got.WorkspaceRef != "ws-1" {
		t.Errorf("mismatch: %+v", got)
	}
}

func TestKBListByWorkspace(t *testing.T) {
	repo, accounts, ctx := setupKBRepo(t)
	acc, _ := accounts.Create(ctx, "bob", "Bob", "personal")
	_, _ = repo.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "ws-a", Name: "k1"})
	_, _ = repo.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "ws-a", Name: "k2"})
	_, _ = repo.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "ws-b", Name: "k3"})

	got, err := repo.ListByWorkspace(ctx, acc.ID, "ws-a")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d, want 2", len(got))
	}
}

func TestKBUniqueNamePerWorkspace(t *testing.T) {
	repo, accounts, ctx := setupKBRepo(t)
	acc, _ := accounts.Create(ctx, "carol", "Carol", "personal")
	if _, err := repo.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "ws-x", Name: "dup"}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := repo.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "ws-x", Name: "dup"})
	if err == nil {
		t.Fatal("expected duplicate name error")
	}
}

func TestKBDelete(t *testing.T) {
	repo, accounts, ctx := setupKBRepo(t)
	acc, _ := accounts.Create(ctx, "dave", "Dave", "personal")
	kb, _ := repo.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "ws-1", Name: "to-delete"})
	if err := repo.Delete(ctx, kb.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ := repo.GetByID(ctx, kb.ID)
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}
```

- [ ] **Step 2: Implement knowledge_bases_repo**

Create `src/services/api/internal/data/knowledge_bases_repo.go`:

```go
package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// KnowledgeBase mirrors a knowledge_bases row.
type KnowledgeBase struct {
	ID              uuid.UUID
	WorkspaceRef    string
	AccountID       uuid.UUID
	Name            string
	Description     string
	IntegrationMode string
	ExamCourseID    *string
	CreatedBy       *uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DocumentCount   int // populated by ListByWorkspace; 0 from GetByID
}

// KBCreate is the input shape for Create.
type KBCreate struct {
	AccountID    uuid.UUID
	WorkspaceRef string
	Name         string
	Description  string
	CreatedBy    *uuid.UUID
}

// ErrKBNotFound signals a non-existent or already-deleted KB.
var ErrKBNotFound = errors.New("knowledge base not found")

// ErrKBDuplicateName signals a UNIQUE constraint violation on (workspace_ref, name).
var ErrKBDuplicateName = errors.New("knowledge base name already exists in this workspace")

// KnowledgeBasesRepository persists knowledge_bases rows.
type KnowledgeBasesRepository struct {
	pool DB
}

// NewKnowledgeBasesRepository constructs the repo.
func NewKnowledgeBasesRepository(pool DB) (*KnowledgeBasesRepository, error) {
	if pool == nil {
		return nil, fmt.Errorf("nil pool")
	}
	return &KnowledgeBasesRepository{pool: pool}, nil
}

// Create inserts a new knowledge_bases row.
func (r *KnowledgeBasesRepository) Create(ctx context.Context, in KBCreate) (*KnowledgeBase, error) {
	row := r.pool.QueryRow(ctx, `
INSERT INTO knowledge_bases (workspace_ref, account_id, name, description, created_by)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, workspace_ref, account_id, name, description, integration_mode, exam_course_id, created_by, created_at, updated_at`,
		in.WorkspaceRef, in.AccountID, in.Name, in.Description, in.CreatedBy)
	var kb KnowledgeBase
	if err := row.Scan(&kb.ID, &kb.WorkspaceRef, &kb.AccountID, &kb.Name, &kb.Description,
		&kb.IntegrationMode, &kb.ExamCourseID, &kb.CreatedBy, &kb.CreatedAt, &kb.UpdatedAt); err != nil {
		if isPGUniqueViolation(err) {
			return nil, ErrKBDuplicateName
		}
		return nil, fmt.Errorf("create kb: %w", err)
	}
	return &kb, nil
}

// GetByID returns the KB or (nil, nil) if absent.
func (r *KnowledgeBasesRepository) GetByID(ctx context.Context, id uuid.UUID) (*KnowledgeBase, error) {
	row := r.pool.QueryRow(ctx, `
SELECT id, workspace_ref, account_id, name, description, integration_mode, exam_course_id, created_by, created_at, updated_at
FROM   knowledge_bases WHERE id = $1`, id)
	var kb KnowledgeBase
	if err := row.Scan(&kb.ID, &kb.WorkspaceRef, &kb.AccountID, &kb.Name, &kb.Description,
		&kb.IntegrationMode, &kb.ExamCourseID, &kb.CreatedBy, &kb.CreatedAt, &kb.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &kb, nil
}

// ListByWorkspace returns all KBs in (account_id, workspace_ref), with document_count populated.
func (r *KnowledgeBasesRepository) ListByWorkspace(ctx context.Context, accountID uuid.UUID, workspaceRef string) ([]KnowledgeBase, error) {
	rows, err := r.pool.Query(ctx, `
SELECT kb.id, kb.workspace_ref, kb.account_id, kb.name, kb.description,
       kb.integration_mode, kb.exam_course_id, kb.created_by, kb.created_at, kb.updated_at,
       COALESCE((SELECT COUNT(*) FROM kb_documents d WHERE d.kb_id = kb.id), 0) AS document_count
FROM   knowledge_bases kb
WHERE  kb.account_id = $1 AND kb.workspace_ref = $2
ORDER  BY kb.created_at DESC`, accountID, workspaceRef)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KnowledgeBase
	for rows.Next() {
		var kb KnowledgeBase
		if err := rows.Scan(&kb.ID, &kb.WorkspaceRef, &kb.AccountID, &kb.Name, &kb.Description,
			&kb.IntegrationMode, &kb.ExamCourseID, &kb.CreatedBy, &kb.CreatedAt, &kb.UpdatedAt, &kb.DocumentCount); err != nil {
			return nil, err
		}
		out = append(out, kb)
	}
	return out, rows.Err()
}

// Delete removes a KB; chunks and documents cascade.
func (r *KnowledgeBasesRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM knowledge_bases WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrKBNotFound
	}
	return nil
}

// isPGUniqueViolation detects pgx unique constraint errors without depending on a vendored pgerrcode lib.
func isPGUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}
```

- [ ] **Step 3: Run KB repo tests**

```bash
docker run --rm -d --name pgv-m1 -e POSTGRES_PASSWORD=test -p 5599:5432 pgvector/pgvector:pg16
sleep 3
cd src/services/api
ARKLOOP_RUN_INTEGRATION_TESTS=1 \
ARKLOOP_TEST_DATABASE_DSN=postgres://postgres:test@localhost:5599/postgres?sslmode=disable \
  go test ./internal/data/ -run KB -v
```

Expected: TestKBCreateAndGet / TestKBListByWorkspace / TestKBUniqueNamePerWorkspace / TestKBDelete all PASS.

- [ ] **Step 4: Write kb_documents_repo test**

Create `src/services/api/internal/data/kb_documents_repo_integration_test.go`:

```go
//go:build !desktop

package data

import (
	"context"
	"testing"

	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"
)

func setupDocsRepo(t *testing.T) (*KBDocumentsRepository, *KnowledgeBasesRepository, *AccountRepository, context.Context) {
	t.Helper()
	db := testutil.SetupPostgresDatabase(t, "api_go_kb_docs")
	ctx := context.Background()
	if _, err := migrate.Up(ctx, db.DSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := NewPool(ctx, db.DSN, PoolLimits{MaxConns: 8})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	docs, _ := NewKBDocumentsRepository(pool)
	kbRepo, _ := NewKnowledgeBasesRepository(pool)
	accountRepo, _ := NewAccountRepository(pool)
	return docs, kbRepo, accountRepo, ctx
}

func TestDocCreateAndStatusTransitions(t *testing.T) {
	docs, kbs, accts, ctx := setupDocsRepo(t)
	acc, _ := accts.Create(ctx, "u", "U", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "w", Name: "n"})
	doc, err := docs.Create(ctx, DocCreate{
		KBID: kb.ID, OriginalFilename: "x.txt", MimeType: "text/plain",
		BlobSHA256: "abc", SizeBytes: 100,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if doc.Status != "queued" {
		t.Errorf("initial status: got %q, want queued", doc.Status)
	}
	if err := docs.UpdateStatus(ctx, doc.ID, "parsing", "", nil); err != nil {
		t.Fatalf("update parsing: %v", err)
	}
	got, _ := docs.GetByID(ctx, doc.ID)
	if got.Status != "parsing" {
		t.Errorf("status after update: got %q, want parsing", got.Status)
	}
	if err := docs.UpdateStatus(ctx, doc.ID, "failed", "OCR error", nil); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	got, _ = docs.GetByID(ctx, doc.ID)
	if got.ErrorMessage != "OCR error" {
		t.Errorf("error_message: got %q", got.ErrorMessage)
	}
}

func TestDocListByKB(t *testing.T) {
	docs, kbs, accts, ctx := setupDocsRepo(t)
	acc, _ := accts.Create(ctx, "u", "U", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "w", Name: "n"})
	for i, name := range []string{"a.txt", "b.txt", "c.txt"} {
		_, err := docs.Create(ctx, DocCreate{KBID: kb.ID, OriginalFilename: name, MimeType: "text/plain", BlobSHA256: "sha", SizeBytes: int64(i + 1)})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
	}
	out, err := docs.ListByKB(ctx, kb.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out) != 3 {
		t.Errorf("got %d docs, want 3", len(out))
	}
}

func TestDocDeleteCascadesChunks(t *testing.T) {
	// chunks repo is exercised in Task 2 Step 6 — here we verify the FK cascade only.
	docs, kbs, accts, ctx := setupDocsRepo(t)
	acc, _ := accts.Create(ctx, "u", "U", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "w", Name: "n"})
	doc, _ := docs.Create(ctx, DocCreate{KBID: kb.ID, OriginalFilename: "a.txt", MimeType: "text/plain", BlobSHA256: "s", SizeBytes: 1})
	if err := docs.Delete(ctx, doc.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ := docs.GetByID(ctx, doc.ID)
	if got != nil {
		t.Errorf("expected nil after delete, got %+v", got)
	}
}
```

- [ ] **Step 5: Implement kb_documents_repo**

Create `src/services/api/internal/data/kb_documents_repo.go`:

```go
package data

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type KBDocument struct {
	ID               uuid.UUID
	KBID             uuid.UUID
	OriginalFilename string
	MimeType         string
	BlobSHA256       string
	SizeBytes        int64
	Status           string
	ErrorMessage     string
	ParseMeta        map[string]any
	CreatedBy        *uuid.UUID
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type DocCreate struct {
	KBID             uuid.UUID
	OriginalFilename string
	MimeType         string
	BlobSHA256       string
	SizeBytes        int64
	CreatedBy        *uuid.UUID
}

var ErrDocNotFound = errors.New("kb document not found")

type KBDocumentsRepository struct {
	pool DB
}

func NewKBDocumentsRepository(pool DB) (*KBDocumentsRepository, error) {
	if pool == nil {
		return nil, fmt.Errorf("nil pool")
	}
	return &KBDocumentsRepository{pool: pool}, nil
}

func (r *KBDocumentsRepository) Create(ctx context.Context, in DocCreate) (*KBDocument, error) {
	row := r.pool.QueryRow(ctx, `
INSERT INTO kb_documents (kb_id, original_filename, mime_type, blob_sha256, size_bytes, created_by)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, kb_id, original_filename, mime_type, blob_sha256, size_bytes, status, error_message, parse_meta_json, created_by, created_at, updated_at`,
		in.KBID, in.OriginalFilename, in.MimeType, in.BlobSHA256, in.SizeBytes, in.CreatedBy)
	return scanDoc(row)
}

func (r *KBDocumentsRepository) GetByID(ctx context.Context, id uuid.UUID) (*KBDocument, error) {
	row := r.pool.QueryRow(ctx, `
SELECT id, kb_id, original_filename, mime_type, blob_sha256, size_bytes, status, error_message, parse_meta_json, created_by, created_at, updated_at
FROM   kb_documents WHERE id = $1`, id)
	doc, err := scanDoc(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return doc, err
}

func (r *KBDocumentsRepository) ListByKB(ctx context.Context, kbID uuid.UUID) ([]KBDocument, error) {
	rows, err := r.pool.Query(ctx, `
SELECT id, kb_id, original_filename, mime_type, blob_sha256, size_bytes, status, error_message, parse_meta_json, created_by, created_at, updated_at
FROM   kb_documents WHERE kb_id = $1 ORDER BY created_at DESC`, kbID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KBDocument
	for rows.Next() {
		doc, err := scanDocFromRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *doc)
	}
	return out, rows.Err()
}

// UpdateStatus moves a doc through the state machine and optionally records error_message + parse_meta.
// parseMeta is nil-safe; pass nil to leave parse_meta_json unchanged.
func (r *KBDocumentsRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, errorMessage string, parseMeta map[string]any) error {
	if parseMeta != nil {
		buf, err := json.Marshal(parseMeta)
		if err != nil {
			return err
		}
		_, err = r.pool.Exec(ctx, `
UPDATE kb_documents SET status = $2, error_message = $3, parse_meta_json = $4, updated_at = now()
WHERE  id = $1`, id, status, errorMessage, buf)
		return err
	}
	_, err := r.pool.Exec(ctx, `
UPDATE kb_documents SET status = $2, error_message = $3, updated_at = now()
WHERE  id = $1`, id, status, errorMessage)
	return err
}

func (r *KBDocumentsRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM kb_documents WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrDocNotFound
	}
	return nil
}

func scanDoc(row pgx.Row) (*KBDocument, error) {
	var d KBDocument
	var metaRaw []byte
	err := row.Scan(&d.ID, &d.KBID, &d.OriginalFilename, &d.MimeType, &d.BlobSHA256, &d.SizeBytes,
		&d.Status, &d.ErrorMessage, &metaRaw, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(metaRaw) > 0 {
		_ = json.Unmarshal(metaRaw, &d.ParseMeta)
	}
	return &d, nil
}

func scanDocFromRows(rows pgx.Rows) (*KBDocument, error) {
	var d KBDocument
	var metaRaw []byte
	err := rows.Scan(&d.ID, &d.KBID, &d.OriginalFilename, &d.MimeType, &d.BlobSHA256, &d.SizeBytes,
		&d.Status, &d.ErrorMessage, &metaRaw, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if len(metaRaw) > 0 {
		_ = json.Unmarshal(metaRaw, &d.ParseMeta)
	}
	return &d, nil
}
```

- [ ] **Step 6: Rebuild kb_chunks_repo for new schema**

Replace `src/services/api/internal/data/kb_chunks_repo.go` entirely:

```go
package data

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// KBChunksRepository persists and searches chunks against pgvector under
// the M1.0 schema (FK to knowledge_bases + kb_documents).
type KBChunksRepository struct {
	pool DB
	dim  int
}

type KBChunkUpsert struct {
	KBID        uuid.UUID
	DocumentID  uuid.UUID
	Ordinal     int
	HeadingPath []string
	ChunkType   string
	Text        string
	TokenCount  int
	Embedding   []float32
}

type KBChunkHit struct {
	ID          uuid.UUID
	KBID        uuid.UUID
	DocumentID  uuid.UUID
	DocumentRef string // joined from kb_documents.original_filename
	Ordinal     int
	HeadingPath []string
	ChunkType   string
	Text        string
	TokenCount  int
	Score       float32
}

func NewKBChunksRepository(pool DB) (*KBChunksRepository, error) {
	if pool == nil {
		return nil, fmt.Errorf("nil pool")
	}
	var dim int
	row := pool.QueryRow(context.Background(), `
SELECT a.atttypmod
FROM   pg_attribute a
JOIN   pg_class c ON c.oid = a.attrelid
WHERE  c.relname = 'kb_chunks' AND a.attname = 'embedding'`)
	if err := row.Scan(&dim); err != nil {
		return nil, fmt.Errorf("probe pgvector dim: %w", err)
	}
	if dim <= 0 {
		return nil, fmt.Errorf("invalid pgvector dim from catalog: %d", dim)
	}
	return &KBChunksRepository{pool: pool, dim: dim}, nil
}

func (r *KBChunksRepository) Dim() int { return r.dim }

func (r *KBChunksRepository) Upsert(ctx context.Context, rows []KBChunkUpsert) error {
	if len(rows) == 0 {
		return nil
	}
	for i, row := range rows {
		if len(row.Embedding) != r.dim {
			return fmt.Errorf("row %d: embedding dim %d != table dim %d", i, len(row.Embedding), r.dim)
		}
	}
	// Single batched insert via pgx Batch.
	// (One Exec per row to keep error attribution simple; ~5000 chunks/doc is fine.)
	for _, row := range rows {
		_, err := r.pool.Exec(ctx, `
INSERT INTO kb_chunks (kb_id, document_id, ordinal, heading_path, chunk_type, text, token_count, embedding, metadata_json)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, '{}'::jsonb)
ON CONFLICT (kb_id, document_id, ordinal) DO UPDATE SET
    heading_path = EXCLUDED.heading_path,
    chunk_type   = EXCLUDED.chunk_type,
    text         = EXCLUDED.text,
    token_count  = EXCLUDED.token_count,
    embedding    = EXCLUDED.embedding`,
			row.KBID, row.DocumentID, row.Ordinal, row.HeadingPath, row.ChunkType,
			row.Text, row.TokenCount, vecLiteral(row.Embedding))
		if err != nil {
			return fmt.Errorf("upsert kb=%s doc=%s ord=%d: %w", row.KBID, row.DocumentID, row.Ordinal, err)
		}
	}
	return nil
}

// Search returns up to k chunks in kbID ordered by cosine similarity desc.
// DocumentRef is joined from kb_documents.original_filename.
func (r *KBChunksRepository) Search(ctx context.Context, kbID uuid.UUID, query []float32, k int) ([]KBChunkHit, error) {
	if len(query) != r.dim {
		return nil, fmt.Errorf("query dim %d != table dim %d", len(query), r.dim)
	}
	if k <= 0 {
		k = 8
	}
	rows, err := r.pool.Query(ctx, `
SELECT c.id, c.kb_id, c.document_id, d.original_filename, c.ordinal, c.heading_path, c.chunk_type, c.text, c.token_count,
       1 - (c.embedding <=> $2) AS score
FROM   kb_chunks c
JOIN   kb_documents d ON d.id = c.document_id
WHERE  c.kb_id = $1
ORDER  BY c.embedding <=> $2
LIMIT  $3`, kbID, vecLiteral(query), k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KBChunkHit
	for rows.Next() {
		var h KBChunkHit
		if err := rows.Scan(&h.ID, &h.KBID, &h.DocumentID, &h.DocumentRef, &h.Ordinal,
			&h.HeadingPath, &h.ChunkType, &h.Text, &h.TokenCount, &h.Score); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func vecLiteral(v []float32) string {
	var sb strings.Builder
	sb.Grow(len(v) * 6)
	sb.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%g", x)
	}
	sb.WriteByte(']')
	return sb.String()
}
```

- [ ] **Step 7: Rewrite kb_chunks_repo integration test for new schema**

Replace `src/services/api/internal/data/kb_chunks_repo_integration_test.go`:

```go
//go:build !desktop

package data

import (
	"context"
	"math"
	"testing"

	"arkloop/services/api/internal/migrate"
	"arkloop/services/api/internal/testutil"
)

func setupChunksRepo(t *testing.T) (*KBChunksRepository, *KBDocumentsRepository, *KnowledgeBasesRepository, *AccountRepository, context.Context) {
	t.Helper()
	db := testutil.SetupPostgresDatabase(t, "api_go_kb_chunks")
	ctx := context.Background()
	if _, err := migrate.Up(ctx, db.DSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := NewPool(ctx, db.DSN, PoolLimits{MaxConns: 8})
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	chunks, _ := NewKBChunksRepository(pool)
	docs, _ := NewKBDocumentsRepository(pool)
	kbs, _ := NewKnowledgeBasesRepository(pool)
	accts, _ := NewAccountRepository(pool)
	return chunks, docs, kbs, accts, ctx
}

func makeVec(dim, pos int) []float32 {
	v := make([]float32, dim)
	v[pos] = 1.0
	return v
}

func TestKBChunksUpsertAndSearch(t *testing.T) {
	chunks, docs, kbs, accts, ctx := setupChunksRepo(t)
	acc, _ := accts.Create(ctx, "u", "U", "personal")
	kb, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "w", Name: "kb"})
	doc, _ := docs.Create(ctx, DocCreate{KBID: kb.ID, OriginalFilename: "physics.txt", MimeType: "text/plain", BlobSHA256: "sha", SizeBytes: 1})

	dim := chunks.Dim()
	in := []KBChunkUpsert{
		{KBID: kb.ID, DocumentID: doc.ID, Ordinal: 0, ChunkType: "paragraph", Text: "光的干涉", TokenCount: 1, Embedding: makeVec(dim, 0)},
		{KBID: kb.ID, DocumentID: doc.ID, Ordinal: 1, ChunkType: "paragraph", Text: "电磁感应", TokenCount: 1, Embedding: makeVec(dim, 1)},
	}
	if err := chunks.Upsert(ctx, in); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	hits, err := chunks.Search(ctx, kb.ID, makeVec(dim, 1), 2)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits: %d", len(hits))
	}
	if hits[0].Ordinal != 1 || hits[0].DocumentRef != "physics.txt" {
		t.Errorf("top hit: %+v", hits[0])
	}
	if math.Abs(float64(hits[0].Score-1.0)) > 0.01 {
		t.Errorf("top score: %f", hits[0].Score)
	}
}

func TestKBChunksIsolatedByKB(t *testing.T) {
	chunks, docs, kbs, accts, ctx := setupChunksRepo(t)
	acc, _ := accts.Create(ctx, "u", "U", "personal")
	kbA, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "w", Name: "kb-a"})
	kbB, _ := kbs.Create(ctx, KBCreate{AccountID: acc.ID, WorkspaceRef: "w", Name: "kb-b"})
	docA, _ := docs.Create(ctx, DocCreate{KBID: kbA.ID, OriginalFilename: "a.txt", MimeType: "text/plain", BlobSHA256: "a", SizeBytes: 1})
	docB, _ := docs.Create(ctx, DocCreate{KBID: kbB.ID, OriginalFilename: "b.txt", MimeType: "text/plain", BlobSHA256: "b", SizeBytes: 1})

	dim := chunks.Dim()
	_ = chunks.Upsert(ctx, []KBChunkUpsert{
		{KBID: kbA.ID, DocumentID: docA.ID, Ordinal: 0, ChunkType: "paragraph", Text: "A", TokenCount: 1, Embedding: makeVec(dim, 0)},
		{KBID: kbB.ID, DocumentID: docB.ID, Ordinal: 0, ChunkType: "paragraph", Text: "B", TokenCount: 1, Embedding: makeVec(dim, 0)},
	})
	hits, _ := chunks.Search(ctx, kbA.ID, makeVec(dim, 0), 5)
	if len(hits) != 1 || hits[0].Text != "A" {
		t.Errorf("isolation failed: %+v", hits)
	}
}
```

- [ ] **Step 8: Run all data tests**

```bash
cd src/services/api
ARKLOOP_RUN_INTEGRATION_TESTS=1 \
ARKLOOP_TEST_DATABASE_DSN=postgres://postgres:test@localhost:5599/postgres?sslmode=disable \
  go test ./internal/data/ -run "KB|Doc" -v
```

Expected: all PASS. Stop the docker container when done: `docker stop pgv-m1`.

- [ ] **Step 9: Commit**

```bash
git add src/services/api/internal/data/knowledge_bases_repo.go \
        src/services/api/internal/data/knowledge_bases_repo_integration_test.go \
        src/services/api/internal/data/kb_documents_repo.go \
        src/services/api/internal/data/kb_documents_repo_integration_test.go \
        src/services/api/internal/data/kb_chunks_repo.go \
        src/services/api/internal/data/kb_chunks_repo_integration_test.go
git commit -m "feat(api): KB / doc / chunks repositories under M1.0 schema

KnowledgeBasesRepository: CRUD with workspace scoping, UNIQUE
(workspace_ref, name). KBDocumentsRepository: CRUD + status state
machine with parse_meta. KBChunksRepository: rebuilt for FK-bearing
schema, search JOINs kb_documents.original_filename for DocumentRef."
```

---

## Task 3: shared/bookparser package — interface + .txt implementation

**Files:**
- Create: `src/services/shared/bookparser/types.go`
- Create: `src/services/shared/bookparser/parser.go`
- Create: `src/services/shared/bookparser/text.go`
- Create: `src/services/shared/bookparser/text_test.go`

**Steps:**

- [ ] **Step 1: Write failing tests**

Create `src/services/shared/bookparser/text_test.go`:

```go
package bookparser

import (
	"strings"
	"testing"
)

func TestTextParserSplitsByBlankLines(t *testing.T) {
	in := "段落 A 内容。\n\n段落 B 内容。\n\n段落 C。"
	doc, err := ParseText(strings.NewReader(in), "text/plain")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("blocks: got %d, want 3", len(doc.Blocks))
	}
	for i, b := range doc.Blocks {
		if b.Type != BlockParagraph {
			t.Errorf("block %d type: got %q, want paragraph", i, b.Type)
		}
		if b.HeadingInferred {
			t.Errorf("block %d: HeadingInferred should be false", i)
		}
	}
}

func TestTextParserSkipsEmptyParagraphs(t *testing.T) {
	in := "\n\nA\n\n\n\n\n\nB\n\n"
	doc, _ := ParseText(strings.NewReader(in), "text/plain")
	if len(doc.Blocks) != 2 {
		t.Errorf("got %d blocks, want 2", len(doc.Blocks))
	}
}

func TestTextParserHandlesCRLF(t *testing.T) {
	in := "A\r\n\r\nB"
	doc, _ := ParseText(strings.NewReader(in), "text/plain")
	if len(doc.Blocks) != 2 || doc.Blocks[0].Text != "A" || doc.Blocks[1].Text != "B" {
		t.Errorf("CRLF handling: %+v", doc.Blocks)
	}
}

func TestTextParserMetadataPopulated(t *testing.T) {
	in := "hello world"
	doc, _ := ParseText(strings.NewReader(in), "text/plain")
	if doc.Meta["source_mime"] != "text/plain" {
		t.Errorf("source_mime: %v", doc.Meta["source_mime"])
	}
}

func TestParserDispatchUnsupportedMime(t *testing.T) {
	p := NewTextOnlyParser()
	_, err := p.Parse(nil, strings.NewReader("x"), "application/pdf")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("expected unsupported-mime error, got %v", err)
	}
}

func TestParserDispatchAcceptsTextPlainAndMarkdown(t *testing.T) {
	p := NewTextOnlyParser()
	for _, mime := range []string{"text/plain", "text/markdown", "text/markdown; charset=utf-8"} {
		if _, err := p.Parse(nil, strings.NewReader("A\n\nB"), mime); err != nil {
			t.Errorf("mime %s rejected: %v", mime, err)
		}
	}
}
```

- [ ] **Step 2: Run tests, expect compile failure**

```bash
cd src/services/shared
go test ./bookparser/...
```

Expected: undefined symbols.

- [ ] **Step 3: Implement types + parser interface**

Create `src/services/shared/bookparser/types.go`:

```go
// Package bookparser converts uploaded document bytes into a structured
// ParsedDoc that the chunker can consume. M1.0 ships text/plain and
// text/markdown only; PDF/DOCX implementations land in M1.1.
package bookparser

import (
	"context"
	"errors"
	"io"
)

// BlockType enumerates the kinds of content blocks a parser can emit.
type BlockType string

const (
	BlockParagraph BlockType = "paragraph"
	BlockHeading   BlockType = "heading"
	BlockImage     BlockType = "image"   // M1.1+
	BlockTable     BlockType = "table"   // M1.1+
	BlockFormula   BlockType = "formula" // M1.1+
)

// Block is one piece of source content.
type Block struct {
	Type              BlockType
	Text              string
	HeadingPath       []string
	HeadingInferred   bool
	HeadingConfidence float32
	Metadata          map[string]any
}

// ParsedDoc is the parser output.
type ParsedDoc struct {
	Blocks []Block
	Meta   map[string]any
}

// Parser turns raw bytes of a given mime into a ParsedDoc.
type Parser interface {
	Parse(ctx context.Context, r io.Reader, mime string) (ParsedDoc, error)
}

// ErrUnsupportedMime is returned when a parser receives a mime it can't handle.
var ErrUnsupportedMime = errors.New("bookparser: unsupported mime type")
```

- [ ] **Step 4: Implement TextOnlyParser dispatch**

Create `src/services/shared/bookparser/parser.go`:

```go
package bookparser

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// TextOnlyParser handles text/plain and text/markdown. M1.1 introduces
// MultiFormatParser that wraps several backends and dispatches by mime.
type TextOnlyParser struct{}

// NewTextOnlyParser returns the M1.0 default parser.
func NewTextOnlyParser() *TextOnlyParser { return &TextOnlyParser{} }

// Parse implements Parser. Only text/plain and text/markdown (with optional
// charset params) are accepted; anything else returns ErrUnsupportedMime.
func (p *TextOnlyParser) Parse(ctx context.Context, r io.Reader, mime string) (ParsedDoc, error) {
	base := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	switch base {
	case "text/plain", "text/markdown":
		return ParseText(r, mime)
	default:
		return ParsedDoc{}, fmt.Errorf("%w: %s", ErrUnsupportedMime, mime)
	}
}
```

- [ ] **Step 5: Implement ParseText**

Create `src/services/shared/bookparser/text.go`:

```go
package bookparser

import (
	"io"
	"strings"
)

// ParseText reads UTF-8 text from r and splits on blank lines.
// Each non-empty paragraph becomes a BlockParagraph.
func ParseText(r io.Reader, mime string) (ParsedDoc, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return ParsedDoc{}, err
	}
	normalized := strings.ReplaceAll(string(raw), "\r\n", "\n")
	parts := strings.Split(normalized, "\n\n")
	blocks := make([]Block, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t == "" {
			continue
		}
		blocks = append(blocks, Block{Type: BlockParagraph, Text: t})
	}
	return ParsedDoc{
		Blocks: blocks,
		Meta: map[string]any{
			"source_mime": strings.SplitN(mime, ";", 2)[0],
			"byte_size":   len(raw),
		},
	}, nil
}
```

- [ ] **Step 6: Run tests, expect pass**

```bash
cd src/services/shared
go test ./bookparser/... -v
```

Expected: all 6 tests PASS.

- [ ] **Step 7: Commit**

```bash
git add src/services/shared/bookparser
git commit -m "feat(shared): add bookparser pkg with .txt-only implementation

Parser interface (Parse) + TextOnlyParser that handles text/plain and
text/markdown. Splits on blank lines; carries HeadingInferred /
HeadingConfidence fields on Block so M1.1 PDF parser can plug in
without changing chunker contract."
```

---

---

## Task 4: bookchunker — breaking change from text to ParsedDoc

**Why:** M0 chunker takes `string`. M1.0 must consume `ParsedDoc` so images/tables/formulas (M1.1+) can be independent chunks. This is the single most invasive change in M1.0; it touches every M0 caller.

**Files:**
- Modify: `src/services/shared/bookchunker/chunker.go` (full rewrite of `Chunk` signature)
- Modify: `src/services/shared/bookchunker/chunker_test.go` (rewrite tests)
- Modify: `src/services/api/internal/kbingest/service.go` (call bookparser before bookchunker)
- Modify: `src/services/api/internal/kbingest/e2e_integration_test.go` (works with new pipeline)

**Steps:**

- [ ] **Step 1: Update chunker tests for ParsedDoc input**

Replace `src/services/shared/bookchunker/chunker_test.go`:

```go
package bookchunker

import (
	"strings"
	"testing"

	"arkloop/services/shared/bookparser"
)

const sampleParagraph = "光的干涉是指两列或多列频率相同的光波相遇时发生的现象，其结果是某些位置振幅相互加强，另一些位置振幅相互削弱，从而形成稳定的明暗条纹。1801 年托马斯·杨通过双缝实验首次证明了光具有波动性，实验中双缝间距、缝至屏的距离以及光的波长共同决定了条纹间距，可以由公式 Δy = λL/d 计算。"

func longParsedDoc() bookparser.ParsedDoc {
	blocks := []bookparser.Block{}
	for i := 0; i < 6; i++ {
		blocks = append(blocks, bookparser.Block{
			Type: bookparser.BlockParagraph,
			Text: sampleParagraph,
		})
	}
	return bookparser.ParsedDoc{Blocks: blocks}
}

func TestChunkLongParsedDocProducesMultipleChunks(t *testing.T) {
	chunks, err := Chunk(longParsedDoc(), DefaultOptions())
	if err != nil {
		t.Fatalf("chunk: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected >= 2 chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if c.TokenCount > DefaultOptions().MaxTokens {
			t.Errorf("chunk %d exceeds MaxTokens: %d > %d", i, c.TokenCount, DefaultOptions().MaxTokens)
		}
		if c.Ordinal != i {
			t.Errorf("chunk %d ordinal: got %d", i, c.Ordinal)
		}
		if c.ChunkType != string(bookparser.BlockParagraph) {
			t.Errorf("chunk %d type: got %q", i, c.ChunkType)
		}
	}
}

func TestChunkShortParsedDocReturnsSingleChunk(t *testing.T) {
	doc := bookparser.ParsedDoc{Blocks: []bookparser.Block{{Type: bookparser.BlockParagraph, Text: "短句。"}}}
	chunks, err := Chunk(doc, DefaultOptions())
	if err != nil {
		t.Fatalf("chunk: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Text != "短句。" {
		t.Errorf("unexpected: %+v", chunks)
	}
}

func TestChunkEmptyDocReturnsNil(t *testing.T) {
	chunks, _ := Chunk(bookparser.ParsedDoc{}, DefaultOptions())
	if chunks != nil {
		t.Errorf("expected nil, got %+v", chunks)
	}
}

func TestChunkPreservesHeadingPathFromBlock(t *testing.T) {
	doc := bookparser.ParsedDoc{Blocks: []bookparser.Block{
		{Type: bookparser.BlockParagraph, Text: "正文 A", HeadingPath: []string{"第一章"}},
		{Type: bookparser.BlockParagraph, Text: "正文 B", HeadingPath: []string{"第一章", "1.2 节"}},
	}}
	chunks, _ := Chunk(doc, DefaultOptions())
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2", len(chunks))
	}
	if len(chunks[0].HeadingPath) != 1 || chunks[0].HeadingPath[0] != "第一章" {
		t.Errorf("chunk 0 heading: %+v", chunks[0].HeadingPath)
	}
	if len(chunks[1].HeadingPath) != 2 || chunks[1].HeadingPath[1] != "1.2 节" {
		t.Errorf("chunk 1 heading: %+v", chunks[1].HeadingPath)
	}
}

func TestChunkDeterministic(t *testing.T) {
	doc := longParsedDoc()
	a, _ := Chunk(doc, DefaultOptions())
	b, _ := Chunk(doc, DefaultOptions())
	if len(a) != len(b) {
		t.Fatalf("len %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Text != b[i].Text {
			t.Fatalf("chunk %d differs", i)
		}
	}
}

func TestChunkHandlesUnknownBlockTypeAsParagraph(t *testing.T) {
	// Image/table/formula blocks become independent chunks; we cover the
	// "single isolated paragraph carrying special metadata" case here.
	doc := bookparser.ParsedDoc{Blocks: []bookparser.Block{
		{Type: bookparser.BlockTable, Text: "| A | B |\n|---|---|\n| 1 | 2 |"},
	}}
	chunks, _ := Chunk(doc, DefaultOptions())
	if len(chunks) != 1 {
		t.Fatalf("got %d, want 1", len(chunks))
	}
	if chunks[0].ChunkType != string(bookparser.BlockTable) {
		t.Errorf("expected table type, got %q", chunks[0].ChunkType)
	}
}

// Silence unused-imports linter for the strings package since we keep
// reuse it in future test cases as the parser grows.
var _ = strings.TrimSpace
```

- [ ] **Step 2: Rewrite the chunker implementation**

Replace `src/services/shared/bookchunker/chunker.go`:

```go
// Package bookchunker splits a ParsedDoc into overlapping chunks suitable
// for embedding-based retrieval. Each input Block becomes one or more
// output Chunks; special blocks (image/table/formula) are always emitted
// as a single chunk each, preserving their type and metadata.
//
// Pure function: no I/O, no goroutines, deterministic given the same input.
package bookchunker

import (
	"fmt"
	"strings"

	"arkloop/services/shared/bookparser"

	"github.com/pkoukk/tiktoken-go"
)

// Chunk is one output unit. Ordinal is 0-based across the whole document.
type Chunk struct {
	Ordinal     int
	ChunkType   string
	Text        string
	HeadingPath []string
	TokenCount  int
}

type ChunkOptions struct {
	MinTokens     int
	MaxTokens     int
	OverlapTokens int
	Encoding      string
}

func DefaultOptions() ChunkOptions {
	return ChunkOptions{
		MinTokens:     256,
		MaxTokens:     512,
		OverlapTokens: 40,
		Encoding:      "cl100k_base",
	}
}

// Chunk transforms doc into a sequence of chunks per opts. Empty doc → nil.
func Chunk(doc bookparser.ParsedDoc, opts ChunkOptions) ([]Chunk, error) {
	if len(doc.Blocks) == 0 {
		return nil, nil
	}
	enc, err := tiktoken.GetEncoding(opts.Encoding)
	if err != nil {
		return nil, fmt.Errorf("load tiktoken encoding %q: %w", opts.Encoding, err)
	}

	var out []Chunk
	for _, block := range doc.Blocks {
		// Image, table, formula → independent single chunk regardless of size.
		if block.Type != bookparser.BlockParagraph && block.Type != bookparser.BlockHeading {
			tokens := enc.Encode(block.Text, nil, nil)
			out = append(out, Chunk{
				Ordinal:     len(out),
				ChunkType:   string(block.Type),
				Text:        block.Text,
				HeadingPath: copyPath(block.HeadingPath),
				TokenCount:  len(tokens),
			})
			continue
		}
		// Paragraph / heading → token sliding window with overlap.
		tokens := enc.Encode(block.Text, nil, nil)
		if len(tokens) <= opts.MaxTokens {
			out = append(out, Chunk{
				Ordinal:     len(out),
				ChunkType:   string(block.Type),
				Text:        block.Text,
				HeadingPath: copyPath(block.HeadingPath),
				TokenCount:  len(tokens),
			})
			continue
		}
		pos := 0
		for pos < len(tokens) {
			end := pos + opts.MaxTokens
			if end > len(tokens) {
				end = len(tokens)
			}
			window := tokens[pos:end]
			out = append(out, Chunk{
				Ordinal:     len(out),
				ChunkType:   string(block.Type),
				Text:        enc.Decode(window),
				HeadingPath: copyPath(block.HeadingPath),
				TokenCount:  len(window),
			})
			if end == len(tokens) {
				break
			}
			step := opts.MaxTokens - opts.OverlapTokens
			if step < opts.MinTokens {
				step = opts.MinTokens
			}
			pos += step
		}
	}
	return out, nil
}

func copyPath(p []string) []string {
	if len(p) == 0 {
		return nil
	}
	cp := make([]string, len(p))
	copy(cp, p)
	return cp
}

// Used by future heading-aware logic; declared now to keep imports stable.
var _ = strings.Repeat
```

- [ ] **Step 3: Run chunker tests, expect pass**

```bash
cd src/services/shared
go test ./bookchunker/... -v
```

Expected: 6 tests PASS.

- [ ] **Step 4: Update kbingest.Service to call bookparser before bookchunker**

Replace `src/services/api/internal/kbingest/service.go`:

```go
package kbingest

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"arkloop/services/api/internal/data"
	"arkloop/services/api/internal/http/kbdebugapi"
	"arkloop/services/shared/bookchunker"
	"arkloop/services/shared/bookparser"
	"arkloop/services/shared/embedding"
)

type Service struct {
	embedder embedding.Embedder
	repo     *data.KBChunksRepository
	parser   bookparser.Parser
}

// New constructs a Service. The embedder's Dim() must match repo.Dim() —
// guards against pgvector(N) drift.
func New(embedder embedding.Embedder, repo *data.KBChunksRepository) (*Service, error) {
	if embedder.Dim() != repo.Dim() {
		return nil, fmt.Errorf("kbingest: embedder dim %d != repo dim %d", embedder.Dim(), repo.Dim())
	}
	return &Service{embedder: embedder, repo: repo, parser: bookparser.NewTextOnlyParser()}, nil
}

// Ingest reads filePath, parses, chunks, embeds, and upserts. M0 debug
// endpoint still calls this. M1.0+ wires the worker job to call the same
// shared packages directly so this method is M0-only.
func (s *Service) Ingest(ctx context.Context, filePath, kbName string) (int, error) {
	buf, err := os.ReadFile(filePath)
	if err != nil {
		return 0, fmt.Errorf("read file: %w", err)
	}
	mime := guessMimeFromExt(filePath)
	doc, err := s.parser.Parse(ctx, bytes.NewReader(buf), mime)
	if err != nil {
		return 0, fmt.Errorf("parse: %w", err)
	}
	chunks, err := bookchunker.Chunk(doc, bookchunker.DefaultOptions())
	if err != nil {
		return 0, fmt.Errorf("chunk: %w", err)
	}
	if len(chunks) == 0 {
		return 0, nil
	}
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}
	vecs, err := s.embedder.Embed(ctx, texts)
	if err != nil {
		return 0, fmt.Errorf("embed: %w", err)
	}
	// M0 schema used kb_name TEXT; M1.0 schema requires kb_id UUID. The
	// M0 debug ingest API is being deleted in Task 8. For now, refuse to
	// run against the new schema since kbName cannot map to a kb_id.
	_ = kbName
	_ = vecs
	return 0, fmt.Errorf("kbingest.Ingest is M0-only and is incompatible with the M1.0 kb_chunks schema; use the new REST endpoints instead")
}

// Search remains useful for the debug API; same incompatibility note.
func (s *Service) Search(ctx context.Context, kbName, query string, k int) ([]kbdebugapi.SearchHit, error) {
	_ = ctx
	_ = kbName
	_ = query
	_ = k
	return nil, fmt.Errorf("kbingest.Search is M0-only and incompatible with the M1.0 schema; the M0 _debug routes are being removed")
}

func guessMimeFromExt(p string) string {
	switch filepath.Ext(p) {
	case ".md":
		return "text/markdown"
	default:
		return "text/plain"
	}
}
```

> **Note:** This change deliberately makes the M0 `_debug` ingest/search routes return 500 errors. Task 8 deletes those routes entirely. Between Task 4 and Task 8 the routes are functionally broken — that's fine, M1.0 is not deployed mid-task.

- [ ] **Step 5: Delete the M0 end-to-end integration test (will be replaced in Task 14)**

```bash
rm src/services/api/internal/kbingest/e2e_integration_test.go
```

- [ ] **Step 6: Verify Go build still passes**

```bash
cd src/services/api && go build ./...
cd ../shared && go build ./...
```

Expected: clean build.

- [ ] **Step 7: Run all shared tests**

```bash
cd src/services/shared
go test ./bookchunker/... ./bookparser/... ./embedding/... -v
```

Expected: all PASS.

- [ ] **Step 8: Commit**

```bash
git add src/services/shared/bookchunker \
        src/services/api/internal/kbingest/service.go
git rm src/services/api/internal/kbingest/e2e_integration_test.go
git commit -m "feat(shared): bookchunker.Chunk takes ParsedDoc (breaking)

bookchunker.Chunk(text, opts) → Chunk(doc bookparser.ParsedDoc, opts).
Each block becomes one or more chunks; image/table/formula blocks are
emitted as single independent chunks regardless of size. HeadingPath
is carried from block to chunk.

kbingest.Service updated to call bookparser before bookchunker. M0
debug ingest/search returns a clear error pointing at the M1.0 REST
routes — debug routes themselves will be removed in Task 8. M0 E2E
test removed; M1.0 final task adds a replacement walkthrough."
```

---

## Task 5: Workspace auth helper for KB routes

**Files:**
- Create: `src/services/api/internal/http/kbapi/auth.go`
- Create: `src/services/api/internal/http/kbapi/auth_test.go`
- Create: `src/services/api/internal/http/kbapi/deps.go`

**Steps:**

- [ ] **Step 1: Read the existing AccountMembershipRepository surface**

```bash
grep -n "func.*AccountMembershipRepository" src/services/api/internal/data/*.go | head -10
grep -n "func.*WorkspaceRegistriesRepository" src/services/api/internal/data/*.go | head -10
```

Note the existing methods used to check `(account_id, user_id)` membership and `(account_id, workspace_ref)` workspace existence. These get reused, not re-implemented.

- [ ] **Step 2: Write deps struct**

Create `src/services/api/internal/http/kbapi/deps.go`:

```go
// Package kbapi serves M1.0 knowledge-base endpoints. Auth follows the
// shared pattern: httpkit.ResolveActor handles user→account, then each
// route checks workspace membership for the requested kb.workspace_ref.
package kbapi

import (
	"arkloop/services/api/internal/audit"
	"arkloop/services/api/internal/auth"
	"arkloop/services/api/internal/data"
	"arkloop/services/shared/embedding"
	"arkloop/services/shared/objectstore"

	"arkloop/services/worker/internal/queue"
)

type Deps struct {
	AuthService           *auth.Service
	AccountMembershipRepo *data.AccountMembershipRepository
	APIKeysRepo           *data.APIKeysRepository
	AuditWriter           *audit.Writer

	Pool                       data.DB
	KnowledgeBasesRepo         *data.KnowledgeBasesRepository
	KBDocumentsRepo            *data.KBDocumentsRepository
	KBChunksRepo               *data.KBChunksRepository
	WorkspaceRegistriesRepo    *data.WorkspaceRegistriesRepository

	BlobStore objectstore.Store
	Embedder  embedding.Embedder // for the search REST endpoint

	JobQueue queue.JobQueue // for enqueuing kb_ingest

	// Limits
	MaxUploadBytes int64 // 10 MB default
}
```

- [ ] **Step 3: Write workspace-access helper test**

Create `src/services/api/internal/http/kbapi/auth_test.go`:

```go
package kbapi

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// fakeMembershipChecker implements the small surface area auth.go uses.
// The integration with the real repos is exercised end-to-end in Task 14.
type fakeMembershipChecker struct {
	memberOf map[string]bool // key = "<accountID>/<workspaceRef>"
}

func (f *fakeMembershipChecker) IsWorkspaceMember(ctx context.Context, accountID uuid.UUID, workspaceRef string) (bool, error) {
	return f.memberOf[accountID.String()+"/"+workspaceRef], nil
}

func TestEnsureWorkspaceMemberAllows(t *testing.T) {
	acc := uuid.New()
	checker := &fakeMembershipChecker{memberOf: map[string]bool{acc.String() + "/ws-1": true}}
	if err := ensureWorkspaceMember(context.Background(), checker, acc, "ws-1"); err != nil {
		t.Errorf("unexpected denial: %v", err)
	}
}

func TestEnsureWorkspaceMemberDenies(t *testing.T) {
	acc := uuid.New()
	checker := &fakeMembershipChecker{memberOf: map[string]bool{}}
	err := ensureWorkspaceMember(context.Background(), checker, acc, "ws-2")
	if err == nil {
		t.Error("expected denial")
	}
}
```

- [ ] **Step 4: Implement workspace access helper**

Create `src/services/api/internal/http/kbapi/auth.go`:

```go
package kbapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// membershipChecker is the minimal interface auth.go consumes. The real
// implementation is a thin wrapper around WorkspaceRegistriesRepository +
// AccountMembershipRepository in Tasks 6/7 wiring; the test replaces it
// with a fake.
type membershipChecker interface {
	IsWorkspaceMember(ctx context.Context, accountID uuid.UUID, workspaceRef string) (bool, error)
}

var errNoWorkspaceAccess = errors.New("user has no access to this workspace")

// ensureWorkspaceMember returns nil if the actor's account is a member of
// the workspace; otherwise errNoWorkspaceAccess.
func ensureWorkspaceMember(ctx context.Context, c membershipChecker, accountID uuid.UUID, workspaceRef string) error {
	ok, err := c.IsWorkspaceMember(ctx, accountID, workspaceRef)
	if err != nil {
		return fmt.Errorf("workspace membership check: %w", err)
	}
	if !ok {
		return errNoWorkspaceAccess
	}
	return nil
}
```

- [ ] **Step 5: Run tests**

```bash
cd src/services/api
go test ./internal/http/kbapi/... -v
```

Expected: 2 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add src/services/api/internal/http/kbapi
git commit -m "feat(api): kbapi pkg with workspace membership auth helper

ensureWorkspaceMember + membershipChecker interface lets the routes
defer to a real WorkspaceRegistriesRepository / AccountMembershipRepository
combination at wiring time while keeping unit tests fast."
```

---

## Task 6: KB CRUD REST endpoints

**Files:**
- Create: `src/services/api/internal/http/kbapi/handler_kb.go`
- Create: `src/services/api/internal/http/kbapi/handler_kb_test.go`
- Create: `src/services/api/internal/http/kbapi/register.go`

**Steps:**

- [ ] **Step 1: Write handler tests using a fake deps fixture**

Create `src/services/api/internal/http/kbapi/handler_kb_test.go`:

```go
package kbapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
)

// fakeKBStore stands in for KnowledgeBasesRepository in unit-level handler tests.
type fakeKBStore struct {
	items map[uuid.UUID]*data.KnowledgeBase
}

func newFakeKBStore() *fakeKBStore { return &fakeKBStore{items: map[uuid.UUID]*data.KnowledgeBase{}} }

func (f *fakeKBStore) Create(ctx context.Context, in data.KBCreate) (*data.KnowledgeBase, error) {
	for _, kb := range f.items {
		if kb.WorkspaceRef == in.WorkspaceRef && kb.Name == in.Name {
			return nil, data.ErrKBDuplicateName
		}
	}
	kb := &data.KnowledgeBase{ID: uuid.New(), AccountID: in.AccountID, WorkspaceRef: in.WorkspaceRef, Name: in.Name, Description: in.Description, IntegrationMode: "standalone"}
	f.items[kb.ID] = kb
	return kb, nil
}

func (f *fakeKBStore) GetByID(ctx context.Context, id uuid.UUID) (*data.KnowledgeBase, error) {
	return f.items[id], nil
}

func (f *fakeKBStore) ListByWorkspace(ctx context.Context, accountID uuid.UUID, ws string) ([]data.KnowledgeBase, error) {
	var out []data.KnowledgeBase
	for _, kb := range f.items {
		if kb.AccountID == accountID && kb.WorkspaceRef == ws {
			out = append(out, *kb)
		}
	}
	return out, nil
}

func (f *fakeKBStore) Delete(ctx context.Context, id uuid.UUID) error {
	if _, ok := f.items[id]; !ok {
		return data.ErrKBNotFound
	}
	delete(f.items, id)
	return nil
}

type fakeMembership struct{ allow bool }

func (f *fakeMembership) IsWorkspaceMember(ctx context.Context, accountID uuid.UUID, ws string) (bool, error) {
	return f.allow, nil
}

func newHandlerCtx(allow bool) *handlerCtx {
	return &handlerCtx{
		kbStore:    newFakeKBStore(),
		membership: &fakeMembership{allow: allow},
	}
}

func TestCreateKBHappyPath(t *testing.T) {
	ctx := newHandlerCtx(true)
	body := strings.NewReader(`{"name":"my-kb","workspace_ref":"ws-1","description":"desc"}`)
	req := httptest.NewRequest("POST", "/v1/knowledge-bases", body)
	req = injectActor(req, uuid.New(), uuid.New())
	w := httptest.NewRecorder()
	handleCreateKB(ctx)(w, req)
	if w.Code != 201 {
		t.Fatalf("got %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		ID, Name, WorkspaceRef string
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Name != "my-kb" || resp.WorkspaceRef != "ws-1" {
		t.Errorf("got %+v", resp)
	}
}

func TestCreateKBRejectsNonMember(t *testing.T) {
	ctx := newHandlerCtx(false)
	body := strings.NewReader(`{"name":"x","workspace_ref":"ws-1"}`)
	req := httptest.NewRequest("POST", "/v1/knowledge-bases", body)
	req = injectActor(req, uuid.New(), uuid.New())
	w := httptest.NewRecorder()
	handleCreateKB(ctx)(w, req)
	if w.Code != 403 {
		t.Errorf("got %d", w.Code)
	}
}

func TestCreateKBRejectsBadJSON(t *testing.T) {
	ctx := newHandlerCtx(true)
	req := httptest.NewRequest("POST", "/v1/knowledge-bases", strings.NewReader("not json"))
	req = injectActor(req, uuid.New(), uuid.New())
	w := httptest.NewRecorder()
	handleCreateKB(ctx)(w, req)
	if w.Code != 400 {
		t.Errorf("got %d", w.Code)
	}
}

func TestCreateKBRejectsDuplicateName(t *testing.T) {
	ctx := newHandlerCtx(true)
	body1 := strings.NewReader(`{"name":"dup","workspace_ref":"ws"}`)
	body2 := strings.NewReader(`{"name":"dup","workspace_ref":"ws"}`)
	for i, body := range []*strings.Reader{body1, body2} {
		req := httptest.NewRequest("POST", "/v1/knowledge-bases", body)
		req = injectActor(req, uuid.New(), uuid.New())
		w := httptest.NewRecorder()
		handleCreateKB(ctx)(w, req)
		if i == 0 && w.Code != 201 {
			t.Fatalf("first should succeed, got %d", w.Code)
		}
		if i == 1 && w.Code != 409 {
			t.Errorf("second should 409, got %d", w.Code)
		}
	}
}

func TestListKBs(t *testing.T) {
	ctx := newHandlerCtx(true)
	acc := uuid.New()
	for _, name := range []string{"a", "b"} {
		_, _ = ctx.kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "ws", Name: name})
	}
	req := httptest.NewRequest("GET", "/v1/knowledge-bases?workspace_ref=ws", nil)
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleListKB(ctx)(w, req)
	if w.Code != 200 {
		t.Fatalf("got %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Items) != 2 {
		t.Errorf("got %d items, want 2", len(resp.Items))
	}
}

func TestDeleteKB(t *testing.T) {
	ctx := newHandlerCtx(true)
	kb, _ := ctx.kbStore.Create(context.Background(), data.KBCreate{AccountID: uuid.New(), WorkspaceRef: "ws", Name: "k"})
	req := httptest.NewRequest("DELETE", "/v1/knowledge-bases/"+kb.ID.String(), nil)
	req.SetPathValue("id", kb.ID.String())
	req = injectActor(req, kb.AccountID, uuid.New())
	w := httptest.NewRecorder()
	handleDeleteKB(ctx)(w, req)
	if w.Code != 204 {
		t.Errorf("got %d", w.Code)
	}
	if got, _ := ctx.kbStore.GetByID(context.Background(), kb.ID); got != nil {
		t.Error("expected nil after delete")
	}
}
```

- [ ] **Step 2: Implement handlers**

Create `src/services/api/internal/http/kbapi/handler_kb.go`:

```go
package kbapi

import (
	"context"
	"encoding/json"
	"errors"
	nethttp "net/http"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
)

// handlerCtx is the per-request collaborator bundle. Production wiring
// passes real repos; tests pass fakes. The interfaces are intentionally
// narrow to keep tests focused.
type handlerCtx struct {
	kbStore    kbStore
	membership membershipChecker
}

type kbStore interface {
	Create(ctx context.Context, in data.KBCreate) (*data.KnowledgeBase, error)
	GetByID(ctx context.Context, id uuid.UUID) (*data.KnowledgeBase, error)
	ListByWorkspace(ctx context.Context, accountID uuid.UUID, ws string) ([]data.KnowledgeBase, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// actor is the resolved caller identity. Real handler wraps httpkit.ResolveActor;
// tests use injectActor() to set this directly.
type actor struct {
	AccountID uuid.UUID
	UserID    uuid.UUID
}

type actorKey struct{}

func injectActor(r *nethttp.Request, accountID, userID uuid.UUID) *nethttp.Request {
	return r.WithContext(context.WithValue(r.Context(), actorKey{}, actor{AccountID: accountID, UserID: userID}))
}

func actorFromCtx(ctx context.Context) (actor, bool) {
	a, ok := ctx.Value(actorKey{}).(actor)
	return a, ok
}

func writeJSON(w nethttp.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w nethttp.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"code": code, "message": msg})
}

type createKBReq struct {
	Name         string `json:"name"`
	WorkspaceRef string `json:"workspace_ref"`
	Description  string `json:"description"`
}

func handleCreateKB(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, 401, "auth.unauthenticated", "unauthenticated")
			return
		}
		var req createKBReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, 400, "kb.invalid_json", "invalid json body")
			return
		}
		if req.Name == "" || req.WorkspaceRef == "" {
			writeErr(w, 400, "kb.missing_field", "name and workspace_ref are required")
			return
		}
		if err := ensureWorkspaceMember(r.Context(), h.membership, a.AccountID, req.WorkspaceRef); err != nil {
			writeErr(w, 403, "auth.no_workspace_access", "no access to this workspace")
			return
		}
		kb, err := h.kbStore.Create(r.Context(), data.KBCreate{
			AccountID: a.AccountID, WorkspaceRef: req.WorkspaceRef, Name: req.Name, Description: req.Description, CreatedBy: &a.UserID,
		})
		if err != nil {
			if errors.Is(err, data.ErrKBDuplicateName) {
				writeErr(w, 409, "kb.duplicate_name", "kb with this name already exists in this workspace")
				return
			}
			writeErr(w, 500, "internal.error", "failed to create kb")
			return
		}
		writeJSON(w, 201, map[string]any{
			"id":            kb.ID,
			"name":          kb.Name,
			"workspace_ref": kb.WorkspaceRef,
			"description":   kb.Description,
			"created_at":    kb.CreatedAt,
		})
	}
}

func handleListKB(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, 401, "auth.unauthenticated", "unauthenticated")
			return
		}
		ws := r.URL.Query().Get("workspace_ref")
		if ws == "" {
			writeErr(w, 400, "kb.missing_workspace_ref", "workspace_ref query param is required")
			return
		}
		if err := ensureWorkspaceMember(r.Context(), h.membership, a.AccountID, ws); err != nil {
			writeErr(w, 403, "auth.no_workspace_access", "no access to this workspace")
			return
		}
		kbs, err := h.kbStore.ListByWorkspace(r.Context(), a.AccountID, ws)
		if err != nil {
			writeErr(w, 500, "internal.error", "list failed")
			return
		}
		items := make([]map[string]any, 0, len(kbs))
		for _, kb := range kbs {
			items = append(items, map[string]any{
				"id":             kb.ID,
				"name":           kb.Name,
				"workspace_ref":  kb.WorkspaceRef,
				"description":    kb.Description,
				"document_count": kb.DocumentCount,
				"created_at":     kb.CreatedAt,
			})
		}
		writeJSON(w, 200, map[string]any{"items": items})
	}
}

func handleGetKB(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, 401, "auth.unauthenticated", "unauthenticated")
			return
		}
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeErr(w, 400, "kb.invalid_id", "invalid kb id")
			return
		}
		kb, err := h.kbStore.GetByID(r.Context(), id)
		if err != nil || kb == nil {
			writeErr(w, 404, "kb.not_found", "kb not found")
			return
		}
		if kb.AccountID != a.AccountID {
			writeErr(w, 404, "kb.not_found", "kb not found")
			return
		}
		if err := ensureWorkspaceMember(r.Context(), h.membership, a.AccountID, kb.WorkspaceRef); err != nil {
			writeErr(w, 403, "auth.no_workspace_access", "no access to this workspace")
			return
		}
		writeJSON(w, 200, map[string]any{
			"id":            kb.ID,
			"name":          kb.Name,
			"workspace_ref": kb.WorkspaceRef,
			"description":   kb.Description,
			"created_at":    kb.CreatedAt,
		})
	}
}

func handleDeleteKB(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, 401, "auth.unauthenticated", "unauthenticated")
			return
		}
		id, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeErr(w, 400, "kb.invalid_id", "invalid kb id")
			return
		}
		kb, err := h.kbStore.GetByID(r.Context(), id)
		if err != nil || kb == nil {
			writeErr(w, 404, "kb.not_found", "kb not found")
			return
		}
		if kb.AccountID != a.AccountID {
			writeErr(w, 404, "kb.not_found", "kb not found")
			return
		}
		if err := ensureWorkspaceMember(r.Context(), h.membership, a.AccountID, kb.WorkspaceRef); err != nil {
			writeErr(w, 403, "auth.no_workspace_access", "no access to this workspace")
			return
		}
		if err := h.kbStore.Delete(r.Context(), id); err != nil {
			writeErr(w, 500, "internal.error", "delete failed")
			return
		}
		w.WriteHeader(204)
	}
}
```

- [ ] **Step 3: Run tests**

```bash
cd src/services/api
go test ./internal/http/kbapi/... -v
```

Expected: 6 tests PASS.

- [ ] **Step 4: Commit**

```bash
git add src/services/api/internal/http/kbapi/handler_kb.go \
        src/services/api/internal/http/kbapi/handler_kb_test.go
git commit -m "feat(api): kbapi CRUD handlers for knowledge_bases

handleCreateKB / handleListKB / handleGetKB / handleDeleteKB with
narrow kbStore + membershipChecker interfaces for unit testing.
Auth enforces actor-account match + workspace membership; 409 on
UNIQUE (workspace_ref, name) violation; 404 hides KBs that aren't
in the caller's account."
```

---

---

## Task 7: Document upload + list + delete + search REST endpoints

**Files:**
- Create: `src/services/api/internal/http/kbapi/handler_doc.go`
- Create: `src/services/api/internal/http/kbapi/handler_doc_test.go`
- Create: `src/services/api/internal/http/kbapi/handler_search.go`
- Create: `src/services/api/internal/http/kbapi/handler_search_test.go`

**Steps:**

- [ ] **Step 1: Write doc handler tests**

Create `src/services/api/internal/http/kbapi/handler_doc_test.go`:

```go
package kbapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
)

type fakeDocStore struct {
	items map[uuid.UUID]*data.KBDocument
}

func newFakeDocStore() *fakeDocStore { return &fakeDocStore{items: map[uuid.UUID]*data.KBDocument{}} }

func (f *fakeDocStore) Create(ctx context.Context, in data.DocCreate) (*data.KBDocument, error) {
	d := &data.KBDocument{ID: uuid.New(), KBID: in.KBID, OriginalFilename: in.OriginalFilename, MimeType: in.MimeType, BlobSHA256: in.BlobSHA256, SizeBytes: in.SizeBytes, Status: "queued"}
	f.items[d.ID] = d
	return d, nil
}
func (f *fakeDocStore) GetByID(ctx context.Context, id uuid.UUID) (*data.KBDocument, error) {
	return f.items[id], nil
}
func (f *fakeDocStore) ListByKB(ctx context.Context, kbID uuid.UUID) ([]data.KBDocument, error) {
	var out []data.KBDocument
	for _, d := range f.items {
		if d.KBID == kbID {
			out = append(out, *d)
		}
	}
	return out, nil
}
func (f *fakeDocStore) Delete(ctx context.Context, id uuid.UUID) error {
	if _, ok := f.items[id]; !ok {
		return data.ErrDocNotFound
	}
	delete(f.items, id)
	return nil
}

type fakeBlobStore struct {
	puts map[string][]byte
}

func newFakeBlobStore() *fakeBlobStore { return &fakeBlobStore{puts: map[string][]byte{}} }

func (b *fakeBlobStore) PutBlob(ctx context.Context, workspaceRef, sha256 string, data []byte) error {
	b.puts[workspaceRef+"/"+sha256] = data
	return nil
}

type fakeJobEnqueue struct {
	called int
}

func (q *fakeJobEnqueue) EnqueueKBIngest(ctx context.Context, accountID, kbID, docID uuid.UUID, workspaceRef, blobSHA256, mimeType, filename, traceID string) (uuid.UUID, error) {
	q.called++
	return uuid.New(), nil
}

func newDocCtx(allow bool) (*handlerCtx, *fakeDocStore, *fakeBlobStore, *fakeJobEnqueue, *fakeKBStore) {
	kbStore := newFakeKBStore()
	docStore := newFakeDocStore()
	blob := newFakeBlobStore()
	jobs := &fakeJobEnqueue{}
	ctx := &handlerCtx{
		kbStore:    kbStore,
		docStore:   docStore,
		membership: &fakeMembership{allow: allow},
		blob:       blob,
		jobs:       jobs,
	}
	return ctx, docStore, blob, jobs, kbStore
}

func buildMultipart(t *testing.T, filename string, body []byte) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	_, _ = fw.Write(body)
	_ = w.Close()
	return buf, w.FormDataContentType()
}

func TestUploadDocHappyPath(t *testing.T) {
	ctx, docs, blob, jobs, kbStore := newDocCtx(true)
	acc := uuid.New()
	kb, _ := kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "ws", Name: "n"})

	body, ctType := buildMultipart(t, "a.txt", []byte("hello world"))
	req := httptest.NewRequest("POST", "/v1/knowledge-bases/"+kb.ID.String()+"/documents", body)
	req.Header.Set("Content-Type", ctType)
	req.SetPathValue("id", kb.ID.String())
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleUploadDoc(ctx)(w, req)
	if w.Code != 201 {
		t.Fatalf("got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		DocumentID string `json:"document_id"`
		JobID      string `json:"job_id"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.DocumentID == "" || resp.JobID == "" {
		t.Errorf("missing ids: %+v", resp)
	}
	if len(docs.items) != 1 {
		t.Errorf("docs not persisted: %d", len(docs.items))
	}
	if len(blob.puts) != 1 {
		t.Errorf("blob not written: %d", len(blob.puts))
	}
	if jobs.called != 1 {
		t.Errorf("job not enqueued: %d", jobs.called)
	}
}

func TestUploadDocRejectsOversize(t *testing.T) {
	ctx, _, _, _, kbStore := newDocCtx(true)
	acc := uuid.New()
	kb, _ := kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "ws", Name: "n"})
	big := bytes.Repeat([]byte("x"), 11*1024*1024) // 11 MB
	body, ctType := buildMultipart(t, "big.txt", big)
	req := httptest.NewRequest("POST", "/v1/knowledge-bases/"+kb.ID.String()+"/documents", body)
	req.Header.Set("Content-Type", ctType)
	req.SetPathValue("id", kb.ID.String())
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleUploadDoc(ctx)(w, req)
	if w.Code != 413 {
		t.Errorf("got %d, want 413", w.Code)
	}
}

func TestUploadDocRejectsUnsupportedExt(t *testing.T) {
	ctx, _, _, _, kbStore := newDocCtx(true)
	acc := uuid.New()
	kb, _ := kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "ws", Name: "n"})
	body, ctType := buildMultipart(t, "a.pdf", []byte("%PDF"))
	req := httptest.NewRequest("POST", "/v1/knowledge-bases/"+kb.ID.String()+"/documents", body)
	req.Header.Set("Content-Type", ctType)
	req.SetPathValue("id", kb.ID.String())
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleUploadDoc(ctx)(w, req)
	if w.Code != 415 {
		t.Errorf("got %d, want 415", w.Code)
	}
}

func TestListDocsByKB(t *testing.T) {
	ctx, docs, _, _, kbStore := newDocCtx(true)
	acc := uuid.New()
	kb, _ := kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "ws", Name: "n"})
	for _, name := range []string{"a.txt", "b.txt"} {
		_, _ = docs.Create(context.Background(), data.DocCreate{KBID: kb.ID, OriginalFilename: name, MimeType: "text/plain", BlobSHA256: "s", SizeBytes: 1})
	}
	req := httptest.NewRequest("GET", "/v1/knowledge-bases/"+kb.ID.String()+"/documents", nil)
	req.SetPathValue("id", kb.ID.String())
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleListDocs(ctx)(w, req)
	if w.Code != 200 {
		t.Fatalf("got %d", w.Code)
	}
	var resp struct {
		Items []map[string]any `json:"items"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Items) != 2 {
		t.Errorf("got %d items", len(resp.Items))
	}
}

func TestGetDocStatus(t *testing.T) {
	ctx, docs, _, _, kbStore := newDocCtx(true)
	acc := uuid.New()
	kb, _ := kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "ws", Name: "n"})
	doc, _ := docs.Create(context.Background(), data.DocCreate{KBID: kb.ID, OriginalFilename: "a.txt", MimeType: "text/plain", BlobSHA256: "s", SizeBytes: 1})
	req := httptest.NewRequest("GET", "/v1/knowledge-bases/"+kb.ID.String()+"/documents/"+doc.ID.String(), nil)
	req.SetPathValue("id", kb.ID.String())
	req.SetPathValue("doc_id", doc.ID.String())
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleGetDoc(ctx)(w, req)
	if w.Code != 200 {
		t.Fatalf("got %d", w.Code)
	}
}

func TestDeleteDoc(t *testing.T) {
	ctx, docs, _, _, kbStore := newDocCtx(true)
	acc := uuid.New()
	kb, _ := kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "ws", Name: "n"})
	doc, _ := docs.Create(context.Background(), data.DocCreate{KBID: kb.ID, OriginalFilename: "a.txt", MimeType: "text/plain", BlobSHA256: "s", SizeBytes: 1})
	req := httptest.NewRequest("DELETE", "/v1/knowledge-bases/"+kb.ID.String()+"/documents/"+doc.ID.String(), nil)
	req.SetPathValue("id", kb.ID.String())
	req.SetPathValue("doc_id", doc.ID.String())
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleDeleteDoc(ctx)(w, req)
	if w.Code != 204 {
		t.Errorf("got %d", w.Code)
	}
	if got, _ := docs.GetByID(context.Background(), doc.ID); got != nil {
		t.Error("not deleted")
	}
}

// Reference to silence unused import.
var _ = errors.New
```

- [ ] **Step 2: Extend handlerCtx and implement doc handlers**

Replace the `handlerCtx` definition in `src/services/api/internal/http/kbapi/handler_kb.go` to include new collaborators. Find the `type handlerCtx struct` block (lines added in Task 6) and replace with:

```go
type handlerCtx struct {
	kbStore    kbStore
	docStore   docStore
	chunksRepo chunksReader
	membership membershipChecker
	blob       blobWriter
	jobs       jobEnqueue
	embedder   embeddingForSearch

	maxUploadBytes int64
}

type docStore interface {
	Create(ctx context.Context, in data.DocCreate) (*data.KBDocument, error)
	GetByID(ctx context.Context, id uuid.UUID) (*data.KBDocument, error)
	ListByKB(ctx context.Context, kbID uuid.UUID) ([]data.KBDocument, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type blobWriter interface {
	PutBlob(ctx context.Context, workspaceRef, sha256 string, data []byte) error
}

type jobEnqueue interface {
	EnqueueKBIngest(ctx context.Context, accountID, kbID, docID uuid.UUID, workspaceRef, blobSHA256, mimeType, filename, traceID string) (uuid.UUID, error)
}

type chunksReader interface {
	Search(ctx context.Context, kbID uuid.UUID, query []float32, k int) ([]data.KBChunkHit, error)
}

type embeddingForSearch interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}
```

Then create `src/services/api/internal/http/kbapi/handler_doc.go`:

```go
package kbapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	nethttp "net/http"
	"path/filepath"
	"strings"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
)

const defaultMaxUploadBytes int64 = 10 * 1024 * 1024 // 10 MB

func handleUploadDoc(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, 401, "auth.unauthenticated", "unauthenticated")
			return
		}
		kbID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeErr(w, 400, "kb.invalid_id", "invalid kb id")
			return
		}
		kb, err := h.kbStore.GetByID(r.Context(), kbID)
		if err != nil || kb == nil {
			writeErr(w, 404, "kb.not_found", "kb not found")
			return
		}
		if kb.AccountID != a.AccountID {
			writeErr(w, 404, "kb.not_found", "kb not found")
			return
		}
		if err := ensureWorkspaceMember(r.Context(), h.membership, a.AccountID, kb.WorkspaceRef); err != nil {
			writeErr(w, 403, "auth.no_workspace_access", "no access to this workspace")
			return
		}
		maxBytes := h.maxUploadBytes
		if maxBytes == 0 {
			maxBytes = defaultMaxUploadBytes
		}
		// MaxBytesReader returns 413 when exceeded.
		r.Body = nethttp.MaxBytesReader(w, r.Body, maxBytes)
		if err := r.ParseMultipartForm(maxBytes); err != nil {
			var maxErr *nethttp.MaxBytesError
			if errors.As(err, &maxErr) {
				writeErr(w, 413, "kb.upload_too_large", "uploaded file exceeds 10MB limit")
				return
			}
			writeErr(w, 400, "kb.bad_multipart", "could not parse multipart body")
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeErr(w, 400, "kb.missing_file", "form field 'file' is required")
			return
		}
		defer file.Close()
		ext := strings.ToLower(filepath.Ext(header.Filename))
		if ext != ".txt" && ext != ".md" {
			writeErr(w, 415, "kb.unsupported_format", "only .txt and .md are supported in M1.0")
			return
		}
		buf := &bytes.Buffer{}
		n, err := io.Copy(buf, file)
		if err != nil {
			writeErr(w, 400, "kb.read_failed", "could not read uploaded file")
			return
		}
		sum := sha256.Sum256(buf.Bytes())
		shaHex := hex.EncodeToString(sum[:])
		if err := h.blob.PutBlob(r.Context(), kb.WorkspaceRef, shaHex, buf.Bytes()); err != nil {
			writeErr(w, 500, "internal.blob_failed", "failed to persist blob")
			return
		}
		mime := "text/markdown"
		if ext == ".txt" {
			mime = "text/plain"
		}
		doc, err := h.docStore.Create(r.Context(), data.DocCreate{
			KBID: kb.ID, OriginalFilename: header.Filename, MimeType: mime,
			BlobSHA256: shaHex, SizeBytes: n, CreatedBy: &a.UserID,
		})
		if err != nil {
			writeErr(w, 500, "internal.doc_create_failed", "failed to record document")
			return
		}
		jobID, err := h.jobs.EnqueueKBIngest(r.Context(), a.AccountID, kb.ID, doc.ID, kb.WorkspaceRef, shaHex, mime, header.Filename, "")
		if err != nil {
			writeErr(w, 500, "internal.enqueue_failed", "failed to enqueue ingest job")
			return
		}
		writeJSON(w, 201, map[string]any{"document_id": doc.ID, "job_id": jobID})
	}
}

func handleListDocs(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, 401, "auth.unauthenticated", "unauthenticated")
			return
		}
		kbID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeErr(w, 400, "kb.invalid_id", "invalid kb id")
			return
		}
		kb, _ := h.kbStore.GetByID(r.Context(), kbID)
		if kb == nil || kb.AccountID != a.AccountID {
			writeErr(w, 404, "kb.not_found", "kb not found")
			return
		}
		if err := ensureWorkspaceMember(r.Context(), h.membership, a.AccountID, kb.WorkspaceRef); err != nil {
			writeErr(w, 403, "auth.no_workspace_access", "no access to this workspace")
			return
		}
		docs, err := h.docStore.ListByKB(r.Context(), kbID)
		if err != nil {
			writeErr(w, 500, "internal.error", "list failed")
			return
		}
		items := make([]map[string]any, 0, len(docs))
		for _, d := range docs {
			items = append(items, map[string]any{
				"id":                d.ID,
				"original_filename": d.OriginalFilename,
				"mime_type":         d.MimeType,
				"size_bytes":        d.SizeBytes,
				"status":            d.Status,
				"error_message":     d.ErrorMessage,
				"created_at":        d.CreatedAt,
				"updated_at":        d.UpdatedAt,
			})
		}
		writeJSON(w, 200, map[string]any{"items": items})
	}
}

func handleGetDoc(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, 401, "auth.unauthenticated", "unauthenticated")
			return
		}
		kbID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeErr(w, 400, "kb.invalid_id", "invalid kb id")
			return
		}
		docID, err := uuid.Parse(r.PathValue("doc_id"))
		if err != nil {
			writeErr(w, 400, "kb.invalid_doc_id", "invalid doc id")
			return
		}
		kb, _ := h.kbStore.GetByID(r.Context(), kbID)
		if kb == nil || kb.AccountID != a.AccountID {
			writeErr(w, 404, "kb.not_found", "kb not found")
			return
		}
		if err := ensureWorkspaceMember(r.Context(), h.membership, a.AccountID, kb.WorkspaceRef); err != nil {
			writeErr(w, 403, "auth.no_workspace_access", "no access to this workspace")
			return
		}
		doc, err := h.docStore.GetByID(r.Context(), docID)
		if err != nil || doc == nil || doc.KBID != kb.ID {
			writeErr(w, 404, "kb.doc_not_found", "document not found")
			return
		}
		writeJSON(w, 200, map[string]any{
			"id":                doc.ID,
			"original_filename": doc.OriginalFilename,
			"mime_type":         doc.MimeType,
			"size_bytes":        doc.SizeBytes,
			"status":            doc.Status,
			"error_message":     doc.ErrorMessage,
			"parse_meta":        doc.ParseMeta,
			"created_at":        doc.CreatedAt,
			"updated_at":        doc.UpdatedAt,
		})
	}
}

func handleDeleteDoc(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, 401, "auth.unauthenticated", "unauthenticated")
			return
		}
		kbID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeErr(w, 400, "kb.invalid_id", "invalid kb id")
			return
		}
		docID, err := uuid.Parse(r.PathValue("doc_id"))
		if err != nil {
			writeErr(w, 400, "kb.invalid_doc_id", "invalid doc id")
			return
		}
		kb, _ := h.kbStore.GetByID(r.Context(), kbID)
		if kb == nil || kb.AccountID != a.AccountID {
			writeErr(w, 404, "kb.not_found", "kb not found")
			return
		}
		if err := ensureWorkspaceMember(r.Context(), h.membership, a.AccountID, kb.WorkspaceRef); err != nil {
			writeErr(w, 403, "auth.no_workspace_access", "no access to this workspace")
			return
		}
		if err := h.docStore.Delete(r.Context(), docID); err != nil {
			if errors.Is(err, data.ErrDocNotFound) {
				writeErr(w, 404, "kb.doc_not_found", "document not found")
				return
			}
			writeErr(w, 500, "internal.error", "delete failed")
			return
		}
		w.WriteHeader(204)
	}
}
```

- [ ] **Step 3: Write search handler test**

Create `src/services/api/internal/http/kbapi/handler_search_test.go`:

```go
package kbapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"arkloop/services/api/internal/data"

	"github.com/google/uuid"
)

type fakeChunks struct {
	hits []data.KBChunkHit
}

func (f *fakeChunks) Search(ctx context.Context, kbID uuid.UUID, q []float32, k int) ([]data.KBChunkHit, error) {
	return f.hits, nil
}

type fakeEmbed struct{}

func (fakeEmbed) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{1.0, 0.0}
	}
	return out, nil
}

func TestSearchHappyPath(t *testing.T) {
	ctx := &handlerCtx{
		kbStore:    newFakeKBStore(),
		membership: &fakeMembership{allow: true},
		chunksRepo: &fakeChunks{hits: []data.KBChunkHit{{DocumentRef: "a.txt", Ordinal: 0, Text: "光的干涉", Score: 0.92}}},
		embedder:   fakeEmbed{},
	}
	acc := uuid.New()
	kb, _ := ctx.kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "w", Name: "n"})
	req := httptest.NewRequest("GET", "/v1/knowledge-bases/"+kb.ID.String()+"/search?q=light&k=3", nil)
	req.SetPathValue("id", kb.ID.String())
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleSearch(ctx)(w, req)
	if w.Code != 200 {
		t.Fatalf("got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Hits []map[string]any `json:"hits"`
	}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Hits) != 1 {
		t.Errorf("got %d hits", len(resp.Hits))
	}
}

func TestSearchRequiresQuery(t *testing.T) {
	ctx := &handlerCtx{kbStore: newFakeKBStore(), membership: &fakeMembership{allow: true}}
	acc := uuid.New()
	kb, _ := ctx.kbStore.Create(context.Background(), data.KBCreate{AccountID: acc, WorkspaceRef: "w", Name: "n"})
	req := httptest.NewRequest("GET", "/v1/knowledge-bases/"+kb.ID.String()+"/search", nil)
	req.SetPathValue("id", kb.ID.String())
	req = injectActor(req, acc, uuid.New())
	w := httptest.NewRecorder()
	handleSearch(ctx)(w, req)
	if w.Code != 400 {
		t.Errorf("got %d", w.Code)
	}
}
```

- [ ] **Step 4: Implement search handler**

Create `src/services/api/internal/http/kbapi/handler_search.go`:

```go
package kbapi

import (
	nethttp "net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

func handleSearch(h *handlerCtx) nethttp.HandlerFunc {
	return func(w nethttp.ResponseWriter, r *nethttp.Request) {
		a, ok := actorFromCtx(r.Context())
		if !ok {
			writeErr(w, 401, "auth.unauthenticated", "unauthenticated")
			return
		}
		kbID, err := uuid.Parse(r.PathValue("id"))
		if err != nil {
			writeErr(w, 400, "kb.invalid_id", "invalid kb id")
			return
		}
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		if q == "" {
			writeErr(w, 400, "kb.missing_query", "q query param is required")
			return
		}
		k := 8
		if v := r.URL.Query().Get("k"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
				k = n
			}
		}
		kb, _ := h.kbStore.GetByID(r.Context(), kbID)
		if kb == nil || kb.AccountID != a.AccountID {
			writeErr(w, 404, "kb.not_found", "kb not found")
			return
		}
		if err := ensureWorkspaceMember(r.Context(), h.membership, a.AccountID, kb.WorkspaceRef); err != nil {
			writeErr(w, 403, "auth.no_workspace_access", "no access to this workspace")
			return
		}
		vecs, err := h.embedder.Embed(r.Context(), []string{q})
		if err != nil {
			writeErr(w, 500, "internal.embed_failed", "could not embed query")
			return
		}
		hits, err := h.chunksRepo.Search(r.Context(), kbID, vecs[0], k)
		if err != nil {
			writeErr(w, 500, "internal.search_failed", "search failed")
			return
		}
		items := make([]map[string]any, 0, len(hits))
		for _, hit := range hits {
			items = append(items, map[string]any{
				"document_ref": hit.DocumentRef,
				"ordinal":      hit.Ordinal,
				"heading_path": hit.HeadingPath,
				"chunk_type":   hit.ChunkType,
				"text":         hit.Text,
				"score":        hit.Score,
			})
		}
		writeJSON(w, 200, map[string]any{"hits": items})
	}
}
```

- [ ] **Step 5: Register routes**

Create `src/services/api/internal/http/kbapi/register.go`:

```go
package kbapi

import (
	nethttp "net/http"
)

// Register wires KB routes onto mux. The actor-injection middleware
// (httpkit) is applied by the caller, so by the time these handlers
// fire, actorFromCtx works.
func Register(mux *nethttp.ServeMux, h *handlerCtx) {
	mux.Handle("POST /v1/knowledge-bases", handleCreateKB(h))
	mux.Handle("GET /v1/knowledge-bases", handleListKB(h))
	mux.Handle("GET /v1/knowledge-bases/{id}", handleGetKB(h))
	mux.Handle("DELETE /v1/knowledge-bases/{id}", handleDeleteKB(h))
	mux.Handle("POST /v1/knowledge-bases/{id}/documents", handleUploadDoc(h))
	mux.Handle("GET /v1/knowledge-bases/{id}/documents", handleListDocs(h))
	mux.Handle("GET /v1/knowledge-bases/{id}/documents/{doc_id}", handleGetDoc(h))
	mux.Handle("DELETE /v1/knowledge-bases/{id}/documents/{doc_id}", handleDeleteDoc(h))
	mux.Handle("GET /v1/knowledge-bases/{id}/search", handleSearch(h))
}
```

- [ ] **Step 6: Run all kbapi tests**

```bash
cd src/services/api
go test ./internal/http/kbapi/... -v
```

Expected: all PASS (handler_kb_test + handler_doc_test + handler_search_test + auth_test).

- [ ] **Step 7: Commit**

```bash
git add src/services/api/internal/http/kbapi/handler_doc.go \
        src/services/api/internal/http/kbapi/handler_doc_test.go \
        src/services/api/internal/http/kbapi/handler_search.go \
        src/services/api/internal/http/kbapi/handler_search_test.go \
        src/services/api/internal/http/kbapi/handler_kb.go \
        src/services/api/internal/http/kbapi/register.go
git commit -m "feat(api): kbapi document upload/list/get/delete + search REST

Upload: multipart form, 10MB cap, .txt/.md only, sha256 → blob store,
then enqueue kb_ingest job. List/Get/Delete + search by embedding;
all routes share workspace-membership auth. Register() wires the 9
endpoints under /v1/knowledge-bases."
```

---

## Task 8: Remove M0 _debug routes

**Files:**
- Delete: `src/services/api/internal/http/kbdebugapi/` (entire package)
- Modify: `src/services/api/internal/http/handler.go` (remove `kbdebugapi.Register` call + import)
- Modify: `src/services/api/internal/app/app.go` (remove KBIngestService construction + HandlerConfig field assignment)
- Modify: `src/services/api/internal/kbingest/service.go` (delete — superseded by Task 7 upload + Task 9 worker job)
- Delete: `src/services/api/internal/kbingest/` directory

**Steps:**

- [ ] **Step 1: Delete kbdebugapi package**

```bash
rm -r src/services/api/internal/http/kbdebugapi
```

- [ ] **Step 2: Delete kbingest package**

```bash
rm -r src/services/api/internal/kbingest
```

- [ ] **Step 3: Remove handler.go references**

Open `src/services/api/internal/http/handler.go`. Find and delete:
- The import lines `"arkloop/services/api/internal/http/kbdebugapi"` and `"arkloop/services/api/internal/kbingest"`
- The fields in `HandlerConfig`:
  ```go
  KBIngestService *kbingest.Service
  KBDebugToken    string
  ```
- The block:
  ```go
  if cfg.KBIngestService != nil {
      kbdebugapi.Register(mux, cfg.KBDebugToken, cfg.KBIngestService, cfg.KBIngestService)
  }
  ```

- [ ] **Step 4: Remove app.go references**

Open `src/services/api/internal/app/app.go`. Find and delete:
- The imports `"arkloop/services/api/internal/kbingest"` and `"arkloop/services/shared/embedding"` **only if not used elsewhere** — if `embedding` is used by the new wiring in Task 9 (which it will be), keep it. Easiest: leave the imports; `go build` will tell you what's unused.
- The block constructing `kbIngestService` (the `if cfg.KBDebugToken != "" ...` section)
- The `KBIngestService` and `KBDebugToken` field assignments in the `HandlerConfig{...}` literal.

- [ ] **Step 5: Build to find lingering references**

```bash
cd src/services/api
go build ./...
```

Fix any unused-import errors that pop up. Expected end state: build clean.

- [ ] **Step 6: Sanity test**

```bash
go test ./...
```

Expected: PASS. Note that the M0 e2e_integration_test was removed in Task 4 Step 5.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(api): remove M0 _debug KB routes superseded by M1.0 kbapi

Delete kbdebugapi package and kbingest service. The M0 debug routes
verified the pipeline end-to-end before the real schema landed;
M1.0 kbapi (Tasks 6-7) is the production-shaped successor. Worker
will own the ingest pipeline (Task 9)."
```

---

## Task 9: Worker kb_ingest job processor

**Files:**
- Create: `src/services/worker/internal/kbingest/processor.go`
- Create: `src/services/worker/internal/kbingest/processor_test.go`
- Create: `src/services/worker/internal/data/kb_chunks_repo.go` (worker-side direct DB access)
- Modify: `src/services/worker/internal/queue/protocol.go` (add `KBIngestJobType` constant)
- Modify: `src/services/worker/internal/app/config.go` (add `KBIngestJobType` to QueueJobTypes + Capabilities)
- Modify: `src/services/worker/cmd/worker/run_cloud.go` (register the handler in dispatchHandler)

**Steps:**

- [ ] **Step 1: Add job type constant**

In `src/services/worker/internal/queue/protocol.go`, find the const block with `RunExecuteJobType` and add:

```go
KBIngestJobType = "kb.ingest"
```

(Place it after `ContextCompactMaintainJobType`.)

- [ ] **Step 2: Wire job type into worker config**

In `src/services/worker/internal/app/config.go`, find the `QueueJobTypes` slice (around line 66) and the `Capabilities` slice (line 67) and append `queue.KBIngestJobType` to both. Then find the map `map[string]struct{}{...}` around line 230 and add `queue.KBIngestJobType: {}`.

- [ ] **Step 3: Add worker-side KB chunks repo**

Create `src/services/worker/internal/data/kb_chunks_repo.go`:

```go
// Package data hosts worker's direct-DB repositories. KB chunks are read
// + written from worker for the ingest pipeline (writes) and kb_search
// tool (reads). Schema mirrors api/internal/data/kb_chunks_repo.go.
package data

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type KBChunksRepository struct {
	pool *pgxpool.Pool
	dim  int
}

type KBChunkUpsert struct {
	KBID        uuid.UUID
	DocumentID  uuid.UUID
	Ordinal     int
	HeadingPath []string
	ChunkType   string
	Text        string
	TokenCount  int
	Embedding   []float32
}

type KBChunkHit struct {
	ID          uuid.UUID
	KBID        uuid.UUID
	DocumentID  uuid.UUID
	DocumentRef string
	Ordinal     int
	HeadingPath []string
	ChunkType   string
	Text        string
	TokenCount  int
	Score       float32
}

func NewKBChunksRepository(pool *pgxpool.Pool) (*KBChunksRepository, error) {
	if pool == nil {
		return nil, fmt.Errorf("nil pool")
	}
	var dim int
	row := pool.QueryRow(context.Background(), `
SELECT a.atttypmod FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
WHERE c.relname = 'kb_chunks' AND a.attname = 'embedding'`)
	if err := row.Scan(&dim); err != nil {
		return nil, fmt.Errorf("probe pgvector dim: %w", err)
	}
	return &KBChunksRepository{pool: pool, dim: dim}, nil
}

func (r *KBChunksRepository) Dim() int { return r.dim }

func (r *KBChunksRepository) Upsert(ctx context.Context, rows []KBChunkUpsert) error {
	for _, row := range rows {
		if len(row.Embedding) != r.dim {
			return fmt.Errorf("kb=%s doc=%s ord=%d: dim %d != table %d", row.KBID, row.DocumentID, row.Ordinal, len(row.Embedding), r.dim)
		}
		_, err := r.pool.Exec(ctx, `
INSERT INTO kb_chunks (kb_id, document_id, ordinal, heading_path, chunk_type, text, token_count, embedding, metadata_json)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, '{}'::jsonb)
ON CONFLICT (kb_id, document_id, ordinal) DO UPDATE SET
    heading_path = EXCLUDED.heading_path,
    chunk_type   = EXCLUDED.chunk_type,
    text         = EXCLUDED.text,
    token_count  = EXCLUDED.token_count,
    embedding    = EXCLUDED.embedding`,
			row.KBID, row.DocumentID, row.Ordinal, row.HeadingPath, row.ChunkType, row.Text, row.TokenCount, vecLiteral(row.Embedding))
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *KBChunksRepository) Search(ctx context.Context, kbID uuid.UUID, q []float32, k int) ([]KBChunkHit, error) {
	if len(q) != r.dim {
		return nil, fmt.Errorf("query dim %d != table dim %d", len(q), r.dim)
	}
	if k <= 0 {
		k = 8
	}
	rows, err := r.pool.Query(ctx, `
SELECT c.id, c.kb_id, c.document_id, d.original_filename, c.ordinal, c.heading_path, c.chunk_type, c.text, c.token_count,
       1 - (c.embedding <=> $2) AS score
FROM   kb_chunks c
JOIN   kb_documents d ON d.id = c.document_id
WHERE  c.kb_id = $1
ORDER  BY c.embedding <=> $2
LIMIT  $3`, kbID, vecLiteral(q), k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KBChunkHit
	for rows.Next() {
		var h KBChunkHit
		if err := rows.Scan(&h.ID, &h.KBID, &h.DocumentID, &h.DocumentRef, &h.Ordinal,
			&h.HeadingPath, &h.ChunkType, &h.Text, &h.TokenCount, &h.Score); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// UpdateDocStatus mirrors the api side helper so the worker can drive the state machine.
func (r *KBChunksRepository) UpdateDocStatus(ctx context.Context, docID uuid.UUID, status, errorMessage string) error {
	_, err := r.pool.Exec(ctx, `
UPDATE kb_documents SET status=$2, error_message=$3, updated_at=now()
WHERE  id=$1`, docID, status, errorMessage)
	return err
}

func vecLiteral(v []float32) string {
	var sb strings.Builder
	sb.Grow(len(v) * 6)
	sb.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, "%g", x)
	}
	sb.WriteByte(']')
	return sb.String()
}
```

- [ ] **Step 4: Write processor test (uses fake collaborators)**

Create `src/services/worker/internal/kbingest/processor_test.go`:

```go
package kbingest

import (
	"context"
	"errors"
	"testing"

	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/queue"

	"github.com/google/uuid"
)

type fakeBlobReader struct {
	content map[string][]byte
}

func (f *fakeBlobReader) GetBlob(ctx context.Context, workspaceRef, sha string) ([]byte, error) {
	v, ok := f.content[workspaceRef+"/"+sha]
	if !ok {
		return nil, errors.New("not found")
	}
	return v, nil
}

type fakeChunksRepo struct {
	upserted []data.KBChunkUpsert
	statuses []string
	errors   []string
}

func (f *fakeChunksRepo) Upsert(ctx context.Context, rows []data.KBChunkUpsert) error {
	f.upserted = append(f.upserted, rows...)
	return nil
}

func (f *fakeChunksRepo) UpdateDocStatus(ctx context.Context, id uuid.UUID, status, msg string) error {
	f.statuses = append(f.statuses, status)
	f.errors = append(f.errors, msg)
	return nil
}

func (f *fakeChunksRepo) Dim() int { return 8 }

type fakeEmb struct {
	dim int
}

func (e fakeEmb) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		v := make([]float32, e.dim)
		v[0] = 1
		out[i] = v
	}
	return out, nil
}
func (e fakeEmb) Dim() int { return e.dim }

func TestProcessorHappyPath(t *testing.T) {
	docID := uuid.New()
	kbID := uuid.New()
	blob := &fakeBlobReader{content: map[string][]byte{"ws/abc": []byte("段落 A 内容。\n\n段落 B 内容。")}}
	chunks := &fakeChunksRepo{}
	p := NewProcessor(blob, chunks, fakeEmb{dim: 8})
	lease := queue.JobLease{
		JobID: uuid.New(),
		PayloadJSON: map[string]any{
			"type":           "kb.ingest",
			"kb_id":          kbID.String(),
			"document_id":    docID.String(),
			"workspace_ref":  "ws",
			"blob_sha256":    "abc",
			"mime_type":      "text/plain",
			"original_filename": "a.txt",
		},
	}
	if err := p.Handle(context.Background(), lease); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(chunks.upserted) != 2 {
		t.Errorf("upserted: got %d, want 2", len(chunks.upserted))
	}
	// Status state machine: queued (initial, not set here) → parsing → chunking → embedding → upserting → ready
	wantStatuses := []string{"parsing", "chunking", "embedding", "upserting", "ready"}
	if len(chunks.statuses) != len(wantStatuses) {
		t.Fatalf("statuses: got %v, want %v", chunks.statuses, wantStatuses)
	}
	for i, s := range wantStatuses {
		if chunks.statuses[i] != s {
			t.Errorf("status %d: got %q, want %q", i, chunks.statuses[i], s)
		}
	}
}

func TestProcessorMarksFailedOnParseError(t *testing.T) {
	docID := uuid.New()
	blob := &fakeBlobReader{content: map[string][]byte{}} // empty → blob.GetBlob will fail
	chunks := &fakeChunksRepo{}
	p := NewProcessor(blob, chunks, fakeEmb{dim: 8})
	lease := queue.JobLease{
		JobID: uuid.New(),
		PayloadJSON: map[string]any{
			"type":           "kb.ingest",
			"kb_id":          uuid.New().String(),
			"document_id":    docID.String(),
			"workspace_ref":  "ws",
			"blob_sha256":    "missing",
			"mime_type":      "text/plain",
			"original_filename": "a.txt",
		},
	}
	err := p.Handle(context.Background(), lease)
	if err == nil {
		t.Error("expected error")
	}
	last := chunks.statuses[len(chunks.statuses)-1]
	if last != "failed" {
		t.Errorf("last status: got %q, want failed", last)
	}
}
```

- [ ] **Step 5: Implement processor**

Create `src/services/worker/internal/kbingest/processor.go`:

```go
// Package kbingest hosts the worker-side kb_ingest job processor that
// drives a document through the state machine:
// parsing → chunking → embedding → upserting → ready
package kbingest

import (
	"bytes"
	"context"
	"fmt"

	"arkloop/services/shared/bookchunker"
	"arkloop/services/shared/bookparser"
	"arkloop/services/shared/embedding"
	"arkloop/services/worker/internal/data"
	"arkloop/services/worker/internal/queue"

	"github.com/google/uuid"
)

// BlobReader fetches raw uploaded bytes by workspace + sha. Production
// uses objectstore-backed fetcher; tests substitute a map-backed fake.
type BlobReader interface {
	GetBlob(ctx context.Context, workspaceRef, sha256 string) ([]byte, error)
}

// ChunksRepo is the slice of worker/internal/data.KBChunksRepository
// the processor needs. Narrow interface lets tests use a fake.
type ChunksRepo interface {
	Upsert(ctx context.Context, rows []data.KBChunkUpsert) error
	UpdateDocStatus(ctx context.Context, docID uuid.UUID, status, errorMessage string) error
	Dim() int
}

// Processor implements consumer.Handler — its Handle is registered to
// dispatchHandler in worker/cmd/worker/run_cloud.go for KBIngestJobType.
type Processor struct {
	blob     BlobReader
	chunks   ChunksRepo
	embedder embedding.Embedder
	parser   bookparser.Parser
}

func NewProcessor(blob BlobReader, chunks ChunksRepo, embedder embedding.Embedder) *Processor {
	return &Processor{
		blob:     blob,
		chunks:   chunks,
		embedder: embedder,
		parser:   bookparser.NewTextOnlyParser(),
	}
}

// Handle runs the ingest pipeline. State machine is durable in
// kb_documents.status; each transition is committed before the next
// stage starts so failures are restartable from the last status.
func (p *Processor) Handle(ctx context.Context, lease queue.JobLease) error {
	pl, err := parsePayload(lease.PayloadJSON)
	if err != nil {
		return fmt.Errorf("kb_ingest: parse payload: %w", err)
	}
	if err := p.chunks.UpdateDocStatus(ctx, pl.DocumentID, "parsing", ""); err != nil {
		return err
	}
	raw, err := p.blob.GetBlob(ctx, pl.WorkspaceRef, pl.BlobSHA256)
	if err != nil {
		_ = p.chunks.UpdateDocStatus(ctx, pl.DocumentID, "failed", "blob fetch: "+err.Error())
		return err
	}
	doc, err := p.parser.Parse(ctx, bytes.NewReader(raw), pl.MimeType)
	if err != nil {
		_ = p.chunks.UpdateDocStatus(ctx, pl.DocumentID, "failed", "parse: "+err.Error())
		return err
	}
	if err := p.chunks.UpdateDocStatus(ctx, pl.DocumentID, "chunking", ""); err != nil {
		return err
	}
	chunkOuts, err := bookchunker.Chunk(doc, bookchunker.DefaultOptions())
	if err != nil {
		_ = p.chunks.UpdateDocStatus(ctx, pl.DocumentID, "failed", "chunk: "+err.Error())
		return err
	}
	if len(chunkOuts) == 0 {
		_ = p.chunks.UpdateDocStatus(ctx, pl.DocumentID, "ready", "")
		return nil
	}
	if err := p.chunks.UpdateDocStatus(ctx, pl.DocumentID, "embedding", ""); err != nil {
		return err
	}
	texts := make([]string, len(chunkOuts))
	for i, c := range chunkOuts {
		texts[i] = c.Text
	}
	vecs, err := p.embedder.Embed(ctx, texts)
	if err != nil {
		_ = p.chunks.UpdateDocStatus(ctx, pl.DocumentID, "failed", "embed: "+err.Error())
		return err
	}
	if err := p.chunks.UpdateDocStatus(ctx, pl.DocumentID, "upserting", ""); err != nil {
		return err
	}
	rows := make([]data.KBChunkUpsert, len(chunkOuts))
	for i, c := range chunkOuts {
		rows[i] = data.KBChunkUpsert{
			KBID: pl.KBID, DocumentID: pl.DocumentID, Ordinal: c.Ordinal,
			HeadingPath: c.HeadingPath, ChunkType: c.ChunkType,
			Text: c.Text, TokenCount: c.TokenCount, Embedding: vecs[i],
		}
	}
	if err := p.chunks.Upsert(ctx, rows); err != nil {
		_ = p.chunks.UpdateDocStatus(ctx, pl.DocumentID, "failed", "upsert: "+err.Error())
		return err
	}
	if err := p.chunks.UpdateDocStatus(ctx, pl.DocumentID, "ready", ""); err != nil {
		return err
	}
	return nil
}

// Payload mirrors the api enqueue shape.
type Payload struct {
	KBID             uuid.UUID
	DocumentID       uuid.UUID
	WorkspaceRef     string
	BlobSHA256       string
	MimeType         string
	OriginalFilename string
}

func parsePayload(p map[string]any) (Payload, error) {
	out := Payload{}
	kbID, err := uuid.Parse(asString(p["kb_id"]))
	if err != nil {
		return out, fmt.Errorf("kb_id: %w", err)
	}
	docID, err := uuid.Parse(asString(p["document_id"]))
	if err != nil {
		return out, fmt.Errorf("document_id: %w", err)
	}
	out.KBID = kbID
	out.DocumentID = docID
	out.WorkspaceRef = asString(p["workspace_ref"])
	out.BlobSHA256 = asString(p["blob_sha256"])
	out.MimeType = asString(p["mime_type"])
	out.OriginalFilename = asString(p["original_filename"])
	if out.WorkspaceRef == "" || out.BlobSHA256 == "" || out.MimeType == "" {
		return out, fmt.Errorf("missing required field")
	}
	return out, nil
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
```

- [ ] **Step 6: Register processor in dispatchHandler**

Open `src/services/worker/cmd/worker/run_cloud.go`. Find the block that builds `dispatchHandler{handlers: map[string]consumer.Handler{...}}` (around line 300 — see the existing webhook + email registrations) and add KB ingest:

```go
kbProcessor, err := buildKBIngestProcessor(ctx, pool, cfg, logger)
if err != nil {
    return nil, fmt.Errorf("kb ingest processor: %w", err)
}
logger.Info("kb_ingest handler enabled", "job_type", queue.KBIngestJobType)
```

In the `handlers` map, add:
```go
queue.KBIngestJobType: kbProcessor,
```

Then implement `buildKBIngestProcessor` near the top of the same file (or in a sibling `kbingest_wiring.go` if preferred):

```go
func buildKBIngestProcessor(ctx context.Context, pool *pgxpool.Pool, cfg *app.Config, logger *slog.Logger) (consumer.Handler, error) {
    chunksRepo, err := data.NewKBChunksRepository(pool)
    if err != nil {
        return nil, err
    }
    embedder := embedding.NewDoubao(embedding.DoubaoConfig{
        BaseURL:    cfg.DoubaoEmbedBaseURL,
        APIKey:     cfg.DoubaoEmbedAPIKey,
        Model:      cfg.DoubaoEmbedModel,
        BatchSize:  cfg.DoubaoEmbedBatchSize,
        MaxRetries: 3,
        Dim:        chunksRepo.Dim(),
    })
    blob := newWorkspaceBlobReader(pool, cfg) // helper that wraps objectstore — see worker's existing blob client
    return kbingest.NewProcessor(blob, chunksRepo, embedder), nil
}
```

For `newWorkspaceBlobReader`: the worker already has objectstore access via the existing run engine. The exact wiring depends on how the worker config exposes object store credentials — look at how `executor.NewNativeRunEngineV1Handler` constructs its store (`cfg.S3` or similar). If unsure, defer to whichever helper the worker uses to read run-output blobs (it should already exist; this is the same MinIO/S3 bucket).

If no clean helper exists, create `src/services/worker/internal/kbingest/blob.go`:

```go
package kbingest

import (
    "context"
    "fmt"

    "arkloop/services/shared/objectstore"
)

type ObjectStoreBlobReader struct {
    Store objectstore.Store
}

func (r *ObjectStoreBlobReader) GetBlob(ctx context.Context, workspaceRef, sha string) ([]byte, error) {
    key := fmt.Sprintf("workspaces/%s/blobs/%s", workspaceRef, sha)
    obj, err := r.Store.Get(ctx, key)
    if err != nil {
        return nil, err
    }
    return obj.Body, nil
}
```

Match `objectstore.Store.Get`'s actual signature (look at how existing code reads from it; the precise method name / shape lives in `shared/objectstore`).

- [ ] **Step 7: Add Doubao config fields to worker config**

Open `src/services/worker/internal/app/config.go`. Add fields to the worker `Config` struct mirroring the api side:

```go
DoubaoEmbedAPIKey    string  // env ARK_API_KEY
DoubaoEmbedBaseURL   string  // env ARK_BASE_URL
DoubaoEmbedModel     string  // env ARK_EMBED_MODEL
DoubaoEmbedBatchSize int     // env ARK_EMBED_BATCH
```

Populate them in the Load function from env, with the same defaults as in the M0 plan (base url `https://ark.cn-beijing.volces.com/api/v3`, model `doubao-embedding-text-240715`, batch 32).

- [ ] **Step 8: Run processor tests**

```bash
cd src/services/worker
go test ./internal/kbingest/... -v
```

Expected: 2 tests PASS.

- [ ] **Step 9: Add api-side helper to enqueue kb_ingest job (wire into kbapi.Deps)**

Create `src/services/api/internal/http/kbapi/jobs.go`:

```go
package kbapi

import (
	"context"

	"arkloop/services/worker/internal/queue"

	"github.com/google/uuid"
)

// JobQueueEnqueuer wraps the worker's JobQueue to provide the narrow
// EnqueueKBIngest method used by the upload handler. Keeps the handler
// from caring about the EnqueueRun signature shape.
type JobQueueEnqueuer struct {
	Q queue.JobQueue
}

func (j *JobQueueEnqueuer) EnqueueKBIngest(ctx context.Context, accountID, kbID, docID uuid.UUID, workspaceRef, blobSHA256, mimeType, filename, traceID string) (uuid.UUID, error) {
	payload := map[string]any{
		"type":              queue.KBIngestJobType,
		"kb_id":             kbID.String(),
		"document_id":       docID.String(),
		"workspace_ref":     workspaceRef,
		"blob_sha256":       blobSHA256,
		"mime_type":         mimeType,
		"original_filename": filename,
		"version":           1,
	}
	return j.Q.EnqueueRun(ctx, accountID, uuid.Nil, traceID, queue.KBIngestJobType, payload, nil)
}
```

- [ ] **Step 10: Wire api's app.go to register kbapi routes with all the deps**

In `src/services/api/internal/app/app.go`, after the line where `pool` is constructed and before the HTTP handler is built, add a block that constructs the kbapi handler context and registers the routes. Concretely:

```go
// kbapi (M1.0): knowledge-base CRUD + ingest enqueue + debug search
if cfg.DoubaoEmbedAPIKey != "" {
    kbRepo, _ := data.NewKnowledgeBasesRepository(pool)
    docsRepo, _ := data.NewKBDocumentsRepository(pool)
    chunksRepo, _ := data.NewKBChunksRepository(pool)
    wsRepo := data.NewWorkspaceRegistriesRepository(pool)  // adjust if signature differs
    membershipRepo := data.NewAccountMembershipRepository(pool)
    blob := &kbapi.WorkspaceBlobAdapter{Store: store}
    embedder := embedding.NewDoubao(embedding.DoubaoConfig{
        BaseURL: cfg.DoubaoEmbedBaseURL, APIKey: cfg.DoubaoEmbedAPIKey,
        Model: cfg.DoubaoEmbedModel, BatchSize: cfg.DoubaoEmbedBatchSize,
        MaxRetries: 3, Dim: chunksRepo.Dim(),
    })
    enqueuer := &kbapi.JobQueueEnqueuer{Q: jobQueue} // jobQueue must already be in scope from existing wiring
    membershipChecker := &kbapi.WorkspaceMembership{Memberships: membershipRepo, Workspaces: wsRepo}
    kbHandlerCtx := kbapi.NewHandlerCtx(kbRepo, docsRepo, chunksRepo, membershipChecker, blob, enqueuer, embedder, 10*1024*1024)
    handlerCfg.KBHandlerCtx = kbHandlerCtx
}
```

And in `handler.go`, add to `HandlerConfig`:

```go
KBHandlerCtx *kbapi.HandlerCtxExt // exposed by kbapi
```

Plus inside the function that builds the mux, after auth middleware has injected actor:

```go
if cfg.KBHandlerCtx != nil {
    kbapi.Register(mux, cfg.KBHandlerCtx)
}
```

Then in `kbapi`, expose a public `HandlerCtxExt` type by aliasing or upgrading the existing `handlerCtx`:

```go
// Add to handler_kb.go:
type HandlerCtxExt = handlerCtx

func NewHandlerCtx(kb kbStore, docs docStore, chunks chunksReader, m membershipChecker, blob blobWriter, jobs jobEnqueue, emb embeddingForSearch, maxBytes int64) *handlerCtx {
    return &handlerCtx{
        kbStore: kb, docStore: docs, chunksRepo: chunks,
        membership: m, blob: blob, jobs: jobs, embedder: emb,
        maxUploadBytes: maxBytes,
    }
}
```

And implement `kbapi.WorkspaceMembership` and `kbapi.WorkspaceBlobAdapter` adapters that wrap the real repos / objectstore — these are thin glue, ~20 lines each.

- [ ] **Step 11: Build everything**

```bash
cd src/services/api && go build ./...
cd ../worker && go build ./...
```

Fix any wiring issues (specific repo method names may differ from the prototype names above; adjust to match what `grep "func NewAccountMembershipRepository" src/services/api/internal/data` shows).

- [ ] **Step 12: Run all backend tests**

```bash
cd src/services/api && go test ./...
cd ../worker && go test ./...
cd ../shared && go test ./...
```

Expected: all PASS.

- [ ] **Step 13: Commit**

```bash
git add src/services/worker/internal/kbingest \
        src/services/worker/internal/data/kb_chunks_repo.go \
        src/services/worker/internal/queue/protocol.go \
        src/services/worker/internal/app/config.go \
        src/services/worker/cmd/worker/run_cloud.go \
        src/services/api/internal/http/kbapi/jobs.go \
        src/services/api/internal/http/kbapi/handler_kb.go \
        src/services/api/internal/app/app.go \
        src/services/api/internal/http/handler.go
git commit -m "feat(worker): kb_ingest job processor + api wiring

Worker now owns the parse→chunk→embed→upsert pipeline driven by
the kb_documents.status state machine. JobQueue.EnqueueRun is
reused with runID=uuid.Nil per the M1.0 design. API enqueues
KBIngestJobType on multipart upload; handler context exposes
NewHandlerCtx for app.go wiring."
```

---

---

## Task 10: kb_search worker tool

**Files:**
- Create: `src/services/worker/internal/tools/builtin/kb/spec.go`
- Create: `src/services/worker/internal/tools/builtin/kb/executor.go`
- Create: `src/services/worker/internal/tools/builtin/kb/executor_test.go`
- Modify: `src/services/worker/internal/tools/builtin/builtin.go` (register the tool)

**Steps:**

- [ ] **Step 1: Look at an existing simple builtin to mirror the structure**

```bash
cat src/services/worker/internal/tools/builtin/web_search/spec.go | head -40
cat src/services/worker/internal/tools/builtin/web_search/executor_test.go | head -30
```

Note the registration pattern (`Spec` constant + `Executor` function).

- [ ] **Step 2: Write executor test**

Create `src/services/worker/internal/tools/builtin/kb/executor_test.go`:

```go
package kb

import (
	"context"
	"encoding/json"
	"testing"

	wdata "arkloop/services/worker/internal/data"

	"github.com/google/uuid"
)

type fakeChunksRepo struct{ hits []wdata.KBChunkHit }

func (f *fakeChunksRepo) Search(ctx context.Context, kbID uuid.UUID, q []float32, k int) ([]wdata.KBChunkHit, error) {
	return f.hits, nil
}
func (f *fakeChunksRepo) Dim() int { return 8 }

type fakeEmbedder struct{ dim int }

func (e fakeEmbedder) Embed(ctx context.Context, t []string) ([][]float32, error) {
	out := make([][]float32, len(t))
	for i := range t {
		v := make([]float32, e.dim)
		v[0] = 1
		out[i] = v
	}
	return out, nil
}
func (e fakeEmbedder) Dim() int { return e.dim }

type fakeAccess struct{ allow bool }

func (f fakeAccess) IsActorWorkspaceMember(ctx context.Context, accountID, kbID uuid.UUID) (bool, error) {
	return f.allow, nil
}

func TestKBSearchHappyPath(t *testing.T) {
	kbID := uuid.New()
	exe := NewExecutor(&fakeChunksRepo{hits: []wdata.KBChunkHit{
		{DocumentRef: "physics.txt", Ordinal: 3, Text: "光的干涉", Score: 0.91},
		{DocumentRef: "physics.txt", Ordinal: 4, Text: "杨氏双缝", Score: 0.74},
	}}, fakeEmbedder{dim: 8}, fakeAccess{allow: true})

	args := map[string]any{"kb_id": kbID.String(), "query": "干涉", "k": 5}
	raw, _ := json.Marshal(args)
	out, err := exe.Execute(context.Background(), uuid.New(), raw)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var resp struct {
		Hits []map[string]any `json:"hits"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Hits) != 2 {
		t.Errorf("got %d hits", len(resp.Hits))
	}
}

func TestKBSearchDeniesNonMember(t *testing.T) {
	exe := NewExecutor(&fakeChunksRepo{}, fakeEmbedder{dim: 8}, fakeAccess{allow: false})
	args := map[string]any{"kb_id": uuid.New().String(), "query": "x"}
	raw, _ := json.Marshal(args)
	_, err := exe.Execute(context.Background(), uuid.New(), raw)
	if err == nil {
		t.Error("expected permission_denied error")
	}
}

func TestKBSearchValidatesInput(t *testing.T) {
	exe := NewExecutor(&fakeChunksRepo{}, fakeEmbedder{dim: 8}, fakeAccess{allow: true})
	// missing kb_id
	raw, _ := json.Marshal(map[string]any{"query": "x"})
	_, err := exe.Execute(context.Background(), uuid.New(), raw)
	if err == nil {
		t.Error("expected error for missing kb_id")
	}
	// missing query
	raw, _ = json.Marshal(map[string]any{"kb_id": uuid.New().String()})
	_, err = exe.Execute(context.Background(), uuid.New(), raw)
	if err == nil {
		t.Error("expected error for missing query")
	}
}
```

- [ ] **Step 3: Implement spec + executor**

Create `src/services/worker/internal/tools/builtin/kb/spec.go`:

```go
// Package kb implements the kb_search builtin tool: semantic retrieval
// over a workspace-scoped knowledge base. M1.2+ adds kb_draft_questions,
// kb_save_questions, kb_compose_paper, kb_list_knowledge_points.
package kb

const (
	ToolNameSearch = "kb_search"
)

// SearchSpec is the JSON schema portion shipped to LLMs.
const SearchSpec = `{
  "name": "kb_search",
  "description": "在指定知识库（KB）中按语义相似度检索教材片段。返回 chunk 列表，每条带 document_ref / 段号 / 章节路径 / 原文与相似度。",
  "parameters": {
    "type": "object",
    "properties": {
      "kb_id": {"type": "string", "description": "KB UUID。"},
      "query": {"type": "string", "description": "中文/英文检索词或问题。"},
      "k":     {"type": "integer", "description": "返回 chunk 数（默认 8，最大 50）。", "default": 8, "maximum": 50}
    },
    "required": ["kb_id", "query"]
  }
}`
```

Create `src/services/worker/internal/tools/builtin/kb/executor.go`:

```go
package kb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"arkloop/services/shared/embedding"
	wdata "arkloop/services/worker/internal/data"

	"github.com/google/uuid"
)

// ChunksReader is the slice of *data.KBChunksRepository the tool needs.
type ChunksReader interface {
	Search(ctx context.Context, kbID uuid.UUID, query []float32, k int) ([]wdata.KBChunkHit, error)
	Dim() int
}

// AccessChecker resolves whether the run's actor account may read a KB.
type AccessChecker interface {
	IsActorWorkspaceMember(ctx context.Context, accountID, kbID uuid.UUID) (bool, error)
}

// Executor wires Search + Embed + Access together. Tool dispatcher passes
// the actor's account_id via context; tests inject directly.
type Executor struct {
	chunks   ChunksReader
	embedder embedding.Embedder
	access   AccessChecker
}

func NewExecutor(chunks ChunksReader, embedder embedding.Embedder, access AccessChecker) *Executor {
	return &Executor{chunks: chunks, embedder: embedder, access: access}
}

// ErrPermissionDenied is mapped to a structured tool result by the dispatcher.
var ErrPermissionDenied = errors.New("kb_search: caller is not a workspace member")

type searchArgs struct {
	KBID  string `json:"kb_id"`
	Query string `json:"query"`
	K     int    `json:"k"`
}

type searchHit struct {
	DocumentRef string   `json:"document_ref"`
	Ordinal     int      `json:"ordinal"`
	HeadingPath []string `json:"heading_path"`
	ChunkType   string   `json:"chunk_type"`
	Text        string   `json:"text"`
	Score       float32  `json:"score"`
}

// Execute runs the tool. accountID identifies the run's caller (resolved
// upstream by the worker run engine from the run's account binding).
func (e *Executor) Execute(ctx context.Context, accountID uuid.UUID, args json.RawMessage) ([]byte, error) {
	var p searchArgs
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("kb_search: invalid args: %w", err)
	}
	if p.KBID == "" {
		return nil, errors.New("kb_search: kb_id is required")
	}
	if p.Query == "" {
		return nil, errors.New("kb_search: query is required")
	}
	kbID, err := uuid.Parse(p.KBID)
	if err != nil {
		return nil, fmt.Errorf("kb_search: invalid kb_id: %w", err)
	}
	if p.K <= 0 {
		p.K = 8
	}
	if p.K > 50 {
		p.K = 50
	}
	ok, err := e.access.IsActorWorkspaceMember(ctx, accountID, kbID)
	if err != nil {
		return nil, fmt.Errorf("kb_search: access check failed: %w", err)
	}
	if !ok {
		return nil, ErrPermissionDenied
	}
	vecs, err := e.embedder.Embed(ctx, []string{p.Query})
	if err != nil {
		return nil, fmt.Errorf("kb_search: embed: %w", err)
	}
	hits, err := e.chunks.Search(ctx, kbID, vecs[0], p.K)
	if err != nil {
		return nil, fmt.Errorf("kb_search: search: %w", err)
	}
	out := make([]searchHit, len(hits))
	for i, h := range hits {
		out[i] = searchHit{
			DocumentRef: h.DocumentRef, Ordinal: h.Ordinal,
			HeadingPath: h.HeadingPath, ChunkType: h.ChunkType,
			Text: h.Text, Score: h.Score,
		}
	}
	return json.Marshal(map[string]any{"hits": out})
}
```

- [ ] **Step 4: Register in builtin tools**

Open `src/services/worker/internal/tools/builtin/builtin.go`. Find where other builtins are listed (search for `exam_recognize_catalog_image` or `web_search`). Add a `kb_search` entry in the same form. Concretely, the file appears to enumerate tool specs in a slice/map — drop in:

```go
{
    Name:    "kb_search",
    Spec:    kb.SearchSpec,
    Factory: func(deps Deps) toolexec.Tool { return wrapKBSearchExecutor(deps) },
},
```

Then implement `wrapKBSearchExecutor` next to where the existing wrappers live. It needs the worker's `kbChunksRepo`, an Embedder, and an `AccessChecker` (the access checker reads `kb.workspace_ref` from `knowledge_bases` and joins `account_memberships`).

The exact registration code shape depends on the existing pattern — copy the closest neighbor verbatim and substitute `kb_search` / `kb` package. If `builtin.go` uses a simple slice of `Tool{Name, Spec, Executor}` records, the change is a 5-liner.

- [ ] **Step 5: Run tests**

```bash
cd src/services/worker
go test ./internal/tools/builtin/kb/... -v
```

Expected: 3 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add src/services/worker/internal/tools/builtin/kb \
        src/services/worker/internal/tools/builtin/builtin.go
git commit -m "feat(worker): add kb_search builtin tool

Semantic retrieval over kb_chunks. Tool resolves caller's account
via dispatcher context, embeds the query through Doubao, runs
cosine search via worker's kb_chunks_repo. Returns hits as JSON
with document_ref / ordinal / heading_path / text / score."
```

---

## Task 11: book-tutor-agent persona

**Files:**
- Create: `src/personas/book-tutor-agent/persona.yaml`
- Create: `src/personas/book-tutor-agent/prompt.md`

**Steps:**

- [ ] **Step 1: Author persona.yaml**

Create `src/personas/book-tutor-agent/persona.yaml`:

```yaml
id: book-tutor-agent
version: "1"
title: 备课助手
description: 帮老师在 ArkLoop 中创建知识库、上传 .txt 教材并按语义搜索内容。出题/组卷功能将在后续版本提供。
soul_file: prompt.md
user_selectable: true
selector_name: 备课助手
selector_order: 6
budgets:
    reasoning_iterations: 30
    temperature: 0.3
reasoning_mode: auto
prompt_cache_control: system_prompt
executor_type: agent.simple
```

- [ ] **Step 2: Author prompt.md**

Create `src/personas/book-tutor-agent/prompt.md`:

```markdown
# 你的角色

你是**备课助手**（book-tutor-agent），帮老师把上传到 ArkLoop 的教材做语义搜索。

# M1.0 能力边界

**支持**：
- 帮老师确认要操作哪个知识库（KB）
- 调 `kb_search` 在指定 KB 中按语义检索教材片段
- 把命中的章节/段落清晰展示给老师，便于核对

**暂不支持**（M1.2 / M1.3 提供，明确告知用户即可）：
- 基于教材生成题目（出题）
- 组卷与试卷导出
- 创建/上传/删除 KB 或文档（让老师去 console-lite 的"知识库"页操作）

# 工作原则

- **意图判定先行**：老师每条消息先判断是要"搜内容"、"问 KB 状态"，还是要做不支持的操作。不支持的操作明确告知"该功能在后续版本提供"，不要装作能做。
- **必须先确认 KB**：第一次互动用 `ask_user` 让老师指明操作哪个 KB（提供"我会调用 kb_search 搜你刚刚上传的 KB，请告诉我 KB 的 ID 或名字"的引导）。后续 turn 记住选定的 KB。
- **kb_search 前先讲清楚**：调用前用一句话告诉老师"我打算搜 X"，避免误检索。
- **不臆造内容**：所有展示给老师的教材内容必须来自 `kb_search` 的命中结果。不要根据训练记忆补充。

# 工作流：教材搜索

1. 用 `ask_user` 确认 KB（如果还没确认）
2. 用一句话告诉老师即将搜索什么
3. 调 `kb_search(kb_id, query, k=5)`
4. 按相似度排序展示命中：每条显示 "**[文档名 - 段落 N]** 段落前 200 字...（相似度 0.XX）"
5. 询问老师是否需要继续搜其他相关内容；老师说"够了"或"试试别的关键词" → 配合调整

# 边界与禁止

- 不创建 KB、不上传文档、不删除文档：明确告知老师去 console-lite "知识库"管理页操作
- 不出题、不组卷：明确告知"出题与组卷功能将在 M1.2 / M1.3 版本提供"
- 不试图猜 kb_id：不知道就 `ask_user`
- 工具报错原样转告，不要包装成"系统繁忙"

# 风格

- 中文回复，正式但不繁琐
- 每一步先用一句话告诉老师"我要做什么"，再调工具
- 工具返回结果后用一两句话概括，不要把 JSON 原样吐给老师
```

- [ ] **Step 3: Verify persona loads at startup**

```bash
cd src/services/worker
go build ./...
# Start the worker briefly and check logs include the new persona id.
# (Or run any persona-loading test that exists; search for "persona" in worker tests.)
grep -rn "book-tutor-agent\|exam-agent.*Load\|LoadPersona" src/services/worker/internal/personas/ 2>&1 | head
```

Expected: no compile errors. Persona loading is data-driven, so adding the directory should be enough.

- [ ] **Step 4: Commit**

```bash
git add src/personas/book-tutor-agent
git commit -m "feat(personas): add book-tutor-agent persona for M1.0

User-selectable persona scoped to KB search only. Prompt explicitly
sets M1.0 boundaries: no question generation, no paper composition,
no KB management (those live in console-lite or future milestones).
Workflow: ask_user → kb_search → present hits."
```

---

## Task 12: console-lite — KB list page + create modal

**Files:**
- Create: `src/apps/console-lite/src/api/knowledge-bases.ts`
- Create: `src/apps/console-lite/src/pages/KnowledgeBasesPage.tsx`
- Create: `src/apps/console-lite/src/components/CreateKBModal.tsx`
- Modify: `src/apps/console-lite/src/App.tsx` (add route)
- Modify: `src/apps/console-lite/src/layouts/LiteLayout.tsx` (add nav item)
- Modify: `src/apps/console-lite/src/locales/en.ts` and `zh.ts` (add labels)

**Steps:**

- [ ] **Step 1: API client module**

Create `src/apps/console-lite/src/api/knowledge-bases.ts`:

```typescript
import { apiFetch } from './client'

export interface KnowledgeBase {
  id: string
  name: string
  workspace_ref: string
  description: string
  document_count?: number
  created_at: string
}

export interface KBDocument {
  id: string
  original_filename: string
  mime_type: string
  size_bytes: number
  status: 'queued' | 'parsing' | 'chunking' | 'embedding' | 'upserting' | 'ready' | 'failed'
  error_message: string
  parse_meta?: Record<string, unknown>
  created_at: string
  updated_at: string
}

export interface SearchHit {
  document_ref: string
  ordinal: number
  heading_path: string[]
  chunk_type: string
  text: string
  score: number
}

export async function listKnowledgeBases(workspaceRef: string, accessToken: string): Promise<KnowledgeBase[]> {
  const resp = await apiFetch<{ items: KnowledgeBase[] }>(
    `/v1/knowledge-bases?workspace_ref=${encodeURIComponent(workspaceRef)}`,
    { accessToken }
  )
  return resp.items ?? []
}

export async function createKnowledgeBase(
  body: { name: string; workspace_ref: string; description?: string },
  accessToken: string
): Promise<KnowledgeBase> {
  return apiFetch<KnowledgeBase>('/v1/knowledge-bases', {
    method: 'POST',
    body: JSON.stringify(body),
    accessToken,
  })
}

export async function deleteKnowledgeBase(id: string, accessToken: string): Promise<void> {
  await apiFetch<void>(`/v1/knowledge-bases/${id}`, {
    method: 'DELETE',
    accessToken,
  })
}

export async function getKnowledgeBase(id: string, accessToken: string): Promise<KnowledgeBase> {
  return apiFetch<KnowledgeBase>(`/v1/knowledge-bases/${id}`, { accessToken })
}

export async function listDocuments(kbId: string, accessToken: string): Promise<KBDocument[]> {
  const resp = await apiFetch<{ items: KBDocument[] }>(
    `/v1/knowledge-bases/${kbId}/documents`,
    { accessToken }
  )
  return resp.items ?? []
}

export async function getDocument(kbId: string, docId: string, accessToken: string): Promise<KBDocument> {
  return apiFetch<KBDocument>(`/v1/knowledge-bases/${kbId}/documents/${docId}`, { accessToken })
}

export async function deleteDocument(kbId: string, docId: string, accessToken: string): Promise<void> {
  await apiFetch<void>(`/v1/knowledge-bases/${kbId}/documents/${docId}`, {
    method: 'DELETE',
    accessToken,
  })
}

// Multipart upload bypasses apiFetch's JSON assumption.
export async function uploadDocument(
  kbId: string,
  file: File,
  accessToken: string
): Promise<{ document_id: string; job_id: string }> {
  const fd = new FormData()
  fd.append('file', file)
  const resp = await fetch(`/v1/knowledge-bases/${kbId}/documents`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${accessToken}` },
    body: fd,
  })
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(`upload failed (${resp.status}): ${text}`)
  }
  return resp.json()
}

export async function searchKnowledgeBase(
  kbId: string,
  query: string,
  k: number,
  accessToken: string
): Promise<SearchHit[]> {
  const resp = await apiFetch<{ hits: SearchHit[] }>(
    `/v1/knowledge-bases/${kbId}/search?q=${encodeURIComponent(query)}&k=${k}`,
    { accessToken }
  )
  return resp.hits ?? []
}
```

- [ ] **Step 2: Create KB modal**

Create `src/apps/console-lite/src/components/CreateKBModal.tsx`:

```tsx
import { useState, useCallback } from 'react'
import { Modal } from './Modal'
import { FormField } from './FormField'
import { createKnowledgeBase } from '../api/knowledge-bases'
import { useLocale } from '../contexts/LocaleContext'

interface Props {
  isOpen: boolean
  onClose: () => void
  onCreated: (kbId: string) => void
  accessToken: string
  workspaceRef: string
}

export function CreateKBModal({ isOpen, onClose, onCreated, accessToken, workspaceRef }: Props) {
  const { t } = useLocale()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleSubmit = useCallback(async () => {
    if (!name.trim()) return
    setSubmitting(true)
    setError(null)
    try {
      const kb = await createKnowledgeBase({ name: name.trim(), workspace_ref: workspaceRef, description }, accessToken)
      onCreated(kb.id)
      setName('')
      setDescription('')
      onClose()
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      setError(msg)
    } finally {
      setSubmitting(false)
    }
  }, [name, description, workspaceRef, accessToken, onCreated, onClose])

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={t.kbCreateTitle}>
      <div className="space-y-4">
        <FormField label={t.kbNameLabel}>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. 大学物理（上）"
            className="w-full px-3 py-2 border rounded"
          />
        </FormField>
        <FormField label={t.kbDescLabel}>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={3}
            className="w-full px-3 py-2 border rounded"
          />
        </FormField>
        {error && <div className="text-red-600 text-sm">{error}</div>}
        <div className="flex justify-end gap-2">
          <button onClick={onClose} className="px-4 py-2 border rounded">
            {t.cancel}
          </button>
          <button
            onClick={handleSubmit}
            disabled={!name.trim() || submitting}
            className="px-4 py-2 bg-blue-600 text-white rounded disabled:opacity-50"
          >
            {submitting ? t.creating : t.create}
          </button>
        </div>
      </div>
    </Modal>
  )
}
```

- [ ] **Step 3: Knowledge-bases list page**

Create `src/apps/console-lite/src/pages/KnowledgeBasesPage.tsx`:

```tsx
import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { PageHeader } from '../components/PageHeader'
import { DataTable } from '../components/DataTable'
import { EmptyState } from '../components/EmptyState'
import { CreateKBModal } from '../components/CreateKBModal'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { ErrorCallout } from '@arkloop/shared'
import { listKnowledgeBases, deleteKnowledgeBase, type KnowledgeBase } from '../api/knowledge-bases'
import { useLocale } from '../contexts/LocaleContext'
import { useAuth } from '../contexts/AuthContext' // exposes accessToken + workspaceRef; if not present, adapt to existing context

export function KnowledgeBasesPage() {
  const { t } = useLocale()
  const navigate = useNavigate()
  const { accessToken, workspaceRef } = useAuth()
  const [kbs, setKBs] = useState<KnowledgeBase[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [createOpen, setCreateOpen] = useState(false)
  const [pendingDelete, setPendingDelete] = useState<KnowledgeBase | null>(null)

  const refresh = useCallback(async () => {
    if (!accessToken || !workspaceRef) return
    setLoading(true)
    setError(null)
    try {
      const items = await listKnowledgeBases(workspaceRef, accessToken)
      setKBs(items)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }, [accessToken, workspaceRef])

  useEffect(() => {
    refresh()
  }, [refresh])

  const handleDelete = useCallback(async () => {
    if (!pendingDelete || !accessToken) return
    try {
      await deleteKnowledgeBase(pendingDelete.id, accessToken)
      setPendingDelete(null)
      await refresh()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [pendingDelete, accessToken, refresh])

  return (
    <div className="p-6">
      <PageHeader
        title={t.kbPageTitle}
        action={
          <button onClick={() => setCreateOpen(true)} className="px-4 py-2 bg-blue-600 text-white rounded">
            {t.kbCreateButton}
          </button>
        }
      />
      {error && <ErrorCallout title={t.error} message={error} />}
      {loading ? (
        <div className="text-gray-500">{t.loading}</div>
      ) : kbs.length === 0 ? (
        <EmptyState title={t.kbEmptyTitle} description={t.kbEmptyDescription} />
      ) : (
        <DataTable
          columns={[
            { key: 'name', label: t.kbColName, render: (kb: KnowledgeBase) => kb.name },
            { key: 'docs', label: t.kbColDocs, render: (kb: KnowledgeBase) => String(kb.document_count ?? 0) },
            {
              key: 'created',
              label: t.kbColCreated,
              render: (kb: KnowledgeBase) => new Date(kb.created_at).toLocaleString(),
            },
            {
              key: 'actions',
              label: '',
              render: (kb: KnowledgeBase) => (
                <div className="flex gap-2">
                  <button onClick={() => navigate(`/knowledge-bases/${kb.id}`)} className="text-blue-600">
                    {t.open}
                  </button>
                  <button onClick={() => setPendingDelete(kb)} className="text-red-600">
                    {t.delete}
                  </button>
                </div>
              ),
            },
          ]}
          rows={kbs}
          rowKey={(kb) => kb.id}
        />
      )}
      <CreateKBModal
        isOpen={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={(id) => navigate(`/knowledge-bases/${id}`)}
        accessToken={accessToken ?? ''}
        workspaceRef={workspaceRef ?? ''}
      />
      <ConfirmDialog
        isOpen={pendingDelete !== null}
        title={t.kbDeleteTitle}
        description={pendingDelete ? `${t.kbDeleteConfirm}: ${pendingDelete.name}` : ''}
        onConfirm={handleDelete}
        onCancel={() => setPendingDelete(null)}
      />
    </div>
  )
}
```

- [ ] **Step 4: Wire route + nav**

In `src/apps/console-lite/src/App.tsx`, add the route inside the authenticated `<Routes>` block:

```tsx
<Route path="knowledge-bases" element={<KnowledgeBasesPage />} />
<Route path="knowledge-bases/:id" element={<KnowledgeBaseDetailPage />} />
```

Import: `import { KnowledgeBasesPage } from './pages/KnowledgeBasesPage'` (and the detail page once Task 13 creates it; for now, comment out the detail line or use a placeholder route).

In `src/apps/console-lite/src/layouts/LiteLayout.tsx`, find the `buildNavItems` function and add an item:

```tsx
{ to: '/knowledge-bases', label: t.navKnowledgeBases, icon: BookOpenIcon } // import BookOpen from lucide-react
```

- [ ] **Step 5: Add locale strings**

In both `src/apps/console-lite/src/locales/en.ts` and `zh.ts`, add:

```typescript
// en.ts:
navKnowledgeBases: 'Knowledge Bases',
kbPageTitle: 'Knowledge Bases',
kbCreateButton: 'Create',
kbCreateTitle: 'Create knowledge base',
kbNameLabel: 'Name',
kbDescLabel: 'Description',
kbColName: 'Name',
kbColDocs: 'Documents',
kbColCreated: 'Created',
kbEmptyTitle: 'No knowledge bases yet',
kbEmptyDescription: 'Create your first KB and upload teaching materials.',
kbDeleteTitle: 'Delete knowledge base',
kbDeleteConfirm: 'This will permanently delete the KB and all its documents',
creating: 'Creating…',
create: 'Create',
open: 'Open',
delete: 'Delete',
cancel: 'Cancel',
loading: 'Loading…',
error: 'Error',

// zh.ts:
navKnowledgeBases: '知识库',
kbPageTitle: '知识库',
kbCreateButton: '新建',
kbCreateTitle: '新建知识库',
kbNameLabel: '名称',
kbDescLabel: '描述',
kbColName: '名称',
kbColDocs: '文档数',
kbColCreated: '创建时间',
kbEmptyTitle: '还没有知识库',
kbEmptyDescription: '新建第一个知识库，上传教材。',
kbDeleteTitle: '删除知识库',
kbDeleteConfirm: '这将永久删除知识库及其所有文档',
creating: '创建中…',
create: '创建',
open: '打开',
delete: '删除',
cancel: '取消',
loading: '加载中…',
error: '错误',
```

If the locale type is strictly typed (look at the existing pattern), also add these keys to the type definition.

- [ ] **Step 6: Build + type check**

```bash
cd src/apps/console-lite
pnpm install
pnpm type-check
pnpm lint
pnpm build
```

Expected: clean build. Watch for any auth context shape mismatches — adjust the `useAuth()` import to match what console-lite actually exports.

- [ ] **Step 7: Commit**

```bash
git add src/apps/console-lite/src/api/knowledge-bases.ts \
        src/apps/console-lite/src/pages/KnowledgeBasesPage.tsx \
        src/apps/console-lite/src/components/CreateKBModal.tsx \
        src/apps/console-lite/src/App.tsx \
        src/apps/console-lite/src/layouts/LiteLayout.tsx \
        src/apps/console-lite/src/locales/en.ts \
        src/apps/console-lite/src/locales/zh.ts
git commit -m "feat(console-lite): KB list page + create modal

New '知识库' nav item. List shows name / document_count / created_at,
plus actions to open detail page or delete. Create modal POSTs to
the KB API; on success redirects to detail page (Task 13). All
strings localised in en.ts / zh.ts."
```

---

## Task 13: console-lite — KB detail page (docs + upload + status polling + search debug)

**Files:**
- Create: `src/apps/console-lite/src/pages/KnowledgeBaseDetailPage.tsx`
- Modify: `src/apps/console-lite/src/App.tsx` (uncomment the detail route from Task 12)
- Modify: `src/apps/console-lite/src/locales/en.ts` and `zh.ts` (add detail-specific labels)

**Steps:**

- [ ] **Step 1: Detail page**

Create `src/apps/console-lite/src/pages/KnowledgeBaseDetailPage.tsx`:

```tsx
import { useState, useEffect, useCallback, useRef } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { PageHeader } from '../components/PageHeader'
import { Badge } from '../components/Badge'
import { DataTable } from '../components/DataTable'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { ErrorCallout } from '@arkloop/shared'
import {
  getKnowledgeBase,
  listDocuments,
  uploadDocument,
  deleteDocument,
  searchKnowledgeBase,
  type KnowledgeBase,
  type KBDocument,
  type SearchHit,
} from '../api/knowledge-bases'
import { useLocale } from '../contexts/LocaleContext'
import { useAuth } from '../contexts/AuthContext'

const POLL_INTERVAL_MS = 3000
const TERMINAL_STATUSES = new Set(['ready', 'failed'])

export function KnowledgeBaseDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { t } = useLocale()
  const { accessToken } = useAuth()
  const fileInputRef = useRef<HTMLInputElement>(null)

  const [kb, setKB] = useState<KnowledgeBase | null>(null)
  const [docs, setDocs] = useState<KBDocument[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [pendingDelete, setPendingDelete] = useState<KBDocument | null>(null)
  const [uploading, setUploading] = useState(false)

  // Search debug state
  const [searchQ, setSearchQ] = useState('')
  const [searchK, setSearchK] = useState(8)
  const [searchHits, setSearchHits] = useState<SearchHit[]>([])
  const [searching, setSearching] = useState(false)
  const [searchError, setSearchError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    if (!id || !accessToken) return
    try {
      const [kbRes, docsRes] = await Promise.all([
        getKnowledgeBase(id, accessToken),
        listDocuments(id, accessToken),
      ])
      setKB(kbRes)
      setDocs(docsRes)
      setError(null)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }, [id, accessToken])

  useEffect(() => {
    refresh()
  }, [refresh])

  // Poll while any doc is in a non-terminal state
  useEffect(() => {
    const hasInflight = docs.some((d) => !TERMINAL_STATUSES.has(d.status))
    if (!hasInflight) return
    const handle = setInterval(refresh, POLL_INTERVAL_MS)
    return () => clearInterval(handle)
  }, [docs, refresh])

  const handleUpload = useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0]
      if (!file || !id || !accessToken) return
      setUploading(true)
      setError(null)
      try {
        await uploadDocument(id, file, accessToken)
        await refresh()
      } catch (err: unknown) {
        setError(err instanceof Error ? err.message : String(err))
      } finally {
        setUploading(false)
        if (fileInputRef.current) fileInputRef.current.value = ''
      }
    },
    [id, accessToken, refresh]
  )

  const handleDelete = useCallback(async () => {
    if (!pendingDelete || !id || !accessToken) return
    try {
      await deleteDocument(id, pendingDelete.id, accessToken)
      setPendingDelete(null)
      await refresh()
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err))
    }
  }, [pendingDelete, id, accessToken, refresh])

  const handleSearch = useCallback(async () => {
    if (!id || !accessToken || !searchQ.trim()) return
    setSearching(true)
    setSearchError(null)
    try {
      const hits = await searchKnowledgeBase(id, searchQ.trim(), searchK, accessToken)
      setSearchHits(hits)
    } catch (err: unknown) {
      setSearchError(err instanceof Error ? err.message : String(err))
    } finally {
      setSearching(false)
    }
  }, [id, searchQ, searchK, accessToken])

  if (loading) return <div className="p-6 text-gray-500">{t.loading}</div>
  if (!kb) return <div className="p-6">{t.kbNotFound}</div>

  return (
    <div className="p-6 space-y-6">
      <PageHeader title={kb.name} subtitle={kb.description || undefined} backTo="/knowledge-bases" />
      {error && <ErrorCallout title={t.error} message={error} />}

      <section>
        <div className="flex justify-between items-center mb-3">
          <h2 className="text-lg font-semibold">{t.kbDocs}</h2>
          <div>
            <input
              ref={fileInputRef}
              type="file"
              accept=".txt,.md"
              onChange={handleUpload}
              className="hidden"
              id="kb-doc-upload"
            />
            <label
              htmlFor="kb-doc-upload"
              className={`inline-block px-4 py-2 rounded cursor-pointer ${
                uploading ? 'bg-gray-300' : 'bg-blue-600 text-white'
              }`}
            >
              {uploading ? t.uploading : t.upload}
            </label>
          </div>
        </div>
        {docs.length === 0 ? (
          <div className="text-gray-500">{t.kbDocsEmpty}</div>
        ) : (
          <DataTable
            columns={[
              { key: 'filename', label: t.kbColFilename, render: (d: KBDocument) => d.original_filename },
              {
                key: 'status',
                label: t.kbColStatus,
                render: (d: KBDocument) => (
                  <span title={d.error_message || undefined}>
                    <Badge tone={statusToTone(d.status)}>{d.status}</Badge>
                    {d.status === 'failed' && d.error_message && (
                      <span className="ml-2 text-xs text-red-600">{d.error_message}</span>
                    )}
                  </span>
                ),
              },
              {
                key: 'size',
                label: t.kbColSize,
                render: (d: KBDocument) => formatBytes(d.size_bytes),
              },
              {
                key: 'created',
                label: t.kbColCreated,
                render: (d: KBDocument) => new Date(d.created_at).toLocaleString(),
              },
              {
                key: 'actions',
                label: '',
                render: (d: KBDocument) => (
                  <button onClick={() => setPendingDelete(d)} className="text-red-600">
                    {t.delete}
                  </button>
                ),
              },
            ]}
            rows={docs}
            rowKey={(d) => d.id}
          />
        )}
      </section>

      <section className="border-t pt-6">
        <h2 className="text-lg font-semibold mb-3">{t.kbSearchDebug}</h2>
        <div className="flex gap-2 mb-3">
          <input
            type="text"
            value={searchQ}
            onChange={(e) => setSearchQ(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') handleSearch()
            }}
            placeholder={t.kbSearchPlaceholder}
            className="flex-1 px-3 py-2 border rounded"
          />
          <input
            type="number"
            min={1}
            max={50}
            value={searchK}
            onChange={(e) => setSearchK(Number(e.target.value) || 8)}
            className="w-20 px-3 py-2 border rounded"
          />
          <button
            onClick={handleSearch}
            disabled={searching || !searchQ.trim()}
            className="px-4 py-2 bg-blue-600 text-white rounded disabled:opacity-50"
          >
            {searching ? t.loading : t.search}
          </button>
        </div>
        {searchError && <ErrorCallout title={t.error} message={searchError} />}
        {searchHits.length > 0 && (
          <div className="space-y-2">
            {searchHits.map((h, i) => (
              <div key={i} className="border rounded p-3 bg-white">
                <div className="text-xs text-gray-500 mb-1">
                  [{h.document_ref} - 段落 {h.ordinal}] {t.kbSearchScore}: {h.score.toFixed(3)}
                </div>
                <div className="text-sm">{h.text.slice(0, 200)}{h.text.length > 200 ? '…' : ''}</div>
              </div>
            ))}
          </div>
        )}
      </section>

      <ConfirmDialog
        isOpen={pendingDelete !== null}
        title={t.kbDocDeleteTitle}
        description={pendingDelete ? `${t.kbDocDeleteConfirm}: ${pendingDelete.original_filename}` : ''}
        onConfirm={handleDelete}
        onCancel={() => setPendingDelete(null)}
      />
    </div>
  )
}

function statusToTone(status: string): 'default' | 'success' | 'warning' | 'error' {
  switch (status) {
    case 'ready':
      return 'success'
    case 'failed':
      return 'error'
    case 'queued':
      return 'default'
    default:
      return 'warning'
  }
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`
}
```

> **Adapter notes:** `DataTable` / `Badge` / `PageHeader` / `ConfirmDialog` shapes shown match the existing console-lite component conventions seen in `RunsPage.tsx`. If the real prop names differ (e.g. `columns`/`data` vs `columns`/`rows`), match what those existing pages do — the structure is the same.

- [ ] **Step 2: Enable the route in App.tsx**

In `src/apps/console-lite/src/App.tsx`, ensure both routes are present:

```tsx
<Route path="knowledge-bases" element={<KnowledgeBasesPage />} />
<Route path="knowledge-bases/:id" element={<KnowledgeBaseDetailPage />} />
```

And the import:
```tsx
import { KnowledgeBaseDetailPage } from './pages/KnowledgeBaseDetailPage'
```

- [ ] **Step 3: Add detail-page locale strings**

In `src/apps/console-lite/src/locales/en.ts` and `zh.ts`:

```typescript
// en.ts:
kbDocs: 'Documents',
kbDocsEmpty: 'No documents yet — upload a .txt or .md file.',
kbColFilename: 'Filename',
kbColStatus: 'Status',
kbColSize: 'Size',
upload: 'Upload',
uploading: 'Uploading…',
kbSearchDebug: 'Search (debug)',
kbSearchPlaceholder: 'Type a query and press Enter…',
kbSearchScore: 'score',
search: 'Search',
kbDocDeleteTitle: 'Delete document',
kbDocDeleteConfirm: 'This will permanently delete the document and its chunks',
kbNotFound: 'Knowledge base not found.',

// zh.ts:
kbDocs: '文档',
kbDocsEmpty: '还没有文档 — 上传一份 .txt 或 .md 文件。',
kbColFilename: '文件名',
kbColStatus: '状态',
kbColSize: '大小',
upload: '上传',
uploading: '上传中…',
kbSearchDebug: '搜索（调试）',
kbSearchPlaceholder: '输入关键词后回车…',
kbSearchScore: '相似度',
search: '搜索',
kbDocDeleteTitle: '删除文档',
kbDocDeleteConfirm: '这将永久删除该文档及其切片',
kbNotFound: '未找到该知识库。',
```

- [ ] **Step 4: Build + type check**

```bash
cd src/apps/console-lite
pnpm type-check
pnpm lint
pnpm build
```

Expected: clean.

- [ ] **Step 5: Manual smoke (optional — full E2E is Task 14)**

Start a local stack (api + worker + postgres + console-lite dev server). In the browser, hit `/knowledge-bases`, create one, navigate into it, try the upload + search flow.

- [ ] **Step 6: Commit**

```bash
git add src/apps/console-lite/src/pages/KnowledgeBaseDetailPage.tsx \
        src/apps/console-lite/src/App.tsx \
        src/apps/console-lite/src/locales/en.ts \
        src/apps/console-lite/src/locales/zh.ts
git commit -m "feat(console-lite): KB detail page with upload + status poll + search debug

Detail page lists docs with status badges, polls every 3s while any
doc is in a non-terminal state, accepts .txt/.md uploads (10MB cap
enforced server-side), shows search results inline with score.
Status badge tone derives from kb_documents.status enum."
```

---

## Task 14: End-to-end acceptance walkthrough

**Files:**
- No code changes. Verification only.
- Optional: add `docs/superpowers/specs/2026-05-21-book-kb-rag-m1.0-acceptance.md` with checklist results.

**Steps:**

- [ ] **Step 1: Bring up the full stack**

```bash
cd /Users/jzefan/work/proj/ArkLoop
docker compose up -d postgres redis minio pgbouncer
cd src/services/api && ARKLOOP_DATABASE_URL=... ARK_API_KEY=... go run ./cmd/migrate up
cd src/services/api && ARK_API_KEY=... go run ./cmd/api &
cd src/services/worker && ARK_API_KEY=... go run ./cmd/worker &
cd src/apps/console-lite && pnpm dev &
```

Wait for all three to settle.

- [ ] **Step 2: Acceptance check 1 — Create a KB**

In console-lite, navigate to "知识库" → click "新建" → enter `大学物理（上）` → submit.

✅ Pass criteria: Page redirects to detail. KB appears in the list when you go back.

- [ ] **Step 3: Acceptance check 2 — Upload a 1MB .txt**

Prepare a ~1MB Chinese physics text file (or 50万字符sample). Use the "上传" button on the detail page.

✅ Pass criteria: Upload completes within a few seconds. Document appears in the doc list with `queued` status.

- [ ] **Step 4: Acceptance check 3 — Observe state machine transitions**

Watch the status column. Within 60s it should pass through `queued → parsing → chunking → embedding → upserting → ready`.

✅ Pass criteria: All transitions observed (polling captures most). Final status is `ready`. `parse_meta` shows `chunk_count` ≥ 1.

- [ ] **Step 5: Acceptance check 4 — Search via debug input**

In the detail page's "搜索（调试）" section, type `光的干涉`, k=3, click "搜索".

✅ Pass criteria: At least one hit returned with score ≥ 0.5. Top hit's text contains "光" or "干涉".

- [ ] **Step 6: Acceptance check 5 — Search via book-tutor-agent**

Open chat → select persona "备课助手" → say "搜光的干涉，从 KB `<paste-kb-id-here>` 里找".

✅ Pass criteria: Persona invokes `kb_search`, returns the same hits as Step 5, formatted as "[文档名 - 段落 N] 内容前 200 字..." per the persona prompt.

- [ ] **Step 7: Acceptance check 6 — Workspace isolation**

Log in as a second user in the same workspace.

✅ Pass criteria: They see the same KB and documents.

Log out, then log in as a user in a **different** account/workspace.

✅ Pass criteria: They do **not** see the KB. Direct `GET /v1/knowledge-bases/<id>` returns 404.

- [ ] **Step 8: Acceptance check 7 — Document deletion cascade**

As the original user, delete a document via the trash button.

✅ Pass criteria: Document disappears from list. Search no longer returns hits from that document. Connect to postgres and confirm `kb_chunks WHERE document_id = '<deleted>'` returns 0 rows.

- [ ] **Step 9: Document the acceptance run**

Create `docs/superpowers/specs/2026-05-21-book-kb-rag-m1.0-acceptance.md` with the 7 checks marked as PASS / FAIL plus any observations (latency, edge cases hit).

- [ ] **Step 10: Final commit + merge**

```bash
# If acceptance doc was created:
git add docs/superpowers/specs/2026-05-21-book-kb-rag-m1.0-acceptance.md
git commit -m "docs(spec): M1.0 acceptance walkthrough — all 7 checks pass"

# Merge into main:
git checkout main
git merge --no-ff feature/book-kb-m1.0 -m "feat: book-kb-rag M1.0 — KB infrastructure end-to-end on .txt

7 acceptance checks passed. Schema, REST CRUD, worker ingest job,
kb_search tool, book-tutor-agent persona, and console-lite UI all
shipping. Next: Spike S1 (PDF parsing quality) + M1.1 (PDF support)."
git push
```

---

## Final M1.0 Verification Checklist

After Task 14, the following must all be true:

- [ ] `cd src/services/api && go test ./... && cd ../worker && go test ./... && cd ../shared && go test ./...` — all PASS
- [ ] `cd src/apps/console-lite && pnpm type-check && pnpm lint && pnpm build` — all PASS
- [ ] Migration `00193_kb_full_schema.sql` applied; `\d kb_chunks` shows the FK shape
- [ ] M0 `/v1/_debug/kb/*` routes return 404 (verify by curl)
- [ ] New routes return JSON shapes matching the design
- [ ] Worker logs show `kb_ingest handler enabled` on startup
- [ ] `book-tutor-agent` appears in the persona selector
- [ ] All 7 acceptance checks documented as PASS

## What M1.0 Explicitly Does Not Address

Carried into M1.1 onward, per the design doc:

- PDF / DOCX / scanned-page parsing (M1.1; depends on Spike S1)
- OCR for scanned content (M1.1)
- Image / table / formula extraction (M1.1)
- `kb_knowledge_points` writes (M1.2)
- `kb_questions` / `kb_papers` tables and writes (M1.2 / M1.3)
- RAG question generation (`kb_draft_questions`, `kb_save_questions`) (M1.2)
- Paper composition (PaperComposer, `kb_compose_paper`) (M1.3)
- exam-system integration (M2; depends on Spike S2)
- Batch upload, KB rename, chunk-level browsing, SSE push
- Multi-user write-conflict handling beyond what the unique constraint catches
