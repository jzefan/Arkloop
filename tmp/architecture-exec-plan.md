# SaaS 配置与运行契约（执行草案）

状态：当前有效
用途：给其他 AI / 工程师执行修复时使用
范围：Desktop / Self-host / Open Source / Web / Console / API / Worker

## 1. 概念定义

- `Platform`：全局兜底配置源。负责默认模型、默认人格、默认工具策略、默认标题摘要模型。
- `User`：私有覆盖层。只允许放 `BYOK`、个性化 `JSYS`、个人偏好。
- `Project`：长期上下文空间。负责聊天、文件、项目记忆、项目目录。不是模型/人格/工具来源层。
- `Workspace`：运行容器层。负责 sandbox、文件系统挂载、运行时隔离。不是模型/人格/工具来源层。

## 2. 配置优先级

- 唯一有效优先级：`User Override > Platform Fallback`
- UI 中的 `Default` 含义：不使用用户覆盖，直接回到 `Platform Fallback`
- `Project` 和 `Workspace` 不参与模型、人格、工具、标题摘要模型的来源决策

## 3. Project 规则

- 每个用户必须始终存在一个 `Default Project`
- 注册、登录、自愈修复时，如果用户没有任何 Project，系统必须幂等创建一个 `Default Project`
- `Default Project` 不可删除
- 非默认 Project 可以新建
- 切换 Project 只影响上下文，不影响模型/人格/工具来源

## 4. Workspace 规则

- `Workspace` 只负责运行容器和挂载环境
- `Workspace` 可以随 Desktop / Self-host 运行方式不同而不同
- `Workspace` 可以影响“东西在哪跑”，不能影响“模型/人格/工具从哪来”
- 不允许把 Project 直接做成配置来源层，也不允许把 Workspace 重新做成配置来源层

## 5. Run 解析规则

- 在 `Run` 启动时解析一次有效配置，并冻结成 run snapshot
- 解析顺序：
  1. 用户显式选择的 `BYOK / User Override`
  2. `Platform Fallback`
- 如果用户选择 `Default`，运行时不得再查用户私有模型路由
- 已启动的 Run 不受后续平台配置变更影响

## 6. 路由可见性

- 采用 `hybrid`
- `Platform` 路由全局可见，作为全局兜底
- `Project` 路由必须严格按 `account_id` 隔离，必要时再叠加 `project_id`
- 不允许 project 路由跨账户泄漏
- 用户 BYOK 路由只在用户显式离开 `Default` 时参与选择

## 7. Tool Auto 语义

- 人格 Tools UI 支持三种状态：`Off / Auto / Manual`
- `Auto`：
  - 跟随平台默认工具策略
  - 只继承 `ready` 的工具能力
  - 未配置、未通过健康检查、运行时不可用的工具，不得暴露给模型
- `Manual`：
  - 人格显式指定工具
- `Off`：
  - 明确关闭
- 现有“继承子级”按钮如果保留，语义必须等价于上面的 `Auto`，不能只是改名

## 8. 错误契约

- 不允许把配置问题、工具问题、运行时策略问题统一压成“无权限”
- 错误至少分成以下 5 类：
  - `config_missing`
  - `config_invalid`
  - `runtime_policy_denied`
  - `external_provider_failed`
  - `internal_platform_error`
- UI 必须告诉操作者“去哪里修”，例如：
  - 模型提供商
  - 人格配置
  - 工具提供商
  - 平台设置
  - 用户 BYOK
- `tool.not_configured`、路由解析失败、BYOK 拒绝，不得再被模糊展示成普通权限错误

## 9. 禁止事项

- 禁止把 `Workspace` 作为模型/人格/工具来源层
- 禁止把 `Project` 作为模型/人格/工具来源层
- 禁止在 `Auto` 模式下向模型暴露未 ready 的工具
- 禁止在请求携带明确目标时，偷偷回退到“当前操作者默认 Project”
- 禁止出现“管理员自测正常，但新用户路径读取另一套配置源”的实现

## 10. 验收标准

- 新注册账号在没有 BYOK 的情况下，使用平台默认配置可以直接发第一条消息
- 同一平台下，管理员账号和新账号在未选择用户覆盖时，读取的是同一套平台兜底配置
- 用户选择 `Default` 后，行为稳定回到平台兜底
- 删除默认 Project 会被拒绝
- Tool `Auto` 不会暴露未 ready 工具
- Platform 路由可作为全局兜底，Project 路由不会跨账户串出

## 11. 当前工程修复应围绕的主题

- 修正配置目标解析，避免写错到“当前操作者自己的默认 Project”
- 修正新用户初始化与默认 Project 自愈
- 修正 Run 启动时的配置冻结
- 修正 Tool `Auto` 的 ready 暴露逻辑
- 修正错误映射与前端提示
