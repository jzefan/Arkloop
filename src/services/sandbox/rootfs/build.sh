#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"

OUTPUT_DIR="${OUTPUT_DIR:-$SCRIPT_DIR/output}"
OUTPUT_FILE="python3.12.ext4"

echo "building rootfs: $OUTPUT_FILE"
echo "context: $PROJECT_ROOT"

mkdir -p "$OUTPUT_DIR"

docker buildx build \
    --platform linux/amd64 \
    --file "$SCRIPT_DIR/Dockerfile.python3.12" \
    --output "type=local,dest=$OUTPUT_DIR" \
    "$PROJECT_ROOT"

echo "output: $OUTPUT_DIR/$OUTPUT_FILE"
ls -lh "$OUTPUT_DIR/$OUTPUT_FILE"

if [[ -n "${DEPLOY_HOST:-}" ]]; then
    DEPLOY_PATH="${DEPLOY_PATH:-/opt/sandbox/rootfs}"
    echo "deploying to $DEPLOY_HOST:$DEPLOY_PATH"
    ssh "$DEPLOY_HOST" "mkdir -p $DEPLOY_PATH"
    scp "$OUTPUT_DIR/$OUTPUT_FILE" "$DEPLOY_HOST:$DEPLOY_PATH/$OUTPUT_FILE"
    echo "deployed"
fi
