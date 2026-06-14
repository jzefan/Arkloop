# Task: 智能组卷 → 三步向导式交互（替换逐条提问）

## 背景
- "智能组卷"(`book-tutor-agent`) 当前纯靠提示词驱动：LLM 一轮问一个参数（总题数→范围→题型→难度→试卷名），体验差。
- 用户要求：选中"智能组卷"后给出**确定性 UI 向导**——①选知识库 ②选章节(支持全选) ③设 A1-A4 题型数量 + 整卷统一难度 → 开始生成 → 保存到考试系统。

## 决策（已与用户确认）
- 承载方式：**聊天内向导 widget**（复用现有 `ui_panel.widget_code` → `show_widget` → `sendPrompt` 回路）。
- A1-A4 难度：**整卷统一难度**（四题型各设数量，全卷共用一个难度）。

## 设计
- 新增 builtin 工具 `kb_paper_wizard`（单次调用，无必填参数）：列出 ready 知识库 + 每个库的章节(知识点)，烘焙进一个自包含多步 widget；最后一步按钮一次性 `sendPrompt` 结构化指令 `【智能组卷·开始生成】`。
- 三步全部客户端完成，零中途 LLM 提问。
- 生成/保存沿用现有路径（agent 收到指令后 `kb_draft_questions` 逐章节按题型生成 A1-A4，预览后存考试系统）。

## 步骤
- [x] 1. `rag_spec.go`：加 `ToolNamePaperWizard` + AgentSpec + LlmSpec
- [x] 2. `executor.go`：dispatch case
- [x] 3. 新建 `paper_wizard.go`：`executePaperWizard`（DB+provider 取数）+ 纯函数 `paperWizardWidget`（建 HTML）
- [x] 4. `builtin.go`：注册 Executors map + LlmSpec 列表 + AgentSpecs() 列表（默认 allowlist 据此放行）
- [x] 5. `book-tutor-agent/prompt.md`：改写组卷工作流走向导
- [x] 6. 新建 `paper_wizard_test.go`：纯函数 widget 单测（含 KB/章节/全选/A1-A4/sendPrompt 指令）
- [x] 7. `go build`(worker 全包) + builtin/runengine/pipeline 测试全绿；`gofmt` 干净；`go vet` 通过

## Review
**改动文件**
- `kb/rag_spec.go`：`kb_paper_wizard` 工具名 + AgentSpec + LlmSpec。
- `kb/executor.go`：dispatch case → `executePaperWizard`。
- `kb/paper_wizard.go`（新）：取数（ready KB + 章节，standalone 走本地表 / exam 走 provider）+ 纯函数 `paperWizardWidget` 烘焙自包含三步 widget。
- `kb/paper_wizard_test.go`（新）：3 个单测，全过。
- `builtin.go`：三处注册（Executors / LlmSpecs / AgentSpecs）。book-tutor-agent 无 tool_allowlist → 默认放行。
- `book-tutor-agent/prompt.md`：核心原则加"组卷走向导"，重写"工作流：组卷"为向导式，工具边界加 `kb_paper_wizard`。

**验证**
- 后端：build/vet/test 全绿（含遍历 AgentSpecs/LlmSpecs 构建 registry 的 runengine/pipeline 测试）。
- 前端交互：浏览器实跑三步——第1步选库、第2步全选3章节(已选3)、第3步 A1=5/A2=3(合计8)+难度切"难"，点"开始生成"发回的指令完全正确：
  `【智能组卷·开始生成】知识库：妇产科学｜kb_id=kb-1…题型数量：A1=5 A2=3 A3=0 A4=0（共8道…）难度：难…保存到考试系统。`

**遗留/边界**
- 生成与保存沿用现有 persona 路径（`kb_draft_questions` → 预览 → `exam_*` 存考试系统 / `kb_*` 兜底本地），未改写，保持 surgical。
- 章节按 KB 单次烘焙；exam 绑定库的章节走 provider，失败则该库章节为空并提示返回换库。teacher 账户 KB 量小，单次取数可接受。
