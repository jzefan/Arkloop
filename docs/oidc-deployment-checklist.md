# OIDC + 题库助手部署 Checklist

> 配套 `docs/oidc-design.md`。

## 主线：让老师在 ArkLoop 里通过题库助手驱动 exam

老师**只**在 ArkLoop 里完成所有 exam 操作。exam 端账号在 worker 第一次调用 exam API 时自动建出来；老师从未打开 exam UI 的浏览器界面。

---

## A. 自动化已包办的事 ✅

执行 `scripts/deploy-source-to-server.sh` 时，**部署脚本会自动**：

1. ✅ 同步源代码到远端
2. ✅ 检查 `.env` 中 `ARKLOOP_INTERNAL_SERVICE_TOKEN`：缺失或占位符时自动生成（48 字节 url-safe 随机串）；已设置则保留
3. ✅ 跑 setup.sh install（含 migration 自动应用 — `00186`–`00191` 全部加上）
4. ✅ 编译并打包 `oauth-seed` CLI 进 api 镜像，等 api 起来后**幂等注册** `exam-web` OAuth client（首次创建 / 重复部署跳过）
5. ✅ 启动 web、worker、gateway 全部服务

部署完成后，ArkLoop 端**0 手工配置**，全套 OIDC + 智能体链路就绪。

---

## B. 你必须手工配置的两件事

### B1. 服务端 `.env` 里指向 exam 部署

部署后 `ssh` 到远端，编辑 `${REMOTE_DIR}/.env`，确认这一行指向你的 exam 后端：

```bash
# 例如 exam 部署在同一台机器另一个 compose 网络，用 host.docker.internal：
EXAM_BASE_URL=http://host.docker.internal:8000

# 例如 exam 在另一台机器：
EXAM_BASE_URL=https://exam.example.com
```

修改后：
```bash
docker compose restart worker api
```

### B2. exam 端 `.env` 指回 ArkLoop（拉 JWKS 验签）

在 exam 部署的 `.env` 中加：
```bash
EXAM_OIDC_ISSUER=http://<arkloop-host>:<gateway-port>
# 例：http://192.168.0.10:20083 或 https://arkloop.example.com
```

跑 exam 的 alembic migration（首次部署或升级才需要）：
```bash
cd ~/work/proj/exam/backend && alembic upgrade head
```

重启 exam 后端服务。

---

## C. 验证主线链路（5 分钟）

1. **ArkLoop 浏览器**：访问 web app → 注册一个新账号（写邮箱、姓名）
2. **选择 persona**：题库助手
3. **上传图片或 Excel**：发"帮我导入这个课程的目录"+ 附件
4. **观察对话**：智能体应当调用 `exam_recognize_catalog_image` 或 `exam_parse_catalog_excel`，展示识别结果
5. **确认导入**：调用 `exam_create_catalog_tree`，返回 `knowledge_points_created: N`
6. **验证 exam 端入库**：
   ```sql
   -- 老师账号自动 provision 出来了
   SELECT username, email, oidc_subject, provider
   FROM users
   WHERE provider = 'arkloop'
   ORDER BY id DESC LIMIT 5;

   -- 课程目录入库了
   SELECT name, level FROM knowledge_points
   ORDER BY created_at DESC LIMIT 20;
   ```
7. **生成题目**：发"为某个章节生成 3 道单选题" → 智能体调 `exam_generate_questions` → exam.questions 表新增

老师**全程没打开 exam 浏览器界面**。

---

## D. 运维后门（可选，平时用不到）

如果某天老师真要进 exam 浏览器界面看东西，运维做这些**额外**配置：

### D1. exam `.env` 加 OIDC client 凭证

```bash
EXAM_OIDC_CLIENT_ID=exam-web
EXAM_OIDC_CLIENT_SECRET=<从下面 D2 拿>
EXAM_OIDC_REDIRECT_URI=http://<exam-host>:8000/api/auth/oidc/callback
```

### D2. 取一份新的 client_secret

部署脚本注册的初始 secret **没记录**（自动化场景不需要）。运维需要时手动 rotate 一份：

```bash
ssh <arkloop-host>
cd <REMOTE_DIR>
docker compose exec api /usr/local/bin/oauth-seed \
  -client-id exam-web -rotate
# 终端打印新 secret，保存到密码管理器，填到 exam 的 .env
```

### D3. 给老师生成进入 exam 的链接

老师在浏览器访问 `https://<exam-host>/api/auth/oidc/authorize?next=/`，会被引导走 SSO，全程无需输密码。

注意 exam 登录页**不再有** "使用 ArkLoop 账号登录" 按钮（产品决策已删），所以这是唯一进入路径。

---

## E. 回滚预案

| 场景 | 操作 |
|---|---|
| 智能体调 exam 报错 401 | 检查 worker 日志，确认 `ARKLOOP_INTERNAL_SERVICE_TOKEN` 在 api 和 worker 两边一致 |
| exam 验签失败 | 检查 exam 端 `EXAM_OIDC_ISSUER` 是否能解析到 ArkLoop；`curl $ISSUER/.well-known/jwks.json` 应返回 RSA 公钥 |
| 老师重复出现 | `oidc_subject` 不变前提下两次 provision 应当 link 到同一行；如有重复行说明 unique index 没生效，查 migration `20260520_oidc_to_users` 是否应用 |
| OIDC 私钥泄露 | `UPDATE oidc_signing_keys SET status='compromised' WHERE kid='...'; ` 然后重启 api，自动生成新 keypair |
| 全量回滚 | 设计文档 §13 表格 |

---

## F. 不在 v1 范围

参见 `docs/oidc-design.md` §15：v1 不实现 RP-Initiated Logout / Dynamic Client Registration / Token Introspection / MTLS。

## 测试覆盖

| 层 | 状态 |
|---|---|
| `auth/oidc/signer_test.go` | ✅ 8/8 通过（不依赖 DB） |
| Repos 集成测试 | TODO — 需要 `ARKLOOP_DATABASE_URL` |
| 端到端浏览器测试 | 手动 §C |
| 智能体闭环测试 | 手动 §C 步骤 4-7 |
