# 书本知识库（KB + RAG）部署 Checklist

> 配套 `docs/prd/book-kb-rag.md`、`docs/superpowers/specs/2026-05-21-book-kb-rag-m1.0-acceptance.md`。

## A. 自动化已包办的事 ✅

执行 `scripts/deploy-source-to-server.sh`（或本地 `docker compose up -d`）时会自动：

1. 应用 KB 相关 migration（`00192_kb_chunks.sql`、`00193_kb_full_schema.sql`），建出 `kb_chunks` / `kb_documents` / `knowledge_bases` / `kb_knowledge_points` 四张表
2. `kb_chunks.embedding` 列固定为 `vector(1024)`，与 `text-embedding-v3` / `doubao-embedding-text-240715` 默认输出维度一致
3. 注册 `/v1/knowledge-bases/*` REST 路由（kbapi）+ console-lite 知识库管理页 + 侧边栏入口
4. 注册 worker `kb.ingest` job 处理器 + `kb_search` 内置工具 + `book-tutor-agent` persona

## B. 必须手工配置的三件事

### B1. `.env`：Embedding 服务

KB ingest / search 都依赖 OpenAI-compatible `/embeddings` 接口。任选一家（推荐 DashScope，对外公开 model）：

**DashScope（阿里云，Qwen）**（实测可用，无需自建 endpoint）：

```bash
ARK_API_KEY=<DashScope API key（通常等同 ARKLOOP_QWEN_API_KEY）>
ARK_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1
ARK_EMBED_MODEL=text-embedding-v3
ARK_EMBED_DIM=1024
ARK_EMBED_BATCH=10   # DashScope 限制 batch ≤ 10

# Worker 读的是 ARK_EMBED_* 前缀，必须同时配置：
ARK_EMBED_API_KEY=<同上 ARK_API_KEY>
ARK_EMBED_BASE_URL=https://dashscope.aliyuncs.com/compatible-mode/v1
```

**Volcengine Ark（字节，Doubao）**：

```bash
ARK_API_KEY=<Ark API key（等同 ARKLOOP_DOUBAO_API_KEY）>
ARK_BASE_URL=https://ark.cn-beijing.volces.com/api/v3
# 控制台先创建 embedding 推理接入点（必须绑定 doubao-embedding-text-* 系列，
# vision 系列和 chat 系列都不支持 /embeddings 调用）
ARK_EMBED_MODEL=ep-xxxxxxxxxxxxx
ARK_EMBED_API_KEY=<同上>
ARK_EMBED_BASE_URL=https://ark.cn-beijing.volces.com/api/v3
```

> **维度警告**：`ARK_EMBED_DIM` 必须等于 endpoint 实际输出维度。否则插入 `kb_chunks` 会触发 `ErrDimMismatch`，ingest 永远失败。
> 自检：`cd src/services/api && ARK_API_KEY=... go run ./cmd/embedprobe -base-url ... -model ...` 会打印实际 dim。

### B2. `.env`：Worker 订阅 `kb.ingest` 队列

worker 默认只 lease `run.execute`，必须显式加 `kb.ingest`，否则文档永远停在「排队中」：

```bash
ARKLOOP_WORKER_QUEUE_JOB_TYPES=run.execute,kb.ingest,webhook.deliver,email.send
```

> 不要加未注册的 job_type（如 `context.compact_maintain`），worker 会启动失败。

### B3. 把目标管理员账号提为 `platform_admin`

console-lite 入口（含「知识库」菜单）门禁 `platform.admin` 权限。`account_memberships.role` 默认 `account_admin`，需手动提权一次：

```sql
-- 替换 username
UPDATE account_memberships m
SET role = 'platform_admin',
    role_id = (SELECT id FROM rbac_roles WHERE name = 'platform_admin')
WHERE m.user_id = (SELECT id FROM users WHERE username = 'admin');
```

或在首次部署时通过 env 让 API 启动自动提权一次：

```bash
ARKLOOP_BOOTSTRAP_PLATFORM_ADMIN=<目标用户的 UUID>
```

> JWT 的 `role` claim 在登录时烤死，提权后**必须重新登录**才生效。

## C. 验证

```bash
# 1. DB 已升到 v193（含 KB schema）
docker exec arkloop-postgres-1 psql -U arkloop -d arkloop -c \
  "SELECT max(version_id) FROM goose_db_version WHERE is_applied;"
# 预期：193

# 2. KB 路由可达（401 表示路由已注册）
curl -i http://localhost:19000/v1/knowledge-bases
# 预期：401（无认证），不是 404

# 3. Embedding endpoint 通且维度对
cd src/services/api && ARK_API_KEY=$ARK_API_KEY \
  go run ./cmd/embedprobe -base-url "$ARK_BASE_URL" -model "$ARK_EMBED_MODEL"
# 预期：model=... dim=1024 batch=1 latency_ms=...

# 4. 浏览器走查：console-lite → 知识库 → 创建 → 上传 .txt → 等 ready → 搜索
```
