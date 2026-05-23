# Design: book-kb-rag M2a — Linked KB 模式 + examstore 真实现 + Spike S2 合约冻结

> Status: ready-for-plan
> Owner: jzefan
> Created: 2026-05-23
> Companion specs: [PRD](../../prd/book-kb-rag.md) · [exam-api contract](../../integrations/exam-api.md) · [M1.1 design](./2026-05-23-book-kb-rag-m1.1-design.md) · [M2b design](./2026-05-23-book-kb-rag-m2b-design.md)
> Supersedes: [M2-prep design](./2026-05-23-book-kb-rag-m2-prep-design.md)（已并入本文档）

## Context

M2a 是把 ArkLoop 从"独立可用的 standalone KB"打通到"与 exam 系统真实联动"的里程碑。这一步要完成 6 件事：

1. **PRD 故事 34 / §集成模式开关**：部署级开关 `ARKLOOP_EXAM_INTEGRATION_ENABLED` + KB 级 `integration_mode` 字段 + KB visibility 字段 — 配置维度全部就位
2. **PRD §F / §F'**：`questionstore` 包从骨架升级为含 `examstore` 真实现 + `localstore`（与 M1.2 共享）— 两端实现都有，模式开关由 `For()` 工厂分流
3. **PRD §对 exam 系统的依赖 / Spike S2**：4 个 exam 端点合约最终冻结，新增 `pattern_tag` 字段贯穿（为 M2b 让路）
4. **PRD 故事 31 + 44 + 45**：Linked KB 在 console-lite 的新建表单出现"绑定 exam 课程"选项；KB 列表显示模式徽章；新书 TOC widget 流挂到 book-tutor-agent
5. **PRD 故事 6 + 40**：M1.0 的现实可用性补漏（mime 白名单 + blob 引用计数 + delete 时清 blob）
6. **PRD §对 exam 的依赖（schema 改动）**：exam 侧 `questions.pattern_tag TEXT NULL` 列 + 4 端点带该字段（合约 PR 与 M2b 共用，但在 M2a 启动前必须冻结）

本文档**吸收并取代**了原 `2026-05-23-book-kb-rag-m2-prep-design.md`，把 visibility / mime / blob / 开关骨架四项与 M2 真接入合并成一次 milestone。

## 范围摘要

预计 7-10 工作日；与 M2b 设计并行写但实施串行（M2b 等 M2a `examstore` PR merge 后启动）。

| 块 | 来源 | 说明 |
| --- | --- | --- |
| Migration 00194 `kb_visibility` | M2-prep | `knowledge_bases.visibility` 字段 + check 约束 |
| Migration 00195 `kb_integration_mode_defaults` | 新增 | 给 `integration_mode` 列加 check 约束 + 给已有 KB backfill `standalone`（M1.0 时该列已经在 schema 里但无 check 约束、无默认行为兜底） |
| kbapi: visibility 过滤 + mime 白名单 + blob ref-count delete | M2-prep | 三件事一遍重构 |
| kbapi: 创建 KB 接受 `integration_mode` + `exam_course_id` | 新增 | 仅在部署级开关开启时允许 `exam`；否则强制 `standalone` |
| 部署级开关 env 校验 + persona loader 过滤 + worker exam tool gate | M2-prep | 三层一致 |
| `shared/questionstore/` 包：interface + `localstore` 真实现 + `examstore` 真实现 | M2-prep + 新增 | 骨架来自 M2-prep，本 milestone 把两端都填好 |
| `examstore` 真实现：HTTP client + 4 端点 + 部分失败 + 重试 + OIDC token | 新增 | 直接调 exam REST，不走 worker tool 层 |
| Spike S2：exam-api.md 7 个 open question 全部 resolve + 新增 `pattern_tag` 字段 | 新增 | 合约 PR 与 exam 团队对齐后 freeze |
| console-lite KB 新建表单：模式单选 + Linked 时选 exam 课程 + 可见性 | M2-prep + 新增 | 模式选项受部署级开关控制 |
| book-tutor-agent persona：Linked 模式 TOC widget 流 | 新增 | 用户故事 31 |
| 测试：visibility 过滤、mime 拒绝、blob ref count、开关 off 行为、examstore httptest、`For()` 分流 | M2-prep + 新增 | examstore 用 `httptest.NewServer` 桩 exam，覆盖 4 端点的成功/部分失败/5xx 重试/401 |

## 数据模型

### Migration 00194 `kb_visibility.sql`（来自 M2-prep，未变动）

```sql
-- +goose Up
-- +goose StatementBegin
ALTER TABLE knowledge_bases
    ADD COLUMN visibility TEXT NOT NULL DEFAULT 'workspace_member';

ALTER TABLE knowledge_bases
    ADD CONSTRAINT knowledge_bases_visibility_check
    CHECK (visibility IN ('workspace_member', 'private'));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE knowledge_bases
    DROP CONSTRAINT IF EXISTS knowledge_bases_visibility_check;
ALTER TABLE knowledge_bases
    DROP COLUMN IF EXISTS visibility;
-- +goose StatementEnd
```

### Migration 00195 `kb_integration_mode_defaults.sql`（新增）

M1.0 的 KB schema 已经有 `integration_mode` / `exam_course_id` 列，但当时**没加 check 约束**（PRD `integration_mode` 改名前的迁移版本不严格），现在补：

```sql
-- +goose Up
-- +goose StatementBegin
-- backfill 已有数据为 standalone（M1.0 时只有这一种模式）
UPDATE knowledge_bases
SET integration_mode = 'standalone'
WHERE integration_mode IS NULL OR integration_mode = '';

ALTER TABLE knowledge_bases
    ALTER COLUMN integration_mode SET DEFAULT 'standalone',
    ALTER COLUMN integration_mode SET NOT NULL;

ALTER TABLE knowledge_bases
    ADD CONSTRAINT knowledge_bases_integration_mode_check
    CHECK (integration_mode IN ('standalone', 'exam'));

-- 当 mode='exam' 时 exam_course_id 必须非空（仅 Postgres，与 SQLite 兼容此约束写法见 dialect.go）
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
    DROP CONSTRAINT IF EXISTS knowledge_bases_integration_mode_check;
ALTER TABLE knowledge_bases
    ALTER COLUMN integration_mode DROP NOT NULL,
    ALTER COLUMN integration_mode DROP DEFAULT;
-- +goose StatementEnd
```

**SQLite 兼容**：dev 环境跑 SQLite 时，CHECK 约束语法相同（SQLite 支持）。若 dev DB 已有 NULL 数据需 backfill。

### exam 侧 schema（**由 exam 团队执行**，作为 M2a 的前置条件）

```sql
ALTER TABLE questions ADD COLUMN pattern_tag TEXT NULL;
-- 索引可选；M2a 不强制
```

ArkLoop 侧不需要任何 schema 改动持有 `pattern_tag` —— Standalone KB 在 M2a 暂不接 `pattern_tag`（这是 M2b 的范围；Standalone 模式与 Option 2 互斥）。

## kbapi 改动

### 创建 KB（POST /v1/knowledge-bases）

```go
type createKBReq struct {
    Name            string  `json:"name"`
    WorkspaceRef    string  `json:"workspace_ref"`
    Description     string  `json:"description"`
    Visibility      string  `json:"visibility"`        // "" -> workspace_member
    IntegrationMode string  `json:"integration_mode"`  // "" -> standalone
    ExamCourseID    *string `json:"exam_course_id"`    // required iff mode=exam
}
```

**校验链**（按顺序，失败立刻返 400）：

1. `Visibility ∈ {"", "workspace_member", "private"}`
2. `IntegrationMode ∈ {"", "standalone", "exam"}`
3. 若 `IntegrationMode == "exam"`：
   - 部署级开关必须开（`cfg.ExamIntegrationEnabled == true`），否则 400 `kb.integration_disabled` + 中文 msg `"本部署未启用 exam 集成，请联系管理员或选择独立模式"`
   - `ExamCourseID` 非空且 trim 后非空
   - **可选但推荐**：调 examstore 探活验证 course_id 真实存在（`examstore.GetCourse(courseID)`，超时 5s；探活失败返 400 `kb.exam_course_not_found` + 错误详情）。这是用户故事 48 "明确提示老师将写入 exam（课程：X）"的依赖。
4. 写库时 `ExamCourseID` 走 `sql.NullString`

### Visibility 过滤 / mime 白名单 / blob ref-count delete

完全沿用 M2-prep 设计（详见已 superseded 的 m2-prep doc §kbapi 改动部分）。本文档不重复列代码，三点关键决策保留：
- visibility 过滤 helper `ensureKBVisible(kb, actor)` 写在 `kbapi/auth.go`
- mime 白名单 7 种：`text/plain` `text/markdown` `application/pdf` `application/vnd.openxmlformats-officedocument.wordprocessingml.document` `image/png` `image/jpeg` `image/webp`，拒绝时 415 `kb.unsupported_mime` + 中文 msg
- blob 删除走 `CountByBlobSHA256` SQL 引用计数，最后一份引用消失时 `WorkspaceBlobAdapter.DeleteBlob`

## 部署级开关

### 环境变量（沿用 M2-prep）

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `ARKLOOP_EXAM_INTEGRATION_ENABLED` | `false` | 部署级总开关 |
| `EXAM_BASE_URL` | （沿用既有） | exam 后端地址；开关开启时必填 |

### 启动校验 + 三层守门

- **api 启动期**：`app/config.go` 校验 + 启动失败硬退出（详见 m2-prep doc 同节）
- **api persona loader**：`exam-agent` 装载时按开关切 `user_selectable`
- **worker exam.* tool 注册**：开关 false 时显式跳过 `exam.NewExecutor` 注册，日志 `"exam_* tools disabled by ARKLOOP_EXAM_INTEGRATION_ENABLED=false"`
- **kbapi 创建 KB**：开关 false 时拒绝 `integration_mode='exam'`（见上文校验链）
- **console-lite**：通过 `GET /v1/config` 暴露的 `exam_integration_enabled` 布尔字段（**新增**：M2a 给 platform config endpoint 加这一字段；console-lite 在 KB 创建表单按它决定是否显示 "Linked to exam" 选项）

### console-lite 配置接入

```ts
// console-lite: hooks/usePlatformConfig.ts (假定既有；若无则新建)
export type PlatformConfig = {
  exam_integration_enabled: boolean
  // ... 既有字段
}
```

api `/v1/config` 返回结构里加 `exam_integration_enabled: cfg.ExamIntegrationEnabled`。前端用它驱动单选展示。

## questionstore 包：M2-prep 骨架 → M2a 真实现

### 接口与类型（沿用 M2-prep 骨架，本节定型）

```go
package questionstore

import "context"

type Question struct {
    ID               string
    KnowledgePointID string
    Type             string  // "single_choice" | "multi_choice" | "fill_in" | "short_answer" | "essay"
    Difficulty       string  // "easy" | "medium" | "hard"
    Stem             string
    Options          []QuestionOption // nil for non-choice
    Answer           string
    Explanation      string
    SourceSnippets   []SourceSnippet
    PatternTag       string  // M2a 起加入；Standalone 永远为 ""
    CreatedBySource  string  // "ai" | "human"
    CreatedAt        time.Time
}

type QuestionOption struct {
    Key  string
    Text string
}

type SourceSnippet struct {
    ChunkRef    string    // "kb_id/document_id/ordinal"
    Snippet     string    // 200-500 字
    IngestTime  time.Time
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
    PatternTag       string // M2b 的 exam_build_questions 强校验时填；M1.2 standalone 留空
    CreatedBySource  string
}

type SaveResult struct {
    Created []SavedQuestion
    Failed  []SaveFailure
}

type SavedQuestion struct {
    Index int    // 对应输入 drafts 下标
    ID    string // 后端分配的 ID
}

type SaveFailure struct {
    Index        int
    Draft        QuestionDraft
    ErrorCode    string // "knowledge_point_not_found" | "validation_error" | "pattern_tag_mismatch" | ...
    ErrorMessage string
}

type KnowledgePoint struct {
    ID       string
    Name     string
    ParentID *string
    Depth    int
    SortOrder int
}

type PaperSpec struct {
    TotalCount                  int
    TypeDistribution            map[string]int
    DifficultyDistribution      map[string]int   // 也可表达比例，调用方自己换算
    KnowledgePointDistribution  map[string]int
    AllowDuplicateKP            bool
    ExcludeQuestionIDs          []string
}

type PaperID string

type ListFilter struct {
    Type        string
    Difficulty  string
    PatternTag  string  // M2b 的 exam_list_seed_questions 用；M2a 透传到 examstore
    Limit       int
    Offset      int
}

type QuestionStore interface {
    ListReferenceQuestions(ctx context.Context, kpID string, filter ListFilter) ([]Question, int, error) // items, total, err
    SaveQuestions(ctx context.Context, drafts []QuestionDraft) (SaveResult, error)
    ListKnowledgePoints(ctx context.Context, scope Scope) ([]KnowledgePoint, error)
    SavePaper(ctx context.Context, name string, scope Scope, spec PaperSpec, questionIDs []string, seed int64) (PaperID, error)
    ListQuestionsForPaperPool(ctx context.Context, kpIDs []string, filter ListFilter) ([]Question, error)
}

type Scope struct {
    // 对 examstore: CourseID; 对 localstore: KBID
    CourseID string
    KBID     string
}

type KBDescriptor struct {
    ID              string
    IntegrationMode string  // "standalone" | "exam"
    ExamCourseID    string
}

// For 工厂：按 KB.integration_mode 返回对应实现
func For(ctx context.Context, kb KBDescriptor, deps Dependencies) (QuestionStore, error)

type Dependencies struct {
    DB              *sql.DB          // for localstore
    ExamClient      *examstore.Client // nil if integration disabled
    OIDCTokenSource func(ctx context.Context) (string, error)
}

// 错误
var (
    ErrIntegrationDisabled = errors.New("questionstore: exam integration disabled")
    ErrUnsupportedMode     = errors.New("questionstore: unsupported integration_mode")
)
```

**`PatternTag` 字段在 M2a 引入但 Standalone 永远不用**（PRD 决定 Option 2 仅 Linked）。`examstore` 透传到 exam（GET `?pattern_tag=` filter + POST body 携带），`localstore` 忽略该字段（写库时丢弃；M1.2 已经实现的 `kb_questions` schema 不动）。

### `For()` 分流逻辑

```go
func For(ctx context.Context, kb KBDescriptor, deps Dependencies) (QuestionStore, error) {
    switch kb.IntegrationMode {
    case "standalone":
        return localstore.New(deps.DB), nil
    case "exam":
        if deps.ExamClient == nil {
            return nil, ErrIntegrationDisabled
        }
        return examstore.New(deps.ExamClient, deps.OIDCTokenSource), nil
    default:
        return nil, ErrUnsupportedMode
    }
}
```

调用方（worker 工具 `kb_save_questions` / `kb_compose_paper` 等）拿到 `kb_id` 后先 `KBLookup(kbID) -> KBDescriptor`，再 `questionstore.For(ctx, kb, deps)`，再调接口。**模式判定每次工具调用都重做**（KB 元数据不缓存）—— 简单，避免分布式失效问题。

### `localstore` 实现（M1.2 已落，M2a 不动）

`localstore.SaveQuestions` 已经覆盖部分失败（duplicate stem detection 等）；M2a 只需保证接口签名与上面一致（M1.2 实施时可能略有差异，统一一次）。

### `examstore` 真实现（**本 milestone 核心**）

```go
package examstore

type Client struct {
    httpClient    *http.Client
    baseURL       string  // ARKLOOP_EXAM_BASE_URL
    apiVersion    string  // "1"
    maxConcurrent int     // 4 per teacher session
    retryPolicy   RetryPolicy
}

type RetryPolicy struct {
    MaxAttempts int           // 3
    BaseDelay   time.Duration // 250ms
    MaxDelay    time.Duration // 5s
}

func New(baseURL string) *Client { ... }

// 4 端点实现：
func (c *Client) ListKnowledgePoints(ctx, token, courseID, limit, offset) (...)
func (c *Client) ListQuestions(ctx, token, knowledgePointID string, filter ListFilter) (...)
func (c *Client) CreateQuestionsBatch(ctx, token, drafts []QuestionDraft) (SaveResult, error)
func (c *Client) CreatePaper(ctx, token, name, courseID, spec PaperSpec, questionIDs []string, seed int64) (PaperID, error)

// HTTP 调用通用流程：
// 1. build request: 设 Authorization Bearer <token>, X-ArkLoop-API-Version: 1, Content-Type: application/json
// 2. concurrency limiter (per teacher session) — 用 channel-based semaphore
// 3. retry: 5xx + 网络错误重试，4xx 不重试
// 4. 解析响应：
//    - 200/201 happy path
//    - 4xx: 解析 error_code/error_message，包成 typed error 返回
//    - 5xx after retries: 包成 ServerError 返回
//    - 401/403: 包成 AuthError，persona 提示老师重新登录或联系管理员
// 5. log: 一行 structured log {endpoint, status, duration_ms, attempt, err}
```

`examstore.Store` 是接口对调用方的 facade，包一层 `Client`：

```go
type Store struct {
    client      *Client
    tokenSource func(ctx context.Context) (string, error) // 拿 OIDC token
    courseID    string  // 从 KB.ExamCourseID 注入
}

func (s *Store) ListReferenceQuestions(ctx context.Context, kpID string, filter ListFilter) ([]Question, int, error) {
    token, err := s.tokenSource(ctx)
    if err != nil {
        return nil, 0, fmt.Errorf("examstore: get oidc token: %w", err)
    }
    return s.client.ListQuestions(ctx, token, kpID, filter)
}
// ... 其余 3 个方法类似
```

### OIDC token 来源

worker 内部用 `actor.OIDCToken()`（执行 persona 工作流时已有 actor context，与现有 exam-agent 走的路径一致）。`tokenSource` 在 `worker/internal/tools/builtin/kb/` 工具组装时注入。

### 部分失败的语义对齐

exam-api.md Endpoint 3 已经定 `{created: [{index, id}], failed: [{index, error_code, error_message}]}` —— `examstore.CreateQuestionsBatch` 解析时按 `index` 还原到 `SaveResult{Created, Failed}`，**保证调用方能 1:1 映射到输入 drafts**。M2b 的 `pattern_tag_mismatch` 在 exam 侧不会发生（exam 不校验 pattern_tag 与种子是否一致），由 ArkLoop 的 `exam_build_questions` 工具在调 `examstore.SaveQuestions` 之前完成校验（详见 M2b 设计）。

## Spike S2：exam-api.md 合约冻结

[exam-api.md](../../integrations/exam-api.md) 当前 status `draft` + 7 个 open question。M2a 启动前必须全部 resolve，否则 examstore 实现停摆。

### 7 个 open question 的目标决定

| # | 议题 | ArkLoop 提案 | 备注 |
| --- | --- | --- | --- |
| 1 | source_snippets 持久化路径 | **选 option A**：exam 加 `source_snippets JSONB` 列，UI 可选展示 | 与新增 `pattern_tag` 同一份 schema PR |
| 2 | knowledge-points name 单列 vs code+display | **选单列 `name`** | 简化；exam 若有 code 字段单独透传不影响 |
| 3 | questions 表 global vs per-course | **接受任一**；ArkLoop 仅靠 `knowledge_point_id` 定位 | 实现侧不可见 |
| 4 | exam 校验 options count vs type | **请确认**；ArkLoop 在 draft prompt 内 mirror 同规则 | 影响 `validation_error` 命中率 |
| 5 | paper template 预先要求 | **请确认**：若需要，ArkLoop 跳过 Endpoint 4，由老师手建 | 影响 PRD 故事 30 措辞 |
| 6 | OIDC scope 名称 | **接受 ArkLoop 提案**：`exam:read:knowledge-points` / `exam:read:questions` / `exam:write:questions` / `exam:write:papers` | exam 可改名，ArkLoop 配置同步 |
| 7 | rate-limit policy | **接受任一**；ArkLoop 已自限并发 4 + 5xx 重试 | exam 可加严，ArkLoop 不主动改 |

### 新增第 8 个 open question（M2a + M2b 共用）

| # | 议题 | ArkLoop 提案 |
| --- | --- | --- |
| 8 | `pattern_tag` 字段：questions 表新增 `pattern_tag TEXT NULL` 列；GET `/api/questions` 接受 `pattern_tag` query；POST `/api/questions/batch` 接受 per-item `pattern_tag` | 详见 M2b 设计 §pattern_tag 全链路 |

**冻结流程**：

1. ArkLoop 把 exam-api.md 更新到把上面 8 项写成具体的"ArkLoop 提案 + 等 exam 确认"措辞，提 PR 给 exam 团队
2. exam 团队对每项打勾 / 修改提案 / 拒绝（拒绝时 ArkLoop 走 fallback，例如 `source_snippets` 走 option B 本地表）
3. 双方在 PR 上对应每个 open question 留 resolution log（带日期）
4. 全部 resolve → exam-api.md 头部 status 改为 `frozen-v1` → ArkLoop 才开始 M2a examstore 实现

**这一步是阻塞 M2a 实施的 PR-level gate**，不是 design-level gate；本设计稿可以先 ready-for-plan，plan 第一个任务就是 "drive exam-api.md to frozen-v1"。

## Linked KB 端到端流（用户故事 31 / 44 / 45 / 48）

### console-lite 新建 KB 表单

```tsx
<Form>
  <Field label="名称" required>
    <Input value={name} onChange={...} />
  </Field>
  <Field label="描述">
    <Textarea value={description} onChange={...} />
  </Field>
  <Field label="可见性">
    <Radio.Group value={visibility}>
      <Radio value="workspace_member">工作区全员可见</Radio>
      <Radio value="private">仅自己可见</Radio>
    </Radio.Group>
  </Field>

  {platformConfig.exam_integration_enabled && (
    <Field label="集成模式" required>
      <Radio.Group value={integrationMode}>
        <Radio value="standalone">独立模式（题目存在 ArkLoop）</Radio>
        <Radio value="exam">绑定 exam 课程（题目写回 exam）</Radio>
      </Radio.Group>
    </Field>
  )}

  {integrationMode === "exam" && (
    <Field label="exam 课程" required>
      <CourseSelect
        value={examCourseID}
        onChange={setExamCourseID}
        fetchCourses={() => api.listExamCourses()}
      />
      <Hint>该 KB 生成的题目会写入此课程；KB 创建后无法切换模式。</Hint>
    </Field>
  )}
</Form>
```

**`CourseSelect` 实现**：调一个新的 api endpoint `GET /v1/exam/courses`（仅在开关 on 时注册），由 api 后端代理调 examstore `ListCourses`。这避免前端持有 OIDC token 直调 exam。

> ⚠ exam-api.md 当前 4 端点未含"列课程"。M2a Spike S2 时一并加入第 9 个端点 `GET /api/courses`（或确认 exam 已经有等价端点）。

### KB 列表徽章

KB 卡片右上角：
- `visibility='private'` → 灰底"私有"徽章
- `integration_mode='exam'` → 蓝底"已绑定 exam: 《课程名》"徽章（课程名走 examstore 缓存或前端调 `/v1/exam/courses` 解析）
- 两者可同时显示

### book-tutor-agent TOC widget 流（用户故事 31）

Linked KB + 上传首份文档完成后，persona 主动判断：
1. 文档 `parse_meta_json` 含 `toc` 字段且 nodes ≥ 5 → 调 `kb_extract_toc(document_id)` 获取标准化 tree
2. `show_widget` 让老师编辑 / 删节点 / 确认
3. 老师 confirm → persona 调既有 `exam_create_catalog_tree` 写入 exam（**注意**：不是新工具，是 exam-agent 现有 4 工具之一）

> `kb_extract_toc` 是**新工具**：从 KB 文档的解析中间结果抽 TOC 树。M2a 范围内新增。
>
> `exam_create_catalog_tree` 是**既有工具**（exam-agent 用），在 worker registry 中开关 on 时已注册；book-tutor-agent 直接 reuse 即可，**不需要新工具**。

#### 新工具 `kb_extract_toc(document_id)` spec

```json
{
  "name": "kb_extract_toc",
  "description": "从 KB 中已 ready 的文档抽取目录结构（TOC tree），用于把书的目录建到 exam 知识点树。返回与 exam_create_catalog_tree 同样形状的 tree（course → chapters → sections）。",
  "input_schema": {
    "type": "object",
    "properties": {
      "document_id": {"type": "string"}
    },
    "required": ["document_id"]
  }
}
```

实现：worker 调 api `GET /v1/knowledge-bases/:kb_id/documents/:doc_id/toc` —— api 后端从 `kb_documents.parse_meta_json` 取已经存在的 toc 字段（M1.1 PDF 解析已经把 outline 持久化），按 exam 接口需要的 tree 形状归一化输出。

book-tutor-agent prompt.md 在 Linked 模式下追加：

```
# 如果当前 KB 是 integration_mode='exam' 且这是该 KB 第一份 ready 文档：
1. 调 kb_extract_toc(document_id=<刚 ready 的>); 若 nodes>=5 →
2. show_widget 给老师确认目录；
3. 老师 confirm → exam_create_catalog_tree(course_id=<kb.exam_course_id>, tree=<确认后>);
4. 老师拒绝 / 没拒绝时 nodes<5 → 跳过，引导老师手动加知识点标签；
```

Standalone 模式不走这条流。

### "明确提示老师将写入 exam"（用户故事 48）

book-tutor-agent 在 `kb_save_questions` 调用前先 `show_widget` 渲染一段：

```
将把 N 道题写入 exam：
- 课程：《大学物理（上）》
- 知识点：《第三章 光的干涉》
- 题型：A2 / single_choice
确认写入？[确认] [取消]
```

老师 confirm 后才真正调 `kb_save_questions` → `examstore.SaveQuestions`。**Standalone 模式的 widget 文案对应改为"写入本地题库"**，由 persona 根据 `kb.integration_mode` 切换。

## kb_search 路径不变

PRD 已经写明：`kb_search` 是 ArkLoop 本地 KB 向量检索，**与模式无关**。Linked KB 同样把 KB 内容存在本地 pgvector，只是出题写回去 exam。worker `kb_search` 工具不需要任何改动。

## 测试

| 块 | 类型 | 覆盖点 |
| --- | --- | --- |
| 00194 / 00195 应用 | integration | `\d knowledge_bases` 校验列 + 约束；既有 KB 默认 visibility=`workspace_member`、mode=`standalone` |
| visibility 过滤 | integration | private KB 仅 creator 可见；同 workspace 其他 actor 列表/get/delete 都返 404 |
| mime 白名单 | unit + handler | 7 种 mime 接受 + `application/zip` 415 拒绝 |
| blob ref count delete | integration | 同 sha 上传两次 → 删第一份 blob 还在 → 删第二份 blob 删除 |
| 创建 KB 校验 | handler | 开关 off + mode=exam → 400 `kb.integration_disabled`；mode=exam + 无 exam_course_id → 400；mode=exam + course_id 不存在 → 400 `kb.exam_course_not_found` |
| 启动期 env 校验 | startup | `ARKLOOP_EXAM_INTEGRATION_ENABLED=true` + `EXAM_BASE_URL=""` → 启动报错退出 |
| exam-agent persona 过滤 | integration | 开关 off → `/v1/personas` 不含 exam-agent |
| worker tool gate | log | 开关 off → worker 启动日志含 "exam_* tools disabled" |
| questionstore.For 分流 | unit | mode=standalone → localstore；mode=exam + 开关 on → examstore；mode=exam + 开关 off → `ErrIntegrationDisabled` |
| examstore HTTP 行为 | unit (httptest) | 4 端点 happy path、partial failure、5xx 重试 3 次后冒泡、401 包成 AuthError、并发限流 ≤4 |
| examstore 部分失败 mapping | unit | exam 返 `{created:[{index:0}], failed:[{index:1}]}` → SaveResult.Created/Failed 1:1 还原 |
| kb_extract_toc | unit | parse_meta_json 中 toc 字段存在/不存在两 case；返 tree 形状与 exam_create_catalog_tree 输入兼容 |
| Linked KB E2E smoke | tests/smoke | 开关 on 起服务 → 创建 mode=exam KB → 上传文档 → ready → kb_extract_toc → mock exam 返成功 → 模拟出题流 → mock examstore.SaveQuestions 成功；端到端串通即可，不要求质量断言 |
| console-lite | UI 截图 + type-check | KB 新建表单按 platformConfig.exam_integration_enabled 显示/隐藏模式单选；列表徽章显示正确 |

## 关键技术决策

1. **examstore 直接调 exam REST 不走 worker tool 层**：tool 层是给 LLM 的，examstore 是工程间调用；维持 PRD §F' 的最终决定不变
2. **OIDC token 由 actor context 提供，不引入服务 token**：与既有 exam-agent 工具同路径
3. **`For()` 工厂每次工具调用都重做**：KB 元数据查询便宜（一行 SELECT），缓存收益小，避免 KB 元数据失效问题
4. **`pattern_tag` 在 M2a 引入但 Standalone 永远不用**：Standalone 模式 + Option 2 互斥（Option 2 仅 Linked），所以 `localstore` 忽略该字段简单干净
5. **`kb_extract_toc` 而不是 reuse `exam_recognize_catalog_image`**：后者输入是图片，前者输入是已解析 doc 的 outline；语义不同，独立工具更清晰；最终 tree 形状对齐让 exam_create_catalog_tree 复用
6. **课程列表走 api 代理而非前端直调 exam**：避免前端持 OIDC token；同时 api 可以加 cache（M2a 不实现，留 followup）
7. **Spike S2 是阻塞 M2a 实施而非阻塞设计**：本文档 ready-for-plan 即可；plan 第一任务就是合约 freeze；examstore 真实现等合约冻结
8. **Linked KB 失败不静默降级**：examstore 失败时 persona 明确告诉老师"写入 exam 失败"，不偷偷写本地表（避免老师以为已写入）
9. **schema 改动放 exam 侧不放 ArkLoop 侧**：`pattern_tag` 是 exam 的题目元数据，权威源在 exam；ArkLoop 仅是 in-flight 字段透传

## 风险与缓解

| 风险 | 缓解 |
| --- | --- |
| exam 团队 Spike S2 反复迭代 → M2a 实施推迟 | 设计可继续；plan 第 1 任务专门盯合约；实现 PR 卡在 freeze 后才 merge |
| `pattern_tag` exam 侧不愿加列 | M2b 退化为 ArkLoop 端持久化 pattern_tag → 在 `kb_question_patterns(exam_question_id, pattern_tag)` 影子表；不阻塞 M2a，但 M2b 设计需有 fallback 章节 |
| Linked KB 创建时 exam_course_id 探活慢/挂 | 探活超时 5s；超时降级为不验证（只校验非空），错误的 course_id 后续出题时 exam 报 404，用户故事 23 的部分失败路径会兜底 |
| examstore 并发限流 4 太低/太高 | 当配置项暴露但默认 4；rate-limit 一致 by exam-api.md 约定，后续按观测调 |
| schema 变化（exam questions 加 pattern_tag）破坏既有 exam-agent 流程 | `pattern_tag NULL` 列向后兼容；现有 4 工具不读不写该字段；exam 侧 UI 不展示即可零影响 |
| 老 KB（M1.0 创建的）integration_mode 字段为空 | migration 00195 backfill `'standalone'`；Down 段保留可逆 |
| visibility / integration_mode 两个新字段在 worker `kb_chunks_repo` 是否需要过滤 | 不需要：worker 走 actor context；persona 已经能拿到 KB 才能调工具；可见性在 api 层就过滤过 |
| examstore httptest 桩与真实 exam 行为漂移 | tests/smoke 加一个 `ARKLOOP_RUN_EXAM_INTEGRATION_TESTS=1` 的 staging 真调入口（默认 skip） |

## 不在 M2a 范围

- M2b 的 Option 2（exam-builder-agent persona、命题技能、4 个新 exam_* 工具、pattern_tag 强校验逻辑）—— 见 [M2b 设计](./2026-05-23-book-kb-rag-m2b-design.md)
- 真正调动 exam 真实部署的 staging 验证（仅自测时 mock httptest，staging E2E 留给后续）
- exam 课程列表的 cache 层
- orphan blob 清理 cron
- KB.integration_mode 在创建后可切换 —— PRD 已决定不可切换（故事 49）
- `kb_documents.visibility` per-document 可见
- Standalone KB 引入 `pattern_tag`

## 下一步

1. 走 `superpowers:writing-plans` 拆 plan（预计 8-10 tasks，命名 `2026-05-23-book-kb-rag-m2a.md`）
2. Plan 第 1 task：**driver exam-api.md 到 frozen-v1**（含第 8 个 open question pattern_tag + 第 9 个 GET /api/courses），与 exam 团队沟通；此 task 完成才能跑后续 PR
3. Plan 第 2 task：migration 00194 + 00195 + repo 加 visibility / integration_mode 字段 / 约束
4. Plan 第 3 task：kbapi visibility 过滤 + mime 白名单 + blob ref-count delete（吃掉 m2-prep 实施面）
5. Plan 第 4 task：kbapi 创建 KB 加 integration_mode 校验链 + `/v1/exam/courses` 代理端点 + `/v1/config` 暴露 `exam_integration_enabled`
6. Plan 第 5 task：deploy 开关 env 校验 + persona loader + worker exam tool gate
7. Plan 第 6 task：questionstore 接口 freeze + localstore 对齐（如 M1.2 实施时签名漂移，本任务一次统一）
8. Plan 第 7 task：examstore 真实现（client + 4 endpoints + retry + concurrency limiter + token source + httptest 全套单测）
9. Plan 第 8 task：kb_extract_toc 工具 + book-tutor-agent prompt.md 加 Linked TOC 流
10. Plan 第 9 task：console-lite 新建 KB 模式单选 + 课程下拉 + 列表徽章
11. Plan 第 10 task：acceptance 跑全套 + Linked KB E2E smoke
