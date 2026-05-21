# Design: 书籍 → KB → RAG 出题/组卷 — 实施前脑暴产出

> Status: ready-for-plan
> Owner: jzefan
> Created: 2026-05-21
> Companion: [docs/prd/book-kb-rag.md](../../prd/book-kb-rag.md)

## Context

PRD（`docs/prd/book-kb-rag.md`）已写完两轮，覆盖了 standalone / Linked 两种集成模式 + QuestionStore 抽象。本文档是实施前的 pressure-test 产出：识别风险、固化关键决策、定义最小可交付切片（M0）、列出 M1 启动前的必要 spike。

PRD 描述 *做什么*，本设计描述 *怎么落地的第一步、哪些决策已经定死、哪些风险待 spike 验证*。

## M0 切片定义

**目标**：用最小代码量验证 chunker + Doubao embedding + pgvector + 检索这一条链通透；同时把 pgvector 上线 / compose 镜像替换这件运维工作提前压一次，避免它在 M1 实施末期成为意外阻塞。

**预算**：3-4 天。

**范围**：

1. **数据库迁移**（api 服务，新增 goose）
   - `CREATE EXTENSION IF NOT EXISTS vector`
   - 建 `kb_chunks(id, kb_name TEXT, document_ref TEXT, ordinal INT, text TEXT, token_count INT, embedding vector(2560), metadata_json JSONB, created_at)` —— M0 用一张表跑通；M1 阶段再拆出 `knowledge_bases` / `kb_documents` 等正式 schema
   - hnsw 索引：`(embedding vector_cosine_ops)`
2. **`shared/bookchunker` 包**：
   - 接口：`Chunk(text string, opts ChunkOptions) []Chunk`
   - 策略：按段落（`\n\n`）切，超长段落按 token 滑窗，token 控制在 [256, 512]，相邻 ~40 token 重叠
   - 单元测试：golden fixture 覆盖正常段落、超长段落、过短段落合并
3. **`shared/embedding` 包**：
   - 接口：`Embed(ctx, texts []string) ([][]float32, error)`
   - 后端：Doubao `doubao-embedding-text-240715`，通过现有 `llmproviders` 体系拿配置
   - 自带 batch（每批 ≤ 32 文本）、限流（按 Doubao QPS 配置）、retry（指数退避，3 次）
4. **`api/internal/data/kb_chunks_repo.go`**：
   - `Upsert(ctx, kbName, []ChunkVec) error`
   - `Search(ctx, kbName, queryVec, k int) ([]Hit, error)`
5. **2 个 debug HTTP endpoint**（仅 M0 验证用，M1 会换掉）：
   - 路径走 `/v1/_debug/kb/...` 前缀，复用 api 现有 `_debug/*` 路由保护方式（需要 env `ARKLOOP_DEBUG_TOKEN` 与请求头匹配；Gateway 路由表不暴露这一前缀）
   - `POST /v1/_debug/kb/ingest` body `{file_path, kb_name}` —— 同步走 chunker→embedder→upsert，返回 `{chunk_count, duration_ms}`
   - `POST /v1/_debug/kb/search` body `{kb_name, query, k}` —— 返回 top-k chunks
   - 实施时若发现 api 没有 `_debug/*` 既有保护模式，直接新加一个 `requireDebugToken` middleware，不引入新鉴权框架
6. **compose / 部署**：
   - `compose.yaml` postgres 镜像换 `pgvector/pgvector:pg16`
   - 在 dev / staging 完整验证一遍后才动生产
   - deploy 脚本加一次性升级确认（沿用 setup.sh / deploy-source-to-server.sh 既有交互模式）
7. **1 个 integration test**：塞一段中文教科书文本（约 5KB）→ `Search("光的干涉")` → 断言命中包含相应 chunk

**不在 M0 范围**：persona、UI、QuestionStore、PDF 解析、Job queue、Workspace 鉴权、worker 工具、`knowledge_bases` 完整 schema。

**M0 验收**：手动 `curl` 跑通 ingest 和 search，命中合理；pgvector 在生产升级方案得到运维确认。

## 已锁决策（M0 → M1 不可变）

| 决策 | 选定 | 备选/为什么不选 |
|------|------|----------------|
| **Embedding 模型** | Doubao `doubao-embedding-text-240715`，**维度 2560** | OpenAI 3-small (1536) 需代理；本地 bge 部署成本重；Doubao 国内母语 + 已有 provider 集成 |
| **Vector store** | pgvector，hnsw 索引，cosine 距离 | Qdrant/Milvus 增加新基建；ivfflat 数据小阶段比 hnsw 差且需训练 |
| **Tokenizer（计 token 数）** | tiktoken `cl100k_base` | Doubao 实际编码会略不同，但作为 chunker 切分边界依据稳定够用 |
| **Worker→KB 数据路径** | Worker 直连 PgBouncer，加 `worker/internal/data/kb_chunks_repo.go` | 沿用 `runs_repo` 现有模式；PRD 原文"worker 不直访 DB"原则**作废**（与现状不符） |
| **题目溯源** | `kb_questions` / exam question schema 加 `source_snippets_json`（chunk_id + 200-500 字快照） | 不版本化 documents（太重）；不接受失效（破坏用户故事 18）；允许重摄入但联级警告影响题数 |
| **集成模式开关** | 维持 PRD 现有设计：env `ARKLOOP_EXAM_INTEGRATION_ENABLED` + KB 级 `integration_mode`，`QuestionStore` 接口 + `examstore`/`localstore` 两实现；examstore 直接调 exam REST，不挂 worker tool |  |
| **Chunker 默认参数** | token 区间 [256, 512]，重叠 ~40 token，段落感知 | 行业默认，不单问 |

## M1 启动前必做的 spike

非编码工作，结论写入 PRD / design 更新；不通过则 PRD 范围要调整。

### Spike S1：PDF 解析可行性（1-2 天）

- **输入**：1 本 jzefan 手上真实中文教材 PDF（最好包含目录页、正文段落、图表、扫描页混合）
- **做法**：在 sandbox 跑 PyMuPDF + Tesseract，输出 ParsedDoc-like JSON
- **判定**：
  - heading 识别率（人工抽样 20 个章节标题，对几个）
  - 扫描页 OCR 中文准确率（抽样 1 页对比）
  - 图片/表格抽出率
- **结论**：
  - Tesseract 中文够不够 → 不够切换 PaddleOCR
  - 公式抽取是否进 M1 → 不行就降级 chunk_type=formula 的纯文本，老师靠原文截图

### Spike S2：exam 后端合约对齐（0.5 天）

- **对齐**：与 exam 后端同事敲定 4 个端点的字段（PRD "对 exam 系统的依赖"小节）+ `source_snippets_json` 在 exam question schema 里的字段名
- **产出**：`docs/integrations/exam-api.md` 作为合约
- **时机**：M2 实施才需要，但合约**现在就要敲**，避免 M2 启动时反复返工

## PDF heading 不可靠时的兜底（M1 实施细则）

- ParsedDoc 的每个 Block 携带 `heading_inferred bool` 和 `heading_confidence float`
- Chunker 输入端：若整份文档 heading_inferred=false 占比 > 50%，自动退化为"纯序列 chunks"模式（按 token 长度均匀切，`heading_path = []`）
- Persona / kb_search 输出端：命中 chunk 的 `heading_path` 为空时，UI/对话明确告知"此段未识别到章节标题，来自第 N 页"
- **不做 LLM 抽 TOC 自动对齐**——超 M1 范围

## 多老师并发写同一 KB（M1 实施细则）

- `kb_documents`、`kb_questions` 加 `updated_at` 列
- 写入采用乐观锁：`UPDATE ... WHERE id = ? AND updated_at = ?`
- 冲突返回 HTTP 409 + 最新 entity；persona 透传给老师让其决定如何 reconcile
- M0 单用户跑通，不验证并发；M1 阶段加并发 integration test

## 风险与责任标记

| 风险 | 缓解 | 触发 spike |
|------|------|------|
| PDF heading 不可靠 → 切分退化 | `heading_inferred` 标志 + 序列 chunks 兜底（上节） | S1 |
| 中文 OCR 质量差 → 扫描件不可用 | Tesseract → PaddleOCR 切换路径预留 | S1 |
| Doubao embedding rate limit / 网络抖动 | embedding 包内 batch + retry + 限流 | M0 摄入测试验证 |
| 重摄入导致 source_chunk_ids 失效 | 软指针 + 原文快照（已锁决策） | M1 实施 |
| exam 端点字段不一致 / 不开放 | M1 走 standalone 跑通，M2 再补 examstore；S2 合约提前 | S2 |
| pgvector 镜像替换 / 现有数据迁移 | M0 阶段 dev/staging 全验证一遍再生产 | M0 部署 |
| PaperComposer 取不到足够题目 | `ShortageWarning` 返回 persona，由老师决定补题或放宽约束；**新增决定**：persona 自动循环"补题→重算" 上限 2 次，超出强制让老师人工裁决，防止死循环 | M1 实施 |

## PRD 已应用的回改（追溯）

本设计 brainstorm 触发以下 PRD 文档同步修改（与本设计落地一致）：

1. 删除"worker 不直访数据库"原则
2. `kb_chunks.embedding` 维度从 1536 改 2560（Doubao）
3. `kb_questions` 加 `source_snippets_json` 列；examstore 接口约定写入 exam 时也带这个字段；用户故事 18 表述更新
4. 新增 M0 里程碑（在 PRD 的"对 exam 系统的依赖"小节末尾扩展为 M0/M1/M2 三段）
5. 锁定 Doubao 2560 作为 embedding 默认（PRD `Embedder` 模块说明里更新默认 provider）

## 不在本设计范围

- M1 完整模块的接口细节（待 writing-plans 拆解）
- M2 examstore 实现细节（等 S2 合约出来再说）
- 桌面端、移动端集成
- 知识库 ACL 精细化（除 Workspace 可见性外）
- KB 与现有 OpenViking memory 系统的关系（目前不混用，KB 是用户级知识，OpenViking 是 agent 级记忆）

## 下一步

进入 writing-plans skill，把 M0 切片拆成可执行的 implementation plan。M1 / M2 在 M0 完成、S1 / S2 spike 出结论后再独立 brainstorm + plan。
