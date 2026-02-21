# Arkloop 目标架构

---

## 一、系统拓扑

```
                          ┌────────────────────┐
                          │    客户端层         │
                          │  Web / Console /   │
                          │  外部 API Client   │
                          └────────┬───────────┘
                                   │ HTTPS
                          ┌────────▼───────────┐
                          │     Gateway        │
                          │  独立 stateless 层  │
                          │  限流 / Auth /     │
                          │  IP过滤 / 路由     │
                          └────────┬───────────┘
                                   │
              ┌────────────────────┼────────────────────┐
              │                    │                    │
       ┌──────▼──────┐     ┌──────▼──────┐     ┌──────▼──────┐
       │  API 服务    │     │ Worker 集群  │     │ Webhook     │
       │  业务逻辑    │     │ Agent 执行   │     │ Delivery    │
       │  数据 CRUD   │     │ LLM 调用     │     │ Service     │
       │  SSE 推送    │     │ MCP 连接     │     │             │
       └──────┬──────┘     └──────┬──────┘     └──────┬──────┘
              │                    │                    │
       ┌──────▼────────────────────▼────────────────────▼──────┐
       │                        数据层                          │
       │                                                        │
       │   PostgreSQL          Redis           对象存储          │
       │   结构化数据           缓存/锁/限流    (MinIO/S3)       │
       │   任务队列(jobs)       心跳/Pub-Sub    附件/Checkpoint  │
       │   事件流(run_events)   配额计数器      冷归档           │
       └────────────────────────────────────────────────────────┘
```

---

## 二、Gateway 层

Gateway 是独立进程，stateless，可水平扩展到任意副本数。

### 2.1 职责

1. **身份验证**：验证 JWT token 或 API Key，从 Redis 查 JWT 黑名单（`arkloop:jwt:revoked:{jti}`），不命中才走 DB。
2. **限流**：Redis token bucket，按 org_id 限频（`arkloop:ratelimit:{org_id}:{window}`）。
3. **并发 Run 限制**：检查 org 当前活跃 run 数是否超限（Redis counter）。
4. **IP 过滤**：从 Redis 缓存加载 org 的 IP allowlist/blocklist 规则。
5. **请求审计**：每个入站请求的 IP、User-Agent、API Key ID 写入审计日志。
6. **路由**：合法请求转发到 API 服务。

### 2.2 技术选型

近期：Go 自研 reverse proxy（`net/http/httputil.ReverseProxy`），复用现有技术栈。
远期：Envoy + Lua/WASM 插件，或 Cloudflare Workers（生产 CDN）。

### 2.3 Gateway 与 API 的分工

| 关注点 | Gateway | API |
|---|---|---|
| Token 验证 | 是 | 否（Gateway 已验证，只传 actor 上下文） |
| Rate Limit | 是 | 否 |
| IP 检查 | 是 | 否 |
| 业务 CRUD | 否 | 是 |
| SSE 推送 | 否（透传长连接） | 是 |
| 数据库事务 | 否 | 是 |

---

## 三、API 服务

保持当前 Go net/http 架构，职责收窄为纯业务逻辑：

- 数据 CRUD（threads、messages、runs、orgs ...）
- Run 创建 + 入队
- SSE 推送（改为 `LISTEN/NOTIFY` 驱动，不再轮询）
- MCP 配置管理端点
- LLM 凭证管理端点
- Webhook endpoint 管理
- 内部健康检查（供 Gateway 探活）

---

## 四、Worker 集群

### 4.1 生命周期

**启动时：**
1. 生成 `worker_id`（UUID）。
2. 向 Redis 注册（`arkloop:worker:{worker_id}` Hash，TTL 30s，每 10s 心跳续期）。
3. 同步写入 `worker_registrations` 表（持久化记录，审计用）。
4. 启动消费循环。

**处理一个 Run 时：**
1. 从 `jobs` 表 Lease 任务（`SKIP LOCKED`，加 `worker_tags` 过滤）。
2. Redis 分布式锁（替代 PG advisory lock）。
3. 从 DB 加载该 org 的 MCP 配置，向 Remote MCP Server 建立 HTTP/SSE 连接。
4. 执行 Agent 引擎。
5. 每个事件立即写入 DB + `pg_notify`。
6. Run 结束时汇总写入 `usage_records`，更新 `runs` 的成本和状态字段。
7. Ack/Nack。

**停止时：**
1. 标记 Redis key 过期 / DEL。
2. DB `worker_registrations.status = 'draining'` → `'dead'`。

### 4.2 任务路由

`jobs` 表新增 `worker_tags TEXT[]`。Worker Lease 时 SQL 加入 `worker_tags <@ $worker_capabilities` 过滤。无 tag 的任务任意 Worker 可拿。未来可演进为 Scheduler 主动分配。

---

## 五、实时推送

### 5.1 目标链路

```
Worker 收到 LLM chunk
    → 立即写入 run_events（delta 事件不再批处理）
    → pg_notify('run_events:{run_id}', seq)
    → API 实例 LISTEN channel → 立即查库推送 SSE
    → 多副本 API：Redis Pub/Sub 做事件广播
```

消除 Worker 200ms 批处理 + API 250ms 轮询的双重延迟。

### 5.2 多实例同步

```
Worker ──pg_notify──► API-A (LISTEN) ──► 直接推送给连到 A 的客户端
                                    └──► Redis Pub/Sub ──► API-B / API-C
```

客户端连接到哪个 API 实例，就由哪个实例推送。Redis Pub/Sub 保证跨实例的事件可达性。

---

## 六、数据库 Schema

以下是完整的目标 Schema。所有表以 migration 追加，不破坏现有结构。

### 6.1 身份与认证

```sql
-- users 补全
ALTER TABLE users
    ADD COLUMN email TEXT,
    ADD COLUMN email_verified_at TIMESTAMPTZ,
    ADD COLUMN status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'suspended', 'deleted')),
    ADD COLUMN deleted_at TIMESTAMPTZ,
    ADD COLUMN avatar_url TEXT,
    ADD COLUMN locale TEXT DEFAULT 'en',
    ADD COLUMN timezone TEXT DEFAULT 'UTC',
    ADD COLUMN last_login_at TIMESTAMPTZ;

CREATE UNIQUE INDEX uq_users_email
    ON users(email) WHERE email IS NOT NULL AND deleted_at IS NULL;

-- 多种登录方式
CREATE TABLE auth_identities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    provider_user_id TEXT NOT NULL,
    credential_hash TEXT,
    metadata_json JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_auth_identities_provider_user UNIQUE (provider, provider_user_id)
);

-- 会话追踪
CREATE TABLE user_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id UUID REFERENCES orgs(id) ON DELETE SET NULL,
    ip_address INET,
    user_agent TEXT,
    country_code CHAR(2),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_active_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX ix_user_sessions_user_id ON user_sessions(user_id);
```

### 6.2 组织与邀请

```sql
-- orgs 补全
ALTER TABLE orgs
    ADD COLUMN owner_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'suspended')),
    ADD COLUMN country CHAR(2),
    ADD COLUMN timezone TEXT DEFAULT 'UTC',
    ADD COLUMN logo_url TEXT,
    ADD COLUMN settings_json JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN deleted_at TIMESTAMPTZ;

-- 邀请机制
CREATE TABLE org_invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    invited_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    email TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'member',
    token TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 6.3 RBAC

```sql
CREATE TABLE rbac_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID REFERENCES orgs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    permissions TEXT[] NOT NULL DEFAULT '{}',
    is_system BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE org_memberships
    ADD COLUMN role_id UUID REFERENCES rbac_roles(id);
```

### 6.4 Teams 与 Projects

```sql
CREATE TABLE teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE team_memberships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_team_memberships UNIQUE (team_id, user_id)
);

CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    team_id UUID REFERENCES teams(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    description TEXT,
    visibility TEXT NOT NULL DEFAULT 'private'
        CHECK (visibility IN ('private', 'team', 'org')),
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE threads
    ADD COLUMN project_id UUID REFERENCES projects(id) ON DELETE SET NULL,
    ADD COLUMN deleted_at TIMESTAMPTZ;
```

### 6.5 Messages 多模态改造

```sql
ALTER TABLE messages
    ADD COLUMN content_json JSONB,
    ADD COLUMN metadata_json JSONB NOT NULL DEFAULT '{}',
    ADD COLUMN deleted_at TIMESTAMPTZ,
    ADD COLUMN token_count INT;
```

`content_json` 格式参考 Anthropic Messages API：
```json
[
  {"type": "text", "text": "..."},
  {"type": "image", "source": {"type": "base64", "media_type": "image/png", "data": "..."}},
  {"type": "tool_use", "id": "...", "name": "...", "input": {...}},
  {"type": "tool_result", "tool_use_id": "...", "content": "..."}
]
```

### 6.6 Runs 补全

```sql
ALTER TABLE runs
    ADD COLUMN parent_run_id UUID REFERENCES runs(id) ON DELETE SET NULL,
    ADD COLUMN status_updated_at TIMESTAMPTZ,
    ADD COLUMN completed_at TIMESTAMPTZ,
    ADD COLUMN failed_at TIMESTAMPTZ,
    ADD COLUMN duration_ms INT,
    ADD COLUMN total_input_tokens INT,
    ADD COLUMN total_output_tokens INT,
    ADD COLUMN total_cost_usd NUMERIC(12, 8),
    ADD COLUMN model TEXT,
    ADD COLUMN skill_id TEXT,
    ADD COLUMN deleted_at TIMESTAMPTZ;

ALTER TABLE runs
    ADD CONSTRAINT ck_runs_status
    CHECK (status IN ('running', 'completed', 'failed', 'cancelled', 'cancelling'));
```

### 6.7 Run Events 分区 + 序号修复

```sql
-- 用 PostgreSQL sequence 替代行级锁争用
CREATE SEQUENCE run_events_seq_global;

-- run_events 改为按月分区
CREATE TABLE run_events_v2 (
    event_id UUID NOT NULL DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL,
    seq BIGINT NOT NULL DEFAULT nextval('run_events_seq_global'),
    ts TIMESTAMPTZ NOT NULL DEFAULT now(),
    type TEXT NOT NULL,
    data_json JSONB NOT NULL DEFAULT '{}',
    tool_name TEXT,
    error_class TEXT
) PARTITION BY RANGE (ts);

-- 每月一个分区，旧分区归档到对象存储后 DROP
-- 当前月 + 下月预创建，由 cron job 管理
```

### 6.8 Secrets 统一管理

```sql
CREATE TABLE secrets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    encrypted_value TEXT NOT NULL,
    key_version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    rotated_at TIMESTAMPTZ,
    CONSTRAINT uq_secrets_org_name UNIQUE (org_id, name)
);
```

加密方式：AES-256-GCM。密钥来源：开发用 `ARKLOOP_ENCRYPTION_KEY` env var，生产用 AWS KMS / GCP KMS / HashiCorp Vault。`key_version` 支持密钥轮换。

### 6.9 MCP 配置

```sql
CREATE TABLE mcp_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    transport TEXT NOT NULL DEFAULT 'streamable_http'
        CHECK (transport IN ('http_sse', 'streamable_http', 'stdio')),
    endpoint_url TEXT,
    auth_type TEXT CHECK (auth_type IN ('bearer', 'oauth2', 'none')),
    auth_secret_id UUID REFERENCES secrets(id),
    command TEXT,
    args TEXT[],
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 6.10 LLM 凭证与路由

```sql
CREATE TABLE llm_credentials (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    provider TEXT NOT NULL
        CHECK (provider IN ('openai', 'anthropic', 'gemini', 'deepseek')),
    name TEXT NOT NULL,
    secret_id UUID REFERENCES secrets(id),
    base_url TEXT,
    revoked_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE llm_routes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    credential_id UUID NOT NULL REFERENCES llm_credentials(id),
    model TEXT NOT NULL,
    priority INT NOT NULL DEFAULT 0,
    is_default BOOLEAN NOT NULL DEFAULT false,
    when_json JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 6.11 Skills 入库

```sql
CREATE TABLE skills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID REFERENCES orgs(id) ON DELETE CASCADE,
    skill_key TEXT NOT NULL,
    version TEXT NOT NULL,
    display_name TEXT NOT NULL,
    prompt_md TEXT NOT NULL,
    tool_allowlist TEXT[] NOT NULL DEFAULT '{}',
    budgets_json JSONB NOT NULL DEFAULT '{}',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_skills_org_key_version UNIQUE (org_id, skill_key, version)
);
```

### 6.12 API Keys

```sql
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    key_hash TEXT NOT NULL UNIQUE,
    scopes TEXT[] NOT NULL DEFAULT '{}',
    expires_at TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 6.13 IP 管控

```sql
CREATE TABLE ip_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('allowlist', 'blocklist')),
    cidr CIDR NOT NULL,
    note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 6.14 订阅与计费

```sql
CREATE TABLE plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    features JSONB NOT NULL DEFAULT '{}',
    limits JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    plan_id UUID NOT NULL REFERENCES plans(id),
    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'past_due', 'cancelled')),
    current_period_start TIMESTAMPTZ NOT NULL,
    current_period_end TIMESTAMPTZ NOT NULL,
    cancelled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_subscriptions_org UNIQUE (org_id)
);

CREATE TABLE usage_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    run_id UUID REFERENCES runs(id) ON DELETE SET NULL,
    model TEXT NOT NULL,
    input_tokens INT NOT NULL DEFAULT 0,
    output_tokens INT NOT NULL DEFAULT 0,
    cost_usd NUMERIC(12, 8) NOT NULL DEFAULT 0,
    effective_unit_price NUMERIC(12, 8),
    pricing_version TEXT,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_usage_records_org_recorded ON usage_records(org_id, recorded_at);
```

### 6.15 Webhooks

```sql
CREATE TABLE webhook_endpoints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    signing_secret TEXT NOT NULL,
    events TEXT[] NOT NULL DEFAULT '{}',
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE webhook_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    endpoint_id UUID NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    payload_json JSONB NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'delivered', 'failed')),
    attempt_count INT NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMPTZ,
    next_attempt_at TIMESTAMPTZ,
    response_status INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_webhook_deliveries_next ON webhook_deliveries(next_attempt_at)
    WHERE status IN ('pending', 'failed');
```

### 6.16 Feature Flags

```sql
CREATE TABLE feature_flags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key TEXT NOT NULL UNIQUE,
    description TEXT,
    default_value BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE org_feature_overrides (
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    flag_key TEXT NOT NULL REFERENCES feature_flags(key),
    value BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, flag_key)
);
```

### 6.17 Worker 注册

```sql
CREATE TABLE worker_registrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    worker_id TEXT NOT NULL UNIQUE,
    hostname TEXT NOT NULL,
    version TEXT,
    status TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'draining', 'dead')),
    capabilities JSONB NOT NULL DEFAULT '{}',
    current_load INT NOT NULL DEFAULT 0,
    max_concurrency INT NOT NULL DEFAULT 4,
    heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    registered_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 6.18 Jobs 增强

```sql
ALTER TABLE jobs
    ADD COLUMN priority INT NOT NULL DEFAULT 0,
    ADD COLUMN org_id UUID,
    ADD COLUMN worker_tags TEXT[] DEFAULT '{}',
    ADD COLUMN sandbox_id UUID;

CREATE INDEX ix_jobs_org_id ON jobs(org_id);
CREATE INDEX ix_jobs_dispatch ON jobs(priority DESC, available_at ASC)
    WHERE status = 'queued';
```

### 6.19 审计日志补全

```sql
ALTER TABLE audit_logs
    ADD COLUMN ip_address INET,
    ADD COLUMN user_agent TEXT,
    ADD COLUMN api_key_id UUID,
    ADD COLUMN before_state_json JSONB,
    ADD COLUMN after_state_json JSONB;
```

### 6.20 通知

```sql
CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id UUID REFERENCES orgs(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    body TEXT,
    payload_json JSONB DEFAULT '{}',
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_notifications_unread ON notifications(user_id)
    WHERE read_at IS NULL;
```

### 6.21 Sandbox（预留）

```sql
CREATE TABLE sandbox_states (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    thread_id UUID REFERENCES threads(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'running', 'suspended', 'terminated')),
    assigned_worker_id TEXT,
    checkpoint_object_key TEXT,
    checkpoint_version INT NOT NULL DEFAULT 0,
    last_checkpoint_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### 6.22 Agent Memory（预留）

```sql
-- 需要 pgvector 扩展
CREATE TABLE agent_memories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    embedding VECTOR(1536),
    source_run_id UUID REFERENCES runs(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ
);
```

---

## 七、Redis 规范

```
arkloop:{namespace}:{identifier}
```

| Key | 类型 | TTL | 用途 |
|---|---|---|---|
| `arkloop:jwt:revoked:{jti}` | String | token 剩余有效期 | JWT 黑名单 |
| `arkloop:session:{session_id}` | Hash | 会话 TTL | 会话缓存 |
| `arkloop:ratelimit:{org_id}:{window}` | String | 窗口时长 | 限流计数 |
| `arkloop:quota:{org_id}:tokens:{month}` | String | 到月末 | 月度 token 配额 |
| `arkloop:worker:{worker_id}` | Hash | 30s | Worker 心跳 |
| `arkloop:lock:run:{run_id}` | String | 任务超时 | 分布式锁 |
| `arkloop:mcp:schema:{config_id}:{hash}` | String | 5min | MCP 工具列表缓存 |
| `arkloop:sub:{org_id}` | Hash | 1h | 订阅状态缓存 |
| `arkloop:feat:{org_id}:{flag}` | String | 5min | Feature flag 缓存 |
| `arkloop:org:active_runs:{org_id}` | String | — | 活跃 run 计数 |
| `arkloop:ip_rules:{org_id}` | String | 5min | IP 规则缓存 |

---

## 八、对象存储

开发环境：MinIO（`compose.yaml` 新增 service）。生产环境：AWS S3 / GCS / 阿里云 OSS（S3 兼容接口统一）。

| Bucket | 路径 | 保留策略 |
|---|---|---|
| `arkloop-sandboxes` | `/{sandbox_id}/v{version}.tar.zst` | 最近 3 个版本 |
| `arkloop-run-logs` | `/{year}/{month}/{run_id}.jsonl.gz` | 按 org 策略 |
| `arkloop-attachments` | `/{org_id}/{attachment_id}` | 按 org 策略 |
| `arkloop-artifacts` | `/{org_id}/{run_id}/{name}` | 7 天 TTL |
| `arkloop-message-content` | `/{org_id}/{message_id}` | 同 thread |

---

## 九、凭证加密

| 凭证 | 存储 | 加密 |
|---|---|---|
| LLM API Key（平台默认） | env var | — |
| LLM API Key（BYOK） | `secrets` 表 | AES-256-GCM |
| MCP Auth Token | `secrets` 表 | AES-256-GCM |
| Webhook signing secret | `webhook_endpoints.signing_secret` | 不加密（内部用） |
| 用户密码 | `auth_identities.credential_hash` | bcrypt |
| API Key | `api_keys.key_hash` | SHA-256 |

---

## 十、MCP 架构

### 10.1 运行时流程

```
用户在 Console 配置 Remote MCP Server（URL + Auth Token）
    → 存入 mcp_configs + secrets 表
    → Worker 执行 Run 时从 DB 加载 org 的 MCP 配置
    → Worker 向 Remote MCP Server 发起 HTTPS 连接（Streamable HTTP transport）
    → 工具列表缓存到 Redis
    → Agent 引擎通过 HTTP 调用 Remote MCP Server
```

### 10.2 Sandbox 内部的 MCP

Sandbox 内部可以运行 stdio MCP Server，但 Sandbox 自身通过 HTTP 代理暴露给 Worker，Worker 不直接管理 stdio 子进程。

---

## 十一、可观测性

### 11.1 分布式追踪（OpenTelemetry）

每个服务启动 OTLP exporter，trace context 在 API → jobs.payload_json → Worker 之间传播。

- API 层：每个 HTTP 请求创建 root span。
- Worker 层：从 job payload 恢复 trace context，LLM 调用和工具调用各创建 child span。
- span attributes：`model`、`tokens`、`duration_ms`、`tool_name`、`error_class`。

### 11.2 指标（Prometheus）

```
run_started_total{org_id, model, skill}
run_completed_total{org_id, model, status}
run_duration_seconds{org_id, status}
llm_tokens_total{org_id, model, direction}
mcp_tool_calls_total{tool_name, status}
worker_active_runs{worker_id}
worker_queue_depth{job_type}
gateway_requests_total{method, path, status}
gateway_request_duration_seconds{method, path}
```

---

## 十二、Sandbox 服务（未来）

独立服务，不在 Worker 进程内。

```
Worker ──HTTP──► Sandbox Service
                      │
                      ├── sandbox_states (DB)
                      ├── checkpoint (对象存储)
                      └── 内部 stdio MCP Server
```

- 每个 Sandbox 关联到 `thread_id`。
- 状态机：`pending → running → suspended → terminated`。
- Worker 崩溃后 Sandbox 独立存活，可由另一个 Worker resume。
- Checkpoint 写对象存储，不走 PostgreSQL。

---

## 十三、开发环境 compose.yaml 目标

```yaml
services:
  postgres:    # 已有
  redis:       # 新增
  minio:       # 新增
  gateway:     # 新增
  api:
  worker:
```

---

## 十四、分阶段实施

### Phase 1：基础设施 + 致命问题修复

1. 引入 Redis（compose.yaml + go-redis）
2. 引入 Gateway 独立服务
3. `users` 补 email + status
4. `run_events` 序号改 PostgreSQL sequence
5. `run_events` 按月分区
6. SSE 改 pg_notify + 取消 delta 批处理
7. `secrets` 表
8. MCP 配置入库 + Remote HTTP/SSE
9. LLM 凭证入库

### Phase 2：核心功能补齐

10. `org_invitations`
11. `runs` 补全字段 + status CHECK + 自动更新
12. `messages` 多模态 content_json
13. 软删除（deleted_at）
14. Webhooks
15. 订阅 Schema
16. API Keys
17. Worker 注册与心跳
18. Jobs 增强字段

### Phase 3：企业级能力

19. RBAC
20. auth_identities + OAuth/SSO
21. user_sessions + IP 追踪
22. Teams + Projects
23. Skills 入库
24. Feature Flags
25. OpenTelemetry
26. 对象存储（MinIO）
27. 通知系统
28. 审计日志补全

### Phase 4：Sandbox + Agent Memory

29. Sandbox Service
30. Agent Memory（pgvector）

---

## 十五、显式不做的事

- 不引入 Kafka/RabbitMQ：PostgreSQL queue + Redis Pub/Sub 足够当前规模。
- 不做多 DB 分片：单 PostgreSQL + 读副本支撑到足够大的规模。
- 不在早期做 Service Mesh：Kubernetes Ingress + 简单 RBAC 足够。
- 不用 ClickHouse 替换 run_events：先用 PG 分区 + 冷归档，到瓶颈再迁移。
