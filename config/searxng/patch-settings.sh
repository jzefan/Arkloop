#!/bin/sh
# Patch SearXNG settings to enable JSON API format.
# Runs as entrypoint wrapper: patches the settings file, then exec's the real entrypoint.

SETTINGS="${SEARXNG_SETTINGS_PATH:-/etc/searxng/settings.yml}"
DEFAULT_SETTINGS="/usr/local/searxng/searx/settings.yml"

if [ ! -f "$SETTINGS" ] && [ -f "$DEFAULT_SETTINGS" ]; then
  mkdir -p "$(dirname "$SETTINGS")"
  cp "$DEFAULT_SETTINGS" "$SETTINGS"
fi

if [ -f "$SETTINGS" ] && grep -q 'secret_key:.*ultrasecretkey' "$SETTINGS"; then
  secret="${SEARXNG_SECRET_KEY:-}"
  if [ -z "$secret" ] && [ -f /proc/sys/kernel/random/uuid ]; then
    secret="$(cat /proc/sys/kernel/random/uuid)"
  fi
  if [ -z "$secret" ]; then
    secret="arkloop-searxng-local-secret"
  fi
  sed -i "s/secret_key:.*/secret_key: \"$secret\"/" "$SETTINGS"
fi

if [ -f "$SETTINGS" ] && ! grep -Eq '^[[:space:]]*-[[:space:]]*json' "$SETTINGS"; then
  if grep -q '^  formats:' "$SETTINGS"; then
    sed -i '/^  formats:/a\    - json' "$SETTINGS"
  else
    cat >> "$SETTINGS" <<'EOF'

search:
  formats:
    - html
    - json
EOF
  fi
fi

# Enable China-accessible engines so deployments behind GFW (where the default
# Google / Bing / DuckDuckGo engines all return empty) still get usable
# results. Idempotent: only flips `disabled` for the named engines, leaves
# every other engine untouched.
#
# Behaviour:
# - ENABLE_LIST is always force-enabled (baidu / 360search / sogou): harmless
#   on overseas deployments (those engines just contribute zero results).
# - DISABLE_LIST is only force-disabled when SEARXNG_DISABLE_GFW_BLOCKED=1.
#   Without this opt-in, overseas-friendly engines (google / duckduckgo / ...)
#   stay enabled. Behind-GFW deployments should set the env so SearXNG does
#   not waste 15-20s waiting on engines that always time out — otherwise the
#   worker's 10s tool timeout kills every search ("web_search timed out").
#
# Important: we MUST use the SearXNG virtualenv's python, not the system
# /usr/sbin/python3 — the system python in the upstream image lacks PyYAML,
# while the venv has it. If neither path is found, fall back to a sed-based
# best-effort patch.
if [ -f "$SETTINGS" ]; then
  VENV_PY="/usr/local/searxng/.venv/bin/python3"
  if [ ! -x "$VENV_PY" ]; then
    VENV_PY="$(command -v python3 2>/dev/null || true)"
  fi
  if [ -n "$VENV_PY" ] && "$VENV_PY" -c 'import yaml' >/dev/null 2>&1; then
    SEARXNG_DISABLE_GFW_BLOCKED="${SEARXNG_DISABLE_GFW_BLOCKED:-}" \
    "$VENV_PY" - "$SETTINGS" <<'PY' || true
import os
import sys
import yaml

path = sys.argv[1]
try:
    with open(path, "r", encoding="utf-8") as fh:
        cfg = yaml.safe_load(fh) or {}
except Exception:
    sys.exit(0)

ENABLE = {
    # CN-friendly general search engines.
    "baidu",
    "360search",
    "sogou",
    "bing",       # international Bing — often partially reachable from CN.
    "quark",      # 夸克搜索 (Alibaba) — CN-friendly.
    # Academic / scholarly indexes that ride OpenAI-compatible category=science.
    # Harmless to leave on for general searches: they only fire when the
    # caller asks for the science category.
    "crossref",
}
disable_gfw = os.environ.get("SEARXNG_DISABLE_GFW_BLOCKED") in {"1", "true", "yes"}
DISABLE = {
    "google",
    "google scholar",
    "duckduckgo",
    "brave",
    "startpage",
    "wikipedia",
    "karmasearch",
} if disable_gfw else set()

engines = cfg.get("engines")
if not isinstance(engines, list):
    sys.exit(0)

changed = False
for entry in engines:
    if not isinstance(entry, dict):
        continue
    name = entry.get("name") or entry.get("engine")
    if name in ENABLE and entry.get("disabled") is not False:
        entry["disabled"] = False
        changed = True
    if name in DISABLE and entry.get("disabled") is not True:
        entry["disabled"] = True
        changed = True

if changed:
    with open(path, "w", encoding="utf-8") as fh:
        yaml.safe_dump(cfg, fh, allow_unicode=True, sort_keys=False)
    print(
        f"patch-settings.sh: enabled={sorted(ENABLE)} disabled={sorted(DISABLE)}",
        file=sys.stderr,
    )
PY
  else
    echo "patch-settings.sh: no python+yaml available, skipping engine patch" >&2
  fi
fi

exec /usr/local/searxng/entrypoint.sh
