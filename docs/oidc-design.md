# ArkLoop OIDC IdP 设计文档

| 字段 | 值 |
|---|---|
| 文档版本 | v1.0 (draft) |
| 创建日期 | 2026-05-19 |
| 状态 | 待评审 |
| 范围 | ArkLoop 升级为 OIDC Identity Provider；exam 作为首个 OIDC client 接入；智能体 `exam-agent` 通过内部 token 铸造机制代用户调用 exam API |
| 非目标 | OIDC Session Management、Front-channel logout、Dynamic Client Registration（v1 不实现，留待 v2） |

---

## 1. 目标与原则

### 1.1 业务目标

> **核心 UX**：老师**只在 ArkLoop 里**完成所有 exam 相关操作。在 ArkLoop 注册 = 在 exam 自动有账号；用智能体上传图片 = exam 自动建好课程目录。老师从不需要打开 exam 的 UI（exam 后台保留作为运维 / 学生侧使用）。

1. ArkLoop 用户**首次使用题库助手时**，exam 端无感自动建账（`provider='arkloop'`，`oidc_subject` 关联 ArkLoop user UUID）
2. ArkLoop 智能体 `exam-agent` 以当前 ArkLoop 用户身份调用 exam API（创建课程目录、生成题目）；token 由 worker 通过 `/internal/oauth/issue` 即取即用
3. exam 自身的浏览器 SSO 入口（"使用 ArkLoop 账号登录"按钮）保留作为**备用入口**，便于老师真要进 exam 后台时无缝登录
4. 为未来其他第三方应用接入 ArkLoop 身份体系铺路

### 1.2 设计原则
- **标准优先**：严格遵循 OpenID Connect Core 1.0 + OAuth 2.1 + RFC 7636 (PKCE)
- **最小权限**：智能体永远拿不到 `exam:admin` scope，prompt injection 提权天花板被锁死
- **零浏览器跳转给智能体**：worker 通过 ArkLoop 内部端点直接铸 token，避免在后端模拟浏览器流程
- **向后兼容**：HS256 本地 JWT 路径保留作为 worker 内部 token，OIDC 走 RS256 + JWKS
- **可灰度**：exam 同时支持本地账号登录（保留）+ OIDC 登录，按需切换

---

## 2. 总体架构

```
┌───────────────────────────────────────────────────────────────────────────────┐
│                                浏览器（用户）                                  │
└──┬──────────────────────────────────────────────┬─────────────────────────────┘
   │                                              │
   │ ① 访问 exam，未登录                          │ ⑥ 后续请求带 Bearer
   │ → 重定向到 ArkLoop /v1/auth/oauth/authorize │
   ▼                                              ▼
┌──────────────────────────────┐    ② code      ┌─────────────────────────────┐
│  ArkLoop API + OIDC IdP      │ ───────────►   │  exam backend                │
│                              │                │                              │
│  /v1/auth/oauth/authorize    │                │  /api/auth/oidc/callback     │
│  /v1/auth/oauth/token        │ ◄────────────  │   - 用 code 换 token         │
│  /v1/auth/oauth/userinfo     │   ③ token      │   - 验签（拉 JWKS）         │
│  /v1/auth/oauth/revoke       │                │   - auto-provision user      │
│  /.well-known/jwks.json      │                │   - 颁发本地 session         │
│  /.well-known/openid-config  │                │                              │
│                              │                │  CurrentUser 依赖：          │
│  /internal/oauth/issue       │                │   - OIDC Bearer (RS256+JWKS) │
│   (service-token only)       │                │   - 本地 JWT (HS256) [兼容]  │
└──────┬───────────────────────┘                └─────────────────────────────┘
       │ ④ worker 调内部端点                                ▲
       │   "为用户 X 铸 exam token"                         │
       ▼                                                    │ ⑤ 智能体调 exam
┌──────────────────────────────┐                            │   Bearer <token>
│  worker (exam-agent)         │ ───────────────────────────┘
│                              │
│  exam_recognize_catalog_image│
│  exam_parse_catalog_excel    │
│  exam_create_catalog_tree    │
│  exam_generate_questions     │
└──────────────────────────────┘
```

### 2.1 信任域
- **强信任**：ArkLoop API ↔ worker（共享 `ARKLOOP_INTERNAL_SERVICE_TOKEN`，同一团队同一部署）
- **弱信任**：ArkLoop ↔ exam（独立部署、独立运维边界，走标准 OIDC，公钥验签，无共享 secret 用于 token 验证）

---

## 3. 关键设计决策

| 决策项 | 选择 | 备选 / 理由 |
|---|---|---|
| 签名算法 | RS256（RSA 4096） | EdDSA 更现代但库支持弱；HS256 不可用（OIDC 要求公私钥分离） |
| 授权流 | Authorization Code Flow + PKCE + Refresh Token | Implicit / ROPC 不实现；Client Credentials 留给 v2 服务-服务调用 |
| Refresh Token | 启用，TTL 30 天，**rotation on use**（一次性使用） | 防止 token replay；老 refresh 在新 refresh 颁发后立即失效 |
| Access Token TTL | 15 分钟 | OIDC 行业惯例；强制走 refresh 才能 long-lived |
| ID Token TTL | 1 小时 | 仅用于身份断言，不能调 API |
| Consent | 首次显式同意（按 scope 集合记录），后续静默 | scope 升级时再次弹 consent |
| Scope 设计 | **分层混合**：`openid` `profile` `email` `offline_access` `exam:read` `exam:write` `exam:admin` | 见 §4 |
| 自动建账（exam） | **两条路径共用同一 provisioning 函数**：浏览器 SSO callback **和** worker token 验签都触发；`persona='teacher'`，`provider='arkloop'` | 用 `oidc_subject + email` 做幂等保护；同一份函数避免双路径漂移 |
| 内部铸 token 时注入用户属性 | claims 携带 `email` / `name` / `picture`，供 exam 首次见到时自动建出合理账号 | 老师从未进过 exam 浏览器界面，否则 exam 只能拿到光秃秃的 `sub` |
| 智能体取 token | worker 调 `POST /internal/oauth/issue`，service token 鉴权，跳过浏览器 | 见 §6.4 |
| 密钥管理 | RSA 4096 keypair，私钥加密存 DB（envelope encryption），JWKS 端点公开公钥 | 启动时若无 active key 自动生成第一对；rotation 走管理接口 |
| Token 存储 | access_token **不入库**（无状态 JWT，验签即可）；refresh_token 哈希入库（可撤销） | OAuth 2.1 推荐 |
| 重定向 URI 匹配 | 精确字符串匹配（不允许通配符、query string 也参与匹配） | RFC 6749 §3.1.2 |

---

## 4. Scope 设计

### 4.1 完整 scope 表

| Scope | 类型 | 含义 | 在 token 中体现 |
|---|---|---|---|
| `openid` | OIDC 必需 | 标记这是 OIDC 请求，触发 ID Token 颁发 | — |
| `profile` | OIDC 标准 | `/userinfo` 可返回 `name` `given_name` `family_name` `picture` `locale` | userinfo |
| `email` | OIDC 标准 | `/userinfo` 可返回 `email` `email_verified` | userinfo |
| `offline_access` | OIDC 标准 | 允许颁发 refresh_token | token endpoint 行为 |
| `exam:read` | 业务 | 调用 exam 任何只读 API（HTTP GET） | access_token.scope |
| `exam:write` | 业务 | 调用 exam 任何写入 API（POST/PUT/PATCH/DELETE 非危险） | access_token.scope |
| `exam:admin` | 业务（高敏） | 危险操作：批量删除、跨用户操作、组织管理 | access_token.scope |

### 4.2 scope 与 exam 路由的映射

| HTTP 方法 + 路径模式 | 所需 scope |
|---|---|
| `GET /learning/*`、`GET /questions*`、`GET /papers*` | `exam:read` |
| `POST/PUT /learning/knowledge-points`、`POST /learning/catalog-photo/recognize` | `exam:write` |
| `POST /questions`、`PUT /questions/{id}`、`POST /questions/ai-generate/*` | `exam:write` |
| `POST /questions/bulk-delete`、`POST /questions/bulk-move`、`DELETE /learning/majors/{id}`、所有 `/operations/*` | `exam:admin` |
| `GET /api/auth/me` 等用户自描述端点 | `openid`（任何 OIDC token 都能调） |

### 4.3 智能体工具与 scope 的对应

| 工具 | 申请 scopes |
|---|---|
| `exam_recognize_catalog_image` | `openid exam:write` |
| `exam_parse_catalog_excel` | `openid`（不调 exam API，纯本地解析） |
| `exam_create_catalog_tree` | `openid exam:write` |
| `exam_generate_questions` | `openid exam:read exam:write` |

**所有智能体工具永远不申请 `exam:admin`。** 内部端点 `/internal/oauth/issue` 在收到 worker 请求时，会校验"请求的 scopes ⊆ worker 允许的 scopes 白名单"，白名单硬编码为不含 `exam:admin`。

---

## 5. 数据模型

### 5.1 ArkLoop 端 (Postgres，goose migration)

```sql
-- 00186_oauth_clients.sql
CREATE TABLE oauth_clients (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id           TEXT NOT NULL UNIQUE,           -- 公开标识，e.g. "exam-web"
    client_secret_hash  TEXT NOT NULL,                  -- bcrypt(secret); confidential client
    client_type         TEXT NOT NULL DEFAULT 'confidential',  -- 'confidential' | 'public'
    name                TEXT NOT NULL,                  -- 展示名
    redirect_uris       TEXT[] NOT NULL,                -- 精确匹配集合
    allowed_scopes      TEXT[] NOT NULL,                -- client 可申请的 scope 上限
    require_pkce        BOOLEAN NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ
);
CREATE INDEX ix_oauth_clients_client_id ON oauth_clients(client_id) WHERE deleted_at IS NULL;

-- 00187_oauth_authorization_codes.sql
CREATE TABLE oauth_authorization_codes (
    code_hash            TEXT PRIMARY KEY,              -- SHA256(code)，code 一次性使用
    client_id            TEXT NOT NULL REFERENCES oauth_clients(client_id),
    user_id              UUID NOT NULL,                 -- ArkLoop user
    redirect_uri         TEXT NOT NULL,                 -- 必须与 token 交换时传入一致
    scopes               TEXT[] NOT NULL,
    code_challenge       TEXT NOT NULL,                 -- PKCE
    code_challenge_method TEXT NOT NULL DEFAULT 'S256',
    nonce                TEXT,                          -- 透传给 id_token
    expires_at           TIMESTAMPTZ NOT NULL,          -- TTL 60s
    consumed_at          TIMESTAMPTZ,                   -- 一次性使用，consumed 后再用即报错
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ix_oauth_codes_expires ON oauth_authorization_codes(expires_at);

-- 00188_oauth_refresh_tokens.sql（区别于已有 /v1/auth refresh_tokens）
CREATE TABLE oauth_refresh_tokens (
    token_hash      TEXT PRIMARY KEY,                   -- SHA256
    client_id       TEXT NOT NULL REFERENCES oauth_clients(client_id),
    user_id         UUID NOT NULL,
    scopes          TEXT[] NOT NULL,
    issued_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL,               -- 30d
    rotated_to      TEXT,                               -- 新 token 的 hash，rotation 链
    revoked_at      TIMESTAMPTZ                         -- 撤销时间
);
CREATE INDEX ix_oauth_refresh_active ON oauth_refresh_tokens(user_id, client_id) 
    WHERE revoked_at IS NULL;

-- 00189_oauth_consents.sql
CREATE TABLE oauth_consents (
    user_id      UUID NOT NULL,
    client_id    TEXT NOT NULL REFERENCES oauth_clients(client_id),
    scopes       TEXT[] NOT NULL,                       -- 已同意的 scope 集合
    granted_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at   TIMESTAMPTZ,
    PRIMARY KEY (user_id, client_id)
);

-- 00190_users_add_oidc_claims.sql
ALTER TABLE users
    ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN given_name TEXT,
    ADD COLUMN family_name TEXT,
    ADD COLUMN picture_url TEXT,
    ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT now();
-- 数据迁移：email_verified_at IS NOT NULL → email_verified = TRUE
UPDATE users SET email_verified = TRUE WHERE email_verified_at IS NOT NULL;

-- 00191_oidc_signing_keys.sql
CREATE TABLE oidc_signing_keys (
    kid                   TEXT PRIMARY KEY,             -- key id，写入 JWT header
    algorithm             TEXT NOT NULL DEFAULT 'RS256',
    public_key_pem        TEXT NOT NULL,                -- 公开（JWKS）
    private_key_encrypted BYTEA NOT NULL,               -- envelope encryption
    status                TEXT NOT NULL DEFAULT 'active', -- 'active' | 'retired' | 'compromised'
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    activated_at          TIMESTAMPTZ,
    retired_at            TIMESTAMPTZ
);
CREATE INDEX ix_oidc_keys_active ON oidc_signing_keys(status, created_at);
```

### 5.2 exam 端 (Alembic)

```python
# alembic/versions/20260520_add_oidc_to_users.py
revision = "20260520_add_oidc_to_users"
down_revision = "<上一个 head>"

def upgrade():
    op.add_column("users", sa.Column("oidc_subject", sa.String, nullable=True))
    op.add_column("users", sa.Column("provider", sa.String, server_default="internal", nullable=False))
    op.create_index(
        "ix_users_oidc_subject_active",
        "users", ["oidc_subject"],
        unique=True,
        postgresql_where=sa.text("deleted_at IS NULL AND oidc_subject IS NOT NULL"),
    )
```

---

## 6. 端点契约

### 6.1 `GET /v1/auth/oauth/authorize`

**Query params**:
| 参数 | 必需 | 说明 |
|---|---|---|
| `response_type` | ✓ | 必须为 `code` |
| `client_id` | ✓ | 已注册的 client_id |
| `redirect_uri` | ✓ | 必须精确匹配 oauth_clients.redirect_uris 之一 |
| `scope` | ✓ | 空格分隔，必须包含 `openid`；超出 client.allowed_scopes 报错 |
| `state` | ✓ | client 透传防 CSRF，长度 ≥ 16 字符 |
| `code_challenge` | ✓ | PKCE，BASE64URL(SHA256(verifier)) |
| `code_challenge_method` | ✓ | 必须为 `S256` |
| `nonce` | optional | 透传到 id_token.nonce，client 防 replay |
| `prompt` | optional | `none` / `login` / `consent` |

**行为**:
1. 校验 query 参数（不符直接返 400，**不重定向**，防开放重定向）
2. 检查用户是否已登录 ArkLoop（cookie/session）；未登录跳 `/login?next=<原 URL>`
3. 检查 oauth_consents 是否已有覆盖性同意；无 → 渲染 consent 页 `/oauth/consent?client=...&scopes=...`
4. 同意后：生成 code（随机 32 字节 Base64URL），SHA256 后入库 `oauth_authorization_codes`，TTL 60s
5. 302 跳转 `{redirect_uri}?code=<code>&state=<state>`

### 6.2 `POST /v1/auth/oauth/token`

**Body (`application/x-www-form-urlencoded`)**:

Grant `authorization_code`:
```
grant_type=authorization_code
&code=<code>
&redirect_uri=<must match authorize-time>
&client_id=<client>
&client_secret=<secret>      // confidential client 必需
&code_verifier=<PKCE>        // 必需
```

Grant `refresh_token`:
```
grant_type=refresh_token
&refresh_token=<token>
&client_id=<client>
&client_secret=<secret>
&scope=<optional, 子集>
```

**Response 200**:
```json
{
  "access_token": "<JWT RS256>",
  "token_type": "Bearer",
  "expires_in": 900,
  "refresh_token": "<opaque>",       // 仅 offline_access scope 时
  "id_token": "<JWT RS256>",         // 仅 openid scope 时
  "scope": "openid profile email exam:write"
}
```

**错误响应**（RFC 6749 §5.2）:
```json
{ "error": "invalid_grant", "error_description": "..." }
```

### 6.3 `GET /v1/auth/oauth/userinfo`

**Header**: `Authorization: Bearer <access_token>`

**Response**: 根据 token.scope 返回字段子集
```json
{
  "sub": "<user.id>",
  "email": "...",                 // 需 scope=email
  "email_verified": true,
  "name": "...",                  // 需 scope=profile
  "given_name": "...",
  "family_name": "...",
  "picture": "...",
  "locale": "zh-CN",
  "updated_at": 1716096000
}
```

### 6.4 `POST /internal/oauth/issue` ⭐ **智能体关键端点**

**Auth**: `Authorization: Bearer <service_token>`（来自 `ARKLOOP_INTERNAL_SERVICE_TOKEN`，仅 worker 持有）

**Body**:
```json
{
  "user_id": "<ArkLoop user uuid>",   // worker 当前正在为谁干活
  "client_id": "exam-web",            // 目标 client
  "scopes": ["openid", "exam:write"]  // 子集
}
```

**Response**: 与 `/oauth/token` 相同 shape，但**无 refresh_token**（worker 每次按需重新铸）

**安全约束**:
- 仅 service_token 可调（与公网 OIDC 端点完全隔离）
- `scopes` 必须 ⊆ ALLOWLIST（hardcoded 白名单，**不含 `exam:admin`**）
- `client_id` 必须存在且状态 active
- TTL 强制为 60s（远短于浏览器流的 15min；智能体单次调用即用即弃）
- 调用全部审计入 `activity_logs`（按 user_id + client_id 索引）

### 6.5 `POST /v1/auth/oauth/revoke`（RFC 7009）

撤销 refresh_token（access_token 是无状态 JWT，到期前无法撤销，靠短 TTL 兜底）。

### 6.6 `GET /.well-known/openid-configuration`

返回 discovery metadata：
```json
{
  "issuer": "https://arkloop.example.com",
  "authorization_endpoint": "https://arkloop.example.com/v1/auth/oauth/authorize",
  "token_endpoint": "https://arkloop.example.com/v1/auth/oauth/token",
  "userinfo_endpoint": "https://arkloop.example.com/v1/auth/oauth/userinfo",
  "revocation_endpoint": "https://arkloop.example.com/v1/auth/oauth/revoke",
  "jwks_uri": "https://arkloop.example.com/.well-known/jwks.json",
  "response_types_supported": ["code"],
  "grant_types_supported": ["authorization_code", "refresh_token"],
  "scopes_supported": ["openid", "profile", "email", "offline_access",
                       "exam:read", "exam:write", "exam:admin"],
  "token_endpoint_auth_methods_supported": ["client_secret_post", "client_secret_basic"],
  "code_challenge_methods_supported": ["S256"],
  "subject_types_supported": ["public"],
  "id_token_signing_alg_values_supported": ["RS256"]
}
```

### 6.7 `GET /.well-known/jwks.json`

```json
{
  "keys": [
    {
      "kty": "RSA", "use": "sig", "alg": "RS256",
      "kid": "<key id>",
      "n": "<base64url modulus>",
      "e": "AQAB"
    }
  ]
}
```

仅返回 status='active' 和 'retired'（仍在 TTL 内的 token 验签需要）。

---

## 7. JWT Claims 规范

### 7.1 access_token（JWT, RS256）

```json
{
  "iss": "https://arkloop.example.com",
  "sub": "<user.id UUID>",
  "aud": "exam-web",
  "azp": "exam-web",                 // authorized party
  "scope": "openid exam:read exam:write",
  "iat": 1716095100,
  "exp": 1716096000,                 // iat + 900s
  "jti": "<uuid>",                   // 唯一 ID，便于审计/去重
  "client_id": "exam-web"
}
```

### 7.2 id_token（JWT, RS256）

```json
{
  "iss": "https://arkloop.example.com",
  "sub": "<user.id UUID>",
  "aud": "exam-web",
  "iat": 1716095100,
  "exp": 1716098700,                 // iat + 3600s
  "auth_time": 1716094800,           // 用户实际登录 ArkLoop 的时间
  "nonce": "<echo from authorize>",  // 防 replay
  "email": "user@example.com",
  "email_verified": true,
  "name": "Zhang San",
  "picture": "https://..."
}
```

### 7.3 refresh_token

不是 JWT，是 opaque 字符串（Base64URL of 32 random bytes）；SHA256 后入 `oauth_refresh_tokens.token_hash`。

---

## 8. 关键流程序列图

### 8.1 Flow 1: SSO 登录（浏览器）

```
浏览器          exam-frontend      exam-backend      ArkLoop-API
  │ ① 访问 exam 首页    │                 │                 │
  ├────────────────────►│                 │                 │
  │ 302 → /api/auth/oidc/authorize                          │
  │◄────────────────────┤                 │                 │
  │                     │                 │                 │
  │ ② GET /api/auth/oidc/authorize                          │
  ├────────────────────────────────────────►│               │
  │                     │ 302 → ArkLoop /oauth/authorize    │
  │                     │       ?client_id=exam-web         │
  │                     │       &state=<csrf>               │
  │                     │       &code_challenge=<PKCE>      │
  │◄─────────────────────────────────────── │               │
  │                                                          │
  │ ③ GET /v1/auth/oauth/authorize?...                      │
  ├──────────────────────────────────────────────────────────►│
  │                          [未登录 → 跳 ArkLoop /login]    │
  │  ... 登录 ArkLoop ...                                    │
  │                          [已登录 → 检查 consent]         │
  │                          [无 consent → 渲染同意页]       │
  │ ④ POST /oauth/consent (同意)                             │
  ├──────────────────────────────────────────────────────────►│
  │                                       生成 code，入库     │
  │ 302 → exam /api/auth/oidc/callback?code=...&state=...    │
  │◄──────────────────────────────────────────────────────── │
  │                                                          │
  │ ⑤ GET /api/auth/oidc/callback?code=...&state=...        │
  ├────────────────────────────────────────►│               │
  │                     │ ⑥ POST /v1/auth/oauth/token       │
  │                     │   grant=code, code_verifier=...   │
  │                     ├──────────────────────────────────►│
  │                     │     access_token + id_token       │
  │                     │◄──────────────────────────────────┤
  │                     │ ⑦ 验签 id_token（拉 JWKS）       │
  │                     │ ⑧ auto-provision user             │
  │                     │    INSERT users (oidc_subject=sub, │
  │                     │      email=..., persona='teacher')│
  │                     │ ⑨ 颁发 exam 本地 session/JWT     │
  │                     │ 302 → /, Set-Cookie               │
  │◄────────────────────┤                                    │
```

### 8.2 Flow 2: Refresh Token Rotation

```
client                ArkLoop /oauth/token
  │ POST grant=refresh_token, refresh_token=R1     │
  ├───────────────────────────────────────────────►│
  │                            ① 查 R1 hash       │
  │                            ② 检查 expired / revoked │
  │                            ③ 生成 R2，R1.rotated_to=R2.hash │
  │                            ④ R1.revoked_at=now()  │
  │                            ⑤ 颁发新 access_token + R2 │
  │   { access_token, refresh_token: R2, ... }     │
  │◄───────────────────────────────────────────────┤
```

**重放检测**：若收到已经 revoked 的 R1 试图换 token，立刻 revoke 整条 rotation 链（沿着 rotated_to 一路撤销），认为被盗用。

### 8.3 Flow 3: 智能体内部铸 token

```
worker (exam-agent)           ArkLoop-API                exam-backend
  │                                │                          │
  │ 用户触发"创建课程目录"          │                          │
  │ ① POST /internal/oauth/issue   │                          │
  │   { user_id, client_id=exam-web, scopes=[openid, exam:write] } │
  │ Authorization: Bearer <SERVICE_TOKEN>                     │
  ├────────────────────────────────►│                         │
  │                                ② 验 service token         │
  │                                ③ 验 scopes 在白名单内      │
  │                                  （不含 exam:admin）       │
  │                                ④ 查 oauth_clients(exam-web)│
  │                                ⑤ 签发 access_token (TTL 60s)│
  │   { access_token, expires_in: 60 }                        │
  │◄───────────────────────────────┤                          │
  │                                                            │
  │ ⑥ POST /learning/knowledge-points                         │
  │ Authorization: Bearer <access_token>                       │
  ├───────────────────────────────────────────────────────────►│
  │                                       ⑦ 验签（拉 JWKS）   │
  │                                       ⑧ 校验 scope:exam:write│
  │                                       ⑨ 用 sub 解析出 user│
  │                                          → CurrentUser     │
  │                                       ⑩ 执行业务逻辑       │
  │   { id, name, ... }                                       │
  │◄───────────────────────────────────────────────────────────┤
```

### 8.4 Flow 4: Consent 撤销

```
用户在 ArkLoop /settings/connected-apps 点"撤销 exam"
  │
  ▼
ArkLoop:
  ① oauth_consents.revoked_at = now()
  ② 撤销该 user × client 所有 oauth_refresh_tokens
  ③ access_token 无法主动撤销，靠 15min TTL 自然过期
  ④ exam 端：下次 access_token 过期且 refresh 失败 → 用户被踢出
```

### 8.5 Flow 5: 用户登出（轻量版）

v1 不实现 OIDC Front-channel / Back-channel logout。用户在 exam 点登出 → exam 清自己 session；用户在 ArkLoop 点登出 → ArkLoop 清自己 session。两边独立，依赖 access_token 短 TTL 兜底。

v2 再实现 RP-Initiated Logout（OIDC Session Management）。

---

## 9. 安全模型

### 9.1 强制要求
| 防御 | 实现 |
|---|---|
| CSRF | `state` 参数强制 ≥16 字符；exam 端 callback 必须校验 |
| Authorization Code Interception | PKCE S256 强制；非 PKCE 请求直接 400 |
| Token Replay | refresh_token rotation；id_token.nonce 校验 |
| Open Redirect | redirect_uri 精确字符串匹配（含 query string、scheme、端口） |
| Token 泄露影响面 | access_token TTL 15min；scope 最小化；`exam:admin` 永远不发给智能体 |
| Phishing client | client_secret bcrypt 存储；token endpoint 强制 client 鉴权 |
| Replay attack on internal endpoint | service token 验证；rate limit（per-source IP）；审计日志 |
| Key compromise | 密钥 rotation 工具；JWKS 同时返回多个 active key 平滑过渡 |
| Race condition on authorization code | code_hash unique constraint + consumed_at 单调写；竞争失败 fail-closed |

### 9.2 不在 v1 范围（接受的风险）
- **JWE 加密 token**：v1 仅签名不加密；token 内容可被中间环节读到。前提是 token 不含敏感数据（仅 sub + scope）
- **Dynamic Client Registration**：v1 仅管理员可注册 client
- **MTLS client auth**：v1 仅 client_secret_post/basic

---

## 10. 密钥管理

### 10.1 启动
worker 起步时若 `oidc_signing_keys` 表为空：生成 RSA 4096 keypair，私钥通过 `sharedencryption` 包加密入库，status='active'。

### 10.2 Rotation
管理接口 `POST /admin/oidc/keys/rotate`：
1. 生成新 keypair，status='active'，写入 DB
2. 老 key status='retired'（仍参与 JWKS，让旧 token 能验签直到自然过期）
3. 等过 access_token + refresh_token 最大 TTL（30d + 15m）后，老 key 可标 `retired_at` 不再返回 JWKS

签发时永远用最新 active key；验签时按 token header.kid 在 active+retired 集合中找。

### 10.3 紧急撤销
若怀疑私钥泄露：管理接口 `POST /admin/oidc/keys/{kid}/compromise`：
1. 该 key status='compromised'
2. 立刻从 JWKS 移除
3. 撤销所有用该 kid 签的 refresh_token
4. 用户被强制重新登录

---

## 11. 错误码

OAuth 标准错误（RFC 6749 §4.1.2.1, §5.2）：
- `invalid_request` — 缺少参数
- `unauthorized_client` — client 无权使用该 grant_type
- `access_denied` — 用户拒绝同意
- `unsupported_response_type` — 非 code flow
- `invalid_scope` — scope 超出 client.allowed_scopes
- `invalid_grant` — code 无效/过期/已用；refresh_token 无效
- `invalid_client` — client_id/secret 错误

内部端点扩展：
- `arkloop.scope_not_allowed_for_internal` — 申请了白名单外 scope（如 admin）
- `arkloop.service_token_invalid`

---

## 12. 监控指标（Prometheus）

```
arkloop_oidc_authorize_total{client_id, result}     # result=success|denied|error
arkloop_oidc_token_total{client_id, grant_type, result}
arkloop_oidc_internal_issue_total{client_id, result}
arkloop_oidc_active_refresh_tokens{client_id}       # gauge
arkloop_oidc_signing_key_age_seconds                # 检测漏 rotation
arkloop_oidc_token_verify_failures_total{reason}    # exam 端，验签失败原因分布
```

告警规则：
- `internal_issue` 错误率 > 1% / 5min → P2 告警（智能体功能受损）
- `token_verify_failures` 异常飙升 → P1（可能密钥同步问题或攻击）

---

## 13. 回滚预案

| 故障 | 回滚策略 |
|---|---|
| ArkLoop OIDC 端点 5xx | 暂停 exam 灰度，exam 退回本地账号登录（已保留路径） |
| JWKS 拉取失败 | exam 本地 cache JWKS 24h，单实例故障不影响 |
| 密钥泄露 | §10.3 紧急撤销流程 |
| 智能体内部端点异常 | worker 工具自身降级：返回明确错误"exam 暂不可用"，不假装成功 |
| Migration 失败 | 所有 migration 必须有 `-- +goose Down`；先备份再上生产 |

---

## 14. 灰度方案

1. **Stage 1**：ArkLoop 实现 OIDC 端点 + JWKS，但 exam 仍只用本地账号
2. **Stage 2**：exam 接入 OIDC，登录页加按钮但默认走本地登录
3. **Stage 3**：少量用户开启 SSO，本地登录路径保留
4. **Stage 4**：智能体 `exam-agent` 上线（仅对启用 SSO 的用户生效）
5. **Stage 5**：观察 1-2 周稳定后，新用户默认 SSO，旧用户保留双登录

---

## 15. 开放问题（评审重点）

请评审时关注：
- [ ] **issuer URL** 用什么？建议 `https://arkloop.example.com`，但 ArkLoop 是否有正式域名规划？测试环境的 issuer 配置？
- [ ] **exam-web client_secret** 怎么发？建议生成后通过 1Password 或 secret manager 发布，禁明文邮件
- [ ] **首次 ArkLoop OIDC 启用时** users 表的存量用户没有 `email_verified` 是否需要补：默认 FALSE，依据 `email_verified_at` 回填？✓（已在 migration 00190 处理）
- [ ] **exam 端 user 自动建账时**，若 ArkLoop user.email 与 exam 已有本地账号 email 冲突，怎么办？建议：标记冲突日志、拒绝自动建账、引导用户手动 link
- [ ] **审计与合规**：consent 授权/撤销是否进 `activity_logs`？建议 ✓
- [ ] **是否需要 `prompt=none` 静默重新认证**支持？（用于刷新 id_token 而不用 refresh_token；浏览器场景）建议 v1 不实现

---

## 附录 A：参考标准
- OpenID Connect Core 1.0
- OAuth 2.1 (draft)
- RFC 6749 - OAuth 2.0 Framework
- RFC 6750 - Bearer Token Usage
- RFC 7009 - Token Revocation
- RFC 7517 - JSON Web Key (JWK)
- RFC 7636 - PKCE
- RFC 8414 - Authorization Server Metadata
- RFC 8693 - Token Exchange（未在 v1 使用，备选 §6.4 替代方案）

## 附录 B：术语
- **IdP** (Identity Provider): ArkLoop 本身
- **RP** (Relying Party): exam，OIDC 客户端
- **UA** (User Agent): 浏览器
- **Confidential Client**: 能保管 secret 的 client（后端）
- **Public Client**: 浏览器/原生 app（无法保管 secret，强制 PKCE）

