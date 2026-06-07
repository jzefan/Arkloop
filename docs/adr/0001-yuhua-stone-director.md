# ADR 0001: Yuhua-Stone-Director — 中国奇石电影分镜智能体

| 字段 | 值 |
|---|---|
| 日期 | 2026-05-26 |
| 状态 | Accepted |
| 关联设计文档 | [docs/agent_design/yuhua-stone-director-design.md](../agent_design/yuhua-stone-director-design.md) |
| 范围 | 新增 5 个 persona（1 主编排 + 4 子智能体）；扩展 `image_generate` 工具；修复 lua executor 数据字段路径 |

---

## 背景

为「上传一张图 → 产出一支可拍摄的中国奇石电影分镜与视频生成 Prompt 包」这一产品需求，引入一组协作智能体。难点不是故事生成本身，而是**16 张分镜图的角色一致性 + 镜头连续性**——靠纯 prompt 复述外貌一定会漂。

## 决策

### 1. 用「Lua 编排 + 4 个 simple 子 persona」架构

- 主 `yuhua-stone-director` 采用 `executor_type: agent.lua`，显式编排 VL / Story / Bible / Storyboard 四个 LLM 阶段 + 17 次 `image_generate` 调用
- 子 persona 全部采用 `executor_type: agent.simple`，省去 Lua wrapper 层；每个子 persona 的 system prompt 是该阶段的契约（输入 schema + 输出 schema）
- 阶段间状态用强约束 JSON 传递（Character Bible / Storyboard），杜绝下游自由复述

### 2. 角色一致性靠「双锚 + Verbatim signature」

- **Anchor 1（视觉）**：Step 5 单独生成 Character Reference Sheet（CRS），后续 16 张分镜的 `input_images[0]` 永远是 CRS
- **Anchor 2（视觉延续）**：Shot 2..N 的 `input_images[1]` 是上一镜，提供局部场景延续
- **Verbatim 注入**：Character Bible 含 `appearance_signature` / `clothing_signature` 两段「必须原样引用」的英文段，主 Lua 在每次 `image_generate` 调用前**字符串拼接**到 prompt 末尾，不让 LLM 在每次调用里自己复述外貌

### 3. 视频生成 v1 不做，只输出 Prompt

v1 终点 = Storyboard 静帧包 + 视频就绪 Prompt 包。这把「产品身份的稳定性」70% 都锁定在静帧阶段；视频环节作为 commodity 步骤留给用户在可灵/Seedance 平台手动跑。v2 时再接 API。

---

## 同时修复的两个上游问题

实施过程中发现两个已存在的 bug，一并修复：

### Bug A：`image_generate` 同 run_id 多次调用 artifact key 冲突

**症状**：同一 run 内每次调用都用 `<account>/<run_id>/generated-image.png` 作为 artifact key，第 17 次调用会覆盖第 1..16 次。
**影响**：yuhua-stone-director 一次跑 17 次（CRS + 16 shots），全部相互覆盖。
**修复**：在 `image_generate` spec 中新增可选 `artifact_name` 参数，加 `sanitizeArtifactName` 保证路径安全；主 lua 用 `character_reference_sheet` / `shot_01` ... `shot_16` 命名。改动向后兼容（未传 `artifact_name` 时退回原默认 `generated-image`）。
**测试**：新增 `TestToolExecutorExecuteHonorsArtifactName` + `TestToolExecutorExecuteSanitizesArtifactName`。

### Bug B：`agent.lua` executor_config 数据字段未注入 Lua runtime

**症状**：`persona.yaml` 中 `executor_config.school_names_file` 会被 loader 读成 `executor_config.school_names`，但生产代码路径中**没有任何位置**把这些字段拷贝到 `rc.InputJSON`。因此 `context.get("school_names")` 在生产里永远是 `nil`，`industry-education-index` 的相关功能形同虚设（lua 端的 fallback 让流程没崩，但匹配能力丢失）。
**影响**：yuhua-stone-director 需要通过 `context.get("style_preset")` 读取风格预设；如果这个路径不修，本智能体启动后预设永远拿不到、退化到 lua 内嵌的最小默认。
**修复**：`NewLuaExecutor` 保存 `script` 以外的 `extras` 字段，`Execute` 时合并到 `rc.InputJSON`（运行时输入优先，已存在的键不覆盖）。
**副作用**：`industry-education-index` 的 `school_names` 现在生产中也真的能用上——这是 persona 作者一直期望的行为（参考 `TestIndustryEducationIndexAgentLuaMatchesSchoolNameFromGeneralCatalog`），不是行为回归。

### 工程基础设施扩展

在 `loadExecutorDataFile` 的硬编码白名单里加 `style_preset_file → style_preset` 映射，沿用现有「`<X>_file` 字段自动读文件并填入 `<X>` 字段」的通用机制。

---

## 模型 selector 与凭据要求

```
local STAGE_MODELS = {
  vl         = "qwen3.6-plus",       -- 自有阿里凭据
  story      = "deepseek-v4-flash",  -- 自有 DeepSeek 凭据；生产可升档到 deepseek-v4-pro
  bible      = "deepseek-v4-flash",
  storyboard = "deepseek-v4-flash",
}
local IMAGE_MODEL = "openai/gpt-5-image-mini"  -- 必需走 OpenRouter
```

**部署前置条件**：ArkLoop 必须配好一条 `provider_kind=openai, base_url=https://openrouter.ai/api/v1` 的 OpenRouter 凭据，并配一条 `model=openai/gpt-5-image-mini` 的 image route（`AdvancedJSON.available_catalog.output_modalities=["image"]`）。

---

## 调试支持

`worker` 容器侧设环境变量即可降到 4 镜头快速跑通流水线：

```bash
ARKLOOP_YUHUA_SHOT_COUNT_DEBUG=4
```

主 lua 顶部解析此变量并 clamp 到 [4, 32]。

---

## 不采纳

| 方案 | 不采纳原因 |
|---|---|
| 单一 `agent.simple` persona 端到端 | 长上下文里关键状态（外貌、服饰）会漂移；缺少阶段重试粒度 |
| 全部子 persona 用 `agent.lua` + `stream_agent` | `parseAgentMessages` 不支持 image content part；VL 阶段拿不到图片附件 |
| 多预设风格 enum | 与「产品级品控」目标冲突；用户选错预设反砸口碑 |
| 主+降级双图像模型 | mini 失败大概率重模型也失败，浪费 quota；只重试同模型 |
| 镜数 ask_user 表单 | 90% 用户走默认 16；文本意图解析（"做一个 8 镜头版本"）更顺滑 |
| 视频集成（v1） | 新增 provider+工具+计费 ≥ 1 周；静帧已锁定 70% 一致性 |
