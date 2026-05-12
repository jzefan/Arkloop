#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="${ROOT_DIR}/release-files"
PACKAGE_NAME=""
VERSION=""
GATEWAY_PORT="3037"
IMAGE_REPOSITORY="ghcr.io/jzefan/arkloop"
IMAGE_VERSION="latest"
INCLUDE_SOURCE="0"
FORCE="0"

usage() {
  cat <<'EOF'
Usage:
  scripts/build-deploy-package.sh [options]

Options:
  --output-dir <dir>          Output directory (default: release-files)
  --name <name>               Package base name (default: arkloop-deploy-<git-sha>)
  --version <tag>             Image/app version written to package env (default: latest)
  --gateway-port <port>       Default gateway port for install.sh (default: 3037)
  --image-repository <repo>   Image prefix without service suffix (default: ghcr.io/jzefan/arkloop)
  --include-source            Include full source tree for local-build installs
  --force                     Overwrite existing package
  -h, --help                  Show this help

The default package is a production deployment bundle. It does not require git
on the server; it expects Docker Compose and pulls images from GHCR with --prod.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --output-dir) OUTPUT_DIR="$2"; shift 2 ;;
    --name) PACKAGE_NAME="$2"; shift 2 ;;
    --version) VERSION="$2"; IMAGE_VERSION="$2"; shift 2 ;;
    --gateway-port) GATEWAY_PORT="$2"; shift 2 ;;
    --image-repository) IMAGE_REPOSITORY="$2"; shift 2 ;;
    --include-source) INCLUDE_SOURCE="1"; shift ;;
    --force) FORCE="1"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown argument: $1" >&2; usage >&2; exit 2 ;;
  esac
done

if ! [[ "$GATEWAY_PORT" =~ ^[0-9]+$ ]] || [ "$GATEWAY_PORT" -lt 1 ] || [ "$GATEWAY_PORT" -gt 65535 ]; then
  echo "Invalid --gateway-port: ${GATEWAY_PORT}" >&2
  exit 2
fi

if [ -z "$VERSION" ]; then
  if git -C "$ROOT_DIR" rev-parse --short=12 HEAD >/dev/null 2>&1; then
    VERSION="$(git -C "$ROOT_DIR" rev-parse --short=12 HEAD)"
  else
    VERSION="$(date +%Y%m%d%H%M%S)"
  fi
fi

if [ "$IMAGE_VERSION" = "latest" ] && [ "$VERSION" != "latest" ]; then
  IMAGE_VERSION="latest"
fi

if [ -z "$PACKAGE_NAME" ]; then
  PACKAGE_NAME="arkloop-deploy-${VERSION}"
fi

mkdir -p "$OUTPUT_DIR"
STAGING_DIR="$(mktemp -d "${TMPDIR:-/tmp}/arkloop-deploy-package.XXXXXX")"
PACKAGE_ROOT="${STAGING_DIR}/${PACKAGE_NAME}"
ARCHIVE_PATH="${OUTPUT_DIR}/${PACKAGE_NAME}.tar.gz"

cleanup() {
  rm -rf "$STAGING_DIR"
}
trap cleanup EXIT

if [ -e "$ARCHIVE_PATH" ] && [ "$FORCE" != "1" ]; then
  echo "Package already exists: ${ARCHIVE_PATH}" >&2
  echo "Use --force to overwrite." >&2
  exit 1
fi

mkdir -p "$PACKAGE_ROOT"

copy_minimal_bundle() {
  tar -C "$ROOT_DIR" \
    --exclude='.DS_Store' \
    --exclude='config/openviking/ov.conf' \
    -cf - \
    .env.example \
    compose.yaml \
    compose.prod.yaml \
    setup.sh \
    install/module_registry.py \
    install/modules.yaml \
    config | tar -C "$PACKAGE_ROOT" -xf -
}

copy_source_bundle() {
  tar -C "$ROOT_DIR" \
    --exclude='.git' \
    --exclude='.env' \
    --exclude='.env.*' \
    --exclude='node_modules' \
    --exclude='**/node_modules' \
    --exclude='release-files' \
    --exclude='.cache' \
    --exclude='**/dist' \
    --exclude='**/.vite' \
    --exclude='src/apps/desktop/release' \
    --exclude='src/apps/desktop/dist' \
    --exclude='src/apps/developers/dist' \
    --exclude='src/apps/developers/.astro' \
    --exclude='src/apps/developers/.next' \
    --exclude='.DS_Store' \
    -cf - . | tar -C "$PACKAGE_ROOT" -xf -
}

if [ "$INCLUDE_SOURCE" = "1" ]; then
  copy_source_bundle
else
  copy_minimal_bundle
fi

cat > "${PACKAGE_ROOT}/install.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
cd "\$SCRIPT_DIR"

export ARKLOOP_IMAGE_REPOSITORY="\${ARKLOOP_IMAGE_REPOSITORY:-${IMAGE_REPOSITORY}}"
export ARKLOOP_VERSION="\${ARKLOOP_VERSION:-${IMAGE_VERSION}}"

if [ ! -f .env ]; then
  cp .env.example .env
fi

python3 - "\$ARKLOOP_IMAGE_REPOSITORY" "\$ARKLOOP_VERSION" <<'PY'
import sys
from pathlib import Path

path = Path(".env")
values = {
    "ARKLOOP_IMAGE_REPOSITORY": sys.argv[1],
    "ARKLOOP_VERSION": sys.argv[2],
}
lines = path.read_text(encoding="utf-8").splitlines() if path.exists() else []
out = []
seen = set()
for raw in lines:
    if raw.strip().startswith("#") or "=" not in raw:
        out.append(raw)
        continue
    key, _ = raw.split("=", 1)
    key = key.strip()
    if key in values:
        out.append(f"{key}={values[key]}")
        seen.add(key)
    else:
        out.append(raw)
for key, value in values.items():
    if key not in seen:
        if out and out[-1] != "":
            out.append("")
        out.append(f"{key}={value}")
path.write_text("\\n".join(out) + "\\n", encoding="utf-8")
PY

exec ./setup.sh install \\
  --profile "\${ARKLOOP_INSTALL_PROFILE:-standard}" \\
  --mode "\${ARKLOOP_INSTALL_MODE:-self-hosted}" \\
  --memory "\${ARKLOOP_INSTALL_MEMORY:-none}" \\
  --sandbox "\${ARKLOOP_INSTALL_SANDBOX:-none}" \\
  --console "\${ARKLOOP_INSTALL_CONSOLE:-lite}" \\
  --browser "\${ARKLOOP_INSTALL_BROWSER:-off}" \\
  --web-tools "\${ARKLOOP_INSTALL_WEB_TOOLS:-builtin}" \\
  --gateway "\${ARKLOOP_INSTALL_GATEWAY:-on}" \\
  --gateway-port "\${ARKLOOP_GATEWAY_PORT:-${GATEWAY_PORT}}" \\
  --prod \\
  --non-interactive \\
  "\$@"
EOF

cat > "${PACKAGE_ROOT}/README.deploy.md" <<EOF
# ArkLoop Deployment Package

This package runs ArkLoop without requiring git on the server.

## Install

\`\`\`bash
tar -xzf ${PACKAGE_NAME}.tar.gz
cd ${PACKAGE_NAME}
./install.sh
\`\`\`

Default entry URL:

\`\`\`text
http://<server-ip>:${GATEWAY_PORT}
\`\`\`

## Configuration

The package defaults to:

- Image repository: \`${IMAGE_REPOSITORY}\`
- Image version: \`${IMAGE_VERSION}\`
- Gateway port: \`${GATEWAY_PORT}\`

Override before running \`install.sh\`:

\`\`\`bash
ARKLOOP_GATEWAY_PORT=3037 \\
ARKLOOP_IMAGE_REPOSITORY=${IMAGE_REPOSITORY} \\
ARKLOOP_VERSION=${IMAGE_VERSION} \\
./install.sh
\`\`\`

If GHCR packages are private, run \`docker login ghcr.io\` on the server first.
EOF

chmod +x "${PACKAGE_ROOT}/setup.sh" "${PACKAGE_ROOT}/install.sh"

tar -C "$STAGING_DIR" -czf "$ARCHIVE_PATH" "$PACKAGE_NAME"

printf 'Created deployment package: %s\n' "$ARCHIVE_PATH"
printf 'Server install:\n'
printf '  tar -xzf %s\n' "$(basename "$ARCHIVE_PATH")"
printf '  cd %s\n' "$PACKAGE_NAME"
printf '  ./install.sh\n'
