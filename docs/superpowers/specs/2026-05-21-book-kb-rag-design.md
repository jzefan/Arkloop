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

0. **Spike S0：Doubao embedding 维度验证**（**M0 第 0 步，阻塞迁移**）
   - 写一个一次性 `cmd/embedprobe/main.go`，调配置里的 Doubao Ark `/embeddings` endpoint，输入一段中文字符串
   - 打印返回向量长度，作为 pgvector 列维度的**唯一权威依据**
   - 结果回填到本设计文档脚注；M0 代码用运行时 env 配置 endpoint/model，DB 维度固定为本次验证值
   - 在此 spike 完成前**禁止应用 vector(N) 迁移**——pgvector 维度改不动，错一次=drop 表重建
   - 顺带 probe：是否需要登录、单批最大数量、QPS 限制、latency baseline
1. **数据库迁移**（api 服务，新增 goose）—— **依赖 S0 结论**
   - `CREATE EXTENSION IF NOT EXISTS vector`
   - 建 `kb_chunks(id, kb_name TEXT, document_ref TEXT, ordinal INT, text TEXT, token_count INT, embedding vector(1024), metadata_json JSONB, created_at)` —— M0 用一张表跑通；M1 阶段再拆出 `knowledge_bases` / `kb_documents` 等正式 schema
   - hnsw 索引：`(embedding vector_cosine_ops)`
2. **`shared/bookchunker` 包**：
   - 接口：`Chunk(text string, opts ChunkOptions) []Chunk`
   - 策略：按段落（`\n\n`）切，超长段落按 token 滑窗，token 控制在 [256, 512]，相邻 ~40 token 重叠
   - 单元测试：golden fixture 覆盖正常段落、超长段落、过短段落合并
3. **`shared/embedding` 包**：
   - 接口：`Embed(ctx, texts []string) ([][]float32, error)`
   - 后端：Doubao Ark `/embeddings`，endpoint/model 通过 env 配置；默认沿用 `doubao-embedding-text-240715`
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
| **Embedding 模型** | Doubao Ark `/embeddings`，默认 endpoint/model `doubao-embedding-text-240715`，**维度 1024**（S0 2026-05-21：当前 key 可列出 embedding models，但公共 model id 调 `/embeddings` 返回 404/不支持，需部署时填可访问的 endpoint id；1024 与现有 OpenViking/Doubao 配置一致） | OpenAI 3-small (1536) 需代理；本地 bge 部署成本重；Doubao 国内母语 + 已有 provider 集成 |
| **Vector store** | pgvector，hnsw 索引，cosine 距离 | Qdrant/Milvus 增加新基建；ivfflat 数据小阶段比 hnsw 差且需训练 |
| **Tokenizer（计 token 数）** | tiktoken `cl100k_base` | Doubao 实际编码会略不同，但作为 chunker 切分边界依据稳定够用 |
| **Worker→KB 数据路径** | Worker 直连 PgBouncer，加 `worker/internal/data/kb_chunks_repo.go` | 沿用 `runs_repo` 现有模式；PRD 原文"worker 不直访 DB"原则**作废**（与现状不符） |
| **kb_ingest job 归属** | **Worker**（M1 起）：解析需要 sandbox / embedder 调外部 LLM provider，二者都是 worker 原生客户；api 仅做 REST CRUD + 状态查询，触发 ingest 就是往 worker queue 投消息。**M0 例外**：因 M0 无解析、无 queue，debug endpoint 同步调用 shared 包（shared 包 M0 写好，M1 在 worker import 复用，零浪费）。 | api 后台 job：api 调 sandbox 没有先例，会引入新模式；split 两段（解析在 worker、其余在 api）：跨服务状态机复杂度不值 |
| **题目溯源** | `kb_questions` / exam question schema 加 `source_snippets_json`（chunk_id + 200-500 字快照）。**Linked 模式下 snapshot 持久化路径由 S2 确认**：优先方案 a（exam 加专用字段，exam UI 也展示）；回退方案 b（ArkLoop 侧 `kb_question_snapshots` 表，仅 ArkLoop UI 展示）。**不接受**承诺丢失 | 不版本化 documents（太重）；不接受失效（破坏用户故事 19）；允许重摄入但联级警告影响题数 |
| **集成模式开关** | 维持 PRD 现有设计：env `ARKLOOP_EXAM_INTEGRATION_ENABLED` + KB 级 `integration_mode`，`QuestionStore` 接口 + `examstore`/`localstore` 两实现；examstore 直接调 exam REST，不挂 worker tool |  |
| **Chunker 默认参数** | token 区间 [256, 512]，重叠 ~40 token，段落感知 | 行业默认，不单问 |

## M1 启动前必做的 spike

非编码工作，结论写入 PRD / design 更新；不通过则 PRD 范围要调整。

### Spike S1：PDF 解析可行性（已完成 2026-05-23）

- **输入**：《妇产科学（第十版）》PDF，511 页 / 38.5MB，227 条 TOC（51 章 + 169 节）
- **做法**：`tools/spike-s1/parse_book.py`（PyMuPDF + pdfplumber + Tesseract）→ `evaluate.py` + 自动 bbox-clustering 多栏判定
- **结果**（详见 `tools/spike-s1/sample_outputs/fuchan-10-eval.txt`）：

| 维度 | 命中率 | 备注 |
| --- | --- | --- |
| heading 类型识别 | **10/10 = 100%** | 章节 + 子节都被识别为 heading |
| heading_path 深度 | **0/10 = 0%** | 当前 depth 启发式有 bug（详见下） |
| 多栏 reading order | **5/5 = 100%** | PyMuPDF 默认 block 顺序对 2-col 教材足够；x-flip 计数 ≤ 2 |
| OCR | n/a | 该书 0 扫描页，未能验证；M1.1 不阻塞此项，遇到扫描书再单独跑 |
| image 检测 | **6/6 = 100%** | 但需要 size 阈值过滤掉 cover/TOC 的矢量装饰元素（占总 image 的 86%） |
| 表格 dim 匹配 | 0/5 = 0% | pdfplumber 抽到了表格，但行列数与人工预期偏差大 |
| 公式 | n/a | 妇产书无公式样本；如理工科书需单独 spike |
| 吞吐 | 7.8 pages/s | 65.1s / 511 页，远高于估算的 6.8 pages/s |

- **结论 & 给 M1.1 的输入**：

  1. **OCR**：保留 Tesseract chi_sim 作为默认，**不引 PaddleOCR**（避免新依赖；后续遇真实扫描书质量不足再切）
  2. **公式**：维持 PRD 决策 — `chunk_type=formula` 存为纯文本；LaTeX 转换不进 M1.1
  3. **heading_path 必须重构**：当前算法 `depth = (p90 - block_size) // 2` 对正文字号均匀（body p90==median）的教材彻底失效。M1.1 改用**字号 + 字体族联合判定**：建一份 `font_size, font_family` → 已知 heading 级别的映射；新 block 落到最接近的一级。本书 `FZLTZCHK--GBK1-0` 是 heading 专用字体，`FZSSK--GBK1-0` 是正文字体 — 字体族即层级信号
  4. **image 过滤**：raster image 必须有 size 阈值（PRD 实施层应剔除 < 32×32 像素或 < 5KB 的"图像"），否则 1 个 PDF 容易掺进数千个无意义的矢量装饰
  5. **多栏**：PyMuPDF 默认 block 顺序已足以应付 2-col 教材；不需要额外 layout-aware 库
  6. **表格**：pdfplumber 检测**位置**准（59 个页面真有表格），但 rows/cols **维度计数对教材表格不可靠**。M1.1 接受这一限制：表格存 markdown 文本，rows/cols 字段写但不作语义保证；表格内容的 ground truth 由 chunk text 自身承载

### Spike S2：exam 后端合约对齐（0.5 天）

- **对齐**：与 exam 后端同事敲定 4 个端点的字段（PRD "对 exam 系统的依赖"小节）
- **关键问题（必须问出明确答案）**：
  1. exam questions schema 能否新增一个长文本字段（如 `source_snippets`）长期保存 ArkLoop 写入时的原文快照？exam 前台是否在题目详情页展示？
  2. 如不能加字段，是否有现成的 `metadata` / `extra_json` 字段可塞，且 exam 前台是否会渲染？
  3. 如二者都不行，确认 ArkLoop 会改用方案 b（自建 `kb_question_snapshots` 表），用户故事 19 在 Linked 模式下由 ArkLoop UI 展示 snapshot，exam UI 不展示——这是降级承诺，老师在 exam 前台看不到原文片段
- **产出**：`docs/integrations/exam-api.md` 作为合约，**明确记录上面 3 个问题的答案**
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
| Doubao endpoint 未开通或维度与 M0 DDL 不符 → 摄入失败 | `cmd/embedprobe` 可直接验证 endpoint/model；运行时 embedder 会检查返回维度必须等于 `kb_chunks.embedding` 维度，失败时不写库 | S0 |
| PDF heading 不可靠 → 切分退化 | `heading_inferred` 标志 + 序列 chunks 兜底（上节） | S1 |
| 中文 OCR 质量差 → 扫描件不可用 | Tesseract → PaddleOCR 切换路径预留 | S1 |
| Doubao embedding rate limit / 网络抖动 | embedding 包内 batch + retry + 限流；S0 顺带 probe QPS 上限 | S0 + M0 摄入测试 |
| 重摄入导致 source_chunk_ids 失效 | 软指针 + 原文快照（已锁决策） | M1 实施 |
| Linked 模式 exam 无字段存 snapshot | S2 优先谈方案 a；回退方案 b（ArkLoop 侧 `kb_question_snapshots` 表，exam UI 不展示但 ArkLoop UI 展示） | S2 |
| exam 端点字段不一致 / 不开放 | M1 走 standalone 跑通，M2 再补 examstore；S2 合约提前 | S2 |
| pgvector 镜像替换 / 现有数据迁移 | M0 阶段 dev/staging 全验证一遍再生产 | M0 部署 |
| PaperComposer 取不到足够题目 | `ShortageWarning` 返回 persona，由老师决定补题或放宽约束；**新增决定**：persona 自动循环"补题→重算" 上限 2 次，超出强制让老师人工裁决，防止死循环 | M1 实施 |
| kb_ingest 跑在 worker，worker queue 是否支持非 run-driven job | M1 启动前先确认 worker queue 抽象能装 "non-LLM job"；不能的话 M1 加一个 sibling queue（同 pg_queue 模式但不与 run 绑定）。**M0 不阻塞**：M0 是同步执行 | M1 启动 |

## PRD 已应用的回改（追溯）

本设计 brainstorm 触发以下 PRD 文档同步修改（与本设计落地一致）：

1. 删除"worker 不直访数据库"原则
2. `kb_chunks.embedding` 维度从 1536/2560 的记忆值改为 M0 固定 `vector(1024)`；运行时用 `cmd/embedprobe` 和 embedder 维度保护确认 endpoint 兼容
3. `kb_questions` 加 `source_snippets_json` 列；examstore 接口约定写入 exam 时也带这个字段；用户故事 19 表述更新
4. 新增 M0 里程碑（在 PRD 的"对 exam 系统的依赖"小节末尾扩展为 M0/M1/M2 三段）
5. 锁定 Doubao Ark `/embeddings` 作为 embedding 默认（PRD `Embedder` 模块说明里更新默认 provider；endpoint/model 运行时可配）

## Review 阶段补充决策（本节为 review 回复直接产出，先于 design 其他章节阅读）

1. **kb_ingest job 归属**：M1 起跑在 **worker** queue（理由：依赖 sandbox + 调外部 LLM provider 都是 worker 原生能力；api 调 sandbox 没先例）。M0 例外：同步在 api debug endpoint 内调 shared 包，shared 包 M0 写好后 M1 在 worker import 复用（零返工）。M1 启动前需确认 worker queue 抽象能装"non-LLM job"，不能则加 sibling queue。
2. **Doubao 维度**：`cmd/embedprobe` 已加入。2026-05-21 用当前 `.env` key 探测时，Ark `/models` 可列出 `doubao-embedding-text-240715` 等 embedding 型号，但直接以公共 model id 调 `/embeddings` 返回 404/不支持；M0 因此采用现有配置体系里的 Doubao embedding 默认维度 1024，并要求部署时通过 `ARK_EMBED_MODEL` 填真实可访问的 endpoint id。
3. **Linked 模式 snapshot 持久化**：S2 必须问到底——优先方案 a（exam 加字段且前台展示），回退方案 b（ArkLoop 侧 `kb_question_snapshots` 表，承诺由 ArkLoop UI 而非 exam UI 兑现）。**不接受**承诺丢失。

## 不在本设计范围

- M1 完整模块的接口细节（待 writing-plans 拆解）
- M2 examstore 实现细节（等 S2 合约出来再说）
- 桌面端、移动端集成
- 知识库 ACL 精细化（除 Workspace 可见性外）
- KB 与现有 OpenViking memory 系统的关系（目前不混用，KB 是用户级知识，OpenViking 是 agent 级记忆）

## 下一步

进入 writing-plans skill，把 M0 切片拆成可执行的 implementation plan。M1 / M2 在 M0 完成、S1 / S2 spike 出结论后再独立 brainstorm + plan。
