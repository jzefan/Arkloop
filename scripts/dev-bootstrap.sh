#!/usr/bin/env bash
#
# dev-bootstrap.sh — one-shot dev environment setup for ArkLoop + exam OIDC.
#
# Idempotent: re-running keeps existing secrets, only fills placeholders.
# After this finishes, the only thing you still need to do manually is run
# `uvicorn app.main:app --reload --port 8000` from the exam backend directory.
#
# Usage:
#   scripts/dev-bootstrap.sh                       # ArkLoop only
#   scripts/dev-bootstrap.sh --exam-dir ~/work/proj/exam   # + exam migrate & .env
#
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EXAM_DIR=""
SKIP_EXAM_MIGRATE=0
SKIP_EXAM_ENV=0
SKIP_COMPOSE=0
SKIP_OAUTH_SEED=0

usage() {
  cat <<'EOF'
Usage:
  scripts/dev-bootstrap.sh [options]

Options:
  --exam-dir <path>        Path to the exam project root (e.g. ~/work/proj/exam).
                           Enables exam alembic migrate + .env injection.
  --skip-compose           Don't start docker compose (assume it's running).
  --skip-oauth-seed        Don't register exam-web OAuth client (assume it exists).
  --skip-exam-migrate      Don't run exam alembic upgrade.
  --skip-exam-env          Don't touch exam's .env.
  -h, --help               Show this help.

What it does (in order, all idempotent):
  1. Copy .env.example -> .env if missing
  2. Generate fresh values for ARKLOOP_INTERNAL_SERVICE_TOKEN, JWT_SECRET,
     ENCRYPTION_KEY, postgres/redis passwords — only when the field is a
     known placeholder. Real values are preserved.
  3. Force dev-only values for ARKLOOP_OIDC_ISSUER, EXAM_BASE_URL,
     ARKLOOP_API_INTERNAL_URL (these must point at a specific dev topology).
  4. docker compose up -d --build, wait for /healthz.
  5. Register exam-web OAuth client (via oauth-seed -idempotent).
  6. (if --exam-dir) alembic upgrade head, write EXAM_OIDC_ISSUER to
     exam's .env.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --exam-dir) EXAM_DIR="$2"; shift 2 ;;
    --skip-compose) SKIP_COMPOSE=1; shift ;;
    --skip-oauth-seed) SKIP_OAUTH_SEED=1; shift ;;
    --skip-exam-migrate) SKIP_EXAM_MIGRATE=1; shift ;;
    --skip-exam-env) SKIP_EXAM_ENV=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown: $1" >&2; usage >&2; exit 2 ;;
  esac
done

# ─── Prereqs ───────────────────────────────────────────────────────

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { echo "✗ missing command: $1" >&2; exit 1; }
}
require_cmd docker
require_cmd python3
require_cmd curl

# docker compose v2 is the supported variant (compose v1 / docker-compose is EOL).
docker compose version >/dev/null 2>&1 || {
  echo "✗ docker compose v2 plugin not found" >&2
  exit 1
}

cd "$ROOT_DIR"

# ─── Step 1+2+3: .env preparation ──────────────────────────────────

if [ ! -f .env ]; then
  cp .env.example .env
  echo "→ created .env from .env.example"
fi

echo "==> Reconciling .env (generate missing secrets, force dev URLs)..."
python3 - <<'PY'
import secrets
from pathlib import Path

# Values we treat as "needs replacement". Real values are kept as-is.
PLACEHOLDERS = {
    "",
    "please_change_me",
    "please_change_me_please_change_me_please_change_me",
    "please_generate_with_openssl_rand_base64_48",
    "please_generate_with_openssl_rand_hex_32",
    "please_generate_with_openssl_rand_hex_32",
    "dev",  # we generate stronger; keep .env stable across re-runs once filled
}

# Keys whose value is auto-generated when placeholder is detected.
GENERATORS = {
    "ARKLOOP_INTERNAL_SERVICE_TOKEN": lambda: secrets.token_urlsafe(48),
    "ARKLOOP_AUTH_JWT_SECRET":        lambda: secrets.token_urlsafe(48),
    "ARKLOOP_ENCRYPTION_KEY":         lambda: secrets.token_hex(32),
    "ARKLOOP_POSTGRES_PASSWORD":      lambda: "dev_" + secrets.token_hex(8),
    "ARKLOOP_REDIS_PASSWORD":         lambda: "dev_" + secrets.token_hex(8),
}

# Keys whose value is forced — these must point at a specific dev topology
# regardless of what the user (or .env.example) had previously.
FORCED = {
    "ARKLOOP_OIDC_ISSUER":      "http://localhost:19000",
    "EXAM_BASE_URL":            "http://host.docker.internal:8000",
    "ARKLOOP_API_INTERNAL_URL": "http://api:19001",
}

path = Path(".env")
text = path.read_text(encoding="utf-8")
lines = text.splitlines()

# We rebuild while remembering generated values, so that when the same
# key appears in DATABASE_URL etc. we could keep them aligned. For now
# we only touch the password fields themselves; in docker compose every
# service constructs its DSN from POSTGRES_PASSWORD anyway.
out, seen = [], set()
changed = []
for raw in lines:
    stripped = raw.strip()
    if not stripped or stripped.startswith("#") or "=" not in stripped:
        out.append(raw)
        continue
    key, _, val = raw.partition("=")
    key = key.strip()
    val_stripped = val.strip()

    if key in FORCED:
        if val_stripped != FORCED[key]:
            out.append(f"{key}={FORCED[key]}")
            changed.append((key, "forced", FORCED[key]))
        else:
            out.append(raw)
        seen.add(key)
        continue

    if key in GENERATORS:
        if val_stripped in PLACEHOLDERS:
            new_val = GENERATORS[key]()
            out.append(f"{key}={new_val}")
            changed.append((key, "generated", new_val[:6] + "…"))
        else:
            out.append(raw)
        seen.add(key)
        continue

    out.append(raw)

# Append anything missing entirely.
trailer = []
for key, val in FORCED.items():
    if key not in seen:
        trailer.append(f"{key}={val}")
        changed.append((key, "appended", val))
for key, gen in GENERATORS.items():
    if key not in seen:
        v = gen()
        trailer.append(f"{key}={v}")
        changed.append((key, "appended-fresh", v[:6] + "…"))

if trailer:
    if out and out[-1] != "":
        out.append("")
    out.extend(trailer)

path.write_text("\n".join(out) + "\n", encoding="utf-8")

if changed:
    for key, kind, preview in changed:
        print(f"   {kind:>15s}  {key}={preview}")
else:
    print("   .env already complete; no changes")
PY

# ─── Step 4: docker compose ─────────────────────────────────────────

if [ "$SKIP_COMPOSE" -ne 1 ]; then
  echo "==> Starting ArkLoop docker compose (this may take a few minutes on first build)..."
  docker compose up -d --build

  echo "==> Waiting for gateway /healthz (up to 180s)..."
  HEALTHY=0
  for i in $(seq 1 90); do
    if curl -sf -m 2 http://localhost:19000/healthz >/dev/null 2>&1; then
      HEALTHY=1
      echo "   gateway healthy after ${i}x2s"
      break
    fi
    sleep 2
  done
  if [ "$HEALTHY" -ne 1 ]; then
    echo "✗ gateway never became healthy. Recent logs:" >&2
    docker compose logs --tail 60 api gateway migrate >&2
    exit 1
  fi

  # Sanity-check that OIDC discovery is reachable (this would silently 404
  # if the gateway routing patch from this PR wasn't applied — better fail
  # loudly here than during the first agent run).
  if ! curl -sf -m 5 http://localhost:19000/.well-known/jwks.json >/dev/null 2>&1; then
    echo "✗ /.well-known/jwks.json not reachable via gateway." >&2
    echo "  Gateway routing patch missing? Rebuild gateway and try again:" >&2
    echo "    docker compose up -d --build --force-recreate gateway" >&2
    exit 1
  fi
  echo "   OIDC discovery endpoint OK"
fi

# ─── Step 5: register exam-web OAuth client ────────────────────────

if [ "$SKIP_OAUTH_SEED" -ne 1 ]; then
  echo "==> Registering exam-web OAuth client (idempotent)..."
  docker compose exec -T api oauth-seed \
    -client-id exam-web \
    -name "Exam Backend (dev)" \
    -redirect "http://localhost:8000/api/auth/oidc/callback" \
    -scopes "openid,profile,email,exam:read,exam:write,exam:admin" \
    -idempotent || {
      echo "✗ oauth-seed failed. Check 'docker compose logs api' for details." >&2
      exit 1
    }
fi

# ─── Step 6: exam side ─────────────────────────────────────────────

if [ -n "$EXAM_DIR" ]; then
  EXAM_DIR_ABS="$(cd "$EXAM_DIR" 2>/dev/null && pwd || true)"
  if [ -z "$EXAM_DIR_ABS" ] || [ ! -d "$EXAM_DIR_ABS/backend" ]; then
    echo "✗ --exam-dir $EXAM_DIR does not contain a backend/ subdirectory" >&2
    exit 1
  fi

  if [ "$SKIP_EXAM_ENV" -ne 1 ]; then
    echo "==> Injecting EXAM_OIDC_ISSUER into $EXAM_DIR_ABS/backend/.env..."
    EXAM_ENV_FILE="$EXAM_DIR_ABS/backend/.env" python3 - <<'PY'
import os
from pathlib import Path

path = Path(os.environ["EXAM_ENV_FILE"])
lines = path.read_text(encoding="utf-8").splitlines() if path.exists() else []
target_key = "EXAM_OIDC_ISSUER"
target_val = "http://localhost:19000"
out, seen = [], False
for raw in lines:
    if raw.startswith(f"{target_key}="):
        out.append(f"{target_key}={target_val}")
        seen = True
    else:
        out.append(raw)
if not seen:
    if out and out[-1] != "":
        out.append("")
    out.append(f"{target_key}={target_val}")
path.write_text("\n".join(out) + "\n", encoding="utf-8")
print(f"   wrote {target_key}={target_val}")
PY
  fi

  if [ "$SKIP_EXAM_MIGRATE" -ne 1 ]; then
    echo "==> Running exam alembic migration..."
    if [ ! -x "$(command -v alembic)" ] && [ ! -f "$EXAM_DIR_ABS/backend/.venv/bin/alembic" ]; then
      echo "⚠ alembic not found on PATH and no .venv/bin/alembic in exam backend." >&2
      echo "  Activate exam's virtualenv first, or skip with --skip-exam-migrate." >&2
      exit 1
    fi
    # exam uses a src-layout (backend/src/app/...) but its pyproject.toml
    # lacks the setuptools config that would put `app` on sys.path. The
    # alembic env.py does `from app.config import settings`, which 404s
    # unless we point PYTHONPATH at src/. Setting it unconditionally is a
    # no-op for projects with a different layout, so it's a safe default.
    if [ -d "$EXAM_DIR_ABS/backend/src/app" ]; then
      EXAM_PYTHONPATH="$EXAM_DIR_ABS/backend/src"
    else
      EXAM_PYTHONPATH=""
    fi
    (cd "$EXAM_DIR_ABS/backend" && \
     PYTHONPATH="$EXAM_PYTHONPATH${PYTHONPATH:+:$PYTHONPATH}" \
     bash -c 'if [ -f .venv/bin/alembic ]; then ./.venv/bin/alembic upgrade head; else alembic upgrade head; fi') || {
      echo "✗ exam alembic upgrade failed" >&2
      exit 1
    }
  fi
fi

# ─── Step 7: summary ───────────────────────────────────────────────

cat <<EOF

────────────────────────────────────────────────────────────────────
✅  ArkLoop dev environment ready.

Services:
   Gateway / API : http://localhost:19000
   Web           : http://localhost:19080
   OIDC issuer   : http://localhost:19000  (token iss claim & JWKS base)

Next:
   1. Start exam backend (in another terminal):
EOF

if [ -n "$EXAM_DIR" ]; then
  cat <<EOF
        cd $EXAM_DIR/backend
        uvicorn app.main:app --reload --port 8000
EOF
else
  cat <<'EOF'
        cd <exam-repo>/backend
        EXAM_OIDC_ISSUER=http://localhost:19000 \
          uvicorn app.main:app --reload --port 8000
EOF
fi

cat <<'EOF'

   2. Open http://localhost:19080 → register a teacher account.
   3. Select "题库助手" persona → upload a catalog image.

Smoke tests (no browser):
   curl -s http://localhost:19000/.well-known/openid-configuration | jq '.issuer'
   curl -s http://localhost:19000/.well-known/jwks.json             | jq '.keys[0].kty'
   curl -s -o /dev/null -w "%{http_code}\n" http://localhost:19000/internal/oauth/issue
        # expected: "RSA" and 404 respectively

Tear down:
   docker compose down            # keep data
   docker compose down -v         # also drop volumes
────────────────────────────────────────────────────────────────────
EOF
