# PRD: 上传书籍 → 知识库 → RAG 出题/组卷

> Status: ready-for-agent
> Owner: jzefan
> Created: 2026-05-21

## Problem Statement

学校/教研团队手上有大量 PDF/Word 教材（含图片、表格、公式）。他们希望：

1. 由管理端把整本书上传进 ArkLoop，让平台"读懂"内容并形成可复用课程资料知识库；
2. 老师在 ArkLoop 老师端直接说"为第 3 章物理光学知识点出 10 道题，5 道单选 5 道填空，中等难度"，由 AI **基于教材内容**生成题目；
3. 生成的**知识点、题目、试卷必须沉淀到 exam 系统**（学校师生最终用的就是 exam，不能让题目散落在 ArkLoop 里没人能再用到）；
4. 生成新题时**能参考 exam 题库已有的题目**——一方面避免和已有题大幅重复，另一方面让 AI 学到这门课已经形成的命题风格、难度区间、答案/解析的表达习惯；
5. 同一个工作区里的多位老师共享同一份教材知识库，不用各自重复上传。

当前差距：
- 现有 `exam-agent` persona 调 `exam_generate_questions` 让 exam 后端凭 LLM 训练记忆出题——**没有教材内容做依据**，且**不会回看已有题库**；
- ArkLoop 端没有任何机制承载用户上传的教材并做语义检索（pgvector / 向量库未接入，没有 PDF/DOCX 解析）；
- "题目/试卷写哪儿"也没明确——如果 ArkLoop 自己存一份，会和 exam 形成两个真相源，老师在 exam 前台看不到这些题，等于白生成。

## Solution

在 ArkLoop 内新增**知识库（KnowledgeBase, KB）**资源（归属 Workspace），承担"教材内容的语义检索层"。题目/试卷/知识点的存储后端**可切换**：连 exam 系统（权威）或留在 ArkLoop 本地（独立模式）。两种模式对老师在 persona 里的操作体验一致。

- **KB（ArkLoop 侧）**：管理端在工作区里创建 KB，上传 PDF / DOCX / 图片；后台异步完成 解析 → 切分 → 向量化 → 写入 pgvector；UI 展示每份文档处理状态。老师端不承担知识库建设、上传、删除、重建等复杂管理工作。
- **两种集成模式**（见下文"集成模式开关"小节，部署级 + KB 级双重控制）：
  - **Linked 模式（连 exam）**：题目/试卷/知识点的真相源在 exam；生成时拉 exam 已有题做参考，老师确认后写回 exam；试卷快照在 exam。**这是有 exam 系统时的推荐模式**。
  - **Standalone 模式（不连 exam）**：题目/试卷存在 ArkLoop 自己的 `kb_questions` / `kb_papers` 表；生成时参考本地题库做去重和风格示范；老师可导出 markdown/PDF 自行使用。**适合没有 exam 系统的自托管用户或试用阶段**。
- **老师端 persona `book-tutor-agent`（展示名：智能组卷）**：老师用自然语言指定知识点/范围/题型/难度/数量；内部三段式：
  1. `kb_search` 从教材 KB 检索相关章节内容；
  2. `kb_draft_questions` 拉该知识点已有题目作为"参考样本 + 去重黑名单"，并把检索内容、参考样本和老师指令交给 LLM 形成草稿——**底层走 exam 还是 ArkLoop 本地由 KB 配置决定，persona 不感知**；
  3. 老师逐题确认后 `kb_save_questions` 写入对应后端。
- **组卷**：按题型/难度/知识点分布从题池抽样；题池实时从对应后端（exam 或本地）拉取；老师确认后 `kb_compose_paper` 落到对应后端，同时返回 markdown 预览，并在老师需要时导出 PDF。
- **目录联动**（仅 Linked 模式）：若书带清晰 TOC，persona 可从 KB 提取目录结构 → `show_widget` 让老师改 → 复用现有 `exam_create_catalog_tree` 建到 exam。Standalone 模式下 KB 自带一个轻量"知识点"标签集合（自由文本，不强制层级）。
- **与 `exam-agent` 的关系**：`exam-agent` 仍只在 Linked 部署下出现（依赖 exam_* 工具），继续负责"Excel/图片目录导入 + 不依赖 KB 的轻量出题"；`book-tutor-agent` 在两种部署下都可用。

### 2026-05-25 产品边界更新

后续实现以当前产品决策为准：

- **管理端负责复杂知识库建设**：创建 KB、绑定课程范围、上传/删除资料、查看摄入状态、检索调试等都放在 console-lite/管理端。
- **老师端只负责使用知识库**：老师通过 web/chat 中的 **智能组卷** 入口问答、出题、组卷。老师不需要知道数据来自本地题库还是外部系统，也不需要进入 exam 前端。
- **Standalone UI 暂缓**：Standalone 模式继续保留后端能力和老师端智能组卷能力，但暂不做 console-lite 里的题库浏览/编辑/删除、试卷列表/导出等管理 UI。
- **保存前必须确认**：AI 生成题目和试卷都先作为草稿展示；只有老师明确确认后才保存。
- **固定题库口径**：通过 ArkLoop 智能组卷生成的题目进入固定的"组卷题库"；题目和试卷都要保存。

## User Stories

1. As a 管理员/教研人员, I want 在工作区下创建一个命名的知识库（如《大学物理（上）》）, so that 同工作区老师能复用同一份教材而不需要各自上传。
2. As a 管理员/教研人员, I want 把一本 PDF 教材上传进指定知识库, so that 系统能解析其文本、图片、表格、公式作为后续出题素材。
3. As a 管理员/教研人员, I want 同时上传多个文件（章节拆开的 PDF、补充的 DOCX 习题册）, so that 一次操作完成整本书的导入。
4. As a 管理员/教研人员, I want 实时看到每个文档的处理进度和状态（queued / parsing / chunking / embedding / ready / failed）, so that 知道什么时候可以让老师开始出题。
5. As a 管理员/教研人员, I want 处理失败的文档能看到具体失败原因（比如"扫描件 OCR 失败"、"文件加密"）, so that 我能针对性修复后重传。
6. As a 管理员/教研人员, I want 删除知识库里某个文档, so that 错传或过期的资料可以清掉，且向量库里对应 chunk 也一并清除。
7. As a 管理员/教研人员, I want 删除整个知识库, so that 学期结束后可以一键清理。
8. As a 管理员/教研人员, I want 看到知识库占用的存储和 chunk 数量, so that 心里有数。
9. As a 管理员/教研人员, I want 在 console-lite "知识库"管理页里浏览 KB 列表、点开看每个 KB 的文档清单, so that 不需要靠 API 才能管理资源。
10. As a 老师, I want 知识库支持 PDF（含扫描件）、DOCX、纯文本、图片（PNG/JPG）这几种常见格式, so that 不被格式所限。
11. As a 系统, I want 教材里的图片在解析时被 OCR + 多模态描述, so that 出题时能引用图示（如"如图所示的电路"）。
12. As a 系统, I want 教材里的表格被转成 markdown 表格保留行列结构, so that 表格内容的语义不丢失。
13. As a 系统, I want 数学公式被识别为 LaTeX 形式, so that 出题时公式能正确显示。
14. As a 系统, I want 切分时保留章节标题路径（如《物理 > 第 3 章 > 3.2 节》）, so that 检索结果能告诉老师"这道题出自哪一节"。
15. As a 系统, I want 图片块和表格块作为独立 chunk 存储（带 caption / 表标题）, so that 检索时图/表是独立的可命中单元，而不被切碎。
16. As a 老师, I want 在 book-tutor-agent 里用自然语言说"给我从第 3 章选 5 道单选题，难度中等", so that 不需要点几十次按钮。
17. As a 老师, I want persona 在生成题目前先告诉我它检索到了哪些章节/段落, so that 我能判断它是不是抓对了内容。
18. As a 老师, I want persona 在生成题目前**先去 exam 拉出该知识点下已有题目**，作为参考样本和去重依据, so that 新题既不会与已有题重复，也能契合这门课已经形成的命题风格/难度区间/解析表达。
19. As a 老师, I want 每道生成的题目都标注来源（书名 + 章节路径 + chunk_id + **写入时的原文 200-500 字快照**）, so that 即使后续管理端重新上传/覆盖了教材文件，老题仍能看到当时的原文片段做核对。
20. As a 老师, I want 生成的题目自动绑定 exam 知识点 ID, so that 写回 exam 后能在 exam 前台按知识点查到。
21. As a 老师, I want persona 一次只生成 ≤ 5 道题, so that 不满意可以中断调整方向，不浪费 token（沿用现有 exam-agent 的节奏约定）。
22. As a 老师, I want **生成完的题目我可以逐题预览，确认后才统一调 exam_create_questions 写回 exam**, so that 不让 AI 直接污染我的官方题库。
23. As a 老师, I want 写入 exam 失败（比如知识点不存在、字段校验失败）时能看到具体错误并选择"修正后重试"或"放弃这道", so that 部分失败不让整批白做。
24. As a 老师, I want 一道生成完的题目我能手动修改题干/答案/解析后再确认写回, so that AI 出错时我能纠正而不是只能丢掉。
25. As a 老师, I want 用一句自然语言（"组一张期中卷：单选 10、多选 5、填空 5、解答 5，难度比例 3:5:2，覆盖第 1-3 章"）触发组卷, so that 不用先去画表格再去抽样。
26. As a 老师, I want 组卷支持显式约束：题型分布、难度分布、知识点分布、总题数、是否允许重复, so that 输出符合教学大纲。
27. As a 老师, I want 组卷支持随机 seed, so that 同样的约束 + seed 输出同样的试卷（可复现，便于复盘）。
28. As a 老师, I want **组卷的候选题池由 persona 实时从 exam 拉取**, so that 取到的是 exam 当下最新的题库，不会用过期快照。
29. As a 老师, I want 组卷时如果 exam 题池里某个知识点/难度的题不够, persona 先提示我"X 知识点 hard 难度只有 1 道，需要补 2 道", so that 我能选择"现场用 RAG 补几道再组"或"放宽约束"。
30. As a 老师, I want **组好的试卷确认后写回 exam**（试卷+题目映射+元数据），同时给我一份 markdown/PDF 副本, so that 学校 exam 前台能直接派发，纸质版也有。
31. As a 管理员/教研人员, I want 上传一本带清晰 TOC 的书后，管理端能辅助提取目录并建设知识点树, so that 老师开始智能组卷前已经有可用的课程资料范围。
32. As a 老师, I want KB 里的 document 与 exam 知识点之间可以建立软关联（哪个文档对应哪些知识点）, so that 后续做"针对这本书的所有题"这种聚合视图。
33. As a 同一个工作区的另一位老师, I want 看到工作区已有的知识库, so that 我不用从零开始建库。
34. As a 工作区管理员, I want 控制知识库的可见性（工作区内全员/只自己）, so that 私人备课资料和共享教材分开管理。
35. As an API 调用方（脚本 / 第三方集成）, I want 通过 REST 调用 `POST /v1/knowledge-bases/:id/documents` 上传文档, so that 可以批量从已有教材库迁移。
36. As an API 调用方, I want 通过 `GET /v1/knowledge-bases/:id/search?q=...` 查询命中的 chunk（含分数、章节路径、原文）, so that 验证检索质量、调参用。
37. As a 平台运维, I want 摄入流水线的每一步（parse/chunk/embed/upsert）有结构化日志和耗时埋点, so that 出问题能定位是哪一段。
38. As a 平台运维, I want exam 写回失败（HTTP 5xx / 鉴权失效 / 字段不匹配）能在 worker 日志里看到具体调用、参数、响应, so that 能快速定位是 ArkLoop bug 还是 exam 后端问题。
39. As a 平台运维, I want 解析后端、embedding provider、vector store 通过接口可替换（pgvector → Qdrant 等）, so that 后续换实现不动业务代码。
40. As a 用户, I want 上传文件大小、类型、单 KB 文档数有明确上限和清晰报错, so that 不会偷偷写挂数据库。
41. As a 用户, I want 解析或入库失败不会把整个 KB 弄坏, so that 单文档失败可重试，其他文档不受影响。
42. As a 自托管部署的用户（没接 exam）, I want 安装 ArkLoop 后不需要任何 exam 相关配置就能用 book-tutor-agent 上传书、生成题、组卷, so that 没 exam 系统也能独立使用这套能力。
43. As a 平台管理员, I want 通过环境变量 `ARKLOOP_EXAM_INTEGRATION_ENABLED=true/false` 一键决定本部署是否启用 exam 集成，并通过 `EXAM_BASE_URL` 配置 exam 后端地址, so that 升级或维护 exam 时可以临时关掉集成不影响 standalone KB 的使用。
44. As a 老师, I want 新建 KB 时显式选择"独立模式"还是"绑定 exam 课程", so that 我清楚这本知识库生成的题最终去哪。
45. As a 老师, I want 看到 KB 列表里每个 KB 标注它的模式（独立 / 已绑定 exam 课程：《大学物理》）, so that 不会搞混。
46. Deferred. Standalone KB 的 console-lite 题库浏览、编辑、删除功能暂不进入当前范围；老师端通过智能组卷完成生成、预览、确认保存。
47. Deferred. Standalone KB 的 console-lite 试卷列表/导出功能暂不进入当前范围；老师端组卷结果先返回 markdown，PDF 导出按老师需要通过智能体触发。
48. As a Linked KB 的老师, I want persona 在保存前明确提示"确认后将保存到组卷题库/试卷库", so that 我知道这步会保存正式数据，不会误操作；老师不需要感知底层系统名称。
49. As a 集成模式由 Linked 转为 Standalone（或反向）的诉求, I want 系统直接告诉我"KB 创建后模式不可变，请新建一个 KB", so that 不会被假承诺误导，知道走正确路径。

### Option 2: 直接出题/组卷（命题技能 + 已有题样本，无 KB）

> 这条流是 Option 1（"上传书籍 → 知识库 → 出题/组卷"）的补充。**不需要上传任何教材**：靠老师装的"命题技能"（如《妇产科国考命题专家》）+ exam 已有题做样本锚点驱动 AI 出题。仅在 Linked 模式（`ARKLOOP_EXAM_INTEGRATION_ENABLED=true`）下提供，Standalone 部署看不见这条流。

50. As a 老师, I want 在工作区里安装一份"命题技能"（如《妇产科国考命题专家》）, so that AI 出题时按该领域权威风格输出（A1-A4 题型、医学命题规范、解析风格），而不是泛泛 LLM 通用风格。
51. As a 老师, I want 进入"命题专家"persona 时它自动列出 profile 已装的命题技能让我选一个, so that 不需要每次手敲 skill_key。
52. As a 老师, I want 选定命题技能后，persona 用一句话和我确认要在 exam 哪个课程下出题, so that 后续操作 scope 限定在课程内，候选知识点和种子题只来自该课程。
53. As a 老师, I want 在 exam 中按知识点 / 难度 / 题型 / pattern_tag（A1-A4 等）筛选已有题作为本次出题的"参考样本", so that AI 能学到这门课已有题的命题水准、用词风格、解析表达。
54. As a 老师, I want AI 出的新题 `pattern_tag` 必须和我选的参考样本一致（参考是 A2 → 新题也只能是 A2）, so that 题型分布稳定，不会被 AI 偷偷换成简单题型。新题 `pattern_tag` 不一致时 persona 把不合规题展示给我，让我选"重生成"或"放过这道"，**绝不静默修正**。
55. As a 老师, I want 当我选的知识点下没有任何已有题, persona 明确拒绝并引导我"先去 exam-agent 录入几道再回来", so that 不会让 AI 在没有水准锚点的情况下野蛮发挥（这是这条流的强约束，不允许"无种子 fallback 模式"）。
56. As a 老师, I want 生成完的题逐题预览，确认后批量写回 exam（沿用 Linked KB 的 `POST /api/questions/batch` 端点）, so that 这套生成的题立刻进 exam 题库可用；写入时 `pattern_tag` 也一并存进 exam 的 questions 表。
57. As a 老师, I want 累够题后用一句话触发组卷（`exam_build_paper`）, so that 这次 session 一站式从生成走到组卷出货，不用再切到别的 persona。
58. As a 平台运维, I want 命题技能必须在 `skill.yaml` 显式声明 `category: item-writing`、`pattern_tags`、`target_question_types`、`subject_tags` 这几个字段, so that worker 加载时能校验、persona 启动时能筛选展示，未声明的"命题技能"不会被错误使用。
59. As a 用户, I want 没有装任何命题技能时进 persona, persona 友好提示我"去 console-lite 知识库/技能页装一个 item-writing 类技能再来", so that 不被空白 selector 困扰。

## Implementation Decisions

### 范围与归属

- **KB 资源归属 Workspace**：每个 Workspace 可有多个 KB；KB 通过 `workspace_ref` 关联，复用现有 `workspace_registries` + `account_memberships` 的访问控制路径。
- 现有 `Account / Workspace / Profile` 模型不变；不引入新的租户层。
- **PDF/DOCX/图片**为 v1 必须支持的格式；EPUB/PPT/Markdown 列为后续。

### 集成模式开关（两层）

**1. 部署级总开关（env，默认 false）**

```
ARKLOOP_EXAM_INTEGRATION_ENABLED=true|false
EXAM_BASE_URL=https://exam.example.edu             # required when enabled
```

- `false`（默认）：`book-tutor-agent` 只暴露 standalone QuestionStore；UI 上"绑定 exam 课程"选项隐藏；`exam-agent` persona 不注册（`user_selectable=false`）；4 个新 `exam_*` 工具不挂到 worker registry。
- `true`：两种模式都可用；新建 KB 时老师可选"独立模式"或"绑定 exam 课程"。

**2. KB 级绑定字段**

- `knowledge_bases.integration_mode` enum：`standalone | exam`
- `knowledge_bases.exam_scope_id` —— 仅在 `integration_mode='exam'` 时必填
- 创建后不可切换（避免迁移题目的复杂性）；要换就建新 KB

**模式判定逻辑**：worker 的 `kb_*` 工具拿到 `kb_id` 后先查 `integration_mode`，按结果挑 `QuestionStore` 实现。Persona prompt 不区分模式，工具内部分流。

### 数据模型

**ArkLoop 侧（api 服务，新增 goose 迁移）**：

- `knowledge_bases(id, workspace_ref, account_id, name, description, visibility, integration_mode, exam_scope_id?, created_by, created_at, updated_at)`
- `kb_documents(id, kb_id, original_filename, mime_type, blob_sha256, size_bytes, status, error_message, parse_meta_json, created_by, created_at, updated_at)` —— 原始文件按 sha256 走现有 Workspace blob 存储复用，不重复保存
- `kb_chunks(id, kb_id, document_id, ordinal, heading_path, chunk_type, text, token_count, embedding vector(1024), metadata_json, created_at)` —— `embedding` 列使用 **pgvector** 扩展；M0 Spike 已固定当前迁移为 `vector(1024)`，运行时用 embedder 维度保护避免模型漂移；建 hnsw 索引（`vector_cosine_ops`）
- `kb_knowledge_points(id, kb_id, name, parent_id?, exam_knowledge_point_id?, sort_order, created_at)` —— 两种模式都用的轻量知识点表：
  - Standalone：完全在本地维护，自由层级
  - Linked：作为 exam 知识点树在 ArkLoop 侧的镜像/快照，`exam_knowledge_point_id` 关联到 exam（同步策略：persona 操作时按需拉取/刷新，不做后台同步）
- `kb_document_knowledge_points(kb_id, document_id, knowledge_point_id, created_at)` —— 文档↔知识点软关联
- **仅 Standalone 模式使用**（Linked 模式下这两张表对该 KB 为空）：
  - `kb_questions(id, kb_id, knowledge_point_id, question_type, difficulty, stem, options_json, answer, explanation, source_chunk_ids_json, source_snippets_json, quality_flag, created_by, created_at, updated_at)` —— `source_snippets_json` 存写入时的 chunk 原文快照（每条目标 200-500 字，过长截断；保存层会在缺失时按 `source_chunk_ids` 自动补齐），保证重摄入后老师仍能看到当时的原文（用户故事 19 的承诺）
  - `kb_papers(id, kb_id, name, spec_json, seed, question_ids_json, created_by, created_at)`
- 所有表通过 `account_id` 隔离，遵循现有租户约束模式

**exam 侧（外部系统，仅 Linked 模式涉及）** —— 题目/试卷/知识点的权威存储；本 PRD 需要 exam 侧新增的端点见下文"对 exam 系统的依赖"。

> **设计原则**：Standalone 表和 Linked 表互不混淆。Standalone KB 的题永远在 `kb_questions`；Linked KB 的题永远在 exam。不存在"缓存""同步"，避免双真相源。

### 模块拆分（按"深模块"组织）

#### A. DocumentParser（独立包 `src/services/shared/bookparser/`）
- 接口：`Parse(ctx, file io.Reader, mime string) (ParsedDoc, error)`，`ParsedDoc.Blocks []Block`
- `Block.Type` 枚举：`heading | paragraph | image | table | formula`
- **解析后端**：Worker 通过 `sandbox` 服务跑一段 Python（PyMuPDF / pdfplumber / python-docx / Pillow + Tesseract）作为子进程；Go 侧只负责发起、收 stdout JSON。这样：
  - PDF/DOCX 生态用最成熟的 Python 工具，零额外服务
  - 解析逻辑可独立迭代，Go 接口稳定
  - Sandbox 已有路径成熟（Firecracker / Docker 都行）
- 图片块：先 OCR，再异步调多模态 LLM 生成 caption（若 provider 可用）；OCR 失败时把图片块标 `needs_review` 但不阻塞整体
- 表格块：转 markdown 表格存原文 + 附带 cells 二维数组在 `parse_meta_json` 里
- 公式块：识别失败时降级成纯文本，但保留 `chunk_type=formula` 标记

#### B. Chunker（独立包 `src/services/shared/bookchunker/`，纯函数）
- 接口：`Chunk(doc ParsedDoc, opts ChunkOptions) []Chunk`
- 切分策略：
  1. heading-aware：在 heading 边界 + 段落边界优先切
  2. 每段控制在 `[min_tokens, max_tokens]` 区间（默认 256–512 tokens，可配）
  3. 相邻段之间保留少量重叠（默认 ~40 tokens）避免上下文断裂
  4. 图片块、表格块、公式块**始终独立成一个 chunk**，不与文本合并
  5. 每个 chunk 携带 `heading_path`（如 `["第三章 物理光学", "3.2 干涉"]`）和 `block_refs`（指回原 Block）
- 纯函数：相同输入恒定输出，便于回归测试

#### C. Embedder（独立包 `src/services/shared/embedding/`）
- 接口：`Embed(ctx, texts []string) ([][]float32, error)`
- 实现复用现有 `api/internal/llmproviders` 体系
- **默认 provider：Doubao `doubao-embedding-text-240715`，当前迁移维度 1024**（中文母语 + 国内可直连 + 已有 provider 集成；详见 design `docs/superpowers/specs/2026-05-21-book-kb-rag-design.md`）
- 接口同时支持 OpenAI 系列和 OpenViking 本地 backend，但**切换模型 = KB 全量重建**（pgvector 列维度是 DDL 级常量）
- 自带 batch（≤32 文本/批）+ 限流 + 重试（指数退避，3 次）封装；接口对外只暴露文本→向量

#### D. VectorStore（`src/services/api/internal/data/kb_vector_repo.go`）
- 接口：
  - `Upsert(ctx, kbID, []ChunkVec) error`
  - `Search(ctx, kbID, queryVec, k int, filters SearchFilters) ([]Hit, error)`
- 默认实现：**PostgreSQL + pgvector**（在 `migrations` 里 `CREATE EXTENSION IF NOT EXISTS vector`）
- `SearchFilters` 支持：`document_ids`、`heading_prefix`、`chunk_type`
- 通过接口隔离，后续可替换 Qdrant/Milvus 不动调用方

#### E. KB 资源 + 摄入流水线（api 服务）
- REST API（在 `api/internal/http/conversationapi` 或新增 `kbapi`）：
  - `POST /v1/knowledge-bases` — 创建 KB
  - `GET  /v1/knowledge-bases` — 列举当前 Workspace 下 KB
  - `DELETE /v1/knowledge-bases/:id` — 删除 KB（连带 documents/chunks/questions/papers）
  - `POST /v1/knowledge-bases/:id/documents` — 多部分上传，返回 `{ document_id, job_id }`，**入流水线后立即返回，不阻塞**
  - `GET  /v1/knowledge-bases/:id/documents` / `GET .../documents/:id` — 状态查询
  - `DELETE /v1/knowledge-bases/:id/documents/:id` — 删除单文档（同时清向量）
  - `GET  /v1/knowledge-bases/:id/search?q=&k=` — 调试用检索接口
- 摄入流水线 `kb_ingest` 作为 job 类型跑在 **worker** 队列里（理由：解析依赖 sandbox、embedder 调外部 LLM provider，都是 worker 原生能力；api 调 sandbox 无先例）。API 端只负责"REST 接收上传 → 落 `kb_documents` → 投 worker queue → 返回 job_id"。
  - 状态机：`queued → parsing → chunking → embedding → upserting → ready`，任何步骤失败 → `failed` + `error_message`
  - 串行流水线：A → B → C → D（每一步幂等，失败可从最后成功步重试）
  - 单 document 失败不影响同 KB 其他 document
  - M0 例外：M0 阶段无 queue，debug endpoint 同步调用 shared 包；M1 改为 worker queue 不破坏 shared 包接口（详见 design 文档）

#### F. QuestionStore（独立包 `src/services/shared/questionstore/`，**新增深模块**）

抽象掉"题目/试卷/知识点最终落到哪里"的差异。这是支撑模式开关的关键深模块。

- 接口：
  - `ListReferenceQuestions(ctx, knowledge_point_id, type?, difficulty?, limit) []Question`
  - `SaveQuestions(ctx, []QuestionDraft) (SaveResult{Created, Failed})` —— 支持部分失败
  - `ListKnowledgePoints(ctx, course_or_kb_scope) []KnowledgePoint`
  - `SavePaper(ctx, PaperSpec, []QuestionID, seed, name) (PaperID, error)`
  - `ListQuestionsForPaperPool(ctx, knowledge_point_ids, type?, difficulty?) []Question`
- 两个实现：
  - `examstore`（仅在部署级开关开启时构建）：调用 exam 后端 4 个新端点（见下文），鉴权走老师 OIDC token
  - `localstore`：直接读写 `kb_questions` / `kb_papers` / `kb_knowledge_points` 表
- 工厂：`questionstore.For(ctx, kb) QuestionStore` —— 按 `kb.integration_mode` 返回对应实现
- **接口对调用方稳定**：persona / `kb_*` 工具不知道也不关心底层是 exam 还是本地

#### G. RAG worker 工具（`src/services/worker/internal/tools/builtin/kb/`）

工具命名**避免 exam 前缀**——它们对两种模式通用，由 QuestionStore 决定走向。

- **`kb_search(kb_id, query, k=8, filters)`** → 返回检索 chunks。只读 ArkLoop 本地 KB，与模式无关。
- **`kb_list_knowledge_points(kb_id)`** → 内部走 `QuestionStore.ListKnowledgePoints`；Linked 走 exam，Standalone 走本地 `kb_knowledge_points`。
- **`kb_draft_questions(kb_id, knowledge_point_id, count<=5, type, difficulty, retrieval_k=12, reference_k=5)`** → 三步：
  1. `kb_search` 拉教材内容；
  2. `QuestionStore.ListReferenceQuestions` 拉参考题（few-shot + 去重黑名单）；
  3. 拼 prompt 调 LLM，解析返回 `[{stem, options, answer, explanation, source_chunk_ids, knowledge_point_id}]`。
  - **不入任何库**——只返给 persona 让老师预览
- **`kb_save_questions(kb_id, questions[])`** → 内部走 `QuestionStore.SaveQuestions`；返回 `{created, failed}` 支持部分失败。
- **`kb_compose_paper(kb_id, spec, seed?, name, knowledge_point_ids)`** → 内部：
  1. `QuestionStore.ListQuestionsForPaperPool` 拉题池；
  2. `papercompose.Compose` 纯函数抽样；
  3. 有 `ShortageWarning` 时返给 persona 决定补题/放宽；
  4. 老师确认题号列表后 `QuestionStore.SavePaper`；返回 `paper_id` + markdown 导出。
- 所有 `kb_*` 工具权限：Workspace 成员。
- **写后端是 persona 显式编排的步骤**——`kb_draft_questions` 永远不直接保存；只有 `kb_save_questions` / `kb_compose_paper` 才落库。

#### F'. 新增的 exam_* 工具（**仅在部署级开关开启时注册到 worker registry**）

`QuestionStore.examstore` 内部依赖；不直接暴露给 persona 工具列表（避免 persona 误用绕过 QuestionStore）：

- exam_list_questions / exam_create_questions_batch / exam_list_knowledge_points / exam_create_paper

或者更简洁：直接在 `examstore` 内用 HTTP client 调 exam 后端，**不走 worker 工具层**——工具层是给 LLM 用的，这里是工程间调用，没必要包一层。**最终设计取这条**，删掉上面 4 个工具的"工具"包装，让 `examstore` 直接调 exam REST。

#### G. PaperComposer（独立包 `src/services/shared/papercompose/`，纯函数）
- 接口：`Compose(spec PaperSpec, pool []Question, seed int64) (Paper, []ShortageWarning, error)`
- `PaperSpec` 字段：`total_count, type_distribution{type→count}, difficulty_distribution{level→ratio_or_count}, knowledge_point_distribution{kp→count}, allow_duplicate_kp, exclude_question_ids`
- `pool` 由调用方（`kb_compose_paper` 工具）从 exam 实时拉取；PaperComposer 本身不知道 exam 存在
- 算法：分层加权抽样 + 简单约束求解（先满足硬约束：题型/难度逐桶；剩余按知识点分布加权抽样）；题不够时返回 `ShortageWarning` 列表
- 纯函数：相同 (spec, pool, seed) 恒定输出 → 复现性、property test 友好

#### H. Persona / UI
- **新 persona `src/personas/book-tutor-agent/`**：`persona.yaml`（user_selectable=true、selector_name="智能组卷"）+ `prompt.md`，统一工作流（与模式无关，由工具自动分流）：
  1. 确认要使用的 KB；如果老师未指定，先调用 `kb_list_knowledge_bases` 列出当前可用且 ready 的课程资料知识库；
  2. 老师端不创建、上传、删除或重建 KB；复杂建设和目录维护由管理端完成；
  3. `kb_search` + `kb_list_knowledge_points` 把"教材内容"和"已有题样本"摆给老师看；
  4. `kb_draft_questions` 单批 ≤5 道生成草稿；
  5. `show_widget` 逐题预览，老师可改可删；
  6. 老师确认 → `kb_save_questions` 保存到"组卷题库"，错的题报回老师修；
  7. 累够题后组卷 → `kb_compose_paper` 先预览，老师确认后保存试卷；返回 markdown，老师需要 PDF 时再调用 `markdown_to_pdf` 导出副本。
- 复用现有 `show_widget / ask_user / markdown_to_pdf / end_reply` builtin tool
- **`exam-agent` 仅在部署级开关开启时注册**（`user_selectable` 由启动时根据 env 动态决定，或在 persona 加载阶段过滤）
- **Console-lite "知识库"管理页**：
  - list KB、查看 documents 状态、上传/删除文档/KB
  - 新建 KB 表单：模式单选（`Standalone` / `Linked to exam scope`），仅当部署级开关开启时显示 Linked 选项并要求选 `exam_scope_id`
  - Standalone KB 当前不提供题库浏览/编辑/删除、试卷列表/导出等管理 UI；这些能力暂缓
  - Linked KB 不做题库/试卷 UI——老师去 exam 前台看
  - 进度通过 polling `GET .../documents/:id` 实现（不引入新 SSE 通道）

### Option 2: 直接出题/组卷（命题技能 + 已有题样本）

> 这是本 PRD 在 M2 阶段并入的第二条独立流，**不依赖 KB**。主流程是"老师选命题技能 → 选课程 → 选种子题 → AI 按技能风格出新题 → 写回 exam → 组卷"。仅在 Linked 模式下提供。

#### O2-A. 新 persona `exam-builder-agent`

- 文件：`src/personas/exam-builder-agent/{persona.yaml, prompt.md}`
- `selector_name`："命题专家"；`selector_order=7`；`user_selectable` 由 `ARKLOOP_EXAM_INTEGRATION_ENABLED` 决定（沿用 `exam-agent` 的过滤模式）
- 工作流（每步在 prompt 里写死）：
  1. 列出 profile 已装的 `category=item-writing` skills；多个让老师选，单个直接用，0 个引导去装
  2. 用 `load_skill(skill_key)` 把 SKILL.md 注入对话历史
  3. `ask_user` 确认 exam 课程（候选可由 skill 的 `subject_tags` 与 exam 课程名做粗匹配建议）
  4. 老师说"在 X 知识点出 N 道 A2 题"——persona 调 `exam_list_seed_questions(knowledge_point_id, type?, difficulty?, pattern_tag?)` 列候选；老师勾选种子题
  5. 调 `exam_build_questions(seed_question_ids[], skill_key, count<=5)`；返回每道新题逐题 `show_widget` 给老师
  6. 老师 confirm → `exam_save_questions(questions[])` 写回 exam（Endpoint 3）；部分失败时持续展示具体 error_code / error_message
  7. 累够题后老师说"组卷"→ `exam_build_paper(course_id, spec, seed?, name)`；写回 exam Endpoint 4；同时给一份 markdown 副本

#### O2-B. 命题技能（item-writing skill）格式标准化

`skill.yaml` 在 ArkLoop 现有字段（`skill_key, version, display_name, description, instruction_path`）基础上，**新增以下字段**（仅 `category=item-writing` 类技能必填）：

```yaml
category: item-writing                    # 必填，标识技能用途
subject_tags: ["妇产科", "医学"]            # 多 skill 时筛选/课程匹配建议
pattern_tags: ["A1", "A2", "A3", "A4"]    # 该技能输出哪些 pattern_tag
target_question_types: ["single_choice"]  # 该技能产出哪种 question.type
```

**校验位置**：`shared/skillstore` 加载 skill 时校验 `category=item-writing` 必有上述字段，缺失则该 skill 不进 selector（普通 chat persona 仍可用，向后兼容）。

**首发 skill**：`src/skills/gyn-medical-exam/{SKILL.md, skill.yaml}`，由现有 `src/skills/exam-build/final_gyn_exam__skill_md.md` 重命名 + 补 yaml 而来。

#### O2-C. `pattern_tag` 字段全链路

- **exam 后端**：`questions` 表新增 `pattern_tag TEXT NULL` 列；`GET /api/questions` 接受 `pattern_tag` 作 query filter，返回值中带该字段；`POST /api/questions/batch` 接受 `pattern_tag` 作 per-item 字段（这部分通过扩 `docs/integrations/exam-api.md` 锁进 Spike S2 合约冻结，**和 M2a 同一份合约 PR 提交给 exam 团队**）
- **ArkLoop 端**：`questionstore.Question` 结构加 `PatternTag string`；`examstore` 透传；`exam_list_seed_questions` 把它作可选 filter；`exam_build_questions` 工具内**强校验**——LLM 返回的每道题 `pattern_tag` 必须等于对应种子题，否则该题进 SaveResult.Failed 并附 `pattern_tag_mismatch` 错码
- **prompt 软约束**：`exam_build_questions` 工具自己拼 LLM prompt 时，把"题型必须保持 X"写进硬约束（双保险，prompt 让 LLM 大概率合规，代码兜底防漂移）

#### O2-D. 4 个新 worker tools（与现有 `exam_*` 同前缀）

| 工具 | 说明 |
|------|------|
| `exam_list_seed_questions` | 内部走 `examstore.ListReferenceQuestions`，过滤 + 分页 |
| `exam_build_questions` | 调 LLM 出题，强校验 pattern_tag，**不写库**——返给 persona 让老师 confirm |
| `exam_save_questions` | 内部走 `examstore.SaveQuestions`（Endpoint 3），返回 SaveResult 含 created/failed |
| `exam_build_paper` | 内部组卷：拉 pool（`examstore.ListQuestionsForPaperPool`）→ `papercompose.Compose` 纯函数抽样 → ShortageWarning 时返给 persona → 老师 confirm 后 `examstore.SavePaper` |

工具注册同样受 `ARKLOOP_EXAM_INTEGRATION_ENABLED` 开关控制。

#### O2-E. 老师无种子题时的处理

由 `exam_build_questions` 工具拒绝（`seed_required` 错码），persona prompt 写明引导话术："该知识点下还没有可参考的种子题。请先用『题库助手』（exam-agent）录入几道，再回来用我做仿题。"——这是这条流的硬边界，**绝不 fallback 到无种子模式**（避免与 exam-agent 职责重叠）。

#### O2-F. console-lite 与 Option 2

- Persona selector 把 `exam-builder-agent` 列进去（条件：开关 on）
- "技能"管理页（如已有则复用；未有则单独一轮设计）：让老师能看到 / 装 / 卸 item-writing 类 skill；本 PRD 不强制要求"技能"页落地，作为下游优化项；底线：老师能通过 ClawHub 装 skill 即可
- 不引入新的"命题专家"主页——所有交互走 chat 即可

### 对 exam 系统的依赖（**仅 Linked 模式必需**）

本 PRD 的 standalone 模式**不依赖 exam**，可独立交付。Linked 模式实施前 exam 后端需开放以下端点（写入按老师 OIDC token 鉴权）：

| 端点 | 用途 |
|------|------|
| `GET /api/knowledge-points?exam_scope_id=` | 列知识点 |
| `GET /api/questions?knowledge_point_id=&type=&difficulty=&pattern_tag=&limit=&offset=` | 拉参考题 / 组卷题池 / Option 2 种子题筛选 |
| `POST /api/questions/batch` | 批量写入（支持部分失败；payload 含 `pattern_tag` 字段） |
| `POST /api/papers` | 写入组好的试卷 + 题目映射 |

**额外 schema 改动**（与 4 端点合约一并冻结）：exam 侧 `questions` 表新增 `pattern_tag TEXT NULL` 列，UI 是否暴露由 exam 团队决定（对 ArkLoop 不强制要求）。

> **里程碑建议**：
> - **M0（已交付）**：chunker + Doubao embedder + pgvector 一张表 + 2 个 debug HTTP endpoint，验证 ingest/search 链路通透，把 pgvector 上线/compose 改动压一次。详见 `docs/superpowers/specs/2026-05-21-book-kb-rag-design.md`。
> - **M1.0 / M1.1（已交付）**：完整 KB schema（含 `integration_mode` / `exam_scope_id` 字段） + PDF 摄入 + `kb_search` + console-lite KB 管理页 + book-tutor-agent 仅 KB 检索能力。
> - **M1.2 / M1.3（部分已交付，继续收尾）**：`kb_draft_questions` + 本地"组卷题库"保存 + `kb_compose_paper` 已打通；组卷约束、PDF 导出触发、来源快照自动补齐、linked 模式工具级端到端测试已补。继续补齐复杂 PaperComposer、草稿编辑体验和真实联调。Standalone 管理 UI 暂缓。
> - **M2a（部分已交付，继续收尾）**：部署级开关、KB `visibility`、mime 白名单、blob 引用计数、exam 合约、Linked KB 模式已落地；Linked KB 的老师端工具代理已打通。继续补齐真实联调与错误观测。
> - **M2b（部分已交付，继续收尾）**：`exam-builder-agent` persona、命题技能标准化、4 个新 `exam_*` 工具、`pattern_tag` 校验、`gyn-medical-exam` 首发 skill 已落地基础链路。继续补齐技能 selector 体验和端到端验收。
> - **M1 启动前必做的 spike**：S1 PDF 解析可行性（PyMuPDF + Tesseract 中文质量，已完成），S2 exam 后端合约对齐（含 `pattern_tag`，作为 M2a 启动前置）。
> - 各阶段对外接口稳定，M1→M2a 只是 QuestionStore 多一个实现 + UI 多一个选项；M2a→M2b 只是新增一个 persona + 4 个工具 + skill 元数据扩字段，无破坏性改动。

### 关键架构约束

- KB 资源通过 Workspace 路径鉴权（`workspace_ref` → `account_memberships`）；不另起 ACL
- 摄入流水线跑在 worker 队列里（M1 起），api 仅做触发和状态查询；不与 agent 推理路径共用 worker 实例（部署时可分组）
- Worker 通过 `worker/internal/data/kb_chunks_repo.go` 直连 PgBouncer 读 `kb_chunks`（沿用现有 `runs_repo` 模式，零额外 HTTP 跳数；schema 通过 `shared/database` 包跨服务同步管理）
- **Linked 模式题目/试卷数据零本地存储**——ArkLoop 不复制 exam 的真相源；Standalone 模式按产品边界写入 ArkLoop 本地 `组卷题库` / `kb_papers`
- 所有 KB 接口走 Gateway → API 路径，复用现有 rate limiting / risk scoring
- 大文件上传走 Workspace blob 存储（manifest + sha256 blob），不写 KB 表本身的 BLOB 列

## Testing Decisions

### 测试什么 & 怎么算好测试

只测外部行为，不测实现细节。一段测试好不好的标准：**改实现不改语义时不会红**。

### 必测模块

#### 1. Chunker（`shared/bookchunker/`）—— 单元测试
- 输入：黄金 `ParsedDoc` fixture（含 heading 嵌套、长段落、短段落、图片块、表格块）
- 断言：
  - 输出 chunk token 数都落在 `[min, max]` 内
  - heading 边界一定被切（不会出现两个章节被合到一块）
  - 图片/表格块独立成 chunk，不与文本合并
  - 相邻文本 chunk 有约定的 token 重叠
  - 每个 chunk 的 `heading_path` 真实反映在 Block 流中的位置
- 测试方式：表驱动 + 几个 golden fixture；纯函数无需 mock
- **Prior art**：`src/services/shared/messagecontent/` 这种纯转换模块的测试风格

#### 2. VectorStore（`api/internal/data/kb_vector_repo.go`）—— integration test（真 Postgres + pgvector）
- 跑真 PostgreSQL + pgvector（沿用现有 `*_integration_test.go` 模式，例如 `runs_repo_integration_test.go`）
- 覆盖：
  - Upsert 后能 Search 命中、按 cosine 排序正确
  - `SearchFilters` 各字段的过滤行为正确（document_ids、heading_prefix、chunk_type）
  - 删除文档后对应 chunks 不再被命中
  - 并发 upsert 不丢数据（小并发场景）
- **Prior art**：`runs_repo_integration_test.go`、`llm_routes_write_error_test.go`、`v1_runs_integration_test.go`

### 不在 v1 必测范围（可选）

- **DocumentParser**：依赖 sandbox + Python 二进制，单元测试成本高。v1 只写 1–2 个 smoke：固定一份小 PDF 跑通管线，断言 Block 数 > 0 且包含期望类型；详细 fixture 覆盖留到稳定后补。
- **PaperComposer**：用户选择不测；建议作为后续 quick-win 补 property test（同 seed 同输入恒定输出、所有硬约束被满足、shortage 报告正确）。
- **Embedder**：调外部 provider，单元层 mock 价值不大；写一个 integration smoke 跑通"文本→向量→维度正确"足矣。

### 端到端

- 需要保留至少一条老师主链路端到端测试。当前已在 worker 工具层覆盖 "source_chunk_ids → source_snippets 自动补齐 → 保存题目 → 实时拉题池 → 确认保存试卷"；后续真实部署 smoke 再补 "上传 PDF → ready → 检索命中 → kb_draft_questions → kb_save_questions → kb_compose_paper"。

### QuestionStore 与集成模式的测试边界

- **`QuestionStore` 接口对调用方就是测试桩**：worker 工具（`kb_draft_questions` / `kb_save_questions` / `kb_compose_paper`）的单元测试注入 fake `QuestionStore`，验证"编排正确"——不关心底层是 exam 还是本地。
- **`localstore` 实现**：跑 integration test 对接真 Postgres，验证读写、`SaveQuestions` 的部分失败、事务边界。和 VectorStore 测试一并跑。
- **`examstore` 实现**：单元测试用 httptest 起 mock exam server，验证：
  - 4 个端点的请求参数符合"对 exam 系统的依赖"表里冻结的字段约定
  - `POST /api/questions/batch` 返回部分失败时正确解析并回传 `SaveResult.Failed`
  - HTTP 5xx / OIDC token 过期等错误正确包装并冒泡
  - 真实 exam 后端的契约测试由 exam 项目负责，ArkLoop 不重复
- **部署级开关**：加一个轻量集成测试，验证 `ARKLOOP_EXAM_INTEGRATION_ENABLED=false` 时：
  - `examstore` 不被构建
  - book-tutor-agent 上传/出题/组卷链路完整可走（用 localstore）
  - `exam-agent` persona 不出现在 selector 列表
- **"部分失败"处理**（用户故事 23）：localstore 和 examstore 两个实现都必须覆盖。

### Option 2 (skill-driven item building) 测试边界

- **`exam_build_questions` 工具单元测试**：注入 fake `examstore` + fake LLM client，断言：
  - skill content 通过 `load_skill` 在工具内被读取并拼到 LLM prompt
  - LLM 返回每道题的 `pattern_tag` 与对应种子题的 `pattern_tag` 一致 → 接受；不一致 → 工具返回包含 `pattern_tag_mismatch` 的 per-item 错误，老师侧可见
  - 没有种子题（`seed_question_ids` 为空）时工具直接返 `seed_required` 错码，不调 LLM
- **`exam_list_seed_questions` / `exam_save_questions` / `exam_build_paper`**：薄壳工具，单元测试覆盖参数转 examstore 调用 + 错误冒泡即可，重复 examstore 自身测试无价值
- **skillstore 校验**：单元测试 `category=item-writing` 类 skill 必填字段缺失时加载失败、合法字段时正确解析 `pattern_tags` / `target_question_types`
- **`exam-builder-agent` persona "条件出现"**：startup test 验证 `ARKLOOP_EXAM_INTEGRATION_ENABLED=false` 时该 persona 不在 `/v1/personas` 列表（与 `exam-agent` 的开关一致行为）
- **gyn-medical-exam skill smoke**：装 skill → 启 persona → list seed → build → 验证产出 5 道题的 `pattern_tag` 全等于种子；不要求 LLM 输出"高质量"，只要求结构合规

## Out of Scope

- EPUB / PPT / Markdown / HTML 等其他文件格式（v2 再加）
- 跨 KB 检索（同时检索多个 KB）
- 多语言混合检索的优化（中英文混排可工作，但不专门调）
- **Linked KB 在 ArkLoop 端缓存 exam 题目/试卷**——Linked 模式下题/卷永远以 exam 为权威源，ArkLoop 不复制
- **Linked KB 的题目/试卷管理 UI**（列题、改题、删题、归档）——这些视图老师去 exam 前台用。Standalone KB 的题库浏览/编辑/删除、试卷列表/导出管理 UI 已暂缓（用户故事 46/47）
- 答案自动判分 / 学生作答记录（exam 系统的事）
- 与 exam 之间的双向同步（exam → ArkLoop 反向反查 KB 关联等）——本 PRD 只做单向写
- 在线协同编辑题目（先纯老师本地确认）
- 知识库版本管理 / diff（同名文件重传 = 覆盖旧版本，旧 chunk 删除）
- 私有 embedding 模型部署（先用 provider；后续 OpenViking 自部署）
- 替换 pgvector 为 Qdrant/Milvus（接口预留，但 v1 只交 pgvector）
- 把 RAG 出题能力反向迁移到 `exam-agent`（保持两 persona 解耦；后续可统一）
- 桌面端（Electron）独立 KB UI（先 console-lite 一处管理）
- 知识库的精细 ACL（除 Workspace 可见性外更细的角色）

## Further Notes

- **pgvector 依赖**：需要在 `compose.yaml` 的 postgres 镜像换成 `pgvector/pgvector:pg16`（或在现有镜像里 `apt install`/`CREATE EXTENSION`），并在 deploy 脚本里加上一次性确认。这是新外部依赖，需要在 PR 描述里显式标出来。
- **Sandbox 解析后端的 Python 依赖**：PyMuPDF (MuPDF AGPL 注意) / pdfplumber / python-docx / Pillow / pytesseract。若 AGPL 不可接受可换 `pypdfium2 + pdfminer.six`；这件事在实施 PR 里二选一即可，不影响 PRD 决策。
- **题目生成的 prompt 模板**：放在 `book-tutor-agent` persona 目录里作为独立 markdown（如 `gen_question_prompt.md`），便于和 persona 主 prompt 分离迭代。
- **节奏**：沿用现有 `exam-agent` 的"单批 ≤ 5 道 + 展示 + 询问继续"节奏；不要一次性长跑。
- **运维埋点**：每个摄入步骤写一行结构化日志（`kb_id, document_id, step, duration_ms, chunk_count, err`），与现有 api 日志格式一致。
- **复用约定**：DocumentParser、Chunker、Embedder、VectorStore、PaperComposer 五个深模块全部放在 `src/services/shared/` 下作为独立包，使其它服务（未来的桌面端、独立 worker）可直接 import 而不耦合 api 内部状态。
- **与 exam 后端的协作流程**（仅 Linked 模式 / M2 阶段）：拉一次 exam 后端的同事对齐"对 exam 系统的依赖"表里 4 个端点的字段细节（特别是 `POST /api/questions/batch` 的部分失败返回结构 和 `POST /api/papers` 是否需要预先创建 paper template）。字段约定冻结后写进 `docs/integrations/exam-api.md` 作为合约。
- **`examstore` 直接调 exam REST，不走 worker tool 层**：因为这是工程间调用而非 LLM 调用，没必要让 LLM 看见。worker tool 注册保持简洁：只有 5 个 `kb_*` 工具对老师可见。
- **现有 `exam-agent` 不动**：它现在的 4 个 `exam_*` 工具继续保留；这些工具与本 PRD 的 `examstore` 没有共用代码（exam-agent 走工具层是因为要给 LLM 调用；examstore 走 HTTP client 是工程间调用），不强行统一。
- **配置缺失的友好降级**：`ARKLOOP_EXAM_INTEGRATION_ENABLED=true` 但 `EXAM_BASE_URL` 未设置时，应用启动直接报错退出（按现有 config 校验风格）；运行时 examstore 调 exam 失败时，工具层返回结构化错误，persona 给老师可读提示（不静默降级到 standalone，避免老师以为题已写入 exam）。
- **去重策略**：`kb_draft_questions` 的"去重"只在 prompt 层做（把已有题题干列给 LLM 让它避开）。**不做向量比对自动去重**——召回准确度不够时会误杀，老师反而失控；保留"老师人眼最终判定"的回路。
- **数据流图（两种模式）**：
  - **共用前半段**：教材 PDF → KB chunks (pgvector) → `kb_search` 召回 → `QuestionStore.ListReferenceQuestions` 拉样本 → 拼 prompt → LLM 生成草稿（不写库） → 老师 `show_widget` 确认 → `QuestionStore.SaveQuestions` 落库
  - **Linked 模式**：QuestionStore 后半段 → exam REST → exam 数据库
  - **Standalone 模式**：QuestionStore 后半段 → `kb_questions` 表
  - 组卷同理：题池来源（exam vs 本地表）由 QuestionStore 决定，PaperComposer 纯函数完全无感。
