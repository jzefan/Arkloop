local EVALUATOR_PERSONA_ID = "industry-education-evaluator"
local SYNTHESIZER_PERSONA_ID = "industry-education-synthesizer"
local WAIT_MS = 8 * 60 * 1000
local WAIT_SLICE_MS = 15 * 1000
local WAIT_SLICE_LIMIT = math.floor(WAIT_MS / WAIT_SLICE_MS) + 1

local DIMENSIONS = {
  "基础与机制",
  "资源共建共享",
  "产学建设与服务",
  "人才培养质量",
}

local DEFAULT_MODELS = {
  { value = "deepseek-chat", label = "DeepSeek / deepseek-chat", family = "DeepSeek", aliases = { "deepseek-chat", "deepseek" } },
  { value = "qwen-max", label = "Qwen / qwen-max", family = "Qwen", aliases = { "qwen-max", "qwen" } },
  { value = "doubao-seed-1-6", label = "Doubao / doubao-seed-1-6", family = "Doubao", aliases = { "doubao-seed-1-6", "doubao" } },
}

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
  if progress_todos == nil or #progress_todos == 0 then return end
  if not progress_tool_started then
    context.emit("tool.call", {
      tool_call_id = "industry_education_progress",
      tool_name = "todo_write",
      arguments = { todos = progress_todos },
      display_description = "双高产教融合评估进度",
    })
    progress_tool_started = true
  end
  context.emit("tool.result", {
    tool_call_id = "industry_education_progress",
    tool_name = "todo_write",
    result = {
      todos = progress_todos,
      completed_count = progress_completed_count(),
      total_count = #progress_todos,
    },
  })
  context.emit("todo.updated", { todos = progress_todos })
end

local function set_progress_step(id, status, content, active_form)
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

local function init_progress_todos(model_count)
  progress_tool_started = false
  progress_todos = {
    { id = "sources", content = "检索公开资料", active_form = "正在检索学校官网、双高、产教融合和人才培养公开资料", status = "in_progress" },
    { id = "evaluate", content = "调用 " .. tostring(model_count) .. " 个评估模型", status = "pending" },
    { id = "score", content = "综合评分", status = "pending" },
    { id = "report", content = "生成报告文件", status = "pending" },
    { id = "pdf", content = "转化为 PDF 文件", status = "pending" },
  }
  emit_progress_todos()
end

local function trim(value)
  if value == nil then return "" end
  return tostring(value):match("^%s*(.-)%s*$") or ""
end

local TEXT_DELIMITERS = { "，", "。", ",", ".", "?", "？", "!", "！", "\n", "\r", "；", ";" }

local function before_first_delimiter(text)
  local earliest = nil
  for _, delimiter in ipairs(TEXT_DELIMITERS) do
    local pos = string.find(text, delimiter, 1, true)
    if pos ~= nil and (earliest == nil or pos < earliest) then
      earliest = pos
    end
  end
  if earliest == nil then return text end
  return string.sub(text, 1, earliest - 1)
end

local function take_before_plain(text, marker)
  local pos = string.find(text, marker, 1, true)
  if pos == nil then return text end
  return string.sub(text, 1, pos - 1)
end

local function extract_after_keyword(text, keyword)
  local _, stop = string.find(text, keyword, 1, true)
  if stop == nil then return "" end
  return trim(before_first_delimiter(string.sub(text, stop + 1)))
end

local function utf8_safe_prefix(text, max_bytes)
  if string.len(text) <= max_bytes then return text end
  local cut = max_bytes
  while cut > 0 do
    local next_byte = string.byte(text, cut + 1)
    if next_byte == nil or next_byte < 128 or next_byte >= 192 then break end
    cut = cut - 1
  end
  if cut <= 0 then return "" end
  return string.sub(text, 1, cut)
end

local function contains_any(text, keywords)
  for _, keyword in ipairs(keywords) do
    if string.find(text, keyword, 1, true) then return true end
  end
  return false
end

local function read_messages()
  local raw = context.get("messages")
  if raw == nil or raw == "" then return {} end
  local parsed, err = json.decode(raw)
  if err ~= nil or parsed == nil then return {} end
  return parsed
end

local function latest_user_text()
  local prompt = trim(context.get("user_prompt"))
  if prompt ~= "" then return prompt end
  local messages = read_messages()
  for i = #messages, 1, -1 do
    if messages[i].role == "user" then
      return trim(messages[i].content)
    end
  end
  return ""
end

local NON_SCHOOL_INPUTS = {
  ["继续"] = true,
  ["请继续"] = true,
  ["继续吧"] = true,
  ["好的"] = true,
  ["好"] = true,
  ["收到"] = true,
  ["开始"] = true,
  ["继续评估"] = true,
  ["请继续评估"] = true,
  ["重试"] = true,
  ["再试一次"] = true,
}

local function looks_like_non_school_input(text)
  local cleaned = trim(text)
  if cleaned == "" then return true end
  if NON_SCHOOL_INPUTS[cleaned] then return true end
  if contains_any(cleaned, { "请继续", "继续", "重试", "再试一次" }) and string.len(cleaned) <= 12 then
    return true
  end
  return false
end

local SCHOOL_NAMES_CACHE = nil

local function school_names()
  if SCHOOL_NAMES_CACHE ~= nil then return SCHOOL_NAMES_CACHE end
  SCHOOL_NAMES_CACHE = {}
  local raw = context.get("school_names")
  if raw == nil or raw == "" then raw = context.get("vocational_colleges") end
  if raw == nil or raw == "" then return SCHOOL_NAMES_CACHE end
  local parsed, err = json.decode(raw)
  if err ~= nil or parsed == nil then return SCHOOL_NAMES_CACHE end
  local rows = parsed
  if type(parsed) == "table" and type(parsed.schools) == "table" then
    rows = parsed.schools
  end
  for _, item in ipairs(rows) do
    local name = ""
    if type(item) == "string" then
      name = trim(item)
    elseif type(item) == "table" then
      name = trim(item.name)
    end
    if name ~= "" then
      table.insert(SCHOOL_NAMES_CACHE, name)
    end
  end
  table.sort(SCHOOL_NAMES_CACHE, function(a, b)
    return string.len(a) > string.len(b)
  end)
  return SCHOOL_NAMES_CACHE
end

local function replace_plain_all(text, needle, replacement)
  if needle == "" then return text end
  local pieces = {}
  local start_pos = 1
  while true do
    local from_pos, to_pos = string.find(text, needle, start_pos, true)
    if from_pos == nil then
      table.insert(pieces, string.sub(text, start_pos))
      break
    end
    table.insert(pieces, string.sub(text, start_pos, from_pos - 1))
    table.insert(pieces, replacement)
    start_pos = to_pos + 1
  end
  return table.concat(pieces)
end

local function normalize_school_match_text(text)
  local normalized = trim(text)
  for _, token in ipairs({ " ", "\t", "\n", "\r", "　", "，", "。", "、", ",", ".", "?", "？", "!", "！", "；", ";", "：", ":", "“", "”", "\"", "'", "《", "》", "（", "）", "(", ")" }) do
    normalized = replace_plain_all(normalized, token, "")
  end
  return normalized
end

local function match_school_name(text)
  local haystack = normalize_school_match_text(text)
  if haystack == "" then return "" end
  for _, name in ipairs(school_names()) do
    local needle = normalize_school_match_text(name)
    if needle ~= "" and string.find(haystack, needle, 1, true) then
      return name
    end
  end
  return ""
end

local SCHOOL_QUERY_PREFIXES = {
  "请继续评估一下",
  "请继续评估",
  "继续评估一下",
  "继续评估",
  "请帮我评估一下",
  "请帮我评估",
  "帮我评估一下",
  "帮我评估",
  "评估一下",
  "请分析一下",
  "帮我分析一下",
  "分析一下",
  "看一下",
  "查一下",
  "了解一下",
  "请帮我",
  "麻烦你",
  "麻烦",
  "帮我",
  "请",
  "一下子",
  "一下",
  "下",
  "对",
  "关于",
}

local function starts_with(text, prefix)
  return prefix ~= "" and string.sub(text, 1, string.len(prefix)) == prefix
end

local function strip_school_query_prefixes(text)
  local cleaned = trim(text)
  local changed = true
  while changed do
    changed = false
    for _, prefix in ipairs(SCHOOL_QUERY_PREFIXES) do
      if starts_with(cleaned, prefix) then
        cleaned = trim(string.sub(cleaned, string.len(prefix) + 1))
        changed = true
        break
      end
    end
  end
  return cleaned
end

local SCHOOL_SUFFIXES = {
  "职业技术大学",
  "职业技术学院",
  "职业大学",
  "职业学院",
  "高等专科学校",
  "专科学校",
}

local function trim_leading_school_noise(text)
  local cleaned = strip_school_query_prefixes(text)
  local earliest = nil
  for _, marker in ipairs({ "学校", "院校", "高校" }) do
    local _, stop = string.find(cleaned, marker, 1, true)
    if stop ~= nil and stop < string.len(cleaned) then
      if earliest == nil or stop < earliest then earliest = stop end
    end
  end
  if earliest ~= nil then
    cleaned = trim(string.sub(cleaned, earliest + 1))
  end
  return cleaned
end

local function extract_by_school_suffix(text)
  local cleaned = trim_leading_school_noise(text)
  if cleaned == "" then return "" end
  local best_stop = nil
  for _, suffix in ipairs(SCHOOL_SUFFIXES) do
    local _, stop = string.find(cleaned, suffix, 1, true)
    if stop ~= nil and (best_stop == nil or stop < best_stop) then
      best_stop = stop
    end
  end
  if best_stop == nil then return cleaned end
  return trim(string.sub(cleaned, 1, best_stop))
end

local function normalize_school_candidate(text)
  local matched = match_school_name(text)
  if matched ~= "" then return matched end
  return extract_by_school_suffix(text)
end

local function extract_school_name(text)
  local matched = match_school_name(text)
  if matched ~= "" then return matched end
  for _, keyword in ipairs({ "评估", "分析" }) do
    local found = extract_after_keyword(text, keyword)
    if found ~= "" and not looks_like_non_school_input(found) then
      local normalized = normalize_school_candidate(found)
      if normalized ~= "" and not looks_like_non_school_input(normalized) then return normalized end
    end
  end
  local after_for = extract_after_keyword(text, "为")
  if after_for ~= "" then
    local found = trim(take_before_plain(after_for, "生成"))
    if found ~= "" and not looks_like_non_school_input(found) then
      local normalized = normalize_school_candidate(found)
      if normalized ~= "" and not looks_like_non_school_input(normalized) then return normalized end
    end
  end
  local cleaned = trim(text)
  if cleaned ~= "" and string.len(cleaned) <= 80 and not looks_like_non_school_input(cleaned) then
    local normalized = normalize_school_candidate(cleaned)
    if normalized ~= "" and not looks_like_non_school_input(normalized) then return normalized end
  end
  return ""
end

local function ask_user(args)
  local raw, err = context.ask_user(json.encode(args))
  if err ~= nil then error(err) end
  local decoded, decode_err = json.decode(raw)
  if decode_err ~= nil or decoded == nil then
    error("无法读取用户表单输入")
  end
  return decoded.user_response or {}
end

local configured_model_options

local function choose_mode()
  local fields = {
    {
      key = "mode",
      type = "string",
      title = "评估模式",
      required = true,
      description = "综合评估会调用多个模型交叉评估，并汇总为一份报告；多模型评估会保留不同模型的评估视角。",
      enum = { "多模型评估", "综合评估", "单模型评估" },
      enumDescriptions = {
        "保留多个模型的评估视角，适合对比不同模型判断。",
        "调用多个模型交叉评估，并汇总为一份报告。",
        "只使用一个模型完成评估。",
      },
      default = "多模型评估",
    },
  }
  return ask_user({
    message = "请选择评估模式。",
    fields = fields,
  })
end

local function choose_models(mode)
  local enum = {}
  local enum_names = {}
  local available = configured_model_options()
  local source_models = available
  if #source_models == 0 then
    source_models = DEFAULT_MODELS
  end
  for _, model in ipairs(source_models) do
    table.insert(enum, model.value)
    table.insert(enum_names, model.label)
  end
  local max_items = nil
  if mode == "单模型评估" then
    max_items = 1
  end
  local field = {
    key = "models",
    type = "array",
    title = "评估模型",
    description = (#available > 0) and "默认不选择模型；请选择一个或多个模型后提交。不选择并点击“使用当前模型”时，将沿用当前聊天模型。" or "默认不选择模型；未读取到可用路由时可点击“使用当前模型”沿用当前聊天模型。",
    required = true,
    enum = enum,
    enumNames = enum_names,
    minItems = 1,
  }
  if max_items ~= nil then field.maxItems = max_items end
  local response = ask_user({
    message = "请选择用于评估的模型。",
    dismissLabel = "使用当前模型",
    fields = { field },
  })
  if response.models == nil or #response.models == 0 then
    return { "" }
  end
  return response.models
end

local function selector_label(value)
  local left, right = string.match(value, "^([^%^]+)%^(.+)$")
  if left ~= nil and right ~= nil then
    return trim(left) .. " / " .. trim(right)
  end
  return value
end

local function model_label(model_value)
  if trim(model_value) == "" then return "当前模型" end
  for _, model in ipairs(DEFAULT_MODELS) do
    if model.value == model_value then return model.label end
  end
  return selector_label(model_value)
end

local function detect_family(text)
  local lower = string.lower(trim(text))
  if string.find(lower, "deepseek") then return "DeepSeek" end
  if string.find(lower, "qwen") or string.find(lower, "tongyi") or string.find(lower, "通义") then return "Qwen" end
  if string.find(lower, "doubao") or string.find(lower, "豆包") then return "Doubao" end
  return ""
end

local function progress_model_label(model_value)
  local label = model_label(model_value)
  local family = detect_family(label .. " " .. tostring(model_value))
  local selected_model = trim(model_value)
  local left, right = string.match(selected_model, "^([^%^]+)%^(.+)$")
  if right ~= nil then selected_model = trim(right) end
  if family ~= "" and selected_model ~= "" then
    return family .. " / " .. selected_model
  end
  return label
end

local function is_text_evaluator_model(text)
  local lower = string.lower(trim(text))
  if lower == "" then return false end
  local blocked = {
    "embedding",
    "embed",
    "rerank",
    "ocr",
    "image",
    "wan",
    "video",
    "audio",
    "whisper",
    "asr",
    "tts",
    "speech",
    "vlm",
  }
  for _, keyword in ipairs(blocked) do
    if string.find(lower, keyword, 1, true) then
      return false
    end
  end
  return true
end

function configured_model_options()
  local raw = context.get("available_models")
  if raw == nil or raw == "" then return {} end
  local parsed, err = json.decode(raw)
  if err ~= nil or parsed == nil then return {} end
  local out = {}
  local seen = {}
  for _, item in ipairs(parsed) do
    local selector = trim(item.selector)
    local model = trim(item.model)
    local provider_kind = trim(item.provider_kind)
    local credential_name = trim(item.credential_name)
    local family = detect_family(provider_kind .. " " .. credential_name .. " " .. model)
    if selector ~= "" and family ~= "" and is_text_evaluator_model(selector .. " " .. model) and seen[selector] == nil then
      seen[selector] = true
      table.insert(out, {
        value = selector,
        label = family .. " / " .. selector_label(selector),
        family = family,
        aliases = { selector },
      })
    end
  end
  return out
end

local function model_selector(model_value)
  for _, model in ipairs(DEFAULT_MODELS) do
    if model.value == model_value then
      return model.aliases[1]
    end
  end
  return model_value
end

local function tool_call(name, args)
  local raw, err = tools.call(name, json.encode(args))
  if err ~= nil then return nil, err end
  local decoded, decode_err = json.decode(raw)
  if decode_err ~= nil then return nil, decode_err end
  return decoded, nil
end

local function active_tool_provider_configs_by_group()
  local raw = context.get("active_tool_provider_configs_by_group")
  if raw == nil or raw == "" then return {} end
  local parsed, err = json.decode(raw)
  if err ~= nil or parsed == nil then return {} end
  return parsed
end

local function active_web_search_provider_name()
  local groups = active_tool_provider_configs_by_group()
  local group = groups.web_search
  if group == nil then return "" end
  if type(group) == "table" then
    if type(group.provider_name) == "string" and trim(group.provider_name) ~= "" then
      return trim(group.provider_name)
    end
    if type(group.provider) == "string" and trim(group.provider) ~= "" then
      return trim(group.provider)
    end
    if type(group.name) == "string" and trim(group.name) ~= "" then
      return trim(group.name)
    end
    if #group > 0 then
      local first = group[1]
      if type(first) == "table" then
        if type(first.provider_name) == "string" and trim(first.provider_name) ~= "" then
          return trim(first.provider_name)
        end
        if type(first.provider) == "string" and trim(first.provider) ~= "" then
          return trim(first.provider)
        end
        if type(first.name) == "string" and trim(first.name) ~= "" then
          return trim(first.name)
        end
      elseif type(first) == "string" then
        return trim(first)
      end
    end
  elseif type(group) == "string" then
    return trim(group)
  end
  return ""
end

local function search_error_message(provider_name, search_err)
  local raw_err = trim(tostring(search_err or "empty_results"))
  local lower_err = string.lower(raw_err)
  if string.find(lower_err, "timeout", 1, true) or string.find(raw_err, "超时", 1, true) then
    if provider_name == "web_search.basic" then
      return "联网搜索超时，当前提供商为 web_search.basic，未获取到可用于核验院校身份的公开结果。请稍后重试；若仍超时，请在“接入”中将 browser-search 端点切换到 Tavily 或 SearXNG 后重试。原始信息：" .. raw_err
    end
    return "联网搜索超时，当前提供商暂未返回可用于核验院校身份的公开结果。请稍后重试；若仍超时，请在“接入”中切换联网搜索提供商后重试。原始信息：" .. raw_err
  end
  if provider_name == "web_search.basic" then
    return "当前使用的联网搜索提供商为 web_search.basic，未返回可用于核验院校身份的公开结果。请在“接入”中切换到 Tavily 或 SearXNG 后重试。原始信息：" .. raw_err
  end
  return "当前搜索提供商未返回可用于核验院校身份的公开结果。请稍后重试，或在“接入”中切换联网搜索提供商。原始信息：" .. raw_err
end

local function search_sources(school_name, year, analysis_focus)
  local queries = {
    school_name,
    school_name .. " 官网",
    school_name .. " 双高 高水平高职 专业群",
    school_name .. " 产教融合 校企合作 实训基地 " .. year,
    school_name .. " 人才培养 就业质量 技能竞赛 科研服务 " .. year,
  }
  local normalized_focus = trim(analysis_focus)
  if normalized_focus ~= "" then
    queries[#queries] = queries[#queries] .. " " .. normalized_focus
  end
  local search_result, search_err = tool_call("web_search", { queries = queries, max_results = 12 })
  if search_err ~= nil or search_result == nil or search_result.results == nil or #search_result.results == 0 then
    return nil, search_error_message(active_web_search_provider_name(), search_err)
  end

  local sources = {}
  local facts = {}
  local fetch_calls = {}
  local seen_urls = {}
  for i, hit in ipairs(search_result.results) do
    if #sources >= 24 then break end
    local url = trim(hit.url)
    local dedupe_key = url
    if dedupe_key == "" then dedupe_key = trim(hit.title) .. "::" .. trim(hit.snippet) end
    if dedupe_key ~= "" and seen_urls[dedupe_key] == nil then
      seen_urls[dedupe_key] = true
      local id = "S" .. tostring(#sources + 1)
      local label = trim(hit.title)
      if label == "" then label = "公开来源 " .. tostring(#sources + 1) end
      table.insert(sources, { id = id, label = label, url = url })
      table.insert(facts, {
        claim = trim(hit.snippet),
        source_ids = { id },
      })
      if url ~= "" and #fetch_calls < 6 then
        table.insert(fetch_calls, { id = id, url = url })
      end
    end
  end

  for _, item in ipairs(fetch_calls) do
    local fetched = tool_call("web_fetch", { url = item.url, max_length = 3600 })
    if fetched ~= nil and fetched.content ~= nil then
      table.insert(facts, {
        claim = utf8_safe_prefix(trim(fetched.content), 1200),
        source_ids = { item.id },
      })
    end
  end

  local evidence_text = ""
  local source_text = ""
  for _, fact in ipairs(facts) do
    evidence_text = evidence_text .. " " .. trim(fact.claim)
  end
  for _, source in ipairs(sources) do
    source_text = source_text .. " " .. trim(source.label) .. " " .. trim(source.url)
  end
  local verification_text = school_name .. " " .. source_text .. " " .. evidence_text
  local verified = false
  if contains_any(verification_text, {
    "高职",
    "高等职业",
    "职业技术",
    "职业大学",
    "职业学院",
    "专科高等学校",
    "双高",
    "高水平高职",
    "高水平专业群",
  }) then
    verified = true
  end

  return {
    school_name = school_name,
    year = year,
    eligibility = {
      verified = verified,
      type = verified and "高职/双高相关院校" or "未核验",
      basis = verified and "公开来源中出现高职、职业技术、双高或高水平高职相关信息。" or "公开来源不足以确认院校类型。",
      source_ids = (#sources > 0) and { "S1" } or {},
    },
    sources = sources,
    facts = facts,
    missing = { "{{就业率}}", "{{企业满意度}}", "{{校企合作企业数量}}" },
    conflicts = {},
    analysis_focus = trim(analysis_focus),
  }, nil
end

local function valid_score(value)
  if type(value) ~= "number" then return nil end
  if value < 0 or value > 100 then return nil end
  return math.floor(value * 10 + 0.5) / 10
end

local function strip_fenced_json(text)
  local fenced = string.match(text, "```%s*[Jj][Ss][Oo][Nn]%s*\n(.-)\n%s*```")
  if fenced ~= nil then return trim(fenced) end
  fenced = string.match(text, "```%s*\n(.-)\n%s*```")
  if fenced ~= nil then return trim(fenced) end
  return nil
end

local function extract_balanced_json_object(text)
  local start_pos = string.find(text, "{", 1, true)
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
      elseif ch == "{" then
        depth = depth + 1
      elseif ch == "}" then
        depth = depth - 1
        if depth == 0 then
          return trim(string.sub(text, start_pos, pos))
        end
      end
    end
  end
  return nil
end

local function decode_evaluation_json(text)
  local decoded, err = json.decode(text)
  if err == nil and decoded ~= nil then return decoded, nil end

  local fenced = strip_fenced_json(text)
  if fenced ~= nil then
    decoded, err = json.decode(fenced)
    if err == nil and decoded ~= nil then return decoded, nil end
  end

  local object_text = extract_balanced_json_object(text)
  if object_text ~= nil then
    decoded, err = json.decode(object_text)
    if err == nil and decoded ~= nil then return decoded, nil end
  end

  return nil, err
end

local function rating_for_score(total)
  if total == nil then return "{{综合评级}}" end
  if total >= 90 then return "A+ 卓越" end
  if total >= 80 then return "A 优秀" end
  if total >= 70 then return "B 良好" end
  if total >= 60 then return "C 达标" end
  return "D 待提升"
end

local function extract_reasoning_text(raw_text)
  local text = trim(raw_text)
  if text == "" then return "" end
  -- 找到 JSON 起始位置，之前的文本作为分析说明
  local json_start = string.find(text, "{", 1, true)
  if json_start == nil or json_start <= 1 then return "" end
  local prefix = trim(string.sub(text, 1, json_start - 1))
  -- 超过 200 字截断
  if string.len(prefix) > 600 then
    prefix = utf8_safe_prefix(prefix, 600) .. "…"
  end
  return prefix
end

local function validate_evaluation(raw_text)
  local text = trim(raw_text)
  if text == "" then
    return nil, "未返回内容"
  end
  local decoded, err = decode_evaluation_json(text)
  if err ~= nil or decoded == nil then
    return nil, "返回格式不是有效 JSON"
  end
  if decoded.eligible ~= true then
    return nil, "未返回 eligible=true 的评估结果"
  end
  if decoded.dimensions == nil then
    return nil, "缺少 dimensions 字段"
  end
  local by_name = {}
  local count = 0
  for _, dim in ipairs(decoded.dimensions) do
    if dim ~= nil and dim.name ~= nil then
      by_name[dim.name] = dim
      count = count + 1
    end
  end
  if count ~= #DIMENSIONS then
    return nil, "维度数量不完整"
  end
  for _, name in ipairs(DIMENSIONS) do
    if by_name[name] == nil then
      return nil, "维度顺序或名称不符合要求"
    end
    local raw_score = by_name[name].score
    if raw_score ~= nil and valid_score(raw_score) == nil then
      return nil, "维度分数超出允许范围"
    end
    by_name[name].score = valid_score(raw_score)
  end
  decoded.dimensions = {
    by_name[DIMENSIONS[1]],
    by_name[DIMENSIONS[2]],
    by_name[DIMENSIONS[3]],
    by_name[DIMENSIONS[4]],
  }
  return decoded, nil
end

local function aggregate(evaluations)
  local dimensions = {}
  for idx, name in ipairs(DIMENSIONS) do
    local sum = 0
    local count = 0
    local confidence = "low"
    for _, evaluation in ipairs(evaluations) do
      local score = evaluation.dimensions[idx].score
      if score ~= nil then
        sum = sum + score
        count = count + 1
        local dc = evaluation.dimensions[idx].data_confidence
        if dc == "high" then confidence = "high" elseif dc == "medium" and confidence ~= "high" then confidence = "medium" end
      end
    end
    local score = nil
    if count > 0 then score = math.floor((sum / count) * 10 + 0.5) / 10 end
    table.insert(dimensions, { name = name, weight = 25, score = score, data_confidence = confidence })
  end
  local total_sum = 0
  local total_count = 0
  for _, dim in ipairs(dimensions) do
    if dim.score ~= nil then
      total_sum = total_sum + dim.score
      total_count = total_count + 1
    end
  end
  if total_count == 0 then return dimensions, nil, "{{综合评级}}" end
  local total = math.floor((total_sum / total_count) * 10 + 0.5) / 10
  return dimensions, total, rating_for_score(total)
end

local function computed_for_evaluation(evaluation)
  local dimensions = {}
  local total_sum = 0
  local total_count = 0
  for idx, name in ipairs(DIMENSIONS) do
    local source_dim = evaluation.dimensions[idx] or {}
    local score = valid_score(source_dim.score)
    if score ~= nil then
      total_sum = total_sum + score
      total_count = total_count + 1
    end
    table.insert(dimensions, {
      name = name,
      weight = 25,
      score = score,
      data_confidence = source_dim.data_confidence or "low",
    })
  end
  local total = nil
  if total_count > 0 then
    total = math.floor((total_sum / total_count) * 10 + 0.5) / 10
  end
  return {
    model_label = evaluation.model_label,
    dimensions = dimensions,
    total_score = total,
    rating = rating_for_score(total),
  }
end

local function per_model_computed(evaluations)
  local out = {}
  for _, evaluation in ipairs(evaluations) do
    table.insert(out, computed_for_evaluation(evaluation))
  end
  return out
end

local function score_text(value)
  if value == nil then return "{{得分}}" end
  return string.format("%.1f", value)
end

local function collect_evaluation_items(evaluations, field)
  local out = {}
  for _, evaluation in ipairs(evaluations or {}) do
    local items = evaluation[field]
    if items ~= nil then
      for _, item in ipairs(items) do
        table.insert(out, item)
      end
    end
  end
  return out
end

local function collect_missing_placeholders(evaluations, fact_pack)
  local seen = {}
  local out = {}
  local function add(value)
    local text = trim(value)
    if text ~= "" and seen[text] == nil then
      seen[text] = true
      table.insert(out, text)
    end
  end
  for _, item in ipairs(fact_pack.missing or {}) do add(item) end
  for _, evaluation in ipairs(evaluations or {}) do
    for _, item in ipairs(evaluation.missing_placeholders or {}) do add(item) end
    for _, item in ipairs(evaluation.missing or {}) do add(item) end
  end
  return out
end

local function source_label(fact_pack, source_id)
  for _, source in ipairs(fact_pack.sources or {}) do
    if source.id == source_id then return trim(source.label) end
  end
  return trim(source_id)
end

local function first_source_text(fact_pack)
  local source = (fact_pack.sources or {})[1]
  if source == nil then return "公开资料" end
  return trim(source.label)
end

local function computed_dimension(computed, idx, name)
  local dim = (computed.dimensions or {})[idx]
  if dim ~= nil then return dim end
  return { name = name, weight = 25, score = nil, data_confidence = "low" }
end

local function compact_fact_by_keywords(fact_pack, keywords, fallback)
  for _, fact in ipairs(fact_pack.facts or {}) do
    local claim = trim(fact.claim)
    if claim ~= "" and contains_any(claim, keywords) then
      return utf8_safe_prefix(claim, 180)
    end
  end
  return fallback
end

local function insert_source_table(lines, honors, fact_pack)
  table.insert(lines, "| 荣誉 / 排名 | 级别 / 来源 |")
  table.insert(lines, "|---|---|")
  local count = 0
  for _, honor in ipairs(honors or {}) do
    local text = trim(honor.text or honor.name or honor.title)
    if text ~= "" then
      local source = trim(honor.source_id)
      if source ~= "" then source = source_label(fact_pack, source) else source = "公开资料" end
      table.insert(lines, "| " .. text .. " | " .. source .. " |")
      count = count + 1
      if count >= 5 then break end
    end
  end
  if count == 0 then
    table.insert(lines, "| 双高/高职相关公开信息 | " .. first_source_text(fact_pack) .. " |")
    table.insert(lines, "| 产教融合、校企合作或专业群建设线索 | 公开资料待复核 |")
  end
end

local function insert_highlights(lines, highlights, fact_pack)
  if highlights ~= nil and #highlights > 0 then
    local count = 0
    for _, item in ipairs(highlights) do
      local name = trim(item.name or item.title)
      local basis = trim(item.basis or item.text)
      if name ~= "" or basis ~= "" then
        if name == "" then name = "产教融合亮点" end
        if basis == "" then basis = "公开资料显示学校具备相关建设基础，需结合最新正式材料复核。" end
        table.insert(lines, "**" .. name .. "**：" .. basis)
        table.insert(lines, "")
        count = count + 1
        if count >= 4 then break end
      end
    end
    if count >= 3 then return end
  end
  table.insert(lines, "**公开资料基础**：" .. compact_fact_by_keywords(fact_pack, { "产教融合", "校企", "专业群", "实训" }, "已检索到学校在产教融合、校企合作和专业建设方面的公开线索，具备继续开展评估的资料基础。"))
  table.insert(lines, "")
  table.insert(lines, "**协同育人导向**：从已检索资料看，学校建设重点与职业教育服务产业需求、技术技能人才培养方向一致。")
  table.insert(lines, "")
  table.insert(lines, "**复核价值较高**：后续若补充质量年报、就业质量报告和企业合作清单，可进一步细化各维度得分和改进路径。")
  table.insert(lines, "")
end

local function insert_improvements(lines, improvements, missing)
  local count = 0
  for _, item in ipairs(improvements or {}) do
    local text = trim(item.text or item.name)
    local dimension = trim(item.dimension)
    if text ~= "" then
      count = count + 1
      local prefix = tostring(count) .. ". "
      if dimension ~= "" then prefix = prefix .. "**" .. dimension .. "**：" end
      table.insert(lines, prefix .. text)
      table.insert(lines, "")
      if count >= 4 then break end
    end
  end
  if count == 0 then
    table.insert(lines, "1. **数据披露完整性有待加强**：建议补充 " .. table.concat(missing or { "{{就业率}}", "{{企业满意度}}" }, "、") .. " 等关键指标，提升人才培养质量维度的证据置信度。")
    table.insert(lines, "")
    table.insert(lines, "2. **产教融合成果需持续量化**：建议进一步公开校企共建平台、企业投入、横向科研、技术服务和学生参与项目等量化数据，便于形成可复核的年度比较。")
    table.insert(lines, "")
  elseif count == 1 then
    table.insert(lines, "2. **复核关键指标**：建议补充就业质量、企业满意度、校企合作投入和社会服务成效等数据，完善报告证据链。")
    table.insert(lines, "")
  end
end

local function failure_reason_text(reason)
  if reason == nil or trim(reason) == "" then
    return "未返回有效评估结果"
  end
  local raw_reason = trim(reason)
  local lower_reason = string.lower(raw_reason)
  if string.find(lower_reason, "timeout", 1, true) or string.find(lower_reason, "超时", 1, true) then
    return "模型调用超时：" .. raw_reason
  end
  if string.find(lower_reason, "json", 1, true) or string.find(lower_reason, "格式", 1, true) then
    return "返回内容格式不符合要求：" .. raw_reason
  end
  if string.find(lower_reason, "eligible", 1, true) then
    return "未能确认院校资格：" .. raw_reason
  end
  return raw_reason
end

local function format_failures(failures)
  if failures == nil or #failures == 0 then
    return "所有评估模型均未成功返回有效结果，请重试或更换模型。"
  end
  local lines = {
    "所有评估模型均未成功返回有效结果。",
    "",
    "失败摘要：",
  }
  for _, failure in ipairs(failures) do
    local label = model_label(failure.model)
    table.insert(lines, "- " .. label .. "：" .. failure_reason_text(failure.error))
  end
  table.insert(lines, "")
  table.insert(lines, "请重试或更换模型。")
  return table.concat(lines, "\n")
end

local function append_failure_summary_lines(lines, failures)
  table.insert(lines, "## 评估模型未返回有效评分")
  table.insert(lines, "")
  table.insert(lines, "所有评估模型均未成功返回有效结果，本次不计算产教融合指数得分。")
  table.insert(lines, "")
  table.insert(lines, "### 失败摘要")
  table.insert(lines, "")
  if failures == nil or #failures == 0 then
    table.insert(lines, "- 未收到评估模型返回。")
  else
    for _, failure in ipairs(failures) do
      table.insert(lines, "- " .. model_label(failure.model) .. "：" .. failure_reason_text(failure.error))
    end
  end
  table.insert(lines, "")
end

local function spawn_evaluators(school_name, year, mode, models, fact_pack)
  local children = {}
  for _, model in ipairs(models) do
    local input = {
      task = "evaluate",
      model_label = model_label(model),
      model_selector = model_selector(model),
      school_name = school_name,
      year = year,
      mode = mode,
      analysis_focus = fact_pack.analysis_focus or "",
      fact_pack = fact_pack,
    }
    local spawn_args = {
      persona_id = EVALUATOR_PERSONA_ID,
      context_mode = "isolated",
      profile = "task",
      nickname = model_label(model),
      input = json.encode(input),
    }
    local selected_model = model_selector(model)
    if trim(selected_model) ~= "" then
      spawn_args.model = selected_model
    end
    local child, err = agent.spawn(spawn_args)
    if child ~= nil then
      table.insert(children, { child = child, model = model })
    end
  end
  return children
end

local function wait_for_evaluators(children, single_model)
  local evaluations = {}
  local failures = {}
  local total = #children
  local pending = {}
  local by_id = {}
  local deadline_ms = nil
  local slices_left = WAIT_SLICE_LIMIT
  local wait_started_total_ms = nil
  if type(context.now_ms) == "function" then
    local now = context.now_ms()
    if type(now) == "number" and now > 0 then
      wait_started_total_ms = now
      deadline_ms = math.floor(now + WAIT_MS)
    end
  end
  for _, item in ipairs(children) do
    table.insert(pending, item.child.id)
    by_id[item.child.id] = item
  end

  local function remaining_wait_timeout()
    if deadline_ms == nil then
      if slices_left <= 0 then return 1 end
      return WAIT_SLICE_MS
    end
    if type(context.now_ms) ~= "function" then return WAIT_MS end
    local now = context.now_ms()
    if type(now) ~= "number" then return WAIT_MS end
    local remaining = math.floor(deadline_ms - now)
    if remaining <= 1 then return 1 end
    if remaining > WAIT_MS then return WAIT_MS end
    return remaining
  end

  local function elapsed_wait_seconds()
    if deadline_ms ~= nil and type(context.now_ms) == "function" then
      local now = context.now_ms()
      if type(now) == "number" and wait_started_total_ms ~= nil then
        local elapsed = now - wait_started_total_ms
        if elapsed < 0 then elapsed = 0 end
        return math.floor(elapsed / 1000)
      end
    end
    local used_slices = WAIT_SLICE_LIMIT - slices_left
    if used_slices < 0 then used_slices = 0 end
    return math.floor((used_slices * WAIT_SLICE_MS) / 1000)
  end

  local function wait_progress_text(base)
    local elapsed = elapsed_wait_seconds()
    local total_seconds = math.floor(WAIT_MS / 1000)
    if elapsed > total_seconds then elapsed = total_seconds end
    return base .. "（已等待" .. tostring(elapsed) .. "秒/" .. tostring(total_seconds) .. "秒）"
  end

  local function wait_tool_description(base, wait_timeout)
    local seconds = math.floor(((wait_timeout or WAIT_SLICE_MS) + 999) / 1000)
    if seconds < 1 then seconds = 1 end
    return wait_progress_text(base) .. "；每" .. tostring(seconds) .. "秒检查一次"
  end

  local function pending_labels(wait_ids)
    local labels = {}
    for _, id in ipairs(wait_ids) do
      local item = by_id[id]
      if item ~= nil then table.insert(labels, progress_model_label(item.model)) end
    end
    if #labels == 0 then return "评估模型" end
    return table.concat(labels, "、")
  end

  local function interrupt_pending(wait_ids, reason)
    if type(agent.interrupt) ~= "function" then return end
    for _, id in ipairs(wait_ids) do
      local _, _ = agent.interrupt(id, reason)
    end
  end

  local function is_timeout_wait_error(err)
    local text = string.lower(trim(tostring(err or "")))
    if text == "" then return false end
    if string.find(text, "deadline exceeded", 1, true) then return true end
    if string.find(text, "context deadline exceeded", 1, true) then return true end
    if string.find(text, "timeout", 1, true) then return true end
    if string.find(text, "超时", 1, true) then return true end
    return false
  end

  while #pending > 0 do
    slices_left = slices_left - 1
    local wait_ids = {}
    for _, id in ipairs(pending) do
      table.insert(wait_ids, id)
    end
    local first_item = by_id[wait_ids[1]]
    local label = progress_model_label(first_item.model)
    local resolved = nil
    local wait_err = nil
    local remaining_timeout = remaining_wait_timeout()
    if remaining_timeout <= 1 then
      interrupt_pending(wait_ids, "评估等待超时（8分钟）")
      for _, id in ipairs(wait_ids) do
        local pending_item = by_id[id]
        if pending_item ~= nil then
          table.insert(failures, { model = pending_item.model, error = "执行失败或超时：等待8分钟未完成；评估模型未在等待时间内返回结果" })
        end
      end
      pending = {}
      by_id = {}
      break
    end
    local wait_timeout = remaining_timeout
    if wait_timeout > WAIT_SLICE_MS then wait_timeout = WAIT_SLICE_MS end
    local wait_slice_started_ms = nil
    if type(context.now_ms) == "function" then
      local now = context.now_ms()
      if type(now) == "number" then wait_slice_started_ms = now end
    end
    if single_model or #wait_ids == 1 or type(agent.wait_any) ~= "function" then
      local description = wait_tool_description("等待 " .. label .. " 返回评估结果（最长8分钟）", wait_timeout)
      set_progress_step("evaluate", "in_progress", "调用 " .. tostring(total) .. " 个评估模型", description)
      resolved, wait_err = agent.wait(wait_ids[1], wait_timeout, { display_description = description })
    else
      local description = wait_tool_description("并行等待 " .. tostring(#wait_ids) .. " 个评估模型返回结果（" .. pending_labels(wait_ids) .. "，最长8分钟）", wait_timeout)
      set_progress_step("evaluate", "in_progress", "调用 " .. tostring(total) .. " 个评估模型", description)
      resolved, wait_err = agent.wait_any(wait_ids, wait_timeout, { display_description = description })
    end

    local should_continue_wait = false
    if wait_err ~= nil and is_timeout_wait_error(wait_err) and resolved == nil then
      local still_remaining = remaining_wait_timeout()
      local waited_long_enough = true
      if wait_slice_started_ms ~= nil and type(context.now_ms) == "function" then
        local now = context.now_ms()
        if type(now) == "number" then
          local elapsed = now - wait_slice_started_ms
          waited_long_enough = (elapsed + 200) >= wait_timeout
        end
      end
      if still_remaining > 1 and slices_left > 0 and waited_long_enough then
        set_progress_step("evaluate", "in_progress", "调用 " .. tostring(total) .. " 个评估模型", wait_progress_text("模型仍在运行，继续等待（最长8分钟）"))
        should_continue_wait = true
      end
    end
    if should_continue_wait then
      -- Keep pending children and continue slicing wait windows until deadline.
    else
      if wait_err ~= nil and not single_model and #wait_ids > 1 and resolved == nil then
        if is_timeout_wait_error(wait_err) then
          interrupt_pending(wait_ids, "评估等待超时或失败，已终止剩余模型")
        end
        for _, id in ipairs(wait_ids) do
          local pending_item = by_id[id]
          if pending_item ~= nil then
            local pending_label = progress_model_label(pending_item.model)
            local pending_resolved, pending_wait_err = agent.wait(id, 1, { display_description = "检查 " .. pending_label .. " 是否已有可用评估结果" })
            local pending_error = "执行失败或超时：" .. tostring(wait_err)
            if pending_wait_err ~= nil then
              pending_error = "执行失败或超时：" .. tostring(pending_wait_err)
            elseif pending_resolved == nil then
              pending_error = "未返回内容"
            elseif pending_resolved.last_error ~= nil and trim(pending_resolved.last_error) ~= "" then
              pending_error = trim(pending_resolved.last_error)
            elseif pending_resolved.output == nil then
              pending_error = "未返回内容"
            else
              local pending_evaluation, pending_reason = validate_evaluation(pending_resolved.output)
              if pending_evaluation ~= nil then
                pending_evaluation.model_label = pending_evaluation.model_label or model_label(pending_item.model)
                table.insert(evaluations, pending_evaluation)
                set_progress_step("evaluate", "in_progress", "调用 " .. tostring(total) .. " 个评估模型", pending_label .. " 评估完成（" .. tostring(#evaluations) .. "/" .. tostring(total) .. "）")
                pending_error = nil
              else
                pending_error = pending_reason or "返回内容不符合评估格式"
              end
            end
            if pending_error ~= nil then
              table.insert(failures, { model = pending_item.model, error = pending_error })
              set_progress_step("evaluate", "in_progress", "调用 " .. tostring(total) .. " 个评估模型", pending_label .. " 评估失败，继续处理可用结果")
            end
          end
        end
        pending = {}
        by_id = {}
        break
      end

      local item = nil
      if resolved ~= nil and resolved.id ~= nil and by_id[resolved.id] ~= nil then
        item = by_id[resolved.id]
      else
        item = first_item
      end
      label = progress_model_label(item.model)

      local next_pending = {}
      for _, id in ipairs(pending) do
        if id ~= item.child.id then
          table.insert(next_pending, id)
        end
      end
      pending = next_pending
      by_id[item.child.id] = nil

      if wait_err ~= nil then
        table.insert(failures, { model = item.model, error = "执行失败或超时：" .. tostring(wait_err) })
        set_progress_step("evaluate", "in_progress", "调用 " .. tostring(total) .. " 个评估模型", label .. " 评估失败，继续处理可用结果")
        if single_model then return evaluations, failures end
      elseif resolved == nil then
        table.insert(failures, { model = item.model, error = "未返回内容" })
        set_progress_step("evaluate", "in_progress", "调用 " .. tostring(total) .. " 个评估模型", label .. " 未返回内容，继续处理可用结果")
        if single_model then return evaluations, failures end
      elseif resolved.last_error ~= nil and trim(resolved.last_error) ~= "" then
        table.insert(failures, { model = item.model, error = trim(resolved.last_error) })
        set_progress_step("evaluate", "in_progress", "调用 " .. tostring(total) .. " 个评估模型", label .. " 评估失败，继续处理可用结果")
        if single_model then return evaluations, failures end
      elseif resolved.output == nil then
        table.insert(failures, { model = item.model, error = "未返回内容" })
        set_progress_step("evaluate", "in_progress", "调用 " .. tostring(total) .. " 个评估模型", label .. " 未返回内容，继续处理可用结果")
        if single_model then return evaluations, failures end
      else
        local evaluation, reason = validate_evaluation(resolved.output)
        if evaluation == nil then
          table.insert(failures, { model = item.model, error = reason or "返回内容不符合评估格式" })
          set_progress_step("evaluate", "in_progress", "调用 " .. tostring(total) .. " 个评估模型", label .. " 返回格式需复核，继续处理可用结果")
          if single_model then return evaluations, failures end
        else
          evaluation.model_label = evaluation.model_label or model_label(item.model)
          table.insert(evaluations, evaluation)
          set_progress_step("evaluate", "in_progress", "调用 " .. tostring(total) .. " 个评估模型", label .. " 评估完成（" .. tostring(#evaluations) .. "/" .. tostring(total) .. "）")
        end
      end
    end
  end
  local completed = #evaluations
  if completed > 0 then
    set_progress_step("evaluate", "completed", "已完成 " .. tostring(completed) .. "/" .. tostring(total) .. " 个评估模型", "评估结果已返回，准备综合评分")
  else
    set_progress_step("evaluate", "completed", "评估模型未返回有效评分", "将生成诊断报告")
  end
  return evaluations, failures
end

local function fallback_report(school_name, year, computed, fact_pack, evaluations)
  local total = computed.total_score or "{{产教融合指数得分}}"
  local dim1 = computed_dimension(computed, 1, DIMENSIONS[1])
  local dim2 = computed_dimension(computed, 2, DIMENSIONS[2])
  local dim3 = computed_dimension(computed, 3, DIMENSIONS[3])
  local dim4 = computed_dimension(computed, 4, DIMENSIONS[4])
  local honors = collect_evaluation_items(evaluations, "honors")
  local highlights = collect_evaluation_items(evaluations, "highlights")
  local improvements = collect_evaluation_items(evaluations, "improvements")
  local missing = collect_missing_placeholders(evaluations, fact_pack)
  local lines = {
    "# " .. school_name .. " · 产教融合指数报告（" .. year .. "年）",
    "",
    "---",
    "",
    "> **综合评级：" .. tostring(computed.rating) .. "**",
    ">",
    "> **产教融合指数得分：" .. score_text(total) .. " / 100**",
    "",
    "---",
    "",
    "## 一、基础与机制（权重 25%，得分：" .. score_text(dim1.score) .. "）",
    "",
    "**制度体系**：" .. compact_fact_by_keywords(fact_pack, { "制度", "机制", "治理", "章程" }, "公开资料显示学校已围绕职业教育改革、专业建设和产教融合开展制度化建设，具体制度文本仍需以学校最新公开材料复核。"),
    "",
    "**治理架构**：建议重点复核学校、二级学院、行业企业共同参与的理事会、专业建设委员会或产业学院治理机制。当前证据置信度为 " .. tostring(dim1.data_confidence or "low") .. "。",
    "",
    "**核心平台**：已检索到的公开资料可作为识别产教融合共同体、产业学院、实训基地和专业群平台的基础；未披露项目保留为占位符。",
    "",
    "**协同机制**：从公开线索看，校企协同育人和专业建设是后续复核重点，建议补充年度合作清单、会议机制和履约成效数据。",
    "",
    "## 二、资源共建共享（权重 25%，得分：" .. score_text(dim2.score) .. "）",
    "",
    "**实训条件**：" .. compact_fact_by_keywords(fact_pack, { "实训", "基地", "教学工厂", "实践" }, "公开资料暂未完整披露实训基地数量、等级和共享范围，需结合学校质量年报进一步核验。"),
    "",
    "**企业投入**：建议补充企业设备、资金、技术、真实项目和岗位资源投入情况；若缺少量化数据，应降低本维度证据置信度。",
    "",
    "**双师队伍**：建议复核双师型教师比例、产业教授、企业导师和教师企业实践等指标，以判断资源共建是否形成常态化能力。",
    "",
    "**资源共享机制**：重点关注校企共投共管、课程资源共建、开放实训平台和校外实践基地的覆盖面。",
    "",
    "## 三、产学建设与服务（权重 25%，得分：" .. score_text(dim3.score) .. "）",
    "",
    "**人才培养**：" .. compact_fact_by_keywords(fact_pack, { "人才培养", "订单", "学徒", "1+X", "课程" }, "公开资料显示学校围绕技术技能人才培养开展建设，现代学徒制、订单班和课证融通等细项仍需补充。"),
    "",
    "**科研与转化**：建议补充横向科研经费、专利转化、技术服务项目和校企联合攻关平台数据，以提高产学建设维度的可比性。",
    "",
    "**社会服务**：关注学校服务地方产业、职业培训、技术推广和乡村振兴等公开成果，避免仅以宣传性表述替代可核验证据。",
    "",
    "**产教共同体**：如学校牵头或参与行业产教融合共同体、区域产教联合体，应列明平台名称、角色和年度运行成效。",
    "",
    "## 四、人才培养质量（权重 25%，得分：" .. score_text(dim4.score) .. "）",
    "",
    "**就业质量**：当前应重点补充 " .. table.concat((#missing > 0 and missing or { "{{就业率}}", "{{对口就业率}}", "{{毕业生薪资}}" }), "、") .. " 等指标；缺失数据会直接影响本维度置信度。",
    "",
    "**竞赛成果**：建议核验职业院校技能大赛、创新创业大赛、省级以上竞赛奖项和学生参与科研项目等成果。",
    "",
    "**企业满意度**：若公开资料未披露企业满意度，应保留 `{{企业满意度}}` 占位符，并在复核说明中提示需要问卷或第三方调研数据支撑。",
    "",
    "**质量保障**：建议结合毕业生跟踪调查、用人单位反馈、专业认证和质量年报，形成对人才培养结果的闭环评价。",
    "",
    "## 五、核心荣誉与排名",
    "",
  }
  insert_source_table(lines, honors, fact_pack)
  table.insert(lines, "")
  table.insert(lines, "## 六、优势亮点")
  table.insert(lines, "")
  insert_highlights(lines, highlights, fact_pack)
  table.insert(lines, "## 七、提升方向")
  table.insert(lines, "")
  insert_improvements(lines, improvements, missing)
  table.insert(lines, "")
  table.insert(lines, "## 数据来源")
  table.insert(lines, "")
  for _, source in ipairs(fact_pack.sources or {}) do
    table.insert(lines, "- [" .. trim(source.label) .. "](" .. trim(source.url) .. ")")
  end
  table.insert(lines, "")
  table.insert(lines, "## 数据局限性声明")
  table.insert(lines, "")
  table.insert(lines, "本报告中的评分是基于公开可检索资料、来源证据和模型评估形成的证据型估计，不等同于教育主管部门或第三方认证机构的官方评价。公开资料不足、指标缺失或口径差异会影响部分维度的评分置信度。")
  table.insert(lines, "")
  table.insert(lines, "## 复核说明")
  table.insert(lines, "")
  table.insert(lines, "涉及政策认定、荣誉资格、项目名单和关键统计指标时，应以教育主管部门、学校官网或正式材料的最新公示为准。")
  return table.concat(lines, "\n")
end

local function diagnostic_report(school_name, year, failures, fact_pack)
  local lines = {
    "# " .. school_name .. " · 产教融合指数报告（" .. year .. "年）",
    "",
  }
  append_failure_summary_lines(lines, failures)
  table.insert(lines, "## 已核验公开来源")
  table.insert(lines, "")
  for _, source in ipairs(fact_pack.sources or {}) do
    table.insert(lines, "- [" .. trim(source.label) .. "](" .. trim(source.url) .. ")")
  end
  if #((fact_pack.sources or {})) == 0 then
    table.insert(lines, "- 未记录可展示来源。")
  end
  table.insert(lines, "")
  table.insert(lines, "## 已提取事实线索")
  table.insert(lines, "")
  for _, fact in ipairs(fact_pack.facts or {}) do
    local source_ids = ""
    if fact.source_ids ~= nil and #fact.source_ids > 0 then
      source_ids = "（来源：" .. table.concat(fact.source_ids, "、") .. "）"
    end
    table.insert(lines, "- " .. utf8_safe_prefix(trim(fact.claim), 260) .. source_ids)
  end
  if #((fact_pack.facts or {})) == 0 then
    table.insert(lines, "- 未提取到可展示事实。")
  end
  table.insert(lines, "")
  table.insert(lines, "## 下一步建议")
  table.insert(lines, "")
  table.insert(lines, "- 更换或减少评估模型后重试，优先选择响应稳定的文本模型。")
  table.insert(lines, "- 如果模型返回了说明文字而不是 JSON，可重试；系统会自动兼容 Markdown JSON 代码块，但仍需要完整四个维度。")
  table.insert(lines, "- 如持续超时，请检查模型供应商限流、网络连接和运行超时配置。")
  table.insert(lines, "")
  table.insert(lines, "## 数据局限性声明")
  table.insert(lines, "")
  table.insert(lines, "本报告仅保留已检索公开来源和失败诊断，未形成有效评分，不应作为正式评价结果。")
  return table.concat(lines, "\n")
end

local function first_artifact(result)
  if result == nil or result.artifacts == nil or #result.artifacts == 0 then return nil end
  return result.artifacts[1]
end

local function default_artifact_key(filename)
  local run_id = trim(context.get("run_id"))
  if run_id == "" then return "" end
  local account_id = trim(context.get("account_id"))
  if account_id == "" then account_id = "_anonymous" end
  return account_id .. "/" .. run_id .. "/" .. filename
end

local function artifact_link(result, fallback_filename)
  local artifact = first_artifact(result)
  local filename = fallback_filename
  if artifact ~= nil and trim(artifact.filename) ~= "" then
    filename = trim(artifact.filename)
  end
  if trim(filename) == "" then filename = "artifact" end
  if artifact ~= nil and trim(artifact.key) ~= "" then
    return "[" .. filename .. "](artifact:" .. trim(artifact.key) .. ")"
  end
  local synthesized = default_artifact_key(filename)
  if synthesized ~= "" then
    return "[" .. filename .. "](artifact:" .. synthesized .. ")"
  end
  return filename
end

local function write_outputs(school_name, year, markdown, status_line)
  set_progress_step("report", "in_progress", "生成报告文件", "正在写入 Markdown 文件")
  local base = school_name .. "_产教融合指数报告_" .. year
  local md_name = base .. ".md"
  local pdf_name = base .. ".pdf"
  local md_result, md_err = tool_call("document_write", {
    filename = md_name,
    content = markdown,
  })
  if md_err ~= nil then
    set_progress_step("report", "completed", "报告文件生成异常", "Markdown 文件保存暂未成功，可稍后重试")
  else
    set_progress_step("report", "completed", "报告文件已生成", "Markdown 文件已生成，准备转化 PDF")
  end
  set_progress_step("pdf", "in_progress", "转化为 PDF 文件", "Markdown 已处理，正在转化为 PDF")
  local pdf_result, pdf_err = tool_call("markdown_to_pdf", {
    filename = pdf_name,
    content = markdown,
  })
  local suffix = "\n\n---\n\n生成文件：" .. artifact_link(md_result, md_name)
  if pdf_err == nil and pdf_result ~= nil then
    suffix = suffix .. "、" .. artifact_link(pdf_result, pdf_name)
    set_progress_step("pdf", "completed", "PDF 文件已生成", "PDF 文件已转化完成")
  else
    suffix = suffix .. "\n\nPDF 导出暂未成功，已保留 Markdown 文件，可稍后重试导出。"
    set_progress_step("pdf", "completed", "PDF 文件生成异常", "PDF 转化暂未成功，已保留 Markdown 文件")
  end
  if md_err ~= nil then
    suffix = suffix .. "\n\n报告文件保存暂未成功，可稍后重试。"
  end
  local final_status = status_line
  if trim(final_status) == "" then
    final_status = "评估完成：已生成《" .. school_name .. " 产教融合指数报告（" .. year .. "年）》及相关文件。"
  end
  context.set_output(final_status .. "\n\n" .. markdown .. suffix)
end

local initial_text = latest_user_text()
local initial_school = extract_school_name(initial_text)
local school_name = initial_school
local year = "2026"
local config = choose_mode()
local mode = trim(config.mode)
if mode == "" then mode = "多模型评估" end
if mode == "分别评估" then mode = "多模型评估" end
local analysis_focus = ""

if school_name == "" then
  context.set_output("评估未完成：需要先提供院校名称，才能生成双高/高职产教融合指数报告。")
  return
end

local selected_models = choose_models(mode)
if selected_models == nil or #selected_models == 0 then
  selected_models = { "" }
end
if mode == "单模型评估" and #selected_models > 1 then
  selected_models = { selected_models[1] }
end
init_progress_todos(#selected_models)

local fact_pack, fact_err = search_sources(school_name, year, analysis_focus)
if fact_err ~= nil or fact_pack == nil then
  set_progress_step("sources", "completed", "公开资料检索未完成", fact_err or "未能完成公开资料搜索")
  context.set_output("评估未完成：" .. (fact_err or "未能完成公开资料搜索，暂不生成报告。"))
  return
end
if fact_pack.eligibility == nil or fact_pack.eligibility.verified ~= true then
  set_progress_step("sources", "completed", "公开资料已检索", "未能确认院校类型，停止生成报告")
  context.set_output("评估未完成：未能从公开来源确认该院校属于双高学院或高职院校，暂不生成产教融合指数报告。")
  return
end
set_progress_step("sources", "completed", "公开资料已检索", "已核验公开来源并提取事实线索")

local children = spawn_evaluators(school_name, year, mode, selected_models, fact_pack)
if #children == 0 then
  set_progress_step("evaluate", "completed", "未能启动评估模型", "请检查模型配置后重试")
  context.set_output("评估未完成：未能启动评估子智能体，请检查模型配置后重试。")
  return
end

local evaluations, failures = wait_for_evaluators(children, mode == "单模型评估")
if #evaluations == 0 then
  set_progress_step("score", "completed", "综合评分", "没有可用评分，将生成诊断报告")
  local diagnostic = diagnostic_report(school_name, year, failures, fact_pack)
  write_outputs(school_name, year, diagnostic, "评估未完成：所有评估模型均未返回有效评分，已生成诊断报告。")
  return
end

set_progress_step("score", "in_progress", "综合评分", "正在合并各模型维度评分和证据置信度")
local dimensions, total_score, rating = aggregate(evaluations)
if total_score == nil then
  set_progress_step("score", "completed", "综合评分", "评估模型未返回可计算的维度分数")
  context.set_output("评估未完成：评估模型未返回可计算的维度分数，请重试或更换模型。")
  return
end
set_progress_step("score", "completed", "综合评分已完成", "产教融合指数得分：" .. score_text(total_score) .. " / 100")

local computed = {
  dimensions = dimensions,
  total_score = total_score,
  rating = rating,
}

set_progress_step("report", "in_progress", "生成报告文件", "正在生成结构化 Markdown 报告")
local report_markdown = fallback_report(school_name, year, computed, fact_pack, evaluations)

write_outputs(school_name, year, report_markdown, "评估完成：已生成《" .. school_name .. " 产教融合指数报告（" .. year .. "年）》及相关文件。")
