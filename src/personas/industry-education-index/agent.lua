local EVALUATOR_PERSONA_ID = "industry-education-evaluator"
local SYNTHESIZER_PERSONA_ID = "industry-education-synthesizer"
local WAIT_MS = 5 * 60 * 1000

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

local function extract_school_name(text)
  for _, keyword in ipairs({ "评估", "分析" }) do
    local found = extract_after_keyword(text, keyword)
    if found ~= "" then return found end
  end
  local after_for = extract_after_keyword(text, "为")
  if after_for ~= "" then
    local found = trim(take_before_plain(after_for, "生成"))
    if found ~= "" then return found end
  end
  local cleaned = trim(text)
  if cleaned ~= "" and string.len(cleaned) <= 80 then
    return cleaned
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
      enum = { "综合评估", "分别评估", "单模型评估" },
      default = "综合评估",
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
  local default_value = {}
  for _, model in ipairs(source_models) do
    table.insert(default_value, model.value)
  end
  if mode == "单模型评估" then
    max_items = 1
    default_value = { source_models[1].value }
  end
  local field = {
    key = "models",
    type = "array",
    title = "评估模型",
    description = (#available > 0) and "已按当前可用路由筛选 DeepSeek / Qwen / Doubao 候选。" or "未读取到可用路由，使用默认推荐候选；未配置的模型会在运行时自动降级为失败项。",
    required = true,
    enum = enum,
    enumNames = enum_names,
    default = default_value,
    minItems = 1,
  }
  if max_items ~= nil then field.maxItems = max_items end
  local response = ask_user({
    message = "请选择用于评估的模型。",
    fields = { field },
  })
  return response.models or default_value
end

local function selector_label(value)
  local left, right = string.match(value, "^([^%^]+)%^(.+)$")
  if left ~= nil and right ~= nil then
    return trim(left) .. " / " .. trim(right)
  end
  return value
end

local function model_label(model_value)
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
    if selector ~= "" and family ~= "" and seen[selector] == nil then
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

local function search_sources(school_name, year, analysis_focus)
  local queries = {
    school_name,
    school_name .. " 官网",
    school_name .. " 学校官网 高职 院校 简介",
    school_name .. " 双高计划 高水平高职学校 专业群",
    school_name .. " 产教融合 校企合作 实训基地 " .. year,
  }
  if trim(analysis_focus) ~= "" then
    queries[#queries] = school_name .. " " .. analysis_focus
  end
  local search_result, search_err = tool_call("web_search", { queries = queries, max_results = 5 })
  if search_err ~= nil or search_result == nil or search_result.results == nil or #search_result.results == 0 then
    return nil, "当前搜索提供商未返回可用于核验院校身份的公开结果。请检查“接入”中的联网搜索配置，建议切换到 Tavily 或 SearXNG 后重试。原始信息：" .. tostring(search_err or "empty_results")
  end

  local sources = {}
  local facts = {}
  local fetch_calls = {}
  for i, hit in ipairs(search_result.results) do
    if i > 6 then break end
    local id = "S" .. tostring(i)
    local label = trim(hit.title)
    if label == "" then label = "公开来源 " .. tostring(i) end
    table.insert(sources, { id = id, label = label, url = trim(hit.url) })
    table.insert(facts, {
      claim = trim(hit.snippet),
      source_ids = { id },
    })
    if hit.url ~= nil and hit.url ~= "" and #fetch_calls < 3 then
      table.insert(fetch_calls, { id = id, url = hit.url })
    end
  end

  for _, item in ipairs(fetch_calls) do
    local fetched = tool_call("web_fetch", { url = item.url, max_length = 2400 })
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

local function rating_for_score(total)
  if total == nil then return "{{综合评级}}" end
  if total >= 90 then return "A+ 卓越" end
  if total >= 80 then return "A 优秀" end
  if total >= 70 then return "B 良好" end
  if total >= 60 then return "C 达标" end
  return "D 待提升"
end

local function validate_evaluation(raw_text)
  local text = trim(raw_text)
  if text == "" then
    return nil, "未返回内容"
  end
  local decoded, err = json.decode(text)
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

local function failure_reason_text(reason)
  if reason == nil or trim(reason) == "" then
    return "未知错误"
  end
  return trim(reason)
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

local function spawn_evaluators(school_name, year, mode, models, fact_pack)
  local children = {}
  for _, model in ipairs(models) do
    local input = {
      task = "evaluate",
      model_label = model_label(model),
      school_name = school_name,
      year = year,
      mode = mode,
      analysis_focus = fact_pack.analysis_focus or "",
      fact_pack = fact_pack,
    }
    local child, err = agent.spawn({
      persona_id = EVALUATOR_PERSONA_ID,
      context_mode = "isolated",
      profile = "task",
      model = model_selector(model),
      nickname = model_label(model),
      input = json.encode(input),
    })
    if child ~= nil then
      table.insert(children, { child = child, model = model })
    end
  end
  return children
end

local function wait_for_evaluators(children, single_model)
  local evaluations = {}
  local failures = {}
  for _, item in ipairs(children) do
    local resolved, wait_err = agent.wait(item.child.id, WAIT_MS)
    if wait_err ~= nil then
      table.insert(failures, { model = item.model, error = "执行失败或超时：" .. tostring(wait_err) })
      if single_model then return evaluations, failures end
    elseif resolved == nil or resolved.output == nil then
      table.insert(failures, { model = item.model, error = "未返回内容" })
      if single_model then return evaluations, failures end
    else
      local evaluation, reason = validate_evaluation(resolved.output)
      if evaluation == nil then
        table.insert(failures, { model = item.model, error = reason or "返回内容不符合评估格式" })
        if single_model then return evaluations, failures end
      else
        evaluation.model_label = evaluation.model_label or model_label(item.model)
        table.insert(evaluations, evaluation)
      end
    end
  end
  return evaluations, failures
end

local function fallback_report(school_name, year, computed, fact_pack)
  local total = computed.total_score or "{{产教融合指数得分}}"
  local lines = {
    "# " .. school_name .. " · 产教融合指数报告（" .. year .. "年）",
    "",
    "## 综合评级与产教融合指数得分",
    "",
    "- 产教融合指数得分：" .. tostring(total) .. "/100.0",
    "- 综合评级：" .. tostring(computed.rating),
    "",
    "## 评分说明",
    "",
    "四个一级维度权重均为 25%，综合分由有效维度得分机械计算。未核验数据保留占位符。",
  }
  for _, dim in ipairs(computed.dimensions) do
    table.insert(lines, "")
    table.insert(lines, "## " .. dim.name)
    table.insert(lines, "")
    table.insert(lines, "- 权重：25%")
    table.insert(lines, "- 得分：" .. tostring(dim.score or "{{" .. dim.name .. "得分}}") .. "/100.0")
    table.insert(lines, "- 证据置信度：" .. tostring(dim.data_confidence or "low"))
  end
  table.insert(lines, "")
  table.insert(lines, "## 数据来源")
  table.insert(lines, "")
  for _, source in ipairs(fact_pack.sources or {}) do
    table.insert(lines, "- [" .. trim(source.label) .. "](" .. trim(source.url) .. ")")
  end
  table.insert(lines, "")
  table.insert(lines, "## 数据局限性声明")
  table.insert(lines, "")
  table.insert(lines, "本报告中的评分是基于公开可检索资料、来源证据和模型评估形成的辅助性估计，不等同于教育主管部门或第三方认证机构的官方评价。")
  table.insert(lines, "")
  table.insert(lines, "## 复核说明")
  table.insert(lines, "")
  table.insert(lines, "涉及政策认定、荣誉资格、项目名单和关键统计指标时，应以教育主管部门、学校官网或正式材料的最新公示为准。")
  return table.concat(lines, "\n")
end

local function write_outputs(school_name, year, markdown)
  local base = school_name .. "_产教融合指数报告_" .. year
  local md_name = base .. ".md"
  local pdf_name = base .. ".pdf"
  local md_result, md_err = tool_call("document_write", {
    filename = md_name,
    content = markdown,
  })
  local pdf_result, pdf_err = tool_call("markdown_to_pdf", {
    title = school_name .. " · 产教融合指数报告（" .. year .. "年）",
    filename = pdf_name,
    template = "formal_report",
    content = markdown,
  })
  local suffix = "\n\n---\n\n生成文件：`" .. md_name .. "`"
  if pdf_err == nil and pdf_result ~= nil then
    suffix = suffix .. "、`" .. pdf_name .. "`"
  else
    suffix = suffix .. "\n\nPDF 导出失败，可稍后重试导出。"
  end
  if md_err ~= nil then
    suffix = suffix .. "\n\nMarkdown artifact 写入失败：" .. tostring(md_err)
  end
  context.set_output(markdown .. suffix)
end

local initial_text = latest_user_text()
local initial_school = extract_school_name(initial_text)
local school_name = initial_school
local year = "2026"
local config = choose_mode()
local mode = trim(config.mode)
if mode == "" then mode = "综合评估" end
local analysis_focus = ""

if school_name == "" then
  context.set_output("需要先提供院校名称，才能生成双高/高职产教融合指数报告。")
  return
end

local selected_models = choose_models(mode)
if selected_models == nil or #selected_models == 0 then
  context.set_output("需要至少选择一个评估模型。")
  return
end
if mode == "单模型评估" and #selected_models > 1 then
  selected_models = { selected_models[1] }
end

local fact_pack, fact_err = search_sources(school_name, year, analysis_focus)
if fact_err ~= nil or fact_pack == nil then
  context.set_output(fact_err or "未能完成公开资料搜索，暂不生成报告。")
  return
end
if fact_pack.eligibility == nil or fact_pack.eligibility.verified ~= true then
  context.set_output("未能从公开来源确认该院校属于双高学院或高职院校，暂不生成产教融合指数报告。")
  return
end

local children = spawn_evaluators(school_name, year, mode, selected_models, fact_pack)
if #children == 0 then
  context.set_output("未能启动评估子智能体，请检查模型配置后重试。")
  return
end

local evaluations, failures = wait_for_evaluators(children, mode == "单模型评估")
if #evaluations == 0 then
  context.set_output(format_failures(failures))
  return
end

local dimensions, total_score, rating = aggregate(evaluations)
if total_score == nil then
  context.set_output("评估模型未返回可计算的维度分数，请重试或更换模型。")
  return
end

local computed = {
  dimensions = dimensions,
  total_score = total_score,
  rating = rating,
}

local template = context.get("report_template") or ""
local synth_input = {
  task = "synthesize",
  school_name = school_name,
  year = year,
  mode = mode,
  fact_pack = fact_pack,
	  computed = computed,
	  per_model_computed = per_model_computed(evaluations),
	  evaluations = evaluations,
  report_template = template,
}

local report_markdown = nil
local synth_model = model_selector(selected_models[1])
local synth, synth_err = agent.spawn({
  persona_id = SYNTHESIZER_PERSONA_ID,
  context_mode = "isolated",
  profile = "task",
  model = synth_model,
  nickname = "报告撰写",
  input = json.encode(synth_input),
})
if synth ~= nil then
  local resolved, wait_err = agent.wait(synth.id, WAIT_MS)
  if wait_err == nil and resolved ~= nil and resolved.output ~= nil and trim(resolved.output) ~= "" then
    report_markdown = trim(resolved.output)
  end
end

if report_markdown == nil then
  report_markdown = fallback_report(school_name, year, computed, fact_pack)
end

write_outputs(school_name, year, report_markdown)
