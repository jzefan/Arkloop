# 给 exam 团队：MCP Server 改造提示词（per-user 鉴权）

> 这是交给 exam 后端（`agent_mcp.py` 所在仓库）的实现说明。整段可直接发给 Codex / 开发。
> 对端：ArkLoop（智能体平台，作为外部智能体经 MCP 调用 exam）。

---

## 背景与目标

ArkLoop 现在以"每个老师用自己的身份"调用你们的 Exam MCP Server。请把 MCP Server 从
"启动时吃一个固定 `EXAM_AGENT_TOKEN`、所有人共用"改为
"**每个 MCP 请求携带该老师的 Bearer token，按请求校验并以该老师身份操作**"。

身份语义和你们 REST 的 OIDC SSO **完全一样**——ArkLoop 就是那个 OIDC 身份提供方，
这枚 Bearer 就是 ArkLoop 签发的 OIDC access_token（和老师走浏览器 SSO 拿到的同类）。
所以**复用你们 REST 已有的 OIDC 校验逻辑即可**，不是新东西。

## 要做的三件事

### 1) 传输：stdio → streamable_http

stdio 在进程启动时就把 token 定死了，无法按请求携带身份。改成 **streamable_http**
（HTTP POST 收 JSON-RPC，响应可为 JSON 或 SSE）。这样每个请求才能带 `Authorization` 头。

### 2) 按请求校验 Bearer（核心）

每个 MCP 请求会带：`Authorization: Bearer <jwt>`。按 OIDC 校验这枚 JWT：

- **算法**：RS256
- **取公钥（JWKS）**：`GET {ARKLOOP_OIDC_ISSUER}/.well-known/jwks.json`（缓存公钥，按 `kid` 选）
  - 元数据：`GET {ARKLOOP_OIDC_ISSUER}/.well-known/openid-configuration`
  - 本地联调 `ARKLOOP_OIDC_ISSUER = http://localhost:19000`（做成可配置）
- **校验**：签名有效；`iss == ARKLOOP_OIDC_ISSUER`；`aud == <你们期望的 client_id>`（ArkLoop 默认发 `exam-web`，确认/对齐这个值）；`exp` 未过期（注意 TTL 只有 60s，属正常，按请求来一枚）。
- **取身份**：从 claims 取 `sub`（= ArkLoop 用户 id）、`email`、`name`、`preferred_username` →
  映射到 / **首次自动开户**对应的 exam 老师账号（和你们 REST OIDC SSO 的自动开户逻辑一致）。
- token 里还有 `scope`（如 `openid exam:read exam:write`）、`internal_issue: true` 标记，可用于审计。
- 校验失败（缺失/过期/签名错/aud 不符）→ 返回 MCP 工具错误（结构化 error），ArkLoop 会原样转述给老师。

> 一句话：把你们 REST 中间件对 OIDC token 的"验签+取用户+自动开户"那套，搬到 MCP Server 的请求入口。

### 3) 按工具区分身份（关键设计，避免污染国考库）

医学国考题库在 **管理员名下、不共享给单个老师**。它是**只读参考**（出题取种子用）。
**不要**把它读写共享给所有 ArkLoop 用户——尤其不能给写权限，否则每个老师 AI 生成的草稿会污染权威国考库。

正确做法是 **MCP Server 按工具用不同身份**：

| MCP 工具 | 用哪种身份 | 目标 |
|---|---|---|
| `exam_list_knowledge_points` | 老师 Bearer | 老师可见知识点 |
| `exam_list_question_banks` | 老师 Bearer | 老师可见题库 |
| `exam_ensure_course_question_bank` | 老师 Bearer | 老师的**课程题库** |
| `exam_save_questions`（写） | 老师 Bearer | 写入老师的**课程题库**（**绝不写国考库**）|
| `exam_create_paper`（写） | 老师 Bearer | 老师的课程/试卷 |
| `exam_list_questions` —— 读**老师课程库** | 老师 Bearer | 老师自己的题 |
| 读**国考参考库**取种子题 | **Server 自己的 admin/service 只读身份** | 管理员名下国考库（**只读**，与老师身份无关）|

要点：
- **写**永远落在老师有权限的**课程题库**（`exam_ensure_course_question_bank` 建/取的那个），per-user 隔离、可审计。
- **读国考参考库**用 **Server 端自己的只读管理员/服务凭据**（保留一份专用 admin/service credential 给这条只读路径），老师**不需要、也不应该**对国考 admin 库有直接权限。
- 这样既保留"所有老师都能用国考题做参考"，又**不共享国考库给任何人、不被污染**。

## 配置（exam 侧新增 env）

```
ARKLOOP_OIDC_ISSUER=http://localhost:19000      # 验签 issuer + JWKS base（生产换成公网地址）
EXAM_MCP_EXPECTED_AUD=exam-web                  # 期望的 aud（= ArkLoop 发 token 的 client_id，需对齐）
EXAM_REFERENCE_ADMIN_TOKEN=<只读国考库的服务凭据>  # 仅用于“读国考参考库”这条服务端路径
```

`EXAM_AGENT_TOKEN`（原 stdio 单一 token）作为身份来源可移除；如需保留为本地单人联调的兜底模式可选。

## 验收

1. 用两个不同老师的 Bearer 分别调 `exam_save_questions` → 各自的题进各自课程题库，互不可见。
2. 读国考种子题正常（走 server admin 只读），但任何老师 Bearer **无法**写国考库。
3. 过期/伪造 Bearer → 明确鉴权错误。
4. 传输为 streamable_http，`Authorization` 按请求生效。

## 参考

- ArkLoop 侧已实现"每次调用现铸老师 token 并注入 `Authorization`"（`worker/internal/mcp/peruser.go`）。
- token 由 ArkLoop `POST /internal/oauth/issue` 现铸，RS256，TTL 60s，带 `sub/email/name/scope` claims。
- 业务字段/错误码/partial-success 等沿用 `docs/integrations/exam-api.md`。
