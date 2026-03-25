# OpenCLI Web Data

从 55+ 网站获取结构化数据的 CLI 工具封装。二进制已预装，无需安装步骤。

## 工具路径

```
~/.arkloop/bin/opencli-rs
```

## 基础用法

```
~/.arkloop/bin/opencli-rs <site> <command> [options] --format <json|table|yaml|csv|markdown>
```

默认输出为 table，agent 处理数据时优先使用 `--format json`。

## AI-native 发现命令

在不确定某个站点支持哪些命令时，先用这三个命令探测：

```bash
# 列出该站点所有可用命令
~/.arkloop/bin/opencli-rs explore <site>

# 为站点自动生成 adapter（站点未内置支持时使用）
~/.arkloop/bin/opencli-rs synthesize <site> <goal>

# 检测站点的认证策略（是否需要 Cookie/扩展）
~/.arkloop/bin/opencli-rs cascade <site>
```

## 常用站点命令示例

### 资讯与社区

```bash
~/.arkloop/bin/opencli-rs hackernews top --format json
~/.arkloop/bin/opencli-rs hackernews best --format json
~/.arkloop/bin/opencli-rs reddit hot --subreddit programming --format json
~/.arkloop/bin/opencli-rs reddit top --subreddit MachineLearning --format json
~/.arkloop/bin/opencli-rs devto articles --format json
~/.arkloop/bin/opencli-rs lobsters top --format json
```

### 中文平台

```bash
~/.arkloop/bin/opencli-rs bilibili trending --format json
~/.arkloop/bin/opencli-rs bilibili search --query "AI" --format json
~/.arkloop/bin/opencli-rs zhihu hot --format json
~/.arkloop/bin/opencli-rs weibo hot --format json
```

### 学术与技术

```bash
~/.arkloop/bin/opencli-rs arxiv search --query "LLM agents" --format json
~/.arkloop/bin/opencli-rs arxiv recent --category cs.AI --format json
~/.arkloop/bin/opencli-rs stackoverflow questions --tag python --format json
~/.arkloop/bin/opencli-rs stackoverflow questions --tag golang --sort votes --format json
~/.arkloop/bin/opencli-rs github trending --format json
~/.arkloop/bin/opencli-rs github trending --language rust --format json
```

### 百科与参考

```bash
~/.arkloop/bin/opencli-rs wikipedia search --query "transformer architecture" --format json
~/.arkloop/bin/opencli-rs wikipedia summary --title "Rust (programming language)" --format json
```

## 输出格式

| 格式 | 适用场景 |
|------|---------|
| `json` | agent 解析数据，默认首选 |
| `table` | 人类阅读 |
| `yaml` | 结构化配置场景 |
| `csv` | 表格数据导出 |
| `markdown` | 直接嵌入文档 |

## 需要浏览器认证的站点

以下站点需要 Chrome + opencli 扩展提供 Cookie，在无浏览器环境下不可用：

- Twitter/X
- YouTube
- TikTok
- LinkedIn
- Instagram

使用前先运行 `cascade` 命令确认认证要求：

```bash
~/.arkloop/bin/opencli-rs cascade twitter
```

## 工作流建议

1. 对陌生站点先运行 `explore <site>` 获取命令列表
2. 对需要认证的站点先运行 `cascade <site>` 确认是否可用
3. 站点完全未知时用 `synthesize <site> <goal>` 动态生成 adapter
4. 数据处理时统一使用 `--format json`
