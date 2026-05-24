# Design: book-kb-rag M2b — Option 2: 命题技能 + 已有题样本驱动出题（无 KB）

> Status: ready-for-plan
> Owner: jzefan
> Created: 2026-05-23
> Companion specs: [PRD](../../prd/book-kb-rag.md) · [exam-api contract](../../integrations/exam-api.md) · [M2a design](./2026-05-23-book-kb-rag-m2a-design.md)
> Depends on: M2a `examstore` real impl + `questionstore.Question.PatternTag` 字段 + Spike S2 freeze（含 `pattern_tag`）

## Context

PRD 在 M2 阶段并入第二条独立流 —— "Option 2: 命题技能 + 已有题样本"。**不依赖 KB**：靠老师装的"命题技能"（如《妇产科国考命题专家》）+ exam 已有题作为种子样本驱动 AI 出题。

为什么单独立 milestone：
- 与 Option 1（KB 出题）正交：用户故事 50-59 是独立流；走错 milestone 会让 KB 流被命题技能的强校验逻辑污染
- 强约束（绝不静默修正 pattern_tag、绝不无种子 fallback）需要专门的工具实现，与 KB 出题的"找上下文 + 自由生成"模式不同
- 仅 Linked 模式提供（Standalone 看不见）—— 实施完全依赖 M2a 的 `examstore` 落地

M2b 与 M2a **设计并行**、**实施串行**（M2b 等 M2a examstore PR merge）。

## 范围摘要

预计 6-8 工作日。

| 块 | 说明 |
| --- | --- |
| 命题技能格式标准化（item-writing skill 协议） | skill.yaml 新增 4 字段 + skillstore 加载校验 |
| 首发 skill `gyn-medical-exam` | 由 `src/skills/exam-build/final_gyn_exam__skill_md.md` 改造 |
| 4 个新 worker tool: `exam_list_seed_questions` / `exam_build_questions` / `exam_save_questions` / `exam_build_paper` | 注册受 `ARKLOOP_EXAM_INTEGRATION_ENABLED` 控制 |
| `pattern_tag` 字段全链路 | exam schema 已在 M2a Spike S2 加列；ArkLoop 端 questionstore.Question.PatternTag 已在 M2a 引入；M2b 把它接到 4 个新工具上 |
| 强校验：LLM 出的每道题 pattern_tag 必须 == 种子题 pattern_tag | 不一致直接 `pattern_tag_mismatch` 错误，绝不静默修正 |
| 无种子题硬拒绝 | `exam_build_questions` 工具检查 seed_question_ids 为空 → `seed_required` 错码，绝不 fallback |
| 新 persona `exam-builder-agent` | persona.yaml + prompt.md；selector "命题专家"；受开关过滤（与 exam-agent 一致） |
| `papercompose` reuse | 与 Option 1 共用现有 PRD §G 纯函数包；输入是 examstore 拉的 pool |
| console-lite | persona selector 已自动支持；技能管理页本 milestone 不强制（fallback：通过 ClawHub 装 skill 即可） |
| 测试 | 4 工具 unit + skill load 校验 + persona 条件出现 startup test + gyn-medical-exam smoke |

## 命题技能（item-writing skill）协议

### `skill.yaml` 新增字段

仅当 `category=item-writing` 时强校验：

```yaml
# 既有字段（不动）
skill_key: gyn-medical-exam
version: "1.0"
display_name: 妇产科国考命题专家
description: 按国家医学考试中心 (NMEC) 规范出妇产科 A1-A4 题，强调临床推理
instruction_path: SKILL.md

# 新增字段（item-writing skill 必填）
category: item-writing
subject_tags: ["妇产科", "医学", "执业医师"]
pattern_tags: ["A1", "A2", "A3", "A4"]
target_question_types: ["single_choice"]

# 可选
locale: zh-CN
recommended_temperature: 0.4  # 仅做提示，工具实际 temperature 由 LLM client 配置
```

**字段语义**：
- `category`: 枚举，目前仅 `item-writing`；未来可扩 `grading` / `analysis` 等
- `subject_tags`: 学科标签，用于 persona 选定 skill 后建议 exam 课程匹配（不强制）
- `pattern_tags`: 该 skill 能输出哪些 pattern_tag —— **exam_list_seed_questions 用它过滤候选；exam_build_questions 用它兜底校验（种子 pattern_tag 必须 ∈ 此集合）**
- `target_question_types`: 该 skill 仅出哪些 `question.type` —— exam_build_questions 强约束 LLM 输出的 type 必须 ∈ 此集合

### `skillstore` 校验

```go
// shared/skillstore/loader.go
func validateItemWritingSkill(sk Skill) error {
    if sk.Category != "item-writing" {
        return nil // 不是 item-writing skill，跳过
    }
    var missing []string
    if len(sk.SubjectTags) == 0 { missing = append(missing, "subject_tags") }
    if len(sk.PatternTags) == 0 { missing = append(missing, "pattern_tags") }
    if len(sk.TargetQuestionTypes) == 0 { missing = append(missing, "target_question_types") }
    if len(missing) > 0 {
        return fmt.Errorf("item-writing skill %q missing required fields: %v", sk.SkillKey, missing)
    }
    return nil
}
```

**加载失败行为**：
- 该 skill 不进 `exam-builder-agent` selector
- 但该 skill 仍可在普通 chat persona 通过 `load_skill` 调用（不破坏向后兼容；缺字段时仅"作为命题技能"功能下线）
- 日志 `skillstore.invalid_item_writing` 含 skill_key + missing 字段

### `Skill` struct 扩展

```go
// shared/skillstore/types.go
type Skill struct {
    SkillKey            string
    Version             string
    DisplayName         string
    Description         string
    InstructionPath     string
    // 新增：
    Category            string
    SubjectTags         []string
    PatternTags         []string
    TargetQuestionTypes []string
    Locale              string
    RecommendedTemperature *float64
}
```

加载 yaml 时 `yaml.Unmarshal` 直接映射；未声明字段默认零值（向后兼容）。

### 首发 skill `gyn-medical-exam`

由现有 `src/skills/exam-build/final_gyn_exam__skill_md.md` 改造：

```
src/skills/gyn-medical-exam/
├── SKILL.md           # 由 final_gyn_exam__skill_md.md 重命名（内容不变）
└── skill.yaml         # 新增（含上述新字段）
```

**操作**：
- `git mv src/skills/exam-build/final_gyn_exam__skill_md.md src/skills/gyn-medical-exam/SKILL.md`
- 删除空目录 `src/skills/exam-build/`（如无其他文件）
- 新建 `src/skills/gyn-medical-exam/skill.yaml`：

```yaml
skill_key: gyn-medical-exam
version: "1.0"
display_name: 妇产科国考命题专家
description: 按 NMEC 规范出妇产科 A1-A4 题，强调临床推理与决策能力
instruction_path: SKILL.md
category: item-writing
subject_tags: ["妇产科", "医学", "执业医师", "NMEC"]
pattern_tags: ["A1", "A2", "A3", "A4"]
target_question_types: ["single_choice"]
locale: zh-CN
recommended_temperature: 0.4
```

## `pattern_tag` 全链路

| 层 | 状态 | 备注 |
| --- | --- | --- |
| exam questions 表新增 `pattern_tag TEXT NULL` 列 | M2a Spike S2 已落 | exam 团队执行 |
| GET `/api/questions` 接受 `pattern_tag` 作 query filter | M2a Spike S2 已落 | examstore 透传 |
| POST `/api/questions/batch` 接受 per-item `pattern_tag` | M2a Spike S2 已落 | examstore 透传 |
| ArkLoop `questionstore.Question.PatternTag` 字段 | M2a 已引入 | 本 milestone 直接用 |
| `examstore.ListReferenceQuestions` 把 filter.PatternTag 透到 exam | M2a 已实现 | 本 milestone 直接用 |
| `exam_list_seed_questions` 工具暴露 pattern_tag filter | **M2b 新增** | LLM 可选传 |
| `exam_build_questions` 工具强校验 LLM 输出 pattern_tag == 种子 pattern_tag | **M2b 新增** | 不一致 → 该题进 SaveResult.Failed 含 `pattern_tag_mismatch` |
| `exam_save_questions` 透传 PatternTag 到 examstore.SaveQuestions | **M2b 新增** | 走 POST batch endpoint |
| Standalone KB / book-tutor-agent 是否用 PatternTag | **永远不用** | Option 2 仅 Linked，与 Option 1 流程互斥 |

### 强校验逻辑

```go
// worker/internal/tools/builtin/exam/build_questions.go
func validatePatternTags(seeds []Question, drafts []QuestionDraft) []SaveFailure {
    // 假设 drafts[i] 对应种子 seeds[i % len(seeds)]
    // 或者每道 draft 在 LLM prompt 中显式标注它仿的是哪道 seed（更可靠）
    var failures []SaveFailure
    for i, draft := range drafts {
        seed := matchSeed(draft, seeds) // 通过 prompt 编排的索引匹配
        if seed.PatternTag == "" {
            continue // 种子无 pattern_tag → 不校验（向后兼容历史题）
        }
        if draft.PatternTag != seed.PatternTag {
            failures = append(failures, SaveFailure{
                Index: i,
                Draft: draft,
                ErrorCode: "pattern_tag_mismatch",
                ErrorMessage: fmt.Sprintf(
                    "新题 pattern_tag=%q 与种子题 pattern_tag=%q 不一致；该 skill 要求保持一致",
                    draft.PatternTag, seed.PatternTag,
                ),
            })
        }
    }
    return failures
}
```

**关键**：失败的题**不调用 `examstore.SaveQuestions`**，直接返给 persona，persona 用 `show_widget` 展示不合规题让老师选"重生成"或"放过这道"。绝不进入 exam。

`matchSeed` 实现：在 LLM prompt 中要求模型按种子顺序出题，并在每道返回里携带 `seed_index` 字段（自然语言提示 + JSON 字段双保险）。如果 LLM 漏带 seed_index，fallback 是位置匹配（drafts[i] ↔ seeds[i]）。

## 4 个新 worker tools

工具注册受 `ARKLOOP_EXAM_INTEGRATION_ENABLED` 控制（与 M2a `exam_*` 4 旧工具同一守门逻辑）。

文件结构：

```
src/services/worker/internal/tools/builtin/examitem/
├── spec.go            # 4 工具的 AgentSpec + LlmSpec
├── list_seeds.go
├── build_questions.go
├── save_questions.go
├── build_paper.go
├── prompt.go          # LLM prompt 模板（含技能注入位）
└── *_test.go
```

**为何与现有 `exam/` 目录分开**：现有 `exam/` 是 exam-agent 的 4 工具（catalog 相关 + 旧 generate_questions），与 Option 2 职责不同；新目录 `examitem/` 更清晰，避免文件越长越乱。

### 1. `exam_list_seed_questions`

```json
{
  "name": "exam_list_seed_questions",
  "description": "List exam-side existing questions as candidate seeds for AI-generated questions. Filters by knowledge_point_id and optional pattern_tag/type/difficulty. Returns at most `limit` items.",
  "input_schema": {
    "type": "object",
    "properties": {
      "knowledge_point_id": {"type": "string"},
      "type": {"type": "string", "enum": ["single_choice", "multi_choice", "fill_in", "short_answer", "essay"]},
      "difficulty": {"type": "string", "enum": ["easy", "medium", "hard"]},
      "pattern_tag": {"type": "string", "description": "Filter by pattern tag (e.g. A1/A2/A3/A4)"},
      "limit": {"type": "integer", "minimum": 1, "maximum": 50, "default": 10},
      "offset": {"type": "integer", "minimum": 0, "default": 0}
    },
    "required": ["knowledge_point_id"]
  }
}
```

实现：薄壳，转发到 `examstore.ListReferenceQuestions(ctx, kpID, ListFilter{...})`。返回原样 + total。

### 2. `exam_build_questions`

```json
{
  "name": "exam_build_questions",
  "description": "Generate ≤5 new questions using the given item-writing skill, with seed questions as style anchors. Does NOT write to exam; returns drafts for teacher review. Hard requirements: seed_question_ids non-empty; new questions' pattern_tag must equal seed's pattern_tag.",
  "input_schema": {
    "type": "object",
    "properties": {
      "seed_question_ids": {"type": "array", "items": {"type": "string"}, "minItems": 1, "maxItems": 5},
      "skill_key": {"type": "string", "description": "Loaded item-writing skill key"},
      "knowledge_point_id": {"type": "string"},
      "count": {"type": "integer", "minimum": 1, "maximum": 5, "default": 3}
    },
    "required": ["seed_question_ids", "skill_key", "knowledge_point_id", "count"]
  }
}
```

实现流程：

```go
func (e *Executor) buildQuestions(ctx context.Context, args BuildArgs) (BuildResult, error) {
    // 1. seed check
    if len(args.SeedQuestionIDs) == 0 {
        return BuildResult{}, ToolError{Code: "seed_required", Message: "该知识点下还没有可参考的种子题。请先用『题库助手』(exam-agent) 录入几道，再回来用我做仿题。"}
    }

    // 2. load skill
    skill, err := e.skills.Load(ctx, args.SkillKey)
    if err != nil {
        return BuildResult{}, fmt.Errorf("load skill: %w", err)
    }
    if skill.Category != "item-writing" {
        return BuildResult{}, ToolError{Code: "skill_not_item_writing", Message: "选定的技能不是命题技能"}
    }

    // 3. fetch seeds from exam
    seeds, err := e.examstore.FetchQuestionsByIDs(ctx, args.SeedQuestionIDs)
    if err != nil { return BuildResult{}, err }
    if len(seeds) == 0 {
        return BuildResult{}, ToolError{Code: "seeds_not_found", Message: "种子题已被删除或无权访问"}
    }

    // 4. validate seed pattern_tag ∈ skill.PatternTags
    for _, s := range seeds {
        if s.PatternTag != "" && !contains(skill.PatternTags, s.PatternTag) {
            return BuildResult{}, ToolError{
                Code: "seed_pattern_not_supported",
                Message: fmt.Sprintf("种子题 %s 的 pattern_tag=%q 不在该技能支持的范围 %v 内", s.ID, s.PatternTag, skill.PatternTags),
            }
        }
    }

    // 5. build LLM prompt (含 skill SKILL.md 内容 + seeds JSON)
    prompt := buildPrompt(skill, seeds, args.Count)

    // 6. call LLM
    raw, err := e.llm.Generate(ctx, prompt)
    if err != nil { return BuildResult{}, err }

    // 7. parse LLM output → []QuestionDraft (含 PatternTag + SeedIndex)
    drafts, err := parseLLMOutput(raw)
    if err != nil { return BuildResult{}, fmt.Errorf("parse llm output: %w", err) }

    // 8. validate pattern_tag matches seed; type ∈ skill.TargetQuestionTypes
    validDrafts, failures := validateDrafts(drafts, seeds, skill)

    // 9. attach knowledge_point_id, created_by_source=ai
    for i := range validDrafts {
        validDrafts[i].KnowledgePointID = args.KnowledgePointID
        validDrafts[i].CreatedBySource = "ai"
    }

    return BuildResult{Drafts: validDrafts, Failures: failures}, nil
}
```

返回结构：

```go
type BuildResult struct {
    Drafts   []QuestionDraft // 通过校验、待老师确认
    Failures []SaveFailure   // 没通过校验，含 pattern_tag_mismatch / type_not_supported
}
```

**关键**：本工具**绝不写库**；只返 drafts + failures。Persona 用 `show_widget` 展示，老师 confirm 后才走 `exam_save_questions`。

#### LLM prompt 结构（`prompt.go`）

```
You are a question writer following the skill: {{skill.DisplayName}}.

# Skill instructions (CRITICAL — follow exactly)
{{skill_md_content}}

# Seed questions (style anchors — your output MUST mirror their pattern_tag, type, and rigor)
{{seeds_json}}

# Constraints (hard — violations will be rejected)
- For each seed, generate {{count_per_seed}} new question(s) that:
  - Same pattern_tag as the seed (DO NOT change pattern)
  - Same `type` as the seed (e.g. single_choice)
  - Same general difficulty band
  - Different clinical scenario / numbers — do NOT plagiarize the seed verbatim
- Return ONLY a JSON array with this shape:
  [{"seed_index": 0, "pattern_tag": "A2", "type": "single_choice", "difficulty": "medium",
    "stem": "...", "options": [{"key":"A","text":"..."}, ...], "answer": "B", "explanation": "..."}]
- No prose before or after the JSON.
```

`{{skill_md_content}}` 由工具加载 SKILL.md 注入；其余变量从 args 计算。

### 3. `exam_save_questions`

```json
{
  "name": "exam_save_questions",
  "description": "Persist teacher-confirmed question drafts to exam. Supports partial failure: returns created[] and failed[] with per-item error codes.",
  "input_schema": {
    "type": "object",
    "properties": {
      "questions": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "knowledge_point_id": {"type": "string"},
            "type": {"type": "string"},
            "difficulty": {"type": "string"},
            "stem": {"type": "string"},
            "options": {"type": "array"},
            "answer": {"type": "string"},
            "explanation": {"type": "string"},
            "pattern_tag": {"type": "string"}
          },
          "required": ["knowledge_point_id", "type", "stem", "answer"]
        },
        "minItems": 1, "maxItems": 10
      }
    },
    "required": ["questions"]
  }
}
```

实现：薄壳，转发到 `examstore.SaveQuestions(ctx, drafts)`，原样返回 `SaveResult`。

### 4. `exam_build_paper`

```json
{
  "name": "exam_build_paper",
  "description": "Compose a paper from exam's question pool using PaperComposer. Returns {paper_id, markdown_export} on success or ShortageWarnings if pool is insufficient.",
  "input_schema": {
    "type": "object",
    "properties": {
      "course_id": {"type": "string"},
      "name": {"type": "string"},
      "spec": {
        "type": "object",
        "properties": {
          "total_count": {"type": "integer"},
          "type_distribution": {"type": "object"},
          "difficulty_distribution": {"type": "object"},
          "knowledge_point_distribution": {"type": "object"},
          "allow_duplicate_kp": {"type": "boolean"},
          "exclude_question_ids": {"type": "array", "items": {"type": "string"}}
        },
        "required": ["total_count", "type_distribution"]
      },
      "knowledge_point_ids": {"type": "array", "items": {"type": "string"}},
      "seed": {"type": "integer"},
      "dry_run": {"type": "boolean", "description": "If true, return composed paper but skip write to exam"}
    },
    "required": ["course_id", "name", "spec", "knowledge_point_ids"]
  }
}
```

实现：
```
1. pool = examstore.ListQuestionsForPaperPool(ctx, kp_ids, filter)
2. paper, warnings, err = papercompose.Compose(spec, pool, seed)
3. if len(warnings) > 0 && !dry_run:
   return {warnings, no_paper_yet}  # 让 persona 决定补题/放宽再调一次
4. if dry_run: return {paper, markdown_export}, 不写
5. paperID = examstore.SavePaper(ctx, name, courseID, spec, paper.QuestionIDs, seed)
6. return {paper_id, markdown_export, warnings}
```

`papercompose` 包**与 Option 1 共用**（PRD §G 已定，M1.3 实现过；本 milestone 直接 import）。

### 工具注册（`builtin.go`）

```go
// 在既有 exam.NewExecutor 注册旁边：
if cfg.ExamIntegrationEnabled && examClient != nil {
    examItemExec := examitem.NewExecutor(examstore, skillstore, llmClient)
    register(examitem.ListSeedQuestionsAgentSpec, examitem.ListSeedQuestionsLlmSpec, examItemExec.ListSeedQuestions)
    register(examitem.BuildQuestionsAgentSpec, examitem.BuildQuestionsLlmSpec, examItemExec.BuildQuestions)
    register(examitem.SaveQuestionsAgentSpec, examitem.SaveQuestionsLlmSpec, examItemExec.SaveQuestions)
    register(examitem.BuildPaperAgentSpec, examitem.BuildPaperLlmSpec, examItemExec.BuildPaper)
} else {
    logger.Info("exam_build_questions / exam_save_questions / exam_list_seed_questions / exam_build_paper disabled by ARKLOOP_EXAM_INTEGRATION_ENABLED=false")
}
```

## `exam-builder-agent` persona

### 文件结构

```
src/personas/exam-builder-agent/
├── persona.yaml
└── prompt.md
```

### `persona.yaml`

```yaml
id: exam-builder-agent
version: "1"
title: 命题专家
description: 按命题技能 + 已有题样本生成新题、组卷写回 exam
soul_file: prompt.md
user_selectable: true   # 由 loader 按 ARKLOOP_EXAM_INTEGRATION_ENABLED 运行时改
selector_name: 命题专家
selector_order: 7
budgets:
    reasoning_iterations: 60
    temperature: 0.4
reasoning_mode: auto
prompt_cache_control: system_prompt
executor_type: agent.simple
```

### `prompt.md`（工作流 9 步）

```markdown
你是命题专家 persona，按命题技能（item-writing skill）+ exam 已有题作种子样本，
帮老师批量出新题并组卷写回 exam。

# 工作流（每步严格按顺序）

## 1. 列出已装命题技能

调 `list_skills(category="item-writing")` 列出 profile 已装的命题技能：
- 0 个 → ask_user "您还没装任何命题技能，请去 console-lite 技能页或 ClawHub 装一个再来"，然后 end_reply
- 1 个 → 自动选定
- 多个 → ask_user "请选择本次使用的命题技能"

## 2. 加载技能

调 `load_skill(skill_key=<上一步选定>)` 把 SKILL.md 注入对话历史。

## 3. 确认 exam 课程

ask_user "您希望在 exam 哪个课程下出题？"
- 候选可参考技能的 subject_tags 做粗匹配建议（可选）

## 4. 确认知识点

ask_user "请选择本次出题的知识点（exam 课程下的某个知识点）"
- 调 `exam_list_knowledge_points(course_id=<上一步>)` （此工具来自 M2a，本 persona reuse）
- show_widget 树状选择

## 5. 列出种子题

调 `exam_list_seed_questions(knowledge_point_id=<上一步>, pattern_tag?=, type?=, difficulty?=, limit=10)`：
- 0 道 → ask_user "该知识点下还没有种子题。请先用『题库助手』(exam-agent) 录入几道再回来。" → end_reply
- ≥1 道 → show_widget 让老师勾选（≥1，≤5）种子题

## 6. 生成新题

调 `exam_build_questions(seed_question_ids=<上一步>, skill_key=<#1>, knowledge_point_id=<#4>, count<=5)`：
- 工具返回 {drafts, failures}
- 若 failures 非空（pattern_tag_mismatch 等）：
  - show_widget 展示不合规题 + 错误原因，ask_user "[重新生成 / 放过这道]"
  - 用户选"重新生成" → 重新调本工具一次（最多 2 次重试，超过仍失败则 ask_user "建议调整种子题"）

## 7. 老师逐题预览

把 drafts 逐题用 show_widget 展示，每道含：
- stem / options / answer / explanation
- 提示：pattern_tag=<X>，与种子保持一致
- 操作：[确认] [修改] [删除]

修改后等老师 confirm；删除的题不进入下一步。

## 8. 写回 exam

调 `exam_save_questions(questions=<老师确认的>)`：
- 返回 {created, failed}
- 失败逐题给老师看错码 + msg，ask_user "[修正后重试 / 放弃这道]"

## 9. 累够题后组卷

ask_user "要继续出更多题还是组卷？"
- 继续 → 回 #4 或 #5
- 组卷 → ask_user 让老师说 spec（题型/难度/总数/知识点分布）
  - 调 `exam_build_paper(course_id, name, spec, knowledge_point_ids, seed?)`
  - 若返回 warnings（题不够） → ask_user "[补题 / 放宽约束]"
  - 成功 → show_widget 展示 paper 概要 + markdown 导出链接 → end_reply

# 硬规则

- 绝不在没有种子题时让 AI 自由发挥（工具会拒，不要绕过）
- 绝不静默修正 LLM 输出的 pattern_tag（工具会校验并报失败，按上面流程处理）
- 一次出题不超过 5 道
- 写回 exam 前必须经老师 confirm
```

### loader 过滤

`api/internal/personas/loader.go`：与 `exam-agent` 同样的开关过滤：

```go
if !cfg.ExamIntegrationEnabled {
    for i := range personas {
        if personas[i].ID == "exam-agent" || personas[i].ID == "exam-builder-agent" {
            personas[i].UserSelectable = false
        }
    }
}
```

## papercompose 复用

PRD §G 已定 papercompose 是纯函数包，在 M1.3 实现（Option 1 组卷）。M2b `exam_build_paper` 直接 `import "arkloop/services/shared/papercompose"`，输入 pool 由 examstore 拉取，**完全无改动**。

## console-lite

### Persona selector 自动支持

console-lite 的 persona selector 已经从 `/v1/personas` 拉列表 + 按 `user_selectable` 过滤；开关 on 时 `exam-builder-agent` 自动出现，**不需要前端改动**。

### 技能管理页

**本 milestone 不强制落地新技能管理页**。底线：

- 老师通过 ClawHub 装 skill（`src/skills/gyn-medical-exam/` 推到 marketplace；老师在 ClawHub 上点装 → profile 自动持有）
- 或运维侧手动把 skill 放到 deploy artifact 内
- 若 console-lite 既有技能页（M1.x 已落）能列已装 skill，本 milestone 顺手把 item-writing 类技能加个 badge 即可（可选）

### 不引入"命题专家"主页

所有交互走 chat persona；selector 选 "命题专家" 即进入 prompt.md 工作流。

## 测试

| 块 | 类型 | 覆盖点 |
| --- | --- | --- |
| skill.yaml 校验 | unit | item-writing 类 skill 缺字段 → 加载失败 + 日志；合法 → 正常解析；非 item-writing 类不校验新字段 |
| gyn-medical-exam smoke load | unit | 加载 → category=item-writing, pattern_tags=[A1..A4], target_question_types=[single_choice] |
| `exam_list_seed_questions` | unit (httptest exam) | 透传 pattern_tag / type / difficulty / limit / offset 到 GET /api/questions; 返回原样 |
| `exam_build_questions` —— seed_required | unit | seed_question_ids=[] → 立即返 seed_required，不调 LLM |
| `exam_build_questions` —— skill 校验 | unit | skill_key 不存在 / category!=item-writing → 错误 |
| `exam_build_questions` —— seed pattern ∈ skill.PatternTags | unit | seed.pattern_tag=A5 (skill 不支持) → seed_pattern_not_supported |
| `exam_build_questions` —— pattern_tag 强校验 | unit (fake LLM) | LLM 返 pattern_tag=A1 但 seed pattern_tag=A2 → failures 含 pattern_tag_mismatch；drafts 不含该题 |
| `exam_build_questions` —— type 强校验 | unit | LLM 返 type=multi_choice 但 skill.TargetQuestionTypes=[single_choice] → failures 含 type_not_supported |
| `exam_build_questions` —— skill content 注入 prompt | unit | 断言 LLM client 收到的 prompt 含 SKILL.md 部分内容 |
| `exam_save_questions` | unit (httptest exam) | 透传 PatternTag 到 POST /api/questions/batch；部分失败 → SaveResult.Failed 1:1 还原 |
| `exam_build_paper` | unit | 调 examstore.ListQuestionsForPaperPool + papercompose.Compose；warnings 时不写；dry_run=true 时不写；成功 → examstore.SavePaper |
| 工具注册开关 | startup | `ARKLOOP_EXAM_INTEGRATION_ENABLED=false` → 4 工具不在 worker registry 中；日志含 disabled 提示 |
| `exam-builder-agent` persona 条件出现 | integration | 开关 on → 在 `/v1/personas` 且 user_selectable=true；开关 off → user_selectable=false |
| gyn-medical-exam E2E smoke | tests/smoke | 装 skill → 起 persona → mock exam 返 2 道 A2 种子题 → mock LLM 返 5 道 A2 新题 → 工具校验全过 → exam_save_questions mock 成功 → 端到端串通 |

**LLM 输出"高质量"不在测试范围**（属于人评，不写自动断言）。

## 关键技术决策

1. **绝不静默修正 pattern_tag / type**：违规题进 failures，由 persona 展示给老师选"重生成/放过"；这是 PRD 故事 54 的硬约束
2. **绝不无种子 fallback**：`exam_build_questions` 在 seed_question_ids=[] 时硬拒，不调 LLM，避免与 exam-agent 的 `exam_generate_questions` 职责重叠
3. **新工具放 `examitem/` 而非 `exam/`**：与既有 4 个 catalog/generate 工具职责分离；目录小且聚焦
4. **examstore 完全 reuse M2a**：不在 M2b 重复实现 HTTP client，避免双真相源
5. **skillstore 校验失败仅下线"作为命题技能"**：保留普通 `load_skill` 能力，向后兼容
6. **seed 与 draft 的映射靠 LLM 自己标 `seed_index` + 位置 fallback**：双保险，简单可靠
7. **persona 工作流写死在 prompt.md 而非工具内编排**：PRD §H 现有 book-tutor-agent 同款做法，便于迭代；工具层只管原子能力
8. **不引入"技能管理页"为本 milestone 必要项**：通过 ClawHub 装 skill 是已有通路，技能页是 nice-to-have 留作 followup
9. **`Skill` struct 字段全部前向兼容**：旧 skill 不写新字段照常加载；只在 category=item-writing 时强校验
10. **首发 skill 改造而非新写**：复用 `final_gyn_exam__skill_md.md`，git mv 保留历史；避免内容重复
11. **`exam_build_paper` 复用 papercompose**：与 Option 1 完全共享纯函数；接 pool 来源不同而已

## 风险与缓解

| 风险 | 缓解 |
| --- | --- |
| LLM 频繁输出错误的 pattern_tag → 老师陷入"反复重生成"循环 | persona 限制 2 次重试；超过提示老师调整种子题；同时把失败率作为埋点（每 100 次 build_questions 中 pattern_tag_mismatch 占比） |
| exam 侧不接受 `pattern_tag` schema 改动 | M2a Spike S2 早做 fallback 决策 —— ArkLoop 侧持久化到影子表 `exam_question_patterns(exam_question_id, pattern_tag)`；examstore 在 List/Save 时本地合并；本设计章节标 "fallback"，启动 M2b 实施前确认是否需要走 fallback |
| 种子题质量差 → AI 仿出的新题质量也差 | 不试图自动判定"种子质量"；老师手动选种子，工具是中性传输；prompt 中提醒老师"种子决定输出风格" |
| 同一 skill 被多 persona session 并行 load | skill load 是只读，无副作用；并发安全 |
| skill.yaml 缺字段但 skill 已被多老师装 | 加载失败仅"作为命题技能下线"；persona 列表过滤；老师在 selector 上看不到该 skill；通过升级 skill 修复 |
| `exam_build_questions` LLM 输出非 JSON | parseLLMOutput 失败时返 tool error；重试 1 次（temperature 不同）；仍失败则报错给老师 |
| seed_question_ids 数量过多 (>5) → prompt 过长 | 工具 input_schema maxItems=5 强制；超过老师在 #5 步勾选时由 widget 限制 |
| pattern_tag 强校验导致几乎所有输出都被拒 | smoke test 时跑通至少 1 道；运行时通过埋点观察 mismatch 率；若 >50% 则触发 alert 让 prompt 工程介入 |
| Option 2 老师误用：以为 Standalone KB 也能用 | persona 仅在开关 on 时出现；selector 文案"（仅 Linked 部署可用）"补充；老师装 skill 但没 Linked KB 时进 persona 流程会卡在 #3 step（exam 课程不存在）；体验上限明确 |
| exam 团队对 4 端点不增 GET /api/questions/by-ids batch endpoint | M2b `exam_build_questions` 内调 `examstore.FetchQuestionsByIDs` 实现走多次 GET /api/questions?id=...（N 次串行或并发）；性能可接受（≤5 个 ID） |

## 不在 M2b 范围

- 命题技能编辑器 / 在线 IDE
- 技能版本管理 / diff
- skill 评分 / 推荐
- 跨课程的种子题（必须同 course_id 内）
- pattern_tag 之外的题型分类（如 Bloom 认知层级）—— 留 v3
- Option 2 与 Option 1 混合工作流（pre KB chunks + seeds 同时驱动）—— 当前 PRD 决定独立
- `exam_build_questions` 自带评分 / 自动改进循环
- Standalone KB 的命题技能能力
- `pattern_tag` 在 exam 前台 UI 是否展示（由 exam 团队决定，对 ArkLoop 不强制）

## 下一步

1. 走 `superpowers:writing-plans` 拆 plan（预计 7-9 tasks，命名 `2026-05-23-book-kb-rag-m2b.md`）
2. Plan 第 1 task：skillstore 加 4 字段 + item-writing 校验 + 单元测试
3. Plan 第 2 task：首发 skill 改造（git mv + 新建 skill.yaml + 加载验证）
4. Plan 第 3 task：worker `examitem/` 包骨架 + 4 工具 spec.go + 注册开关
5. Plan 第 4 task：`exam_list_seed_questions`（薄壳）+ httptest
6. Plan 第 5 task：`exam_build_questions` 工具核心实现（含 prompt.go + pattern_tag/type 强校验 + 错误路径全套单测）
7. Plan 第 6 task：`exam_save_questions`（薄壳）+ httptest
8. Plan 第 7 task：`exam_build_paper`（薄壳 + papercompose 接入）+ 单测
9. Plan 第 8 task：`exam-builder-agent` persona.yaml + prompt.md + loader 开关过滤
10. Plan 第 9 task：acceptance + gyn-medical-exam E2E smoke + 文档更新
