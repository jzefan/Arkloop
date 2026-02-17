# Go API 迁移基线（P.01）

本文用于把当前 Python API 的关键对外契约冻结为可重复验证的回归资产，服务于后续 “Go API + Go Worker” 的薄片迁移。

## 1. 基线范围

P.01 只做契约冻结，不引入任何生产行为变化。

覆盖点：
- Auth：`/v1/auth/login`、`/v1/auth/refresh`、`/v1/auth/logout`、`/v1/auth/register`、`/v1/me`
- 资源最小闭环：threads/messages/runs
- SSE：`GET /v1/runs/{run_id}/events`
  - `after_seq` 续传
  - `follow=true` 心跳（comment ping）
- Queue 协议：`jobs.payload_json` 的 v1 关键字段（API enqueue 与 Worker 解析一致）

## 2. 资产文件

- 契约测试：`src/tests/contracts/test_backend_api_contracts.py`
- Golden（job payload）：`src/tests/contracts/golden/job-payload/run_execute.v1.json`

## 3. 验收命令

先启动 Postgres：

```bash
docker compose up -d postgres
```

执行契约回归：

```bash
python -m pytest -m integration src/tests/contracts/test_backend_api_contracts.py
```

说明：
- 用例会优先读取 `.env.test`（若存在）来获得 `ARKLOOP_DATABASE_URL`。
- 如你习惯显式指定，也可以手动设置 `ARKLOOP_DATABASE_URL`/`DATABASE_URL` 指向本地 Postgres。

## 4. 失败判定

任一条件成立即判定 P.01 未通过：
- 契约测试失败（HTTP 状态码、ErrorEnvelope、SSE 续传/心跳、job payload 协议）
- Golden 校验失败（字段缺失或版本不一致）

