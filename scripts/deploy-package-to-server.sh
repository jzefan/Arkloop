#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BUILD_SCRIPT="${ROOT_DIR}/scripts/build-deploy-package.sh"

REMOTE=""
REMOTE_DIR="~/arkloop"
GATEWAY_PORT="3037"
IMAGE_REPOSITORY="ghcr.io/jzefan/arkloop"
IMAGE_VERSION="latest"
OUTPUT_DIR="${ROOT_DIR}/release-files"
PACKAGE_NAME=""
INCLUDE_SOURCE="0"
SSH_OPTS=()
RUN_INSTALL="1"

usage() {
  cat <<'EOF'
Usage:
  scripts/deploy-package-to-server.sh <user@server-ip> [options]

Options:
  --remote-dir <dir>          Remote deployment directory (default: ~/arkloop)
  --gateway-port <port>       Gateway port exposed on the server (default: 3037)
  --image-repository <repo>   Image prefix without service suffix (default: ghcr.io/jzefan/arkloop)
  --image-version <tag>       Image tag used by --prod install (default: latest)
  --output-dir <dir>          Local package output directory (default: release-files)
  --package-name <name>       Override generated package name
  --include-source            Include source tree in package
  --no-install                Upload and unpack only; do not run install.sh
  --ssh-option <option>       Extra ssh/scp option, repeatable. Example: --ssh-option "-p 2222"
  -h, --help                  Show this help

Examples:
  scripts/deploy-package-to-server.sh root@1.2.3.4
  scripts/deploy-package-to-server.sh ubuntu@1.2.3.4 --remote-dir /opt/arkloop
  scripts/deploy-package-to-server.sh ubuntu@1.2.3.4 --ssh-option "-p 2222"

The remote server does not need git. It needs Docker, Docker Compose, tar, and bash.
If GHCR packages are private, run docker login ghcr.io on the server first.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --remote-dir) REMOTE_DIR="$2"; shift 2 ;;
    --gateway-port) GATEWAY_PORT="$2"; shift 2 ;;
    --image-repository) IMAGE_REPOSITORY="$2"; shift 2 ;;
    --image-version) IMAGE_VERSION="$2"; shift 2 ;;
    --output-dir) OUTPUT_DIR="$2"; shift 2 ;;
    --package-name) PACKAGE_NAME="$2"; shift 2 ;;
    --include-source) INCLUDE_SOURCE="1"; shift ;;
    --no-install) RUN_INSTALL="0"; shift ;;
    --ssh-option) SSH_OPTS+=("$2"); shift 2 ;;
    -h|--help) usage; exit 0 ;;
    -*)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
    *)
      if [ -n "$REMOTE" ]; then
        echo "Unexpected argument: $1" >&2
        usage >&2
        exit 2
      fi
      REMOTE="$1"
      shift
      ;;
  esac
done

if [ -z "$REMOTE" ]; then
  usage >&2
  exit 2
fi

if ! [[ "$GATEWAY_PORT" =~ ^[0-9]+$ ]] || [ "$GATEWAY_PORT" -lt 1 ] || [ "$GATEWAY_PORT" -gt 65535 ]; then
  echo "Invalid --gateway-port: ${GATEWAY_PORT}" >&2
  exit 2
fi

if [ ! -x "$BUILD_SCRIPT" ]; then
  echo "Missing executable build script: ${BUILD_SCRIPT}" >&2
  exit 1
fi

if ! command -v ssh >/dev/null 2>&1; then
  echo "Missing local dependency: ssh" >&2
  exit 1
fi

if ! command -v scp >/dev/null 2>&1; then
  echo "Missing local dependency: scp" >&2
  exit 1
fi

if [ -z "$PACKAGE_NAME" ]; then
  stamp="$(date +%Y%m%d%H%M%S)"
  if git -C "$ROOT_DIR" rev-parse --short=12 HEAD >/dev/null 2>&1; then
    PACKAGE_NAME="arkloop-deploy-$(git -C "$ROOT_DIR" rev-parse --short=12 HEAD)-${stamp}"
  else
    PACKAGE_NAME="arkloop-deploy-${stamp}"
  fi
fi

remote_quote() {
  local value="$1"
  printf "'%s'" "${value//\'/\'\\\'\'}"
}

remote_path_expr() {
  local value="$1"
  if [ "$value" = "~" ]; then
    printf '$HOME'
  elif [[ "$value" == "~/"* ]]; then
    printf '"$HOME"/%s' "$(remote_quote "${value#~/}")"
  else
    remote_quote "$value"
  fi
}

run_ssh() {
  if [ "${#SSH_OPTS[@]}" -gt 0 ]; then
    ssh "${SSH_OPTS[@]}" "$@"
  else
    ssh "$@"
  fi
}

run_scp() {
  if [ "${#SSH_OPTS[@]}" -gt 0 ]; then
    scp "${SSH_OPTS[@]}" "$@"
  else
    scp "$@"
  fi
}

mkdir -p "$OUTPUT_DIR"
ARCHIVE_PATH="${OUTPUT_DIR}/${PACKAGE_NAME}.tar.gz"
REMOTE_ARCHIVE="${REMOTE_DIR%/}/${PACKAGE_NAME}.tar.gz"
REMOTE_PACKAGE_DIR="${REMOTE_DIR%/}/${PACKAGE_NAME}"
REMOTE_DIR_EXPR="$(remote_path_expr "$REMOTE_DIR")"
REMOTE_PACKAGE_DIR_EXPR="$(remote_path_expr "$REMOTE_PACKAGE_DIR")"

build_args=(
  --output-dir "$OUTPUT_DIR"
  --name "$PACKAGE_NAME"
  --gateway-port "$GATEWAY_PORT"
  --image-repository "$IMAGE_REPOSITORY"
  --version "$IMAGE_VERSION"
  --force
)

if [ "$INCLUDE_SOURCE" = "1" ]; then
  build_args+=(--include-source)
fi

echo "Building deployment package..."
"$BUILD_SCRIPT" "${build_args[@]}"

echo "Preparing remote directory: ${REMOTE}:${REMOTE_DIR}"
run_ssh "$REMOTE" "mkdir -p ${REMOTE_DIR_EXPR}"

echo "Uploading package: ${ARCHIVE_PATH}"
RESOLVED_REMOTE_DIR="$(run_ssh "$REMOTE" "eval echo ${REMOTE_DIR_EXPR}")"
run_scp "$ARCHIVE_PATH" "${REMOTE}:${RESOLVED_REMOTE_DIR}/$(basename "$REMOTE_ARCHIVE")"

echo "Unpacking package on server..."
run_ssh "$REMOTE" "cd ${REMOTE_DIR_EXPR} && tar -xzf '$(basename "$REMOTE_ARCHIVE")'"

if [ "$RUN_INSTALL" = "1" ]; then
  echo "Stopping any existing Arkloop deployment..."
  run_ssh "$REMOTE" "cd ${REMOTE_DIR_EXPR} && for d in arkloop-deploy-*/; do [ -f \"\$d/compose.yaml\" ] && (cd \"\$d\" && docker compose down --remove-orphans 2>/dev/null || true); done"
  echo "Running remote installer..."
  run_ssh "$REMOTE" "cd ${REMOTE_PACKAGE_DIR_EXPR} && ARKLOOP_GATEWAY_PORT='${GATEWAY_PORT}' ARKLOOP_IMAGE_REPOSITORY='${IMAGE_REPOSITORY}' ARKLOOP_VERSION='${IMAGE_VERSION}' ./install.sh"
else
  echo "Upload complete. Remote package directory: ${REMOTE_PACKAGE_DIR}"
fi

echo "Deployment command finished."
echo "Entry URL: http://<server-ip>:${GATEWAY_PORT}"
