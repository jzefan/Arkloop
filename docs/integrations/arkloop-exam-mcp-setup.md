# ArkLoop 侧接入 exam MCP（per-user）操作清单

> 前提：exam 已按 `exam-mcp-server-changes.md` 改完——streamable_http + 按请求校验老师 Bearer。
> 这份是 ArkLoop 侧的"注册连接器 + 开开关 + 验证"，全部可复制执行。
> 关键一致性：`install_key` = `ARKLOOP_MCP_OIDC_SERVERS` = `exam-agent`；
> `ARKLOOP_MCP_OIDC_CLIENT_ID` = exam 的 `EXAM_MCP_EXPECTED_AUD`（都用 `exam-web`）。

---

## 1) worker 加 per-user 鉴权 env

在 worker 服务的环境里（`.env` 或 compose.yaml 的 worker service）加：

```bash
# 启用 per-user 注入：值必须等于下面注册用的 install_key
ARKLOOP_MCP_OIDC_SERVERS=exam-agent
# 老师 token 的 aud / client_id：必须和 exam 的 EXAM_MCP_EXPECTED_AUD 一致
ARKLOOP_MCP_OIDC_CLIENT_ID=exam-web
# 可选，默认就是这个
ARKLOOP_MCP_OIDC_SCOPES=openid exam:read exam:write
```

确认这两项已存在（现有 exam REST 集成本来就在用；铸 token 靠它们）：

```bash
ARKLOOP_INTERNAL_SERVICE_TOKEN=...        # worker→ArkLoop API 的服务凭据
ARKLOOP_API_INTERNAL_URL=http://api:19001 # 默认即此，可不填
```

> worker 是**进程启动时读一次** env → **改完要重启 worker**。

可选：迁移期想让内置直连 REST 工具退场，把 `ARKLOOP_EXAM_INTEGRATION_ENABLED` 设为非 `true`（隐藏内置 `exam_*`，只留 MCP 路）。想保留 REST 回退就先不动。

## 2) 注册 exam MCP 连接器（streamable_http，无静态 token）

per-user 的 token 由 worker 每次调用现铸注入，所以**注册时不带任何 auth/bearer**。
以**当前老师/管理员**身份调一次 ArkLoop API：

```bash
ARKLOOP_API_URL=http://localhost:19001          # ArkLoop API 基址
ARKLOOP_TOKEN=<老师/管理员 access token>          # 调注册 API 用
EXAM_MCP_URL=http://localhost:8000/mcp           # exam MCP server 的 HTTP 端点（按 exam 实际填）

curl -fsS -X POST "${ARKLOOP_API_URL}/v1/mcp-installs" \
  -H "Authorization: Bearer ${ARKLOOP_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "$(jq -n --arg url "$EXAM_MCP_URL" '{
        install_key: "exam-agent",
        display_name: "Exam Agent",
        transport: "streamable_http",
        launch_spec: { transport: "streamable_http", url: $url }
      }')" | jq .
```

- `install_key: exam-agent` → 工具前缀 `mcp__exam_agent__`，也匹配第 1 步的 `ARKLOOP_MCP_OIDC_SERVERS`。
- 不传 `env_secrets` / `bearer_token`：身份走 per-user 注入。
- stdio 单 token 的旧脚本 `scripts/dev/register-exam-mcp.sh` 只用于"单人本地联调"，per-user 用这条 HTTP 注册，二选一。

## 3) 验证

```bash
# a) install 已登记、发现成功
docker exec arkloop-postgres-1 psql -U arkloop -d arkloop -tAc \
  "select install_key, transport, discovery_status from profile_mcp_installs where install_key='exam-agent';"
#   期望：exam-agent | streamable_http | ok

# b) 用「智能组卷」persona 走一遍出题→保存，确认调用的是 mcp__exam_agent__exam_save_questions，
#    且题目出现在 exam（用对应老师账号登录 exam 前台查），不再进本地「组卷题库」。

# c) 两个不同老师各保存一批 → 在 exam 里各自只看到自己的（per-user 隔离生效）。
```

## 已知坑

- **SSRF/loopback**：worker 的出站策略默认可能拦 `127.0.0.1/localhost`。本地联调若 discovery 连不上 exam MCP，
  用宿主机局域网 IP（或容器服务名，如 `http://exam:8000/mcp`）替代 loopback，或放行出站策略。
- **aud 不一致**：`ARKLOOP_MCP_OIDC_CLIENT_ID` 与 exam 的 `EXAM_MCP_EXPECTED_AUD` 不同会被 exam 拒签——两边都用 `exam-web`。
- **server id 不一致**：`ARKLOOP_MCP_OIDC_SERVERS` 必须等于 `install_key`（`exam-agent`），否则不注入 per-user 头、退回无鉴权。
- **改 env 没重启 worker**：配置懒加载，必须重启。

## 串起来的全链路（确认）

```
智能组卷 persona（默认存 exam）
  → 调 mcp__exam_agent__exam_save_questions
    → worker 现铸该老师 60s OIDC token（/internal/oauth/issue）注入 Authorization
      → streamable_http 发给 exam MCP server
        → exam 验签取老师身份 → 写入该老师课程题库
读国考参考种子：exam server 用自己的只读 admin 身份，不碰老师权限、不污染国考库
```
