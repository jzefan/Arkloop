#!/usr/bin/env bash
set -euo pipefail

# One-command deploy: sync source to remote server, build and run there.
# No CI, no GHCR — just local code → remote server.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

REMOTE=""
REMOTE_DIR="~/arkloop"
GATEWAY_PORT="20083"
SSH_OPTS=()

usage() {
  cat <<'EOF'
Usage:
  scripts/deploy-source-to-server.sh <user@server-ip> [options]

Options:
  --remote-dir <dir>       Remote directory (default: ~/arkloop)
  --gateway-port <port>    Gateway port (default: 20083)
  --ssh-option <option>    Extra ssh option, repeatable
  -h, --help               Show this help

Example:
  scripts/deploy-source-to-server.sh xht2020@218.244.152.142
  scripts/deploy-source-to-server.sh xht2020@218.244.152.142 --gateway-port 3037
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --remote-dir) REMOTE_DIR="$2"; shift 2 ;;
    --gateway-port) GATEWAY_PORT="$2"; shift 2 ;;
    --ssh-option) SSH_OPTS+=("$2"); shift 2 ;;
    -h|--help) usage; exit 0 ;;
    -*)  echo "Unknown option: $1" >&2; exit 2 ;;
    *)
      if [ -n "$REMOTE" ]; then echo "Unexpected: $1" >&2; exit 2; fi
      REMOTE="$1"; shift ;;
  esac
done

[ -n "$REMOTE" ] || { usage >&2; exit 2; }

ssh_cmd() { ssh "${SSH_OPTS[@]+"${SSH_OPTS[@]}"}" "$REMOTE" "$@"; }

echo "==> Syncing source to ${REMOTE}:${REMOTE_DIR}..."
rsync -az --delete \
  --exclude='.git' \
  --exclude='node_modules' \
  --exclude='**/node_modules' \
  --exclude='.env' \
  --exclude='release-files' \
  --exclude='.cache' \
  --exclude='**/dist' \
  --exclude='src/apps/desktop/release' \
  --exclude='arkloop-deploy-*' \
  -e "ssh ${SSH_OPTS[*]+"${SSH_OPTS[*]}"}" \
  "$ROOT_DIR/" "${REMOTE}:${REMOTE_DIR}/"

echo "==> Stopping old services..."
ssh_cmd "cd ${REMOTE_DIR} && docker compose down --remove-orphans 2>/dev/null || true"

echo "==> Installing (build from source)..."
ssh_cmd "cd ${REMOTE_DIR} && ./setup.sh install \
  --profile standard \
  --mode self-hosted \
  --memory none \
  --sandbox none \
  --console lite \
  --browser off \
  --web-tools builtin \
  --gateway on \
  --gateway-port ${GATEWAY_PORT} \
  --non-interactive"

echo "==> Done! http://<server-ip>:${GATEWAY_PORT}"
