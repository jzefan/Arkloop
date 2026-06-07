#!/usr/bin/env bash
#
# register-exam-mcp-http.sh —— 把 exam 的 streamable_http MCP Server 注册成 ArkLoop 连接器（per-user）。
#
# "注册连接器" = 在 ArkLoop 里写一条配置记录，告诉 worker：exam 的 MCP 服务在某 HTTP 地址，
# 用 streamable_http 去连。注册后 worker 自动发现 exam 工具（mcp__exam_agent__*），
# 调用时按当前老师身份现铸 token 注入（前提：worker 已配 ARKLOOP_MCP_OIDC_SERVERS=exam-agent 并重启）。
#
# per-user 不带任何静态 token —— 身份由 worker 每次调用注入。
#
# 必填 env：
#   ARKLOOP_URL    ArkLoop 对外地址。compose 默认只开 Gateway → http://localhost:19000
#                  （直接跑服务则 API 在 http://localhost:19001）
#   EXAM_MCP_URL   exam MCP server 的 HTTP 端点，且**从 worker 容器内可达**。
#                  exam 在宿主机另一套 compose 时常用：http://host.docker.internal:8000/mcp
#   鉴权二选一：
#     ARKLOOP_TOKEN              直接给一个 ArkLoop access token（Bearer）
#   或  ARKLOOP_LOGIN + ARKLOOP_PASSWORD   用账号现登一个（脚本帮你换 token）
#
# 可选：
#   INSTALL_KEY    默认 exam-agent（必须与 worker 的 ARKLOOP_MCP_OIDC_SERVERS 一致）

set -euo pipefail
command -v jq >/dev/null || { echo "需要 jq" >&2; exit 1; }

: "${ARKLOOP_URL:?需要 ARKLOOP_URL，如 http://localhost:19000}"
: "${EXAM_MCP_URL:?需要 EXAM_MCP_URL（exam MCP 的 HTTP 端点，worker 容器内可达）}"
INSTALL_KEY="${INSTALL_KEY:-exam-agent}"

# 1) 拿 access token
if [ -z "${ARKLOOP_TOKEN:-}" ]; then
  : "${ARKLOOP_LOGIN:?需要 ARKLOOP_TOKEN，或 ARKLOOP_LOGIN + ARKLOOP_PASSWORD}"
  : "${ARKLOOP_PASSWORD:?需要 ARKLOOP_PASSWORD}"
  echo "→ 登录 ${ARKLOOP_URL}/v1/auth/login (${ARKLOOP_LOGIN})"
  ARKLOOP_TOKEN=$(curl -fsS -X POST "${ARKLOOP_URL}/v1/auth/login" \
    -H "Content-Type: application/json" \
    -d "$(jq -n --arg l "$ARKLOOP_LOGIN" --arg p "$ARKLOOP_PASSWORD" '{login:$l, password:$p}')" \
    | jq -r '.access_token')
  [ -n "$ARKLOOP_TOKEN" ] && [ "$ARKLOOP_TOKEN" != "null" ] || { echo "登录失败：没拿到 access_token" >&2; exit 1; }
fi

# 2) 注册 streamable_http 连接器（不带任何静态 token）
echo "→ POST ${ARKLOOP_URL}/v1/mcp-installs  (install_key=${INSTALL_KEY}, transport=streamable_http)"
curl -fsS -X POST "${ARKLOOP_URL}/v1/mcp-installs" \
  -H "Authorization: Bearer ${ARKLOOP_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "$(jq -n --arg key "$INSTALL_KEY" --arg url "$EXAM_MCP_URL" '{
        install_key: $key,
        display_name: "Exam Agent",
        transport: "streamable_http",
        launch_spec: { transport: "streamable_http", url: $url }
      }')" | jq .

cat <<EOF

✓ 已注册。检查发现状态：
  docker exec arkloop-postgres-1 psql -U arkloop -d arkloop -tAc \\
    "select install_key, transport, discovery_status from profile_mcp_installs where install_key='${INSTALL_KEY}';"
  期望：${INSTALL_KEY} | streamable_http | ok
若 discovery_status 不是 ok：多半是 EXAM_MCP_URL 从 worker 容器内不可达（换 host.docker.internal/服务名），
或 exam MCP server 没起/路径不对。
EOF
