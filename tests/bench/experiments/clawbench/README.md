# Arkloop ClawBench Runner

这个目录只放 Arkloop 对接 ClawBench 的 runner。ClawBench 任务库不要克隆到仓库内，runner 只接受这个位置：

```bash
/Users/qqqqqf/Documents/claw-bench
```

准备任务库：

```bash
git clone --depth 1 https://github.com/claw-bench/claw-bench.git ~/Documents/claw-bench
```

安装 verifier 依赖：

```bash
python3 -m venv /tmp/arkloop-clawbench-venv
/tmp/arkloop-clawbench-venv/bin/python -m pip install pytest pytest-json-report
```

列出 quick 任务：

```bash
/tmp/arkloop-clawbench-venv/bin/python tests/bench/experiments/clawbench/arkloop_clawbench_runner.py \
  --list-tasks
```

跑单个任务：

```bash
/tmp/arkloop-clawbench-venv/bin/python tests/bench/experiments/clawbench/arkloop_clawbench_runner.py \
  --task file-002 \
  --host http://127.0.0.1:19002 \
  --persona work \
  --model accounts/fireworks/models/deepseek-v4-pro
```

跑 quick 集：

```bash
/tmp/arkloop-clawbench-venv/bin/python tests/bench/experiments/clawbench/arkloop_clawbench_runner.py \
  --suite quick \
  --host http://127.0.0.1:19002 \
  --persona work \
  --model accounts/fireworks/models/deepseek-v4-pro
```

`--model` 必须显式传入。runner 不会隐式使用 Arkloop 默认模型。

结果默认写到 `/tmp/arkloop-clawbench-runs/<run-id>`，workspace 默认写到 `/tmp/arkloop-clawbench-workspaces/<run-id>`。如果覆盖输出路径，也必须放在 `/tmp` 下。
每个任务目录会保存 prompt、Arkloop CLI 输出、Arkloop run events、pytest 输出和单任务结果。

任务只有在 Arkloop run 状态为 `completed` 且 pytest verifier 通过时才算 `passed=true`。
如果 ClawBench 任务自身 setup 失败，runner 会记录 `bench_error=true` 并继续跑后续任务；这类错误不计入 `arkloop_errors`。
