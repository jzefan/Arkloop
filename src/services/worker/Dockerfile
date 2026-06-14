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

# --- Runtime: Python slim (Debian/glibc) ---
FROM python:3.12-slim-bookworm
ARG PIP_INDEX_URL=https://pypi.tuna.tsinghua.edu.cn/simple
ARG PIP_DEFAULT_TIMEOUT=180

# OCR, Chinese fonts, and Chromium are optional enhancements. Text-based
# PDF/DOCX ingestion remains available when a regional apt mirror does not
# carry one of these packages.
RUN set -eux; \
    apt_opts='-o Acquire::ForceIPv4=true -o Acquire::Retries=5 -o Acquire::http::Timeout=60 -o Acquire::https::Timeout=60'; \
    if apt-get $apt_opts update; then \
      apt-get $apt_opts install -y --no-install-recommends \
        fonts-arphic-uming \
        fonts-wqy-zenhei \
        tesseract-ocr \
        tesseract-ocr-chi-sim \
        chromium \
      || echo "WARNING: optional OCR/font/Chromium packages unavailable; related features disabled"; \
    else \
      echo "WARNING: apt index unavailable; optional OCR/font/Chromium features disabled"; \
    fi; \
    rm -rf /var/lib/apt/lists/*

RUN pip3 install --no-cache-dir --break-system-packages \
        --index-url "$PIP_INDEX_URL" \
        --timeout "$PIP_DEFAULT_TIMEOUT" \
        --retries 5 \
        Pillow PyMuPDF pdfplumber python-docx pytesseract

COPY --from=builder /opt/onnxruntime/lib/libonnxruntime*.so* /usr/lib/
RUN ldconfig

# rtk 是 sandbox executor 的可选命令重写工具（缺它时 fallback 到原命令，
# 不影响 worker 启动或题库助手）。ghfast.top 代理偶尔不稳，失败时打 warning
# 继续构建，不阻塞主链路。
# x86_64 二进制：linux/amd64 镜像直接可用；linux/arm64 镜像下 QEMU 跑得起
# 但偏慢——若长期跑 arm64，应换 rtk 的 aarch64 release（项目尚未发布）。
ARG RTK_URL=https://ghfast.top/https://github.com/rtk-ai/rtk/releases/download/v0.40.0/rtk-x86_64-unknown-linux-musl.tar.gz
RUN (python3 -c 'import sys, urllib.request; urllib.request.urlretrieve(sys.argv[1], sys.argv[2])' "$RTK_URL" /tmp/rtk.tar.gz \
     && tar -xzf /tmp/rtk.tar.gz -C /usr/local/bin rtk \
     && chmod 755 /usr/local/bin/rtk \
     && rm -f /tmp/rtk.tar.gz) \
    || echo "WARNING: rtk download failed; sandbox executor falls back to raw shell (no optimization)"

COPY --from=builder /worker /usr/local/bin/worker
COPY --from=builder /workspace/src/personas /usr/local/share/arkloop/personas
COPY --from=builder /workspace/src/services/shared/bookparser/sandbox /usr/local/share/arkloop/bookparser
ENV ARKLOOP_PERSONAS_ROOT=/usr/local/share/arkloop/personas
ENV ARKLOOP_BOOKPARSER_RUNNER=/usr/local/share/arkloop/bookparser/bookparser_runner.py

# ffmpeg 用于 video_concat 工具（合并多段 mp4 为完整短片）。
# 走 PyPI 的 imageio-ffmpeg —— 它打包了 johnvansickle 的 static 二进制
# （linux x86_64 + aarch64 都有），用清华 PyPI 镜像下载稳定可达，比走 apt
# 命中 debian-security 国内慢/超时 + 比直接拉 johnvansickle 站点更可靠。
# 放在 Dockerfile 末尾避免 cache invalidation 触发上面 chromium/apt 步骤重跑。
# 失败时打 warning 但不阻塞 build——video_concat 缺时运行期返回
# tool.not_configured，不影响主流水线生成单镜视频。
RUN pip3 install --no-cache-dir --break-system-packages \
        --index-url "$PIP_INDEX_URL" \
        --timeout "$PIP_DEFAULT_TIMEOUT" \
        --retries 5 \
        imageio-ffmpeg && \
    ln -sf "$(python3 -c 'import imageio_ffmpeg; print(imageio_ffmpeg.get_ffmpeg_exe())')" /usr/local/bin/ffmpeg && \
    ffmpeg -version | head -1 \
    || echo "WARNING: imageio-ffmpeg install failed; video_concat will return tool.not_configured at runtime"

ENTRYPOINT ["worker"]
