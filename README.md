# ZenMUArkDesktop

中文 | [English](#english)

ZenMUArkDesktop 是基于 Arkloop 的桌面端定制版本，重点加入了 ZenMux 模型入口、图片生成、视频生成、参考图输入、会话产物导出、工作目录与 Skill 等桌面使用能力。

本仓库基于上游开源项目：

- 上游仓库 / Upstream: [qqqqqf-q/Arkloop](https://github.com/qqqqqf-q/Arkloop)
- 本仓库 / This fork: [bennix/ZenMUArkDesktop](https://github.com/bennix/ZenMUArkDesktop)

## 说明

本仓库是一个干净发布副本，不包含本地 `.env`、构建产物、桌面 release 目录、真实 API Key 或本地配置文件。ZenMux / OpenAI / Anthropic / Gemini 等服务的 API Key 需要用户在应用设置中自行配置。

## 主要改动

- ZenMux 供应商配置入口
- ZenMux 模型列表加载与筛选
- 图片生成与视频生成工具链
- 最多 5 张参考图上传与缩略图展示
- 生成图片的保存、复制、平移、缩放
- 生成视频播放
- Markdown 与 LaTeX 渲染
- Skill 导入与对话框 `~` 调用
- 当前工作目录选择、文件上下文 `@` 调用
- 会话批量删除
- 会话批量导出，包含消息、图片、视频、源码、附件等产物

## 开发

### 本地运行桌面版（推荐）

前置依赖：

- Node.js / pnpm
- Go 工具链（`go.work` 使用 Go `1.26.0`；运行 `go version` 应能看到版本）

桌面版会自动构建本地 sidecar、启动 Vite 开发服务，并打开 Electron 应用：

```bash
pnpm install

cd src/apps/desktop
pnpm dev
```

启动后，在应用设置中配置 ZenMux / OpenAI / Anthropic / Gemini 等服务的 API Key。

### 本地运行 Web + 后端服务

如需运行完整 Web 与后端服务，请先准备 Docker，然后复制并修改环境变量：

```bash
cp .env.example .env
```

至少需要在 `.env` 中修改这些值：

```env
ARKLOOP_POSTGRES_PASSWORD=your_postgres_password
ARKLOOP_REDIS_PASSWORD=your_redis_password
ARKLOOP_AUTH_JWT_SECRET=please_use_at_least_32_characters
ARKLOOP_ENCRYPTION_KEY=64_hex_characters_generated_by_openssl
```

`ARKLOOP_ENCRYPTION_KEY` 可以这样生成：

```bash
openssl rand -hex 32
```

启动完整服务：

```bash
docker compose -f compose.yaml -f compose.dev.yaml up --build
```

常用地址：

- Web: <http://localhost:19080>
- Gateway: <http://localhost:19000>
- API health: <http://localhost:19001/healthz>

### 只运行 Web 前端

如果后端或 Gateway 已经在本机 `19000` 端口运行，可以只启动 Web 前端：

```bash
cd src/apps/web
pnpm dev
```

默认会连接 `http://127.0.0.1:19000`。

### 构建

```bash
pnpm install

cd src/apps/web
pnpm type-check
pnpm build

cd ../desktop
pnpm type-check
pnpm build:web
pnpm build:electron
pnpm exec electron-builder --mac --arm64 --dir
```

## 安全

不要把真实 API Key、`.env`、桌面本地配置、数据库、release 包或日志提交到仓库。仓库已包含 `.gitignore` 规则，但提交前仍建议运行：

```bash
rg -n --hidden --glob '!**/.git/**' 'sk-[A-Za-z0-9_-]{20,}|sk-ss-v1-[A-Za-z0-9]+|api[_-]?key'
```

## 许可证

本项目继承上游 Arkloop 的许可证与限制。详见 [LICENSE](LICENSE)。

---

# English

[中文](#zenmuarkdesktop) | English

ZenMUArkDesktop is a desktop-focused customization based on Arkloop. It adds ZenMux model integration, image generation, video generation, reference image input, conversation artifact export, workspace directory support, and Skill workflows.

This repository is based on the upstream open-source project:

- Upstream: [qqqqqf-q/Arkloop](https://github.com/qqqqqf-q/Arkloop)
- This fork: [bennix/ZenMUArkDesktop](https://github.com/bennix/ZenMUArkDesktop)

## Notes

This repository is a clean publishing copy. It does not include local `.env` files, build artifacts, desktop release outputs, real API keys, or local configuration files. API keys for ZenMux, OpenAI, Anthropic, Gemini, and other providers must be configured by the user in the app settings.

## Highlights

- ZenMux provider configuration
- ZenMux model loading and filtering
- Image and video generation toolchains
- Up to 5 reference image uploads with thumbnails
- Generated image save, copy, pan, and zoom
- Generated video playback
- Markdown and LaTeX rendering
- Skill import and `~` invocation in chat
- Workspace directory selection and `@` file context insertion
- Bulk conversation deletion
- Bulk conversation export with messages, images, videos, source code, attachments, and other artifacts

## Development

### Run Desktop Locally (Recommended)

Prerequisites:

- Node.js / pnpm
- Go toolchain (`go.work` uses Go `1.26.0`; `go version` should print a version)

The desktop app builds the local sidecar, starts the Vite development server, and opens the Electron app:

```bash
pnpm install

cd src/apps/desktop
pnpm dev
```

After launch, configure API keys for ZenMux / OpenAI / Anthropic / Gemini and other providers in the app settings.

### Run Web + Backend Locally

To run the full Web and backend stack, prepare Docker first, then copy and edit the environment file:

```bash
cp .env.example .env
```

At minimum, update these values in `.env`:

```env
ARKLOOP_POSTGRES_PASSWORD=your_postgres_password
ARKLOOP_REDIS_PASSWORD=your_redis_password
ARKLOOP_AUTH_JWT_SECRET=please_use_at_least_32_characters
ARKLOOP_ENCRYPTION_KEY=64_hex_characters_generated_by_openssl
```

Generate `ARKLOOP_ENCRYPTION_KEY` with:

```bash
openssl rand -hex 32
```

Start the full stack:

```bash
docker compose -f compose.yaml -f compose.dev.yaml up --build
```

Common URLs:

- Web: <http://localhost:19080>
- Gateway: <http://localhost:19000>
- API health: <http://localhost:19001/healthz>

### Run Web Only

If the backend or Gateway is already running on local port `19000`, start only the Web frontend:

```bash
cd src/apps/web
pnpm dev
```

By default, it connects to `http://127.0.0.1:19000`.

### Build

```bash
pnpm install

cd src/apps/web
pnpm type-check
pnpm build

cd ../desktop
pnpm type-check
pnpm build:web
pnpm build:electron
pnpm exec electron-builder --mac --arm64 --dir
```

## Security

Do not commit real API keys, `.env` files, local desktop configuration, databases, release packages, or logs. The repository includes `.gitignore` rules, but it is still recommended to scan before committing:

```bash
rg -n --hidden --glob '!**/.git/**' 'sk-[A-Za-z0-9_-]{20,}|sk-ss-v1-[A-Za-z0-9]+|api[_-]?key'
```

## License

This project inherits the upstream Arkloop license and restrictions. See [LICENSE](LICENSE).
