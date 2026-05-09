# 本地模式登录与 Setup 分流修复设计

**目标**：修复 Arkloop Web 在本地桌面模式下将“未登录/会话失效”错误地路由到 setup 页的问题，让首次进入走 setup，凭证失效走普通登录。

## 背景

当前本地桌面模式下，`/login` 会根据 `shouldUseLocalSetupRoute(localMode, accessToken)` 决定渲染普通登录页还是 `HeadlessSetupPage`。

现状实现：

- `src/apps/web/src/appAuthStartup.ts`
  - `shouldUseLocalSetupRoute(localMode, accessToken)` 当前仅基于 `localMode && !accessToken` 判断。
- `src/apps/web/src/App.tsx`
  - `/login` 在本地模式且无 `accessToken` 时，会直接渲染 `HeadlessSetupPage`。
- `src/apps/web/src/components/HeadlessSetupPage.tsx`
  - 该页面调用 `setLocalOwnerPassword`，对应后端 `POST /v1/auth/local-owner-password`。

进一步确认后端语义可知：

- `local-owner-password` 并不是普通登录接口，而是为固定桌面用户设置或重置用户名/密码凭证。
- 本地模式还存在 `POST /v1/auth/local-session`，可以在桌面可信上下文中直接恢复本地会话。

因此，当前“没有 accessToken 就进入 setup 页”的前端逻辑，把以下两种状态混淆了：

1. 首次进入，需要 setup 本地 owner 凭证
2. 已 setup 过，但当前只是 access token 失效，需要重新登录或恢复会话

## 问题定义

当前错误行为：

- 用户曾经已经完成本地 owner 凭证设置
- 当前 access token 失效或不存在
- 访问 `/login` 时仍被带到 setup 页

这会造成两个问题：

1. 用户认知错误：setup 页被误认为普通登录页
2. 行为风险：再次提交 setup 会覆盖本地 owner 的用户名/密码，而不是执行登录

## 设计原则

1. `setup` 与 `login` 语义必须严格分离。
2. “没有 access token”只表示“当前未登录”，不能推导出“需要 setup”。
3. 保持最小改动：本次只修前端入口判定与路由分流，不扩展后端状态接口。
4. 不改变现有 `HeadlessSetupPage` 的提交行为，只改变它被使用的入口条件。

## 方案

### 方案选择

采用“拆分本地 setup 与本地登录入口”的方案：

- `/setup`：显式 setup 入口，继续渲染 `HeadlessSetupPage`
- `/login`：登录入口，未登录时始终渲染普通 `AuthPage`
- 自动 `local-session` 恢复成功时仍直接进入应用
- 自动 `local-session` 恢复失败时落到普通登录页，而不是 setup 页

### 具体实现

#### 1. 调整本地 setup 路由判定语义

修改 `src/apps/web/src/appAuthStartup.ts`：

- 不再让 `shouldUseLocalSetupRoute` 仅凭 `localMode && !accessToken` 返回 `true`
- 将 setup 判定收紧到“显式 setup 入口”语义

本次最小实现建议：

- `/setup` 作为唯一 setup 页面入口
- `/login` 不再动态切换成 `HeadlessSetupPage`

这样 setup 判定不再依赖“当前是否持有 access token”。

#### 2. 收紧 `/login` 的职责

修改 `src/apps/web/src/App.tsx`：

- 本地桌面模式下，`/login` 始终显示 `AuthPage`
- 移除当前 `/login` 基于 `useLocalSetupRoute` 动态切换到 `HeadlessSetupPage` 的行为

#### 3. 保留 `/setup` 的显式 setup 入口

继续保留 `src/apps/web/src/App.tsx` 中：

- `/setup` => `HeadlessSetupPage`

`HeadlessSetupPage` 继续承担：

- 首次本地 owner 凭证设置
- 明确 setup 入口下的本地凭证初始化

#### 4. 保持自动本地会话恢复行为不变

本次不修改 `local-session` 的恢复流程本身。

目标仅是确保：

- 恢复成功 => 正常进应用
- 恢复失败 => 进入普通登录页
- 不再因为恢复失败而显示 setup 页

## 非目标

本次不包含：

- 新增后端“是否已完成 owner setup”状态接口
- 修改 `local-owner-password` 的后端语义
- 新增“重置本地凭证”专用页面
- 调整 setup 页提交逻辑
- 重构整套桌面认证体系

## 风险

1. 某些首次进入场景如果实际上走的是 `/login`，改完后会看到普通登录页，而不是 setup 页。
2. 如果桌面端尚未稳定区分“首次 setup 入口”和“普通登录入口”，本次改动会把这个入口设计问题暴露出来。

这两个风险是可接受的，因为它们比“把普通登录错误导向 setup 并可能覆盖凭证”更安全。

## 测试设计

至少补充以下测试：

1. `src/apps/web/src/__tests__/appAuthStartup.test.ts`
   - 更新当前依赖 `!accessToken` 决定 setup 的旧断言
2. `src/apps/web/src/App.tsx` 相关路由测试
   - 本地模式 + 无 token + `/login` => `AuthPage`
   - `/setup` => `HeadlessSetupPage`
3. 保留或补充现有本地 session 启动测试
   - 自动恢复成功时仍进入应用，不回归

## 验收标准

1. 本地桌面模式下，`/login` 在无 `accessToken` 时显示普通登录页，而不是“设置远程登录账号”页。
2. `/setup` 仍然显示 setup 页，并保持现有 `setLocalOwnerPassword` 行为。
3. 自动 `local-session` 恢复成功时，仍然直接进入应用。
4. 自动 `local-session` 恢复失败时，落到普通登录页，而不是 setup 页。

## 备注

本次修复的核心不是修改 setup 功能本身，而是修正“setup 与 login 被错误复用”的入口分流。后续如果需要更完整的首次进入体验，可以在此基础上再引入明确的“是否已完成本地 owner 初始化”状态判断。