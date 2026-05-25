# book-kb-rag M1.2-bridge Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development.

**Goal:** 把 PRD 故事 18-30 + 46-47 接通——让老师在 web 端用 `book-tutor-agent` 一句话生成题、确认入库、组卷；console-lite 提供 Standalone 题库/试卷/知识点管理 UI。

**Architecture:** 4 个新 worker `kb_*` 工具走 HTTP 调 ArkLoop api 新增的 13 个 kbapi 端点；端点内部按 `kb.integration_mode` 分流到 localstore（Standalone）或 examstore（Linked）；console-lite KB 详情页改 tabs 结构。

**Tech Stack:** Go 1.22 (worker + api), pgx v5, React + Tailwind (console-lite), 复用现有 `questionstore` / `papercompose` / `kb_search`。

---

## File Structure

**api — kbapi handlers + repo glue**
- Create: `src/services/api/internal/http/kbapi/handler_kp.go` — KP CRUD
- Create: `src/services/api/internal/http/kbapi/handler_question.go` — 题 CRUD + pool
- Create: `src/services/api/internal/http/kbapi/handler_paper.go` — 试卷 CRUD
- Modify: `src/services/api/internal/http/kbapi/register.go` — 挂 13 个新路由（注意：另一个 thread 在 m2b 改了这个文件做 exam-api-proxy；本 plan 与其在不同行追加；rebase 时手动合并）
- Modify: `src/services/api/internal/http/kbapi/deps.go` — handlerCtx 加 `questionStoreFactory` 或类似收口
- Create: `src/services/api/internal/http/kbapi/handler_kp_test.go` + `handler_question_test.go` + `handler_paper_test.go`

**worker — RAG executor**
- Move from untracked WIP: `src/services/worker/internal/tools/builtin/kb/rag_spec.go` — 4 工具 LlmSpec/AgentSpec（草稿已在 worktree；本 plan 取用）
- Create: `src/services/worker/internal/tools/builtin/kb/rag_executor.go` — 4 工具 handler
- Create: `src/services/worker/internal/tools/builtin/kb/rag_executor_test.go`
- Modify: `src/services/worker/internal/tools/builtin/kb/executor.go` — Execute() switch 加 4 case 分发到 rag_executor
- Modify: `src/services/worker/internal/tools/builtin/builtin.go` — 注册 4 个新工具

**personas**
- Modify: `src/personas/book-tutor-agent/persona.yaml` — description 去掉"出题/组卷功能将在后续版本提供"
- Modify: `src/personas/book-tutor-agent/prompt.md` — 完整工作流（参照 exam-builder-agent）

**console-lite**
- Modify: `src/apps/console-lite/src/pages/KnowledgeBaseDetailPage.tsx` — 重构成 tabs 容器
- Create: `src/apps/console-lite/src/pages/kb-tabs/DocumentsTab.tsx` — 原 KB detail 内容搬迁
- Create: `src/apps/console-lite/src/pages/kb-tabs/KnowledgePointsTab.tsx`
- Create: `src/apps/console-lite/src/pages/kb-tabs/QuestionsTab.tsx`
- Create: `src/apps/console-lite/src/pages/kb-tabs/PapersTab.tsx`
- Modify: `src/apps/console-lite/src/api/knowledge-bases.ts` — 加 KP / 题 / 卷 client functions
- Create: `src/apps/console-lite/src/components/QuestionEditModal.tsx` — 题编辑 modal

---

## Pre-Flight

- 已读 [PRD](../docs/prd/book-kb-rag.md) §User Stories 18-30 + 46-47 + §Implementation Decisions §F + §G
- 已读 [M1.2-bridge design](../docs/superpowers/specs/2026-05-24-book-kb-rag-m1.2-bridge-design.md)
- 已确认 `questionstore.QuestionStore` 接口 + `localstore` 真实现已在 M2a-prereq B 落地（`src/services/api/internal/questionstore/localstore`）
- 已确认 `papercompose.Compose` 已在 M2a-prereq C 落地
- 已确认 `examstore` 已在 M2a Task 9 落地（Linked 模式 SaveQuestions/SavePaper 走它）
- 已确认 `kb_search` 与 `kb_extract_toc` 是 worker `kb` 包既有工具（不动）
- **不要触碰**：另一个 thread 在 m2b 改的文件（`handler_kb.go`/`register.go`/`deps.go`/`exam_token_source.go`/`exam_scopes_lister.go`/console-lite api+modal+pages 等 exam-api-proxy 相关）；我方新增的 handler 用独立文件 + 在 register.go 末尾追加新行

---

### Task 1: API — KP 资源 CRUD（GET / POST / PATCH / DELETE）

**Files:**
- Create: `src/services/api/internal/http/kbapi/handler_kp.go`
- Create: `src/services/api/internal/http/kbapi/handler_kp_test.go`
- Modify: `src/services/api/internal/http/kbapi/register.go` — 末尾追加 4 路由

- [ ] **Step 1: 加路由 stub + handler 签名**

```go
// handler_kp.go
package kbapi

import (
    "encoding/json"
    nethttp "net/http"
    "strings"

    "arkloop/services/api/internal/data"
    "github.com/google/uuid"
)

func handleListKP(h *handlerCtx) nethttp.HandlerFunc { ... }
func handleCreateKP(h *handlerCtx) nethttp.HandlerFunc { ... }
func handlePatchKP(h *handlerCtx) nethttp.HandlerFunc { ... }
func handleDeleteKP(h *handlerCtx) nethttp.HandlerFunc { ... }
```

register.go 末尾追加：
```go
mux.Handle("GET /v1/knowledge-bases/{id}/knowledge-points", auth(handleListKP(h)))
mux.Handle("POST /v1/knowledge-bases/{id}/knowledge-points", auth(handleCreateKP(h)))
mux.Handle("PATCH /v1/knowledge-bases/{id}/knowledge-points/{kp_id}", auth(handlePatchKP(h)))
mux.Handle("DELETE /v1/knowledge-bases/{id}/knowledge-points/{kp_id}", auth(handleDeleteKP(h)))
```

- [ ] **Step 2: handlerCtx 加 KP repo 依赖**

`deps.go` 检查现有 `handlerCtx`：若没有 `kpStore *data.KBKnowledgePointsRepository`，加进去 + Deps 同步加 + NewHandlerCtx 传参。

- [ ] **Step 3: 实现 4 handlers**

每个 handler 走标准三段：
1. `actorFromCtx` + `loadAuthorizedKB`（继承 visibility 检查）
2. 解析请求体 / path param
3. 调 repo + 返 JSON

`handleCreateKP` body schema:
```go
type createKPReq struct {
    Name      string  `json:"name"`
    ParentID  *string `json:"parent_id,omitempty"`
    SortOrder int     `json:"sort_order,omitempty"`
}
```

校验：name trim 后非空；parent_id（若有）需是 UUID。

- [ ] **Step 4: handler 单测（mock store）**

`handler_kp_test.go` 覆盖：
- list 返回的 items 含 parent_id 字段
- create 重复名（同 kb_id）—— repo 不强制 unique 跳过，handler 不校验
- create 接受 parent_id=null
- patch 改 name
- delete 不存在的 kp 返 404
- 跨 KB 访问 KP 拒绝（kp.kb_id != path kb_id → 404）

- [ ] **Step 5: integration test**（真 PG）

复用 `kb_knowledge_points_repo_integration_test.go` 的 fixture 风格。覆盖：
- POST → GET 拉到刚建的 KP
- POST 含 parent_id → GET list 树形顺序正确

- [ ] **Step 6: 跑测 + commit**

```bash
go test ./src/services/api/internal/http/kbapi/ -run 'KP' -race -count=1
gofmt -l src/services/api/internal/http/kbapi/handler_kp.go src/services/api/internal/http/kbapi/handler_kp_test.go
go vet ./src/services/api/internal/http/kbapi/...
git add src/services/api/internal/http/kbapi/handler_kp.go src/services/api/internal/http/kbapi/handler_kp_test.go src/services/api/internal/http/kbapi/register.go src/services/api/internal/http/kbapi/deps.go
git commit -m "feat(kbapi): KP resource CRUD endpoints (M1.2-bridge Task 1)"
```

---

### Task 2: API — Questions CRUD + pool

**Files:**
- Create: `src/services/api/internal/http/kbapi/handler_question.go`
- Create: `src/services/api/internal/http/kbapi/handler_question_test.go`
- Modify: `register.go`、`deps.go`

- [ ] **Step 1: handler 签名 + 路由注册**

```go
func handleListQuestions(h *handlerCtx) nethttp.HandlerFunc { /* GET /v1/knowledge-bases/:id/questions */ }
func handleSaveQuestionsBatch(h *handlerCtx) nethttp.HandlerFunc { /* POST /v1/knowledge-bases/:id/questions/batch */ }
func handlePatchQuestion(h *handlerCtx) nethttp.HandlerFunc { /* PATCH /v1/knowledge-bases/:id/questions/:qid */ }
func handleDeleteQuestion(h *handlerCtx) nethttp.HandlerFunc { /* DELETE /v1/knowledge-bases/:id/questions/:qid */ }
func handleListQuestionsPool(h *handlerCtx) nethttp.HandlerFunc { /* GET /v1/knowledge-bases/:id/questions/pool */ }
```

5 路由追加到 register.go。

- [ ] **Step 2: handlerCtx 加 questionStoreFactory**

为简化分流，handlerCtx 加一个工厂方法：
```go
// deps.go
type questionStoreFactory func(ctx context.Context, kb *data.KnowledgeBase) (questionstore.QuestionStore, error)
```

构造时注入：`func(ctx, kb) { return questionstore.For(questionstore.KBDescriptor{ID: kb.ID.String(), IntegrationMode: kb.IntegrationMode, ExamScopeID: deref(kb.ExamScopeID)}, h.examIntegrationEnabled) }`

- [ ] **Step 3: 实现 handlers**

- `handleListQuestions`: filter ?knowledge_point_id&type&difficulty&limit&offset → store.ListReferenceQuestions
- `handleSaveQuestionsBatch`: body `{questions: [...]}` → store.SaveQuestions → 返 SaveResult JSON
- `handlePatchQuestion`: Linked 模式直接 409（"请到 exam 前台改"）；Standalone 直调 `data.KBQuestionsRepository.Update`
- `handleDeleteQuestion`: 同上；Standalone 走 repo.Delete
- `handleListQuestionsPool`: query `?kp_ids=a,b,c&type&difficulty` → store.ListQuestionsForPaperPool

Linked vs Standalone 分流统一在 handler 入口：从 KB 拿 mode，决定调 store 还是直接报 409。

- [ ] **Step 4: 单测**

`handler_question_test.go` 覆盖：
- list 返回 total + items
- saveBatch 部分失败：注入 fake store 返 `{created: [...], failed: [...]}`，handler 透传
- Linked 模式下 PATCH/DELETE → 409
- pool 接受多个 kp_ids（query 用逗号分隔）

- [ ] **Step 5: integration test**

真 PG + localstore，覆盖 saveBatch 部分失败、list 分页。

- [ ] **Step 6: commit**

```bash
git add src/services/api/internal/http/kbapi/handler_question.go src/services/api/internal/http/kbapi/handler_question_test.go src/services/api/internal/http/kbapi/register.go src/services/api/internal/http/kbapi/deps.go
git commit -m "feat(kbapi): question CRUD + batch save + pool endpoint (M1.2-bridge Task 2)"
```

---

### Task 3: API — Papers CRUD

**Files:** `handler_paper.go` + `handler_paper_test.go` + register.go

- [ ] **Step 1: 4 路由**

- `GET /v1/knowledge-bases/:id/papers`
- `POST /v1/knowledge-bases/:id/papers`
- `GET /v1/knowledge-bases/:id/papers/:pid`
- `DELETE /v1/knowledge-bases/:id/papers/:pid`

- [ ] **Step 2: handlers**

- list: store.ListByKB（需要 localstore 新增 ListPapers 方法 OR 直接调 `data.KBPapersRepository.ListByKB`；推荐后者，避免给 QuestionStore 接口加方法）
- create: body `{name, spec, seed, question_ids, markdown}` → store.SavePaper（已实现）+ 返 paper_id；Linked 模式调 examstore.SavePaper（走 POST /api/papers）
- get: 拉单个 paper + 解析 question_ids_json，**额外**返回 questions 数组（resolve 后端拉对应 questions）
- delete: 直 repo.Delete（Standalone 限定）

- [ ] **Step 3: 单测 + integration test**

覆盖 paper 创建后 GET 返回的 question_ids 顺序与输入一致；删 KB 触发 paper cascade。

- [ ] **Step 4: commit**

```bash
git commit -m "feat(kbapi): paper CRUD endpoints (M1.2-bridge Task 3)"
```

---

### Task 4: Worker — `kb_list_knowledge_points` + `kb_save_questions` 工具

**Files:**
- Move: `src/services/worker/internal/tools/builtin/kb/rag_spec.go`（untracked → 加入 git）
- Create: `src/services/worker/internal/tools/builtin/kb/rag_executor.go`
- Create: `src/services/worker/internal/tools/builtin/kb/rag_executor_test.go`
- Modify: `src/services/worker/internal/tools/builtin/kb/executor.go` — Execute() switch 加 case
- Modify: `src/services/worker/internal/tools/builtin/builtin.go` — 注册 spec

- [ ] **Step 1: 把现有 untracked rag_spec.go 加入 git**

```bash
git add src/services/worker/internal/tools/builtin/kb/rag_spec.go
```

（先不 commit；与本 task 其余 changes 一起 commit）

- [ ] **Step 2: rag_executor.go 骨架**

```go
package kb

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "time"

    "arkloop/services/worker/internal/tools"
    "github.com/google/uuid"
)

type RAGExecutor struct {
    httpClient *http.Client
    apiBaseURL string  // os.Getenv("ARKLOOP_API_BASE_URL"), fallback http://localhost:19001
    // kb_search 复用既有 Executor.searchHandler 即可（同包内访问）
    search *Executor
}

func NewRAGExecutor(search *Executor) *RAGExecutor {
    return &RAGExecutor{
        httpClient: &http.Client{Timeout: 30 * time.Second},
        apiBaseURL: firstNonEmpty(os.Getenv("ARKLOOP_API_BASE_URL"), "http://localhost:19001"),
        search:     search,
    }
}

// callAPI helper: do GET/POST with actor token (from ctx) → unmarshal into dst
func (e *RAGExecutor) callAPI(ctx context.Context, method, path string, body any, dst any) error { ... }
```

Token 来源：worker 执行 persona 时有 `actor.OIDCToken()`（M2a 同款）。如果没有，回退到 internal service token。

- [ ] **Step 3: `kb_list_knowledge_points` handler**

```go
func (e *RAGExecutor) listKnowledgePoints(ctx context.Context, args map[string]any, _ uuid.UUID, started time.Time) tools.ExecutionResult {
    kbID, _ := args["kb_id"].(string)
    if kbID == "" {
        return errResult(errorArgsInvalid, "kb_id is required", started)
    }
    var resp struct {
        Items []map[string]any `json:"items"`
    }
    if err := e.callAPI(ctx, "GET", "/v1/knowledge-bases/"+kbID+"/knowledge-points", nil, &resp); err != nil {
        return errResult("kb.upstream_error", err.Error(), started)
    }
    return ok(map[string]any{"items": resp.Items}, started)
}
```

- [ ] **Step 4: `kb_save_questions` handler**

```go
func (e *RAGExecutor) saveQuestions(ctx context.Context, args map[string]any, _ uuid.UUID, started time.Time) tools.ExecutionResult {
    kbID, _ := args["kb_id"].(string)
    questionsRaw, ok := args["questions"].([]any)
    if kbID == "" || !ok || len(questionsRaw) == 0 {
        return errResult(errorArgsInvalid, "kb_id and non-empty questions required", started)
    }
    var resp struct {
        Created []map[string]any `json:"created"`
        Failed  []map[string]any `json:"failed"`
    }
    body := map[string]any{"questions": questionsRaw}
    if err := e.callAPI(ctx, "POST", "/v1/knowledge-bases/"+kbID+"/questions/batch", body, &resp); err != nil {
        return errResult("kb.upstream_error", err.Error(), started)
    }
    return ok(map[string]any{
        "created":       resp.Created,
        "failed":        resp.Failed,
        "created_count": len(resp.Created),
        "failed_count":  len(resp.Failed),
    }, started)
}
```

- [ ] **Step 5: executor.go Execute() 加 case 分发到 RAGExecutor**

```go
case ToolNameListKnowledgePoints:
    return e.rag.listKnowledgePoints(ctx, args, ...)
case ToolNameSaveQuestions:
    return e.rag.saveQuestions(ctx, args, ...)
```

Constructor `NewToolExecutor` 内构造 `e.rag = NewRAGExecutor(e)`。

- [ ] **Step 6: 单测（httptest）**

注入 fake api server，断言：
- list KP：GET 命中正确 path
- saveQuestions：POST body 正确，response 透传 created/failed

- [ ] **Step 7: 注册到 builtin.go**

```go
// 在既有 kb 工具注册旁
register(kb.ListKnowledgePointsAgentSpec, kb.ListKnowledgePointsLlmSpec, kbExec.Execute)
register(kb.SaveQuestionsAgentSpec, kb.SaveQuestionsLlmSpec, kbExec.Execute)
```

- [ ] **Step 8: commit**

```bash
go test ./src/services/worker/internal/tools/builtin/kb/ -race -count=1
git add src/services/worker/internal/tools/builtin/kb/rag_spec.go src/services/worker/internal/tools/builtin/kb/rag_executor.go src/services/worker/internal/tools/builtin/kb/rag_executor_test.go src/services/worker/internal/tools/builtin/kb/executor.go src/services/worker/internal/tools/builtin/builtin.go
git commit -m "feat(worker): kb_list_knowledge_points + kb_save_questions tools (M1.2-bridge Task 4)"
```

---

### Task 5: Worker — `kb_draft_questions` 工具（含 kb_search 复用）

**Files:** `rag_executor.go` 扩展 + 测试

- [ ] **Step 1: handler**

```go
func (e *RAGExecutor) draftQuestions(ctx context.Context, args map[string]any, userID uuid.UUID, started time.Time) tools.ExecutionResult {
    kbID, _ := args["kb_id"].(string)
    kpID, _ := args["knowledge_point_id"].(string)
    if kbID == "" || kpID == "" {
        return errResult(errorArgsInvalid, "kb_id + knowledge_point_id required", started)
    }
    count := 5
    if c, ok := args["count"].(float64); ok && c > 0 && c <= 5 {
        count = int(c)
    }
    qType, _ := args["type"].(string)
    difficulty, _ := args["difficulty"].(string)

    // 1) retrieval — 默认用 KP 名做 query
    retrievalQuery, _ := args["retrieval_query"].(string)
    if retrievalQuery == "" {
        // 拉 KP name；HTTP GET
        var kpResp struct{ Name string }
        if err := e.callAPI(ctx, "GET", "/v1/knowledge-bases/"+kbID+"/knowledge-points/"+kpID, nil, &kpResp); err != nil {
            return errResult("kb.upstream_error", "fetch kp: "+err.Error(), started)
        }
        retrievalQuery = kpResp.Name
    }
    // 调既有 kb_search executor (in-process)
    hits, err := e.search.searchKBChunks(ctx, kbID, retrievalQuery, 8)
    if err != nil {
        return errResult("kb.search_failed", err.Error(), started)
    }

    // 2) reference questions
    var refResp struct {
        Items []map[string]any `json:"items"`
        Total int              `json:"total"`
    }
    refPath := fmt.Sprintf("/v1/knowledge-bases/%s/questions?knowledge_point_id=%s&limit=5", kbID, kpID)
    if qType != "" { refPath += "&type=" + qType }
    if difficulty != "" { refPath += "&difficulty=" + difficulty }
    if err := e.callAPI(ctx, "GET", refPath, nil, &refResp); err != nil {
        // not fatal — first time KP has no questions
        refResp.Items = []map[string]any{}
    }

    return ok(map[string]any{
        "action":              "draft_questions",
        "kb_id":               kbID,
        "knowledge_point_id":  kpID,
        "count":               count,
        "type":                qType,
        "difficulty":          difficulty,
        "retrieval_hits":      hits,
        "reference_questions": refResp.Items,
        "instruction": "Generate ≤" + fmt.Sprint(count) + " questions of type=" + qType + " difficulty=" + difficulty +
            " using the retrieved KB content. For choice questions, options must be ≥3. " +
            "Mark source_chunk_ids from retrieval_hits.chunk_id where the question content originates. " +
            "Each question must include stem / options / answer / explanation. " +
            "Do NOT plagiarize reference_questions verbatim; learn their style and depth.",
    }, started)
}
```

`e.search.searchKBChunks(ctx, kbID, q, k)` 是 Executor 的现有 helper 抽出（或新增 export）。

- [ ] **Step 2: 单测**

httptest mock api + fake search。断言：
- retrieval_query 为空时 fall back 到 KP name
- 返回的 instruction 含 count / type / difficulty
- reference_questions 为空时不报错

- [ ] **Step 3: 注册 + commit**

```bash
git add src/services/worker/internal/tools/builtin/kb/rag_executor.go src/services/worker/internal/tools/builtin/kb/rag_executor_test.go src/services/worker/internal/tools/builtin/kb/executor.go src/services/worker/internal/tools/builtin/builtin.go
git commit -m "feat(worker): kb_draft_questions tool with retrieval + reference (M1.2-bridge Task 5)"
```

---

### Task 6: Worker — `kb_compose_paper` 工具（含 markdown 拼装）

**Files:** `rag_executor.go` 扩展 + 测试 + `paper_markdown.go` helper

- [ ] **Step 1: handler**

```go
func (e *RAGExecutor) composePaper(ctx context.Context, args map[string]any, _ uuid.UUID, started time.Time) tools.ExecutionResult {
    kbID, _ := args["kb_id"].(string)
    name, _ := args["name"].(string)
    kpIDs := parseStringSlice(args["knowledge_point_ids"])
    total, _ := args["total_count"].(float64)
    if kbID == "" || name == "" || len(kpIDs) == 0 || total <= 0 {
        return errResult(errorArgsInvalid, "kb_id+name+knowledge_point_ids+total_count required", started)
    }
    // ...build spec from args
    var seed int64 = 42
    if s, ok := args["seed"].(float64); ok { seed = int64(s) }

    // 1) fetch pool
    poolPath := "/v1/knowledge-bases/" + kbID + "/questions/pool?kp_ids=" + strings.Join(kpIDs, ",")
    var poolResp struct {
        Items []map[string]any `json:"items"`
    }
    if err := e.callAPI(ctx, "GET", poolPath, nil, &poolResp); err != nil {
        return errResult("kb.upstream_error", "fetch pool: "+err.Error(), started)
    }

    // 2) papercompose
    spec := papercompose.PaperSpec{TotalCount: int(total), ...}
    pool := convertPoolToQuestions(poolResp.Items)
    paper, warnings, err := papercompose.Compose(spec, pool, seed)
    if err != nil {
        return errResult("kb.compose_failed", err.Error(), started)
    }
    if len(warnings) > 0 {
        return ok(map[string]any{
            "shortage_warnings": warnings,
            "pool_size":         len(pool),
        }, started)
    }

    // 3) build markdown
    markdown := renderPaperMarkdown(name, paper, pool)

    // 4) save
    saveBody := map[string]any{
        "name":         name,
        "spec":         spec,
        "seed":         seed,
        "question_ids": paper.QuestionIDs,
        "markdown":     markdown,
    }
    var saveResp struct{ ID string }
    if err := e.callAPI(ctx, "POST", "/v1/knowledge-bases/"+kbID+"/papers", saveBody, &saveResp); err != nil {
        return errResult("kb.upstream_error", "save paper: "+err.Error(), started)
    }
    return ok(map[string]any{
        "paper_id":     saveResp.ID,
        "markdown":     markdown,
        "question_count": len(paper.QuestionIDs),
    }, started)
}
```

- [ ] **Step 2: `paper_markdown.go` helper**

```go
package kb

func renderPaperMarkdown(name string, paper papercompose.Paper, pool []papercompose.Question) string { ... }
```

按题型分组渲染：
```markdown
# {name}
> 总题数：{n}

## 一、单选题（{n_sc} 道）
1. {stem}
   A. ...
   ...
   **答案**：{answer}
   **解析**：{explanation}

## 二、多选题 ...
```

- [ ] **Step 3: 单测**

- composePaper 注入 fake api + fake pool → 验证 papercompose 被调 + markdown 包含所有题 + saveBody.question_ids 顺序匹配
- 注入返 shortage 的 pool（少于 total）→ 验证 handler 返回 warnings 不写库

- [ ] **Step 4: 注册 + commit**

```bash
git commit -m "feat(worker): kb_compose_paper tool with markdown export (M1.2-bridge Task 6)"
```

---

### Task 7: book-tutor-agent prompt + persona.yaml

**Files:**
- Modify: `src/personas/book-tutor-agent/persona.yaml`
- Modify: `src/personas/book-tutor-agent/prompt.md`

- [ ] **Step 1: persona.yaml description 更新**

```yaml
description: 上传教材到知识库，用一句话生成题目和试卷；支持本地保存或写回 exam（Linked 模式）。
```

- [ ] **Step 2: prompt.md 重写（参照 `exam-builder-agent/prompt.md` 结构）**

5 个段落：
1. **角色** — 备课助手；操作老师的 KB；按教材内容生成题
2. **工作原则** — 教材为依据、≥3 options、模式透明（按 KB 决定写哪里）、小步 ≤5 道一批、失败可读
3. **启动流程** — 用 `list_knowledge_bases` 让老师选 KB；若是新 KB + 首次 ready 文档 → 提议 `kb_extract_toc`（Linked 模式才走 `exam_create_catalog_tree`）
4. **工作流：出题** —
   - 用 `kb_list_knowledge_points(kb_id)` 列 KP
   - 老师确定 KP 后调 `kb_draft_questions(kb_id, kp_id, count, type?, difficulty?)`
   - 工具返 `retrieval_hits + reference_questions + instruction` — agent loop 在下一轮按 instruction 生成 N 道题（输出 JSON 数组）
   - 逐题 `show_widget` 展示（含 source_chunk_id 引用 retrieval_hits 原文 200 字）
   - 老师 confirm → `kb_save_questions(kb_id, questions=[...])`
   - 部分失败逐条展示
5. **工作流：组卷** —
   - 老师说"组卷" → 用 `ask_user` 收集 spec
   - 调 `kb_compose_paper(kb_id, name, kp_ids, total_count, type_distribution, difficulty_distribution, seed?)`
   - 返 `shortage_warnings` 时让老师补题/放宽
   - 成功后 `show_widget` 展示概览 + markdown 下载链接
   - 在末尾告诉老师"已写入"（措辞按 KB 模式：Standalone "本地题库" / Linked "exam 题库"）

- [ ] **Step 3: 不需要测；commit**

```bash
git add src/personas/book-tutor-agent/persona.yaml src/personas/book-tutor-agent/prompt.md
git commit -m "feat(persona): book-tutor-agent full RAG workflow for draft/save/compose (M1.2-bridge Task 7)"
```

---

### Task 8: console-lite — KB 详情页 tabs 重构 + KP tab

**Files:**
- Modify: `src/apps/console-lite/src/pages/KnowledgeBaseDetailPage.tsx`
- Create: `src/apps/console-lite/src/pages/kb-tabs/DocumentsTab.tsx`
- Create: `src/apps/console-lite/src/pages/kb-tabs/KnowledgePointsTab.tsx`
- Modify: `src/apps/console-lite/src/api/knowledge-bases.ts`

- [ ] **Step 1: api client 加 KP CRUD**

```ts
// knowledge-bases.ts
export type KnowledgePoint = {
  id: string
  name: string
  parent_id: string | null
  depth: number
  sort_order: number
}

export async function listKnowledgePoints(kbId: string): Promise<KnowledgePoint[]> { ... }
export async function createKnowledgePoint(kbId: string, body: {name: string, parent_id?: string|null, sort_order?: number}): Promise<KnowledgePoint> { ... }
export async function patchKnowledgePoint(kbId: string, kpId: string, body: Partial<{name: string, sort_order: number}>): Promise<KnowledgePoint> { ... }
export async function deleteKnowledgePoint(kbId: string, kpId: string): Promise<void> { ... }
```

- [ ] **Step 2: KnowledgeBaseDetailPage 重构成 tabs 容器**

把现有 427 行内容搬到 `DocumentsTab.tsx`（接收 kb prop）。主页面变成：

```tsx
const tabs = [
  { key: 'documents', label: '📄 文档', component: DocumentsTab },
  { key: 'kps', label: '🏷️ 知识点', component: KnowledgePointsTab },
  { key: 'questions', label: '❓ 题库', component: QuestionsTab, standaloneOnly: true },
  { key: 'papers', label: '📝 试卷', component: PapersTab, standaloneOnly: true },
]
// Render tab strip + active tab content
// Linked KB 的 questions/papers tab 显示占位
```

- [ ] **Step 3: KnowledgePointsTab.tsx**

- 列表（带 parent_id 缩进树）
- 「新增」modal: name + 选 parent（dropdown 当前所有 KP） + sort_order
- 行内 inline edit name
- 删除（confirm）
- Linked 模式：disable 编辑按钮 + 显示"数据来自 exam 同步"提示

- [ ] **Step 4: type-check + lint + build**

```bash
cd src/apps/console-lite && pnpm type-check && pnpm lint && pnpm build
```

- [ ] **Step 5: commit**

```bash
git add src/apps/console-lite/src/
git commit -m "feat(console-lite): KB detail tabs + KP management (M1.2-bridge Task 8)"
```

---

### Task 9: console-lite — Questions tab

**Files:**
- Create: `src/apps/console-lite/src/pages/kb-tabs/QuestionsTab.tsx`
- Create: `src/apps/console-lite/src/components/QuestionEditModal.tsx`
- Modify: `src/apps/console-lite/src/api/knowledge-bases.ts`

- [ ] **Step 1: api client 加题目 CRUD**

```ts
export type KBQuestion = {
  id: string
  knowledge_point_id: string | null
  question_type: string
  difficulty: string
  stem: string
  options: Array<{key: string, text: string}>
  answer: string
  explanation: string
  quality_flag: string
  created_at: string
  updated_at: string
}

export type QuestionFilter = { knowledge_point_id?: string; question_type?: string; difficulty?: string; limit?: number; offset?: number }
export type QuestionListResp = { items: KBQuestion[]; total: number }

export async function listQuestions(kbId: string, filter: QuestionFilter): Promise<QuestionListResp> { ... }
export async function patchQuestion(kbId: string, qid: string, body: Partial<KBQuestion>): Promise<KBQuestion> { ... }
export async function deleteQuestion(kbId: string, qid: string): Promise<void> { ... }
```

- [ ] **Step 2: QuestionsTab.tsx**

- 筛选条：KP（dropdown） / 题型（dropdown） / 难度（dropdown）
- DataTable: stem 前 80 字 / 题型 badge / 难度 badge / KP 名 / 操作（查看 / 编辑 / 删除）
- 分页（20/50/100 条）

- [ ] **Step 3: QuestionEditModal.tsx**

- stem textarea
- options 动态 list（增/删行，至少 3 行限制；choice 题型）
- answer 输入（按题型动态：单选下拉 / 多选 checkbox / 填空文本）
- explanation textarea
- 保存 → PATCH /v1/knowledge-bases/:id/questions/:qid

- [ ] **Step 4: type-check + lint + build + commit**

```bash
git commit -m "feat(console-lite): Standalone question library tab + edit modal (M1.2-bridge Task 9)"
```

---

### Task 10: console-lite — Papers tab + markdown 导出

**Files:**
- Create: `src/apps/console-lite/src/pages/kb-tabs/PapersTab.tsx`
- Modify: `src/apps/console-lite/src/api/knowledge-bases.ts`

- [ ] **Step 1: api client 加试卷 CRUD**

```ts
export type KBPaper = { id: string; name: string; spec: object; seed: number; question_ids: string[]; created_at: string }
export type KBPaperDetail = KBPaper & { markdown: string; questions: KBQuestion[] }

export async function listPapers(kbId: string): Promise<KBPaper[]> { ... }
export async function getPaper(kbId: string, pid: string): Promise<KBPaperDetail> { ... }
export async function deletePaper(kbId: string, pid: string): Promise<void> { ... }
```

- [ ] **Step 2: PapersTab.tsx**

- DataTable: 试卷名 / 题数 / 创建时间 / 操作
- 详情 modal: spec 摘要 + 题列表（按 paper 顺序展示，题号 + stem 前 60 字）
- 「导出 markdown」按钮：直接生成 .md blob + 触发 `<a download>`
- 「删除」（confirm）

- [ ] **Step 3: type-check + lint + build + commit**

```bash
git commit -m "feat(console-lite): Standalone paper tab + markdown export (M1.2-bridge Task 10)"
```

---

### Task 11: Acceptance + PR

**Files:**
- Create: `tests/smoke/m1.2_bridge_smoke.sh`
- Create: `docs/superpowers/specs/2026-05-24-book-kb-rag-m1.2-bridge-acceptance.md`

- [ ] **Step 1: 跑全套 Go 测**

```bash
go test ./src/services/... -race -count=1
```

- [ ] **Step 2: 跑全套 console-lite**

```bash
cd src/apps/console-lite && pnpm test && pnpm type-check && pnpm build
```

- [ ] **Step 3: E2E smoke 脚本**

```bash
# tests/smoke/m1.2_bridge_smoke.sh
# Prereq: api + worker running, KB exists with PDF ready
KB=...
# 1. POST KP
KP=$(curl -X POST .../knowledge-bases/$KB/knowledge-points -d '{"name":"测试 KP"}' | jq -r .id)
# 2. POST batch questions (simulating LLM output)
curl -X POST .../questions/batch -d '{"questions":[{...}, {...}]}' 
# 3. GET pool + papers POST
# 4. assert paper.markdown contains all questions
```

- [ ] **Step 4: 写 acceptance log**

按 PRD 故事勾✅/❌：
| 故事 | 状态 | 证据 |
|---|---|---|
| 18 | ✅ | kb_draft_questions test PASS |
| 22 | ✅ | UI 题库 tab 截图 |
| 23 | ✅ | handler_question_test partial failure PASS |
| 25-30 | ✅ | kb_compose_paper test PASS |
| 46 | ✅ | console-lite QuestionsTab 可改可删 |
| 47 | ✅ | PapersTab markdown 下载 |

- [ ] **Step 5: push + PR**

```bash
git push -u origin book-kb-rag/m1.2-bridge
gh pr create --base main --title "M1.2-bridge: RAG draft/save/compose + Standalone UI (close PRD 18-30, 46-47)" --body "$(cat <<'EOF'
## Summary
Closes the 'last mile' of PRD Option 1: teachers can now generate questions / compose papers in the web via book-tutor-agent persona, AND browse / edit them in console-lite for Standalone KBs.

- 13 new kbapi endpoints: KP CRUD / questions CRUD + batch / questions pool / papers CRUD
- 4 new worker kb_* RAG tools: list_knowledge_points / draft_questions / save_questions / compose_paper
- book-tutor-agent prompt + description rewritten (no longer says '后续版本提供')
- console-lite KB detail tabs: 文档 / 知识点 / 题库 / 试卷
- markdown 导出 (PDF v2)
- Linked-mode KBs show 'manage in exam frontend' placeholder for question/paper tabs

## Test plan
- [x] go test ./src/services/... -race
- [x] pnpm test/type-check/build console-lite
- [x] tests/smoke/m1.2_bridge_smoke.sh PASS
- [x] PRD story acceptance in docs/superpowers/specs/2026-05-24-book-kb-rag-m1.2-bridge-acceptance.md
EOF
)"
```

---

## Self-Review Notes

**Spec coverage check (M1.2-bridge design):**
- ✅ A (kbapi endpoints): Tasks 1+2+3 cover 13 endpoints
- ✅ B (worker tools): Tasks 4+5+6 cover 4 tools
- ✅ C (persona prompt): Task 7
- ✅ D (console-lite tabs): Tasks 8+9+10
- ✅ E (acceptance): Task 11

**Placeholder scan**: each task has actual code blocks (handler skeletons + executor body + tsx components). No TBD / "similar to". Good.

**Type consistency**:
- `KBQuestion` in api ts client vs `data.KBQuestion` Go struct — same fields (id/stem/options/answer)
- `KnowledgePoint` ts type vs `data.KBKnowledgePoint` Go struct — same shape (id/name/parent_id/depth/sort_order)
- `KBPaper` ts: id/name/spec/seed/question_ids/markdown matches `data.KBPaper` repo
- examstore wrapping at the api layer is transparent to console-lite (it just sees JSON)
