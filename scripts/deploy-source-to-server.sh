#!/usr/bin/env bash
set -euo pipefail

# Simple full deploy: sync source, clean everything, run setup.sh install.
# No incremental tricks. Clean slate every time. Just works.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

REMOTE=""
REMOTE_DIR="~/arkloop"
GATEWAY_PORT="20083"
WEB_PORT="20084"
WEB_TOOLS="searxng"
RESET_DATA=0
SSH_OPTS=()
SSH_PASS=""

usage() {
  cat <<'EOF'
Usage:
  scripts/deploy-source-to-server.sh <user@server-ip> [options]

Options:
  --remote-dir <dir>       Remote directory (default: ~/arkloop)
  --gateway-port <port>    Gateway port (default: 20083)
  --web-port <port>        Web app port (default: 20084)
  --web-tools <mode>       Web search/fetch mode: searxng, self-hosted, or builtin (default: searxng)
  --reset-data             Drop docker volumes (postgres data, etc.) before install.
                           DESTRUCTIVE: deletes all business data. Use for clean reinstall.
  --password <pass>        SSH password (requires sshpass installed locally)
  --ssh-option <option>    Extra ssh option, repeatable
  -h, --help               Show this help
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --remote-dir) REMOTE_DIR="$2"; shift 2 ;;
    --gateway-port) GATEWAY_PORT="$2"; shift 2 ;;
    --web-port) WEB_PORT="$2"; shift 2 ;;
    --web-tools) WEB_TOOLS="$2"; shift 2 ;;
    --reset-data) RESET_DATA=1; shift ;;
    --password) SSH_PASS="$2"; shift 2 ;;
    --ssh-option) SSH_OPTS+=("$2"); shift 2 ;;
    -h|--help) usage; exit 0 ;;
    -*)  echo "Unknown option: $1" >&2; exit 2 ;;
    *)
      if [ -n "$REMOTE" ]; then echo "Unexpected: $1" >&2; exit 2; fi
      REMOTE="$1"; shift ;;
  esac
done

[ -n "$REMOTE" ] || { usage >&2; exit 2; }

SSH_PREFIX=()
if [ -n "$SSH_PASS" ]; then
  command -v sshpass >/dev/null 2>&1 || { echo "Error: sshpass not installed. Run: brew install sshpass (macOS) or apt install sshpass (Linux)" >&2; exit 1; }
  SSH_PREFIX=(sshpass -p "$SSH_PASS")
fi

ssh_cmd() { "${SSH_PREFIX[@]+"${SSH_PREFIX[@]}"}" ssh "${SSH_OPTS[@]+"${SSH_OPTS[@]}"}" "$REMOTE" "$@"; }

case "$WEB_TOOLS" in
  searxng|self-hosted|builtin) ;;
  *) echo "Unsupported --web-tools: ${WEB_TOOLS}" >&2; exit 2 ;;
esac

SETUP_WEB_TOOLS="$WEB_TOOLS"
if [ "$WEB_TOOLS" = "searxng" ]; then
  SETUP_WEB_TOOLS="builtin"
fi

RSYNC_RSH="ssh${SSH_OPTS[*]+ ${SSH_OPTS[*]}}"
if [ -n "$SSH_PASS" ]; then
  RSYNC_RSH="sshpass -p $SSH_PASS $RSYNC_RSH"
fi

# Detect Python on remote
REMOTE_PYTHON=""
for _py in python3.12 python3 python; do
  if ssh_cmd "command -v $_py" >/dev/null 2>&1; then
    REMOTE_PYTHON="$_py"
    break
  fi
done
[ -n "$REMOTE_PYTHON" ] || { echo "Error: Python 3.7+ not found on remote server." >&2; exit 1; }

echo "==> Syncing source to ${REMOTE}:${REMOTE_DIR}..."
rsync -az --delete \
  --exclude='.git' \
  --exclude='node_modules' \
  --exclude='**/node_modules' \
  --exclude='.env' \
  --exclude='install/.state.env' \
  --exclude='release-files' \
  --exclude='.cache' \
  --exclude='**/dist' \
  --exclude='src/apps/desktop/release' \
  --exclude='arkloop-deploy-*' \
  -e "$RSYNC_RSH" \
  "$ROOT_DIR/" "${REMOTE}:${REMOTE_DIR}/"

echo "==> Ensuring ARKLOOP_INTERNAL_SERVICE_TOKEN is set in remote .env..."
# Background: worker calls /internal/oauth/issue on api with this token to mint
# 60-second exam access tokens on behalf of the current ArkLoop user. If it is
# missing or still a placeholder, both api (via compose :? gate) and worker
# would refuse to start. Generate one on first deploy and persist it; leave
# untouched on subsequent deploys.
ssh_cmd "cd ${REMOTE_DIR} && ${REMOTE_PYTHON} - <<'PY'
import secrets
from pathlib import Path

KEY = 'ARKLOOP_INTERNAL_SERVICE_TOKEN'
PLACEHOLDERS = {
    '',
    'please_generate_with_openssl_rand_base64_48',
    'please_change_me',
}

path = Path('.env')
lines = path.read_text(encoding='utf-8').splitlines() if path.exists() else []
out, replaced = [], False
for raw in lines:
    if raw.strip().startswith('#') or '=' not in raw:
        out.append(raw)
        continue
    current_key, _, current_val = raw.partition('=')
    if current_key.strip() == KEY:
        replaced = True
        if current_val.strip() in PLACEHOLDERS:
            out.append(f'{KEY}={secrets.token_urlsafe(48)}')
            print(f'   generated fresh {KEY}')
        else:
            out.append(raw)
            print(f'   {KEY} already set, kept as-is')
    else:
        out.append(raw)
if not replaced:
    if out and out[-1] != '':
        out.append('')
    out.append(f'{KEY}={secrets.token_urlsafe(48)}')
    print(f'   {KEY} not present, appended fresh value')
path.write_text('\\n'.join(out) + '\\n', encoding='utf-8')
PY" || {
  echo "==> Failed to seed ARKLOOP_INTERNAL_SERVICE_TOKEN." >&2
  exit 1
}

echo "==> Cleaning up ALL arkloop containers and networks..."
if [ "$RESET_DATA" = "1" ]; then
  echo "    --reset-data: also dropping volumes (DESTRUCTIVE)"
  ssh_cmd "cd ${REMOTE_DIR} && docker compose down -v --remove-orphans 2>/dev/null || true"
else
  ssh_cmd "cd ${REMOTE_DIR} && docker compose down --remove-orphans 2>/dev/null || true"
fi
ssh_cmd "docker ps -aq --filter 'name=arkloop' | xargs -r docker stop 2>/dev/null || true"
ssh_cmd "docker ps -aq --filter 'name=arkloop' | xargs -r docker rm -f 2>/dev/null || true"
ssh_cmd "docker network ls --filter 'name=arkloop' -q | xargs -r docker network rm 2>/dev/null || true"
if [ "$RESET_DATA" = "1" ]; then
  ssh_cmd "docker volume ls -q --filter 'name=arkloop' | xargs -r docker volume rm 2>/dev/null || true"
fi

echo "==> Running full install..."
ssh_cmd "cd ${REMOTE_DIR} && ./setup.sh install \
  --profile standard \
  --mode self-hosted \
  --memory none \
  --sandbox none \
  --console lite \
  --browser off \
  --web-tools ${SETUP_WEB_TOOLS} \
  --gateway on \
  --gateway-port ${GATEWAY_PORT} \
  --non-interactive" || {
  echo "==> Install failed. Checking logs..."
  ssh_cmd "cd ${REMOTE_DIR} && docker compose ps"
  ssh_cmd "cd ${REMOTE_DIR} && docker compose logs --tail 100 migrate api worker web postgres redis"
  exit 1
}

echo "==> Pinning canonical platform model IDs (Qwen / Doubao)..."
# Background: setup.sh uses set_if_empty for ARKLOOP_*_MODELS, which means
# previously-deployed servers carrying stale placeholder IDs (e.g.
# Qwen3.6-27B, Doubao-Seed-2.0-Mini) will keep them across re-installs and
# upstream APIs reject those names ("model does not exist"). This step:
#   1. force-overwrites the model lines in remote .env to canonical IDs;
#   2. prunes any platform-level llm_routes whose model is not in the
#      canonical set (so the picker stops surfacing the bad ones);
#   3. force-recreates api so syncPlatformModelPresets re-upserts the
#      canonical routes from the corrected .env.
# DeepSeek defaults already match its public docs (deepseek-v4-flash /
# deepseek-v4-pro), so they are intentionally not touched here.
ssh_cmd "cd ${REMOTE_DIR} && ${REMOTE_PYTHON} - <<'PY'
from pathlib import Path

CANONICAL = {
    'ARKLOOP_QWEN_MODELS':   'qwen3.5-plus,qwen3-max-2026-01-23',
    'ARKLOOP_DOUBAO_MODELS': 'doubao-seed-2-0-lite-260428,doubao-seed-2-0-mini-260428',
}

path = Path('.env')
lines = path.read_text(encoding='utf-8').splitlines() if path.exists() else []
out, seen = [], set()
for raw in lines:
    if raw.strip().startswith('#') or '=' not in raw:
        out.append(raw)
        continue
    key, _ = raw.split('=', 1)
    key = key.strip()
    if key in CANONICAL:
        out.append(f'{key}={CANONICAL[key]}')
        seen.add(key)
    else:
        out.append(raw)
for key, value in CANONICAL.items():
    if key in seen:
        continue
    if out and out[-1] != '':
        out.append('')
    out.append(f'{key}={value}')
path.write_text('\\n'.join(out) + '\\n', encoding='utf-8')
print('canonical model IDs written to .env')
PY" || {
  echo "==> .env pin step failed."
  exit 1
}

ssh_cmd "cd ${REMOTE_DIR} && docker compose exec -T postgres psql -U arkloop -d arkloop <<'SQL'
DELETE FROM llm_routes
WHERE id IN (
  SELECT r.id
  FROM llm_routes r
  JOIN llm_credentials c ON c.id = r.credential_id
  WHERE c.provider IN ('qwen','doubao')
    AND c.owner_kind = 'platform'
    AND r.account_id IS NULL
    AND r.project_id IS NULL
    AND r.model NOT IN (
      'qwen3.5-plus',
      'qwen3-max-2026-01-23',
      'doubao-seed-2-0-lite-260428',
      'doubao-seed-2-0-mini-260428'
    )
);
SQL" || {
  echo "==> Pruning stale platform model routes failed. Checking logs..."
  ssh_cmd "cd ${REMOTE_DIR} && docker compose logs --tail 50 postgres"
  exit 1
}

ssh_cmd "cd ${REMOTE_DIR} && docker compose up -d --force-recreate api" || {
  echo "==> api force-recreate after model pin failed."
  ssh_cmd "cd ${REMOTE_DIR} && docker compose logs --tail 100 api"
  exit 1
}

if [ "$WEB_TOOLS" = "searxng" ] || [ "$WEB_TOOLS" = "self-hosted" ]; then
  echo "==> Configuring web search/fetch providers..."
  if [ "$WEB_TOOLS" = "searxng" ]; then
    # --force-recreate: re-run config/searxng/patch-settings.sh against the
    # mounted settings.yml every deploy so engine flips (e.g. enabling
    # baidu/360search/sogou for behind-GFW deployments) actually apply.
    # Without this, an existing searxng container keeps running with the old
    # in-memory settings and the patch-settings.sh changes never take effect.
    ssh_cmd "cd ${REMOTE_DIR} && docker compose --profile searxng up -d --force-recreate searxng"
  fi
  ssh_cmd "cd ${REMOTE_DIR} && docker compose exec -T postgres psql -U arkloop -d arkloop <<'SQL'
INSERT INTO tool_provider_configs (owner_kind, owner_user_id, group_name, provider_name, is_active, base_url, config_json)
VALUES ('platform', NULL, 'web_search', 'web_search.searxng', TRUE, 'http://searxng:8080', '{}'::jsonb)
ON CONFLICT (provider_name) WHERE owner_kind = 'platform'
DO UPDATE SET
  group_name = EXCLUDED.group_name,
  is_active = TRUE,
  base_url = EXCLUDED.base_url,
  updated_at = now();

UPDATE tool_provider_configs
SET is_active = FALSE, updated_at = now()
WHERE owner_kind = 'platform'
  AND group_name = 'web_search'
  AND provider_name <> 'web_search.searxng';

INSERT INTO tool_provider_configs (owner_kind, owner_user_id, group_name, provider_name, is_active, base_url, config_json)
VALUES ('platform', NULL, 'web_fetch', 'web_fetch.basic', TRUE, NULL, '{}'::jsonb)
ON CONFLICT (provider_name) WHERE owner_kind = 'platform'
DO UPDATE SET
  group_name = EXCLUDED.group_name,
  is_active = TRUE,
  base_url = EXCLUDED.base_url,
  updated_at = now();

UPDATE tool_provider_configs
SET is_active = FALSE, updated_at = now()
WHERE owner_kind = 'platform'
  AND group_name = 'web_fetch'
  AND provider_name <> 'web_fetch.basic';
SQL
docker compose restart worker api >/dev/null" || {
    echo "==> Web provider configuration failed. Checking logs..."
    ssh_cmd "cd ${REMOTE_DIR} && docker compose ps"
    ssh_cmd "cd ${REMOTE_DIR} && docker compose logs --tail 100 searxng worker api postgres"
    exit 1
  }
fi

echo "==> Starting web app on port ${WEB_PORT}..."
ssh_cmd "cd ${REMOTE_DIR} && ${REMOTE_PYTHON} - <<'PY'
from pathlib import Path

path = Path('.env')
key = 'ARKLOOP_WEB_PORT'
value = '${WEB_PORT}'
lines = path.read_text(encoding='utf-8').splitlines() if path.exists() else []
out = []
seen = False
for raw in lines:
    if raw.strip().startswith('#') or '=' not in raw:
        out.append(raw)
        continue
    current_key, _ = raw.split('=', 1)
    if current_key.strip() == key:
        out.append(f'{key}={value}')
        seen = True
    else:
        out.append(raw)
if not seen:
    if out and out[-1] != '':
        out.append('')
    out.append(f'{key}={value}')
path.write_text('\\n'.join(out) + '\\n', encoding='utf-8')
PY
docker compose up -d --build web" || {
  echo "==> Web startup failed. Checking logs..."
  ssh_cmd "cd ${REMOTE_DIR} && docker compose ps"
  ssh_cmd "cd ${REMOTE_DIR} && docker compose logs --tail 100 web gateway api"
  exit 1
}

echo "==> Registering exam-web OAuth client (idempotent)..."
# Background: the worker → exam tool chain needs an oauth_clients row with
# client_id='exam-web' so that /internal/oauth/issue validates "scopes ⊆
# client.allowed_scopes". The client_secret itself is not used by the agent
# path (worker authenticates with ARKLOOP_INTERNAL_SERVICE_TOKEN), but the
# CLI still generates one and stores its bcrypt hash for the rare browser-SSO
# operations backdoor. -idempotent makes re-runs a no-op when the row exists.
ssh_cmd "cd ${REMOTE_DIR} && docker compose exec -T api /usr/local/bin/oauth-seed \
  -client-id exam-web \
  -name 'Exam Backend' \
  -redirect 'http://exam-backend:8000/api/auth/oidc/callback' \
  -scopes 'openid,profile,email,exam:read,exam:write,exam:admin' \
  -idempotent" || {
  echo "==> oauth-seed failed (non-fatal: the agent path can still work if the row was created earlier)."
  ssh_cmd "cd ${REMOTE_DIR} && docker compose logs --tail 50 api postgres"
}

echo "==> Done!"
REMOTE_HOST="${REMOTE#*@}"
echo "    Gateway/console: http://${REMOTE_HOST}:${GATEWAY_PORT}"
echo "    Web app:         http://${REMOTE_HOST}:${WEB_PORT}"
