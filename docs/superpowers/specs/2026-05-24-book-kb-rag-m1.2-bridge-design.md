# Design: book-kb-rag M1.2-bridge — RAG 出题/组卷 + Standalone UI

> Status: ready-for-plan
> Owner: jzefan
> Created: 2026-05-24
> Companion specs: [PRD](../../prd/book-kb-rag.md) · [M1 decomposition](./2026-05-21-book-kb-rag-m1-decomposition-design.md) · [M2a design](./2026-05-23-book-kb-rag-m2a-design.md) · [M2b design](./2026-05-23-book-kb-rag-m2b-design.md)

## Context

M1.0 + M1.1 落地了 KB 创建 / 上传 / 解析 / kb_search；M2a 落地了 examstore + Linked KB 模式 + questionstore localstore + papercompose；M2b 落地了 exam-builder-agent。**但 PRD 核心承诺——"老师对 persona 说一句话生成题目并组卷"——还差最后一公里：4 个 RAG worker 工具 + book-tutor-agent 完整 prompt + Standalone UI**。

PRD 故事 18-30 与 46-47 当前**无法跑通**。`book-tutor-agent` persona.yaml 自述：「出题/组卷功能将在后续版本提供」。本 milestone 就是把这条尾巴接上。

## 范围

| 块 | 目标 | PRD 故事 |
|---|------|---------|
| A. 新增 kbapi 端点 | 给 Standalone 模式提供 KP/题/卷的 REST 接口（worker 工具 + console-lite UI 共用）| 36 |
| B. 新增 4 个 worker `kb_*` RAG 工具 | `kb_list_knowledge_points` / `kb_draft_questions` / `kb_save_questions` / `kb_compose_paper` | 18, 19, 22, 25, 26, 27, 28 |
| C. 重写 book-tutor-agent prompt | 完整工作流：搜索 → 列 KP → 草稿 → 老师 confirm → 写库 → 累够题后组卷 | 16, 17, 21, 23, 29, 30 |
| D. console-lite KB 详情页 tabs | 文档（既有） / 知识点 / 题库 / 试卷 | 46, 47 |

**不在范围**：
- Linked 模式下出题/组卷的 web UI（老师写回 exam 后去 exam 前台看，PRD §Out of scope）
- 真接入 exam staging 跑端到端（M2a 12 task 已经埋了 mock smoke；本 milestone 不要求）
- Standalone PDF 导出（M1.3 原范围；本 milestone 只交 markdown 导出，PDF 留下次）

## 架构关键决策

### 1. localstore 走 HTTP，不直接 worker→DB

PRD §关键架构约束写"worker 直连 PgBouncer 读 kb_chunks"，那是为了**重读**路径（kb_search 高频）。M1.2 的写路径（kb_save_questions 等）走 ArkLoop api HTTP——理由：

- `questionstore.localstore` 已经在 `src/services/api/internal/questionstore/localstore`（M2a-prereq B 决定的路径，因为 `data` 包是 internal，shared 无法导入）
- 让 worker 复用 localstore 需要要么搬代码，要么开 HTTP——HTTP 路径不会大幅影响出题速度（每分钟最多几次调用），代码改动最小
- 新 kbapi 端点同时被 console-lite UI 用，**省一份重复实现**（worker 用 HTTP，UI 用同 HTTP，事实上的"双消费者"）
- Linked 模式 worker 仍直调 examstore（M2a Task 9 已落），与 Standalone 模式对称

### 2. kb_draft_questions 是"准备 LLM 上下文 + 不写库"工具

参照 M2b `exam_build_questions` 的模式：工具自己**不生成题目**，而是返回一个 `{action, retrieval_hits, reference_questions, instruction}` 结构，让 persona 的 agent loop 在下一轮拼到 LLM 上下文里去生成。工具职责清晰：搜索 + 拉参考 + 拼指令。生成由 agent 自然完成。

这与"工具直接调 LLM"的设计相比，优势是：
- LLM 调用走 agent 主链路（统一计费、统一缓存、统一 thinking 控制）
- 工具单测不需要 mock LLM
- 老师能看到 `kb_search` 返的 hits 在中间步骤里（透明）

### 3. Standalone 与 Linked 在工具层透明分流

四个工具内部统一拿 `kb_id` → 查 `knowledge_bases` 获取 `integration_mode` → `questionstore.For(kb)` 返回对应实现 → 调接口。Persona prompt 完全不区分模式；只在向老师措辞时切换（"写入本地题库" vs "写入 exam"）。

### 4. console-lite 试卷 markdown 导出走前端拼装

Standalone 试卷的 markdown 在前端用题目 JSON + 试卷 spec 拼装即可（不依赖 markdown_to_pdf service）。简单字符串模板：
```markdown
# {paper.name}
> 出卷时间：{paper.created_at} · 总题数：{n}

## 一、单选题（10 道）
1. {stem}
   A. ...
   ...
   **答案：B**
   **解析：...**
```

需要 PDF 时复用既有 `markdown_to_pdf` worker tool（M1.3 可选交付项）。

## A — 新增 kbapi 端点

| Method + Path | 用途 | 调用方 |
|---|---|---|
| `GET /v1/knowledge-bases/:id/knowledge-points` | 列 KB 知识点 | worker `kb_list_knowledge_points` + UI 知识点 tab |
| `POST /v1/knowledge-bases/:id/knowledge-points` | 新增/批量新增 KP（Standalone 手动加标签 / Linked 从 exam 同步） | UI 知识点 tab |
| `PATCH /v1/knowledge-bases/:id/knowledge-points/:kp_id` | 改 KP 名/排序 | UI |
| `DELETE /v1/knowledge-bases/:id/knowledge-points/:kp_id` | 删 KP | UI |
| `GET /v1/knowledge-bases/:id/questions` | 列题库（filter: kp/type/difficulty + limit/offset） | worker `kb_draft_questions`（拉参考样本）+ UI 题库 tab |
| `POST /v1/knowledge-bases/:id/questions/batch` | 批量写题（partial-success） | worker `kb_save_questions` + UI（手动新增题，可选） |
| `PATCH /v1/knowledge-bases/:id/questions/:qid` | 改单题 | UI 题库 tab |
| `DELETE /v1/knowledge-bases/:id/questions/:qid` | 删单题 | UI |
| `GET /v1/knowledge-bases/:id/questions/pool` | 拉组卷题池（filter: kp_ids + type/difficulty） | worker `kb_compose_paper` |
| `GET /v1/knowledge-bases/:id/papers` | 列试卷 | UI 试卷 tab |
| `POST /v1/knowledge-bases/:id/papers` | 写新试卷（含 markdown 字段） | worker `kb_compose_paper` + UI 手动建卷（可选） |
| `GET /v1/knowledge-bases/:id/papers/:pid` | 试卷详情（含题列表） | UI 试卷 tab |
| `DELETE /v1/knowledge-bases/:id/papers/:pid` | 删试卷 | UI |

**所有端点的鉴权**：复用既有 `loadAuthorizedKB`（actor → workspace 成员 → visibility 过滤）。Standalone vs Linked 由 handler 内部按 `integration_mode` 分流：
- Standalone：调 `questionstore.For()` → localstore → Postgres
- Linked：上面 KP/questions 相关的 handler 应当返 409 + msg "Linked KB 的题/卷管理请在 exam 前台进行" —— 不允许 ArkLoop UI 改 Linked 模式题库（PRD §Out of scope "Linked KB 的题目/试卷管理 UI"）。**例外**：`POST /v1/knowledge-bases/:id/questions/batch` 在 Linked 下走 examstore.SaveQuestions（worker `kb_save_questions` 调）。

**handler 实现复用模式**：照 M2a Task 9/10 的 `handler_toc.go` 风格——单文件单 handler，依赖注入走 `handlerCtx`，错误用 `writeErr(w, code, "kb.xxx", msg)`。

## B — 4 个新 worker `kb_*` 工具

文件：`src/services/worker/internal/tools/builtin/kb/rag_executor.go`（新增）+ 复用既有 `rag_spec.go`（草稿已在 m2b 分支 untracked，本 milestone 取用并补完）。

### `kb_list_knowledge_points(kb_id)`

实现：
1. HTTP `GET {api}/v1/knowledge-bases/{kb_id}/knowledge-points` 透传 actor token
2. 返回 `{items: [{id, name, parent_id, depth, sort_order}]}` 给 LLM

### `kb_draft_questions(kb_id, knowledge_point_id, count, type?, difficulty?, retrieval_query?)`

实现（不写库，返 build context）：
1. 计算 retrieval_query（默认 = 知识点名字）→ 调 `kb_search(kb_id, query, k=8)` 拿 hits
2. HTTP `GET {api}/v1/knowledge-bases/{kb_id}/questions?knowledge_point_id={kp_id}&limit=5` 拿参考样本
3. 返回 `{action: "draft_questions", retrieval_hits: [...], reference_questions: [...], count, type, difficulty, instruction: "Use the retrieved KB content + reference questions to generate ≤count new questions for knowledge point X. ..."}`
4. **不调 LLM**，让 agent loop 在下一轮自然完成生成

prompt 在 persona 里写明生成约束：≥3 options for choice questions, 标 source_chunk_ids 引用 retrieval_hits 的 chunk_ref，写解析。

### `kb_save_questions(kb_id, questions[])`

实现：
1. HTTP `POST {api}/v1/knowledge-bases/{kb_id}/questions/batch` body=`{questions}`
2. 返回 `{created: [{index, id}], failed: [{index, error_code, error_message}]}` 透传

api 内部按 `kb.integration_mode` 分流：Standalone → localstore.SaveQuestions；Linked → examstore.SaveQuestions（已在 M2a Task 9 实现）。

### `kb_compose_paper(kb_id, name, knowledge_point_ids, total_count, type_distribution, difficulty_distribution, seed?)`

实现：
1. HTTP `GET {api}/v1/knowledge-bases/{kb_id}/questions/pool?kp_ids=a,b,c` → 拉题池
2. 调 `papercompose.Compose(spec, pool, seed)` 纯函数抽样
3. 若有 `ShortageWarnings` → 直接返给 persona（不写库）
4. 老师 confirm 后（持有 `question_ids` 列表）→ HTTP `POST {api}/v1/knowledge-bases/{kb_id}/papers` body=`{name, spec, seed, question_ids, markdown}` 写库
5. markdown 拼装也在 worker 端做（用 pool 里的题数据，按 paper 顺序排版）
6. 返回 `{paper_id, markdown}` 给 persona，persona 用 `show_widget` 给老师看 + 提供下载

> **简化**：本 milestone 把"列 pool → compose → 写卷"合并到一个工具，少一次工具往返。老师只看到一次 `kb_compose_paper` 调用就拿到结果（或 shortage 警告）。

## C — book-tutor-agent prompt 重写

参照 `exam-builder-agent/prompt.md` 的结构（角色 / 工作原则 / 启动流程 / 工作流：出题 / 工作流：组卷 / 失败处理 / 边界）。

关键工作流（出题段）：
```
1. 老师说"为 X 知识点出 N 道 A 题型"
2. 若不知道 KB，先 ask_user 让老师选 → 调 list_knowledge_bases (existing tool)
3. 调 kb_list_knowledge_points(kb_id) → 解析老师说的"X" 到具体 kp_id；多个候选 ask_user 让老师选
4. 调 kb_draft_questions(kb_id, kp_id, count=N, type, difficulty)
5. 工具返 retrieval_hits + reference_questions + instruction
   → 紧接着按 instruction 生成 N 道题（这一步由 agent loop 自然完成，输出 JSON 数组）
6. 逐题 show_widget 展示（题干 / 选项 / 答案 / 解析 / source: chunk_ref + retrieval_hit 原文）
   → 老师可改 / 删 / 确认
7. 老师确认后调 kb_save_questions(kb_id, questions=[...])
   → 部分失败时逐条展示 error_code + msg，让老师"修正后重试"或"放弃这道"
8. 问"要继续出题还是组卷？"
```

组卷段类似 M2b exam-builder-agent。

persona.yaml 改 description：去掉「出题/组卷功能将在后续版本提供」，改为「上传教材后用一句话生成题目和试卷，支持本地保存或写回 exam」。

## D — console-lite KB 详情页 tabs

`src/apps/console-lite/src/pages/KnowledgeBaseDetailPage.tsx` 当前是单页文档列表 + 搜索。重构成 tabs：

```
[KB Header (name + visibility + mode badge)]
[Tabs]
  📄 文档 (既有内容，427 行原代码移到 DocumentsTab.tsx)
  🏷️ 知识点
  ❓ 题库  (仅 Standalone 显示；Linked 显示"请到 exam 前台管理"占位)
  📝 试卷  (同上)
```

### 知识点 tab

- list KP（带 parent_id 缩进树）
- 「新增」按钮 → modal 输入 name + 选 parent + sort_order
- 行内编辑 name / 删除
- Linked 模式：禁用编辑，显示 readonly 树（数据来自 exam 同步快照）

### 题库 tab (Standalone only)

- 筛选条：知识点 / 题型 / 难度
- DataTable: 题号 / 题干前 80 字 / 题型 / 难度 / 知识点 / 操作（查看 / 编辑 / 删除）
- 「编辑」打开 modal：题干（textarea） + options（动态行 ≥3） + answer（单选 / 多选 / 文本） + explanation
- 「批量删除」（多选 + 确认）
- 分页

### 试卷 tab (Standalone only)

- DataTable: 试卷名 / 题数 / 创建时间 / 操作（详情 / 导出 markdown / 删除）
- 详情 modal: 显示 spec + 题列表
- 「导出 markdown」直接下载 .md 文件（用 paper.markdown 字段，新窗口或 blob URL）
- PDF 导出留 TODO 占位（v2 接 markdown_to_pdf service）

### Linked 模式占位

Linked KB 在题库/试卷 tab 显示：
```
本知识库绑定 exam 范围《X》，题库与试卷由 exam 系统管理。
[去 exam 前台查看 →]
```

## E — 测试边界

- **api 端点**：handler 单测（mock store） + integration 测试（真 PG，复用 M2a-prereq A 的 fixture pattern）
- **worker 工具**：单测注入 fake httpClient + fake kb_search，断言 HTTP 调用形态 + 返回 mapping。不必跑真 LLM
- **book-tutor-agent prompt**：不写自动断言（人评 / 后续 smoke）
- **console-lite**：type-check + lint + build 全绿；可手动跑一遍 happy path

## 风险与缓解

| 风险 | 缓解 |
| --- | --- |
| api 端点数量翻倍（13 个新端点） | 路由命名一致，handler 文件按资源拆分（handler_kp.go / handler_question.go / handler_paper.go），review 友好 |
| Standalone 老师在 UI 改题但 worker 也在改同一题 | 题库表带 updated_at；UI 提交时带 If-Unmodified-Since 或 version → 冲突 409。本 milestone 简化：last-write-wins，不上乐观锁 |
| Linked 模式下 UI 题库 tab 误编辑 | handler 拒绝 + UI 直接显示占位，不渲染编辑 UI |
| LLM 生成的题不带 source_chunk_ids 或 chunk_ref | prompt 强调 + persona 在 step 6 不接受没 source 的题；不强校验工具层（避免 LLM 卡死） |
| paper.markdown 字段大（极端 100 题 ×几 KB = 几百 KB） | JSONB/text 都能装；DB 不限；列表查询时不返 markdown 字段（只在详情接口返） |
| 另一个 thread 的 exam-api-proxy WIP 与本 plan 冲突 | 本 plan 不动 exam-api-proxy 涉及的文件（handler_kb.go / register.go / exam_scopes_lister.go 等）；新端点用 handler_kp.go / handler_question.go / handler_paper.go 独立文件 |

## 不在 milestone 范围

- PDF 导出（用既有 markdown_to_pdf 串起来，留下个 PR）
- KB 题库的乐观锁
- KP 的 Linked 模式自动同步（Linked KB 的 KP tab 显示 exam 数据快照，刷新按钮触发同步——可选）
- 老师自建题（非 AI 生成）的"新增题"UI（题库 tab 暂不开放手动新增按钮；可后续加）
- 题库/试卷的批量操作（批量改难度等）
- 题库的全文搜索（按题干文本搜）

## 下一步

走 `superpowers:writing-plans` 拆 plan（预计 9-10 task），命名 `tasks/2026-05-24-book-kb-rag-m1.2-bridge.md`。
