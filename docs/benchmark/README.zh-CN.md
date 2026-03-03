# G2 压力测试基线

本目录保存单节点压测脚本与一次基线结果。

假设与边界：

- 基线结果来自 docker compose 单机开发环境，不同机器差异很大；用于趋势参考，不作为跨环境硬门槛

## Prerequisites

启动专用 bench compose（端口为标准端口 +5，不影响你的日常开发 compose）：

```bash
docker compose -f compose.bench.yaml -p arkloop-bench up -d
```

说明：

- bench 默认端口：
  - Gateway：`http://127.0.0.1:8005`
  - API：`http://127.0.0.1:8006`
  - Browser：`http://127.0.0.1:3105`
  - Postgres：`127.0.0.1:5437`（用于 bench 自动注册/bootstrapping）
- baseline suite 默认不包含 OpenViking 场景（默认跳过，需显式开启）
- `compose.bench.yaml` 默认不配置 OpenViking / Browser / Sandbox 工具，并且不注入 LLM 路由配置，目标是让基线不依赖外部网络与付费模型（Worker 默认走 stub）

bench 自动注册依赖 `DATABASE_URL`（连到 bench 的 Postgres）：

```bash
export DATABASE_URL="postgresql://arkloop:<你的 ARKLOOP_POSTGRES_PASSWORD>@127.0.0.1:5437/arkloop"
```

## Recommended Env

`compose.bench.yaml` 已内置基线推荐值（通常不需要你再额外设置）：

```bash
# Gateway：避免 429 干扰吞吐与延迟
ARKLOOP_RATELIMIT_CAPACITY=120000
ARKLOOP_RATELIMIT_RATE_PER_MINUTE=120000

# API：并发 run 上限（默认 10，不满足 worker_runs=50）
ARKLOOP_LIMIT_CONCURRENT_RUNS=60

# Worker：并发执行（默认 4，会导致 runs 排队超时）
ARKLOOP_WORKER_CONCURRENCY=50
```

认证方式（二选一）：

- 显式提供 token：`-access-token "$ARKLOOP_BENCH_ACCESS_TOKEN"`
- 自动注册：需要 `DATABASE_URL` 可用；并在必要时加 `-force-open-registration`

## Run Baseline

在仓库根目录执行：

```bash
go run ./tests/bench/cmd/bench baseline \
  -out docs/benchmark/baseline-2026-03-03.json
```

如果你不想提前 `export DATABASE_URL`，也可以用一条命令内联（推荐在 bench compose 下使用）：

```bash
DATABASE_URL="postgresql://arkloop:<你的 ARKLOOP_POSTGRES_PASSWORD>@127.0.0.1:5437/arkloop" \
  go run ./tests/bench/cmd/bench baseline -out docs/benchmark/baseline-2026-03-03.json
```

如果只是本地跑一次、不想改动仓库里的基线文件，建议输出到临时路径：

```bash
go run ./tests/bench/cmd/bench baseline -out /tmp/arkloop-baseline.json
```

可选开启 OpenViking：

```bash
go run ./tests/bench/cmd/bench baseline \
  -include-openviking \
  -openviking-root-key "$ARKLOOP_OPENVIKING_ROOT_API_KEY" \
  -out docs/benchmark/baseline-2026-03-03.json
```

## Interpretation

- 输出为 JSON，`overall_pass=false` 时进程退出码为 1
- `results[].pass` 是单项结论
- `results[].stats` 包含指标与采样结果（例如 `latency_ms`、`retention`、`run_completion_ms`）
- `browser_navigate.stats.docker_memory_start` / `browser_navigate.stats.docker_memory` 为开始/结束采样值（非峰值）。如需记录峰值，建议旁路运行 `docker stats` 观察，并把峰值写入基线文件顶层 `manual_notes`（例如 `{"browser_memory_peak":"2.8GiB"}`）

## OpenViking

OpenViking 的压测默认不在 baseline suite 中执行，避免触发外部 embedding / VLM 调用导致成本与波动；需要显式 `-include-openviking`，并提供 root key。

当前 `compose.bench.yaml` 不包含 OpenViking 服务。如果你要测 OpenViking：

- 自行启动 OpenViking（独立 compose/容器），并确保 bench 能访问到它
- 运行 baseline 时显式传 `-include-openviking -openviking-base-url ... -openviking-root-key ...`

## Troubleshooting

- `gateway.not_ready` / `api.not_ready` / `browser.not_ready` / `openviking.not_ready`：服务未就绪（检查对应服务 `/healthz` 或 OpenViking `/health`）
- `auth.register.code.auth.invite_code_required`：注册模式为 invite_only（需要配置 `registration.open=true` 或提供邀请码/显式 token）
- `worker_runs.runs_create_failed` 很高：通常是 `limit.concurrent_runs` 太低或 Worker 未消费队列
