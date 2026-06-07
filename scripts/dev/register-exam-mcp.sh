#!/usr/bin/env bash
#
# register-exam-mcp.sh — 把 exam 的 stdio MCP Server 注册到 ArkLoop（本地联调）。
#
# 背景：exam 侧已实现 stdio MCP Server（app.job_models.agent_mcp），暴露
#   exam_list_knowledge_points / exam_list_question_banks /
#   exam_ensure_course_question_bank / exam_list_questions /
#   exam_save_questions / exam_create_paper
# 本系统（ArkLoop）已具备完整的 MCP 客户端 + 发现 + 连接器能力，无需改 Go 代码——
# 只需注册一个 profile MCP install。worker 下次 run 即发现这些工具（带
# mcp__<install_key>__ 前缀），供「智能组卷 / 录题助手 / 命题专家」persona 使用。
#
# 这条流程只做一件事：以当前老师身份 POST /v1/mcp-installs（launch_spec + 加密 env 凭据）。
#
# 必填环境变量：
#   ARKLOOP_API_URL   ArkLoop API 基址，如 http://localhost:19001
#   ARKLOOP_TOKEN     当前老师/管理员 access token（用于 Authorization: Bearer）
#   EXAM_BACKEND_DIR  exam 后端目录（stdio 进程 cwd），如 /Users/jzefan/work/proj/exam/backend
#   EXAM_AGENT_TOKEN  exam 侧凭据（注入 MCP 进程 env，经 ArkLoop 加密存储）
#
# 可选：
#   EXAM_AGENT_BASE_URL  exam 后端基址（默认 http://localhost:8000）
#   EXAM_AGENT_ORG_ID    组织隔离 id（默认不传）
#   INSTALL_KEY          install key（默认 exam-agent；决定工具前缀 mcp__exam_agent__）
#
# 前置：exam 后端可本地启动；worker 主机能执行 `uv`（stdio install 默认 host=cloud_worker）。
# 注意：迁移期内置直连 exam_* 工具仍可作为回退（ARKLOOP_EXAM_INTEGRATION_ENABLED=true）；
#       persona prompt 已要求优先走 MCP、不绕过。完全切换后把该开关设为 false 即可。

set -euo pipefail

: "${ARKLOOP_API_URL:?需要 ARKLOOP_API_URL，如 http://localhost:19001}"
: "${ARKLOOP_TOKEN:?需要 ARKLOOP_TOKEN（老师/管理员 access token）}"
: "${EXAM_BACKEND_DIR:?需要 EXAM_BACKEND_DIR（exam 后端目录）}"
: "${EXAM_AGENT_TOKEN:?需要 EXAM_AGENT_TOKEN（exam 侧凭据）}"

EXAM_AGENT_BASE_URL="${EXAM_AGENT_BASE_URL:-http://localhost:8000}"
INSTALL_KEY="${INSTALL_KEY:-exam-agent}"

command -v jq >/dev/null || { echo "需要 jq" >&2; exit 1; }

env_secrets=$(jq -n \
  --arg tok "$EXAM_AGENT_TOKEN" \
  --arg org "${EXAM_AGENT_ORG_ID:-}" \
  '{EXAM_AGENT_TOKEN: $tok} + (if $org == "" then {} else {EXAM_AGENT_ORG_ID: $org} end)')

payload=$(jq -n \
  --arg key "$INSTALL_KEY" \
  --arg cwd "$EXAM_BACKEND_DIR" \
  --arg base "$EXAM_AGENT_BASE_URL" \
  --argjson secrets "$env_secrets" \
  '{
    install_key: $key,
    display_name: "Exam Agent",
    transport: "stdio",
    launch_spec: {
      command: "uv",
      args: ["run", "python", "-m", "app.job_models.agent_mcp"],
      cwd: $cwd,
      env: { PYTHONPATH: "src", EXAM_AGENT_BASE_URL: $base }
    },
    env_secrets: $secrets
  }')

echo "POST ${ARKLOOP_API_URL}/v1/mcp-installs  (install_key=${INSTALL_KEY})"
curl -fsS -X POST "${ARKLOOP_API_URL}/v1/mcp-installs" \
  -H "Authorization: Bearer ${ARKLOOP_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "$payload" | jq .

prefix="mcp__${INSTALL_KEY//-/_}__"
cat <<EOF

✓ 已注册 exam MCP install（${INSTALL_KEY}）。
  worker 下次 run 会发现工具：${prefix}exam_list_knowledge_points / ${prefix}exam_list_questions /
  ${prefix}exam_ensure_course_question_bank / ${prefix}exam_save_questions / ${prefix}exam_create_paper 等。
  在 console / web 的 MCP 管理页可查看发现状态（discovery_status）。
EOF
