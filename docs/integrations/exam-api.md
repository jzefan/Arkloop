# ArkLoop ↔ exam — API Contract (v1, frozen)

> **Status**: **frozen-v1** (all 9 open questions resolved 2026-05-24; see §Resolution log)
> **Owner**: jzefan (ArkLoop side) + exam team
> **Created**: 2026-05-21
> **Frozen**: 2026-05-24
> **Powers**: ArkLoop M2a (Linked-mode KB) + M2b (Option 2 skill-driven item building) — see `docs/superpowers/specs/2026-05-23-book-kb-rag-m2a-design.md` and `…-m2b-design.md`
>
> Changes after freeze require a v2 contract PR and matching ArkLoop migration. See `§Versioning` for procedure.

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
GET /api/knowledge-points?exam_scope_id=<scope_uuid>
Authorization: Bearer <oidc_token>
```

Query params:
- `exam_scope_id` (required, string): scopes the knowledge-point tree to one exam scope. The scope id may resolve to a `major`, `direction`, or `topic` (主知识点) node — see `Endpoint 5` for the scope hierarchy. The endpoint returns all knowledge points that fall under the given scope.
- `limit` (optional, int, default 500): pagination cap
- `offset` (optional, int, default 0): pagination offset

### Response (200)

```json
{
  "items": [
    {
      "id": "exam-kp-uuid-1",
      "exam_scope_id": "exam-scope-uuid",
      "code": "K-3.2-INTF",
      "display_name": "光的干涉",
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
- `code` — short stable code from exam's curriculum codebook (per Q2 resolution; exam side uses code+display split rather than single name)
- `display_name` — human-readable label shown to teachers
- `parent_id` — null for top-level chapters; tree is flat-list with parent pointers (not nested JSON), so ArkLoop reconstructs the tree client-side
- `depth` — 0-indexed; chapter=0, section=1, subsection=2

### Errors

- `401` if token invalid / expired
- `403` if scope `exam:read:knowledge-points` not granted, or teacher has no access to the exam scope
- `404` if `exam_scope_id` not found

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

Per Q1 resolution: exam side adds a `source_snippets JSONB` column to `questions` table. ArkLoop persists 200-500 character snapshot per source chunk so that even after KB re-ingest the original passage stays retrievable. Schema change:

```sql
ALTER TABLE questions ADD COLUMN source_snippets JSONB NOT NULL DEFAULT '[]'::jsonb;
```

Exam UI optionally surfaces these snapshots in question detail view (not required for v1).

- `options` — only present for choice-type questions; null/omitted for `fill_in`, `short_answer`, `essay`. Per Q4 resolution: `single_choice` / `multi_choice` accept **≥3 options** (no strict 4-option requirement); ArkLoop's LLM prompt mirrors this.
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

### Resolutions (Q3 + Q4)

- **Q3**: exam uses a **single global `questions` table**. `knowledge_point_id` is the only scoping filter — there is no per-course partition.
- **Q4**: exam accepts **≥3 options** for `single_choice` / `multi_choice` (3, 4, or more — not strictly 4). ArkLoop's `kb_draft_questions` / `exam_build_questions` LLM prompt is loosened accordingly.

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
  "exam_scope_id": "exam-scope-uuid",
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
- `403` no write access to this exam scope

### Resolution (Q5)

Per Q5: exam does **not** require a pre-existing paper template. `POST /api/papers` creates a standalone paper directly; ArkLoop's persona writes it back without any pre-setup step.

> Follow-up to exam team: please document whether the paper auto-renders for students or whether the teacher needs further setup (assign to class, schedule, etc.) after creation. ArkLoop persona will adjust its success-confirmation message once known. Tracked in §Resolution log Q5.

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

## Open-questions tracker (all resolved 2026-05-24)

| # | Question | Owner | Status |
|---|----------|-------|--------|
| 1 | source_snippets persistence path (option a vs b) | exam | ✅ resolved 2026-05-24 — Option A (JSONB column) |
| 2 | knowledge-points schema: name vs code+display | exam | ✅ resolved 2026-05-24 — `code + display_name` |
| 3 | questions table: global or per-course | exam | ✅ resolved 2026-05-24 — single global table |
| 4 | options count validation by `type` | exam | ✅ resolved 2026-05-24 — minimum 3 options (not strictly 4) |
| 5 | paper template pre-required for Endpoint 4 | exam | ✅ resolved 2026-05-24 — not required |
| 6 | OIDC scope names — accept proposed list? | exam | ✅ resolved 2026-05-24 — accept as proposed |
| 7 | rate-limit policy documentation | exam | ✅ resolved 2026-05-24 — defer to ArkLoop defaults |
| 8 | `pattern_tag` field on questions (schema + endpoint changes) | exam | ✅ resolved 2026-05-24 — accepted |
| 9 | `GET /api/courses` endpoint name | exam | ✅ resolved 2026-05-24 — renamed `GET /api/exam-scopes` (no course concept exam-side; scope hierarchy is 专业 / 方向 / 主知识点) |

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

## Endpoint 5 (new): `GET /api/exam-scopes`

> Required by ArkLoop M2a console-lite KB creation form (teacher selects which exam scope to bind a Linked KB to).
> Per Q9 resolution: exam has no "course" concept — the curriculum hierarchy is **专业 (major) / 方向 (direction) / 主知识点 (topic)**. A Linked KB may bind to any of these three levels. The endpoint is named neutrally as `exam-scopes` so future binding at the major level (whole-discipline KB) is forward-compatible.

### Request

```http
GET /api/exam-scopes
Authorization: Bearer <oidc_token>
```

Returns the union of all scopes accessible to the authenticated teacher across all three levels.

### Response (200)

```json
{
  "items": [
    {
      "id": "scope-uuid-1",
      "type": "major",
      "code": "100201",
      "display_name": "临床医学",
      "parent_id": null
    },
    {
      "id": "scope-uuid-2",
      "type": "direction",
      "code": "0201-1",
      "display_name": "妇产科方向",
      "parent_id": "scope-uuid-1"
    },
    {
      "id": "scope-uuid-3",
      "type": "topic",
      "code": "0201",
      "display_name": "妇产科学",
      "parent_id": "scope-uuid-2"
    }
  ]
}
```

**Field semantics**:
- `type` — enum `major | direction | topic` indicating curriculum hierarchy level. `topic` corresponds to 主知识点 (today's "course-equivalent"); `major` and `direction` are higher-level groupings.
- `code` — short stable curriculum code from exam's codebook
- `display_name` — human-readable label for UI display
- `parent_id` — null for top-level (`major`); otherwise points to the parent scope so ArkLoop can render a tree picker

### Errors

- `401` if token invalid / expired
- `403` if scope not granted

### Downstream binding semantics

A Linked KB binds to exactly one `exam_scope_id`. That scope id is then used as the filter for `GET /api/knowledge-points?exam_scope_id=...`:
- If the bound scope is a `topic` → the KP endpoint returns the topic itself (single leaf) or its sub-knowledge-points if exam has further sub-divisions
- If the bound scope is a `direction` → the KP endpoint returns all topics under that direction
- If the bound scope is a `major` → the KP endpoint returns all topics across all directions under that major

---

## Resolution log

### 2026-05-24 — all 9 questions resolved by exam team (contract frozen v1)

**Q1 (source_snippets persistence)** — ✅ Accept Option A. Exam side will add `source_snippets JSONB NOT NULL DEFAULT '[]'` column to `questions` table. UI surfacing is optional / exam team's discretion.

**Q2 (knowledge-points name vs code+display)** — ⚠ Counter-proposal accepted by ArkLoop. Exam uses `code + display_name` split (not single `name`). ArkLoop `examstore.KPItem` updates to carry both fields; the `kb_knowledge_points` table on ArkLoop side persists only `display_name` for UI, with `exam_knowledge_point_id` as the stable identifier for re-fetching `code` if needed.

**Q3 (questions table layout)** — ✅ Single global `questions` table. `knowledge_point_id` is the scoping filter; no per-course partition exists.

**Q4 (options count by type)** — ⚠ Counter-proposal: minimum **3** options for `single_choice` / `multi_choice`, not strictly 4. ArkLoop loosens its LLM prompt in `kb_draft_questions` and `exam_build_questions`: "options count must be ≥3" (previously "4 typical"). Existing tests asserting 4-option outputs are updated to accept 3-option outputs as well.

**Q5 (paper template pre-required)** — ✅ Not required. `POST /api/papers` creates a standalone paper directly. Follow-up to exam team: please document downstream behavior (auto-render? teacher needs further setup?) so persona success message can be accurate. Tracked separately, not blocking.

**Q6 (OIDC scope names)** — ✅ Accept ArkLoop proposal: `exam:read:knowledge-points`, `exam:read:questions`, `exam:write:questions`, `exam:write:papers`.

**Q7 (rate-limit policy)** — ✅ No exam-side per-endpoint limits documented. ArkLoop's defaults apply: 4 concurrent + 3 retries with exponential backoff.

**Q8 (`pattern_tag` field)** — ✅ Accepted. Exam adds `pattern_tag TEXT NULL` column to `questions`. `GET /api/questions` accepts `pattern_tag` query filter and returns the field in items. `POST /api/questions/batch` accepts per-item `pattern_tag`.

**Q9 (`GET /api/courses` endpoint)** — ⚠ Counter-proposal: exam has no "course" concept; the curriculum hierarchy is **专业 / 方向 / 主知识点**. KBs may bind to any of these three levels (今天最常见绑定 topic，未来可能绑 major)。Endpoint renamed `GET /api/exam-scopes` with multi-shape response carrying `type: major | direction | topic`. ArkLoop side follow-ups: rename `knowledge_bases.exam_course_id` → `exam_scope_id` (migration 00196); rename `examstore.ListCourses` → `ListExamScopes`; rename `/v1/exam/courses` proxy → `/v1/exam/scopes`; console-lite UI text 「绑定 exam 课程」 → 「绑定 exam 范围」.

### Versioning rule

This contract is frozen as **v1**. Any subsequent endpoint additions, removals, field renames, or semantic changes require:
1. A new PR amending this doc with `Status: draft-v2` while v1 stays the authoritative section
2. An exam-side schema migration PR linked from this doc
3. An ArkLoop-side compatibility PR keeping v1 client logic alive until exam has v2 ready
4. Bump `X-ArkLoop-API-Version` and `X-Exam-API-Version` headers to `2` after both sides cut over
