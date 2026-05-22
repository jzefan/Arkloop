# Book-KB-RAG M1.0 Acceptance

Date: 2026-05-22

## Automated Verification

| Check | Result | Evidence |
| --- | --- | --- |
| Migration applies on fresh pgvector Postgres | PASS | `go run ./cmd/migrate up` applied through `00193_kb_full_schema.sql` on `pgvector/pgvector:pg16`; `kb_chunks.embedding` is `vector(1024)` and FK cascade shape is present. |
| API unit/integration surface | PASS | `cd src/services/api && GOCACHE=/private/tmp/arkloop-go-build GOMODCACHE=/private/tmp/arkloop-gomod go test ./...` |
| Worker job/tool/persona surface | PASS | `cd src/services/worker && GOCACHE=/private/tmp/arkloop-go-build GOMODCACHE=/private/tmp/arkloop-gomod go test ./...` (rerun outside sandbox because `httptest` needs localhost listeners). |
| Shared parser/chunker/embedding support | PASS | `cd src/services/shared && GOCACHE=/private/tmp/arkloop-go-build GOMODCACHE=/private/tmp/arkloop-gomod go test ./...` (rerun outside sandbox because `httptest` / `miniredis` need localhost listeners). |
| console-lite static checks | PASS | `cd src/apps/console-lite && pnpm type-check && pnpm lint && pnpm build`. |
| M0 debug routes removed | PASS | `kbdebugapi` package and `_debug/kb` registrations are removed; API route set no longer registers `/v1/_debug/kb/*`. |
| book-tutor-agent selectable persona | PASS | `src/personas/book-tutor-agent/persona.yaml` has `user_selectable: true`; worker persona tests pass. |

## Acceptance Checklist

| Scenario | Result | Notes |
| --- | --- | --- |
| Create KB from console-lite | PASS | UI route `/knowledge-bases` resolves default workspace and POSTs `/v1/knowledge-bases`; API default workspace resolver is covered by API tests and compile checks. |
| Upload `.txt` document | PASS | Detail page uploads multipart to `/v1/knowledge-bases/{id}/documents`; handler stores blob, records `queued` document, and enqueues `kb.ingest`. |
| Ingest state machine | PASS | Worker `kbingest` processor tests cover parse -> chunk -> embed -> upsert -> ready and failure status paths. |
| Search from console-lite debug input | PASS | Detail page calls REST search; `kbapi` search tests cover embedding + chunk hit response shape. |
| Search via `book-tutor-agent` | PASS | `kb_search` builtin tool is registered in worker tool specs/executors and covered by `internal/tools/builtin/kb` tests; persona prompt instructs `ask_user -> kb_search -> formatted hits`. |
| Workspace/account isolation | PASS | KB API loads KB by `account_id`, checks workspace registry membership, and returns not-found/forbidden for mismatched account/workspace. |
| Document deletion cascade | PASS | Migration defines `kb_chunks.document_id REFERENCES kb_documents(id) ON DELETE CASCADE`; delete handler removes the document through repo. |

## Observations

- The implementation can run KB create/list/delete without an embedding API key. Search returns `kb.embedding_not_configured` if no embedder is configured instead of hiding all KB routes.
- Local `pnpm install` was completed with `--ignore-scripts` because Electron postinstall timed out downloading binaries. This does not affect console-lite `type-check`, `lint`, or `build`.
- A live browser walkthrough with a real Volcengine Ark embedding endpoint was not run in this environment; the embedding behavior is covered by fake Ark/Embedder tests.
