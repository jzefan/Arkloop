-- yuhua-stone-director: 主编排 Lua。
--
-- 流水线（V2 含视频生成）：
--   1) VL          (qwen3.6-plus,  fork_recent)  — 看图、选 mood/music、写概要
--   2) Storywriter (deepseek-v4-flash, isolated) — 写长故事 + N 段拆分
--   3) Bible       (deepseek-v4-flash, isolated) — Character Bible JSON
--   4) Storyboard  (deepseek-v4-flash, isolated) — N 镜头 JSON
--   5) CRS         (image_generate)              — Character Reference Sheet
--   6) Shots loop  (image_generate × N)          — 每镜锚定 [CRS, prev?]
--   7) Videos loop (video_generate × N)          — image-to-video，每镜 5s mp4
--   8) Final report (document_write)             — final_report.md（含视频链接）

-- ====== 常量：模型 selector / 时长 / 失败处理 ======

local STAGE_MODELS = {
  vl         = "qwen3.6-plus",
  story      = "deepseek-v4-flash",
  bible      = "deepseek-v4-flash",
  storyboard = "deepseek-v4-flash",
}
-- 唯一走 OpenRouter 的模型；生产部署需在 ArkLoop 中配好 OpenRouter 凭据 +
-- 一条 model=openai/gpt-5-image-mini 的 image route，否则启动校验会拒跑。
local IMAGE_MODEL = "openai/gpt-5-image-mini"

-- V2：Seedance image-to-video。火山引擎 ARK 异步任务模式（POST 创建 → 轮询 →
-- 下载 mp4）。部署需在 ArkLoop 中配好 Doubao 凭据 + 一条 model=doubao-seedance-
-- 2-0-fast-260128 + output_modalities=["video"] 的 route。
--
-- 模型 id 取火山引擎 ARK 列表里的真实带版本日期 id：
--   doubao-seedance-2-0-fast-260128  ← Console 显示名 "Doubao-Seedance-2.0-fast"
-- 实测 2.0-fast 接受 duration ∈ {4, 5, ...}，**不接受 3**——所以下面 5s 是安全值。
local VIDEO_MODEL = "doubao-seedance-2-0-fast-260128"
-- 每镜视频生成时长（秒）。2.0-fast 最小 dur 是 5（实测 dur=3 报 InvalidParameter）。
local VIDEO_DURATION_S = 5
-- Seedance 视频分辨率，"480p" 或 "720p"。开发期 480p。
local VIDEO_RESOLUTION = "480p"

-- 旁白 TTS。默认走阿里 DashScope 上的 qwen3-tts-flash（国内服务，比 OpenAI
-- gpt-4o-mini-tts 便宜很多；CosyVoice 系列已合并进 qwen3-tts 体系）。
-- DashScope 协议由 audio_tts/dashscope.go 自动识别 base_url 切换，无需改其它代码。
-- voice=Cherry 是知性中文女声，与「现代知性美女鉴赏者」人设一致。
local AUDIO_TTS_MODEL = "qwen3-tts-flash"
local AUDIO_TTS_VOICE = "Cherry"

local SHOT_COUNT_DEFAULT = 4
local SHOT_COUNT_MIN     = 4
local SHOT_COUNT_MAX     = 32
local SHOT_DURATION_S    = 8

-- 每阶段子 agent 最长等待时间（毫秒），单次 wait 调用上限。
-- 与 industry-education-index 一致取 8 分钟：deepseek 长文 + JSON 拆分实测 30-60s，
-- 8 分钟覆盖端的慢响应/限流退避。**注意**：worker 端 desktop 模式 reaper 默认
-- ARKLOOP_RUN_TIMEOUT_MINUTES=5min（src/services/worker/internal/desktoprun/lifecycle.go:27），
-- 部署时务必 export ARKLOOP_RUN_TIMEOUT_MINUTES=30 否则子 run 会被 reaper 提前杀死。
local LLM_STAGE_TIMEOUT_MS = 8 * 60 * 1000

-- 单镜 image_generate 失败时的同模型重试上限（不做模型 fallback，参见设计文档 §5.7）。
local IMAGE_RETRY_MAX = 2

-- 非 Web channel 拒跑列表（设计文档 §5.8）。
local UNSUPPORTED_CHANNELS = {
  telegram = true,
  feishu = true,
  qq = true,
  discord = true,
  weixin = true,
  onebot = true,
}

-- ====== 通用工具 ======

local function trim(value)
  if value == nil then return "" end
  return tostring(value):match("^%s*(.-)%s*$") or ""
end

local function string_contains_any(text, keywords)
  if text == nil or text == "" then return false end
  for _, keyword in ipairs(keywords) do
    if string.find(text, keyword, 1, true) then return true end
  end
  return false
end

local function safe_json_decode(text)
  if text == nil or text == "" then return nil end
  local ok, decoded = pcall(json.decode, text)
  if not ok or decoded == nil then return nil end
  if type(decoded) == "table" and decoded.error ~= nil and decoded[1] == nil then
    -- gopher-lua-json 在某些版本里把 decode 错误塞进 .error；防御性处理。
    return nil
  end
  return decoded
end

local function json_encode_safe(value)
  local ok, encoded = pcall(json.encode, value)
  if not ok then return "{}" end
  return encoded
end

local function clamp(n, lo, hi)
  if n == nil then return lo end
  if n < lo then return lo end
  if n > hi then return hi end
  return n
end

-- 直接调 image_generate / document_write 工具。返回 (decoded_result, err)。
local function tool_call(name, args)
  local raw, err = tools.call(name, json_encode_safe(args))
  if err ~= nil then return nil, err end
  local decoded = safe_json_decode(raw)
  if decoded == nil then return nil, "tool result not JSON: " .. tostring(name) end
  return decoded, nil
end

-- ====== 进度 TODO 事件（参考 industry-education-index）======

local progress_todos = {}
local progress_tool_started = false

local function progress_completed_count()
  local count = 0
  for _, item in ipairs(progress_todos) do
    if item.status == "completed" then count = count + 1 end
  end
  return count
end

local function emit_progress_todos()
  if #progress_todos == 0 then return end
  if not progress_tool_started then
    context.emit("tool.call", {
      tool_call_id = "yuhua_stone_progress",
      tool_name = "todo_write",
      arguments = { todos = progress_todos },
      display_description = "雨花石导演进度",
    })
    progress_tool_started = true
  end
  context.emit("tool.result", {
    tool_call_id = "yuhua_stone_progress",
    tool_name = "todo_write",
    result = {
      todos = progress_todos,
      completed_count = progress_completed_count(),
      total_count = #progress_todos,
    },
  })
  context.emit("todo.updated", { todos = progress_todos })
end

local function set_progress(id, status, content, active_form)
  for _, item in ipairs(progress_todos) do
    if item.id == id then
      item.status = status
      if content ~= nil and content ~= "" then item.content = content end
      if active_form ~= nil and active_form ~= "" then
        item.active_form = active_form
      else
        item.active_form = nil
      end
      emit_progress_todos()
      return
    end
  end
end

local function init_progress_todos(shot_count)
  progress_tool_started = false
  progress_todos = {
    { id = "vl",         content = "解读图片", active_form = "正在读取图片并选择情绪/音乐", status = "in_progress" },
    { id = "story",      content = "撰写故事", status = "pending" },
    { id = "bible",      content = "生成 Character Bible", status = "pending" },
    { id = "storyboard", content = string.format("生成 %d 镜头分镜", shot_count), status = "pending" },
    { id = "crs",        content = "生成角色定档（CRS）", status = "pending" },
    { id = "shots",      content = string.format("生成 %d 张分镜图", shot_count), status = "pending" },
    { id = "video_frames", content = string.format("生成 %d 张视频安全首帧", shot_count), status = "pending" },
    { id = "videos",     content = string.format("生成 %d 段视频（每段 %ds）", shot_count, VIDEO_DURATION_S), status = "pending" },
    { id = "tts",        content = "生成旁白音频", status = "pending" },
    { id = "concat",     content = "合并视频为完整短片", status = "pending" },
    { id = "report",     content = "整理最终报告", status = "pending" },
  }
  emit_progress_todos()
end

-- ====== styles.json 加载 ======
--
-- persona 目录在 worker 启动时被打包到镜像里，Lua sandbox 默认无 io 全局。
-- 我们通过 context.get("styles") 让 worker 在加载 persona 时把
-- styles.json 的内容预注入到 InputJSON 里。
-- 若未注入则退回内嵌的最小默认（保证流水线不因配置缺失整体崩）。

local MINIMAL_DEFAULT_PRESET = {
  preset_id = "chinese-stone-cinema-v1",
  display_name = "中国奇石电影",
  global_style_tokens = {
    "Chinese cinema", "Chinese aesthetics", "slow cinematic motion",
    "natural lighting", "85mm cinema lens", "shallow depth of field", "film grain",
  },
  mood_palette = {
    { id = "zen-quiet", name = "禅意静谧", lighting = "晨雾微光 / 北窗柔光 / 日落金光", pacing = "缓慢、长镜头、留白多" },
  },
  music_style_palette = {
    { id = "guqin-solo", name = "古琴独奏", mood_fit = { "zen-quiet" }, tempo = "slow", keywords = { "古琴" } },
  },
  negative_prompts_global = {
    "no anime style", "no cartoon style", "no neon colors",
    "no Western fantasy elements", "no exaggerated animation", "no character deformation",
  },
  image_prompt_suffix_template = "Style: Chinese cinema, Chinese aesthetics, natural lighting, 85mm cinema lens, shallow depth of field, film grain. Mood: {mood_name}; lighting tone: {mood_lighting}. Avoid: anime, cartoon, neon, Western fantasy.",
  video_prompt_suffix_template = "Cinematic motion matching mood pacing ({mood_pacing}). Subtle realistic micro-movement only. Maintain exact character identity from reference image. No costume change, no facial variation, no character deformation, no sudden movement.",
  voice_style_default = {
    tone = "温和真诚",
    speed = "中等偏慢",
    language_style = "以现代白话诗为主（约 3/4），可点缀古诗词、宋词、《诗经》、唐诗（约 1/4）",
  },
}

local function load_style_preset()
  local raw = context.get("styles")
  if raw == nil or raw == "" then
    -- 兼容旧 persona.yaml: style_preset_file: style_preset.json
    raw = context.get("style_preset")
  end
  if raw == nil or raw == "" then
    return MINIMAL_DEFAULT_PRESET
  end
  local decoded = safe_json_decode(raw)
  if decoded == nil then return MINIMAL_DEFAULT_PRESET end
  return decoded
end

-- V2.4 风格 picker
-- styles.json 结构为 { styles: {id: {...}}, default_style_id, shared: {...} }
-- 这里弹一个 ask_user 表单让用户选风格，然后把 shared + style 的字段合并成一个
-- flat preset table（保持与下游 render_*_prompt / final_report 等代码兼容的字段名：
-- mood_palette / music_style_palette / image_prompt_suffix_template /
-- video_prompt_suffix_template / negative_prompts_global / voice_over_examples /
-- voice_style_default / color_palette_options）。

local function ask_user(args)
  local raw, err = context.ask_user(json_encode_safe(args))
  if err ~= nil then return nil, err end
  local decoded = safe_json_decode(raw)
  if decoded == nil then return nil, "无法读取用户表单输入" end
  return decoded.user_response or {}, nil
end

-- select_style 弹表单让用户选风格。返回选定的 style_id；用户取消或异常时退回 default。
local function select_style(preset)
  -- 如果 preset 不是 V2 schema（没有 styles 字段），直接退回默认
  if type(preset) ~= "table" or type(preset.styles) ~= "table" then
    return "realistic-documentary"
  end
  local default_id = preset.default_style_id or "realistic-documentary"

  -- 按 default 优先的稳定顺序排列 enum
  local style_ids = {}
  local style_descs = {}
  local style_names = {}
  if preset.styles[default_id] then
    table.insert(style_ids, default_id)
    table.insert(style_names, trim(preset.styles[default_id].display_name or default_id))
    table.insert(style_descs, trim(preset.styles[default_id].long_description or preset.styles[default_id].short_description or ""))
  end
  -- 其它 style 字母序排
  local rest = {}
  for id, _ in pairs(preset.styles) do
    if id ~= default_id then table.insert(rest, id) end
  end
  table.sort(rest)
  for _, id in ipairs(rest) do
    table.insert(style_ids, id)
    table.insert(style_names, trim(preset.styles[id].display_name or id))
    table.insert(style_descs, trim(preset.styles[id].long_description or preset.styles[id].short_description or ""))
  end

  if #style_ids <= 1 then return default_id end

  local response, err = ask_user({
    message = "请选择短片风格：",
    dismissLabel = "用默认（写实纪录风）",
    fields = { {
      key = "style_id",
      type = "string",
      title = "短片风格",
      required = true,
      description = "不同风格的取景、光影、旁白会完全不同。可以反复尝试，看哪种最贴近你期望的啧啧称奇效果。",
      enum = style_ids,
      enumNames = style_names,
      enumDescriptions = style_descs,
      default = default_id,
    } },
  })
  if err ~= nil or response == nil or trim(response.style_id) == "" then
    return default_id
  end
  -- 校验返回 id 真在 styles 表里
  if preset.styles[response.style_id] then
    return response.style_id
  end
  return default_id
end

-- resolve_effective_style 把 shared + 选中的 style 合并成一个 flat table，
-- 适配 director lua 其它部分（render_*_prompt / final_report）对老字段名的引用。
local function resolve_effective_style(preset, style_id)
  if type(preset) ~= "table" then return MINIMAL_DEFAULT_PRESET end
  local style = nil
  if type(preset.styles) == "table" then
    style = preset.styles[style_id] or preset.styles[preset.default_style_id]
  end
  if style == nil then return preset end  -- 兼容 V1 schema 直接返回 raw

  local shared = preset.shared or {}
  local effective = {}

  -- 顶层 metadata
  effective.preset_id = preset.preset_id or "yuhua-stone-multi-style"
  effective.version = preset.schema_version or "2"
  effective.style_id = style_id
  effective.display_name = style.display_name or style_id
  effective.description = style.short_description or style.long_description or ""

  -- shared
  effective.color_palette_options = shared.color_palette_options or {}
  effective.music_style_palette = shared.music_style_palette or {}

  -- style-specific（按老字段名暴露）
  effective.mood_palette = style.mood_palette or {}
  effective.image_prompt_suffix_template = style.image_prompt_suffix_template or ""
  effective.video_prompt_suffix_template = style.video_prompt_suffix_template or ""
  effective.negative_prompts_global = style.negative_prompts or {}
  effective.voice_style_default = style.voice_style_default or {}
  effective.voice_over_examples = style.voice_over_examples or {}
  effective.scene_locations_hint = style.scene_locations_hint or ""

  return effective
end

-- ====== 用户消息解析 ======

local function read_user_messages()
  local raw = context.get("messages")
  if raw == nil or raw == "" then return {} end
  local parsed = safe_json_decode(raw)
  if parsed == nil then return {} end
  return parsed
end

-- 获取用户最新一条文本。用户输入是上传图片的简单介绍，可作为故事引子；
-- 它不是标题，标题由 VL/故事链路根据图片与简介共同生成。
-- 注意：context.get("messages") 不暴露 attachments；图片附件能否被 VL 子 persona
-- 拿到，取决于 spawn 时使用 fork_recent + simple executor 的 ApplyImageFilter
-- 路径（见 src/services/worker/internal/executor/simple.go）。
local function latest_user_text()
  local messages = read_user_messages()
  for i = #messages, 1, -1 do
    if messages[i].role == "user" then return trim(messages[i].content) end
  end
  return ""
end

-- 解析镜数：
--   1) 用户文本里的 "(\d+) 镜" / "(\d+) 段" / "(\d+) shots?"
--   2) 环境变量 ARKLOOP_YUHUA_SHOT_COUNT_DEBUG 覆盖
--   3) 默认 16
local function parse_shot_count(user_text)
  local n = nil
  if user_text and user_text ~= "" then
    local m = string.match(user_text, "(%d+)%s*镜")
    if m == nil then m = string.match(user_text, "(%d+)%s*段") end
    if m == nil then m = string.match(string.lower(user_text), "(%d+)%s*shots?") end
    if m ~= nil then n = tonumber(m) end
  end
  if type(os) == "table" and type(os.getenv) == "function" then
    local debug_count = os.getenv("ARKLOOP_YUHUA_SHOT_COUNT_DEBUG")
    if debug_count ~= nil and debug_count ~= "" then
      local d = tonumber(debug_count)
      if d ~= nil then n = d end
    end
  end
  if n == nil then return SHOT_COUNT_DEFAULT end
  return clamp(math.floor(n), SHOT_COUNT_MIN, SHOT_COUNT_MAX)
end

-- ====== 启动校验 ======
--
-- 主流程入口的硬约束（设计文档 §5.8）。任何一项不满足，立刻 set_output
-- 给用户读得懂的提示并退出，不消耗任何 token / 图像配额。

local function find_image_attachment_hint()
  -- context.get("messages") 不暴露附件列表，我们只能间接观察：父 RunContext
  -- 的最近一条 user message 是否带 attachments。Lua sandbox 看不到结构化数据，
  -- 退而求其次用 context.get("has_image_attachment") 一类预注入开关。
  -- worker 当前没注入这个键，所以 v1 选择 "信任父对话已有图片"：如果用户
  -- 真的没传图，VL 子 persona 会自己产出"无图"概要（质量差但流程不死）。
  -- TODO(v2): worker.runcontext 预填 input.has_image_attachment，主 lua 据此
  --           做硬性拒跑。
  return true
end

local function check_channel()
  local channel = trim(context.get("channel_type"))
  if channel == "" then return true, "" end
  if UNSUPPORTED_CHANNELS[string.lower(channel)] then
    return false, string.format("本智能体目前仅适配 Web 客户端。检测到当前渠道：%s。请在 ArkLoop Web 中使用。", channel)
  end
  return true, ""
end

-- 检查 STAGE_MODELS / IMAGE_MODEL 是否都能在路由表中解析到 route。
-- 失败立刻 set_output 给运维侧明确的错误信息，避免主流程跑到一半才发现路由不通。
-- 返回 (ok_bool, message_string)。
local function check_routes_available()
  local raw = context.get("available_models")
  if raw == nil or raw == "" then
    -- 路由信息不可见时不阻塞流程（worker 可能跳过 mw_routing 阶段注入），
    -- 后续真正调用时仍会失败但不致提前误报。
    return true, ""
  end
  local available = safe_json_decode(raw)
  if type(available) ~= "table" or #available == 0 then
    return true, ""
  end
  -- 建立 model → bool 索引（裸模型名匹配）
  local by_model = {}
  for _, item in ipairs(available) do
    local model = trim(item.model)
    if model ~= "" then
      by_model[model] = true
    end
    local selector = trim(item.selector)
    if selector ~= "" then
      by_model[selector] = true
    end
  end
  local required = {
    { stage = "图像理解",      selector = STAGE_MODELS.vl },
    { stage = "故事撰写",      selector = STAGE_MODELS.story },
    { stage = "Character Bible", selector = STAGE_MODELS.bible },
    { stage = "分镜",          selector = STAGE_MODELS.storyboard },
    { stage = "分镜图像生成",  selector = IMAGE_MODEL },
    { stage = "视频生成",      selector = VIDEO_MODEL },
    { stage = "旁白 TTS",      selector = AUDIO_TTS_MODEL },
  }
  local missing = {}
  for _, item in ipairs(required) do
    if not by_model[item.selector] then
      table.insert(missing, string.format("- **%s** 需要模型 `%s`，但在你的路由表中找不到对应 route。",
        item.stage, item.selector))
    end
  end
  if #missing > 0 then
    local msg = "## 雨花石导演无法启动：缺少必需的模型路由\n\n" ..
      table.concat(missing, "\n") ..
      "\n\n请到 ArkLoop console 的「接入」页面添加这些 model 对应的 route 后再试。\n" ..
      "（DeepSeek 模型请走 provider_kind=deepseek 凭据；图像模型必须走 OpenRouter，credential base_url=https://openrouter.ai/api/v1；旁白 TTS 走阿里 DashScope qwen3-tts-flash，credential=千问 即可）"
    return false, msg
  end
  return true, ""
end

-- ====== 子 persona spawn 封装 ======

local function is_timeout_error(err)
  if err == nil then return false end
  local text = string.lower(tostring(err))
  if string.find(text, "deadline exceeded", 1, true) then return true end
  if string.find(text, "timeout", 1, true) then return true end
  if string.find(text, "超时", 1, true) then return true end
  return false
end

local function spawn_and_wait(persona_id, opts)
  opts = opts or {}
  local spawn_args = {
    persona_id = persona_id,
    context_mode = opts.context_mode or "isolated",
    profile = "task",
    nickname = opts.nickname or persona_id,
    input = opts.input or "",
  }
  if opts.model ~= nil and opts.model ~= "" then
    spawn_args.model = opts.model
  end
  -- isolated 不允许 inherit；fork_recent 默认全部继承。
  -- 我们只在 VL 阶段用 fork_recent（保留 attachments）。
  local child, err = agent.spawn(spawn_args)
  if err ~= nil or child == nil then
    return nil, "spawn " .. persona_id .. " failed: " .. tostring(err)
  end
  local timeout_ms = opts.timeout_ms or LLM_STAGE_TIMEOUT_MS
  local resolved, wait_err = agent.wait(child.id, timeout_ms, {
    display_description = opts.display_description or ("等待 " .. (opts.nickname or persona_id)),
  })
  if wait_err ~= nil then
    local seconds = math.floor(timeout_ms / 1000)
    if is_timeout_error(wait_err) then
      -- 超时的常见原因（运维侧诊断顺序）：
      -- ① 路由表里实际没有 spawn_args.model 对应的 route（虽然 available_models 通过预检，
      --    但 hide_from_picker 的 route 不在列表里 → 我们这边 spawn 时也走不通）；
      -- ② worker 端 ARKLOOP_RUN_TIMEOUT_MINUTES 默认 5min（lifecycle.go:27），子 run 被 reaper kill；
      -- ③ 模型 API 真的卡死了。
      local hint = string.format(
        "\n\n等待 %s 已超过 %d 秒。可能原因（按可能性排序）：\n" ..
        "1. **路由配置**：检查 ArkLoop 路由表中是否真有 `model=%s` 的可用 route（HideFromPicker 的 route 不算）。\n" ..
        "2. **worker 超时**：worker 容器未设 `ARKLOOP_RUN_TIMEOUT_MINUTES=30`，默认 5 分钟会被 reaper 提前杀掉子 run。\n" ..
        "3. **模型响应**：DeepSeek/Qwen API 可能临时限流或网络异常，稍后重试。",
        opts.nickname or persona_id, seconds, opts.model or "<unspecified>")
      return nil, tostring(wait_err) .. hint
    end
    return nil, "wait " .. persona_id .. " failed: " .. tostring(wait_err)
  end
  if resolved == nil then
    return nil, "wait " .. persona_id .. " returned nil"
  end
  if resolved.last_error ~= nil and trim(resolved.last_error) ~= "" then
    return nil, "sub-agent " .. persona_id .. " errored: " .. tostring(resolved.last_error)
  end
  local out = trim(resolved.output)
  if out == "" then
    return nil, "sub-agent " .. persona_id .. " returned empty output"
  end
  return out, nil
end

-- ====== VL 输出解析 ======
--
-- VL 子 persona prompt 约定的固定 H1 段：建议标题 / 选定情绪 / 选定音乐 /
-- 图片理解 / 情绪氛围 / 故事概要。

local function extract_h1_section(text, heading)
  local pattern = "#%s+" .. heading .. "%s*\n(.-)\n#"
  local section = string.match(text, pattern)
  if section == nil then
    -- 末段没有下一个 # 边界
    pattern = "#%s+" .. heading .. "%s*\n(.+)$"
    section = string.match(text, pattern)
  end
  if section == nil then return "" end
  return trim(section)
end

local function find_in_palette(palette, id)
  if palette == nil or id == nil then return nil end
  local lower_id = string.lower(trim(id))
  for _, item in ipairs(palette) do
    if string.lower(trim(item.id or "")) == lower_id then return item end
  end
  return nil
end

local function parse_vl_output(vl_markdown, preset)
  local title = extract_h1_section(vl_markdown, "建议标题")
  if title == "" then title = "雨花石电影" end

  local mood_id  = extract_h1_section(vl_markdown, "选定情绪")
  local music_id = extract_h1_section(vl_markdown, "选定音乐")

  local mood  = find_in_palette(preset.mood_palette, mood_id)
  local music = find_in_palette(preset.music_style_palette, music_id)
  -- fallback：取 palette 第一条
  if mood == nil and preset.mood_palette ~= nil then mood = preset.mood_palette[1] end
  if music == nil and preset.music_style_palette ~= nil then music = preset.music_style_palette[1] end

  return {
    title = title,
    mood = mood or { id = "zen-quiet", name = "禅意静谧", lighting = "柔光", pacing = "缓慢" },
    music = music or { id = "guqin-solo", name = "古琴独奏", tempo = "slow", keywords = {} },
    understanding = extract_h1_section(vl_markdown, "图片理解"),
    atmosphere    = extract_h1_section(vl_markdown, "情绪氛围"),
    summary       = extract_h1_section(vl_markdown, "故事概要"),
  }
end

-- ====== Storywriter / Bible / Storyboard 输出解析 ======

local function strip_fenced_json(text)
  local fenced = string.match(text, "```%s*[Jj][Ss][Oo][Nn]%s*\n(.-)\n%s*```")
  if fenced ~= nil then return trim(fenced) end
  fenced = string.match(text, "```%s*\n(.-)\n%s*```")
  if fenced ~= nil then return trim(fenced) end
  return nil
end

local function extract_first_balanced_object_or_array(text, open_char, close_char)
  local start_pos = string.find(text, open_char, 1, true)
  if start_pos == nil then return nil end
  local depth = 0
  local in_string = false
  local escape = false
  for pos = start_pos, string.len(text) do
    local ch = string.sub(text, pos, pos)
    if in_string then
      if escape then
        escape = false
      elseif ch == "\\" then
        escape = true
      elseif ch == '"' then
        in_string = false
      end
    else
      if ch == '"' then
        in_string = true
      elseif ch == open_char then
        depth = depth + 1
      elseif ch == close_char then
        depth = depth - 1
        if depth == 0 then return string.sub(text, start_pos, pos) end
      end
    end
  end
  return nil
end

-- repair_unescaped_quotes 修复 LLM 在 JSON 字符串值里写裸双引号的常见 bug。
--
-- 失败模式（实测在 storyboard 输出里出现过）：
--   "key_props": ["刻字——"戊寅年秋""]
-- 中间的两个 " 应该是 \" 但 LLM 没转义。
--
-- 算法：扫描时跟踪 in_string / escape 状态。当 in_string=true 遇到 "：
--   读到 " 之后的下一个非空白字符 next：
--     - 若 next ∈ {, : } ] EOF}：这个 " 是合法闭合，in_string=false
--     - 否则：这个 " 是字符串内的裸引号，把它替换成 \"，in_string 不变
-- 该算法只在 in_string 状态下加 escape，不会把合法 JSON 改坏。
--
-- 实现注意：gopher-lua 的 LState register stack 对单次 table.concat 的元素数有
-- 隐性上限——直接 table.insert 逐字符累积、最后一次性 concat 数万字符会触发
-- "registry overflow"。这里改成分块 concat：每累积 CHUNK_SIZE 个 piece 立刻合
-- 并成一个长字符串，重置 piece 表，最终只 concat 少量大 chunk。
local function repair_unescaped_quotes(text)
  if text == nil or text == "" then return text end
  local CHUNK_SIZE = 1024
  local chunks = {}
  local out = {}
  local count = 0
  local in_string = false
  local escape = false
  local len = #text

  local function push(piece)
    out[#out + 1] = piece
    count = count + 1
    if count >= CHUNK_SIZE then
      chunks[#chunks + 1] = table.concat(out)
      out = {}
      count = 0
    end
  end

  local i = 1
  while i <= len do
    local ch = string.sub(text, i, i)
    if escape then
      push(ch); escape = false
    elseif ch == "\\" then
      push(ch); escape = true
    elseif ch == '"' then
      if not in_string then
        push(ch); in_string = true
      else
        local j = i + 1
        while j <= len do
          local c = string.sub(text, j, j)
          if c ~= " " and c ~= "\t" and c ~= "\n" and c ~= "\r" then break end
          j = j + 1
        end
        local next_ch = (j <= len) and string.sub(text, j, j) or ""
        local is_closing =
          next_ch == "" or next_ch == "," or next_ch == ":" or
          next_ch == "}" or next_ch == "]"
        if is_closing then
          push(ch); in_string = false
        else
          push("\\\"")
        end
      end
    else
      push(ch)
    end
    i = i + 1
  end
  if count > 0 then
    chunks[#chunks + 1] = table.concat(out)
  end
  return table.concat(chunks)
end

-- 尝试多种方式从输出里取 JSON：原文 / fenced / 平衡括号扫描 / 修复未转义引号。
local function parse_loose_json(text, expect_array)
  if text == nil or text == "" then return nil end
  local decoded = safe_json_decode(text)
  if decoded ~= nil then return decoded end
  local fenced = strip_fenced_json(text)
  if fenced ~= nil then
    decoded = safe_json_decode(fenced)
    if decoded ~= nil then return decoded end
  end
  local open_char  = expect_array and "[" or "{"
  local close_char = expect_array and "]" or "}"
  local extracted = extract_first_balanced_object_or_array(text, open_char, close_char)
  if extracted ~= nil then
    decoded = safe_json_decode(extracted)
    if decoded ~= nil then return decoded end
  end
  -- 最后一道兜底：扫描修复字符串值内的未转义引号后再试一次。
  -- 只对已被识别为 JSON 形态的候选（fenced code block / balanced bracket
  -- extraction）跑修复——不要把整段 Markdown 原文喂进去，否则几万字的
  -- 中文+代码 raw text 会让 repair 的 string-piece 累积突破 gopher-lua 的
  -- registry stack 上限，触发 "registry overflow" 整次脚本崩。
  local candidates = {}
  if fenced ~= nil then candidates[#candidates + 1] = fenced end
  if extracted ~= nil then candidates[#candidates + 1] = extracted end
  for _, candidate in ipairs(candidates) do
    local repaired = repair_unescaped_quotes(candidate)
    if repaired ~= candidate then
      decoded = safe_json_decode(repaired)
      if decoded ~= nil then return decoded end
    end
  end
  return nil
end

local function parse_story_output(text)
  local full = extract_h1_section(text, "全文")
  if full == "" then full = trim(text) end
  -- 镜头段落区块在 `# 镜头段落` 之下
  local segments_section = extract_h1_section(text, "镜头段落")
  local segments = parse_loose_json(segments_section ~= "" and segments_section or text, true)
  if type(segments) ~= "table" then segments = {} end
  return full, segments
end

local function parse_bible(text)
  local decoded = parse_loose_json(text, false)
  if type(decoded) ~= "table" then return nil end
  if decoded.character == nil then return nil end
  if decoded.character.appearance_signature == nil
      or trim(decoded.character.appearance_signature) == "" then
    return nil
  end
  return decoded
end

local function parse_storyboard(text, expected_count)
  local decoded = parse_loose_json(text, false)
  if type(decoded) ~= "table" or type(decoded.shots) ~= "table" then return nil end
  -- 长度对齐：超长截断、不足保留（CRS 后循环时会按实际长度走）
  if #decoded.shots > expected_count then
    local trimmed = {}
    for i = 1, expected_count do
      table.insert(trimmed, decoded.shots[i])
    end
    decoded.shots = trimmed
  end
  return decoded
end

-- ====== Prompt 渲染 ======

local function suffix_render(template, mood)
  if template == nil or template == "" then return "" end
  local result = template
  result = string.gsub(result, "{mood_name}", trim(mood.name or ""))
  result = string.gsub(result, "{mood_lighting}", trim(mood.lighting or ""))
  result = string.gsub(result, "{mood_pacing}", trim(mood.pacing or ""))
  return result
end

local function join_negative_prompts(preset, bible)
  local seen = {}
  local out = {}
  local function add(value)
    local v = trim(value)
    if v ~= "" and seen[v] == nil then
      seen[v] = true
      table.insert(out, v)
    end
  end
  for _, item in ipairs(preset.negative_prompts_global or {}) do add(item) end
  if bible and bible.negative_prompts then
    for _, item in ipairs(bible.negative_prompts) do add(item) end
  end
  return out
end

local function safe_get(tbl, key, default)
  if type(tbl) ~= "table" then return default end
  local v = tbl[key]
  if v == nil or (type(v) == "string" and trim(v) == "") then return default end
  return v
end

local function render_crs_prompt(bible, preset, meta)
  local char = bible.character or {}
  local stone = bible.stone or {}
  local lines = {
    "A single character reference sheet, front-facing, coherent with the selected visual style.",
    "Beauty direction: exceptionally beautiful goddess-level modern Chinese woman, graceful and radiant, refined facial features, elegant presence, natural contemporary makeup, NOT fantasy, NOT costume drama.",
    "Character (verbatim, do not paraphrase):",
    trim(char.appearance_signature or ""),
    trim(char.clothing_signature or ""),
    -- V2.3：第一张 input_image 是用户实际上传的奇石照片。模型必须把这块石原样画出来。
    "IMPORTANT — the exotic stone in this composition MUST be the FIRST reference image (the user-uploaded stone): preserve its exact color palette, shape, surface pattern, internal veining, and translucency. Do NOT invent a different stone, do NOT alter the markings.",
    "Stone description from bible (use only as supplementary info, the reference photo wins on visual details):",
    trim(stone.appearance_signature or ""),
    "Pose: the woman holds the stone gently with both hands at chest height, looking down at it with curious calm attention. Full body visible from waist up.",
    "Setting: follow the selected style's scene guidance. The woman remains modern; the uploaded stone remains the visual hero.",
    suffix_render(preset.image_prompt_suffix_template, meta.mood),
    "Avoid: " .. table.concat(join_negative_prompts(preset, bible), ", ") .. ".",
  }
  return table.concat(lines, "\n")
end

local function render_shot_prompt(shot, bible, preset, meta)
  local char = bible.character or {}
  local stone = bible.stone or {}
  local scene = shot.scene or {}
  local lines = {
    string.format("Selected-style cinematic image, %s framing, %s. Keep the composition coherent with the style brief; the woman remains modern and the stone remains central.",
      shot.shot_type or "medium", shot.camera_motion or "static"),
    "Beauty direction: the woman is exceptionally beautiful, goddess-level, graceful, radiant, refined, modern Chinese / contemporary East Asian; natural high-end styling, never ancient costume.",
    "Character (verbatim from bible, do not paraphrase):",
    trim(char.appearance_signature or ""),
    trim(char.clothing_signature or ""),
  }
  if shot.stone_visibility ~= nil and shot.stone_visibility ~= "absent" then
    table.insert(lines, "Stone — CRITICAL: the stone in frame MUST be the user-uploaded reference photo (first input_image). Preserve its exact color/shape/pattern/veining; do not invent another stone.")
    table.insert(lines, "Stone bible note (supplementary): " .. trim(stone.appearance_signature or ""))
    table.insert(lines, "Stone visibility: " .. trim(shot.stone_visibility))
  end
  table.insert(lines, string.format("Scene: %s | %s | %s.",
    trim(scene.setting or "interior"),
    trim(scene.location or ""),
    trim(scene.time_of_day or "")))
  if type(scene.background_elements) == "table" and #scene.background_elements > 0 then
    table.insert(lines, "Background: " .. table.concat(scene.background_elements, ", "))
  end
  table.insert(lines, string.format("Action: %s %s (%s).",
    trim(char.name or "the man"),
    trim(shot.character_action or ""),
    trim(shot.character_pose or "")))
  table.insert(lines, "Lighting: " .. trim(shot.lighting or ""))
  table.insert(lines, "Mood: " .. trim(shot.emotion or ""))
  if shot.color_accent ~= nil and trim(shot.color_accent) ~= "" then
    table.insert(lines, "Color accent: " .. trim(shot.color_accent))
  end
  if shot.key_props ~= nil and type(shot.key_props) == "table" and #shot.key_props > 0 then
    table.insert(lines, "Key props: " .. table.concat(shot.key_props, ", "))
  end
  if shot.continuity_from_prev ~= nil and trim(shot.continuity_from_prev) ~= "" then
    table.insert(lines, "Continuity from previous shot: " .. trim(shot.continuity_from_prev))
  end
  table.insert(lines, suffix_render(preset.image_prompt_suffix_template, meta.mood))
  table.insert(lines, "Avoid: " .. table.concat(join_negative_prompts(preset, bible), ", ") .. ".")
  return table.concat(lines, "\n")
end

local function render_video_prompt(shot, bible, preset, meta)
  local char = bible.character or {}
  local stone_visibility = trim(shot.stone_visibility or "")
  local lines = {
    string.format("[Shot] %s. [Motion] %s.",
      shot.shot_type or "medium",
      shot.camera_motion or "static"),
    string.format("%s %s, %s.",
      trim(char.name or "the man"),
      trim(shot.character_action or ""),
      trim(shot.character_pose or "")),
    "[Stone lock] the exact uploaded stone remains the main subject throughout the video. Preserve its shape, size, color, surface pattern, veins, translucency, and silhouette from the first frame. Do not replace it with another stone. Stone visibility: " .. stone_visibility .. ".",
    "Character: " .. trim(char.appearance_signature or ""),
    "Clothing: " .. trim(char.clothing_signature or ""),
    "[Privacy-safe person framing] If a person appears, show elegant hands, sleeves, back view, or soft side silhouette only; avoid a clear frontal face in the first frame.",
    "[Mood] " .. trim(shot.emotion or "") .. ". [Lighting] " .. trim(shot.lighting or "") .. ".",
    suffix_render(preset.video_prompt_suffix_template, meta.mood),
    "[Forbid] no costume change, no facial variation, no sudden movement, no character deformation, no new stone, no swapped stone, no text-to-video reinterpretation.",
    string.format("[Duration] %ds.", shot.duration_seconds or SHOT_DURATION_S),
  }
  return table.concat(lines, "\n")
end

local function render_video_reference_frame_prompt(shot, bible, preset, meta)
  local char = bible.character or {}
  local stone = bible.stone or {}
  local scene = shot.scene or {}
  local lines = {
    "video_reference_frame for image-to-video, privacy-safe first frame.",
    "the exact uploaded stone remains the main subject: copy the FIRST reference image stone exactly, preserving shape, silhouette, colors, veins, speckles, cracks, translucency, and surface texture. Do NOT invent, replace, recolor, polish, carve, or resize the stone.",
    "Composition: macro or close-up of the stone on an elegant surface or held by graceful female hands. NO visible face, NO identifiable face, NO frontal portrait, NO readable identity. Hands, sleeves, shoulder/back silhouette are allowed.",
    "Beauty direction through hands and styling: exceptionally beautiful goddess-level modern Chinese woman implied by graceful hands, refined silk/linen sleeves, elegant posture, high-end contemporary styling. NOT fantasy, NOT ancient costume, NOT hanfu.",
    "Character continuity note (use only for hands/clothing mood, not face):",
    trim(char.clothing_signature or ""),
    "Stone bible note (supplementary only; FIRST reference image wins): " .. trim(stone.appearance_signature or ""),
    string.format("Scene: %s | %s | %s.",
      trim(scene.setting or "interior"),
      trim(scene.location or ""),
      trim(scene.time_of_day or "")),
    "Shot action cue: " .. trim(shot.character_action or ""),
    "Lighting: " .. trim(shot.lighting or ""),
    suffix_render(preset.image_prompt_suffix_template, meta.mood),
    "Avoid: " .. table.concat(join_negative_prompts(preset, bible), ", ") .. ", visible face, identifiable face, portrait, changed stone, different stone, fantasy creature.",
  }
  return table.concat(lines, "\n")
end

-- ====== Image generation 调用（含重试）======

local function call_image_generate(payload)
  local attempt = 0
  local last_err = ""
  while attempt <= IMAGE_RETRY_MAX do
    local result, err = tool_call("image_generate", payload)
    if err == nil and result ~= nil then
      local artifacts = result.artifacts
      if type(artifacts) == "table" and artifacts[1] ~= nil and trim(artifacts[1].key) ~= "" then
        return artifacts[1].key, nil
      end
      last_err = "image_generate returned no artifacts"
    else
      last_err = tostring(err)
    end
    attempt = attempt + 1
    if attempt > IMAGE_RETRY_MAX then break end
    -- 简单线性退避：1s → 3s
    local sleep_ms = (attempt == 1) and 1000 or 3000
    if type(context.sleep_ms) == "function" then context.sleep_ms(sleep_ms) end
  end
  return nil, last_err
end

-- ====== Final report 渲染 ======

local function markdown_escape_inline(text)
  if text == nil then return "" end
  -- 仅对 backticks 做最小转义（最常见的破坏 fenced block 边界的字符）。
  return (string.gsub(tostring(text), "```", "``\\`"))
end

local function render_narration_script(board, bible, preset, meta, user_intro)
  local lines = {}
  local voice_over_lines = {}
  local function out(s) table.insert(lines, s) end

  for i, shot in ipairs(board.shots or {}) do
    local voice = trim(shot.voice_over or "")
    if voice == "" then
      local scene = shot.scene or {}
      local location = trim(scene.location or "画面")
      voice = string.format("镜头 %d，石纹在%s的光里慢慢显出气韵。", i, location)
    end
    table.insert(voice_over_lines, voice)
  end

  local voice_over_full = table.concat(voice_over_lines, "\n")

  out("# 旁白脚本")
  out("")
  out("## 配音说明")
  out("")
  out("- 语气：" .. trim((bible.voice_style or {}).tone or (preset.voice_style_default or {}).tone or "温润知性"))
  out("- 节奏：" .. trim((bible.voice_style or {}).rhythm or (preset.voice_style_default or {}).rhythm or "从容舒展"))
  out("- 风格：" .. trim(preset.display_name or ""))
  if trim(user_intro) ~= "" then
    out("- 故事引子：" .. markdown_escape_inline(user_intro))
  end
  out("")
  out("## 完整旁白")
  out("")
  out(voice_over_full)
  out("")
  out("## 分镜旁白")
  out("")
  for i, voice in ipairs(voice_over_lines) do
    local start_s = (i - 1) * SHOT_DURATION_S
    local end_s = i * SHOT_DURATION_S
    out(string.format("- %02d:%02d-%02d:%02d Shot %02d：%s",
      math.floor(start_s / 60), start_s % 60,
      math.floor(end_s / 60), end_s % 60,
      i, voice))
  end

  return table.concat(lines, "\n")
end

local function render_voice_over_full(board)
  local lines = {}
  for i, shot in ipairs(board.shots or {}) do
    local voice = trim(shot.voice_over or "")
    if voice == "" then
      local scene = shot.scene or {}
      voice = string.format("镜头 %d，石纹在%s的光里慢慢显出气韵。", i, trim(scene.location or "画面"))
    end
    table.insert(lines, voice)
  end
  return table.concat(lines, "\n")
end

local function render_final_report(args)
  local meta             = args.meta
  local preset           = args.preset
  local bible            = args.bible
  local board            = args.storyboard
  local crs_key          = args.crs_key
  local shot_keys        = args.shot_keys
  local video_keys       = args.video_keys or {}
  local final_video_key  = args.final_video_key
  local voice_audio_key  = args.voice_audio_key
  local video_duration   = args.video_duration_s or 5
  local failures         = args.failures
  local video_failures   = args.video_failures
  local story_full       = args.story_full
  local narration_script = args.narration_script
  local user_intro       = args.user_intro

  local lines = {}
  local function out(s) table.insert(lines, s) end

  -- 统计有效视频数
  local valid_videos = 0
  for _, k in ipairs(video_keys) do
    if k ~= nil and k ~= "" then valid_videos = valid_videos + 1 end
  end

  out("# " .. trim(meta.title))
  out("")
  out(string.format("> 由「奇石鉴赏」智能体生成 · 共 %d 镜 · 静帧总时长约 %ds · 视频片段 %d × %ds",
    #board.shots, #board.shots * SHOT_DURATION_S, valid_videos, video_duration))
  out(string.format("> 风格：%s · 情绪：%s · 配乐：%s",
    trim(preset.display_name or ""),
    trim(meta.mood.name or ""),
    trim(meta.music.name or "")))
  out("")

  -- 最终完整短片放在最顶位置，方便用户立刻播放/下载
  if final_video_key ~= nil and final_video_key ~= "" then
    out("## 🎬 完整短片")
    out("")
    out("<video controls width=\"720\" src=\"artifact:" .. final_video_key .. "\"></video>")
    out("")
    out(string.format("**[⬇️ 下载完整 mp4](artifact:%s)** · 由 %d 段分镜视频按序 crossfade 拼接而成，总时长约 %d 秒",
      final_video_key, valid_videos, valid_videos * video_duration))
    out("")
  end

  if voice_audio_key ~= nil and voice_audio_key ~= "" then
    out("## 旁白音频")
    out("")
    out("<audio controls src=\"artifact:" .. voice_audio_key .. "\"></audio>")
    out("")
    out(string.format("**[下载旁白音频](artifact:%s)**", voice_audio_key))
    out("")
  end

  out("## 角色定档（Character Reference Sheet）")
  out("")
  if crs_key ~= nil then
    out("![CRS](artifact:" .. crs_key .. ")")
  else
    out("> CRS 未生成。")
  end
  out("")

  out("### Character Bible")
  out("")
  out("```json")
  out(json_encode_safe(bible))
  out("```")
  out("")

  out("## 故事")
  out("")
  if trim(user_intro) ~= "" then
    out("**用户图片简介 / 故事引子**：" .. markdown_escape_inline(user_intro))
    out("")
  end
  if trim(meta.summary) ~= "" then
    out("**概要**：" .. markdown_escape_inline(meta.summary))
    out("")
  end
  out(markdown_escape_inline(story_full))
  out("")

  if trim(narration_script) ~= "" then
    out("## 完整旁白脚本")
    out("")
    out(markdown_escape_inline(narration_script))
    out("")
  end

  out(string.format("## %d 镜分镜与视频生成 Prompt", #board.shots))
  out("")
  for i, shot in ipairs(board.shots) do
    local duration = shot.duration_seconds or SHOT_DURATION_S
    out(string.format("### Shot %d / %ds / %s / %s",
      shot.shot_id or i,
      duration,
      trim(shot.shot_type or "medium"),
      trim(shot.camera_motion or "static")))
    out("")
    local shot_key = shot_keys[i]
    if shot_key ~= nil then
      out("![shot " .. tostring(i) .. "](artifact:" .. shot_key .. ")")
    else
      out("> 本镜图像未生成（见末尾失败清单）。")
    end
    out("")
    local video_key = video_keys[i]
    if video_key ~= nil and video_key ~= "" then
      out(string.format("**🎬 视频片段**（%ds，mp4）：[shot_%02d_video.mp4](artifact:%s)", video_duration, i, video_key))
      out("")
      out("<video controls width=\"640\" src=\"artifact:" .. video_key .. "\"></video>")
      out("")
    end
    local scene = shot.scene or {}
    out("- 场景：" .. trim(scene.location or "") .. " · " ..
      trim(scene.time_of_day or "") .. " · " .. trim(scene.setting or ""))
    out("- 动作：" .. trim(shot.character_action or "") .. "（" .. trim(shot.character_pose or "") .. "）")
    out("- 情绪：" .. trim(shot.emotion or ""))
    out("- 光影：" .. trim(shot.lighting or ""))
    if shot.voice_over ~= nil and trim(shot.voice_over) ~= "" then
      out("- 旁白：" .. trim(shot.voice_over))
    end
    if shot.subtitle ~= nil and trim(shot.subtitle) ~= "" then
      out("- 字幕：" .. trim(shot.subtitle))
    end
    out("")
    out("**原始视频生成 Prompt**（可灵/Seedance 通用，亦即本镜实际使用的 Seedance 提示）：")
    out("")
    out("```")
    out(render_video_prompt(shot, bible, preset, args.meta))
    out("```")
    out("")
    out("---")
    out("")
  end

  if failures ~= nil and #failures > 0 then
    out("## 失败镜头图像清单")
    out("")
    for _, f in ipairs(failures) do
      out(string.format("- Shot %d：%s", f.shot_id, f.reason))
    end
    out("")
  end

  if video_failures ~= nil and #video_failures > 0 then
    out("## 失败视频清单")
    out("")
    for _, f in ipairs(video_failures) do
      out(string.format("- Shot %d：%s", f.shot_id, f.reason))
    end
    out("")
  end

  out("## 关于本片")
  out("")
  out(string.format("- **共 %d 镜分镜静帧** + **%d 段 %ds Seedance 视频片段**（image-to-video）", #board.shots, valid_videos, video_duration))
  out("- 视频已嵌入到上面每个镜头下面。如需独立下载，点击「shot_NN_video.mp4」链接。")
  out("- 若想用可灵或其它平台重新生成，可复制每镜下面的「原始视频生成 Prompt」+ 对应分镜图自行送入。")

  return table.concat(lines, "\n")
end

-- ====== 主流程 ======

local function main()
  local raw_preset = load_style_preset()
  local user_intro = latest_user_text()

  -- 启动校验
  local channel_ok, channel_msg = check_channel()
  if not channel_ok then
    context.set_output(channel_msg)
    return
  end
  if not find_image_attachment_hint() then
    context.set_output("请先上传一张奇石图片。我会以这块石为视觉主角，生成 4 镜头的鉴赏短片（含视频）。想要更多镜头可在文本里写「做 8 镜头版本」/「做 16 镜头版本」。")
    return
  end
  local routes_ok, routes_msg = check_routes_available()
  if not routes_ok then
    context.set_output(routes_msg)
    return
  end

  -- V2.3: 拿到用户上传的奇石图 artifact key，作为后续 CRS / 每张分镜的
  -- 视觉锚点 input_image——这样生成出来的"石头"就是用户那块，不是 LLM
  -- 凭想象画的另一块石。
  local user_stone_key = nil
  do
    local raw = context.get("user_attachments")
    if raw ~= nil and raw ~= "" then
      local atts = safe_json_decode(raw)
      if type(atts) == "table" and #atts > 0 and trim(atts[1].key) ~= "" then
        user_stone_key = atts[1].key
      end
    end
  end

  -- V2.4 风格 picker：弹 ask_user 表单让用户选风格（写实/梦幻/东方诗意）。
  -- 用户取消时退回 default_style_id。然后把 shared + 选中风格的字段合并成
  -- 一个 flat preset table，下游 render_*_prompt / final_report 不用改。
  local selected_style_id = select_style(raw_preset)
  local preset = resolve_effective_style(raw_preset, selected_style_id)
  -- preset.style_id / preset.display_name 暴露给后续报告

  local shot_count = parse_shot_count(user_intro)
  init_progress_todos(shot_count)

  -- V2.4 style brief：传给所有子 persona 的统一风格简档，让它们按选定风格调整输出。
  local style_brief = {
    style_id = preset.style_id,
    display_name = preset.display_name,
    description = preset.description,
    scene_locations_hint = preset.scene_locations_hint,
    mood_palette = preset.mood_palette,
    voice_style_default = preset.voice_style_default,
    voice_over_examples = preset.voice_over_examples,
    negative_prompts = preset.negative_prompts_global,
  }

  -- 1) VL
  local vl_input = json_encode_safe({
    user_intro = user_intro,
    shot_count = shot_count,
    instruction = "请按系统 prompt 严格输出固定 6 段 Markdown。请只观察父对话最近一条 user 消息中的图片附件。用户输入是上传图片的简单介绍，可作为故事引子，不要机械当标题。请按下方 style 字段约束选定情绪和场景。",
    style = style_brief,
  })
  local vl_output, vl_err = spawn_and_wait("yuhua-stone-vl", {
    context_mode = "fork_recent",
    nickname = "图像理解",
    input = vl_input,
    model = STAGE_MODELS.vl,
    display_description = "解读图片",
  })
  if vl_err ~= nil then
    set_progress("vl", "completed", "解读图片（失败）")
    context.set_output("图像理解阶段失败：" .. vl_err)
    return
  end
  local meta = parse_vl_output(vl_output, preset)
  set_progress("vl", "completed")

  -- 2) Story
  set_progress("story", "in_progress")
  local story_input = json_encode_safe({
    title = meta.title,
    mood = meta.mood,
    music = meta.music,
    vl_understanding = meta.understanding,
    vl_atmosphere = meta.atmosphere,
    vl_summary = meta.summary,
    user_intro = user_intro,
    shot_count = shot_count,
    style = style_brief,
  })
  local story_output, story_err = spawn_and_wait("yuhua-stone-storywriter", {
    context_mode = "isolated",
    nickname = "故事撰写",
    input = story_input,
    model = STAGE_MODELS.story,
    display_description = "撰写故事",
  })
  if story_err ~= nil then
    set_progress("story", "completed", "撰写故事（失败）")
    context.set_output("故事撰写阶段失败：" .. story_err)
    return
  end
  local story_full, story_segments = parse_story_output(story_output)
  set_progress("story", "completed")

  -- 3) Bible
  set_progress("bible", "in_progress")
  local bible_input = json_encode_safe({
    title = meta.title,
    mood = meta.mood,
    music = meta.music,
    vl_understanding = meta.understanding,
    vl_atmosphere = meta.atmosphere,
    vl_summary = meta.summary,
    story_full_text = story_full,
    user_intro = user_intro,
    style_preset = preset,
    style = style_brief,
  })
  local bible_output, bible_err = spawn_and_wait("yuhua-stone-bible", {
    context_mode = "isolated",
    nickname = "角色圣经",
    input = bible_input,
    model = STAGE_MODELS.bible,
    display_description = "生成 Character Bible",
  })
  if bible_err ~= nil then
    set_progress("bible", "completed", "生成 Character Bible（失败）")
    context.set_output("Character Bible 阶段失败：" .. bible_err)
    return
  end
  local bible = parse_bible(bible_output)
  if bible == nil then
    set_progress("bible", "completed", "生成 Character Bible（解析失败）")
    context.set_output("Character Bible JSON 解析失败。LLM 输出未通过 schema 校验。\n\n模型原文：\n" ..
      string.sub(bible_output, 1, 2000))
    return
  end
  set_progress("bible", "completed")

  -- 4) Storyboard
  set_progress("storyboard", "in_progress")
  local board_input = json_encode_safe({
    title = meta.title,
    shot_count = shot_count,
    shot_duration_seconds = SHOT_DURATION_S,
    bible = bible,
    story_full_text = story_full,
    story_segments = story_segments,
    user_intro = user_intro,
    mood = meta.mood,
    music = meta.music,
    style_preset_voice_examples = preset.voice_over_examples or {},
    style_preset_negative_prompts = preset.negative_prompts_global or {},
    style = style_brief,
  })
  local board_output, board_err = spawn_and_wait("yuhua-stone-storyboard", {
    context_mode = "isolated",
    nickname = "分镜导演",
    input = board_input,
    model = STAGE_MODELS.storyboard,
    display_description = "生成分镜 JSON",
  })
  if board_err ~= nil then
    set_progress("storyboard", "completed", "生成分镜（失败）")
    context.set_output("分镜阶段失败：" .. board_err)
    return
  end
  local board = parse_storyboard(board_output, shot_count)
  if board == nil or board.shots == nil or #board.shots == 0 then
    set_progress("storyboard", "completed", "生成分镜（解析失败）")
    context.set_output("Storyboard JSON 解析失败。\n\n模型原文：\n" ..
      string.sub(board_output, 1, 2000))
    return
  end
  set_progress("storyboard", "completed")
  local narration_script = render_narration_script(board, bible, preset, meta, user_intro)

  -- 5) CRS — 把用户上传的奇石作为唯一/首要 input_image 锚点，模型在画出
  -- 角色时就以这块石为参考。输出 CRS 含「现代知性美女 + 用户那块石」。
  set_progress("crs", "in_progress")
  local crs_prompt = render_crs_prompt(bible, preset, meta)
  local crs_input_images = nil
  if user_stone_key ~= nil then
    crs_input_images = { "attachment:" .. user_stone_key }
  end
  local crs_key, crs_err = call_image_generate({
    prompt = crs_prompt,
    input_images = crs_input_images,
    size = "1024x1024",
    output_format = "png",
    model_selector = IMAGE_MODEL,
    artifact_name = "character_reference_sheet",
  })
  if crs_err ~= nil or crs_key == nil then
    set_progress("crs", "completed", "生成角色定档（失败）")
    context.set_output("Character Reference Sheet 生成失败：" .. tostring(crs_err) ..
      "\n\n请确认 OpenRouter 凭据与 image route（model=" .. IMAGE_MODEL .. "）已正确配置。")
    return
  end
  set_progress("crs", "completed")

  -- 6) Shots loop
  local total_shots = #board.shots
  set_progress("shots", "in_progress", string.format("生成 %d 张分镜图（0/%d）", total_shots, total_shots),
    string.format("第 1/%d 镜", total_shots))
  local shot_keys = {}
  local failures = {}
  local prev_key = nil
  for i, shot in ipairs(board.shots) do
    local shot_prompt = render_shot_prompt(shot, bible, preset, meta)
    -- V2.3: input_images 顺序 [user_stone, CRS, prev_shot?]——用户原图放第一位，
    -- 保证模型每张分镜里的石头都是用户那块；CRS 锁人物；prev_shot 维持视觉延续。
    -- OpenRouter chat-completions 出图对 2-3 张 reference 处理稳定。
    local input_images = {}
    if user_stone_key ~= nil then
      table.insert(input_images, "attachment:" .. user_stone_key)
    end
    table.insert(input_images, "artifact:" .. crs_key)
    if prev_key ~= nil then
      table.insert(input_images, "artifact:" .. prev_key)
    end
    local key, err = call_image_generate({
      prompt = shot_prompt,
      input_images = input_images,
      size = "1024x1024",
      output_format = "png",
      model_selector = IMAGE_MODEL,
      artifact_name = string.format("shot_%02d", i),
    })
    if err == nil and key ~= nil then
      shot_keys[i] = key
      prev_key = key
    else
      shot_keys[i] = nil
      table.insert(failures, { shot_id = i, reason = tostring(err) })
    end
    set_progress("shots", "in_progress",
      string.format("生成 %d 张分镜图（%d/%d）", total_shots, i, total_shots),
      string.format("第 %d/%d 镜", math.min(i + 1, total_shots), total_shots))
  end
  set_progress("shots", "completed", string.format("生成 %d 张分镜图（已完成）", total_shots))

  -- 6.5) Video reference frames — Seedance 对含清晰真人正脸的 first_frame
  -- 容易触发隐私审核；但 text-to-video 又会丢失用户原石参考，导致视频里的石头
  -- 被模型重画成另一块。这里为每一镜额外生成“石头 + 女性手部/侧影 + 无清晰脸”的
  -- video-safe 首帧：第一参考图仍是用户上传的石头，输出图作为视频 first_frame。
  set_progress("video_frames", "in_progress",
    string.format("生成 %d 张视频安全首帧（0/%d）", total_shots, total_shots),
    string.format("第 1/%d 张", total_shots))
  local video_frame_keys = {}
  local video_frame_failures = {}
  for i, shot in ipairs(board.shots) do
    if shot_keys[i] == nil then
      video_frame_keys[i] = nil
      table.insert(video_frame_failures, { shot_id = i, reason = "分镜图缺失，跳过视频安全首帧" })
    else
      local frame_prompt = render_video_reference_frame_prompt(shot, bible, preset, meta)
      local input_images = {}
      if user_stone_key ~= nil then
        table.insert(input_images, "attachment:" .. user_stone_key)
      end
      -- 分镜图只作为构图/光感参考；prompt 明确规定石头以第一参考图为准。
      table.insert(input_images, "artifact:" .. shot_keys[i])
      local key, err = call_image_generate({
        prompt = frame_prompt,
        input_images = input_images,
        size = "1024x1024",
        output_format = "png",
        model_selector = IMAGE_MODEL,
        artifact_name = string.format("video_reference_frame_%02d", i),
      })
      if err == nil and key ~= nil then
        video_frame_keys[i] = key
      else
        video_frame_keys[i] = nil
        table.insert(video_frame_failures, { shot_id = i, reason = tostring(err) })
      end
    end
    set_progress("video_frames", "in_progress",
      string.format("生成 %d 张视频安全首帧（%d/%d）", total_shots, i, total_shots),
      string.format("第 %d/%d 张", math.min(i + 1, total_shots), total_shots))
  end
  set_progress("video_frames", "completed", string.format("生成 %d 张视频安全首帧（已完成）", total_shots))

  -- 7) Videos loop — Seedance image-to-video，每镜 first_frame 使用 video-safe
  -- 首帧，而不是带正脸的分镜图或上一镜尾帧。这样牺牲一点无缝连续性，换取：
  -- ① 用户上传的石头持续作为视觉锚点；② 显著降低隐私审核；③ 避免 text-to-video
  -- 重画错误石头。若仍触发隐私审核，不再降级 text-to-video，以免生成错石头。
  set_progress("videos", "in_progress",
    string.format("生成 %d 段视频（每段 %ds，0/%d）", total_shots, VIDEO_DURATION_S, total_shots),
    string.format("第 1/%d 段", total_shots))
  local video_keys = {}
  local video_failures = {}
  local video_success = 0
  local video_privacy_rejected = 0

  local function call_video(payload)
    local raw, err = tools.call("video_generate", json_encode_safe(payload))
    if err ~= nil then return nil, tostring(err) end
    local decoded = safe_json_decode(raw) or {}
    local arts = decoded.artifacts
    if type(arts) == "table" and arts[1] ~= nil and trim(arts[1].key) ~= "" then
      return arts[1].key, nil
    end
    return nil, "video_generate 返回无 artifact"
  end

  local function is_privacy_error(err_str)
    if err_str == nil then return false end
    return string.find(err_str, "InputImageSensitiveContent", 1, true) ~= nil
        or string.find(err_str, "PrivacyInformation", 1, true) ~= nil
  end

  for i, shot in ipairs(board.shots) do
    if shot_keys[i] == nil or video_frame_keys[i] == nil then
      video_keys[i] = nil
      table.insert(video_failures, { shot_id = i, reason = "分镜图或视频安全首帧缺失，跳过视频生成" })
    else
      local video_prompt = render_video_prompt(shot, bible, preset, meta)
      local base_payload = {
        prompt = video_prompt,
        duration_seconds = VIDEO_DURATION_S,
        resolution = VIDEO_RESOLUTION,
        model_selector = VIDEO_MODEL,
        artifact_name = string.format("shot_%02d_video", i),
      }

      base_payload.first_frame = "artifact:" .. video_frame_keys[i]
      local key, err = call_video(base_payload)
      if err ~= nil and is_privacy_error(err) then
        video_privacy_rejected = video_privacy_rejected + 1
        video_keys[i] = nil
        table.insert(video_failures, { shot_id = i,
          reason = "视频安全首帧仍被隐私审核拒绝；已停止 text-to-video fallback，避免生成错误石头" })
      elseif err ~= nil then
        video_keys[i] = nil
        table.insert(video_failures, { shot_id = i, reason = err })
      else
        video_keys[i] = key
        video_success = video_success + 1
      end
    end
    set_progress("videos", "in_progress",
      string.format("生成 %d 段视频（每段 %ds，%d/%d，成功 %d）",
        total_shots, VIDEO_DURATION_S, i, total_shots, video_success),
      string.format("第 %d/%d 段", math.min(i + 1, total_shots), total_shots))
  end
  local videos_summary
  if #video_failures == 0 then
    videos_summary = string.format("生成 %d 段视频（全部成功）", total_shots)
  else
    videos_summary = string.format("生成 %d 段视频（成功 %d / 失败 %d）",
      total_shots, video_success, #video_failures)
  end
  if video_privacy_rejected > 0 then
    videos_summary = videos_summary .. string.format("，其中 %d 段被隐私审核拒绝，未降级 text-to-video 以避免石头变形", video_privacy_rejected)
  end
  set_progress("videos", "completed", videos_summary)

  -- 7.25) TTS — 生成整片旁白音频 artifact，随后交给 video_concat 混进最终 mp4。
  local voice_audio_key = nil
  local voice_audio_err = nil
  local voice_over_full = render_voice_over_full(board)
  if trim(voice_over_full) ~= "" then
    set_progress("tts", "in_progress", "生成旁白音频")
    local tts_result, tts_err = tool_call("audio_tts", {
      text = voice_over_full,
      voice = AUDIO_TTS_VOICE,
      response_format = "mp3",
      model_selector = AUDIO_TTS_MODEL,
      artifact_name = "voice_over_audio",
    })
    if tts_err ~= nil then
      voice_audio_err = tostring(tts_err)
      set_progress("tts", "completed", "生成旁白音频（失败）")
    else
      local arts = tts_result.artifacts
      if type(arts) == "table" and arts[1] ~= nil and trim(arts[1].key) ~= "" then
        voice_audio_key = arts[1].key
        set_progress("tts", "completed", "生成旁白音频（已完成）")
      else
        voice_audio_err = "audio_tts 返回无 artifact"
        set_progress("tts", "completed", "生成旁白音频（无 artifact）")
      end
    end
  else
    set_progress("tts", "completed", "无旁白文本，跳过 TTS")
  end

  -- 7.5) Concat — 把所有成功的视频按 shot 顺序合并成一个完整短片
  local final_video_key = nil
  if video_success >= 1 then
    set_progress("concat", "in_progress", "合并视频为完整短片")
    local concat_inputs = {}
    for i = 1, total_shots do
      if video_keys[i] ~= nil and video_keys[i] ~= "" then
        table.insert(concat_inputs, "artifact:" .. video_keys[i])
      end
    end
    local concat_payload = {
      inputs = concat_inputs,
      artifact_name = "final_film",
      transition = "crossfade",
      transition_seconds = 0.35,
      reencode = true,
    }
    if voice_audio_key ~= nil and voice_audio_key ~= "" then
      concat_payload.audio = "artifact:" .. voice_audio_key
      concat_payload.audio_mode = "mix"
      concat_payload.narration_volume = 1.0
      concat_payload.background_volume = 0.18
    end
    local raw_c, err_c = tools.call("video_concat", json_encode_safe(concat_payload))
    if err_c ~= nil then
      set_progress("concat", "completed", "合并失败：" .. tostring(err_c))
    else
      local decoded = safe_json_decode(raw_c) or {}
      local arts = decoded.artifacts
      if type(arts) == "table" and arts[1] ~= nil and trim(arts[1].key) ~= "" then
        final_video_key = arts[1].key
        set_progress("concat", "completed",
          string.format("已处理 %d 段为完整短片", #concat_inputs))
      else
        set_progress("concat", "completed", "合并未返回 artifact")
      end
    end
  else
    set_progress("concat", "completed", "无可用视频，跳过合并")
  end

  -- 8) Final report
  set_progress("report", "in_progress")
  local report_md = render_final_report({
    meta = meta,
    preset = preset,
    bible = bible,
    storyboard = board,
    crs_key = crs_key,
    shot_keys = shot_keys,
    video_keys = video_keys,
    final_video_key = final_video_key,
    voice_audio_key = voice_audio_key,
    video_duration_s = VIDEO_DURATION_S,
    failures = failures,
    video_failures = video_failures,
    story_full = story_full,
    narration_script = narration_script,
    user_intro = user_intro,
  })
  local report_result, report_err = tool_call("document_write", {
    filename = "final_report.md",
    content = report_md,
  })
  local report_key = nil
  if report_err ~= nil then
    set_progress("report", "completed", "整理最终报告（上传失败）")
  else
    local arts = report_result.artifacts
    if type(arts) == "table" and arts[1] ~= nil and trim(arts[1].key) ~= "" then
      report_key = arts[1].key
    end
    set_progress("report", "completed")
  end

  local narration_key = nil
  local narration_result, narration_err = tool_call("document_write", {
    filename = "voice_over_script.md",
    content = narration_script,
  })
  if narration_err == nil then
    local arts = narration_result.artifacts
    if type(arts) == "table" and arts[1] ~= nil and trim(arts[1].key) ~= "" then
      narration_key = arts[1].key
    end
  end

  local summary_lines = {
    "## 奇石鉴赏短片素材包已生成",
    "",
    string.format("- 标题：%s", trim(meta.title)),
    string.format("- 风格：%s", trim(preset.display_name or "")),
    string.format("- 分镜图：%d 张", #shot_keys),
    string.format("- 视频：成功 %d / 共 %d 段", video_success, total_shots),
  }
  if final_video_key ~= nil and final_video_key ~= "" then
    table.insert(summary_lines, string.format("- 完整短片（含已生成的人声旁白，若 TTS 成功）：[final_film.mp4](artifact:%s)", final_video_key))
  end
  if report_key ~= nil and report_key ~= "" then
    table.insert(summary_lines, string.format("- 最终报告：[final_report.md](artifact:%s)", report_key))
  elseif report_err ~= nil then
    table.insert(summary_lines, "- 最终报告上传失败：" .. tostring(report_err))
  end
  if narration_key ~= nil and narration_key ~= "" then
    table.insert(summary_lines, string.format("- 旁白脚本：[voice_over_script.md](artifact:%s)", narration_key))
  elseif narration_err ~= nil then
    table.insert(summary_lines, "- 旁白脚本上传失败：" .. tostring(narration_err))
  end
  if voice_audio_key ~= nil and voice_audio_key ~= "" then
    table.insert(summary_lines, string.format("- 旁白音频：[voice_over_audio.mp3](artifact:%s)，已混入完整短片。", voice_audio_key))
  elseif voice_audio_err ~= nil then
    table.insert(summary_lines, "- 旁白音频生成失败，完整短片将不含人声：" .. tostring(voice_audio_err))
  end
  if #failures > 0 then
    table.insert(summary_lines, string.format("- 失败分镜图：%d 个，详情见 final_report.md", #failures))
  end
  if #video_failures > 0 then
    table.insert(summary_lines, string.format("- 失败视频：%d 个，详情见 final_report.md", #video_failures))
  end
  if video_privacy_rejected > 0 then
    table.insert(summary_lines, string.format("- 隐私审核：%d 段被拒绝；已避免 text-to-video fallback，防止石头被重画。", video_privacy_rejected))
  end
  context.set_output(table.concat(summary_lines, "\n"))
end

main()
