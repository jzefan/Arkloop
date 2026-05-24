# ArkLoop ↔ exam — API Contract (draft)

> **Status**: draft, ready for review by exam backend team
> **Owner**: jzefan (ArkLoop side) + TBD (exam side)
> **Created**: 2026-05-21
> **Blocks**: ArkLoop M2 (Linked-mode exam integration) — see `docs/superpowers/specs/2026-05-21-book-kb-rag-design.md` Spike S2
>
> This document is **proposal-shaped**: ArkLoop has written down what it wants. exam-side team should accept, reject, or counter-propose each endpoint. Once both sides initial this doc, it becomes a frozen contract.

## Scope

This contract covers the **4 new endpoints** that exam must expose so ArkLoop can:
1. Browse exam's existing knowledge-point tree and question bank (for RAG retrieval + few-shot reference)
2. Write back AI-drafted questions to exam (after teacher confirmation)
3. Write back composed papers (with question mappings)

**Out of scope** (already shipped, do NOT touch):
- The existing 4 exam tools used by `exam-agent`: `exam_recognize_catalog_image`, `exam_parse_catalog_excel`, `exam_create_catalog_tree`, `exam_generate_questions`. These keep their current shapes.

## Auth

All endpoints below run **as the teacher's OIDC token** (the same SSO flow already powering existing `exam_*` tools). ArkLoop's M2 `examstore` package will pass `Authorization: Bearer <oidc_token>` headers; exam side enforces scope based on the existing OIDC scope mapping.

**Required scopes** (proposed):
- `exam:read:knowledge-points`, `exam:read:questions` — read endpoints
- `exam:write:questions`, `exam:write:papers` — write endpoints

ArkLoop's worker `examstore` impl uses the teacher's session token; never a service token. No additional secrets configured.

---

## Endpoint 1: `GET /api/knowledge-points`

**Purpose**: ArkLoop's `kb_list_knowledge_points` tool calls this to populate persona dropdowns when a teacher selects "which knowledge point to generate questions for".

### Request

```http
GET /api/knowledge-points?course_id=<course_uuid>
Authorization: Bearer <oidc_token>
```

Query params:
- `course_id` (required, string): scopes the knowledge-point tree to one course
- `limit` (optional, int, default 500): pagination cap
- `offset` (optional, int, default 0): pagination offset

### Response (200)

```json
{
  "items": [
    {
      "id": "exam-kp-uuid-1",
      "course_id": "exam-course-uuid",
      "name": "光的干涉",
      "parent_id": "exam-kp-uuid-parent-or-null",
      "depth": 2,
      "sort_order": 3
    }
  ],
  "total": 147
}
```

**Field semantics**:
- `id` — exam-side stable identifier; ArkLoop stores this in `kb_knowledge_points.exam_knowledge_point_id`
- `parent_id` — null for top-level chapters; tree is flat-list with parent pointers (not nested JSON), so ArkLoop reconstructs the tree client-side
- `depth` — 0-indexed; chapter=0, section=1, subsection=2

### Errors

- `401` if token invalid / expired
- `403` if scope `exam:read:knowledge-points` not granted, or teacher has no access to the course
- `404` if `course_id` not found

### Open question for exam team

- ⚠ Does the existing knowledge-point table have a single "name" column, or split into "code + display name"? ArkLoop expects single `name`.

---

## Endpoint 2: `GET /api/questions`

**Purpose**: Two callers in ArkLoop:
1. `kb_draft_questions` — pulls 5 reference questions per knowledge point as few-shot examples + dup-avoidance signal
2. `kb_compose_paper` — pulls candidate pool for paper composition

### Request

```http
GET /api/questions?knowledge_point_id=<kp_uuid>&type=<type>&difficulty=<level>&limit=50&offset=0
Authorization: Bearer <oidc_token>
```

Query params:
- `knowledge_point_id` (required, string)
- `type` (optional, enum): `single_choice`, `multi_choice`, `fill_in`, `short_answer`, `essay` — pick the canonical set with exam
- `difficulty` (optional, enum): `easy`, `medium`, `hard`
- `limit` (optional, int, default 20, max 200)
- `offset` (optional, int, default 0)

### Response (200)

```json
{
  "items": [
    {
      "id": "exam-q-uuid-1",
      "knowledge_point_id": "exam-kp-uuid",
      "type": "single_choice",
      "difficulty": "medium",
      "stem": "光的干涉是指…",
      "options": [
        {"key": "A", "text": "选项 A"},
        {"key": "B", "text": "选项 B"},
        {"key": "C", "text": "选项 C"},
        {"key": "D", "text": "选项 D"}
      ],
      "answer": "B",
      "explanation": "因为…",
      "source_snippets": [
        {
          "chunk_ref": "kb-id/document-id/ordinal",
          "snippet": "原文 200-500 字快照",
          "ingest_time": "2026-05-19T08:30:00Z"
        }
      ],
      "created_at": "2026-05-19T08:30:00Z",
      "created_by_source": "ai" // or "human"
    }
  ],
  "total": 47
}
```

**Critical field — `source_snippets`** (see Endpoint 3 too):

> ⚠ **Open question (resolves Spike S2 task 1)**: does exam's questions schema have a long-text JSON-ish field that can persist `source_snippets`?
>
> - **Preferred (option A)**: exam adds a `source_snippets JSONB` (or similar) column to questions table; exam UI optionally surfaces "原文出处" in question detail view
> - **Fallback (option B)**: exam declines; ArkLoop stores snapshots in its own `kb_question_snapshots(exam_question_id, snapshot_json, written_at)` table. User Story 19's "看到原文" promise is then served only by ArkLoop UI when the teacher views generated questions, not by exam frontend.
>
> Whichever choice is made, lock it before M2 begins. ArkLoop will not implement option A without a confirmed exam-side schema change PR.

- `options` — only present for choice-type questions; null/omitted for `fill_in`, `short_answer`, `essay`
- `answer` — for choice: comma-joined keys (`"A"` or `"A,C"`); for fill_in: expected text; for short_answer/essay: model answer
- `created_by_source` — lets ArkLoop filter "show me only human-authored references" if AI noise floods reference pool

### Pagination semantics

- Stable ordering: `created_at DESC` then `id` as tiebreaker
- `offset` + `limit` is acceptable for M2 volumes (thousands per knowledge point at most). Cursor-based can wait for v3.

---

## Endpoint 3: `POST /api/questions/batch`

**Purpose**: After the teacher confirms AI-drafted questions in `book-tutor-agent`, ArkLoop calls this once with the batch.

### Request

```http
POST /api/questions/batch
Authorization: Bearer <oidc_token>
Content-Type: application/json

{
  "questions": [
    {
      "knowledge_point_id": "exam-kp-uuid",
      "type": "single_choice",
      "difficulty": "medium",
      "stem": "...",
      "options": [{"key": "A", "text": "..."}, ...],
      "answer": "B",
      "explanation": "...",
      "source_snippets": [
        {"chunk_ref": "kb/doc/ord", "snippet": "原文 200-500 字", "ingest_time": "2026-05-21T..."}
      ],
      "created_by_source": "ai"
    }
  ]
}
```

### Response (200) — partial-success shape

```json
{
  "created": [
    {"index": 0, "id": "exam-q-new-uuid"},
    {"index": 2, "id": "exam-q-new-uuid-2"}
  ],
  "failed": [
    {
      "index": 1,
      "error_code": "knowledge_point_not_found",
      "error_message": "knowledge point exam-kp-uuid-X does not exist"
    }
  ]
}
```

**Why partial success and not all-or-nothing**:
- A batch of 5 generated questions might have 1 with a stale `knowledge_point_id` (the teacher reorganised the tree mid-flow). ArkLoop wants to keep the 4 good ones and report only the 1 failure to the teacher.
- This is **User Story 23**: "写入 exam 失败时能看到具体错误并选择修正后重试或放弃这道"

### Error codes that must be supported

`knowledge_point_not_found`, `validation_error` (e.g. missing answer for choice question), `permission_denied`, `quota_exceeded` (if exam has limits)

Each `error_code` is a stable string; `error_message` is human-readable for ArkLoop persona to forward to teacher.

### Open questions for exam team

- ⚠ Is there a single global `questions` table or per-course? Endpoint can stay the same either way; just confirms storage layout
- ⚠ Does exam validate `options` count matches `type` (single_choice = 4 options typical)? If so, ArkLoop's draft step will mirror those constraints in the LLM prompt

---

## Endpoint 4: `POST /api/papers`

**Purpose**: After paper composition (`kb_compose_paper`), ArkLoop writes the assembled paper back to exam so teachers see it in exam's papers list.

### Request

```http
POST /api/papers
Authorization: Bearer <oidc_token>
Content-Type: application/json

{
  "name": "大学物理（上）期中卷 2026-05",
  "course_id": "exam-course-uuid",
  "spec": {
    "total_count": 25,
    "type_distribution": {"single_choice": 10, "multi_choice": 5, "fill_in": 5, "short_answer": 5},
    "difficulty_distribution": {"easy": 7, "medium": 13, "hard": 5},
    "knowledge_point_distribution": {"exam-kp-uuid-1": 8, "exam-kp-uuid-2": 12, "exam-kp-uuid-3": 5},
    "seed": 42
  },
  "question_ids": [
    "exam-q-uuid-a", "exam-q-uuid-b", "..."
  ]
}
```

`question_ids` ordering = paper question order (ArkLoop has already shuffled / interleaved per type per its `papercompose` rules; exam just stores).

### Response (201)

```json
{
  "id": "exam-paper-uuid",
  "name": "大学物理（上）期中卷 2026-05",
  "question_count": 25,
  "created_at": "2026-05-21T..."
}
```

### Errors

- `400` validation (mismatched count, unknown question_ids)
- `403` no write access to this course

### Open questions for exam team

- ⚠ Does exam require a paper "template" or "exam definition" record before creating a paper? If so, ArkLoop will skip Endpoint 4 and instead document that teachers must pre-create the paper template in exam, then ArkLoop only attaches questions. **Need to know before M2 plan.**
- ⚠ Does exam UI auto-render this paper, or does the teacher need to do further setup (assign to class, schedule, etc.)? Document so ArkLoop's persona can say the right thing after a successful write.

---

## Versioning

These endpoints are **v1** of the ArkLoop↔exam contract. Versioning approach:
- URL stays unversioned (`/api/...`) — exam currently has no API version segment
- ArkLoop sends `X-ArkLoop-API-Version: 1` header on every call
- exam responds with `X-Exam-API-Version: 1`
- Mismatch → ArkLoop logs warning but tries the call anyway; tracking via logs informs when v2 is needed

## Rate limits

ArkLoop's `examstore` will:
- Cap concurrent in-flight requests per teacher session at 4
- Retry 5xx with exponential backoff (3 attempts, base 250ms)
- Surface 429 verbatim to teacher (no client-side rate limiting)

If exam wants documented per-endpoint limits, add them here.

## Test plan

ArkLoop side (in `examstore` package, M2 task):
- Unit tests using `httptest.NewServer` faking each of the 4 endpoints — covers happy path, partial failure, auth failure, 5xx retry
- Integration smoke against a real exam staging deployment — gated by `ARKLOOP_RUN_EXAM_INTEGRATION_TESTS=1`

exam side (responsibility of exam team):
- Standard endpoint tests using whatever framework exam uses
- Specifically: `POST /api/questions/batch` with mixed success/failure inputs

## Open-questions tracker

| # | Question | Owner | Status |
|---|----------|-------|--------|
| 1 | source_snippets persistence path (option a vs b) | exam | open |
| 2 | knowledge-points schema: name vs code+display | exam | open |
| 3 | questions table: global or per-course | exam | open |
| 4 | options count validation by `type` | exam | open |
| 5 | paper template pre-required for Endpoint 4 | exam | open |
| 6 | OIDC scope names — accept proposed list? | exam | open |
| 7 | rate-limit policy documentation | exam | open |
| 8 | `pattern_tag` field on questions (schema + endpoint changes) | exam | open |
| 9 | `GET /api/courses` endpoint (or confirm equivalent) | exam | open |

> Resolve each → mark with date + decision → commit. Once all 9 resolved, this doc becomes a frozen v1 contract referenced by ArkLoop M2a spec.

---

## ArkLoop proposals for open questions

For exam team review — accept / counter-propose / reject each:

| # | ArkLoop Proposal |
|---|------------------|
| 1 | **Option A preferred**: exam adds `source_snippets JSONB` column to questions table. If rejected, ArkLoop stores snapshots in its own `kb_question_snapshots` table (fallback option B). |
| 2 | Single `name` column is sufficient. ArkLoop does not need a separate code field. |
| 3 | Either global or per-course is fine — endpoint shape stays the same. Just confirm so ArkLoop knows whether `course_id` is a filter or a partition key. |
| 4 | If exam validates options count by type, document the rules (e.g. single_choice = 4-5 options). ArkLoop will mirror in LLM prompt constraints. |
| 5 | ArkLoop prefers no paper template pre-required — `POST /api/papers` creates a standalone paper. If exam requires a template, document the pre-creation endpoint. |
| 6 | Accept proposed scopes: `exam:read:knowledge-points`, `exam:read:questions`, `exam:write:questions`, `exam:write:papers`. |
| 7 | ArkLoop accepts any documented rate limits. If none, ArkLoop defaults to 4 concurrent + 3 retries. |

---

## Endpoint addendum: `pattern_tag` field on questions

> Required by ArkLoop M2b (Option 2: skill-driven item building, PRD stories 50-59).

### Schema change (exam side)

```sql
ALTER TABLE questions ADD COLUMN pattern_tag TEXT NULL;
```

### Endpoint changes

- **`GET /api/questions`**: accept optional query param `pattern_tag`; include `pattern_tag` in response items (null when not set)
- **`POST /api/questions/batch`**: accept optional per-item `pattern_tag` field; persist to column

### Semantics

`pattern_tag` encodes item-writing pattern style. For medical exams: `A1` / `A2` / `A3` / `A4`. For other domains: extensible free-text (e.g. `proof_based`, `case_study`). ArkLoop enforces tag consistency in its own tool layer; exam just stores and filters.

---

## Endpoint 5 (new): `GET /api/courses`

> Required by ArkLoop M2a console-lite KB creation form (teacher selects which exam course to bind a Linked KB to).

### Request

```http
GET /api/courses
Authorization: Bearer <oidc_token>
```

Returns courses accessible to the authenticated teacher.

### Response (200)

```json
{
  "items": [
    {"id": "exam-course-uuid", "name": "大学物理（上）"}
  ]
}
```

### Errors

- `401` if token invalid / expired
- `403` if scope not granted

### Open question 9

Does this endpoint already exist under a different path (e.g. `/api/learning/directions`)? If so, ArkLoop will use that path instead. Please confirm.

---

## Resolution log

(empty — fill as questions resolve)
