# Arkloop Open Source Readiness Roadmap

本文是面向开源发布的统一路线图。整合现有三份 roadmap（development-roadmap、architecture-refactor-roadmap、agent-system-roadmap）中**尚未完成的工作**，并新增架构治理、代码共享、插件体系、基础设施建设四个维度。

关联文档（历史参考）：
- `src/docs/roadmap/development-roadmap.zh-CN.md` — 已归档，不再新增内容
- `src/docs/roadmap/architecture-refactor-roadmap.zh-CN.md` — 已归档，不再新增内容
- `src/docs/roadmap/agent-system-roadmap.zh-CN.md` — 已归档，不再新增内容
- `src/docs/architecture/architecture-design-v2.zh-CN.md` — 目标架构参考
- `src/docs/architecture/architecture-problems.zh-CN.md` — 架构审计报告

---

## 0. 当前系统基线

### 0.1 已交付能力

**基础设施层**：PostgreSQL + PgBouncer + Redis + MinIO + Gateway（Go reverse proxy），compose.yaml 完整编排。

**API 服务**：JWT 双 Token 认证、RBAC、Teams/Projects、Org 邀请、API Key 管理、IP 过滤、Rate Limiting、SSE 推送、run_events 月分区、Feature Flags、邀请码/兑换码/积分体系、Webhooks、Entitlements/Plans。

**Worker 执行引擎**：Pipeline 中间件链、Executor 注册表（SimpleExecutor / InteractiveExecutor / ClassifyRouteExecutor / LuaExecutor）、Skills（YAML + DB 双源）、MCP 连接池、Provider 路由（when 条件匹配 + default fallback）、Human-in-the-loop（WaitForInput + input_requested）、Sub-agent Spawning（parent_run_id + spawn_agent tool）、Memory System（OpenViking 适配 + memory_search/read/write/forget tool）、Cost Budget 追踪（RunContext.ToolBudget 预留，执行侧未强制）。

**独立服务**：Sandbox（Firecracker microVM + Warm Pool + Snapshot + MinIO 持久化）、Browser Service（Playwright + Session Manager + BrowserPool）、OpenViking（Python HTTP 记忆服务）。

**前端**：Web App（React + Vite + TypeScript）、Console（React 管理后台，含运营/配置/集成/安全/组织/计费/平台八大模块）、CLI（参考客户端）。

### 0.2 核心问题

以下是当前系统在开源准备度上的结构性缺陷：

**P1 -- 配置管理无统一抽象**

三种配置读取路径并行（ENV 直读、platform_settings DB 查询、文件读取），无统一的 `config.Resolve(key, scope)` 接口。每个工具自建 `config_db.go`（email、web_search、web_fetch 三份几乎相同的代码），新增配置点 = 复制粘贴。硬编码的 magic number 散落在 20+ 个文件中，Console 无法调整。

**P2 -- Scope 解析不一致**

Agent Config 走 thread -> project -> org -> platform 四级解析；ASR Credentials 走 org -> platform 两级；web_search/email 只读 platform_settings 不区分 org；browser/sandbox 构造函数注入无动态解析。同一系统内 scope 解析有四种写法，新模块无从参考。

**P3 -- Tool Provider 管理缺失**

web_search 的 Tavily/SearXNG、web_fetch 的 Jina/Firecrawl/Basic 是硬编码的后端切换逻辑。无 per-org Provider 激活、无 Console 管理入口、无 `AgentToolSpec.LlmName` 双名机制。AS-11 已设计但未实现。

**P4 -- 前端代码完全割裂**

Web 和 Console 零共享代码。`apiFetch` 逻辑各写一份，类型定义（LoginResponse、Theme 等）各维护一份，无 shared package。唯一的"共享"是 localStorage access_token key 用了相同字符串。

**P5 -- 系统限制不透明**

threadMessageLimit(200)、maxInputContentBytes(32KB)、defaultAgentMaxIterations(10)、maxParallelTasks(32)、entitlement defaults(999999 runs) 等限制硬编码在代码中，无集中注册、无文档暴露、Console 无法修改。用户和开发者在遇到限制时才知道它存在。

**P6 -- 缺少质量保证基础设施**

无压力测试基线、无 CI 流水线、无自动化质量门禁。代码合并完全依赖人工判断。

---

## 1. Track A -- 统一配置体系

**目标**：建立单一的配置解析链，所有配置点走同一路径，Console 可管理。

### A1 -- Config Resolver 核心

在 `src/services/shared/config/` 中建立统一配置解析器。

解析链（优先级从高到低）：
1. ENV override（部署层强制覆盖）
2. DB org 级配置（`org_settings` 表，per-org 定制）
3. DB platform 级配置（`platform_settings` 表，全局默认）
4. 代码注册的默认值（Registry 注册时声明）

核心接口：
```go
// shared/config/resolver.go
type Resolver interface {
    // 按 scope 解析单个 key
    Resolve(ctx context.Context, key string, scope Scope) (string, error)
    // 批量解析指定前缀的所有 key
    ResolvePrefix(ctx context.Context, prefix string, scope Scope) (map[string]string, error)
}

type Scope struct {
    OrgID *uuid.UUID // nil = platform scope
}
```

实现要点：
- DB 查询结果 Redis 缓存（TTL 可配置，写入时主动失效）
- ENV override 始终最高优先级（部署层可强制覆盖任何配置）
- 解析结果包含来源标记（env/org_db/platform_db/default），便于调试

### A2 -- Config Registry（配置声明注册）

所有可配置项必须注册后才能使用，注册时声明 key、类型、默认值、描述、是否敏感：

```go
// shared/config/registry.go
type Entry struct {
    Key          string
    Type         string // "string" | "int" | "bool" | "duration"
    Default      string
    Description  string
    Sensitive    bool   // true = Console 不显示原值，写入走 secrets 表
    Scope        string // "platform" | "org" | "both"
}
```

注册入口放在各模块 init 阶段，例如：
```go
config.Register(config.Entry{
    Key:     "email.smtp_host",
    Type:    "string",
    Default: "",
    Scope:   "platform",
})
```

Registry 同时为 Console 提供"所有可配置项"的元数据查询接口（`GET /v1/config/schema`），前端据此动态渲染配置页面。

### A3 -- org_settings 表

新建 migration：
```sql
CREATE TABLE org_settings (
    org_id  uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    key     text NOT NULL,
    value   text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, key)
);
```

与现有 `platform_settings` 结构对齐。Resolver 内部统一查询两张表。

### A4 -- 迁移现有配置消费方

将现有散落的配置读取逻辑逐步迁移到 Resolver：

| 模块 | 当前方式 | 迁移后 |
|------|---------|--------|
| email | 自建 config_db.go + ENV fallback | `config.ResolvePrefix(ctx, "email.", scope)` |
| web_search | 自建 config_db.go + ENV fallback | `config.ResolvePrefix(ctx, "web_search.", scope)` |
| web_fetch | 自建 config_db.go + ENV fallback | `config.ResolvePrefix(ctx, "web_fetch.", scope)` |
| openviking | 自建 config.go + ENV fallback | `config.ResolvePrefix(ctx, "openviking.", scope)` |
| turnstile | ENV 直读 | `config.Resolve(ctx, "turnstile.secret_key", scope)` |
| gateway rate limit | ENV + DB 混合 | `config.ResolvePrefix(ctx, "gateway.", scope)` |
| LLM retry | ENV 直读 | `config.ResolvePrefix(ctx, "llm.retry.", scope)` |

迁移完成后删除各模块的 `config_db.go`。

### A5 -- Console 配置管理页升级

基于 A2 的 Registry schema 接口，Console 配置页改为动态渲染：
- Platform Settings：全局配置项，平台管理员可调
- Org Settings：org 级覆盖，org admin 可调
- 敏感值（Sensitive=true）通过 secrets 表存储，Console 只显示 mask

---

## 2. Track B -- 系统限制集中声明

**目标**：所有系统限制注册到统一 Registry，Console 可调、文档可查。

### B1 -- Limits Registry

扩展 Track A 的 Config Registry，将现有硬编码限制收入配置体系：

| Key | 当前硬编码位置 | 默认值 | Scope |
|-----|---------------|--------|-------|
| `limit.thread_message_history` | mw_input_loader.go | 200 | org |
| `limit.max_input_content_bytes` | v1_runs.go | 32768 | org |
| `limit.agent_max_iterations` | mw_skill_resolution.go | 10 | org |
| `limit.max_parallel_tasks` | lua.go | 32 | platform |
| `limit.concurrent_runs` | entitlement resolve.go | 10 | org |
| `limit.team_members` | entitlement resolve.go | 50 | org |
| `quota.runs_per_month` | entitlement resolve.go | 999999 | org |
| `quota.tokens_per_month` | entitlement resolve.go | 1000000 | org |
| `credit.initial_grant` | entitlement resolve.go | 1000 | platform |
| `credit.invite_reward` | entitlement resolve.go | 500 | platform |
| `credit.per_usd` | handler_agent_loop.go | 1000 | platform |
| `llm.max_response_bytes` | anthropic.go | 16384 | platform |
| `browser.max_body_bytes` | server.ts | 1048576 | platform |
| `browser.context_max_lifetime_s` | config.ts | 1800 | platform |
| `sandbox.idle_timeout_lite_s` | .env | 180 | platform |
| `sandbox.idle_timeout_pro_s` | .env | 300 | platform |
| `sandbox.idle_timeout_ultra_s` | .env | 600 | platform |
| `sandbox.max_lifetime_s` | .env | 1800 | platform |

### B2 -- Entitlement 接入 Resolver

`shared/entitlement/resolve.go` 中的 hardcoded `defaults` map 迁移到 Config Registry。Entitlement 服务读取 plan 定义 -> org 覆盖 -> platform 默认值，链路与 Resolver 对齐。

### B3 -- 限制文档自动生成

从 Registry 自动导出 markdown 文档（所有注册 key、类型、默认值、scope、描述），放在 `src/docs/reference/configuration.zh-CN.md`。CI 中检查此文件与 Registry 代码是否同步。

---

## 3. Track C -- Tool Provider 管理（AS-11）

**目标**：同名工具支持多后端注册，per-org 激活指定 Provider，Console 可管理凭证和配置。

此 Track 对应 agent-system-roadmap 中 AS-11 的完整设计，已有详细薄片，不再重复。关键里程碑：

### C1 -- AgentToolSpec.LlmName + 多后端注册（AS-11.1）
- `AgentToolSpec` 增加 `LlmName` 字段
- `DispatchExecutor.Bind()` 建立 LlmName -> Name 反向索引
- web_search 拆分为 `web_search.tavily`、`web_search.searxng`
- web_fetch 拆分为 `web_fetch.jina`、`web_fetch.firecrawl`、`web_fetch.basic`

### C2 -- DB Schema + per-org 激活（AS-11.2）
- 新建 `tool_provider_configs` 表
- 同 org + group_name 最多一条 is_active = true
- 敏感值走 `secrets` 表加密存储

### C3 -- Worker Pipeline 注入（AS-11.3）
- 新建 `mw_tool_provider.go`
- MCPDiscovery 之后、ToolBuild 之前插入
- 从 DB 读取 org 激活的 Provider，覆盖默认 executor

### C4 -- Console API + UI（AS-11.4 / AS-11.5）
- CRUD 接口：列出 Provider Group、激活/停用 Provider、配置凭证
- Console 页面：Tool Provider 管理（列表 + 配置 + 测试连通性）

---

## 4. Track D -- 前端共享层

**目标**：Web 和 Console 共享基础代码，消除重复，统一开发范式。

### D1 -- shared package 建立

在 `src/apps/shared/` 创建共享包，pnpm workspace 方式引用：

```
src/apps/shared/
├── package.json          # @arkloop/shared
├── src/
│   ├── api/
│   │   ├── client.ts     # apiFetch、ApiError、token 管理
│   │   └── types.ts      # LoginResponse、MeResponse 等共享类型
│   ├── storage/
│   │   └── tokens.ts     # access/refresh token 读写
│   ├── hooks/
│   │   └── useAuth.ts    # 认证状态 hook（如适用）
│   └── index.ts
└── tsconfig.json
```

### D2 -- 迁移 Web 和 Console 的重复代码

| 模块 | Web 当前位置 | Console 当前位置 | 共享后位置 |
|------|-------------|-----------------|-----------|
| apiFetch | `src/apps/web/src/api.ts` | `src/apps/console/src/api/client.ts` | `@arkloop/shared/api/client` |
| 类型定义 | `src/apps/web/src/api.ts` | `src/apps/console/src/api/*.ts` | `@arkloop/shared/api/types` |
| Token 管理 | `src/apps/web/src/storage.ts` | `src/apps/console/src/storage.ts` | `@arkloop/shared/storage/tokens` |
| Theme 类型 | `src/apps/web/src/contexts/ThemeContext.tsx` | `src/apps/console/src/contexts/ThemeContext.tsx` | `@arkloop/shared/theme` |

迁移原则：只迁移已确认重复的代码，不做预设抽象。Web 和 Console 各自 import `@arkloop/shared` 替换本地实现。

### D3 -- pnpm workspace 配置

根目录 `pnpm-workspace.yaml` 已存在，补充 shared 包声明：
```yaml
packages:
  - src/apps/shared
  - src/apps/web
  - src/apps/console
```

Web 和 Console 的 `package.json` 添加：
```json
"dependencies": {
  "@arkloop/shared": "workspace:*"
}
```

---

## 5. Track E -- Agent System 未完成工作

以下 AS-* 项在 agent-system-roadmap 中已有完整设计薄片，此处仅列出状态和执行优先级。

### E1 -- Skill 路由绑定（AS-2.1）

状态：未实现。Skill 缺少 `preferred_credential` 字段，model 选择完全依赖外部传入 route_id。

内容：Skill YAML 增加 `preferred_credential` 字段；mw_routing.go 中的选路逻辑读取此字段作为 hint。

### E2 -- Memory 提炼管线（AS-5.7）

状态：未实现。memory_search/read/write/forget tool 已存在，但 run 结束后无自动提炼流程。

内容：run 完成后触发轻量 LLM 提取结构化知识点，写入 Memory。仅在 tool call >= 2 或对话轮数 >= 3 时触发。

### E3 -- Memory 测试（AS-5.8）

状态：未实现。

内容：MemoryProvider 接口测试 + OpenViking 适配器集成测试 + Memory Tool 端到端测试。

### E4 -- Cost Budget 执行侧强制（AS-8）

状态：预留字段存在，执行侧未强制。RunContext.ToolBudget 字段可用但 Loop 内无 token 消耗检查。

内容：SimpleExecutor 内每次 LLM 调用后累加 token 消耗，超限触发 `budget.exceeded` 终止。

### E5 -- Thinking 展示协议（AS-10）

状态：前端 ThinkingBlock 组件存在，后端无 thinking channel 分离和 segment 事件。

内容：
- 子轨道 A：LLM 原生 thinking 输出分离到 `channel: "thinking"` 事件
- 子轨道 B：`run.segment.start/end` 事件，Agent 级执行过程分组

### E6 -- Browser SSRF 防护（AS-7.5）

状态：Browser Service 基础功能完整，SSRF 防护未实现。

内容：Playwright route 拦截内网地址（RFC 1918/4193/6890），阻断 SSRF 攻击路径。

### E7 -- 可扩展性与性能基线（AS-12）

状态：未实现。

内容：
- AS-12.1：Browser Service 横向扩展路径（Session Affinity vs Stateless Mode 决策）
- AS-12.2：Sandbox 多节点调度接口（SandboxClient 抽象）
- AS-12.3：MCP Pool 运行时指标暴露
- AS-12.4：OpenViking 容量基线压测
- AS-12.5：Worker DB 连接池配置暴露

---

## 6. Track F -- 插件体系规划

**目标**：为未来的插件化扩展建立接口契约，但不在核心路径上引入插件运行时。

### F1 -- 插件边界识别

以下功能未来可能以插件形式提供，当前开发时需预留接口边界：

| 候选插件 | 当前状态 | 接口要求 |
|----------|---------|---------|
| Stripe 订阅 | 未接入 | 计费接口抽象（BillingProvider） |
| OAuth Provider | 未实现 | 认证接口抽象（AuthProvider） |
| 通知渠道（Slack/Discord/Telegram） | 仅邮件 | 通知接口抽象（NotificationChannel） |
| 存储后端（AWS S3/GCS/Azure Blob） | 仅 MinIO | 对象存储接口（ObjectStore，已由 MinIO SDK 抽象） |
| LLM Provider 扩展 | 硬编码 Anthropic/OpenAI 等 | Provider 路由已抽象 |

### F2 -- 接口契约原则

- 插件是接口实现，不是 hook 或 event bus。核心系统依赖接口，插件提供实现
- 核心功能不依赖任何插件。无插件时系统完整可用，插件只提供扩展能力
- 插件配置走 Config Resolver（Track A），不引入新的配置路径
- 第一个正式插件（预计 Stripe）落地时再制定插件发现和加载机制，不提前过度设计

### F3 -- BillingProvider 接口预留

当前积分和 Plan 体系是硬编码逻辑。预留接口供 Stripe 等外部计费系统接入：

```go
type BillingProvider interface {
    CreateSubscription(ctx context.Context, orgID uuid.UUID, planID string) error
    CancelSubscription(ctx context.Context, orgID uuid.UUID) error
    SyncUsage(ctx context.Context, orgID uuid.UUID, usage UsageRecord) error
    HandleWebhook(ctx context.Context, payload []byte) error
}
```

默认实现为内置积分系统（现有逻辑）。Stripe 插件实现此接口后通过配置切换。

---

## 7. Track G -- 基础设施建设

**目标**：建立质量门禁和性能基线，保障开源后的代码质量。

### G1 -- CI 流水线

GitHub Actions 配置，触发条件：PR 和 main 分支 push。

**Go 服务**：
- `go vet ./...` + `staticcheck ./...`（静态分析）
- `go test ./...`（单元测试 + 覆盖率报告）
- `go build ./...`（编译检查）
- 对 api、gateway、worker、sandbox、shared 五个 module 各自运行

**TypeScript 前端**：
- `pnpm lint`（ESLint）
- `pnpm type-check`（tsc --noEmit）
- `pnpm test`（Vitest）
- 对 web、console、browser、shared 各自运行

**数据库**：
- Migration 前进/回滚测试（apply all -> rollback all -> reapply）

### G2 -- 压力测试基线

建立各服务的单节点容量上限，用 k6 或 Go bench：

| 目标 | 并发 | 指标 |
|------|------|------|
| Gateway 限流吞吐 | 1000 req/s | P99 延迟 < 10ms |
| API CRUD | 200 并发 | P99 延迟 < 100ms |
| SSE 长连接 | 500 并发 | 连接保持率 > 99% |
| Worker Agent Loop | 50 并发 run | DB 连接池不溢出 |
| OpenViking 检索 | 100 并发 | P99 延迟 < 500ms |
| Browser 并发 session | 20 并发 | 内存 < 4GB |

压测脚本放在 `tests/bench/`，结果记录在 `docs/benchmark/`。

### G3 -- 开发环境一键启动

确保 `docker compose up` + 最少的 ENV 配置即可启动完整开发环境。验证清单：
- `.env.example` 覆盖所有必要配置
- `compose.yaml` 包含所有服务的健康检查
- migration 自动运行
- seed data 可选注入（管理员账户 + 示例 org）

---

## 8. 执行优先级与依赖关系

```
Track A（配置体系）—— 最高优先，所有 Track 的地基
  A1 → A2 → A3 → A4 → A5

Track B（系统限制）—— 依赖 A1/A2
  B1 → B2 → B3

Track C（Tool Provider）—— 独立，可与 A 并行
  C1 → C2 → C3 → C4

Track D（前端共享）—— 独立，可与 A/C 并行
  D1 → D2 → D3

Track E（Agent System 未完成）—— 各项独立
  E1（Skill 路由绑定）
  E2 → E3（Memory 提炼 → 测试）
  E4（Cost Budget）
  E5（Thinking 协议）
  E6（Browser SSRF）
  E7（性能基线，依赖 E6）

Track F（插件体系）—— 最低优先，仅预留接口
  F1 → F2 → F3

Track G（基础设施）—— 独立，建议与 A 同步启动
  G1（CI）
  G2（压测，依赖 E7 完成后执行更有意义）
  G3（开发环境）
```

**建议执行顺序**：

第一批（并行启动）：
- Track A（A1-A3）：配置体系核心
- Track D（D1-D3）：前端共享层
- Track G（G1, G3）：CI + 开发环境

第二批（A1-A3 完成后）：
- Track A（A4-A5）：配置迁移
- Track B（B1-B3）：系统限制
- Track C（C1-C4）：Tool Provider

第三批（按需推进）：
- Track E 各项（按产品优先级排序）
- Track F（第一个外部集成需求出现时启动）
- Track G（G2 压测）

---

## 9. 不变量与决策记录

以下决策在本路线图内固定：

- **配置解析链固定**：ENV override > org_settings DB > platform_settings DB > 代码默认值。不允许新增其他配置来源（文件、远程配置中心等）。
- **所有配置必须注册**：未注册的 key 调用 Resolve 返回错误，不允许"悄悄"读取未声明的配置。
- **Scope 模型固定**：platform 和 org 两级。不引入 user 级、team 级配置（过度设计）。thread/project 级别的配置走 AgentConfig 继承链（已有机制），不经过 Config Resolver。
- **前端共享包仅包含确定重复的代码**：不做预设抽象，不建 UI 组件库。Web 和 Console 的 UI 层保持独立。
- **插件不是当前重点**：只预留接口边界，不实现插件发现/加载/沙箱机制。第一个正式插件需求落地时再设计运行时。
- **CI 不阻塞开发**：CI 失败产生警告，不阻塞合并（开源发布前切换为强制门禁）。
- **旧 roadmap 归档不删除**：三份旧 roadmap 保留作为历史参考，不再新增内容。所有新工作在本文档中追踪。
- **Browser SSRF 在开源前必须完成**：这是安全底线，不可妥协。
- **沿用已有决策**：agent-system-roadmap 中第 16 节的所有不变量继续生效（Sandbox 独立服务、Executor 注册表、Memory 降级策略、Model 优先级链、Sub-agent 层级限制、Thinking 渲染协议、Browser Service 独立部署、Tool Provider 双名机制、Lua Executor 选型等）。
