# Installation

## For Humans

Paste this into your LLM agent (Claude Code, Copilot, Cursor, etc.):

```
Install and configure Arkloop by following the instructions here:
https://raw.githubusercontent.com/arkloop/arkloop/main/docs/installation.md
```

Or read the [installation guide](./installation.md), but seriously, let an agent do it.

## For LLM Agents

> **IMPORTANT: Use `curl` to fetch this file, NOT WebFetch.** WebFetch may summarize content and lose critical details. Always use:
>
> ```bash
> curl -fsSL https://raw.githubusercontent.com/arkloop/arkloop/main/docs/installation.md
> ```

You are helping a user install Arkloop, a self-hosted conversational AI agent platform. Follow these steps in order.

### Step 0: Detect Host Environment

Run the doctor command first. Parse the output to inform later decisions.

```bash
./setup.sh doctor
```

Expected output fields:

| Field | Meaning |
|-------|---------|
| `platform=linux\|macos\|wsl2` | Host OS |
| `docker=1\|0` | Docker daemon reachable |
| `compose=1\|0` | `docker compose` plugin available |
| `docker_socket=<path>\|not-found` | Accessible Docker socket path |
| `kvm=1\|0` | KVM device present (Linux only) |
| `port_19000=free\|in-use` | Default gateway port availability |
| `port_9000=free\|in-use` | SeaweedFS S3 API port availability |

If `docker=0` or `compose=0`, **stop here** and follow the [Docker prerequisite guide](#docker-not-available).

If this is a fresh checkout and `setup.sh` isn't executable yet:

```bash
chmod +x ./setup.sh
```

### Step 1: Ask the User 8 Questions

Ask these questions to determine installation parameters. Keep them in this order. Do not expose internal implementation details (compose profiles, port numbers, Redis database numbers) as options.

---

**Q1 — Deployment profile**

> Do you want the standard self-hosted install, or a full install with all optional modules?

- Standard (recommended): core stack + gateway + console → `--profile standard`
- Full: standard + memory system + code execution + browser → `--profile full`

**Default: standard**

---

**Q2 — Memory system**

> Do you want to enable a persistent memory system for agents (OpenViking)?

- No: agents have no cross-session memory → `--memory none`
- Yes: install and connect OpenViking → `--memory openviking`

**Default: none**

---

**Q3 — Code execution sandbox**

> Do you want agents to be able to execute code in an isolated environment?

- No: disable code execution → `--sandbox none`
- Yes: enable sandbox (auto-detect below) → `--sandbox docker` or `--sandbox firecracker`

**Default: none**

If the user answers Yes, proceed to **Q3a** before continuing.

**Q3a — Sandbox backend (auto-detect, ask only if Q3 = yes)**

Check doctor output:

- `platform=linux` AND `kvm=1` → recommend Firecracker (`--sandbox firecracker`)
- Otherwise → Docker sandbox only (`--sandbox docker`)

Tell the user which backend will be used. Only ask for confirmation if the recommendation differs from their expectation.

---

**Q4 — Web search and scraping**

> Do you want agents to use web search and content scraping?
>
> - **Builtin** (recommended): uses third-party APIs (Tavily + Firecrawl/Jina). Zero infrastructure overhead, but requires API keys and has per-call cost.
> - **Self-hosted**: installs SearXNG + Firecrawl locally. No API costs, but adds resource usage and exposes a crawler from your IP.

- Builtin → `--web-tools builtin`
- Self-hosted → `--web-tools self-hosted`

**Default: builtin**

---

**Q5 — Console tier**

> Do you want the standard Console (recommended) or the full advanced Console?
>
> The full Console includes additional management pages for billing, RBAC, and system configuration. It requires more resources.

- Standard (Console Lite) → `--console lite`
- Full → `--console full`

**Default: lite**

---

**Q6 — Browser module**

> Do you want to enable the browser automation module? (Requires code execution sandbox to be enabled.)

Only ask this if Q3 = yes (sandbox enabled). If sandbox is disabled, set `--browser off` automatically.

- No → `--browser off`
- Yes → `--browser on`

**Default: off**

---

**Q7 — Gateway**

The gateway is the entry point for all traffic. For standard self-hosted installs it should always be enabled.

Set `--gateway on` unless the user explicitly says they want to run the API directly without a gateway.

**Default: on**

---

**Q8 — Deployment mode**

> Are you deploying this for personal or team use (self-hosted), or is this an Arkloop SaaS deployment?

- Self-hosted → `--mode self-hosted`
- SaaS → `--mode saas`

**Default: self-hosted**

> **Note**: `--mode saas` automatically enables PGBouncer (connection pooling), S3 storage (SeaweedFS), and the full Console. Gateway rate limiting and Turnstile placeholders are configured by default. Bootstrap admin URL generation is skipped (handled by CI/CD pipeline).

---

### Step 2: Run the Installer

Construct the install command from the answers above and run it. Always include `--non-interactive`.

```bash
./setup.sh install \
  --profile <standard|full> \
  --mode self-hosted \
  --memory <none|openviking> \
  --sandbox <none|docker|firecracker> \
  --console <lite|full> \
  --browser <off|on> \
  --web-tools <builtin|self-hosted> \
  --gateway <on|off> \
  --non-interactive
```

**Examples:**

Minimal install, no optional modules:

```bash
./setup.sh install \
  --profile standard \
  --mode self-hosted \
  --memory none \
  --sandbox none \
  --console lite \
  --browser off \
  --web-tools builtin \
  --gateway on \
  --non-interactive
```

Full install with memory, Docker sandbox, and self-hosted search:

```bash
./setup.sh install \
  --profile full \
  --mode self-hosted \
  --memory openviking \
  --sandbox docker \
  --console lite \
  --browser off \
  --web-tools self-hosted \
  --gateway on \
  --non-interactive
```

Full install with Firecracker (Linux with KVM):

```bash
./setup.sh install \
  --profile full \
  --mode self-hosted \
  --memory openviking \
  --sandbox firecracker \
  --console lite \
  --browser on \
  --web-tools builtin \
  --gateway on \
  --non-interactive
```

The installer will:

1. Check host prerequisites (Docker, Compose, KVM if needed, port availability)
2. Generate missing secrets and populate `.env`
3. Start core services, then start the gateway
4. Wait for health checks to pass
5. Print the admin initialization URL

#### Production Mode (Pre-built Images)

To use pre-built images from GHCR instead of building locally, add `--prod`:

```bash
./setup.sh install --prod \
  --profile standard \
  --mode self-hosted \
  ...
  --non-interactive
```

This appends `-f compose.prod.yaml` to all Compose commands, pulling multi-arch images (amd64 + arm64) from `ghcr.io/jzefan/arkloop-{service}:latest` by default. You can override the image repository prefix:

```bash
ARKLOOP_IMAGE_REPOSITORY=ghcr.io/your-org/arkloop ./setup.sh install --prod --non-interactive ...
```

You can also pin a version:

```bash
ARKLOOP_VERSION=v0.5.0 ./setup.sh install --prod --non-interactive ...
```

### Step 3: Verify Installation

After the installer completes, verify the stack is running:

```bash
./setup.sh status
```

Check the gateway is reachable:

```bash
curl -sf http://localhost:19000/healthz && echo "gateway ok"
```

If gateway is healthy, Arkloop is running. The Console is accessible at `http://localhost:19000/`.

### Step 4: Complete Admin Initialization

The installer prints a one-time bootstrap URL at the end:

```
管理员初始化地址：http://localhost:19000/bootstrap/<token>
```

Open this URL in a browser (or instruct the user to open it). The user will set their admin username and password. The token is single-use and expires after 30 minutes.

If the URL was not printed (gateway was off, or bootstrap already completed), skip this step.

---

## Parameter Reference

| Flag | Values | Default | Notes |
|------|--------|---------|-------|
| `--profile` | `standard`, `full` | `standard` | `full` enables all optional modules |
| `--mode` | `self-hosted`, `saas` | `self-hosted` | `saas` enables PGBouncer, S3, full Console |
| `--memory` | `none`, `openviking` | `none` | Adds persistent agent memory |
| `--sandbox` | `none`, `docker`, `firecracker` | `none` | Code execution isolation |
| `--console` | `lite`, `full` | `lite` | Full adds advanced management UI |
| `--browser` | `off`, `on` | `off` | Requires sandbox to be enabled |
| `--web-tools` | `builtin`, `self-hosted` | `builtin` | `self-hosted` installs SearXNG + Firecrawl |
| `--gateway` | `on`, `off` | `on` | Entry point proxy; disable only for development |
| `--non-interactive` | flag | — | Must be set when called by an agent |
| `--prod` | flag | — | Use pre-built GHCR images via `compose.prod.yaml`; defaults to `ARKLOOP_IMAGE_REPOSITORY=ghcr.io/jzefan/arkloop` |

---

## Error Handling

### Docker Not Available

Symptom: `docker=0` or `compose=0` in doctor output, or installer fails with `Docker 不可用`.

**Linux:**

```bash
# Install Docker Engine
curl -fsSL https://get.docker.com | sh
sudo usermod -aG docker $USER
# Re-login or run: newgrp docker
```

**macOS:** Install [Docker Desktop](https://www.docker.com/products/docker-desktop/) and start it.

**Windows (WSL2):** Install [Docker Desktop for Windows](https://www.docker.com/products/docker-desktop/) with WSL2 backend enabled.

After Docker is available, re-run `./setup.sh doctor` to confirm, then retry the installer.

---

### Docker Socket Not Found (sandbox=docker)

Symptom: `docker_socket=not-found` in doctor output, or installer fails with `未找到用户态 Docker socket`.

The sandbox requires a user-accessible Docker socket. On Linux, this is usually `/var/run/docker.sock`. Ensure the current user is in the `docker` group:

```bash
sudo usermod -aG docker $USER
newgrp docker
docker info  # should succeed without sudo
```

---

### Firecracker Not Available

Symptom: installer fails with `firecracker 仅支持 Linux` or `当前宿主未检测到 KVM`.

Firecracker requires:
- A Linux host (not macOS, not WSL2)
- KVM support (`/dev/kvm` must exist)

If running in a VM, enable nested virtualization. If running on macOS or WSL2, use `--sandbox docker` instead.

---

### Port Already in Use

Symptom: installer fails with `端口 19000 已被占用` (or similar).

Find what is using the port:

```bash
lsof -i :19000
```

Options:
- Stop the conflicting process
- Change the Arkloop gateway port: set `ARKLOOP_GATEWAY_PORT=<port>` in `.env` before re-running the installer

---

### Pre-flight Failure Summary

If `./setup.sh install` exits with `pre-flight 检测未通过`, the output will list individual warnings. Address each one listed, then re-run the installer. The installer is idempotent — re-running it on an already-partially-installed stack is safe.

---

### Health Check Timeout

Symptom: installer exits with `服务健康检查超时`.

Check what is failing:

```bash
./setup.sh status
docker compose logs --tail=50
```

Common causes: insufficient memory (minimum 2 GB free RAM), port conflicts discovered after startup, or missing required environment variables. Fix the underlying issue, then re-run the installer.

---

### Already Installed

If Arkloop is already running, `./setup.sh install` is safe to re-run — it will not reset secrets or recreate containers unless the profile changes.

To check current installation state:

```bash
./setup.sh status
```
