# Arkloop 架构审计报告

本文是对 Arkloop 当前架构的完整审计。以 Anthropic/Microsoft 级别的 AI SaaS 平台为参照系，逐层分析现有系统在面对 10 万-100 万用户规模时将遇到的结构性问题。

---

## 当前系统全貌

Arkloop 目前由四个部分组成：

- **API 服务**（Go）：承担 HTTP 路由、JWT 认证、SSE 推送、业务 CRUD。
- **Worker 服务**（Go）：消费 PostgreSQL 队列、执行 Agent 引擎、调用 LLM、管理 MCP 子进程。
- **Web 前端**（React + TypeScript）。
- **PostgreSQL**：唯一的持久化层，同时承担关系型存储、任务队列、事件流三重角色。

系统没有 Redis、没有对象存储、没有独立的 API Gateway、没有可观测性基础设施。

---

## 一、流量入口：没有 Gateway 层

当前 API 服务直接暴露给客户端，身份验证、限流、IP 过滤、请求路由全部耦合在业务代码里。这种设计在早期原型阶段是可以理解的，但在企业级场景下有根本性问题：

**API 和 Gateway 的扩展需求完全不同。** Gateway 是 stateless 的——它只做 token 验证、rate limit、IP 检查、请求转发，可以水平扩展到任意数量。API 服务是 stateful 的——它持有数据库连接池、维护 SSE 长连接、执行事务逻辑。把两者放在一个进程里，意味着你无法独立伸缩它们：限流压力大时你不得不连同业务逻辑一起扩副本，这是资源浪费。

更关键的是，Gateway 层是实现以下能力的唯一正确位置：
- **Rate Limiting**：当前系统没有任何限流机制，一个恶意客户端可以打满数据库连接池。
- **IP 过滤**：`ip_rules` 表即使建了，检查逻辑放在 API 服务里会绕过 WebSocket/SSE 等非标准路径。
- **API Key 验证**：API Key 验证应该在请求到达业务逻辑之前完成，不应该混在 handler 里。
- **请求审计**：每个入站请求的 IP、User-Agent、API Key ID 应该在 Gateway 层统一记录，而不是分散到各个 handler。

Anthropic 和 OpenAI 的公开 API 都有独立的 Gateway 层（通常基于 Cloudflare Workers、Envoy 或自研网关），负责 token 验证、限流、请求日志，业务 API 只处理到达的合法请求。

---

## 二、数据库设计：只够搭一个 demo

### 2.1 用户身份是残缺的

`users` 表只有三个业务字段：`id`、`display_name`、`created_at`。没有 email——这意味着无法做密码重置、无法发送通知、无法满足 GDPR 数据主体识别要求。没有 status——无法冻结/停用账户。没有 `deleted_at`——误删用户不可恢复。

`user_credentials` 表只支持密码登录（`login` + `password_hash`），没有为 OAuth、OIDC、SAML 预留扩展点。一个企业级产品至少需要支持 Google/GitHub OAuth 和企业 SSO，当前的表结构做不到这些。

### 2.2 组织模型过于简陋

`orgs` 表只有 `slug`、`name`、`created_at`。没有 `owner_user_id`（谁是账单责任人），没有 `status`（无法停用组织），没有 `settings_json`（无法存储 org 级配置如 MFA 强制、session timeout）。

更关键的问题是：**没有邀请机制**。`org_memberships` 只能通过直接 INSERT 创建，没有 `org_invitations` 表，没有邀请链接，没有邀请过期。用户根本无法通过正常流程加入一个组织。

### 2.3 消息表不支持多模态

`messages.content` 是 `TEXT NOT NULL`，无法存储图片引用、文件附件元数据、工具调用的结构化输入输出。在 2025 年，所有主流 AI 平台的消息格式都是结构化 JSON（Anthropic 的 `content` 是数组，OpenAI 的 `content` 支持 `image_url` 和 `input_audio`）。当前的纯文本字段将来改起来是破坏性的。

### 2.4 Run 表缺少关键生命周期字段

`runs` 表没有 `completed_at`、`failed_at`、`duration_ms`——无法查询一个 run 跑了多久。没有 `total_input_tokens`、`total_output_tokens`、`total_cost_usd`——要统计成本必须扫描所有 `run_events` 的 JSON。没有 `parent_run_id`——无法追踪子 Agent 调用链。没有 `model` 和 `skill_id` 快照——历史 run 无法知道当时用的什么模型和 skill。

`runs.status` 是 `TEXT` 且没有 CHECK 约束，Worker 只在创建时写入 `'running'`，之后从不更新。run 的实际状态只能通过扫描 `run_events` 来推断，`runs.status` 字段事实上是不可信的。

### 2.5 事件表有三个定时炸弹

**第一个：行级锁热点。** 每次 append 一个事件，都要 `UPDATE runs SET next_event_seq = next_event_seq + 1 WHERE id = $1` 来分配序号。流式输出时每秒对同一行做 10-20 次排他锁更新。1000 个并发 run 意味着每秒上万次行级锁争用，PostgreSQL 连接池会先崩。当前的 200ms 批提交只是延迟了问题爆发的时间点。

**第二个：无分区无归档。** 一次 2000 token 的回复产生约 150 个 `message.delta` 事件行。10 万用户每天 10 次对话 = 每天 1.5 亿行事件，全部堆在一张无分区的表里。半年后这张表的索引扫描性能会退化到不可用。

**第三个：没有成本字段抽列。** token 用量和费用埋在 `data_json` JSONB 里，无法在不解析 JSON 的情况下做聚合查询。这使得按量计费在 SQL 层面不可实现。

### 2.6 没有层级结构

`threads` 直挂在 `org_id` 下，没有 `projects`、没有 `teams`。组织内部无法对内容做分组，无法做部门级权限隔离。现有的 `database-architecture.zh-CN.md` spec 已经定义了 `org → team → project → thread` 的层级模型，但代码和 migration 中完全没有实现。

### 2.7 所有删除都是硬删除

`threads`、`messages`、`runs`、`users`——全部 ON DELETE CASCADE，误删后不可恢复。对于企业产品这是不可接受的。

---

## 三、Worker 与执行引擎

### 3.1 MCP 是单机 IDE 的设计，不是 SaaS 的设计

`mcp/config.go` 从单一环境变量读取 MCP 配置，`mcp/pool.go` 是纯内存连接池，MCP Server 是 Worker 的 stdio 子进程。这个设计只适合桌面客户端（Cursor、Claude Desktop），不适合 Web SaaS。

ChatGPT 和 Manus 的做法是：MCP 走远端 HTTP/SSE 协议（Remote MCP），配置存数据库，后端按需动态连接。当前的 stdio 绑定意味着：Worker 重启连接全丢，多用户无法各自配置不同的 MCP Server，横向扩展后 MCP 配置无法跨 Worker 共享。

### 3.2 Worker 调度是盲目的

`consumer/loop.go` 的 `SKIP LOCKED` 模式让任意 Worker 抢任意 Job。Worker 不注册自己的能力、不上报负载。如果某个 Job 需要特定 MCP Server 或特定 Sandbox，但被一个不具备这些条件的 Worker 拿走，它只会执行失败。横向扩展 Worker 后，任务失败率会上升而不是下降——这是反直觉的，也是致命的。

### 3.3 LLM 凭证锁死在环境变量里

`routing/config.go` 从 `ARKLOOP_PROVIDER_ROUTING_JSON` 环境变量加载。BYOK 的代码逻辑存在，但密钥来源只能是 Worker 进程所在机器的 env var。无法在运行时为某个 org 动态添加/轮换 API Key，无法在 Console 给用户一个"添加你的 API Key"的界面。

### 3.4 Skills 绑定在文件系统

`skills/loader.go` 从本地文件系统加载 skill 定义。多个 Worker 副本需要挂载同一个文件系统，无法在运行时修改 skill、无法做版本管理、无法做 A/B 测试、不同 org 无法有不同的 skill 配置。

### 3.5 整个"Agent 行为配置"层不存在

这是当前架构中最容易被忽视但影响最深远的缺失。数据库只管数据（用户、消息、事件），完全不管行为——Agent 怎么说话、用什么模型、温度多少、system prompt 是什么，这些全部要么硬编码在 Go 代码里，要么根本不存在。

**System Prompt：** 当前逻辑是：如果请求指定了 `skill_id`，Worker 从文件系统加载 skill 的 `prompt.md` 作为 system prompt；如果没有指定 skill，`systemPrompt` 是空字符串——Agent 就是一个裸的 LLM。数据库里没有任何地方可以配置、编辑、版本管理 system prompt。Console 无法提供"编辑 system prompt"的界面。

**模型参数：** `temperature`、`max_output_tokens`、`top_p` 全部没有持久化。当前代码里 `defaultAgentMaxIterations = 10` 硬编码在 `runengine/v1.go`，`threadMessageLimit = 200` 也硬编码。不同 org、不同 project、不同 thread 无法配置不同的参数。

**工具策略：** 只有 skill 级别的 `tool_allowlist`，没有 org 级或 project 级的工具策略。没有 tool denylist，没有"需要用户确认才能执行"的机制。

**内容安全策略：** 完全不存在。没有 content filter level，没有 safety rules。

**Prompt 模板：** 不存在。用户无法在 Console 创建/复用 prompt 模板。

这对于 Agent 平台来说是致命的——想象一下 Anthropic Console 或 OpenAI Playground 没有修改 system prompt 和 temperature 的能力。

---

## 四、实时推送链路是假的

表面上系统有"流式输出"——Worker 调用 LLM 时确实用了 `stream: true`，也确实逐 chunk 收到了 SSE 事件。但从 Worker 到浏览器的路径上有两段人为引入的延迟：

**第一段：Worker 200ms 批提交。** `eventWriter` 把 `message.delta` 这类流式事件攒在内存里的 pending transaction 中，满 20 条或超过 200ms 才 commit 到数据库。

**第二段：API 250ms 轮询。** SSE handler 每 250ms 查一次数据库看有没有新事件。`pg_notify` 存在于代码中，但只用于 run 取消信号，不用于事件推送。

最坏情况下一个 token 到达浏览器的额外延迟是 450ms。前端看到的"打字机效果"是一批批文字突然出现，而不是逐 token 平滑输出。

---

## 五、安全与合规基础缺失

### 5.1 没有 `secrets` 表

架构设计中多处引用了一个统一的 secrets 表（MCP auth token、LLM API key 的加密存储），但这个表从未创建。各处敏感数据将分散在不同表的不同字段里，加密策略会不一致，密钥轮换会变成噩梦。

### 5.2 审计日志缺少关键信息

`audit_logs` 没有 `ip_address`、`user_agent`、`api_key_id`、`before_state_json`/`after_state_json`。无法知道一个操作来自哪个 IP、是浏览器还是 API 客户端发起的、操作前后数据变化了什么。这在 SOC 2 / GDPR 审计中是不合格的。

### 5.3 没有并发限制

没有任何机制限制单个 org 同时跑多少个 run。一个恶意用户或失控的自动化脚本可以耗尽整个 Worker 集群的计算资源。

---

## 六、缺失的企业级能力

以下是当前系统中完全不存在、但企业级 AI SaaS 必须具备的能力：

| 能力 | 当前状态 | 影响 |
|---|---|---|
| API Key 管理 | 不存在 | 外部系统无法程序化接入 |
| RBAC 权限 | `role TEXT` 一个字段 | 无法表达细粒度权限 |
| Webhooks | 不存在 | 企业客户无法被异步通知 |
| 订阅与计费 | 不存在 | 无法商业化 |
| Feature Flags | 不存在 | 无法灰度发布 |
| 通知系统 | 不存在 | run 完成后关掉浏览器就收不到结果 |
| OpenTelemetry | trace_id 传递但无 Span 层级 | 出问题无法定位到具体层 |
| Agent Memory | 每次从最近 200 条消息重建上下文 | 无跨 thread 记忆，无 RAG |
| 数据导出/导入 | 不存在 | 无法迁移数据，无法满足数据可携权 |
| 多区域数据驻留 | 不存在 | 无法满足 GDPR 数据驻留要求 |

---

## 七、问题严重程度总览

### 致命（P0）——阻塞扩展，必须在重构中解决

- 没有独立 Gateway 层
- `users` 表没有 email
- `run_events` 序号分配的行级锁热点
- `run_events` 无分区无归档策略
- MCP stdio 进程绑定
- Worker 调度盲目
- 没有 Redis

### 严重（P1）——核心功能缺失

- `messages.content` 不支持多模态
- `runs` 缺少生命周期和成本字段
- `runs.status` 不可信
- 没有 org 邀请机制
- 没有 Webhooks
- SSE 双重延迟
- Skills 绑定文件系统
- LLM 凭证锁死 env var
- 没有软删除
- 审计日志缺少 IP 和变更状态

### 中等（P2）——规模化后逐渐暴露

- users/orgs 字段不完整
- 没有 projects/teams 层级
- RBAC 过于简陋
- 没有 secrets 统一管理
- 没有 Agent Memory / RAG
- 没有 OpenTelemetry
- 没有 Feature Flags
- 没有并发 Run 限制
- 没有通知系统
- messages 大内容无限存 DB
- 没有多区域数据驻留
