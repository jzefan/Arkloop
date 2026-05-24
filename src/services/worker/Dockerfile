# syntax=docker/dockerfile:1

# --- Builder: Debian (glibc) for onnxruntime compatibility ---
FROM golang:1.26-bookworm AS builder
ENV GOPROXY=https://goproxy.cn,https://goproxy.io,https://proxy.golang.org,direct
ENV GOSUMDB=sum.golang.google.cn

ARG TARGETARCH
ARG ORT_VERSION=1.24.1

RUN apt-get update && apt-get install -y --no-install-recommends wget && rm -rf /var/lib/apt/lists/*

RUN ORT_ARCH=$([ "$TARGETARCH" = "arm64" ] && echo "aarch64" || echo "x64") && \
    wget -q "https://github.com/microsoft/onnxruntime/releases/download/v${ORT_VERSION}/onnxruntime-linux-${ORT_ARCH}-${ORT_VERSION}.tgz" \
      -O /tmp/ort.tgz && \
    mkdir -p /opt/onnxruntime && \
    tar xzf /tmp/ort.tgz -C /opt/onnxruntime --strip-components=1 && \
    rm /tmp/ort.tgz

WORKDIR /workspace
COPY src/services/worker/go.mod src/services/worker/go.sum src/services/worker/
COPY src/services/shared/go.mod src/services/shared/go.sum src/services/shared/
COPY src/services/api/go.mod src/services/api/go.sum src/services/api/
RUN printf 'go 1.26.0\nuse (\n\t./src/services/api\n\t./src/services/worker\n\t./src/services/shared\n)\n' > go.work
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY src/services/worker src/services/worker
COPY src/services/shared src/services/shared
COPY src/services/api src/services/api
COPY src/personas src/personas

ENV CGO_ENABLED=1
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags="-s -w" -o /worker ./src/services/worker/cmd/worker

# --- Runtime: Debian slim (glibc) ---
FROM debian:bookworm-slim
RUN sed -i 's|deb.debian.org|mirrors.aliyun.com|g; s|security.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list.d/debian.sources 2>/dev/null || true
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata curl \
    fonts-arphic-uming \
    fonts-wqy-zenhei \
    python3 \
    python3-pip \
    python3-pil \
    python3-pytesseract \
    tesseract-ocr \
    tesseract-ocr-chi-sim \
    && rm -rf /var/lib/apt/lists/*
RUN pip3 install --no-cache-dir --break-system-packages \
    PyMuPDF pdfplumber python-docx

# Chromium 单独 RUN，失败时降级为警告而非 build 失败：
# 1) aliyun mirror 偶发同步落后（尤其 arm64 channel），不应阻塞主链路 build。
# 2) markdown_to_pdf 是可选工具；缺 chromium 时工具运行时返回 NotConfigured，
#    不影响题库助手 / OIDC 主链路。
RUN apt-get update && \
    (apt-get install -y --no-install-recommends chromium || \
     echo "WARNING: chromium install failed; markdown_to_pdf disabled at runtime") && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /opt/onnxruntime/lib/libonnxruntime*.so* /usr/lib/
RUN ldconfig

# rtk 是 sandbox executor 的可选命令重写工具（缺它时 fallback 到原命令，
# 不影响 worker 启动或题库助手）。ghfast.top 代理偶尔不稳，失败时打 warning
# 继续构建，不阻塞主链路。
# x86_64 二进制：linux/amd64 镜像直接可用；linux/arm64 镜像下 QEMU 跑得起
# 但偏慢——若长期跑 arm64，应换 rtk 的 aarch64 release（项目尚未发布）。
ARG RTK_URL=https://ghfast.top/https://github.com/rtk-ai/rtk/releases/download/v0.40.0/rtk-x86_64-unknown-linux-musl.tar.gz
RUN (curl -fsSL --retry 3 --retry-delay 2 --max-time 90 "$RTK_URL" -o /tmp/rtk.tar.gz \
     && tar -xzf /tmp/rtk.tar.gz -C /usr/local/bin rtk \
     && chmod 755 /usr/local/bin/rtk \
     && rm -f /tmp/rtk.tar.gz) \
    || echo "WARNING: rtk download failed; sandbox executor falls back to raw shell (no optimization)"

COPY --from=builder /worker /usr/local/bin/worker
COPY --from=builder /workspace/src/personas /usr/local/share/arkloop/personas
COPY --from=builder /workspace/src/services/shared/bookparser/sandbox /usr/local/share/arkloop/bookparser
ENV ARKLOOP_PERSONAS_ROOT=/usr/local/share/arkloop/personas
ENV ARKLOOP_BOOKPARSER_RUNNER=/usr/local/share/arkloop/bookparser/bookparser_runner.py
ENTRYPOINT ["worker"]
