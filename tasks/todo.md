# Task: 抽出 papercompose 共享包 → 接通 exam_build_paper（审计项 #2 / A1+B1）

## 背景
- 设计文档（PRD §G / m2b）要求 kb 和 exam 两条组卷路共用一个纯函数组卷器 `papercompose`，但 `src/services/shared/papercompose/` 是**空目录**。
- 后果：
  - `exam_build_paper` 是桩（`builder_executor.go:326` 取前 N 道，忽略 type/difficulty/seed）。
  - kb 路自带私有 `selectQuestions`/`greedySelectQuestionRows`（`rag_executor.go`），与 exam 路逻辑分叉、重复。

## 目标（成功标准）
1. 新建纯函数包 `arkloop/services/shared/papercompose`：`Compose(pool, spec, seed) -> (selected, shortages)`，storage-agnostic（string ID）。
2. kb 路 `selectQuestions` 改为**薄委托**到 papercompose —— 行为不变，现有 kb 测试全绿。
3. exam 路 `exam_build_paper` 接 papercompose —— 真正按 type/difficulty 分布 + seed 抽样，题不足返回 shortage_warnings（不再取前 N）。
4. 新增单测：papercompose 包 + exam build_paper（分布命中 / shortage）。
5. `go test ./...` worker + shared 全绿；`go vet` 通过。

## 设计
- `papercompose.Question{ID, Type, Difficulty, KnowledgePointID string}`
- `papercompose.Spec{Total int; TypeDist, DifficultyDist, KPDist map[string]int}`
- `papercompose.Shortage{Dimension, Value string; Requested, Available int; Message string}`
- `Compose`：排序(ID)→seed 洗牌→分布校验(shortages)→总量校验→贪心选择→选后复校验。纯函数。
- kb 适配器：questionRow ↔ papercompose.Question 互转；Shortage → 现有 `map[string]any` 警告形状（保持 `warnings[0]["type"]` 等键），保证旧测试不变。
- exam 适配器：pool `map[string]any` → papercompose.Question；Compose；选中 ID → `POST /api/papers`；shortages → `shortage_warnings`。
- **零 go.mod 改动**（同属 shared 模块）。

## 步骤
- [x] 1. TDD：写 `papercompose/papercompose_test.go`（分布命中 / seed 确定性 / type·difficulty·kp·总量 shortage / 过约束 / 不可变输入）
- [x] 2. 实现 `papercompose/papercompose.go`（移植算法 + 导出 `ShortagesToMaps`），包内测试绿
- [x] 3. 重构 kb `selectQuestions` 委托到 papercompose，删除已迁移的私有 helper（greedy/validateDistributions/count/copyIntMap/sumPositive/unmetScore/decrementIfNeeded + distributionSpec/Set）
- [x] 4. kb 测试全绿（TestSelectQuestions* 未改即过）
- [x] 5. 重构 exam `buildPaper` 接 papercompose（删除 take-first-N stub + 手动 total 检查）
- [x] 6. exam 新增 TestBuildPaper_HonorsDistribution / TestBuildPaper_ReturnsShortageWithoutSaving
- [x] 7. 测试 + vet + 全模块 build 全绿

## Review
**结果：完成，全部验证通过。**

改动文件（surgical，未触碰工作区里其它历史未提交改动）：
- 新增 `src/services/shared/papercompose/{papercompose.go,papercompose_test.go}` —— 纯函数组卷器（storage-agnostic，string ID，不可变输入，seed 确定性 splitmix64 洗牌）+ 导出 `Compose` / `ShortagesToMaps`。
- `kb/rag_executor.go`：`selectQuestions` 改为薄委托（-178 行旧算法，questionRow↔Question 互转，复用 `papercompose.ShortagesToMaps` 保持旧警告形状）。移除 `math/rand`、`sort` 依赖。
- `exam/builder_executor.go`：`buildPaper` 由"取前 N 道"桩 → 真正 `papercompose.Compose`（按 type/difficulty 分布 + seed 抽样；shortage 不写 exam；新增 `asStr`/`argDistribution` 小工具）。

验证证据：
- `go test` papercompose ✓ / kb ✓（旧 TestSelectQuestions* 未改即绿，证明行为保持）/ exam ✓（新分布命中 + shortage 不保存）
- `go vet` kb+exam ✓ / shared ✓；`go build ./...`（worker 全模块）✓；`gofmt -l` 干净。

效果：消除 A1（exam 桩实现）+ B1（两路算法分叉）。exam 路与 kb 路现共用同一组卷器，exam 组卷真正遵守题型/难度分布与 seed，并返回结构化 shortage_warnings。

未覆盖（留后续，超出本项范围）：
- exam 侧 `knowledge_point_distribution` 入参（当前 BuildPaper spec 只暴露 type/difficulty；KPDist 已在 papercompose 支持，加一个入参即可启用）。
- A2（book-tutor 在 Linked 模式是否写回 exam）属独立问题，未在本次处理。
- 贪心算法的联合可行性上限（B2）保持现状。

---

# Task: A2 — 打通 book-tutor（智能组卷）Linked 模式发布链路

## 背景
- 已上线代码有意把 book-tutor 设计成"本地优先"：`kb_compose_paper` 无条件写本地 `kb_papers`，注释 "Linked KBs do not write to exam"。
- 但 `questionstore.For(kb)` 工厂 + `examstore`（已实现 `ListQuestionsForPaperPool`/`CreatePaper`）+ PRD 都指向：mode=exam（Linked）应写回 exam。worker 路从未接上。

## 决策（依据 in-repo 证据，未阻塞提问）
- Linked KB（`integration_mode=="exam"`）：`kb_compose_paper` 从 exam 拉池 → papercompose 组卷 → 以教师 per-user OIDC 发布到 exam `/api/papers`，考生在 exam 前台可见。
- Standalone KB：保持本地 `kb_papers` 不变。

## 实现
- `rag_executor.go`：
  - `executeComposePaper` 顶部加 Linked 分支 → `composePaperLinked`；订正旧注释。
  - `composePaperLinked`（编排）：校验 → per-user `CallExam GET /api/questions` 拉池 → `composeFromExamPool` → shortage 不写；`confirmed=false` 只预览；`confirmed=true` → `CallExam POST /api/papers` 发布。
  - `composeFromExamPool`（纯函数，无 DB/网络）：exam 题 map → `papercompose.Compose` → 选中 ids + 预览 rows + shortages。
  - `examQuestionToRow`：exam 题 map → questionRow，复用既有 `renderPaperMarkdown` / `paperPreviewPanel`。
- 复用既有 `ProviderClient.CallExam`（per-user OIDC），零新增 plumbing。

## 验证
- 新增 3 个 DB-free 单测：分布命中 / shortage / exam题→markdown 渲染。
- `go test` kb ✓ exam ✓ papercompose ✓；`go vet` kb ✓；`go build ./...` ✓；`gofmt -l` 干净。

## Review
完成。现在三条状态：
- 命题专家 `exam_build_paper`：发布到 exam（分布+seed 正确）✓
- book-tutor Linked `kb_compose_paper`：发布到 exam ✓（本次）
- book-tutor Standalone：本地 `kb_papers` ✓（不变）
即"分布达标的试卷 → 发布到 exam 给考生"链路已打通（A2 关闭）。

未覆盖（留后续）：Linked 下 `kb_save_questions` 仍写本地题库（compose 改为从 exam 拉池，二者题库来源不同——若要 book-tutor 在 Linked 下端到端用 exam 题库，需把 save 也路由到 examstore，属更大改动，本次未做）。
