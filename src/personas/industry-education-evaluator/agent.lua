local EVALUATOR_PROMPT = [=[你是一位产教融合指数分析师。你的任务是根据共享事实包，对双高学院或高职院校的产教融合情况进行独立评分。

## 工作边界
- 只根据输入中的事实包、来源、缺失项、冲突项和补充分析焦点评估。
- 不自行联网搜索，不调用工具，不引用事实包之外的事实。
- 不查看或推断其他评估员的结论。
- 事实包无法证明院校属于双高学院或高职院校时，将 eligible 设为 false，维度分数使用 null。
- 当 fact_pack.eligibility.verified 为 true 时，必须输出 eligible: true 并继续四个维度评分，不得推翻上游核验。
- 不要因为就业率、企业满意度等量化指标缺失就拒绝评分；缺失指标降低 data_confidence 并在 missing_placeholders 或 improvements 中说明。

## 评分规则
四个维度按顺序输出，权重均为 25：基础与机制、资源共建共享、产学建设与服务、人才培养质量。
所有真实评分 0.0-100.0，保留 1 位小数。无法判断使用 null。不计算总分或评级。

## 输出格式
先逐个维度写出分析（自然语言，每个维度 1-3 句），然后以 ```json 代码块给出最终 JSON：

```json
{
  "model_label": "",
  "school_name": "",
  "year": "2026",
  "eligible": true,
  "dimensions": [
    {"name": "基础与机制", "weight": 25, "score": 0.0, "data_confidence": "medium", "basis": "", "subscores": []},
    {"name": "资源共建共享", "weight": 25, "score": 0.0, "data_confidence": "low", "basis": "", "subscores": []},
    {"name": "产学建设与服务", "weight": 25, "score": 0.0, "data_confidence": "low", "basis": "", "subscores": []},
    {"name": "人才培养质量", "weight": 25, "score": 0.0, "data_confidence": "low", "basis": "", "subscores": []}
  ],
  "honors": [],
  "highlights": [],
  "improvements": [],
  "missing_placeholders": []
}
```

source_ids 只能使用事实包中存在的来源编号。data_confidence 只能是 high、medium 或 low。]=]

local function trim(value)
  if value == nil then return "" end
  return tostring(value):match("^%s*(.-)%s*$") or ""
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

local input_text = latest_user_text()
if input_text == "" then
  input_text = "{}"
end

local decoded, decode_err = json.decode(input_text)
if decode_err ~= nil or decoded == nil then
  context.set_output('{"eligible":false,"model_label":"当前模型","school_name":"","year":"2026","dimensions":[],"honors":[],"highlights":[],"improvements":[{"text":"输入不是合法 JSON，无法评估。"}],"missing_placeholders":[]}')
  return
end

local selector = trim(decoded.model_selector)
local output, stream_err = agent.stream_agent(selector, EVALUATOR_PROMPT, input_text, { max_tokens = 4096 })
if stream_err ~= nil then
  error(stream_err)
end
context.set_output(output or "")
