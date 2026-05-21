# Design: book-kb-rag M1 分解 + M1.0 子里程碑

> Status: ready-for-plan
> Owner: jzefan
> Created: 2026-05-21
> Companion specs: [M0 design](./2026-05-21-book-kb-rag-design.md), [PRD](../../prd/book-kb-rag.md)

## Context

PRD `docs/prd/book-kb-rag.md` 把整套"书→KB→RAG 出题/组卷"拆成 M0/M1/M2。M0 已经定义并交付实现（chunker + Doubao embedder + pgvector + 2 个 debug endpoint）。本文档：

1. 把 M1（"Standalone 模式可独立交付"那一大块）分解为 4 个可独立 demo 的子里程碑
2. 把第一个子里程碑 M1.0（"KB 基础设施跑 .txt 端到端"）写成可执行 spec，供 writing-plans 拆任务

设计依据：

- **YAGNI**：M1.0 不实现 RAG / 组卷 / PDF；那是后续子里程碑的事
- **演示驱动**：每个 sub-milestone 终态必须能演示给一个真实老师看，他能说出"我看到 X"
- **S1 spike 与 M1.0 并行**：M1.0 故意挑 .txt 输入，与 jzefan 在 sandbox 评估 PyMuPDF 质量的工作没有交互依赖

## M1 子里程碑分解

| 子里程碑 | 主题 | 上游依赖 | 演示形态 | 预算 |
|---------|------|---------|---------|------|
| **M1.0** | KB 基础设施 + .txt 摄入 + 检索演示 | M0 完成 | 老师建 KB → 上传 .txt → ready → persona 搜内容 | 5-7 工作日 |
| **M1.1** | PDF / DOCX 解析（DocumentParser 真实实现） | M1.0 + Spike S1 结论 | 上传 PDF → 看到 heading_path 和 chunk_type=image/table 命中 | 5-7 工作日 |
| **M1.2** | RAG 出题入库 | M1.1 + Doubao 推理模型可用 | 老师对 persona 说"为'光的干涉'出 3 道单选题"→ 草稿展示 → 老师确认 → 入 `kb_questions`；console-lite 可浏览/编辑题库 | 7-10 工作日 |
| **M1.3** | 组卷 + 导出 | M1.2 + PaperComposer 纯函数 | 老师组一张完整期中卷 → markdown/PDF 导出 → 副本下载 | 5-7 工作日 |

> **M2（exam 集成）独立于 M1.3**。M1 全部 standalone，QuestionStore 抽象在 M1.2 阶段同时引入两个接口实现（`localstore` 真用、`examstore` 留空 stub），M2 阶段只补 examstore 真实现 + UI 上的 Linked 选项。

## M1.0 详细 Scope

### 范围摘要

5-7 个工作日交付：完整 KB schema、KB CRUD REST API、Workspace 鉴权、`.txt` 摄入流水线（走 worker queue）、DocumentParser 接口（仅 `.txt` 实现）、`kb_search` worker 工具、`book-tutor-agent` persona shell、console-lite "知识库"管理页 MVP。

### 数据模型（api 服务，goose 迁移 00193）

```sql
-- 00193_kb_full_schema.sql

-- 用 ALTER + DROP 重建 kb_chunks，把 M0 的 TEXT 列改成 UUID 外键
DROP TABLE kb_chunks;

CREATE TABLE knowledge_bases (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_ref TEXT NOT NULL,
    account_id    UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    integration_mode TEXT NOT NULL DEFAULT 'standalone',  -- standalone | exam (M2)
    exam_course_id   TEXT,                                 -- nullable, M2-only
    created_by    UUID REFERENCES users(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_ref, name)
);
CREATE INDEX knowledge_bases_workspace_idx ON knowledge_bases(workspace_ref);
CREATE INDEX knowledge_bases_account_idx ON knowledge_bases(account_id);

CREATE TABLE kb_documents (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id             UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    original_filename TEXT NOT NULL,
    mime_type         TEXT NOT NULL,
    blob_sha256       TEXT NOT NULL,    -- workspaces/{workspace_ref}/blobs/{sha256}
    size_bytes        BIGINT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'queued',
        -- queued | parsing | chunking | embedding | upserting | ready | failed
    error_message     TEXT NOT NULL DEFAULT '',
    parse_meta_json   JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by        UUID REFERENCES users(id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX kb_documents_kb_idx ON kb_documents(kb_id);
CREATE INDEX kb_documents_status_idx ON kb_documents(status);

CREATE TABLE kb_chunks (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id         UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    document_id   UUID NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
    ordinal       INTEGER NOT NULL,
    heading_path  TEXT[] NOT NULL DEFAULT '{}',     -- M1.0 always empty
    chunk_type    TEXT NOT NULL DEFAULT 'paragraph', -- M1.1+: image/table/formula
    text          TEXT NOT NULL,
    token_count   INTEGER NOT NULL,
    embedding     vector(<DOUBAO_DIM>) NOT NULL,     -- same N as M0 migration 00192
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (kb_id, document_id, ordinal)
);
CREATE INDEX kb_chunks_kb_idx ON kb_chunks(kb_id);
CREATE INDEX kb_chunks_document_idx ON kb_chunks(document_id);
CREATE INDEX kb_chunks_embedding_hnsw_idx ON kb_chunks USING hnsw (embedding vector_cosine_ops);

-- Schema 提前建好，M1.0 不写入；M1.2 才有数据。
CREATE TABLE kb_knowledge_points (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kb_id     UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    name      TEXT NOT NULL,
    parent_id UUID REFERENCES kb_knowledge_points(id) ON DELETE CASCADE,
    exam_knowledge_point_id TEXT,  -- M2-only
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE kb_document_knowledge_points (
    kb_id              UUID NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    document_id        UUID NOT NULL REFERENCES kb_documents(id) ON DELETE CASCADE,
    knowledge_point_id UUID NOT NULL REFERENCES kb_knowledge_points(id) ON DELETE CASCADE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (document_id, knowledge_point_id)
);
```

迁移策略：**直接 DROP `kb_chunks` 重建**。M0 数据是验证性的，正式上线公告"M0 阶段上传的 .txt 失效，请重新上传"。

### REST API（新增包 `api/internal/http/kbapi`）

所有路由前缀 `/v1/knowledge-bases`，强制 OIDC/session 认证 + Workspace 成员验证：

| 方法 | 路径 | 用途 | 响应 |
|------|------|------|------|
| POST | `/v1/knowledge-bases` | 创建 KB（在请求的当前 workspace 下） | `{id, name, workspace_ref, created_at}` |
| GET  | `/v1/knowledge-bases` | 列当前 workspace 下的 KB | `{items: [{id, name, document_count, created_at}, ...]}` |
| GET  | `/v1/knowledge-bases/:id` | 单 KB 详情 | `{id, name, description, document_count, ...}` |
| DELETE | `/v1/knowledge-bases/:id` | 删 KB（联级 documents/chunks/blob） | 204 |
| POST | `/v1/knowledge-bases/:id/documents` | 上传文档（multipart，`.txt` only in M1.0） | `{document_id, job_id}` |
| GET  | `/v1/knowledge-bases/:id/documents` | 列 KB 下文档 | `{items: [{id, original_filename, status, ...}, ...]}` |
| GET  | `/v1/knowledge-bases/:id/documents/:doc_id` | 单文档详情（含 status） | `{id, original_filename, status, error_message, chunk_count, ...}` |
| DELETE | `/v1/knowledge-bases/:id/documents/:doc_id` | 删文档（联级 chunks + blob） | 204 |
| GET  | `/v1/knowledge-bases/:id/search?q=&k=` | 检索（M1.0 调试用） | `{hits: [{document_ref, ordinal, text, score, ...}, ...]}` |

**鉴权**：沿用现有 workspace 路由模式（参考 `api/internal/http/accountapi/register.go:67` 的 `workspaceFilesEntry`、`api/internal/http/catalogapi/register.go:94` 的 `workspaceSkillsEntry`）：

1. `httpkit` 解析 `Actor`（auth + account membership，错误码 `auth.no_account_membership`）—— 已有逻辑直接复用
2. 在 KB 路由 handler 内 JOIN `account_memberships` 验证 `actor.AccountID` 是 `knowledge_bases.account_id` 的成员，且 `workspace_ref` 在该 account 下可见（用 `WorkspaceRegistriesRepo` 验证 workspace 归属当前 account）
3. 失败统一返回 403 + `WriteError(w, ..., "auth.no_workspace_access", ...)`

具体 helper 在 plan 阶段以"接 `workspaceFilesEntry` 的依赖注入风格"实现，不另起鉴权框架。

**M0 `/v1/_debug/kb/*` 路由**：M1.0 上线前一次性删除。

### Worker 摄入流水线

**Job 类型常量**：`KBIngestJobType = "kb_ingest"`，与 `RunExecuteJobType` 等并列在 `worker/internal/queue/protocol.go` 添加。

**Job payload schema**:
```json
{
  "kb_id": "uuid",
  "document_id": "uuid",
  "workspace_ref": "string",
  "blob_sha256": "string",
  "mime_type": "string",
  "original_filename": "string",
  "version": 1
}
```

**Enqueue**：api 服务收到 `POST .../documents` 后，先把文件存到 workspace blob（`shared/workspaceblob`），写 `kb_documents` 行 `status=queued`，调 `JobQueue.EnqueueRun(accountID, uuid.Nil, traceID, KBIngestJobType, payload, nil)`，返回 `{document_id, job_id}`。

**Processor**（worker 服务）：新增 `worker/internal/jobs/kbingest/processor.go`，注册到 worker dispatcher：

1. `status=parsing`：从 blob 拉文件 → `bookparser.Parse(reader, mime_type)` → 拿到 `ParsedDoc`
2. `status=chunking`：`bookchunker.Chunk(parsedDoc, opts)` → 拿到 `[]Chunk`
3. `status=embedding`：`embedder.Embed(ctx, texts)`（worker 直接构造 Doubao Embedder，复用 `shared/embedding`）
4. `status=upserting`：`worker/internal/data/kb_chunks_repo.go`（直连 PgBouncer，沿用 `runs_repo` 模式）`.Upsert(...)`
5. `status=ready`：更新 `kb_documents.status='ready'`、`updated_at=now()`、`parse_meta_json={chunk_count, duration_ms, ...}`

每一步失败：`status='failed'`、`error_message=<具体错误>`。流水线幂等：失败后重 enqueue 同一个 job_id，从最后成功步重试（status 字段作为断点）。

**单 doc 失败不影响同 KB 其他 doc**：每个 doc 一个独立 job。

### DocumentParser 接口（新增包 `src/services/shared/bookparser`）

```go
package bookparser

import (
	"context"
	"io"
)

type BlockType string

const (
	BlockHeading   BlockType = "heading"
	BlockParagraph BlockType = "paragraph"
	BlockImage     BlockType = "image"   // M1.1+
	BlockTable     BlockType = "table"   // M1.1+
	BlockFormula   BlockType = "formula" // M1.1+
)

type Block struct {
	Type              BlockType
	Text              string
	HeadingPath       []string  // 当前位置的章节路径（M1.0 始终为空）
	HeadingInferred   bool      // 是否启发式推断的标题
	HeadingConfidence float32   // 0..1
	Metadata          map[string]any
}

type ParsedDoc struct {
	Blocks []Block
	Meta   map[string]any
}

type Parser interface {
	// Parse reads content from r per mime. Returns ParsedDoc with at least
	// one Block on success. Unsupported mime returns ErrUnsupportedMime.
	Parse(ctx context.Context, r io.Reader, mime string) (ParsedDoc, error)
}

// ErrUnsupportedMime is returned when a parser is asked to handle a mime
// type it doesn't support. Callers should map this to a 400-level error.
var ErrUnsupportedMime = errors.New("bookparser: unsupported mime type")
```

**M1.0 唯一实现**：`text.go` 处理 `text/plain` 和 `text/markdown`：把整份输入按空行（`\n\n`）切成 `BlockParagraph`，`HeadingPath=[]`、`HeadingInferred=false`、`HeadingConfidence=0`。`ParsedDoc.Meta = {"source_mime": mime, "byte_size": ...}`。

**Chunker 集成**：M0 的 `bookchunker.Chunk(text string, opts)` 升级为 `Chunk(doc ParsedDoc, opts)`：每个 Block 单独走 chunker（保留 BlockType 在输出 Chunk 的 metadata），文本段按 token 滑窗、图片/表格/公式块独立成 chunk（M1.0 不会出现这些类型）。**这个 chunker 签名变化是 M1.0 的 breaking change，需要同步改 M0 的端到端测试。**

### kb_search worker 工具

新增 `worker/internal/tools/builtin/kb/kb_search.go`，注册到 builtin registry：

```go
// Tool spec
Name:        "kb_search",
Description: "在指定知识库中按语义检索相关教材内容。",
Params: {
    "kb_id":   string (required, uuid),
    "query":   string (required),
    "k":       integer (default 8, max 50),
}
// Returns: {hits: [{document_ref: filename, ordinal: int, text: str, score: float, heading_path: []}]}
```

**实现**：
1. 验证当前 run 的老师属于 KB 的 workspace（查 `account_memberships` via worker 直连 DB）；不属于返回 `permission_denied`
2. 调 worker 内构造的 Doubao Embedder（与 ingest 用同一份配置）对 query 做 embed → 拿到 query vec
3. 调 `worker/internal/data/kb_chunks_repo.Search(kb_id, query_vec, k)` 拿命中
4. 拼装结果（包含 `document_ref` = `kb_documents.original_filename`，需 JOIN 一次）

**权限**：M1.0 阶段所有 KB 工具的权限统一为"Workspace 成员"。M1.2 可能引入更细控制（题目编辑权等）。

### book-tutor-agent persona

新增 `src/personas/book-tutor-agent/`:

```yaml
# persona.yaml
id: book-tutor-agent
version: "1"
title: 备课助手
description: 帮老师把上传的教材做语义搜索；后续版本将支持基于教材出题与组卷
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

`prompt.md` 工作流（精简版，M1.0 阶段）：

```markdown
# 你的角色

你是**备课助手**（book-tutor-agent），帮老师把上传到 ArkLoop 的教材做语义搜索。

# 工作原则

- **意图判定先行**：老师每条消息先判断是要"搜内容"还是"问 KB 状态"。其他意图（出题、组卷等）目前不支持，直接说明"该功能在后续版本提供"。
- **必须先确认 KB**：第一次互动用 `ask_user` 让老师指明操作哪个 KB（提供 KB 列表）。后续 turn 记住选定的 KB。
- **kb_search 之前讲清楚你打算搜什么**：避免误检索。

# 工作流：教材搜索

1. 用 `kb_search(kb_id, query, k=5)` 检索
2. 把命中按相似度排序展示给老师：每条显示 "[文档名 - 段落 N] 内容前 200 字..." + 相似度分数
3. 询问老师是否需要继续搜其他相关内容

# 边界

- M1.0 不能出题/组卷/删除内容。被问到时直接告知功能在后续版本。
- 不臆造未在 kb_search 结果中出现的内容。
```

### console-lite "知识库" UI

新增导航项 + 3 个视图：

1. **`/knowledge-bases`** —— 列表页
   - 表格列：KB 名 / 文档数 / 创建时间 / 操作（详情、删除）
   - 右上角"新建知识库"按钮 → 弹窗
2. **新建 KB 弹窗** —— 只有一个 `name` 输入框，workspace 自动绑定到当前活跃 workspace（沿用现有 console-lite workspace context）
3. **`/knowledge-bases/:id`** —— KB 详情页
   - 顶部 KB 名 + 描述
   - "上传文档"按钮（接受 .txt / .md，M1.0 单文件上传，size ≤ 10MB）
   - 文档列表：filename / status (badge 形式：queued/parsing/.../ready/failed) / chunk_count / 上传时间 / 操作（删除）
   - 文档状态通过 polling `GET .../documents/:id` 每 3 秒，直到 `ready` 或 `failed`
   - "搜索调试"区：输入框 + k 选择器 + 显示 hits（document_ref, ordinal, text 前 200 字, score）
   - 删除 KB 按钮（带确认）

**不做**：KB 重命名、文档批量上传、chunk 浏览、上传时进度条（一次性 POST 等待）、移动端适配（沿用现有 console-lite 响应式）

### M1.0 验收

一个真实老师在生产环境（或与生产同 schema 的 staging）能完整走完：

1. 在 console-lite 新建 KB "大学物理（上）"
2. 上传一份 1MB 大小的 `.txt`（≈ 50 万字符）
3. KB 详情页看到状态从 `queued → parsing → chunking → embedding → upserting → ready`（< 60s）
4. 调试搜索框输入"光的干涉"，看到 3 条相关 chunks，相似度 > 0.5
5. 切到 chat 选"备课助手"，说"搜光的干涉"，persona 调 kb_search 把命中段落展示
6. **同 workspace 的另一个老师账号**登录，看到这个 KB 和文档；非 workspace 成员**看不到**
7. 删除文档后，blob / chunks / kb_documents 行都被清理；KB 详情页文档列表不再显示

**Definition of Done**：上面 7 条全部 demo 给 jzefan 看一遍，无异常。

## 关键技术决策

1. **`kb_chunks` 直接 DROP 重建**——M0 数据是验证性的，迁移 00193 不做数据保留。M1.0 上线公告"重新上传"。
2. **M0 `/v1/_debug/kb/*` 路由 M1.0 同步删除**——避免双路径混淆；M1.0 的 `GET .../search` 调试入口替代。
3. **Worker queue 复用 `EnqueueRun` + `runID=uuid.Nil`**——与 `webhook.deliver` / `email.send` 同模式，不引入新 queue 类型。
4. **DocumentParser 接口 M1.0 定义、仅 `.txt` 实现**——给 M1.1 提供稳定 plug-in 点；`ParsedDoc.Block` 字段（含 `HeadingInferred` / `HeadingConfidence`）现在就定下，M1.1 加 PDF/DOCX 实现时不动 chunker 和 worker job。
5. **M0 的 `bookchunker.Chunk(text, opts)` 升级为 `Chunk(doc ParsedDoc, opts)`**——signature breaking change。M0 的端到端测试 + kbingest service 同步改。M0 已交付代码需要 jzefan 与 codex 协调时间窗修改。
6. **Search REST endpoint 保留**——M1.0 console-lite 调试用、API 用户故事 36 入口；M1.2 老师走 persona 后仍保留。
7. **状态轮询 3s 间隔**——不引入 SSE / WebSocket（沿用 PRD 决定）。
8. **Workspace 鉴权沿用现有 helper**——具体函数 plan 阶段查（看 `v1_workspace_files.go` / `v1_project_workspace_integration_test.go` 的鉴权写法）。

## 风险与缓解

| 风险 | 缓解 |
|------|------|
| M0 chunker 接口升级时 codex 还在跑 M0 实施，并发改动冲突 | M1.0 plan 第一个任务就是"等 M0 完全合并、tag 一个 `m0-frozen` commit 后再开 M1.0 分支" |
| Worker queue 复用 `runID=uuid.Nil` 在其他 query/log 里看着奇怪 | 加 audit log + structured log key `kb_ingest_job_id` 让查询能用；plan 里把这一点列为"必须有的 metric" |
| 中文 `.txt` 文件编码不是 UTF-8 老师上传后乱码 | API 收到 multipart 后立刻探测 + 转码（用 `golang.org/x/text/encoding`），失败返回 415；M1.0 默认只接受 UTF-8 编码的 `.txt` |
| 老师上传巨大文件把 worker 卡住 | M1.0 文件 size 上限 10MB（API 层 reject 超出）；单文件 chunker 输出 ≤ 5000 chunks，超过截断并标 `parse_meta.truncated=true` |
| 同 workspace 多老师同时操作同一 KB | M1.0 单用户场景为主，先不做乐观锁；M1.2 (kb_questions 写入) 才上锁 |

## 不在 M1.0 范围（推到 M1.1+）

- PDF / DOCX 解析（M1.1，依赖 S1）
- 扫描件 OCR、图表抽取、公式提取（M1.1）
- `kb_knowledge_points` 表的写入和管理（M1.2 才开始用）
- `kb_questions` / `kb_papers` 表（M1.2 / M1.3 才建）
- RAG 出题工具（`kb_draft_questions` / `kb_save_questions`）（M1.2）
- 组卷与 PaperComposer（M1.3）
- exam 系统集成 / QuestionStore exam 实现（M2，依赖 S2）
- 文档批量上传、KB 重命名、KB 详情编辑、文档手动重新摄入
- chunk-level 浏览/编辑
- 上传进度条、SSE 推送
- 多用户协作冲突处理
- KB 容量/配额限制
- 跨 KB 检索

## 不在本设计范围

- M1.1 / M1.2 / M1.3 的内部模块设计（各自下次 brainstorm）
- M2 examstore 实现细节（等 Spike S2 出 exam 合约后再说）
- 桌面端 / 移动端
- ACL 精细化（除 Workspace 成员外更细的角色）

## 下一步

进入 writing-plans skill，把 M1.0 拆成可执行任务清单。预算 5-7 天，按数据模型 → REST API → Worker job → DocumentParser → kb_search 工具 → persona → UI 7 大块组织。M1.1 / M1.2 / M1.3 等 M1.0 落地后单独 brainstorm + plan。
