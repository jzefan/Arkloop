# Go Worker 迁移基线（WG00）

本文定义 Go Worker 迁移第一步（WG00）的基线资产与验收标准，目标是把 Python 现有行为冻结为可重复验证的契约。

## 1. 基线范围

WG00 只做契约冻结，不引入任何生产行为变化。

覆盖范围：
- `jobs.payload_json` 关键字段契约
- `run_events` 事件序列与终态契约
- Queue 核心语义（lease/ack/nack/dead-letter）
- Worker 消费回放语义（SSE 续传与 seq 单调递增）

## 2. 基线用例清单

### 2.1 Queue 协议基线
- 用例文件：`src/tests/integration/test_job_queue_pg_integration.py`
- 覆盖点：
  - 并发 lease 互斥
  - payload 与 `WorkerJobPayload` 兼容
  - 达到 `max_attempts` 后 dead-letter

### 2.2 Worker 消费基线
- 用例文件：`src/tests/integration/test_worker_consumer_loop_integration.py`
- 覆盖点：
  - API enqueue -> Worker run_once -> job done
  - SSE 回放与续传
  - 同 run 并发 job 去重（advisory lock）

### 2.3 Golden 资产校验
- 资产文件：`src/tests/contracts/golden/run-events/run_execute_success.v1.json`
- 用例文件：`src/tests/integration/test_worker_migration_baseline_integration.py`
- 覆盖点：
  - `seq` 严格递增
  - 终态事件唯一且位于序列末尾
  - `run.started -> worker.job.received -> 终态` 关键顺序稳定

## 3. 验收命令

```bash
docker compose up -d postgres
ARKLOOP_LOAD_DOTENV=1 /Users/qqqqqf/Documents/Arkloop/.venv312/bin/python -m pytest -m integration \
  /Users/qqqqqf/Documents/Arkloop/src/tests/integration/test_job_queue_pg_integration.py \
  /Users/qqqqqf/Documents/Arkloop/src/tests/integration/test_worker_consumer_loop_integration.py \
  /Users/qqqqqf/Documents/Arkloop/src/tests/integration/test_worker_migration_baseline_integration.py
```

## 4. 失败判定

任一条件成立即判定 WG00 未通过：
- 集成测试有失败或报错
- Golden 校验失败（事件顺序、seq、终态不满足约束）
- `test_worker_consumer_loop_integration.py` 出现外部 provider 不稳定导致的非确定性失败

## 5. 约束与回滚

- WG00 不涉及运行时路径切换，无需业务回滚。
- 若后续 WG 变更导致基线失败，需先修复语义偏差，再继续迁移。
