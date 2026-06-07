# 雨花石导演智能体 设计文档

| 字段 | 值 |
|---|---|
| 文档版本 | v1.0 (draft) |
| 创建日期 | 2026-05-26 |
| 状态 | 待实现 |
| 范围 | 新增一组 5 个 persona（1 主编排 + 4 子智能体）+ 改造 `image_generate` 工具支持 `model_selector` 参数；输出 `Image → Story → Storyboard → 视频就绪 Prompt 包` 全流程 |
| 非目标 | v1 不集成视频生成（可灵 / Seedance），不实现"多预设风格选择"，不接 memory 系统，不做多镜头平台 channel 适配（仅 Web 客户端）|

---

## 1. 目标与定位

### 1.1 业务目标

> **核心承诺**：用户上传一张图片，系统稳定产出"统一角色、统一美术风格、镜头连续、具有中国奇石电影气质"的高质量分镜素材包，用户可一键拷贝到可灵 / Seedance 等 image-to-video 平台跑出视频。

1. 产品定位不是"AI 视频生成器"，而是"**东方电影导演智能体**"——核心竞争力是**稳定输出有东方文化气质的电影语言**
2. v1 交付物 = `final_report.md` + 17 张图片 artifact（CRS×1 + Shots×16）+ 2 份 JSON artifact（`character_bible.json` / `storyboard.json`）
3. v2 范围：接入可灵（首选）/ Seedance（备选）API，把分镜静帧直接驱动成视频片段

### 1.2 设计原则

- **状态显式传递优于上下文记忆**：通过 Lua 编排 + 子 persona 隔离上下文，关键状态（Character Bible / Storyboard）以 JSON schema 强约束传递，杜绝 LLM 自由复述导致的角色漂移
- **CRS 锚点优于纯 prompt 一致性**：用一张 Character Reference Sheet 作为 16 镜头共同视觉锚点，切断误差累计
- **产品身份硬约束**：主角永远是东方文人/匠人，奇石永远是叙事锚点；不通用化、不开放化
- **最小改动**：仅新增 persona 文件 + `image_generate` 加一个可选参数，不引入新工具、新 provider kind、新 channel

---

## 2. 总体架构

```
用户消息（图片 + 可选文本）
        │
        ▼
┌──────────────────────────────────────────────────────────────────────┐
│  yuhua-stone-director (agent.lua 主编排, user_selectable=true)         │
│  ─────────────────────────────────────────────────────────────────── │
│  Step 0  启动校验：图必填 / channel 校验 / OpenRouter 凭据预检         │
│  Step 1  解析 shot_count（文本意图 / env 调试覆盖 / 默认 16）          │
│  Step 2  spawn yuhua-stone-vl                                          │
│             └─ qwen3.6-plus，输入图+title_hint                         │
│             └─ 输出 Markdown（含 建议标题 / 选定情绪 / 选定音乐 ...）   │
│  Step 3  spawn yuhua-stone-storywriter                                 │
│             └─ deepseek-v4-flash                                       │
│             └─ 输出长故事 Markdown + 末尾 16 段 JSON                   │
│  Step 4  spawn yuhua-stone-bible                                       │
│             └─ deepseek-v4-flash                                       │
│             └─ 输出严格 JSON（含 appearance/clothing_signature）       │
│  Step 5  spawn yuhua-stone-storyboard                                  │
│             └─ deepseek-v4-flash                                       │
│             └─ 输出严格 JSON 数组（含 continuity_from_prev）           │
│  Step 6  tools.call image_generate（CRS）                              │
│             └─ openai/gpt-5-image-mini，单张正面立绘                   │
│  Step 7  for shot in 1..N:                                             │
│             tools.call image_generate(input_images=[CRS, prev_shot?])  │
│  Step 8  tools.call document_write → final_report.md                   │
└──────────────────────────────────────────────────────────────────────┘
        │
        ▼
final_report.md（嵌入所有图片 + 视频 Prompt）+ 独立 artifact 集合
```

---

## 3. 文件布局

```
src/personas/
├── yuhua-stone-director/         # 主编排，user_selectable=true
│   ├── persona.yaml
│   ├── prompt.md                 # 用户面说明 + 系统级行为约束
│   ├── agent.lua                 # 主编排逻辑（~250 行）
│   └── style_preset.json         # 单一风格预设 chinese-stone-cinema-v1
├── yuhua-stone-vl/               # 子：图像理解 + 故事概要 + 选 mood/music
│   ├── persona.yaml              # user_selectable=false, is_system=true
│   ├── prompt.md
│   └── agent.lua                 # ~15 行 stream_agent wrapper
├── yuhua-stone-storywriter/      # 子：长故事 + 16 段拆分
│   ├── persona.yaml
│   ├── prompt.md
│   └── agent.lua
├── yuhua-stone-bible/            # 子：Character Bible JSON
│   ├── persona.yaml
│   ├── prompt.md
│   └── agent.lua
└── yuhua-stone-storyboard/       # 子：N 镜头 storyboard JSON
    ├── persona.yaml
    ├── prompt.md
    └── agent.lua
```

> **目录组织约定**：沿用 `industry-education-*` 既有命名约定（顶层平铺 + 前缀分组）。不引入嵌套子目录，避免改动 `src/services/worker/internal/personas/loader.go` / `watcher.go` / `src/services/api/internal/personasync/` 三处的目录扫描逻辑。

---

## 4. 阶段间状态契约

### 4.1 输入/输出形态总览

| 阶段 | 输入 | 输出形态 | 模型 selector |
|---|---|---|---|
| VL | image attachment + (optional) title_hint | **Markdown**，固定 6 个 H1 段 | `qwen3.6-plus` |
| Story Writer | VL 输出 | **Markdown + 末尾 JSON code block** | `deepseek-v4-flash` |
| Bible Builder | VL + Story + style_preset | **纯 JSON**（含原样段） | `deepseek-v4-flash` |
| Storyboard | VL + Story + Bible + style_preset + shot_count | **纯 JSON 数组**（含 continuity 字段） | `deepseek-v4-flash` |
| CRS gen | Bible signatures + style suffix | 单张图 artifact | `openai/gpt-5-image-mini` |
| Per-shot loop | shot prompt + `[crs, prev?]` | N 张图 artifact | `openai/gpt-5-image-mini` |
| Final report | 全部 artifact + Bible + Storyboard | `final_report.md` artifact | document_write |

> **生产环境模型升档**：`deepseek-v4-flash` → `deepseek-v4-pro`（仅改 `STAGE_MODELS` 常量表）

### 4.2 VL 子 persona 输出 Markdown 规范

```markdown
# 建议标题
雨花石中的山月

# 选定情绪
zen-quiet

# 选定音乐
guqin-solo

# 图片理解
...

# 情绪氛围
...

# 故事概要
...
```

主 Lua 解析规则：
- `建议标题` 段：若用户消息提供了 `title_hint`，则忽略此段；否则取此段作为最终标题
- `选定情绪` / `选定音乐`：必须在 `style_preset.json` 的 palette ID 列表内；不在则 fallback 到该 palette 的第一个

### 4.3 Character Bible JSON Schema

```json
{
  "schema_version": "1",
  "character": {
    "name": "沈砚秋",
    "identity": "雨花石鉴赏专家",
    "age": "45",
    "gender": "male",
    "appearance_signature": "ONE PARAGRAPH ~80字，下游所有 image prompt 原样引用，禁止改写",
    "appearance_detail": {
      "face": "...",
      "hair": "...",
      "eyes": "...",
      "body": "..."
    },
    "clothing_signature": "ONE PARAGRAPH ~50字，下游所有 image prompt 原样引用",
    "clothing_detail": {
      "main_outfit": "...",
      "material": "...",
      "accessories": ["..."]
    },
    "personality": ["..."]
  },
  "cinematic_style": {
    "preset_id": "chinese-stone-cinema-v1",
    "mood_id": "zen-quiet",
    "lighting": "...",
    "color_palette": ["..."],
    "camera_style": "...",
    "mood": "..."
  },
  "music_style": {
    "preset_id": "guqin-solo",
    "tempo": "slow",
    "keywords": ["..."]
  },
  "voice_style": {
    "tone": "...",
    "speed": "...",
    "language_style": "..."
  },
  "negative_prompts": [
    "no costume change",
    "no facial variation",
    "no age change",
    "no hairstyle change"
  ]
}
```

> **关键设计**：`appearance_signature` / `clothing_signature` 是两段"必须原样引用"的冗余段。下游每次调 `image_generate` 时，由主 Lua 字符串注入，**不让 LLM 在每次调用里自己复述外貌**——这是角色一致性的核心工程锚点。

### 4.4 Storyboard JSON Schema

```json
{
  "schema_version": "1",
  "shots": [
    {
      "shot_id": 1,
      "duration_seconds": 8,
      "shot_type": "wide|medium|close-up|extreme-close-up|over-shoulder|...",
      "camera_motion": "static|slow push in|pull back|pan left|...",
      "scene": {
        "setting": "interior|exterior|...",
        "location": "茶室 / 院落 / 山径 / ...",
        "time_of_day": "dawn|day|dusk|night",
        "background_elements": ["..."]
      },
      "character_action": "短句，主语必须是 Bible.character.name",
      "character_pose": "站立|跪坐|侧卧|...",
      "emotion": "...",
      "lighting": "...",
      "key_props": ["..."],
      "voice_over": "10-25 字中文（3/4 现代白话 + 1/4 古典点缀）",
      "subtitle": "10-25 字中文",
      "continuity_from_prev": "与上一镜的连续性约束，主 Lua 注入到下游 image prompt"
    }
  ]
}
```

---

## 5. 关键工程机制

### 5.1 Character Lock

每次 `image_generate` 调用时，主 Lua 拼接 prompt：

```
<style_preset.image_prompt_suffix_template (rendered)>

Character (verbatim from bible, do not paraphrase):
{bible.character.appearance_signature}
{bible.character.clothing_signature}

Scene:
{shot.scene.setting} | {shot.scene.location} | {shot.scene.time_of_day}
{shot.scene.background_elements joined}

Action:
{bible.character.name} {shot.character_action}, {shot.character_pose}

Camera: {shot.shot_type}, {shot.camera_motion}
Lighting: {shot.lighting}
Mood: {shot.emotion}
Key props: {shot.key_props joined}

Continuity from previous shot:
{shot.continuity_from_prev}

Negative prompts:
{style_preset.negative_prompts_global ∪ bible.negative_prompts (deduplicated, comma-joined)}
```

**严禁让 LLM 改写 `appearance_signature` / `clothing_signature`**——这两段是定值字符串注入。

### 5.2 镜头连续性（CRS 锚定）

- **Step 6 生成 CRS**：单张正面立绘，`input_images=[]`（仅靠 signature 段 + style preset 驱动）
- **Shot 1**：`input_images=[CRS]`
- **Shot 2..N**：`input_images=[CRS, prev_shot]`
  - CRS 永远在 `[0]` 槽位 → 切断误差累计
  - prev_shot 在 `[1]` 槽位 → 提供局部场景延续
- **用户上传的原图不进入 `input_images`**：避免石头纹理污染人物 latent

### 5.3 Mood / Music 选择

- 由 **VL 阶段统一决策**（唯一同时看到图和标题的阶段）
- 下游 Bible / Storyboard / image gen / 报告全部消费同一个 mood/music
- **允许在 mood 主线内 1-2 镜短反差**（zen-quiet 中第 8-9 镜可短暂出现明亮调子）

### 5.4 奇石叙事约束

写入 VL 子 persona system prompt：

```
你必须围绕一位东方文人/匠人主角展开故事，
故事的核心物件、情感寄托或情节转折必须围绕"奇石"
（雨花石、灵璧石、太湖石、戈壁石等任一种）。
- 若输入图本身就是石头特写，主角是来鉴赏/把玩/思索这块石的人。
- 若输入图是其他事物（茶具、山水、人物），故事中必须存在一块奇石作为情感锚点。
```

### 5.5 Voice-over 风格规则

- 3/4 现代白话诗 + 1/4 古典诗词点缀（《诗经》/ 唐诗 / 宋词）
- few-shot 示例（写入 Storyboard 子 persona prompt）：
  - "石头沉默了三千年，他第一次听见。" — 现代白话
  - "山色空濛，他看见的是另一片心海。" — 现代白话+意象
  - "如李白所言，举杯邀明月——他对这块石头举起了茶杯。" — 显式古典点缀
  - "宋人云：'物非物，心非心。'此刻他懂了。" — 引用化用

### 5.6 shot_count 解析

```lua
local function parse_shot_count(user_text)
  local n
  -- 文本意图解析
  local m = user_text and user_text:match("(%d+)%s*[镜段]") or user_text and user_text:match("(%d+)%s*shots?")
  if m then n = tonumber(m) end
  -- 环境变量覆盖（开发用）
  local debug_count = os.getenv("ARKLOOP_YUHUA_SHOT_COUNT_DEBUG")
  if debug_count then n = tonumber(debug_count) end
  -- clamp + 默认
  if not n or n < 4 then n = 16 end
  if n > 32 then n = 32 end
  return n
end
```

### 5.7 失败处理

- **单镜 image gen** 失败：同模型限流/超时重试 2 次（退避 1s → 3s）→ 仍失败则写 placeholder artifact，记入"失败镜头清单"，继续后续 shot
- **任一 LLM 阶段** 失败：整轮终止，set_output 提示用户重试
- **OpenRouter 凭据未配置**：启动时通过 `image_generate.IsAvailableForModelSelector(ctx, "openai/gpt-5-image-mini")` 预检，未通过直接拒跑

### 5.8 启动校验

| 校验项 | 不满足时行为 |
|---|---|
| 用户消息含图片附件 | set_output 提示"需要先上传一张图片" |
| `channel_type ∉ {telegram, feishu, qq, discord, weixin, onebot}` | set_output 提示"本智能体仅适配 Web 客户端" |
| `openai/gpt-5-image-mini` 路由可达 | set_output 提示部署方"请配置 OpenRouter 凭据" |
| 多张图片附件 | 仅取第一张，set_output 提示"已忽略其余图片"，继续 |
| 零文本 | 允许，VL 自己产标题 |

---

## 6. 风格预设 `style_preset.json`

单一预设 ID = `chinese-stone-cinema-v1`，硬编码于 `yuhua-stone-director/style_preset.json`，用户不可改：

```json
{
  "preset_id": "chinese-stone-cinema-v1",
  "version": "1",
  "display_name": "中国奇石电影",
  "core_subject": "奇石鉴赏 / appreciation of exotic stones",

  "global_style_tokens": [
    "Chinese cinema", "Chinese aesthetics", "slow cinematic motion",
    "natural lighting", "85mm cinema lens", "shallow depth of field", "film grain"
  ],

  "mood_palette": [
    { "id": "zen-quiet",          "name": "禅意静谧",
      "lighting": "晨雾微光 / 北窗柔光 / 日落金光", "pacing": "缓慢、留白多" },
    { "id": "bright-cheerful",    "name": "明亮欢快",
      "lighting": "正午自然光、高对比、明亮饱和", "pacing": "中速、轻快切换" },
    { "id": "elegant-warm",       "name": "雅致温暖",
      "lighting": "黄昏暖光、室内柔光",          "pacing": "慢速、对称构图" },
    { "id": "contemplative-cool", "name": "沉思清冷",
      "lighting": "雨天、青灰色、北光",          "pacing": "慢速、长定格" }
  ],

  "music_style_palette": [
    { "id": "guqin-solo",       "name": "古琴独奏",
      "mood_fit": ["zen-quiet", "contemplative-cool"],
      "tempo": "slow",         "keywords": ["古琴", "《平沙落雁》风格"] },
    { "id": "dizi-flute",       "name": "笛箫",
      "mood_fit": ["zen-quiet", "elegant-warm", "bright-cheerful"],
      "tempo": "medium",       "keywords": ["竹笛", "民乐"] },
    { "id": "guzheng-bright",   "name": "古筝明朗",
      "mood_fit": ["bright-cheerful", "elegant-warm"],
      "tempo": "medium-fast",  "keywords": ["古筝", "《渔舟唱晚》风格"] },
    { "id": "modern-guofeng",   "name": "现代国风器乐",
      "mood_fit": ["bright-cheerful", "elegant-warm"],
      "tempo": "medium",       "keywords": ["国风器乐", "民乐+电子"] },
    { "id": "ambient-pad",      "name": "氛围声学",
      "mood_fit": ["zen-quiet", "contemplative-cool"],
      "tempo": "very-slow",    "keywords": ["雨声", "钟磬", "ambient drone"] }
  ],

  "negative_prompts_global": [
    "no anime style", "no cartoon style", "no neon colors",
    "no Western fantasy elements", "no modern technology unless required by scene",
    "no exaggerated animation"
  ],

  "image_prompt_suffix_template":
    "Style: {global_style_tokens, comma-joined}. Mood: {selected_mood.name}; {selected_mood.lighting}. Camera: 85mm cinema lens, shallow depth of field, film grain.",
  "video_prompt_suffix_template":
    "Cinematic motion matching {selected_mood.pacing}. Subtle realistic micro-movement. No character deformation. Maintain exact character identity.",

  "voice_style_default": {
    "tone": "温和真诚",
    "speed": "中等偏慢",
    "language_style": "以现代白话诗为主（约 3/4），可点缀古诗词、宋词、《诗经》、唐诗（约 1/4），切忌通篇文言"
  }
}
```

---

## 7. Persona YAML 草案

### 7.1 `yuhua-stone-director/persona.yaml`

```yaml
id: yuhua-stone-director
version: "1"
title: 雨花石导演
description: 上传一张图，生成 16 镜头中国奇石电影分镜与视频生成 Prompt 包
soul_file: prompt.md
user_selectable: true
selector_name: 雨花石导演
selector_order: 8
is_builtin: true
core_tools:
  - image_generate
  - document_write
tool_allowlist:
  - image_generate
  - document_write
budgets:
  reasoning_iterations: 200
  temperature: 0.7
  max_output_tokens: 8192
reasoning_mode: auto
prompt_cache_control: system_prompt
executor_type: agent.lua
executor_config:
  script_file: agent.lua
  style_preset_file: style_preset.json
```

> **`executor_config.style_preset_file` 字段验证待办**：实现期需确认 `loader.go` 对未知字段的处理。若严格拒绝，则改为主 lua 直接 `io.open("./style_preset.json")` 读相对路径，移除该字段。

### 7.2 子 persona（vl / storywriter / bible / storyboard）

通用模板：

```yaml
id: yuhua-stone-{stage}
version: "1"
title: 雨花石导演-{阶段中文}
soul_file: prompt.md
user_selectable: false
settings_visible: false
is_system: true
is_builtin: true
core_tools: []
tool_allowlist: []
budgets:
  reasoning_iterations: 50
  temperature: 0.7
  max_output_tokens: 4096
reasoning_mode: auto
executor_type: agent.lua
executor_config:
  script_file: agent.lua
```

子 persona 的 `agent.lua` 模板（共用）：

```lua
local input = context.get("input") or {}
local system_prompt = context.get("system_prompt")
local selector = input.model_selector or context.get("default_model_selector")
local user_text = input.user_text or ""
local messages = input.messages or { { role = "user", content = user_text } }

local result = agent.stream_agent(selector, system_prompt, messages, {
  temperature = input.temperature,
  max_tokens = input.max_tokens,
})

set_output(result.content)
```

---

## 8. 模型 selector（主 Lua 顶部常量）

```lua
local STAGE_MODELS = {
  vl         = "qwen3.6-plus",       -- 视觉，自有凭据
  story      = "deepseek-v4-flash",  -- 自有凭据，生产可改 deepseek-v4-pro
  bible      = "deepseek-v4-flash",
  storyboard = "deepseek-v4-flash",
}
local IMAGE_MODEL = "openai/gpt-5-image-mini"  -- 唯一走 OpenRouter
```

> **selector 字符串格式确认待办**：实现期需根据账号实际 route 配置确认是否带 `/` 前缀（`qwen3.6-plus` vs `qwen/qwen3.6-plus`）；不带斜杠时 `GetHighestPriorityRouteByModel` 走纯模型名匹配。

---

## 9. `image_generate` 工具改造

唯一的工具/基建变更，影响面收口在 `src/services/worker/internal/tools/builtin/image_generate/`：

### 9.1 `spec.go`

在 JSONSchema 加可选参数：

```go
"model_selector": {
  Type:        "string",
  Description: "optional override for the routing selector; e.g. \"openai/gpt-5-image-mini\". When unset, falls back to account override + system config.",
}
```

### 9.2 `executor.go`

- `resolveSelectedRoute(ctx, accountID, opts)` 接受可选 `selector` 字段
- 传入 `selector` 时：直接调 `routing.GetHighestPriorityRouteByModel(ctx, accountID, selector)`，**绕过** account `image_generative.model` override 与系统 config 查询
- 未传时：保留原有 `account_entitlement_overrides` → 系统 config 的两级回落
- 新增公共方法 `IsAvailableForModelSelector(ctx context.Context, accountID string, selector string) error`：供 director 启动时预检使用

### 9.3 测试覆盖

- 传 `model_selector` 命中已配路由 → 返回该路由
- 传 `model_selector` 无匹配路由 → 错误 `tool.not_configured` + 含 selector 的诊断信息
- 未传 `model_selector` → 走原 account override 路径（回归测试）
- 未传 + 系统 config 兜底（回归测试）

---

## 10. 主 Lua 流程伪代码

```lua
-- yuhua-stone-director/agent.lua

local STAGE_MODELS = { vl="qwen3.6-plus", story="deepseek-v4-flash",
                      bible="deepseek-v4-flash", storyboard="deepseek-v4-flash" }
local IMAGE_MODEL  = "openai/gpt-5-image-mini"

local function main()
  -- ① 加载风格预设
  local preset = load_style_preset()   -- io.open + json.decode

  -- ② 启动校验
  reject_if_no_image()
  reject_if_bad_channel()
  reject_if_image_route_unavailable(IMAGE_MODEL)

  -- ③ 解析 shot_count
  local shot_count = parse_shot_count(get_user_text())

  -- ④ 初始化进度 TODO
  init_todos({
    "解读图片", "撰写故事", "生成 Character Bible",
    string.format("生成 %d 镜头分镜", shot_count),
    "生成角色定档 CRS",
    string.format("生成 %d 张分镜图", shot_count),
    "整理最终报告",
  })

  -- ⑤ VL
  set_progress(1, "in_progress")
  local vl_md = spawn_and_wait("yuhua-stone-vl", {
    model_selector = STAGE_MODELS.vl,
    messages       = build_vl_messages(),
    style_preset   = preset,
  })
  local meta = parse_vl_markdown(vl_md)  -- title / mood_id / music_id / 三段散文
  meta = validate_mood_and_music(meta, preset)
  set_progress(1, "completed")

  -- ⑥ Story Writer
  set_progress(2, "in_progress")
  local story_md, segments = spawn_and_wait("yuhua-stone-storywriter", {
    model_selector = STAGE_MODELS.story,
    vl_output      = vl_md,
    shot_count     = shot_count,
    style_preset   = preset,
    meta           = meta,
  })
  set_progress(2, "completed")

  -- ⑦ Bible
  set_progress(3, "in_progress")
  local bible = spawn_and_wait("yuhua-stone-bible", {
    model_selector = STAGE_MODELS.bible,
    vl_output      = vl_md,
    story          = story_md,
    style_preset   = preset,
    meta           = meta,
  })
  bible = validate_bible_schema(bible)
  save_artifact("character_bible.json", bible)
  set_progress(3, "completed")

  -- ⑧ Storyboard
  set_progress(4, "in_progress")
  local board = spawn_and_wait("yuhua-stone-storyboard", {
    model_selector = STAGE_MODELS.storyboard,
    vl_output      = vl_md,
    story          = story_md,
    bible          = bible,
    segments       = segments,
    style_preset   = preset,
    meta           = meta,
    shot_count     = shot_count,
  })
  board = validate_storyboard_schema(board, shot_count)
  save_artifact("storyboard.json", board)
  set_progress(4, "completed")

  -- ⑨ CRS
  set_progress(5, "in_progress")
  local crs = tools.call("image_generate", {
    prompt         = render_crs_prompt(bible, preset, meta),
    input_images   = {},
    size           = "1024x1024",
    model_selector = IMAGE_MODEL,
    artifact_name  = "character_reference_sheet",
  })
  set_progress(5, "completed")

  -- ⑩ Shots 循环
  set_progress(6, "in_progress", { sub_total = shot_count, sub_done = 0 })
  local shot_artifacts, failures = {}, {}
  local prev_artifact = nil
  for i, shot in ipairs(board.shots) do
    local input_images = (i == 1) and { crs.artifact_id }
                                  or { crs.artifact_id, prev_artifact }
    local ok, art = retry_image_gen(2, function()
      return tools.call("image_generate", {
        prompt         = render_shot_prompt(shot, bible, preset, meta),
        input_images   = input_images,
        size           = "1024x1024",
        model_selector = IMAGE_MODEL,
        artifact_name  = string.format("shot_%02d", i),
      })
    end)
    if ok then
      shot_artifacts[i] = art.artifact_id
      prev_artifact     = art.artifact_id
    else
      shot_artifacts[i] = nil
      table.insert(failures, i)
    end
    set_progress(6, "in_progress", { sub_total = shot_count, sub_done = i })
  end
  set_progress(6, "completed")

  -- ⑪ Final report
  set_progress(7, "in_progress")
  local md = render_final_report({
    title         = meta.title,
    preset        = preset,
    meta          = meta,
    bible         = bible,
    storyboard    = board,
    crs_artifact  = crs.artifact_id,
    shot_artifacts= shot_artifacts,
    failures      = failures,
  })
  tools.call("document_write", {
    filename = "final_report.md",
    content  = md,
    format   = "markdown",
  })
  set_progress(7, "completed")
end

main()
```

---

## 11. `final_report.md` 模板

```markdown
# {title}

> 由「雨花石导演」智能体基于输入图生成
> 共 {shot_count} 个镜头 · 总时长 {total_seconds}s · 风格：{preset.display_name}
> 情绪：{meta.mood.name} · 配乐：{meta.music.name}

## 起源图片
![input](artifact:input_image)

## 角色定档（Character Bible）
![character reference sheet](artifact:character_reference_sheet)

\`\`\`json
{完整 Bible JSON}
\`\`\`

## 故事概要
{Story Writer 的概要段}

## 全文
{Story Writer 的长故事}

## {shot_count} 镜头分镜

### Shot 1 / {duration_seconds}s / {shot_type} / {camera_motion}
![shot-01](artifact:shot_01)

- 场景：{scene}
- 人物动作：{character_action}
- 情绪：{emotion}
- 光影：{lighting}
- 旁白：{voice_over}
- 字幕：{subtitle}

**视频生成 Prompt（可灵 / Seedance 通用）**：
\`\`\`
{rendered_video_prompt}
\`\`\`

---

（Shot 2..N 重复）

## 完整资源
- `character_bible.json` — Character Bible
- `storyboard.json` — Storyboard
- `character_reference_sheet.png` — 角色定档
- `shot_01.png` .. `shot_{N}.png` — 分镜静帧

## 失败镜头清单（如有）
- Shot 3 / Shot 9：图像生成失败，已写入 placeholder，请手动重试

## 后续：把图变成视频
把每个 shot 的 Prompt 和对应分镜图分别贴到可灵或 Seedance 的 image-to-video 输入框：
- 可灵：https://klingai.com（首选）
- Seedance：豆包视频生成

每段建议时长：8 秒。
```

### 11.1 视频 Prompt 单一格式（可灵 / Seedance 通用）

```
[镜头类型] {shot_type}, [运动] {camera_motion}.
{bible.character.name} {character_action}, {character_pose}.
{bible.character.appearance_signature}
{bible.character.clothing_signature}
[氛围] {emotion}, [光影] {lighting}.
[禁止] no costume change, no facial variation, no sudden movement, no character deformation.
[时长] {duration_seconds}s.
```

---

## 12. 进度 UI（TODO 事件）

主 Lua 通过 `todo_write` 事件驱动前端进度卡片，7 项：

| # | TODO 文案 | 子计数 |
|---|---|---|
| 1 | 解读图片 | — |
| 2 | 撰写故事 | — |
| 3 | 生成 Character Bible | — |
| 4 | 生成 N 镜头分镜 | — |
| 5 | 生成角色定档（CRS） | — |
| 6 | 生成 N 张分镜图 | `0/N` → `N/N` |
| 7 | 整理最终报告 | — |

---

## 13. 运行预算

| 项 | 数量 | 单次估算 | 总计 |
|---|---|---|---|
| LLM 阶段（qwen + 3× deepseek-flash） | 4 | $0.005-0.02 | ~$0.05 |
| 图像生成（CRS + 16 shots） | 17 | gpt-5-image-mini ~$0.04 | ~$0.7 |
| **总成本** | — | — | **~$0.75 / run** |
| LLM 耗时 | — | 30-90s 每阶段 | 2-6 分钟 |
| 图像耗时 | — | 15-30s 每张 | 5-9 分钟 |
| **端到端** | — | — | **8-15 分钟 / run** |

> **运行超时保障**：实现期需确认 `src/services/api/internal/app/config.go` 的 `defaultRunTimeoutMinutes` ≥ 20，否则在主 Lua 顶部加注释提醒部署方调整（参考 `industry-education-index` 中 `WAIT_MS = 8 * 60 * 1000` 的注释模式）。

---

## 14. 范围决策与不做之事

| 决策项 | v1 选择 | 理由 |
|---|---|---|
| 视频生成 | **不做**，仅输出 prompt | Arkloop 当前无视频工具，新增 provider/工具/计费 ≥ 1 周；v1 价值闭环在静帧 |
| 视频备选 Provider | v2 首选可灵，备选 Seedance | 可灵 API 申请门槛低，Seedance 贵且额度难拿 |
| Fallback 图像模型 | **不做** | mini 失败大概率重模型也失败，节省 quota |
| 多预设风格 | **不做**，单 `chinese-stone-cinema-v1` | 产品身份硬约束，避免品控分散 |
| Persona 设置面板 | **不做** | Arkloop 无 `settings_schema` 机制；`shot_count` 用文本意图解析替代 |
| 镜数 ask_user 表单 | **不弹** | 90% 用户走默认 16，表单是体验摩擦 |
| Memory 系统 | **不接** | 一次性内容生成，无跨 run 语义 |
| 多 channel 适配 | **仅 Web** | 其他 channel 长 Markdown + 17 图渲染崩坏 |
| 多图输入 | **v1 单图** | 多图分类逻辑复杂，留 v2 |
| PDF 导出 | **不默认** | 与"拿到能跑视频的素材"目标冲突；图烘焙进 PDF 反而碍事 |
| 关键帧锚（Shot 4/8/12/16 二级锚） | **不做** | 16 镜规模收益不大，CRS 单锚足够 |
| Persona 嵌套目录 | **不做** | 改 loader 影响面大；沿用平铺前缀分组 |

---

## 15. 实现路径

| Step | 内容 | 估时 |
|---|---|---|
| 1 | 改 `image_generate`：加 `model_selector` 参数 + `IsAvailableForModelSelector` 方法 + 单测 | 0.5d |
| 2 | 写 `style_preset.json`（按 §6 草案） | 0.2d |
| 3 | 写 4 个子 persona（yaml + prompt.md + 极简 `agent.lua`） | 1d |
| 4 | 写 director 主 `agent.lua`（stage 编排、进度、文本解析、image gen 循环、报告渲染） | 2d |
| 5 | 写 director `persona.yaml` + `prompt.md` | 0.3d |
| 6 | 端到端跑通：`ARKLOOP_YUHUA_SHOT_COUNT_DEBUG=4` 跑 4 镜头打通管线 | 1d |
| 7 | 跑 16 镜头完整流程，调 prompt 直到 mood/music/角色一致性稳定 | 2-3d |
| 8 | 写 ADR + README，提醒 OpenRouter 凭据要求 | 0.5d |

**总计**：~7-9 工作日（不含 review / iteration）。

---

## 16. 待实现期确认的开放问题

| # | 问题 | 备选方案 |
|---|---|---|
| O1 | `loader.go` 是否接受 `executor_config.style_preset_file` 未知字段 | 若拒绝则改为 lua 直接 `io.open("./style_preset.json")` |
| O2 | Selector 字符串是否需 `/` 前缀（如 `qwen/qwen3.6-plus`） | 取决于账号中 route.model 字段写法 |
| O3 | `defaultRunTimeoutMinutes` 当前值 | 不足 20min 时在 lua 顶部加部署注释 |
| O4 | `agent.stream_agent` 在传入 image content part 时的 messages 格式 | 参考 `industry-education-evaluator/agent.lua` |
| O5 | `tools.call("image_generate", ...)` 在 Lua 中的同步返回值结构 | 实现期对照 `executor/lua.go` 中的工具桥接 |

---

## 17. 附：访谈追溯

本设计文档基于 2026-05-26 与 Opus 的设计访谈最终结论锁定，关键决策点：

- **Q1**：v1 终点 = Storyboard + Prompt 包；v2 视频首选可灵
- **Q2**：Lua 编排 + 子 persona 架构
- **Q3**：`image_generate` 加 `model_selector` 参数（路径 B）
- **Q4**：仅同模型重试，不做模型 fallback
- **Q5**：混合状态契约（Bible / Storyboard 强 JSON，含 `appearance_signature` / `continuity_from_prev`）
- **Q6**：CRS 锚定 + 单张正面立绘 + Shot 2..N 永远 `[crs, prev]`
- **Q7**：模型 selector 硬编码，v1 不暴露
- **Q8**：图必填 + 零文本允许 + 单图
- **Q9**：单 Markdown 报告 + 独立 artifact，不默认 PDF，单一视频 Prompt 格式
- **Q10**：单预设 + 4 mood × 5 music palette + 奇石强约束 + 现代白话诗为主
- **Q11**：默认 16 镜 × 8s，文本意图解析镜数 + env debug 覆盖
- **Q12**：核心工具仅 `image_generate` + `document_write`；不接 memory；非 Web channel 拒跑
