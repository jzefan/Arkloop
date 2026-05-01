# 平台配置引导

## 配置 Web Search

Web Search 让 Agent 能够联网搜索实时信息。通过 `platform_manage` 工具的 `add_tool_provider` 动作配置。

可选 Provider：

- **Basic**：桌面端内置搜索，无需 API Key。
  调用方式：`add_tool_provider`，参数 `group: "web_search"`, `provider: "basic"`

- **Tavily**：更高质量的搜索结果，需要 API Key（从 https://tavily.com 获取）。
  调用方式：`add_tool_provider`，参数 `group: "web_search"`, `provider: "tavily"`, `config: {"api_key": "tvly-..."}`

- **SearXNG**：自托管搜索引擎，需要部署 SearXNG 实例并提供 Base URL。
  调用方式：`add_tool_provider`，参数 `group: "web_search"`, `provider: "searxng"`, `config: {"base_url": "https://your-searxng-instance"}`

## 配置 Web Fetch

Web Fetch 让 Agent 能够获取并解析网页内容。通过 `platform_manage` 的 `add_tool_provider` 配置。

可选 Provider：

- **Basic**：内置抓取，无需 API Key，内容提取质量一般。
  调用方式：`add_tool_provider`，参数 `group: "web_fetch"`, `provider: "basic"`

- **Jina**：高质量内容提取，需要 API Key（从 https://jina.ai 获取）。
  调用方式：`add_tool_provider`，参数 `group: "web_fetch"`, `provider: "jina"`, `config: {"api_key": "..."}`

- **Firecrawl**：高级网页抓取与解析，需要 API Key（从 https://firecrawl.dev 获取）。
  调用方式：`add_tool_provider`，参数 `group: "web_fetch"`, `provider: "firecrawl"`, `config: {"api_key": "..."}`

## 配置 Memory

Arkloop 的记忆系统由两个独立子系统组成，可以单独或同时启用。所有记忆绑定在 bot owner（User）上，切换 persona 不改变记忆归属。

### 子系统一：Notebook（结构化笔记）

内置的纯文本笔记系统，无需外部服务，无需配置 Provider。启用方式：

- 设置环境变量 `ARKLOOP_MEMORY_ENABLED=true`
- Agent 将获得 `notebook_read`、`notebook_write`、`notebook_edit`、`notebook_forget` 四个工具
- 数据存储在 PostgreSQL（服务端）或 SQLite（桌面端）的 `notebook_entries` 表中

### 子系统二：Memory / OpenViking（语义记忆）

基于向量检索的语义记忆，依赖外部 OpenViking 服务，支持分层内容读取（L0/L1/L2）和自动会话提炼。

启用方式：

- 设置环境变量 `ARKLOOP_MEMORY_ENABLED=true`
- 设置环境变量 `ARKLOOP_OPENVIKING_BASE_URL` 指向 OpenViking 服务地址
- 通过 `platform_manage` 配置 Provider：
  调用方式：`add_tool_provider`，参数 `group: "memory"`, `provider: "openviking"`, `config: {"base_url": "http://your-openviking-instance"}`
- Agent 将获得 `memory_search`、`memory_read`、`memory_write`、`memory_edit`、`memory_forget` 五个工具

### 双系统叠加

当两个子系统都启用时，Agent 同时拥有 Notebook 和 Memory 的全部工具，系统 prompt 中会分别注入 `<notebook>` 和 `<memory>` 块。

### 运行模式总览

| 模式 | 条件 | 注入内容 | 可用工具 |
|------|------|---------|---------|
| disabled | `ARKLOOP_MEMORY_ENABLED=false` 或未设置 | 无 | 无 |
| notebook-only | 启用，无 `ARKLOOP_OPENVIKING_BASE_URL` | `<notebook>` | notebook_read, notebook_write, notebook_edit, notebook_forget |
| memory + notebook | 启用，有 `ARKLOOP_OPENVIKING_BASE_URL` | `<notebook>` + `<memory>` 叠加 | notebook_* + memory_search, memory_read, memory_write, memory_edit, memory_forget |

## 配置 Read（文件读取增强）

Read 增强 Agent 的文件读取能力。

- **MiniMax**：增强型文件读取，需要 API Key（从 https://minimax.io 获取）。
  调用方式：`add_tool_provider`，参数 `group: "read"`, `provider: "minimax"`, `config: {"api_key": "..."}`

## 配置 LLM Provider

管理 LLM 模型提供商。

- 查看已有 Provider 和模型：使用 `list_providers` 动作
- 添加新 Provider：使用 `add_provider` 动作，参数 `name`（自定义名称）、`provider`（提供商标识）、`api_key`、`base_url`（可选）
- 更新 Provider：`update_provider`，参数 `id`（Provider UUID）、以及要修改的字段
- 删除 Provider：`delete_provider`，参数 `id`（Provider UUID）
- 查看模型列表：`list_models`，参数 `provider_id`
- 配置模型参数：`configure_model`，参数 `provider_id`、`model_id`、`config`（可选配置对象）
- 支持的 Provider 包括：openai、anthropic、deepseek、zhipu、moonshot、qwen 等

## 如何调用 platform_manage

`platform_manage` 是平台管理的核心工具，所有配置操作通过它完成。

参数说明：

- **`action`**（必填，字符串）：操作类型，可用值：
  `get_settings`、`set_setting`、`list_providers`、`add_provider`、`delete_provider`、
  `list_models`、`configure_model`、`list_agents`、`create_agent`、`update_agent`、
  `delete_agent`、`get_agent`、`list_skills`、`install_skill_market`、`install_skill_github`、
  `remove_skill`、`list_mcp_installs`、`add_mcp_install`、`update_mcp_install`、
  `delete_mcp_install`、`check_mcp_install`、`list_workspace_mcp_enablements`、
  `set_workspace_mcp_enablement`、`list_tool_providers`、`add_tool_provider`、
  `update_tool_provider`、`list_ip_rules`、`add_ip_rule`、`delete_ip_rule`、
  `list_api_keys`、`create_api_key`、`revoke_api_key`、`get_status`

- **`params`**（可选，对象）：具体操作的参数，键值对形式，不同 action 所需参数不同。

`add_tool_provider` 调用示例：
```json
{
  "action": "add_tool_provider",
  "params": {
    "group": "web_search",
    "provider": "tavily",
    "config": {
      "api_key": "tvly-..."
    }
  }
}
```
