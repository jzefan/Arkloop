# 产教融合评估搜索超时修复设计

**目标**：降低“双高产教融合评估”在 `web_search.basic` 下因搜索阶段过于激进而超时失败的概率，并在超时发生时输出可诊断、可操作的错误提示。

## 背景

当前 `src/personas/industry-education-index/agent.lua` 在搜索阶段会一次性向 `web_search` 发出 5 条查询，用于收集学校官网、院校类型和产教融合公开资料。

运行时调查确认：

- 当前桌面实例激活的搜索 provider 是 `web_search.basic`
- `web_search.basic` 并不直接访问公网搜索服务，而是依赖本地桌面 browser-search 端点
- `web_search` 工具默认超时为 10 秒
- `industry-education-index` persona 本身没有单独设置 `tool_timeout_ms`

因此当前问题的典型表现是：

1. `web_search.basic` 依赖的本地桌面搜索代理响应慢或不可用
2. persona 同时发出 5 条查询，增加了全部查询在工具超时窗口内失败的概率
3. 搜索阶段直接终止，评估流程无法进入院校资格核验和评分

## 问题定义

当前用户看到的错误虽然提示了 Tavily / SearXNG，但缺少足够运行态信息，无法快速判断：

- 当前实际使用的是哪个 provider
- 当前失败是否与 `web_search.basic` 的本地代理依赖有关
- 超时是不是来自工具层窗口，而不是模型评估本身

这导致用户容易误判问题在模型、提示词或评分逻辑，而实际根因是搜索 provider 路径不稳定。

## 设计原则

1. 优先降低失败概率，而不是只润色报错文案。
2. 保持最小改动：仅修改 `industry-education-index/agent.lua` 与对应测试，不改 `web_search` 工具实现。
3. 对 `web_search.basic` 做更明确的诊断提示，但不把其他 provider 错误地描述为本地代理问题。
4. 不扩大到工具层重构或 provider 默认值调整。

## 方案

### 方案选择

采用“减少 `basic` 压力 + 提升超时可诊断性”的组合方案：

- 将搜索 query 数量从 5 条收缩到 3 条
- 保留最关键的院校识别与公开资料搜索意图
- 在搜索失败时根据当前 active provider 输出更有针对性的诊断提示

### 具体实现

#### 1. 收缩搜索 query 数量

修改 `src/personas/industry-education-index/agent.lua` 中 `search_sources` 的查询构造逻辑。

当前 5 条 query：

1. `school_name`
2. `school_name 官网`
3. `school_name 学校官网 高职 院校 简介`
4. `school_name 双高计划 高水平高职学校 专业群`
5. `school_name 产教融合 校企合作 实训基地 {year}`

本次改为 3 条：

1. `school_name`
2. `school_name 官网`
3. `school_name 高职 双高 产教融合 校企合作 {year}`

如果 `analysis_focus` 非空，不再额外追加新 query，而是合并进第 3 条，避免查询数再次膨胀。

#### 2. 在 persona 层读取当前 active provider 名称

在 `industry-education-index/agent.lua` 中增加一个轻量 helper，用于从 runtime/context 中拿到当前 `web_search` active provider 名称；拿不到时降级为 `web_search`。

目标是让 persona 错误提示可以区分：

- `web_search.basic`
- `web_search.tavily`
- `web_search.searxng`
- 未知 provider

#### 3. 改进搜索失败提示

当前错误提示为：

- “当前搜索提供商未返回可用于核验院校身份的公开结果。请检查‘接入’中的联网搜索配置，建议切换到 Tavily 或 SearXNG 后重试。原始信息：...”

本次改成结构化、更有诊断性的提示，至少包含：

- 当前联网搜索 provider 名称
- 搜索在工具超时窗口内未完成（若原始错误为 timeout）
- 当 provider 为 `web_search.basic` 时，额外说明其依赖本地桌面 browser-search 端点
- 建议切换到 Tavily / SearXNG
- 保留原始错误字符串

#### 4. 不改 `web_search` 工具默认 timeout

本次不调整：

- `web_search` 默认 10 秒 timeout
- tool runtime 的 provider 加载策略
- platform / profile 的 `tool_timeout_ms` 规则

这些属于更大范围的工具层调优，不纳入本次 persona 定向修复。

## 非目标

本次不包含：

- 修改 `web_search` executor 默认超时值
- 调整 `web_search.basic` 的桌面 endpoint 实现
- 自动切换 provider 或多 provider fallback
- 在工具层增加 provider 诊断字段
- 重构整套搜索 provider 配置系统

## 风险

1. query 数减少后，某些边缘学校名称的召回率可能略有下降。
2. 仅在 persona 层拼接 provider 诊断信息，依赖当前 runtime/context 能拿到 active provider 名称；若拿不到，只能退化显示通用 provider 名称。
3. 这次降低了 `basic` 场景下的超时概率，但不能彻底消除 provider 本身不可用带来的失败。

这些风险可接受，因为当前主要问题是用户几乎无法从错误中判断真实根因。

## 测试设计

至少补充以下测试：

1. 在 `src/services/worker/internal/executor/lua_test.go` 验证搜索 query 数量已由 5 条改为 3 条。
2. 在 `lua_test.go` 中构造 `web_search` timeout，断言最终错误包含：
   - provider 名称
   - timeout 说明
   - Tavily / SearXNG 建议
3. 当 provider 为 `web_search.basic` 时，断言错误提示额外包含“依赖本地桌面 browser-search 端点”的说明。
4. 保留现有成功路径测试，确保搜索成功时仍能进入后续评估流程。

## 验收标准

1. `industry-education-index` 的搜索 query 数量从 5 条降为 3 条。
2. 搜索 provider 超时时，错误提示必须包含当前 provider 名称。
3. 当 provider 为 `web_search.basic` 时，错误提示必须明确指出其依赖本地桌面 browser-search 端点。
4. 错误提示继续给出 Tavily / SearXNG 的切换建议。
5. 搜索成功路径不回归，现有行业教育评估主流程测试继续通过。

## 备注

本次修复针对的是 persona 层“如何使用搜索工具”和“如何向用户解释搜索失败”，不是对搜索工具实现本身做平台级稳定性改造。后续若仍频繁出现 timeout，再考虑工具层 timeout、provider fallback 或 provider 运行态可观测性增强。