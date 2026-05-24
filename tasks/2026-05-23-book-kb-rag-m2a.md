# book-kb-rag M2a Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 ArkLoop 从"独立可用的 standalone KB"打通到"与 exam 系统真实联动"——落地部署级开关、Linked KB UI、`examstore` 真实现，并冻结 4 端点合约（含 `pattern_tag`）。

**Architecture:** 部署级 env 开关 (`ARKLOOP_EXAM_INTEGRATION_ENABLED`) 控制三层注册（api persona loader、worker exam.* 工具、kbapi 创建 KB 校验链）；`shared/questionstore` 包提供工厂 `For(kb)` 按 `kb.integration_mode` 返回 `localstore`（M1.2 落地）或 `examstore`（本 milestone）；`examstore` 直接调 exam REST 4 端点，不走 worker tool 层；console-lite 通过 `GET /v1/config` 暴露的 flag 决定是否显示 Linked 选项。

**Tech Stack:** Go 1.22 (api/worker), pgx v5 + pgvector, React + Tailwind (console-lite), goose migrations (Postgres + SQLite), httptest for examstore unit tests.

---

## File Structure

**Migrations**
- Create: `src/services/api/internal/migrate/migrations/00194_kb_visibility_and_exam_constraint.sql`

**API — data layer**
- Modify: `src/services/api/internal/data/knowledge_bases_repo.go` — add `Visibility` field、`CountByBlobSHA256` 移到 kb_documents_repo
- Modify: `src/services/api/internal/data/kb_documents_repo.go` — add `CountByBlobSHA256(sha) (int, error)`

**API — http layer**
- Modify: `src/services/api/internal/http/kbapi/handler_kb.go` — createKB 校验链 + visibility 字段
- Modify: `src/services/api/internal/http/kbapi/auth.go` — `ensureKBVisible` helper
- Modify: `src/services/api/internal/http/kbapi/handler_doc.go` — sniffed mime 白名单 + blob ref-count delete
- Modify: `src/services/api/internal/http/kbapi/adapters.go` — `WorkspaceBlobAdapter.DeleteBlob`
- Create: `src/services/api/internal/http/examapi/handler.go` — `GET /v1/exam/courses` 代理
- Create: `src/services/api/internal/http/examapi/handler_test.go`
- Modify: `src/services/api/internal/http/configapi/handler.go`（或同等位置）— `/v1/config` 暴露 `exam_integration_enabled`

**API — app config**
- Modify: `src/services/api/internal/app/config.go` — `ExamIntegrationEnabled` + `ExamBaseURL` + validation
- Modify: `src/services/api/internal/app/config_kb_debug_test.go`（或新建 `config_exam_test.go`）

**API — persona loader**
- Modify: `src/services/api/internal/personas/loader.go` — 注入 `ExamIntegrationEnabled` 后过滤 exam-agent.user_selectable

**Worker — exam tool gate**
- Modify: `src/services/worker/internal/tools/builtin/builtin.go` — env 优先级守门

**Worker — kb tools (TOC)**
- Modify: `src/services/worker/internal/tools/builtin/kb/spec.go` — 新增 `kb_extract_toc` AgentSpec + LlmSpec
- Modify: `src/services/worker/internal/tools/builtin/kb/executor.go` — `extractTOC` handler
- Create: `src/services/api/internal/http/kbapi/handler_toc.go` — `GET /v1/knowledge-bases/:id/documents/:doc_id/toc`

**Shared — questionstore**
- Create: `src/services/shared/questionstore/types.go`
- Create: `src/services/shared/questionstore/interface.go`
- Create: `src/services/shared/questionstore/factory.go`
- Create: `src/services/shared/questionstore/factory_test.go`
- Create: `src/services/shared/questionstore/localstore/store.go`
- Create: `src/services/shared/questionstore/localstore/store_integration_test.go`
- Create: `src/services/shared/questionstore/examstore/client.go`
- Create: `src/services/shared/questionstore/examstore/client_test.go`
- Create: `src/services/shared/questionstore/examstore/store.go`

**Personas**
- Modify: `src/personas/book-tutor-agent/prompt.md` — Linked 模式 TOC widget 流

**console-lite**
- Modify: `src/apps/console-lite/src/components/CreateKBModal.tsx` — visibility + integration_mode 字段
- Modify: `src/apps/console-lite/src/pages/KnowledgeBasesPage.tsx` — 列表徽章
- Create: `src/apps/console-lite/src/components/ExamCourseSelect.tsx` — 课程下拉
- Modify: `src/apps/console-lite/src/api/...` — 暴露 platformConfig.exam_integration_enabled

**Docs**
- Modify: `docs/integrations/exam-api.md` — resolve 7 open questions + 新增 #8 pattern_tag、#9 GET /api/courses → frozen-v1

---

## Pre-Flight

- 已读 [PRD](../docs/prd/book-kb-rag.md) §集成模式开关 + §F/F'/H + §对 exam 系统的依赖
- 已读 [M2a 设计](../docs/superpowers/specs/2026-05-23-book-kb-rag-m2a-design.md)
- 已读 [exam-api 合约草稿](../docs/integrations/exam-api.md)
- 已确认 migration `00193_kb_full_schema.sql` 已落地（含 `integration_mode` + `exam_course_id` 字段 + integration_mode check 约束）—— 本 plan 不重做这部分
- 已确认 `mimeFromExt` 仅按扩展名判断 —— 本 plan 加 sniff 兜底白名单（design §kbapi 改动）

---

### Task 1: Drive exam-api.md to frozen-v1（合约对齐，**阻塞 Task 9**）

**Files:**
- Modify: `docs/integrations/exam-api.md`

> ⚠ 这是 **协调任务**，不是代码任务。需要与 exam 后端 owner 同步 resolve 现有 7 个 open question + 新增 2 个。
> 完成判定：`exam-api.md` 头部 `Status` 改为 `frozen-v1` 且 §Open-questions tracker 7+2 项全部带 resolution + 日期。
> 实施 Task 9（`examstore` 真实现）前必须完成此 task；Task 2-8、10-11 可并行不阻塞。

- [ ] **Step 1: 把当前文档已有的 7 个 open question 各加上 "ArkLoop 提案"**

在 `docs/integrations/exam-api.md` 每个 `## Open question` / `### Open questions for exam team` 小节末尾加一行 `**ArkLoop 提案**：...`，措辞照 M2a 设计 §Spike S2 表格（option A / 单列 name / 单列接受 / mirror 校验 / paper template 等 exam 决定 / 接受 scope 名 / 接受 rate-limit 任一）。

- [ ] **Step 2: 新增第 8 个 open question — `pattern_tag` 字段**

在 §Open-questions tracker 表后追加一节：

```markdown
## Endpoint addendum: `pattern_tag` field on questions

ArkLoop M2b (Option 2) requires a `pattern_tag` field on exam questions to
encode item-writing pattern style (A1/A2/A3/A4 for medical exams, extensible
for other domains).

### Schema change

```sql
ALTER TABLE questions ADD COLUMN pattern_tag TEXT NULL;
```

### Endpoint changes

- `GET /api/questions`: accept query param `pattern_tag`; include `pattern_tag` in response items
- `POST /api/questions/batch`: accept per-item `pattern_tag` field

### Open question 8

| # | Question | ArkLoop Proposal |
|---|----------|------------------|
| 8 | Accept the `pattern_tag` schema + endpoint changes? | YES, ArkLoop needs this for M2b |
```

加入 `| 8 | pattern_tag schema + endpoint changes | exam | open |` 到现有 tracker 表。

- [ ] **Step 3: 新增第 9 个 open question — `GET /api/courses`**

```markdown
## Endpoint 5 (new): `GET /api/courses`

**Purpose**: ArkLoop's console-lite KB creation form needs to list exam
courses for the teacher to bind a new Linked KB to. Currently no such
endpoint exists publicly.

### Request

```http
GET /api/courses
Authorization: Bearer <oidc_token>
```

### Response (200)

```json
{
  "items": [
    {"id": "exam-course-uuid", "name": "大学物理（上）", "owner_id": "..."}
  ]
}
```

### Open question 9

| # | Question | ArkLoop Proposal |
|---|----------|------------------|
| 9 | Expose `GET /api/courses` (or confirm equivalent existing endpoint) | YES, scoped by teacher token |
```

加入 `| 9 | GET /api/courses or equivalent | exam | open |` 到 tracker 表。

- [ ] **Step 4: 提 PR 给 exam 团队 review**

```bash
git checkout -b book-kb-rag/m2a-exam-api-contract
git add docs/integrations/exam-api.md
git commit -m "docs(exam-api): propose resolutions for 7 open questions + add pattern_tag + GET /api/courses (M2a Spike S2)"
git push -u origin book-kb-rag/m2a-exam-api-contract
gh pr create --title "exam-api M2a Spike S2: freeze contract + pattern_tag + GET /api/courses" --body "$(cat <<'EOF'
## Summary

Drive `docs/integrations/exam-api.md` to frozen-v1 for ArkLoop M2a kickoff:
- Resolve 7 existing open questions with ArkLoop proposals
- Add open question #8: `pattern_tag` field on questions (required by M2b)
- Add open question #9: `GET /api/courses` endpoint (required by M2a Linked KB UI)

## Action requested from exam team

For each of the 9 open questions in §Open-questions tracker, please leave a
comment on this PR with ✅ accept / ❌ counter-propose / 🚫 reject + reasoning.
Once all 9 are resolved, ArkLoop flips `Status: frozen-v1` and starts M2a
examstore implementation.

## Blocking

- ArkLoop M2a Task 9 (examstore real impl) is blocked on this PR merging
- ArkLoop M2b is blocked on M2a Task 9
EOF
)"
```

- [ ] **Step 5: 等 exam 团队 review 完，更新文档 + merge**

每个 open question 在 §Resolution log 写一行 `### YYYY-MM-DD Q<N>: <accepted|counter|rejected> — <details>`。全部 resolve 后改 `Status: frozen-v1`，PR merge。

```bash
# 等 exam 团队回复后
git add docs/integrations/exam-api.md
git commit -m "docs(exam-api): freeze v1 contract — all 9 open questions resolved"
git push
gh pr merge --squash
```

---

### Task 2: Migration 00194 + KB repo Visibility 字段

**Files:**
- Create: `src/services/api/internal/migrate/migrations/00194_kb_visibility_and_exam_constraint.sql`
- Modify: `src/services/api/internal/data/knowledge_bases_repo.go`
- Modify: `src/services/api/internal/data/knowledge_bases_repo_integration_test.go`

- [ ] **Step 1: 写 migration 00194**

```sql
-- +goose Up
-- +goose StatementBegin
ALTER TABLE knowledge_bases
    ADD COLUMN visibility TEXT NOT NULL DEFAULT 'workspace_member';

ALTER TABLE knowledge_bases
    ADD CONSTRAINT knowledge_bases_visibility_check
    CHECK (visibility IN ('workspace_member', 'private'));

-- 00193 已经定义 integration_mode check IN ('standalone','exam'),
-- 但没强制 mode='exam' 时 exam_course_id 必填; 这里补上
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
```

- [ ] **Step 2: 在 knowledge_bases_repo.go 加 `Visibility` 字段**

```go
type KnowledgeBase struct {
    ID              uuid.UUID
    WorkspaceRef    string
    AccountID       uuid.UUID
    Name            string
    Description     string
    Visibility      string  // NEW: 'workspace_member' | 'private'
    IntegrationMode string
    ExamCourseID    *string
    CreatedBy       *uuid.UUID
    CreatedAt       time.Time
    UpdatedAt       time.Time
    DocumentCount   int
}

type KBCreate struct {
    AccountID       uuid.UUID
    WorkspaceRef    string
    Name            string
    Description     string
    Visibility      string  // NEW: "" treated as workspace_member
    IntegrationMode string  // NEW: "" treated as standalone
    ExamCourseID    *string // NEW: required iff mode=exam
    CreatedBy       *uuid.UUID
}
```

更新 `Create`：

```go
func (r *KnowledgeBasesRepository) Create(ctx context.Context, in KBCreate) (*KnowledgeBase, error) {
    visibility := in.Visibility
    if visibility == "" {
        visibility = "workspace_member"
    }
    mode := in.IntegrationMode
    if mode == "" {
        mode = "standalone"
    }
    row := r.pool.QueryRow(ctx, `
INSERT INTO knowledge_bases (workspace_ref, account_id, name, description, visibility, integration_mode, exam_course_id, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id, workspace_ref, account_id, name, description, visibility, integration_mode, exam_course_id, created_by, created_at, updated_at`,
        in.WorkspaceRef, in.AccountID, in.Name, in.Description, visibility, mode, in.ExamCourseID, in.CreatedBy)
    var kb KnowledgeBase
    if err := row.Scan(&kb.ID, &kb.WorkspaceRef, &kb.AccountID, &kb.Name, &kb.Description,
        &kb.Visibility, &kb.IntegrationMode, &kb.ExamCourseID, &kb.CreatedBy, &kb.CreatedAt, &kb.UpdatedAt); err != nil {
        if isPGUniqueViolation(err) {
            return nil, ErrKBDuplicateName
        }
        return nil, fmt.Errorf("create kb: %w", err)
    }
    return &kb, nil
}
```

同步更新 `GetByID` / `ListByWorkspace` 的 SELECT 列表 + Scan。

- [ ] **Step 3: 写 integration test 验证 visibility 字段**

在 `knowledge_bases_repo_integration_test.go` 末尾追加：

```go
func TestKBRepo_Create_DefaultsVisibilityToWorkspaceMember(t *testing.T) {
    pool, cleanup := newTestPool(t)
    defer cleanup()
    repo, _ := data.NewKnowledgeBasesRepository(pool)
    ctx := context.Background()
    accountID := makeTestAccount(t, pool)

    kb, err := repo.Create(ctx, data.KBCreate{
        AccountID:    accountID,
        WorkspaceRef: "ws-1",
        Name:         "test-kb",
    })
    if err != nil { t.Fatal(err) }
    if kb.Visibility != "workspace_member" {
        t.Errorf("expected visibility=workspace_member, got %q", kb.Visibility)
    }
}

func TestKBRepo_Create_AcceptsPrivateVisibility(t *testing.T) {
    pool, cleanup := newTestPool(t)
    defer cleanup()
    repo, _ := data.NewKnowledgeBasesRepository(pool)
    ctx := context.Background()
    accountID := makeTestAccount(t, pool)

    kb, err := repo.Create(ctx, data.KBCreate{
        AccountID:    accountID,
        WorkspaceRef: "ws-1",
        Name:         "private-kb",
        Visibility:   "private",
    })
    if err != nil { t.Fatal(err) }
    if kb.Visibility != "private" { t.Errorf("got %q", kb.Visibility) }
}

func TestKBRepo_Create_RejectsExamModeWithoutCourse(t *testing.T) {
    pool, cleanup := newTestPool(t)
    defer cleanup()
    repo, _ := data.NewKnowledgeBasesRepository(pool)
    ctx := context.Background()
    accountID := makeTestAccount(t, pool)

    _, err := repo.Create(ctx, data.KBCreate{
        AccountID:       accountID,
        WorkspaceRef:    "ws-1",
        Name:            "kb-exam-bad",
        IntegrationMode: "exam",
        // ExamCourseID nil
    })
    if err == nil {
        t.Fatal("expected check violation, got nil")
    }
    if !strings.Contains(err.Error(), "knowledge_bases_exam_mode_requires_course") {
        t.Errorf("expected exam_mode_requires_course constraint error, got: %v", err)
    }
}
```

`makeTestAccount` 复用现有 helper（已存在）。

- [ ] **Step 4: 跑测试验证失败再加 migration 让其通过**

```bash
cd /Users/jzefan/work/proj/ArkLoop
go test ./src/services/api/internal/data/ -run TestKBRepo_Create_ -v
```

Expected: 第一次 FAIL（visibility 列不存在）。然后跑 migration：

```bash
cd src/services/api && go run ./cmd/api migrate up
go test ./internal/data/ -run TestKBRepo_Create_ -v
```

Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add src/services/api/internal/migrate/migrations/00194_kb_visibility_and_exam_constraint.sql \
        src/services/api/internal/data/knowledge_bases_repo.go \
        src/services/api/internal/data/knowledge_bases_repo_integration_test.go
git commit -m "feat(kb): add visibility column + exam_mode_requires_course constraint (M2a migration 00194)"
```

---

### Task 3: kbapi createKB 校验链 + ensureKBVisible

**Files:**
- Modify: `src/services/api/internal/http/kbapi/handler_kb.go`
- Modify: `src/services/api/internal/http/kbapi/auth.go`
- Modify: `src/services/api/internal/http/kbapi/handler_kb_test.go`

- [ ] **Step 1: 在 deps.go / handlerCtx 加 `ExamIntegrationEnabled bool` 字段**

```go
type handlerCtx struct {
    // ... existing fields
    examIntegrationEnabled bool
}

// Deps 加：
type Deps struct {
    // ... existing
    ExamIntegrationEnabled bool
}

// NewHandlerCtx 末尾：
return &handlerCtx{
    // ... existing wiring
    examIntegrationEnabled: deps.ExamIntegrationEnabled,
}
```

- [ ] **Step 2: 修改 createKB handler 接受新字段 + 校验链**

在 `handler_kb.go` createKB 段（找到 `type createKBReq` 或同等位置）：

```go
type createKBReq struct {
    Name            string  `json:"name"`
    WorkspaceRef    string  `json:"workspace_ref"`
    Description     string  `json:"description"`
    Visibility      string  `json:"visibility"`         // "" -> workspace_member
    IntegrationMode string  `json:"integration_mode"`   // "" -> standalone
    ExamCourseID    *string `json:"exam_course_id"`     // required iff mode=exam
}

func handleCreateKB(h *handlerCtx) nethttp.HandlerFunc {
    return func(w nethttp.ResponseWriter, r *nethttp.Request) {
        // ... 现有 actor + workspace membership check ...

        var req createKBReq
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            writeErr(w, nethttp.StatusBadRequest, "kb.bad_request", "invalid json body")
            return
        }
        req.Name = strings.TrimSpace(req.Name)
        if req.Name == "" {
            writeErr(w, nethttp.StatusBadRequest, "kb.missing_name", "name is required")
            return
        }
        // visibility
        switch req.Visibility {
        case "", "workspace_member", "private":
        default:
            writeErr(w, nethttp.StatusBadRequest, "kb.invalid_visibility", "visibility must be workspace_member or private")
            return
        }
        // integration_mode
        switch req.IntegrationMode {
        case "", "standalone":
        case "exam":
            if !h.examIntegrationEnabled {
                writeErr(w, nethttp.StatusBadRequest, "kb.integration_disabled",
                    "本部署未启用 exam 集成，请联系管理员或选择独立模式")
                return
            }
            if req.ExamCourseID == nil || strings.TrimSpace(*req.ExamCourseID) == "" {
                writeErr(w, nethttp.StatusBadRequest, "kb.missing_exam_course",
                    "选择绑定 exam 课程模式时必须指定 exam_course_id")
                return
            }
        default:
            writeErr(w, nethttp.StatusBadRequest, "kb.invalid_integration_mode",
                "integration_mode must be standalone or exam")
            return
        }
        // ... 调 h.kbStore.Create(...) 传入新字段 ...
    }
}
```

- [ ] **Step 3: 写 handler_kb_test 覆盖校验**

在 `handler_kb_test.go` 加：

```go
func TestHandleCreateKB_RejectsExamModeWhenIntegrationDisabled(t *testing.T) {
    h := newTestHandlerCtx(t, false /* examIntegrationEnabled */)
    body := `{"name":"k","workspace_ref":"ws","integration_mode":"exam","exam_course_id":"c1"}`
    rec := doRequest(t, h, "POST", "/v1/knowledge-bases", body)
    if rec.Code != 400 { t.Fatalf("want 400, got %d", rec.Code) }
    if !strings.Contains(rec.Body.String(), "kb.integration_disabled") {
        t.Errorf("expected kb.integration_disabled, got %s", rec.Body.String())
    }
}

func TestHandleCreateKB_RejectsExamModeWithoutCourseID(t *testing.T) {
    h := newTestHandlerCtx(t, true)
    body := `{"name":"k","workspace_ref":"ws","integration_mode":"exam"}`
    rec := doRequest(t, h, "POST", "/v1/knowledge-bases", body)
    if rec.Code != 400 || !strings.Contains(rec.Body.String(), "kb.missing_exam_course") {
        t.Errorf("unexpected: %d %s", rec.Code, rec.Body.String())
    }
}

func TestHandleCreateKB_RejectsInvalidVisibility(t *testing.T) {
    h := newTestHandlerCtx(t, true)
    body := `{"name":"k","workspace_ref":"ws","visibility":"public"}`
    rec := doRequest(t, h, "POST", "/v1/knowledge-bases", body)
    if rec.Code != 400 || !strings.Contains(rec.Body.String(), "kb.invalid_visibility") {
        t.Errorf("unexpected: %d %s", rec.Code, rec.Body.String())
    }
}
```

`newTestHandlerCtx` 签名按现有 helper 调整加 `examIntegrationEnabled bool` 参数。

- [ ] **Step 4: 在 auth.go 加 ensureKBVisible helper**

```go
// ensureKBVisible enforces KB.Visibility on top of workspace membership.
// Returns true if the actor can see the KB; otherwise writes 404 and returns false.
func ensureKBVisible(w nethttp.ResponseWriter, kb *data.KnowledgeBase, actor *Actor) bool {
    if kb.Visibility != "private" {
        return true
    }
    if kb.CreatedBy != nil && *kb.CreatedBy == actor.UserID {
        return true
    }
    writeErr(w, nethttp.StatusNotFound, "kb.not_found", "kb not found")
    return false
}
```

(`Actor` / `actorFromCtx` 沿用既有；`*data.KnowledgeBase` 已含 `CreatedBy`)

- [ ] **Step 5: 在 `loadAuthorizedKB` 内调用**

`auth.go` 现有 `loadAuthorizedKB`（list/get/delete/document handlers 都走这个）— 在 workspace membership check 通过、KB 加载完成、返回前追加：

```go
if !ensureKBVisible(w, kb, a) {
    return nil, false
}
```

- [ ] **Step 6: 写 visibility 过滤 integration test**

`auth_test.go` 加：

```go
func TestVisibility_PrivateKBHiddenFromNonCreator(t *testing.T) {
    // user_a 创建 private KB
    // user_b 同 workspace 但调 GET /v1/knowledge-bases/<id> 返 404
    // user_a 自己调返 200
}
```

(实现细节按 existing test scaffold；body 含两个 actor 的 token 切换)

- [ ] **Step 7: 跑全套测试**

```bash
go test ./src/services/api/internal/http/kbapi/... -v
```

Expected: 全 PASS。

- [ ] **Step 8: Commit**

```bash
git add src/services/api/internal/http/kbapi/
git commit -m "feat(kb): createKB validation chain + ensureKBVisible filter (M2a Task 3)"
```

---

### Task 4: handler_doc mime 白名单 + blob ref-count delete

**Files:**
- Modify: `src/services/api/internal/http/kbapi/handler_doc.go`
- Modify: `src/services/api/internal/http/kbapi/adapters.go`
- Modify: `src/services/api/internal/http/kbapi/deps.go` — `blobWriter` 接口加 `DeleteBlob`
- Modify: `src/services/api/internal/data/kb_documents_repo.go` — `CountByBlobSHA256`
- Modify: `src/services/api/internal/http/kbapi/handler_doc_test.go`

- [ ] **Step 1: 在 kb_documents_repo.go 加 CountByBlobSHA256**

```go
func (r *KBDocumentsRepository) CountByBlobSHA256(ctx context.Context, sha string) (int, error) {
    var n int
    err := r.pool.QueryRow(ctx,
        `SELECT COUNT(*) FROM kb_documents WHERE blob_sha256 = $1`, sha).Scan(&n)
    if err != nil {
        return 0, fmt.Errorf("count by sha: %w", err)
    }
    return n, nil
}
```

- [ ] **Step 2: 扩 blobWriter 接口 + WorkspaceBlobAdapter**

`deps.go`：

```go
type blobWriter interface {
    PutBlob(ctx context.Context, workspaceRef, sha256 string, data []byte) error
    DeleteBlob(ctx context.Context, workspaceRef, sha256 string) error  // NEW
}
```

`adapters.go`：

```go
func (a *WorkspaceBlobAdapter) DeleteBlob(ctx context.Context, workspaceRef, sha256 string) error {
    key := workspaceBlobKey(workspaceRef, sha256)
    return a.Store.Delete(ctx, key)
}
```

(`workspaceBlobKey` 沿用既有；`objectstore.Store.Delete` 已存在)

- [ ] **Step 3: 在 docStore 接口加 CountByBlobSHA256**

`handler_kb.go`：

```go
type docStore interface {
    // ... existing
    CountByBlobSHA256(ctx context.Context, sha string) (int, error)
}
```

- [ ] **Step 4: 修改 handler_doc 上传：sniff mime + 白名单**

替换 `handleUploadDoc` 中 `mimeFromExt` 段为：

```go
const sniffBytes = 512

// 先读 sniffBytes 用 http.DetectContentType；不可靠时 fallback 到扩展名
peek := make([]byte, sniffBytes)
peekN, _ := file.Read(peek)
detected := strings.ToLower(strings.TrimSpace(strings.SplitN(
    nethttp.DetectContentType(peek[:peekN]), ";", 2)[0]))

// fallback 给 markdown（DetectContentType 把 .md 识别为 text/plain，丢失语义）
ext := strings.ToLower(filepath.Ext(header.Filename))
if ext == ".md" && detected == "text/plain" {
    detected = "text/markdown"
}

allowed := map[string]struct{}{
    "text/plain": {},
    "text/markdown": {},
    "application/pdf": {},
    "application/vnd.openxmlformats-officedocument.wordprocessingml.document": {},
    "image/png": {},
    "image/jpeg": {},
    "image/webp": {},
}
if _, ok := allowed[detected]; !ok {
    writeErr(w, nethttp.StatusUnsupportedMediaType, "kb.unsupported_mime",
        fmt.Sprintf("不支持的文件类型：%s（支持 .txt/.md/.pdf/.docx/.png/.jpg/.webp）", detected))
    return
}

// rewind file reader
if _, err := file.Seek(0, io.SeekStart); err != nil {
    writeErr(w, nethttp.StatusInternalServerError, "kb.read_failed", "")
    return
}
mime := detected
```

把后面把 `mime` 写入 `data.DocCreate{MimeType: mime}`。

- [ ] **Step 5: 修改 handleDeleteDoc：blob ref-count delete**

找到 `handleDeleteDoc`，在 `h.docStore.Delete(ctx, doc.ID)` 成功后追加：

```go
remaining, err := h.docStore.CountByBlobSHA256(r.Context(), doc.BlobSHA256)
if err != nil {
    logger.WarnContext(r.Context(), "kb.blob_ref_count_failed",
        slog.String("sha", doc.BlobSHA256), slog.Any("err", err))
} else if remaining == 0 {
    if err := h.blob.DeleteBlob(r.Context(), kb.WorkspaceRef, doc.BlobSHA256); err != nil {
        logger.WarnContext(r.Context(), "kb.blob_orphan",
            slog.String("sha", doc.BlobSHA256), slog.Any("err", err))
    }
}
```

(用 existing logger import；`slog` 已经全局可用。`logger` 变量按当前文件风格命名)

- [ ] **Step 6: 单元测试 mime 白名单**

`handler_doc_test.go`：

```go
func TestHandleUploadDoc_RejectsZip(t *testing.T) {
    h := newTestHandlerCtx(t, true)
    body := bytesWithMultipart(t, "file", "x.zip", []byte("PK\x03\x04...."))
    rec := doRequest(t, h, "POST", "/v1/knowledge-bases/<id>/documents", body)
    if rec.Code != 415 { t.Fatalf("want 415, got %d", rec.Code) }
    if !strings.Contains(rec.Body.String(), "kb.unsupported_mime") {
        t.Errorf("expected kb.unsupported_mime: %s", rec.Body.String())
    }
}

func TestHandleUploadDoc_AcceptsPDF(t *testing.T) { /* PDF magic header */ }
func TestHandleUploadDoc_AcceptsMarkdownByExt(t *testing.T) { /* .md fallback */ }
```

- [ ] **Step 7: Integration test — blob ref-count delete**

`handler_doc_test.go`：

```go
func TestHandleDeleteDoc_DropsBlobOnLastReference(t *testing.T) {
    // 1. Upload same content twice (same sha) → 2 docs share blob
    // 2. Delete doc1 → blob still exists in store
    // 3. Delete doc2 → blob deleted
    // 用真 objectstore.Memory 实例验证 Has() 状态
}
```

- [ ] **Step 8: 跑测试**

```bash
go test ./src/services/api/internal/http/kbapi/ ./src/services/api/internal/data/ -v -run 'Upload|Delete|BlobSHA'
```

Expected: PASS。

- [ ] **Step 9: Commit**

```bash
git add src/services/api/internal/http/kbapi/ src/services/api/internal/data/kb_documents_repo.go
git commit -m "feat(kb): mime sniff allowlist + blob ref-count delete (M2a Task 4)"
```

---

### Task 5: 部署级开关 — env 校验 + /v1/config 暴露

**Files:**
- Modify: `src/services/api/internal/app/config.go`
- Modify: `src/services/api/internal/app/app.go` — wire ExamIntegrationEnabled → kbapi.Deps
- Create or modify: `src/services/api/internal/http/configapi/handler.go`（按 codebase 现状）
- Modify: `src/services/api/internal/app/config_kb_debug_test.go` 或新建 `config_exam_test.go`

- [ ] **Step 1: 在 config.go 加 env 常量 + 字段**

`const (` 块追加：

```go
examIntegrationEnabledEnv = "ARKLOOP_EXAM_INTEGRATION_ENABLED"
examBaseURLEnv            = "EXAM_BASE_URL"
```

`Config` struct 加：

```go
ExamIntegrationEnabled bool
ExamBaseURL            string
```

- [ ] **Step 2: 在 Load 加解析 + validate**

在 `Load()` 函数现有 Validate 段前：

```go
cfg.ExamIntegrationEnabled = parseBoolEnv(examIntegrationEnabledEnv, false)
cfg.ExamBaseURL = strings.TrimSpace(os.Getenv(examBaseURLEnv))
```

在 Validate 段（或 Load 末尾）加：

```go
if cfg.ExamIntegrationEnabled && cfg.ExamBaseURL == "" {
    return Config{}, fmt.Errorf("%s=true but %s is empty",
        examIntegrationEnabledEnv, examBaseURLEnv)
}
```

(`parseBoolEnv` 已有；查同文件 helper)

- [ ] **Step 3: 单元测试 env 校验**

`config_exam_test.go`：

```go
package app

import (
    "strings"
    "testing"
)

func TestConfig_ExamIntegration_RequiresBaseURL(t *testing.T) {
    t.Setenv("ARKLOOP_EXAM_INTEGRATION_ENABLED", "true")
    t.Setenv("EXAM_BASE_URL", "")
    // 复用 Load 入口；如果 Load 还会校验其他必填 env，请用更细的 sub-validation 直调
    _, err := Load()
    if err == nil || !strings.Contains(err.Error(), "EXAM_BASE_URL is empty") {
        t.Fatalf("expected EXAM_BASE_URL empty error, got: %v", err)
    }
}

func TestConfig_ExamIntegration_DefaultsToFalse(t *testing.T) {
    t.Setenv("ARKLOOP_EXAM_INTEGRATION_ENABLED", "")
    cfg, err := Load()
    if err != nil { t.Fatal(err) }
    if cfg.ExamIntegrationEnabled {
        t.Error("expected false default")
    }
}
```

(如 `Load()` 还要其他必填 env，set them all to a fake-but-valid set in `t.Setenv`. Look at `config_kb_debug_test.go` for prior art.)

- [ ] **Step 4: app.go 把 cfg 注入到 kbapi.Deps**

找到 `kbapi.Deps{...}` 构造处（一般在 `app.go` 或 `register.go`），加 `ExamIntegrationEnabled: cfg.ExamIntegrationEnabled,`。

- [ ] **Step 5: /v1/config 暴露 flag**

找 `/v1/config` 现有 handler（grep `"/v1/config"`）。若已暴露 platform config struct，加字段：

```go
type platformConfigResp struct {
    // ... existing fields
    ExamIntegrationEnabled bool `json:"exam_integration_enabled"`
}

// in handler:
resp.ExamIntegrationEnabled = h.cfg.ExamIntegrationEnabled
```

若 `/v1/config` 不存在，新建 `src/services/api/internal/http/configapi/handler.go`：

```go
package configapi

import (
    nethttp "net/http"
    "encoding/json"
)

type Config struct {
    ExamIntegrationEnabled bool `json:"exam_integration_enabled"`
}

func Handler(cfg Config) nethttp.HandlerFunc {
    return func(w nethttp.ResponseWriter, r *nethttp.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(cfg)
    }
}
```

在 `register.go` 挂载：`mux.HandleFunc("GET /v1/config", configapi.Handler(configapi.Config{ExamIntegrationEnabled: cfg.ExamIntegrationEnabled}))`。

- [ ] **Step 6: integration test /v1/config**

```go
func TestConfigEndpoint_ExposesExamIntegrationFlag(t *testing.T) {
    h := configapi.Handler(configapi.Config{ExamIntegrationEnabled: true})
    req := httptest.NewRequest("GET", "/v1/config", nil)
    rec := httptest.NewRecorder()
    h(rec, req)
    if rec.Code != 200 { t.Fatalf("want 200, got %d", rec.Code) }
    if !strings.Contains(rec.Body.String(), `"exam_integration_enabled":true`) {
        t.Errorf("missing flag in body: %s", rec.Body.String())
    }
}
```

- [ ] **Step 7: 跑测试 + commit**

```bash
go test ./src/services/api/internal/app/ ./src/services/api/internal/http/configapi/ -v
```

```bash
git add src/services/api/internal/app/config.go src/services/api/internal/app/config_exam_test.go \
        src/services/api/internal/app/app.go src/services/api/internal/http/configapi/
git commit -m "feat(api): add ARKLOOP_EXAM_INTEGRATION_ENABLED env + expose via /v1/config (M2a Task 5)"
```

---

### Task 6: Persona loader gate + worker exam tool gate

**Files:**
- Modify: `src/services/api/internal/personas/loader.go`
- Modify: `src/services/api/internal/personas/loader_test.go`
- Modify: `src/services/worker/internal/tools/builtin/builtin.go`
- Modify: `src/services/worker/internal/tools/builtin/builtin_test.go`

- [ ] **Step 1: 在 loader.go 加 `ApplyExamIntegrationGate` 函数**

```go
// ApplyExamIntegrationGate flips user_selectable=false for exam-bound personas
// when integration is disabled. Idempotent.
func ApplyExamIntegrationGate(personas []RepoPersona, examIntegrationEnabled bool) []RepoPersona {
    if examIntegrationEnabled {
        return personas
    }
    for i := range personas {
        switch personas[i].ID {
        case "exam-agent", "exam-builder-agent":
            personas[i].UserSelectable = false
        }
    }
    return personas
}
```

- [ ] **Step 2: 调用方接入**

找 `LoadFromDir` 调用方（registry 装载阶段），在结果上调 `ApplyExamIntegrationGate(personas, cfg.ExamIntegrationEnabled)`。

- [ ] **Step 3: 单元测试**

```go
func TestApplyExamIntegrationGate_DisablesExamAgentWhenOff(t *testing.T) {
    personas := []RepoPersona{
        {ID: "exam-agent", UserSelectable: true},
        {ID: "book-tutor-agent", UserSelectable: true},
    }
    out := ApplyExamIntegrationGate(personas, false)
    if out[0].UserSelectable { t.Error("exam-agent should be disabled") }
    if !out[1].UserSelectable { t.Error("book-tutor-agent should stay enabled") }
}

func TestApplyExamIntegrationGate_PreservesWhenOn(t *testing.T) {
    personas := []RepoPersona{{ID: "exam-agent", UserSelectable: true}}
    out := ApplyExamIntegrationGate(personas, true)
    if !out[0].UserSelectable { t.Error("exam-agent should stay enabled") }
}
```

- [ ] **Step 4: worker builtin.go env gate**

找到 `exam.NewClient()` 调用段（按 builtin.go:114 附近）：

```go
var examExec tools.Executor
if os.Getenv("ARKLOOP_EXAM_INTEGRATION_ENABLED") != "true" {
    examExec = nil
    slog.Info("worker.exam_tools_disabled",
        slog.String("reason", "ARKLOOP_EXAM_INTEGRATION_ENABLED=false"))
} else if examClient, err := exam.NewClient(); err == nil {
    examExec = exam.NewExecutor(examClient)
} else {
    slog.Warn("worker.exam_tools_disabled",
        slog.String("reason", "exam.NewClient failed"),
        slog.Any("err", err))
}
```

- [ ] **Step 5: worker 单测 — 开关 off 时 exam_* 工具未注册**

`builtin_test.go`：

```go
func TestRegisterBuiltins_SkipsExamWhenEnvOff(t *testing.T) {
    t.Setenv("ARKLOOP_EXAM_INTEGRATION_ENABLED", "false")
    reg := NewTestRegistry(t)
    RegisterAll(reg, /* deps */)
    if reg.Has("exam_recognize_catalog_image") {
        t.Error("exam_recognize_catalog_image should NOT be registered")
    }
}
```

(`NewTestRegistry` 沿用既有；`reg.Has` 是 helper)

- [ ] **Step 6: 跑测试 + commit**

```bash
go test ./src/services/api/internal/personas/ ./src/services/worker/internal/tools/builtin/ -v
git add src/services/api/internal/personas/ src/services/worker/internal/tools/builtin/builtin.go src/services/worker/internal/tools/builtin/builtin_test.go
git commit -m "feat(api,worker): exam integration gate at persona loader + worker tool registry (M2a Task 6)"
```

---

### Task 7: shared/questionstore 接口 + factory + ErrIntegrationDisabled

**Files:**
- Create: `src/services/shared/questionstore/types.go`
- Create: `src/services/shared/questionstore/interface.go`
- Create: `src/services/shared/questionstore/factory.go`
- Create: `src/services/shared/questionstore/factory_test.go`

- [ ] **Step 1: types.go**

```go
package questionstore

import "time"

type QuestionOption struct {
    Key  string `json:"key"`
    Text string `json:"text"`
}

type SourceSnippet struct {
    ChunkRef   string    `json:"chunk_ref"`
    Snippet    string    `json:"snippet"`
    IngestTime time.Time `json:"ingest_time"`
}

type Question struct {
    ID               string
    KnowledgePointID string
    Type             string
    Difficulty       string
    Stem             string
    Options          []QuestionOption
    Answer           string
    Explanation      string
    SourceSnippets   []SourceSnippet
    PatternTag       string // 仅 examstore 用；localstore 写入时丢弃
    CreatedBySource  string
    CreatedAt        time.Time
}

type QuestionDraft struct {
    KnowledgePointID string
    Type             string
    Difficulty       string
    Stem             string
    Options          []QuestionOption
    Answer           string
    Explanation      string
    SourceSnippets   []SourceSnippet
    PatternTag       string
    CreatedBySource  string
}

type SavedQuestion struct {
    Index int
    ID    string
}

type SaveFailure struct {
    Index        int
    Draft        QuestionDraft
    ErrorCode    string
    ErrorMessage string
}

type SaveResult struct {
    Created []SavedQuestion
    Failed  []SaveFailure
}

type KnowledgePoint struct {
    ID        string
    Name      string
    ParentID  *string
    Depth     int
    SortOrder int
}

type PaperSpec struct {
    TotalCount                 int
    TypeDistribution           map[string]int
    DifficultyDistribution     map[string]int
    KnowledgePointDistribution map[string]int
    AllowDuplicateKP           bool
    ExcludeQuestionIDs         []string
}

type PaperID string

type ListFilter struct {
    Type       string
    Difficulty string
    PatternTag string
    Limit      int
    Offset     int
}

type Scope struct {
    CourseID string // examstore
    KBID     string // localstore
}

type KBDescriptor struct {
    ID              string
    IntegrationMode string
    ExamCourseID    string
}
```

- [ ] **Step 2: interface.go**

```go
package questionstore

import "context"

type QuestionStore interface {
    ListReferenceQuestions(ctx context.Context, knowledgePointID string, filter ListFilter) ([]Question, int, error)
    SaveQuestions(ctx context.Context, drafts []QuestionDraft) (SaveResult, error)
    ListKnowledgePoints(ctx context.Context, scope Scope) ([]KnowledgePoint, error)
    SavePaper(ctx context.Context, name string, scope Scope, spec PaperSpec, questionIDs []string, seed int64) (PaperID, error)
    ListQuestionsForPaperPool(ctx context.Context, knowledgePointIDs []string, filter ListFilter) ([]Question, error)
}
```

- [ ] **Step 3: factory.go**

```go
package questionstore

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
)

var (
    ErrIntegrationDisabled = errors.New("questionstore: exam integration disabled")
    ErrUnsupportedMode     = errors.New("questionstore: unsupported integration_mode")
)

// ExamClient 是 examstore.Client 的最小接口 ——
// factory 不直接依赖 examstore 子包，避免循环 import。
type ExamClient interface {
    // 标记接口；具体方法在 examstore 子包定义 + Store wrapper 内调用
    BaseURL() string
}

type Dependencies struct {
    DB              *sql.DB
    ExamClient      ExamClient
    OIDCTokenSource func(ctx context.Context) (string, error)
    // 工厂在 examstore mode 时把 KBDescriptor.ExamCourseID 透传到具体 Store
}

// NewLocalFunc / NewExamFunc 是子包注入点，避免顶层 import 它们
var (
    NewLocalFunc func(db *sql.DB) QuestionStore
    NewExamFunc  func(client ExamClient, tokenSource func(ctx context.Context) (string, error), courseID string) QuestionStore
)

func For(ctx context.Context, kb KBDescriptor, deps Dependencies) (QuestionStore, error) {
    switch kb.IntegrationMode {
    case "standalone":
        if NewLocalFunc == nil {
            return nil, fmt.Errorf("questionstore: localstore not registered")
        }
        return NewLocalFunc(deps.DB), nil
    case "exam":
        if deps.ExamClient == nil {
            return nil, ErrIntegrationDisabled
        }
        if NewExamFunc == nil {
            return nil, fmt.Errorf("questionstore: examstore not registered")
        }
        return NewExamFunc(deps.ExamClient, deps.OIDCTokenSource, kb.ExamCourseID), nil
    default:
        return nil, ErrUnsupportedMode
    }
}
```

- [ ] **Step 4: factory_test.go**

```go
package questionstore_test

import (
    "context"
    "testing"

    "arkloop/services/shared/questionstore"
)

type fakeStore struct{ questionstore.QuestionStore }

func TestFor_ExamMode_WithDisabledClient_ReturnsErr(t *testing.T) {
    _, err := questionstore.For(context.Background(),
        questionstore.KBDescriptor{IntegrationMode: "exam"},
        questionstore.Dependencies{})
    if err != questionstore.ErrIntegrationDisabled {
        t.Errorf("want ErrIntegrationDisabled, got %v", err)
    }
}

func TestFor_UnknownMode_ReturnsErr(t *testing.T) {
    _, err := questionstore.For(context.Background(),
        questionstore.KBDescriptor{IntegrationMode: "weird"},
        questionstore.Dependencies{})
    if err != questionstore.ErrUnsupportedMode {
        t.Errorf("want ErrUnsupportedMode, got %v", err)
    }
}

func TestFor_StandaloneMode_DelegatesToNewLocalFunc(t *testing.T) {
    var called bool
    questionstore.NewLocalFunc = func(_ *sql.DB) questionstore.QuestionStore {
        called = true
        return fakeStore{}
    }
    defer func() { questionstore.NewLocalFunc = nil }()

    _, err := questionstore.For(context.Background(),
        questionstore.KBDescriptor{IntegrationMode: "standalone"},
        questionstore.Dependencies{DB: nil})
    if err != nil { t.Fatal(err) }
    if !called { t.Error("NewLocalFunc not called") }
}
```

- [ ] **Step 5: 跑测试**

```bash
go test ./src/services/shared/questionstore/ -v
```

Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add src/services/shared/questionstore/types.go src/services/shared/questionstore/interface.go src/services/shared/questionstore/factory.go src/services/shared/questionstore/factory_test.go
git commit -m "feat(questionstore): interface + factory + ErrIntegrationDisabled (M2a Task 7)"
```

---

### Task 8: localstore real implementation

**Files:**
- Create: `src/services/shared/questionstore/localstore/store.go`
- Create: `src/services/shared/questionstore/localstore/store_integration_test.go`

> 注：若 M1.2 已落 localstore，本 task 改为"对齐签名"，复用其逻辑。先 grep 验证。

```bash
find /Users/jzefan/work/proj/ArkLoop/src -name "kb_questions_repo*" -o -name "kb_papers_repo*" 2>/dev/null
```

如果有，重构为按 `questionstore.QuestionStore` 接口包装即可；否则按下面新建。

- [ ] **Step 1: store.go 实现 QuestionStore**

```go
package localstore

import (
    "context"
    "database/sql"
    "fmt"

    "arkloop/services/shared/questionstore"
)

type Store struct {
    db *sql.DB
}

func New(db *sql.DB) questionstore.QuestionStore {
    return &Store{db: db}
}

func init() {
    questionstore.NewLocalFunc = func(db *sql.DB) questionstore.QuestionStore {
        return New(db)
    }
}

func (s *Store) ListReferenceQuestions(ctx context.Context, kpID string, filter questionstore.ListFilter) ([]questionstore.Question, int, error) {
    // SELECT FROM kb_questions WHERE knowledge_point_id=$1 AND type=$2 (optional) AND difficulty=$3 (optional)
    // ORDER BY created_at DESC LIMIT $4 OFFSET $5
    // pattern_tag filter 忽略（localstore 无此字段）
    // ...
}

func (s *Store) SaveQuestions(ctx context.Context, drafts []questionstore.QuestionDraft) (questionstore.SaveResult, error) {
    // 单事务批量 INSERT；任一 INSERT 失败时 → 该 draft 进 Failed，其他继续
    // pattern_tag 在 drafts 内但 localstore 写入时丢弃
    // ...
}

func (s *Store) ListKnowledgePoints(ctx context.Context, scope questionstore.Scope) ([]questionstore.KnowledgePoint, error) {
    // SELECT FROM kb_knowledge_points WHERE kb_id=$1
    // ...
}

func (s *Store) SavePaper(ctx context.Context, name string, scope questionstore.Scope, spec questionstore.PaperSpec, questionIDs []string, seed int64) (questionstore.PaperID, error) {
    // INSERT INTO kb_papers; returns generated id
    // ...
}

func (s *Store) ListQuestionsForPaperPool(ctx context.Context, kpIDs []string, filter questionstore.ListFilter) ([]questionstore.Question, error) {
    // SELECT FROM kb_questions WHERE knowledge_point_id = ANY($1) ...
    // ...
}
```

(SQL 实际写法看 M1.2 落地代码，本 plan 给接口契约骨架。)

- [ ] **Step 2: 写 integration tests (真 Postgres)**

仿 `runs_repo_integration_test.go` 风格：

```go
func TestLocalStore_SaveQuestions_PartialFailure(t *testing.T) {
    // 准备 2 个 draft，其中 1 个 knowledge_point_id 不存在
    // → SaveResult.Created=1, Failed=1
}

func TestLocalStore_ListReferenceQuestions_FiltersByTypeAndDifficulty(t *testing.T) { ... }

func TestLocalStore_SavePaper_ReturnsID(t *testing.T) { ... }
```

- [ ] **Step 3: 跑测试**

```bash
go test ./src/services/shared/questionstore/localstore/ -v -run Integration
```

Expected: PASS（需真 Postgres）。

- [ ] **Step 4: Commit**

```bash
git add src/services/shared/questionstore/localstore/
git commit -m "feat(questionstore): localstore implementation (M2a Task 8)"
```

---

### Task 9: examstore real implementation（**前置：Task 1 frozen-v1**）

**Files:**
- Create: `src/services/shared/questionstore/examstore/client.go`
- Create: `src/services/shared/questionstore/examstore/client_test.go`
- Create: `src/services/shared/questionstore/examstore/store.go`
- Create: `src/services/shared/questionstore/examstore/types.go`

- [ ] **Step 1: client.go 核心结构**

```go
package examstore

import (
    "context"
    "net/http"
    "time"
)

type Client struct {
    httpClient    *http.Client
    baseURL       string
    apiVersion    string  // "1"
    maxConcurrent int
    retryPolicy   RetryPolicy
    sema          chan struct{}
}

type RetryPolicy struct {
    MaxAttempts int
    BaseDelay   time.Duration
    MaxDelay    time.Duration
}

func New(baseURL string) *Client {
    return &Client{
        httpClient: &http.Client{Timeout: 30 * time.Second},
        baseURL:    baseURL,
        apiVersion: "1",
        maxConcurrent: 4,
        retryPolicy: RetryPolicy{MaxAttempts: 3, BaseDelay: 250 * time.Millisecond, MaxDelay: 5 * time.Second},
        sema:       make(chan struct{}, 4),
    }
}

// BaseURL — satisfies questionstore.ExamClient marker interface
func (c *Client) BaseURL() string { return c.baseURL }

// doJSON wraps: concurrency limiter + retry on 5xx/network + structured error parsing.
func (c *Client) doJSON(ctx context.Context, method, path, token string, body any, dst any) error {
    // 详见下面 Step 2
}
```

- [ ] **Step 2: doJSON 重试 + 错误分类**

```go
func (c *Client) doJSON(ctx context.Context, method, path, token string, body any, dst any) error {
    select {
    case c.sema <- struct{}{}:
    case <-ctx.Done():
        return ctx.Err()
    }
    defer func() { <-c.sema }()

    var lastErr error
    for attempt := 1; attempt <= c.retryPolicy.MaxAttempts; attempt++ {
        var bodyReader io.Reader
        if body != nil {
            b, err := json.Marshal(body)
            if err != nil { return fmt.Errorf("marshal: %w", err) }
            bodyReader = bytes.NewReader(b)
        }
        req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
        if err != nil { return fmt.Errorf("new request: %w", err) }
        req.Header.Set("Authorization", "Bearer "+token)
        req.Header.Set("X-ArkLoop-API-Version", c.apiVersion)
        if body != nil { req.Header.Set("Content-Type", "application/json") }

        resp, err := c.httpClient.Do(req)
        if err != nil {
            lastErr = err
            if attempt < c.retryPolicy.MaxAttempts {
                sleepBackoff(ctx, c.retryPolicy, attempt)
                continue
            }
            return fmt.Errorf("http: %w", err)
        }
        respBody, _ := io.ReadAll(resp.Body)
        resp.Body.Close()

        switch {
        case resp.StatusCode >= 500:
            lastErr = &ServerError{Status: resp.StatusCode, Body: string(respBody)}
            if attempt < c.retryPolicy.MaxAttempts {
                sleepBackoff(ctx, c.retryPolicy, attempt)
                continue
            }
            return lastErr
        case resp.StatusCode == 401 || resp.StatusCode == 403:
            return &AuthError{Status: resp.StatusCode, Body: string(respBody)}
        case resp.StatusCode >= 400:
            return &ClientError{Status: resp.StatusCode, Body: string(respBody)}
        }
        if dst != nil {
            if err := json.Unmarshal(respBody, dst); err != nil {
                return fmt.Errorf("unmarshal: %w", err)
            }
        }
        return nil
    }
    return lastErr
}

type ServerError struct{ Status int; Body string }
func (e *ServerError) Error() string { return fmt.Sprintf("examstore: server %d: %s", e.Status, e.Body) }

type ClientError struct{ Status int; Body string }
func (e *ClientError) Error() string { return fmt.Sprintf("examstore: client %d: %s", e.Status, e.Body) }

type AuthError struct{ Status int; Body string }
func (e *AuthError) Error() string { return fmt.Sprintf("examstore: auth %d: %s", e.Status, e.Body) }

func sleepBackoff(ctx context.Context, p RetryPolicy, attempt int) {
    d := p.BaseDelay * time.Duration(1<<(attempt-1))
    if d > p.MaxDelay { d = p.MaxDelay }
    t := time.NewTimer(d)
    defer t.Stop()
    select {
    case <-t.C:
    case <-ctx.Done():
    }
}
```

- [ ] **Step 3: 4 端点方法**

```go
// 详细参数对齐 exam-api.md frozen-v1

func (c *Client) ListKnowledgePoints(ctx context.Context, token, courseID string, limit, offset int) (KPListResp, error) {
    path := fmt.Sprintf("/api/knowledge-points?course_id=%s&limit=%d&offset=%d",
        url.QueryEscape(courseID), limit, offset)
    var resp KPListResp
    err := c.doJSON(ctx, "GET", path, token, nil, &resp)
    return resp, err
}

func (c *Client) ListQuestions(ctx context.Context, token, kpID string, filter ListFilter) (QListResp, error) {
    qs := url.Values{}
    qs.Set("knowledge_point_id", kpID)
    if filter.Type != "" { qs.Set("type", filter.Type) }
    if filter.Difficulty != "" { qs.Set("difficulty", filter.Difficulty) }
    if filter.PatternTag != "" { qs.Set("pattern_tag", filter.PatternTag) }
    if filter.Limit > 0 { qs.Set("limit", strconv.Itoa(filter.Limit)) }
    if filter.Offset > 0 { qs.Set("offset", strconv.Itoa(filter.Offset)) }
    var resp QListResp
    err := c.doJSON(ctx, "GET", "/api/questions?"+qs.Encode(), token, nil, &resp)
    return resp, err
}

func (c *Client) CreateQuestionsBatch(ctx context.Context, token string, drafts []DraftReq) (BatchResp, error) {
    var resp BatchResp
    err := c.doJSON(ctx, "POST", "/api/questions/batch", token,
        struct{ Questions []DraftReq `json:"questions"` }{drafts}, &resp)
    return resp, err
}

func (c *Client) CreatePaper(ctx context.Context, token string, req PaperReq) (PaperResp, error) {
    var resp PaperResp
    err := c.doJSON(ctx, "POST", "/api/papers", token, req, &resp)
    return resp, err
}

// 列课程 — 给 KB UI 用
func (c *Client) ListCourses(ctx context.Context, token string) (CourseListResp, error) {
    var resp CourseListResp
    err := c.doJSON(ctx, "GET", "/api/courses", token, nil, &resp)
    return resp, err
}
```

- [ ] **Step 4: types.go — request/response shapes**

按 frozen-v1 exam-api.md 字段一一对应：

```go
package examstore

type KPListResp struct {
    Items []KPItem `json:"items"`
    Total int      `json:"total"`
}

type KPItem struct {
    ID       string  `json:"id"`
    CourseID string  `json:"course_id"`
    Name     string  `json:"name"`
    ParentID *string `json:"parent_id"`
    Depth    int     `json:"depth"`
    SortOrder int    `json:"sort_order"`
}

type QListResp struct {
    Items []QItem `json:"items"`
    Total int     `json:"total"`
}

type QItem struct {
    ID               string            `json:"id"`
    KnowledgePointID string            `json:"knowledge_point_id"`
    Type             string            `json:"type"`
    Difficulty       string            `json:"difficulty"`
    Stem             string            `json:"stem"`
    Options          []OptionItem      `json:"options"`
    Answer           string            `json:"answer"`
    Explanation      string            `json:"explanation"`
    SourceSnippets   []SnippetItem     `json:"source_snippets"`
    PatternTag       string            `json:"pattern_tag"`
    CreatedAt        time.Time         `json:"created_at"`
    CreatedBySource  string            `json:"created_by_source"`
}

type OptionItem struct {
    Key  string `json:"key"`
    Text string `json:"text"`
}

type SnippetItem struct {
    ChunkRef   string    `json:"chunk_ref"`
    Snippet    string    `json:"snippet"`
    IngestTime time.Time `json:"ingest_time"`
}

type DraftReq struct {
    KnowledgePointID string         `json:"knowledge_point_id"`
    Type             string         `json:"type"`
    Difficulty       string         `json:"difficulty"`
    Stem             string         `json:"stem"`
    Options          []OptionItem   `json:"options,omitempty"`
    Answer           string         `json:"answer"`
    Explanation      string         `json:"explanation,omitempty"`
    SourceSnippets   []SnippetItem  `json:"source_snippets,omitempty"`
    PatternTag       string         `json:"pattern_tag,omitempty"`
    CreatedBySource  string         `json:"created_by_source"`
}

type BatchResp struct {
    Created []BatchCreated `json:"created"`
    Failed  []BatchFailed  `json:"failed"`
}

type BatchCreated struct {
    Index int    `json:"index"`
    ID    string `json:"id"`
}

type BatchFailed struct {
    Index        int    `json:"index"`
    ErrorCode    string `json:"error_code"`
    ErrorMessage string `json:"error_message"`
}

type PaperReq struct {
    Name        string         `json:"name"`
    CourseID    string         `json:"course_id"`
    Spec        PaperSpecReq   `json:"spec"`
    QuestionIDs []string       `json:"question_ids"`
}

type PaperSpecReq struct {
    TotalCount                 int            `json:"total_count"`
    TypeDistribution           map[string]int `json:"type_distribution"`
    DifficultyDistribution     map[string]int `json:"difficulty_distribution"`
    KnowledgePointDistribution map[string]int `json:"knowledge_point_distribution"`
    Seed                       int64          `json:"seed"`
}

type PaperResp struct {
    ID            string    `json:"id"`
    Name          string    `json:"name"`
    QuestionCount int       `json:"question_count"`
    CreatedAt     time.Time `json:"created_at"`
}

type CourseListResp struct {
    Items []CourseItem `json:"items"`
}

type CourseItem struct {
    ID      string `json:"id"`
    Name    string `json:"name"`
    OwnerID string `json:"owner_id"`
}

type ListFilter struct {
    Type, Difficulty, PatternTag string
    Limit, Offset                int
}
```

- [ ] **Step 5: store.go — questionstore.QuestionStore 适配器**

```go
package examstore

import (
    "context"

    "arkloop/services/shared/questionstore"
)

type Store struct {
    client      *Client
    tokenSource func(ctx context.Context) (string, error)
    courseID    string
}

func NewStore(client *Client, tokenSource func(ctx context.Context) (string, error), courseID string) questionstore.QuestionStore {
    return &Store{client: client, tokenSource: tokenSource, courseID: courseID}
}

func init() {
    questionstore.NewExamFunc = func(client questionstore.ExamClient, ts func(ctx context.Context) (string, error), courseID string) questionstore.QuestionStore {
        c, ok := client.(*Client)
        if !ok { return nil }
        return NewStore(c, ts, courseID)
    }
}

func (s *Store) ListReferenceQuestions(ctx context.Context, kpID string, filter questionstore.ListFilter) ([]questionstore.Question, int, error) {
    token, err := s.tokenSource(ctx)
    if err != nil { return nil, 0, fmt.Errorf("oidc token: %w", err) }
    resp, err := s.client.ListQuestions(ctx, token, kpID, ListFilter{
        Type: filter.Type, Difficulty: filter.Difficulty, PatternTag: filter.PatternTag,
        Limit: filter.Limit, Offset: filter.Offset,
    })
    if err != nil { return nil, 0, err }
    items := make([]questionstore.Question, len(resp.Items))
    for i, q := range resp.Items {
        items[i] = mapToQuestionstore(q)
    }
    return items, resp.Total, nil
}

// SaveQuestions, ListKnowledgePoints, SavePaper, ListQuestionsForPaperPool — 类似 mapping
// ...
```

(`mapToQuestionstore` 把 examstore types → questionstore types)

- [ ] **Step 6: client_test.go — httptest 全套**

```go
func TestClient_ListQuestions_TransmitsPatternTagFilter(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Query().Get("pattern_tag") != "A2" {
            t.Errorf("missing pattern_tag in query: %s", r.URL.RawQuery)
        }
        w.WriteHeader(200)
        w.Write([]byte(`{"items":[],"total":0}`))
    }))
    defer srv.Close()
    c := New(srv.URL)
    _, err := c.ListQuestions(context.Background(), "tok", "kp1", ListFilter{PatternTag: "A2"})
    if err != nil { t.Fatal(err) }
}

func TestClient_CreateQuestionsBatch_PartialSuccessMapping(t *testing.T) { /* 返 {created:[{index:0}],failed:[{index:1, error_code:"x"}]} → mapping 正确 */ }
func TestClient_5xx_RetriesThenGivesUp(t *testing.T) { /* server 一直 500 → MaxAttempts 后冒泡 ServerError */ }
func TestClient_5xx_RetriesAndSucceeds(t *testing.T) { /* 第 2 次 200 → 成功 */ }
func TestClient_401_NoRetry_WrapsAuthError(t *testing.T) { /* server 401 → AuthError, attempts==1 */ }
func TestClient_ConcurrencyLimit(t *testing.T) { /* 触发 5 个并发，验证 server side max in-flight ≤4 */ }
func TestClient_SetsAuthAndVersionHeaders(t *testing.T) { /* 检 Authorization Bearer + X-ArkLoop-API-Version */ }
```

- [ ] **Step 7: 跑测试**

```bash
go test ./src/services/shared/questionstore/examstore/ -v
```

Expected: PASS。

- [ ] **Step 8: Commit**

```bash
git add src/services/shared/questionstore/examstore/
git commit -m "feat(examstore): HTTP client + 4 endpoints + retry + concurrency limiter (M2a Task 9)"
```

---

### Task 10: kb_extract_toc 工具 + /v1/exam/courses 代理 + book-tutor-agent prompt

**Files:**
- Create: `src/services/api/internal/http/examapi/handler.go`
- Create: `src/services/api/internal/http/examapi/handler_test.go`
- Modify: `src/services/api/internal/http/kbapi/handler_toc.go`（or add to handler_doc.go）
- Modify: `src/services/worker/internal/tools/builtin/kb/spec.go`
- Modify: `src/services/worker/internal/tools/builtin/kb/executor.go`
- Modify: `src/personas/book-tutor-agent/prompt.md`

- [ ] **Step 1: api /v1/exam/courses 代理**

`src/services/api/internal/http/examapi/handler.go`：

```go
package examapi

import (
    "encoding/json"
    nethttp "net/http"

    "arkloop/services/shared/questionstore/examstore"
)

type Deps struct {
    Client *examstore.Client // nil 时 endpoint 不挂
    TokenFromRequest func(r *nethttp.Request) (string, error)
}

func RegisterRoutes(mux *nethttp.ServeMux, deps Deps) {
    if deps.Client == nil { return }  // integration disabled
    mux.HandleFunc("GET /v1/exam/courses", func(w nethttp.ResponseWriter, r *nethttp.Request) {
        token, err := deps.TokenFromRequest(r)
        if err != nil {
            writeErr(w, 401, "auth.unauthenticated", err.Error())
            return
        }
        resp, err := deps.Client.ListCourses(r.Context(), token)
        if err != nil {
            writeErr(w, 502, "exam.upstream_failed", err.Error())
            return
        }
        json.NewEncoder(w).Encode(resp)
    })
}
```

`app.go` 在 RegisterRoutes 段：

```go
if cfg.ExamIntegrationEnabled {
    examClient := examstore.New(cfg.ExamBaseURL)
    examapi.RegisterRoutes(mux, examapi.Deps{
        Client:           examClient,
        TokenFromRequest: tokenFromActor,
    })
}
```

- [ ] **Step 2: /v1/knowledge-bases/:id/documents/:doc_id/toc**

`handler_toc.go`：

```go
func handleGetTOC(h *handlerCtx) nethttp.HandlerFunc {
    return func(w nethttp.ResponseWriter, r *nethttp.Request) {
        // ... actor + kb load 同其他 handler ...
        docID, err := uuid.Parse(r.PathValue("doc_id"))
        if err != nil { writeErr(w, 400, "kb.bad_doc_id", ""); return }
        doc, err := h.docStore.GetByID(r.Context(), docID)
        if err != nil || doc == nil || doc.KBID != kb.ID {
            writeErr(w, 404, "kb.doc_not_found", "")
            return
        }
        toc := extractTOCFromMeta(doc.ParseMetaJSON) // from kb_documents.parse_meta_json
        if toc == nil || len(toc.Nodes) < 5 {
            writeJSON(w, 200, map[string]any{"tree": nil, "node_count": 0})
            return
        }
        writeJSON(w, 200, map[string]any{"tree": toc, "node_count": len(toc.Nodes)})
    }
}
```

`extractTOCFromMeta` 把 `parse_meta_json.toc` 字段（M1.1 PDF 解析已存）归一化为 `[{name, depth, children}]` 树。

`register.go`：`mux.HandleFunc("GET /v1/knowledge-bases/{id}/documents/{doc_id}/toc", withMiddleware(handleGetTOC(h)))`。

- [ ] **Step 3: worker kb_extract_toc 工具**

`src/services/worker/internal/tools/builtin/kb/spec.go`：

```go
var ExtractTOCAgentSpec = tools.AgentToolSpec{
    Name:        "kb_extract_toc",
    Version:     "1",
    Description: "extract a document's TOC tree (heading hierarchy) for downstream catalog tree creation in exam",
    RiskLevel:   tools.RiskLevelLow,
    SideEffects: false,
}

var ExtractTOCLlmSpec = llm.ToolSpec{
    Name: "kb_extract_toc",
    Description: strPtr("Extract the TOC (table of contents) tree from an already-ingested KB document. Returns a tree (course → chapters → sections) in the same shape as exam_create_catalog_tree input. Returns nil tree + node_count=0 if TOC is missing or too small (<5 nodes)."),
    JSONSchema: map[string]any{
        "type": "object",
        "properties": map[string]any{
            "kb_id":       map[string]any{"type": "string"},
            "document_id": map[string]any{"type": "string"},
        },
        "required": []string{"kb_id", "document_id"},
    },
}
```

`executor.go` 加 `extractTOC(ctx, args)` 调 api `GET .../toc`，返回原样到 LLM。

- [ ] **Step 4: book-tutor-agent prompt 追加 Linked TOC 流**

`src/personas/book-tutor-agent/prompt.md` 在适当位置（如"上传文档后"段落）插入：

```markdown
## Linked KB 首份文档处理（仅 kb.integration_mode == "exam"）

文档进入 `ready` 状态后，第一次时检查并提议建目录：

1. 调 `kb_extract_toc(kb_id=<>, document_id=<>)`；
2. 若返回 `node_count >= 5`：show_widget 把 tree 渲染给老师，含 [确认建目录] [跳过] 两个选项；
3. 老师确认 → 调 `exam_create_catalog_tree(course_id=<kb.exam_course_id>, tree=<上一步>)`；
4. 老师跳过 / `node_count < 5`：自然进入下一步交互（搜索/出题），不强求建目录。

注：Standalone KB 不走此流。
```

- [ ] **Step 5: 测试**

```bash
go test ./src/services/api/internal/http/examapi/ ./src/services/api/internal/http/kbapi/ ./src/services/worker/internal/tools/builtin/kb/ -v
```

httptest 桩 examstore + 桩 kb_documents 验证：
- /v1/exam/courses 开关 off 时 404
- /v1/.../toc 取 parse_meta_json.toc，节点数 < 5 返 `{tree: nil}`

- [ ] **Step 6: Commit**

```bash
git add src/services/api/internal/http/examapi/ src/services/api/internal/http/kbapi/handler_toc.go \
        src/services/api/internal/http/kbapi/register.go \
        src/services/worker/internal/tools/builtin/kb/ src/personas/book-tutor-agent/prompt.md
git commit -m "feat(kb): kb_extract_toc tool + /v1/exam/courses proxy + book-tutor-agent Linked TOC flow (M2a Task 10)"
```

---

### Task 11: console-lite KB 表单 + 列表徽章

**Files:**
- Modify: `src/apps/console-lite/src/components/CreateKBModal.tsx`
- Modify: `src/apps/console-lite/src/pages/KnowledgeBasesPage.tsx`
- Create: `src/apps/console-lite/src/components/ExamCourseSelect.tsx`
- Modify: `src/apps/console-lite/src/api/*.ts` — 加 `getPlatformConfig` / `listExamCourses`

- [ ] **Step 1: api 客户端**

`src/apps/console-lite/src/api/platform.ts`（或同等位置）：

```typescript
export type PlatformConfig = {
  exam_integration_enabled: boolean
}

export async function getPlatformConfig(): Promise<PlatformConfig> {
  const r = await fetch('/v1/config')
  if (!r.ok) throw new Error('config fetch failed')
  return r.json()
}

export type ExamCourse = { id: string; name: string }

export async function listExamCourses(): Promise<ExamCourse[]> {
  const r = await fetch('/v1/exam/courses', { credentials: 'include' })
  if (!r.ok) throw new Error('courses fetch failed')
  const j = await r.json()
  return j.items
}
```

- [ ] **Step 2: ExamCourseSelect 组件**

```tsx
import { useQuery } from '@tanstack/react-query' // 沿用既有
import { listExamCourses } from '../api/platform'

export function ExamCourseSelect({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const { data, isLoading, error } = useQuery({ queryKey: ['exam-courses'], queryFn: listExamCourses })
  if (isLoading) return <div>加载课程中...</div>
  if (error) return <div className="text-red-500">加载失败：{String(error)}</div>
  return (
    <select value={value} onChange={(e) => onChange(e.target.value)} className="...">
      <option value="">— 选择 exam 课程 —</option>
      {data!.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
    </select>
  )
}
```

- [ ] **Step 3: CreateKBModal 加 visibility + integration_mode 字段**

读现状 `CreateKBModal.tsx`，在表单加：

```tsx
const cfg = useQuery({ queryKey: ['platform-config'], queryFn: getPlatformConfig })

const [visibility, setVisibility] = useState<'workspace_member' | 'private'>('workspace_member')
const [integrationMode, setIntegrationMode] = useState<'standalone' | 'exam'>('standalone')
const [examCourseID, setExamCourseID] = useState('')

// 提交时校验
async function submit() {
  if (integrationMode === 'exam' && !examCourseID) {
    setError('请选择要绑定的 exam 课程')
    return
  }
  await api.createKB({ name, workspace_ref: ws, description, visibility,
                        integration_mode: integrationMode,
                        exam_course_id: integrationMode === 'exam' ? examCourseID : undefined })
}

// 表单 JSX：
<Field label="可见性">
  <Radio name="vis" value="workspace_member" checked={visibility==='workspace_member'} onChange={() => setVisibility('workspace_member')}>工作区全员可见</Radio>
  <Radio name="vis" value="private" checked={visibility==='private'} onChange={() => setVisibility('private')}>仅自己可见</Radio>
</Field>

{cfg.data?.exam_integration_enabled && (
  <Field label="集成模式" required>
    <Radio name="mode" value="standalone" checked={integrationMode==='standalone'} onChange={() => setIntegrationMode('standalone')}>独立模式（题目存在 ArkLoop）</Radio>
    <Radio name="mode" value="exam" checked={integrationMode==='exam'} onChange={() => setIntegrationMode('exam')}>绑定 exam 课程（题目写回 exam）</Radio>
  </Field>
)}

{integrationMode === 'exam' && (
  <Field label="exam 课程" required>
    <ExamCourseSelect value={examCourseID} onChange={setExamCourseID} />
    <Hint>KB 创建后无法切换模式。</Hint>
  </Field>
)}
```

- [ ] **Step 4: KnowledgeBasesPage 列表徽章**

```tsx
{kb.visibility === 'private' && <Badge tone="gray">私有</Badge>}
{kb.integration_mode === 'exam' && <Badge tone="blue">已绑定 exam{kb.exam_course_name ? `: 《${kb.exam_course_name}》` : ''}</Badge>}
```

(`exam_course_name` 由后端 KB list 返回时 join，或前端通过 listExamCourses 解析；本 plan 简化为前端 lazy join — 调一次 listExamCourses 缓存到 map)

- [ ] **Step 5: 跑构建 + lint**

```bash
cd src/apps/console-lite
pnpm type-check && pnpm lint && pnpm build
```

Expected: 全绿。

- [ ] **Step 6: 手测**

```bash
# 起 api（开关 on + EXAM_BASE_URL=https://mock-exam.local）
# 起 console-lite dev server
# 浏览器：
# 1. 打开新建 KB modal — 看到模式 + 可见性字段
# 2. 切到 Standalone — 课程下拉消失
# 3. 切到 Linked → 课程下拉出现 → 选一个 → 提交 → KB 列表里看到蓝色徽章
# 4. 把 ARKLOOP_EXAM_INTEGRATION_ENABLED 设 false 重启 api → 刷新 → 模式字段消失
```

- [ ] **Step 7: Commit**

```bash
git add src/apps/console-lite/src/
git commit -m "feat(console-lite): KB form visibility + integration_mode + exam course select (M2a Task 11)"
```

---

### Task 12: Acceptance + Linked KB E2E smoke

**Files:**
- Create: `tests/smoke/m2a_linked_kb_smoke.sh` (or `.go` per repo convention)
- Modify: `docs/superpowers/specs/2026-05-23-book-kb-rag-m2a-acceptance.md`（新建 acceptance log）

- [ ] **Step 1: 跑全套 Go 测试**

```bash
cd /Users/jzefan/work/proj/ArkLoop
go test ./src/services/... -race -count=1
```

Expected: 全 PASS。失败立刻定位是哪一个 task 引入。

- [ ] **Step 2: 跑全套 console-lite 测试**

```bash
cd src/apps/console-lite
pnpm test && pnpm type-check && pnpm build
```

- [ ] **Step 3: Linked KB E2E smoke 脚本**

`tests/smoke/m2a_linked_kb_smoke.sh`：

```bash
#!/usr/bin/env bash
set -euo pipefail

# Prereq:
#   - ARKLOOP_EXAM_INTEGRATION_ENABLED=true
#   - EXAM_BASE_URL=<mock httptest server URL>
#   - api 已起、worker 已起、PG 已 migrate

# 1. 创建 Linked KB
KB=$(curl -sS -X POST http://localhost:19001/v1/knowledge-bases \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"name":"m2a-smoke","workspace_ref":"ws-test","integration_mode":"exam","exam_course_id":"course-test"}' \
    | jq -r .id)
[[ -n "$KB" ]] || { echo "FAIL: create KB"; exit 1; }

# 2. 上传 PDF
DOC=$(curl -sS -X POST http://localhost:19001/v1/knowledge-bases/$KB/documents \
    -H "Authorization: Bearer $TOKEN" \
    -F "file=@tests/fixtures/short.pdf" | jq -r .document_id)

# 3. 等 ready
for i in $(seq 1 30); do
  STATUS=$(curl -sS http://localhost:19001/v1/knowledge-bases/$KB/documents/$DOC \
      -H "Authorization: Bearer $TOKEN" | jq -r .status)
  [[ "$STATUS" == "ready" ]] && break
  sleep 2
done
[[ "$STATUS" == "ready" ]] || { echo "FAIL: doc not ready, last status=$STATUS"; exit 1; }

# 4. kb_extract_toc 走 api endpoint
TOC=$(curl -sS http://localhost:19001/v1/knowledge-bases/$KB/documents/$DOC/toc \
    -H "Authorization: Bearer $TOKEN")
NODES=$(echo "$TOC" | jq -r .node_count)
echo "TOC nodes: $NODES"

# 5. 删 KB（级联清 docs + blob）
curl -sS -X DELETE http://localhost:19001/v1/knowledge-bases/$KB -H "Authorization: Bearer $TOKEN"

echo "PASS"
```

- [ ] **Step 4: 跑 smoke**

```bash
# 起 mock exam httptest server (Go scratch program) + api/worker
ARKLOOP_EXAM_INTEGRATION_ENABLED=true EXAM_BASE_URL=http://127.0.0.1:9090 \
    bash tests/smoke/m2a_linked_kb_smoke.sh
```

- [ ] **Step 5: 写 acceptance 文档**

`docs/superpowers/specs/2026-05-23-book-kb-rag-m2a-acceptance.md` 按 M1.0 acceptance 风格逐项打勾：

```markdown
# M2a Acceptance Log

> Validated against the M2a design's 验收表 (acceptance checklist)。

| Check | Status | Evidence |
| --- | --- | --- |
| 00194 应用，visibility 默认 workspace_member | ✅ | psql `\d knowledge_bases` |
| 私有 KB 仅 creator 可见 | ✅ | TestVisibility_PrivateKBHiddenFromNonCreator PASS |
| 上传 .zip 返 415 kb.unsupported_mime | ✅ | TestHandleUploadDoc_RejectsZip PASS |
| 同 sha 上传两次：删第一份 blob 还在；删第二份 blob 被 Delete | ✅ | TestHandleDeleteDoc_DropsBlobOnLastReference PASS |
| ARKLOOP_EXAM_INTEGRATION_ENABLED=true + EXAM_BASE_URL="" 启动报错 | ✅ | TestConfig_ExamIntegration_RequiresBaseURL PASS |
| 开关 false 时 exam-agent 不在 /v1/personas | ✅ | curl /v1/personas 输出 |
| worker 启动日志含 "exam_tools_disabled" | ✅ | log greppy |
| questionstore.For(mode=exam, disabled) 返 ErrIntegrationDisabled | ✅ | TestFor_ExamMode_WithDisabledClient_ReturnsErr PASS |
| examstore httptest 全套 PASS | ✅ | examstore/client_test.go all PASS |
| examstore 部分失败 1:1 还原 | ✅ | TestClient_CreateQuestionsBatch_PartialSuccessMapping PASS |
| console-lite 新建 KB 含可见性 + 模式下拉，徽章正确 | ✅ | screenshot attached |
| Linked KB E2E smoke 通 | ✅ | m2a_linked_kb_smoke.sh exit 0 |
| exam-api.md 状态 = frozen-v1 | ✅ | doc head: `Status: frozen-v1` |
```

- [ ] **Step 6: 最终 commit**

```bash
git add tests/smoke/m2a_linked_kb_smoke.sh docs/superpowers/specs/2026-05-23-book-kb-rag-m2a-acceptance.md
git commit -m "test: M2a Linked KB E2E smoke + acceptance log (M2a Task 12)"
```

- [ ] **Step 7: 推 PR**

```bash
git push -u origin book-kb-rag/m2a
gh pr create --title "M2a: Linked KB + examstore real impl + Spike S2 freeze" --body "$(cat <<'EOF'
## Summary

Closes book-kb-rag M2a per design `docs/superpowers/specs/2026-05-23-book-kb-rag-m2a-design.md`.

- Migration 00194: visibility + exam_mode_requires_course constraints
- kbapi: createKB 校验链、ensureKBVisible、sniff mime 白名单、blob 引用计数 delete
- Deploy switch ARKLOOP_EXAM_INTEGRATION_ENABLED：api Config validate、persona loader gate、worker exam tool gate、/v1/config 暴露
- shared/questionstore：interface + factory + localstore + examstore（真实现）
- /v1/exam/courses 代理 + kb_extract_toc 工具 + book-tutor-agent Linked TOC 流
- console-lite: KB 表单可见性 + 模式 + 课程下拉 + 列表徽章

## Test plan

- [x] go test ./src/services/... -race（全绿）
- [x] pnpm test / type-check / build console-lite
- [x] Linked KB E2E smoke script PASS
- [x] Acceptance log filed at docs/superpowers/specs/2026-05-23-book-kb-rag-m2a-acceptance.md
- [x] exam-api.md status = frozen-v1（Task 1 PR 已 merge）
EOF
)"
```

---

## Self-Review Notes

**Spec coverage check (M2a design §范围摘要):**
- ✅ Migration 00194 — Task 2
- ⚠️ 设计原本提 00195 (integration_mode defaults/check)，但实际 00193 已含 check IN ('standalone','exam') — 故 Task 2 只新建 00194，补 visibility + exam_mode_requires_course；这是设计文档与实现的偏差，但 plan 处理正确（comment 写明）
- ✅ kbapi visibility/mime/blob — Task 3+4
- ✅ kbapi 创建 KB integration_mode 校验 + /v1/exam/courses + /v1/config — Task 3 + 5 + 10
- ✅ deploy 开关 env + persona loader + worker tool gate — Task 5 + 6
- ✅ questionstore interface + localstore + examstore — Task 7+8+9
- ✅ kb_extract_toc + book-tutor-agent TOC — Task 10
- ✅ console-lite — Task 11
- ✅ Spike S2 合约 freeze — Task 1
- ✅ Acceptance — Task 12

**Placeholder scan:**
- localstore SQL 写法在 Task 8 标"看 M1.2 落地代码"——若 M1.2 未落，需展开为完整 SQL。**当 plan 实施到 Task 8 时若 grep 没找到 M1.2 实现，需先回头补 M1.2 spec/plan**（这是 PRD §里程碑明确的 M1.2 范围）
- Task 4 step 5 "(`workspaceBlobKey` 沿用既有...)" — 若不存在需当场加一行 helper

**Type consistency:**
- `KnowledgeBase.Visibility` / `KBCreate.Visibility` 一致
- `ExamCourseID *string` 一致
- `questionstore.QuestionStore` interface 在 factory_test 用 marker; localstore + examstore 都 implement 同一 interface
- examstore 内部 `ListFilter` / `OptionItem` 与 questionstore 顶层 type 不同包同名 —— mapping 函数 `mapToQuestionstore` 显式转换
- `kb.integration_mode` 字符串值统一 'standalone' / 'exam'，前端后端 SQL 三处一致

---

## Execution Notes

- Task 1 (exam-api freeze) **阻塞 Task 9 only**，与其他 task 可并行。建议 plan 启动当天就提 PR 给 exam 团队，给他们 ≥3 工作日 review；同时跑 Task 2-8、10-11。
- Task 8 localstore 依赖 M1.2 是否已落 —— 若没落，plan 实施前需检查 PRD §里程碑并补做 M1.2，**或在 M2a 实施时一并并行做 M1.2 的 localstore 落地**（推荐后者，避免里程碑空挡）
- Task 11 console-lite 改动小，建议放在 Task 5（/v1/config 暴露 flag）之后立刻做，方便手测 visual feedback
- Commit 粒度：每 task 一个 commit；如果 task 内子步骤独立可拆，照 TDD 风格 RED→GREEN→commit
