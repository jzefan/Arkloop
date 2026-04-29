# Arkloop ClawBench Runner

这个目录只放 Arkloop 对接 ClawBench 的 runner。ClawBench 任务库不要克隆到仓库内，默认位置是：

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
  --model accounts/fireworks/models/kimi-k2p6
```

跑 quick 集：

```bash
/tmp/arkloop-clawbench-venv/bin/python tests/bench/experiments/clawbench/arkloop_clawbench_runner.py \
  --suite quick \
  --host http://127.0.0.1:19002 \
  --persona work \
  --model accounts/fireworks/models/kimi-k2p6
```

`--model` 必须显式传入。runner 不会隐式使用 Arkloop 默认模型。

结果默认写到 `/tmp/arkloop-clawbench-runs/<run-id>`，workspace 默认写到 `/tmp/arkloop-clawbench-workspaces/<run-id>`。
每个任务目录会保存 prompt、Arkloop CLI 输出、Arkloop run events、pytest 输出和单任务结果。
