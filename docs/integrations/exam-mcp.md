# ArkLoop ↔ exam — MCP 集成契约（draft-v2）

> **Status**: **draft-v2**（exam 侧 stdio MCP 适配已可用于本地联调；ArkLoop 侧仍保留旧 REST provider 作为兼容回退）
> **Owner**: jzefan（ArkLoop 侧）+ exam 团队
> **Created**: 2026-05-31
> **Updated**: 2026-06-01
> **取代关系**: 逐步取代 `docs/integrations/exam-api.md`（REST 直连，frozen-v1）的**调用方式**；REST 端点本身作为 MCP Server 背后的实现保留，业务语义/错误码/数据形状沿用 exam-api.md。

## 1. 背景与目标

两个系统：
- **本系统（ArkLoop）**：智能体平台，扮演「外部智能体」。
- **exam**：传统考试软件平台，沉淀业务权限/组织隔离/版本控制/审计。

交互方式由「ArkLoop worker 直连 exam REST（OIDC 逐次铸 token）」改为「**经 MCP Server 接入**」：

```
外部智能体（ArkLoop persona / worker）
  └─> exam MCP Server                      ← exam 侧新建（适配进行中）
        └─> exam REST/OpenAPI 或内部 service
              └─> 业务权限 / 组织隔离 / 版本控制 / 审计
                    └─> PostgreSQL
```

目标：把传输层从 REST 直连换成 MCP，**业务语义不变**；身份/权限继续由 exam 在 MCP Server 之后强制执行。

## 2. ArkLoop 侧现状（已具备，无需新建）

ArkLoop 本身就是 MCP 客户端平台：

| 能力 | 位置 | 说明 |
|---|---|---|
| MCP 客户端 | `worker/internal/mcp/` | 支持 `stdio` / `streamable_http` / `http_sse`（JSON-RPC：Initialize/ListTools/CallTool）|
| 工具发现并注入 agent | `pipeline/mw_mcp_discovery.go` | 按 `account/profile/workspace` 从 DB 发现外部 MCP 工具，合并进工具集（带缓存）|
| 连接器配置 | `api/.../catalogapi/v1_mcp_configs.go`、`data/mcp_configs_repo.go`、`mcp_installs_repo.go` | 每账户配置 MCP server；console/web 有管理页 |
| 鉴权 | `shared/mcpinstall` `AuthPayload{Headers, Env}` | 每 install 一份加密 secret；stdio 可注入 env，HTTP 可注入 header |

**仍需关注**：如果同一个 MCP install 被多个教师共享，生产环境需要每 run 的教师身份动态注入或等价隔离（见 §4）。

## 3. 传输与工具

### 3.1 传输
- 本地开发推荐 **`stdio`**，直接启动 exam 后端里的 MCP 适配器。
- 生产部署可以切到 **`streamable_http`**，届时 exam MCP Server URL 走 ArkLoop `outboundurl` 安全策略校验（SSRF 防护，见 `mcp/safedialer.go`）。

本地启动命令：

```bash
cd /Users/jzefan/work/proj/exam/backend
EXAM_AGENT_BASE_URL=http://localhost:8000 \
EXAM_AGENT_TOKEN=<teacher-or-service-token> \
EXAM_AGENT_ORG_ID=<optional-org-id> \
PYTHONPATH=src uv run python -m app.job_models.agent_mcp
```

ArkLoop profile install 的 `launch_spec` 示例：

```json
{
  "transport": "stdio",
  "command": "uv",
  "args": ["run", "python", "-m", "app.job_models.agent_mcp"],
  "cwd": "/Users/jzefan/work/proj/exam/backend",
  "env": {
    "PYTHONPATH": "src",
    "EXAM_AGENT_BASE_URL": "http://localhost:8000"
  },
  "callTimeoutMs": 60000
}
```

敏感参数放 install secret 的 `env` 里：

```json
{
  "env": {
    "EXAM_AGENT_TOKEN": "<teacher-or-service-token>",
    "EXAM_AGENT_ORG_ID": "<optional-org-id>"
  }
}
```

### 3.2 工具映射（exam MCP Server 应暴露）

| 现 REST（exam-api.md）/ 内置工具 | → exam MCP 工具（当前名） | 语义保留 |
|---|---|---|
| `GET /api/knowledge-points` | `exam_list_knowledge_points` | 当前教师可见知识点；支持传 `exam_scope_id` |
| `GET /api/question-banks` | `exam_list_question_banks` | 当前教师可见题库 |
| `POST /api/question-banks/ensure-course-bank` | `exam_ensure_course_question_bank` | 固定课程题库 |
| `GET /api/questions` | `exam_list_questions` | 过滤知识点、type、difficulty |
| `POST /api/questions/bulk` | `exam_save_questions` | 保存老师确认后的 AI 题目 |
| `POST /api/papers` | `exam_create_paper` | name/spec/question_ids 转换为 exam 试卷 |
| （录题）catalog 识别/解析/建树/生成 | `exam.*`（第二阶段再迁） | 见 §6 phasing |

- 工具入参允许使用 ArkLoop 侧字段，MCP Server 会映射到 exam 当前后端字段。例如 `single_choice` / `multi_choice` 会映射为 exam 的 `choice`，`easy` / `medium` / `hard` 会映射为 `1` / `3` / `5`。
- ArkLoop 工具注册时会把 MCP 工具名加上 server id 前缀，例如 install key 为 `exam-agent` 时，LLM 看到的实际工具名可能是 `mcp__exam_agent__exam_list_questions`。persona 可以描述语义名，执行时以工具列表中的实际名称为准。
- MCP `CallTool` 的错误以结构化 `error_code` 返回，ArkLoop persona 原样转述给老师（"失败可读"原则）。

## 4. 身份与鉴权（关键契约点）

| | REST 直连（现状） | MCP（目标） |
|---|---|---|
| 身份来源 | ArkLoop `/internal/oauth/issue` 现铸 **60s 教师级** exam token，逐次调用 | exam MCP Server 必须知道「当前是哪个老师」|
| ArkLoop MCP 现状 | — | stdio install 可通过 secret `env.EXAM_AGENT_TOKEN` 注入 token；如果一个 install 给多教师共享，必须避免教师身份串号 |

**当前本地联调方案**：

1. ArkLoop profile install 使用 stdio 启动 exam MCP Server。
2. token 通过 install secret 的 `env.EXAM_AGENT_TOKEN` 注入 MCP 进程。
3. exam MCP Server 使用该 token 调 exam REST；权限、组织隔离、版本控制、审计仍由 exam 后端执行。

**多教师生产推荐方案（ArkLoop 侧承担，与 §2 缺口对应）**：

1. ArkLoop 仍用现有 `/internal/oauth/issue` 铸**短时教师 token**。
2. worker MCP 客户端在 **`CallTool` 时按当前 run 的教师身份动态注入** `Authorization: Bearer <teacher_token>`（**不**用 install 静态头、且对 exam 连接**绕开按 account 的发现缓存**，避免串号）。
3. exam MCP Server 校验该 token → 落到具体老师 → 在其 REST/OpenAPI 之后执行权限/隔离/版本/审计。

> 等价语义：身份与现在的 REST 直连完全一致，只是把"逐次带 token 的 REST 调用"换成"逐次带 token 的 MCP CallTool"。

**待 exam 团队确认（见 §7 Q1/Q2）**：token 类型（沿用现有 OIDC access_token？）、放在 `Authorization` 头还是自定义头、Server 是否做组织隔离的二次校验。

## 5. ArkLoop 侧工作项

> **审核结论（2026-06-01）**：本系统 MCP 客户端栈（stdio/streamable_http/http_sse + 发现 + 连接器 + env/header 加密凭据）**已完整支持**，连接 exam 的 stdio MCP Server **无需任何 Go 代码改动**——只差「注册一个 profile MCP install」这一步配置。

1. **把 exam MCP Server 注册为连接器** —— ✅ 已提供脚本 `scripts/dev/register-exam-mcp.sh`：单条 `POST /v1/mcp-installs`（`launch_spec` 启 stdio + 加密 `env_secrets` 注入 `EXAM_AGENT_TOKEN`）。`install_key=exam-agent` → 工具前缀 `mcp__exam_agent__`。worker 下次 run 自动发现并注入。
2. **persona 切到 MCP 工具** —— ✅ 已完成：`智能组卷(book-tutor)` / `录题助手(exam-agent)` / `命题专家(exam-builder)` 的 prompt 已改为「统一走 MCP、不绕过 REST」，并引用 `exam_list_knowledge_points / exam_list_questions / exam_ensure_course_question_bank / exam_save_questions / exam_create_paper`。
   - **存储正源 = Exam（设计决策 2026-06-01）**：题目/试卷的正源是 Exam 考试系统，老师确认后**默认经 MCP 保存到 Exam**（直接可组卷/考试，无需二次导入）。本地「组卷题库」(`kb_save_questions`/`kb_compose_paper`) **降级为兜底**——仅当平台未注入 exam MCP 工具（Exam 未接入）时使用，且 persona 会明确告知"已暂存本地，接入 Exam 后可用"。`智能组卷` prompt 的出题/组卷保存步骤已据此改写（exam 优先、本地兜底、诚实提示）。
   - 注意：因本地保存改为兜底，**本地与 Exam 不互通**——连上 Exam 前用本地库存的题不会自动出现在 Exam（如需迁移历史本地题，要单独的"本地→Exam 导出"动作，目前未接）。
3. **迁移期保留 REST 回退** —— 当前 `.env` `ARKLOOP_EXAM_INTEGRATION_ENABLED=true`，内置直连 `exam_*` 工具仍在（作回退）；persona prompt 已要求优先 MCP。完全切换后置为 `false` 即隐藏内置直连工具。
4. **多教师生产：per-run 教师身份注入** —— ✅ 已实现（ArkLoop 侧，2026-06-01）。worker 在**每次 CallTool 前**按当前 run 的老师 `userID` 经 `/internal/oauth/issue` 现铸 60s token，经 ctx 注入到对该 MCP server 的 `Authorization` 头（覆盖 install 静态头，仅当次请求生效）。代码：`worker/internal/mcp/peruser.go`（minter + ctx override + 懒加载配置）、`executor.go`（CallTool 前铸+注入）、`http_client.go`（`sendHTTP` 应用 override）。**仅 HTTP 传输生效**（stdio 的 token 在进程启动时定死）。
   - 启用方式（env，不配则不注入、退回静态头，安全）：
     - `ARKLOOP_MCP_OIDC_SERVERS=exam-agent`（需 per-user 注入的 server id，逗号分隔）
     - `ARKLOOP_MCP_OIDC_CLIENT_ID`（默认 `exam-web`）、`ARKLOOP_MCP_OIDC_SCOPES`（默认 `openid exam:read exam:write`）
     - 复用 `ARKLOOP_API_INTERNAL_URL` + `ARKLOOP_INTERNAL_SERVICE_TOKEN`
   - 单测：`worker/internal/mcp/peruser_test.go`（铸 token / sendHTTP 注入 / executor 注入 / 非白名单不注入，4 例全绿）。
   - **待 exam 侧配合**：exam MCP Server 需从"启动时单一 `EXAM_AGENT_TOKEN`"改为"每次请求校验传入的 Bearer（老师 token）"，并切到 `streamable_http` 传输。这是 §7 Q1–Q3 的落地。
5. **最终下线内置 REST 工具**（P3）—— 移除 `worker/.../tools/builtin/exam` 的 `exam_*` 直连 + kb provider `CallExam*`；`papercompose` 组卷逻辑保留在 ArkLoop 侧（与传输无关）。

### 5.1 连接步骤（本地联调）

```bash
# 1) 起 exam 后端（另一个仓库），确认 MCP 适配器可跑：见 §3.1
# 2) 注册 install（worker 主机需能执行 uv）：
ARKLOOP_API_URL=http://localhost:19001 \
ARKLOOP_TOKEN=<老师/管理员 access token> \
EXAM_BACKEND_DIR=/Users/jzefan/work/proj/exam/backend \
EXAM_AGENT_TOKEN=<exam 凭据> \
bash scripts/dev/register-exam-mcp.sh
# 3) 在 console/web 的 MCP 管理页确认 discovery_status=ok；
#    或直接用「智能组卷」persona 发起一次组卷，观察是否调用 mcp__exam_agent__* 工具。
```

## 6. 迁移分期（不破坏现状）

- **P0 契约**：本文档对齐 → frozen。
- **P1 ArkLoop 能力**：实现 §5.1 per-user MCP 鉴权 + 单测（exam Server 未就绪也能先做）。
- **P2 灰度**：feature flag 下 persona 走 exam MCP 工具；REST 直连作为回退保留。
- **P3 下线**：MCP 路验证通过后移除内置 `exam_*` 直连工具。
- 录题（catalog 系列）在 P2 之后再迁，先迁组卷相关（list_scopes/KP/questions、create_questions、create_paper）。

## 7. Open questions（待 exam 团队）

| # | 问题 | 默认/建议 |
|---|---|---|
| Q1 | 教师身份用什么凭据？沿用现有 OIDC access_token？ | 沿用 `/internal/oauth/issue` 铸的 token |
| Q2 | 放标准 `Authorization: Bearer` 头，还是自定义头（如 `X-Exam-Acting-User`）？ | 标准 `Authorization` 头 |
| Q3 | 传输：`streamable_http`？Server URL、是否需要 `Mcp-Session-Id`？ | `streamable_http` |
| Q4 | 工具命名空间/名称（`exam.list_questions` 还是 `exam_list_questions`）？ | `exam.<verb>_<noun>` |
| Q5 | 平台级管理读（国考题库，原 `CallExamAsAdmin`）在 MCP 下怎么暴露？ | 单独 admin-scope 工具或 service 凭据 |
| Q6 | partial-success / shortage / pattern_tag 数据形状沿用 exam-api.md？ | 是 |
| Q7 | 版本头（`X-ArkLoop-API-Version`）在 MCP 下如何承载？ | initialize 时声明 clientInfo + 工具版本 |

## 8. 参考
- REST 契约（被取代的调用方式，业务语义仍有效）：`docs/integrations/exam-api.md`
- ArkLoop MCP 客户端：`src/services/worker/internal/mcp/`、`src/services/shared/mcpinstall/`
- 当前直连实现（待下线）：`src/services/worker/internal/tools/builtin/exam/`、kb provider `CallExam*`
