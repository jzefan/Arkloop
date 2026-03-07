#!/usr/bin/env bash
set -euo pipefail

resolve_node_arch() {
    local raw_arch="${TARGETARCH:-}"
    if [[ -z "$raw_arch" ]]; then
        raw_arch="$(dpkg --print-architecture)"
    fi
    case "$raw_arch" in
        amd64|x86_64)
            printf 'x64\n'
            ;;
        arm64|aarch64)
            printf 'arm64\n'
            ;;
        *)
            printf 'unsupported TARGETARCH for node: %s\n' "$raw_arch" >&2
            return 1
            ;;
    esac
}

apt-get update
apt-get install -y --no-install-recommends \
    bash \
    busybox \
    ca-certificates \
    coreutils \
    curl \
    wget \
    git \
    jq \
    grep \
    sed \
    gawk \
    findutils \
    diffutils \
    patch \
    make \
    file \
    less \
    tree \
    procps \
    ripgrep \
    tar \
    gzip \
    bzip2 \
    unzip \
    zip \
    xz-utils \
    sqlite3 \
    openssh-client \
    gcc \
    g++ \
    iproute2 \
    fonts-noto-cjk \
    libnss3 \
    libnspr4 \
    libatk1.0-0 \
    libatk-bridge2.0-0 \
    libcups2 \
    libdrm2 \
    libdbus-1-3 \
    libxkbcommon0 \
    libxcomposite1 \
    libxdamage1 \
    libxfixes3 \
    libxrandr2 \
    libgbm1 \
    libpango-1.0-0 \
    libcairo2 \
    libasound2
rm -rf /var/lib/apt/lists/*

NODE_ARCH="$(resolve_node_arch)"

curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" | \
    tar -xJ --strip-components=1 -C /usr/local
npm install -g pnpm@10.5.2

pip install --no-cache-dir -r /tmp/guest-requirements.txt

if ! getent group arkloop >/dev/null 2>&1; then
    groupadd -g 1000 arkloop
fi
if ! id -u arkloop >/dev/null 2>&1; then
    useradd -u 1000 -g 1000 -m -d /home/arkloop -s /bin/bash arkloop
fi

mkdir -p /tmp/output /tmp/arkloop /tmp/matplotlib /workspace /home/arkloop /usr/local/share/arkloop
chown 1000:1000 /tmp/output /tmp/arkloop /workspace /home/arkloop /tmp/matplotlib
