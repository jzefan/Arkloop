# Design: book-kb-rag M2 prep + KB schema/blob/visibility patches

> Status: **SUPERSEDED by [M2a design](./2026-05-23-book-kb-rag-m2a-design.md)** — 本文档原始范围已被 M2a 完整吞掉（PRD §里程碑 line 351 "M2a 统一吞掉原 M2-prep 范围"）。保留作为历史草稿。
> Owner: jzefan
> Created: 2026-05-23
> Companion specs: [PRD](../../prd/book-kb-rag.md) · [M1 decomposition](./2026-05-21-book-kb-rag-m1-decomposition-design.md) · [exam-api contract](../../integrations/exam-api.md)

## Context

M1.0 端到端验证后浮现 4 项 PRD 已点名、当前实现里仍缺的小补丁，单独跑一个 milestone 不值，但合在一起对"M2 启动前的现实可用性"很重要：

1. **PRD 故事 34**：`knowledge_bases.visibility` 字段（工作区全员可见 vs 仅自己可见）从未落地
2. **PRD 故事 6**：删除文档时清 workspace blob — 当前只 cascade 删 `kb_chunks`，blob 留库占盘
3. **PRD 故事 40**：上传 size/type 显式上限 + 友好错误 — size 有了（10MB），mime 白名单缺，不支持类型当前要等 worker 解析失败才报错
4. **PRD §"集成模式开关"**：M2 部署级开关 `ARKLOOP_EXAM_INTEGRATION_ENABLED` 从未实现 — 当前 `exam-agent` persona 是无条件 `user_selectable: true`，没有 exam 后端的部署也会出现在 selector

第 4 项是 M2 的预装基础设施：开关、env 校验、QuestionStore 接口骨架 — 真 `examstore` 实现留给 M2，但**入口和契约现在定下来**，避免 M2 启动时反复返工 PRD 文字。

第 1-3 项是 M1.0 的现实可用性补漏。

四项内部依赖弱、合并成本低（共享 1 个 migration、共享 kbapi 一遍重构、共享 1 次 push），所以本 spec 打包做一个 plan。

## 范围摘要

3-4 工作日交付：

- Migration `00194_kb_visibility.sql`：`knowledge_bases.visibility TEXT NOT NULL DEFAULT 'workspace_member'`，check 约束 `IN ('workspace_member', 'private')`
- kbapi 写路径：visibility 创建时可选；读路径：list/get 根据 visibility + actor 是否 creator 过滤
- Upload handler：mime 白名单（`text/plain`, `text/markdown`, `application/pdf`, `application/vnd.openxmlformats-officedocument.wordprocessingml.document`, `image/png`, `image/jpeg`, `image/webp`），返回 `kb.unsupported_mime` 错码 + 中文 msg
- Delete document handler：count 同 sha256 引用，最后一份引用消失时 `blob.Delete`
- 部署级开关 `ARKLOOP_EXAM_INTEGRATION_ENABLED` + 复用既有 `EXAM_BASE_URL`：
  - api 启动期校验：开关 = true 但 EXAM_BASE_URL 空 → 直接退出
  - api persona loader：开关 = false 时 `exam-agent` 不进 selector
  - worker `exam_*` 工具：开关 = false 时跳过注册（沿用现有 NewClient 失败即跳过逻辑，增加一层显式 env 优先级校验）
- 新增 `src/services/shared/questionstore/` 包：QuestionStore 接口 + `localstore` 真实现（M1.2 时复用）+ `examstore` 留 `NotImplementedError` stub（M2 时填）
- console-lite 新建 KB 表单：可见性下拉（"工作区全员可见" / "仅自己"）
- 测试：visibility 过滤的 integration test、mime 拒绝的 handler test、blob 引用计数 delete 的 integration test、开关 off 时 exam-agent 不出现的 startup test

## 数据模型

**新增迁移 `00194_kb_visibility.sql`**：

```sql
-- +goose Up
-- +goose StatementBegin
ALTER TABLE knowledge_bases
    ADD COLUMN visibility TEXT NOT NULL DEFAULT 'workspace_member';

ALTER TABLE knowledge_bases
    ADD CONSTRAINT knowledge_bases_visibility_check
    CHECK (visibility IN ('workspace_member', 'private'));

-- 既有数据默认全部可见（DEFAULT 已经搞定）。
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE knowledge_bases
    DROP CONSTRAINT IF EXISTS knowledge_bases_visibility_check;
ALTER TABLE knowledge_bases
    DROP COLUMN IF EXISTS visibility;
-- +goose StatementEnd
```

`KnowledgeBase` Go struct + `KnowledgeBasesRepo.Create` / Get / List 全部加 `Visibility string` 字段。

**无其他 schema 变更**。

## kbapi 改动

### 创建 KB（POST /v1/knowledge-bases）

请求体新增可选 `visibility` 字段（缺省 `workspace_member`）：

```go
type createKBReq struct {
    Name         string `json:"name"`
    WorkspaceRef string `json:"workspace_ref"`
    Description  string `json:"description"`
    Visibility   string `json:"visibility"` // "" -> workspace_member
}
```

handler 校验 visibility ∈ `{"", "workspace_member", "private"}`，写入 `KBCreate.Visibility`。

### 列表与 Get 的可见性过滤

`handleListKB` 与 `loadAuthorizedKB`：在已通过 workspace membership check 之后**追加**一层 visibility filter：

```go
if kb.Visibility == "private" && kb.CreatedBy != nil && *kb.CreatedBy != actor.UserID {
    // private KB only visible to creator
    writeErr(w, 404, "kb.not_found", "kb not found")
    return nil, false
}
```

写到 `kbapi/auth.go` 作 helper `ensureKBVisible(kb, actor)`，复用在 Get / Delete / 所有 document 路径上。

### 上传 mime 白名单

`handleUploadDoc`（`handler_doc.go`）：sniff 完 mime 后立刻 whitelist check：

```go
var allowedMimes = map[string]struct{}{
    "text/plain": {},
    "text/markdown": {},
    "application/pdf": {},
    "application/vnd.openxmlformats-officedocument.wordprocessingml.document": {},
    "image/png": {},
    "image/jpeg": {},
    "image/webp": {},
}

base := strings.ToLower(strings.TrimSpace(strings.SplitN(detectedMime, ";", 2)[0]))
if _, ok := allowedMimes[base]; !ok {
    writeErr(w, 415, "kb.unsupported_mime", fmt.Sprintf("不支持的文件类型：%s（支持 .txt/.md/.pdf/.docx/.png/.jpg/.webp）", base))
    return
}
```

Tests：表驱动单元测试覆盖每种 mime 接受 + 1 个拒绝（如 `application/zip`）。

### 删除文档的 blob 引用计数

`kb_documents_repo.go` 新增 `CountByBlobSHA256(ctx, sha string) (int, error)`：

```go
SELECT COUNT(*) FROM kb_documents WHERE blob_sha256 = $1
```

`handleDeleteDoc`：

```go
sha := doc.BlobSHA256
if err := h.docStore.Delete(ctx, doc.ID); err != nil { ... }
remaining, _ := h.docStore.CountByBlobSHA256(ctx, sha)
if remaining == 0 {
    // last reference — drop blob
    if err := h.blob.DeleteBlob(ctx, kb.WorkspaceRef, sha); err != nil {
        // log but don't fail the DELETE; blob orphan is recoverable
        logger.Warn("kb.blob_orphan", "sha", sha, "err", err)
    }
}
```

`blobWriter` 接口扩 `DeleteBlob(ctx, workspaceRef, sha string) error`；`WorkspaceBlobAdapter.DeleteBlob` 包装 `objectstore.Store.Delete`。

**测试**（integration）：先上传同一 sha 文件两次得两个 doc → 删第一个，blob 还在 → 删第二个，blob 没了。

## 部署级开关

### 环境变量

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `ARKLOOP_EXAM_INTEGRATION_ENABLED` | `false` | 部署级总开关；M2 examstore + Linked KB UI + exam-agent persona 是否启用 |
| `EXAM_BASE_URL` | （沿用） | exam 后端地址；开关开启时必填，否则启动报错 |

### api 启动期校验

`app/config.go` 加：

```go
const (
    examIntegrationEnabledEnv = "ARKLOOP_EXAM_INTEGRATION_ENABLED"
    examBaseURLEnv            = "EXAM_BASE_URL"
)

type Config struct {
    ...
    ExamIntegrationEnabled bool
    ExamBaseURL            string
}

// Load:
cfg.ExamIntegrationEnabled = parseBoolEnv(examIntegrationEnabledEnv, false)
cfg.ExamBaseURL = strings.TrimSpace(os.Getenv(examBaseURLEnv))

// Validate:
if cfg.ExamIntegrationEnabled && cfg.ExamBaseURL == "" {
    return errors.New("ARKLOOP_EXAM_INTEGRATION_ENABLED=true but EXAM_BASE_URL is empty")
}
```

### persona loader 过滤

`api/internal/personas/loader.go`（查实际位置）：装载 `exam-agent` 时，若 `cfg.ExamIntegrationEnabled == false`，把 `persona.UserSelectable = false`（或干脆从注册表跳过）。这样 console-lite selector 不会出现该 persona，老师不会迷惑。

`book-tutor-agent` 不受影响（两种部署都可用，已在 M1.0 落地）。

### worker exam.* tool 双层守门

现有 `worker/internal/tools/builtin/builtin.go:114` 已经 `exam.NewClient()` 失败时跳过 examExec — 这是 client-level 的隐式 gate。补一层 env-level 显式 gate：

```go
if os.Getenv("ARKLOOP_EXAM_INTEGRATION_ENABLED") != "true" {
    examExec = nil
    logger.Info("exam_* tools disabled by ARKLOOP_EXAM_INTEGRATION_ENABLED=false")
} else if examClient, err := exam.NewClient(); err == nil {
    examExec = exam.NewExecutor(examClient)
}
```

这样部署级开关同时关掉 api persona + worker tools，行为一致。

## QuestionStore 包骨架（M2 预装）

新增 `src/services/shared/questionstore/`：

```go
package questionstore

import "context"

type Question struct { /* id, stem, options, answer, knowledge_point_id, ... */ }
type QuestionDraft struct { /* same minus id */ }
type SaveResult struct { Created []Question; Failed []SaveFailure }
type SaveFailure struct { Draft QuestionDraft; Reason string }
type KnowledgePoint struct { /* id, name, parent_id */ }
type PaperSpec struct { /* total_count, type_distribution, ... */ }
type PaperID string

type QuestionStore interface {
    ListReferenceQuestions(ctx context.Context, knowledgePointID string, qType string, difficulty string, limit int) ([]Question, error)
    SaveQuestions(ctx context.Context, drafts []QuestionDraft) (SaveResult, error)
    ListKnowledgePoints(ctx context.Context, scope string) ([]KnowledgePoint, error)
    SavePaper(ctx context.Context, spec PaperSpec, questionIDs []string, seed int64, name string) (PaperID, error)
    ListQuestionsForPaperPool(ctx context.Context, knowledgePointIDs []string, qType string, difficulty string) ([]Question, error)
}

// For 按 KB.integration_mode 返回对应实现：
//   "standalone" → localstore（M1.2 实现）
//   "exam"       → examstore（M2 实现）
func For(ctx context.Context, kb KBDescriptor) (QuestionStore, error)
```

**M1.2 阶段填 `localstore`**（PRD §F），**M2 阶段填 `examstore`**（PRD §M2）。本 milestone 只交骨架 + `examstore` stub return `ErrNotImplemented`，让 PRD/`exam-api.md` 里冻结的接口对应到代码里有桩。

`For()` 在 KB.integration_mode == "exam" 但 `ARKLOOP_EXAM_INTEGRATION_ENABLED == false` 时返回 `ErrIntegrationDisabled` — 防止配置错乱时 RAG 工具沉默走错路。

## console-lite 改动

`CreateKBModal.tsx`：新增可见性单选（默认 `workspace_member`）

```tsx
<Field label="可见性">
  <Radio value="workspace_member">工作区全员可见</Radio>
  <Radio value="private">仅自己</Radio>
</Field>
```

`KnowledgeBasesPage.tsx`：KB 卡片右上角加 visibility 徽章（"共享" / "私有"）。

`KnowledgeBaseDetailPage.tsx`：document 上传失败时显示 `kb.unsupported_mime` 错误（已经显示后端 error_message，无需改）。

**Linked KB 选项不在本 milestone 暴露**（要等 M2 examstore 实现 + Linked UI 单独一轮设计）。新建 KB 表单的 `integration_mode` 暂时只显示 standalone。

## 验收

| Check | 验证方式 |
| --- | --- |
| `00194_kb_visibility.sql` 应用，`knowledge_bases.visibility` 默认 `workspace_member` | `\d knowledge_bases` |
| 私有 KB 创建者外的同 workspace 用户列表/get/delete 都返 404 | integration test |
| 上传 `.zip` 立刻返 415 `kb.unsupported_mime` + 中文 msg | handler test + curl 实测 |
| 同 sha 上传两次：删第一份 blob 还在；删第二份 blob 被 Delete | integration test |
| `ARKLOOP_EXAM_INTEGRATION_ENABLED=true` + `EXAM_BASE_URL=""` 时 api 启动报错退出 | startup test |
| `ARKLOOP_EXAM_INTEGRATION_ENABLED=false` 时 `exam-agent` 不在 `/v1/personas` 列表 | integration test |
| 同 false 时 worker 启动日志含 "exam_* tools disabled" | log assertion |
| `questionstore.For(ctx, kbWithModeExam)` 在开关 false 时返 `ErrIntegrationDisabled` | unit test |
| console-lite 创建 KB 表单含可见性下拉，KB 列表显示徽章 | UI 截图 |
| 现有 console-lite type-check / lint / build 全绿 | pnpm 跑 |
| 既有 KB 测试不受 visibility filter 退化（默认 workspace_member 应保持 M1.0 行为） | full test suite 跑 |

## 关键技术决策

1. **visibility 二值不是 enum 表**：当前需求只有 "工作区可见" 和 "仅自己" 两种；用 TEXT + check 约束足够；以后需要"项目组可见"等再单独加 column 或拆出独立 ACL 表
2. **blob 引用计数走 SQL `COUNT`** 而非维护单独引用表：sha 重复在 KB 场景罕见（PRD 故事 35-36 是 API 批量上传场景才会触发），多查一次 COUNT 成本可忽略，省一张表
3. **mime 白名单写在 kbapi 而非 bookparser**：上传时立刻拒绝比等 worker 解析失败友好得多；bookparser 作为下游仍可以 raise ErrUnsupportedMime 兜底
4. **questionstore 接口骨架但留 stub**：避免 PRD/`exam-api.md` 的接口约定"无代码对应"而漂移；M1.2 填 localstore 时第一件事就是替掉 stub
5. **部署级开关用 env 而非 platform config**：开关影响的是 persona 注册、tool 注册、env 校验 — 这些在启动期决定；改 platform config 需要重启服务才生效，那不如直接走 env
6. **persona 过滤改 `user_selectable=false` 而非物理移除**：保留 persona 配置文件本身，便于 toggle；selector 只查 `user_selectable=true` 的，自然过滤

## 风险与缓解

| 风险 | 缓解 |
| --- | --- |
| 已有 KB 历史数据 visibility 字段空 | 迁移用 `DEFAULT 'workspace_member'` 自动填，不需要 backfill 脚本 |
| 删 blob 失败导致 document 删除卡住 | 改为 log + 继续（孤儿 blob 后续清理脚本扫，不阻塞 UI） |
| sha 引用计数有竞态（两个用户同时上传同 sha → 同时删时可能少删） | KB delete + document delete 都在同 transaction 内查 COUNT；接受最坏 case = 孤儿 blob（不会误删活的） |
| exam-agent persona 配置文件改了 user_selectable 后被覆盖 | persona loader 加载完成后**运行时**改 `user_selectable`，不写回文件；下次进程重启重新按开关计算 |
| examstore stub 误被调用导致 panic | 接口返 `ErrNotImplemented` 而非 panic；调用方 wrap 成业务错误返回前端 |
| 新 visibility 字段对 worker 数据层（kb_search 等）是否需要可见性过滤？ | worker `kb_search` 是 persona 内部调用，已经走 actor 鉴权链路；persona 自己拿到 KB 才能搜 — 不需要再过滤 |

## 不在本 milestone 范围

- 真 examstore 实现（M2）
- Linked KB 在 console-lite 的 UI 选项（M2）
- 更细的 ACL（per-document visibility、project group 可见性）
- arphan blob 清理 cron job
- `kb_documents.visibility` per-document 可见（PRD 只到 KB 级别）

## 不在本设计范围

- M1.1 PDF / DOCX / image 解析（单独 spec）
- M1.2 RAG 出题（单独 spec）
- M1.3 组卷（单独 spec）

## 下一步

1. 走 `superpowers:writing-plans` 拆 plan（预计 6-8 tasks，命名 `2026-05-23-book-kb-rag-m2-prep.md`）
2. Plan 第 1 task：migration 00194 + repo 加 visibility 字段
3. Plan 第 2 task：kbapi visibility 过滤 + mime 白名单 + blob ref-count delete
4. Plan 第 3 task：deploy 开关 + persona loader + worker exam tool gate
5. Plan 第 4 task：questionstore 骨架 + stub
6. Plan 第 5 task：console-lite UI（visibility 下拉 + 徽章）
7. Plan 第 6 task：acceptance 跑全套
