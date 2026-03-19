export type RunEventRaw = {
  event_id: string
  run_id: string
  seq: number
  ts: string
  type: string
  data: Record<string, unknown>
  tool_name?: string
  error_class?: string
}

export type LlmTurn = {
  requestMessages: Array<{
    role: string
    text: string
    meta?: Record<string, string>
  }>
  llmCallId: string
  providerKind: string
  apiMode: string
  inputTokens?: number
  outputTokens?: number
  cachedTokens?: number
  cacheCreationTokens?: number
  payloadBytes?: number
  estimatedInputTokens?: number
  userInput?: string
  inputMeta?: Record<string, string>
  assistantText: string
  toolCalls: Array<{
    toolCallId: string
    toolName: string
    argsJSON: Record<string, unknown>
    resultJSON?: Record<string, unknown>
    errorClass?: string
  }>
  model?: string
  systemPrompt?: string
  toolCount?: number
  toolNames?: string[]
  messageCount?: number
  temperature?: number
  maxOutputTokens?: number
  systemBytes?: number
  toolsBytes?: number
  messagesBytes?: number
  roleBytes?: Record<string, number>
  toolSchemaBytesMap?: Record<string, number>
  stablePrefixHash?: string
}

function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined
}

function cleanText(value: string | undefined): string | undefined {
  const trimmed = value?.trim()
  return trimmed ? trimmed : undefined
}

function extractToolName(tool: Record<string, unknown>): string {
  if (typeof tool.name === 'string') return tool.name
  const fn = tool.function
  if (fn && typeof fn === 'object') {
    const name = (fn as Record<string, unknown>).name
    if (typeof name === 'string') return name
  }
  return ''
}

function extractMessageText(msg: Record<string, unknown>): string {
  const content = msg.content
  if (typeof content === 'string') return content
  if (Array.isArray(content)) {
    return content
      .map((part: unknown) => {
        if (typeof part === 'string') return part
        if (typeof part === 'object' && part !== null) {
          const p = part as Record<string, unknown>
          return typeof p.text === 'string' ? p.text : JSON.stringify(p)
        }
        return ''
      })
      .join('')
  }
  return JSON.stringify(content)
}

function parseChannelEnvelope(text: string): { text: string; meta: Record<string, string> } | null {
  const normalized = text.replace(/\r\n/g, '\n')
  const match = normalized.match(/^---\n([\s\S]*?)\n---\n([\s\S]*)$/)
  if (!match) return null

  const header = match[1]
  const body = cleanText(match[2]) ?? ''
  const meta: Record<string, string> = {}

  for (const line of header.split('\n')) {
    const idx = line.indexOf(':')
    if (idx <= 0) continue
    const key = line.slice(0, idx).trim()
    const rawValue = line.slice(idx + 1).trim()
    if (!key || !rawValue) continue
    meta[key] = rawValue.replace(/^"|"$/g, '')
  }

  if (!body && Object.keys(meta).length === 0) return null
  return { text: body, meta }
}

function extractUserInputFromPayload(payload: Record<string, unknown> | undefined): {
  userInput?: string
  inputMeta?: Record<string, string>
  messages: Array<Record<string, unknown>>
} {
  const messages = Array.isArray(payload?.messages)
    ? (payload.messages as Array<Record<string, unknown>>)
    : []

  for (let i = messages.length - 1; i >= 0; i--) {
    const msg = messages[i]
    if (msg.role === 'user' || msg.role === 'tool') {
      const text = cleanText(extractMessageText(msg))
      if (text) {
        const parsed = parseChannelEnvelope(text)
        if (parsed) {
          return { userInput: parsed.text, inputMeta: parsed.meta, messages }
        }
        return { userInput: text, messages }
      }
    }
  }

  const fallbackCandidates = [payload?.input, payload?.prompt, payload?.input_text]
  for (const candidate of fallbackCandidates) {
    if (typeof candidate !== 'string') continue
    const text = cleanText(candidate)
    if (!text) continue
    const parsed = parseChannelEnvelope(text)
    if (parsed) {
      return { userInput: parsed.text, inputMeta: parsed.meta, messages }
    }
    return { userInput: text, messages }
  }

  const inputRecord = asRecord(payload?.input)
  const inputText = cleanText(
    typeof inputRecord?.text === 'string'
      ? inputRecord.text
      : typeof inputRecord?.content === 'string'
        ? inputRecord.content
        : undefined,
  )
  if (inputText) {
    const parsed = parseChannelEnvelope(inputText)
    if (parsed) {
      return { userInput: parsed.text, inputMeta: parsed.meta, messages }
    }
    return { userInput: inputText, messages }
  }

  return { messages }
}

function extractCompletedAssistantText(data: Record<string, unknown>): string | undefined {
  const candidates = [data.output_text, data.assistant_text, data.final_output_text, data.text]
  for (const candidate of candidates) {
    if (typeof candidate === 'string') {
      const text = cleanText(candidate)
      if (text) return text
    }
  }
  return undefined
}

function extractRequestMessages(messages: Array<Record<string, unknown>>): Array<{
  role: string
  text: string
  meta?: Record<string, string>
}> {
  const result: Array<{
    role: string
    text: string
    meta?: Record<string, string>
  }> = []

  for (const message of messages) {
    const role = typeof message.role === 'string' ? message.role : ''
    if (!role || role === 'system') continue

    const rawText = cleanText(extractMessageText(message))
    if (!rawText) continue

    if (role === 'user' || role === 'tool') {
      const parsed = parseChannelEnvelope(rawText)
      if (parsed) {
        result.push({ role, text: parsed.text, meta: parsed.meta })
        continue
      }
    }

    result.push({ role, text: rawText })
  }

  return result
}

function mergeTurnResults(
  turns: LlmTurn[],
  resultMap: Record<string, { resultJSON?: Record<string, unknown>; errorClass?: string }>,
) {
  for (const turn of turns) {
    for (const tc of turn.toolCalls) {
      const r = resultMap[tc.toolCallId]
      if (r) {
        tc.resultJSON = r.resultJSON
        tc.errorClass = r.errorClass
      }
    }
  }
}

export function buildTurns(events: RunEventRaw[]): LlmTurn[] {
  const orderedEvents = [...events].sort((left, right) => left.seq - right.seq)
  const turns: LlmTurn[] = []
  let current: LlmTurn | null = null
  const assistantChunks: string[] = []
  const resultMap: Record<string, { resultJSON?: Record<string, unknown>; errorClass?: string }> = {}
  const turnByCallId = new Map<string, LlmTurn>()

  const finalizeCurrentTurn = (fallbackText?: string) => {
    if (!current) return
    const merged = cleanText(assistantChunks.join(''))
    current.assistantText = merged ?? cleanText(fallbackText) ?? current.assistantText
    assistantChunks.length = 0
  }

  for (const ev of orderedEvents) {
    if (ev.type === 'llm.request') {
      finalizeCurrentTurn()
      const d = ev.data as Record<string, unknown>
      const payload = d.payload as Record<string, unknown> | undefined
      const { userInput, inputMeta, messages } = extractUserInputFromPayload(payload)

      const systemMsg = messages.find((m) => m.role === 'system')
      const systemPrompt = systemMsg ? extractMessageText(systemMsg) : undefined
      const tools = Array.isArray(payload?.tools)
        ? (payload.tools as Array<Record<string, unknown>>)
        : []
      const toolNames = tools.map(extractToolName).filter(Boolean)

      current = {
        requestMessages: extractRequestMessages(messages),
        llmCallId: String(d.llm_call_id ?? ''),
        providerKind: String(d.provider_kind ?? ''),
        apiMode: String(d.api_mode ?? ''),
        userInput,
        inputMeta,
        assistantText: '',
        toolCalls: [],
        model: payload?.model != null ? String(payload.model) : undefined,
        systemPrompt,
        toolCount: tools.length > 0 ? tools.length : undefined,
        toolNames: toolNames.length > 0 ? toolNames : undefined,
        messageCount: messages.length > 0 ? messages.length : undefined,
        temperature: typeof payload?.temperature === 'number' ? payload.temperature : undefined,
        maxOutputTokens:
          typeof payload?.max_tokens === 'number'
            ? payload.max_tokens
            : typeof payload?.max_output_tokens === 'number'
              ? payload.max_output_tokens
              : undefined,
        systemBytes: typeof d.system_bytes === 'number' ? d.system_bytes : undefined,
        toolsBytes: typeof d.tools_bytes === 'number' ? d.tools_bytes : undefined,
        messagesBytes: typeof d.messages_bytes === 'number' ? d.messages_bytes : undefined,
        roleBytes: d.role_bytes as Record<string, number> | undefined,
        toolSchemaBytesMap: d.tool_schema_bytes_by_name as Record<string, number> | undefined,
        stablePrefixHash: typeof d.stable_prefix_hash === 'string' ? d.stable_prefix_hash : undefined,
      }
      turns.push(current)
      if (current.llmCallId) {
        turnByCallId.set(current.llmCallId, current)
      }
    } else if (ev.type === 'message.delta' && current) {
      const d = ev.data as Record<string, unknown>
      if (d.channel !== 'thinking') {
        assistantChunks.push(String(d.content_delta ?? ''))
      }
    } else if (ev.type === 'tool.call' && current) {
      const d = ev.data as Record<string, unknown>
      current.toolCalls.push({
        toolCallId: String(d.tool_call_id ?? ''),
        toolName: String(d.tool_name ?? ev.tool_name ?? ''),
        argsJSON: (d.arguments as Record<string, unknown>) ?? {},
      })
    } else if (ev.type === 'tool.result') {
      const d = ev.data as Record<string, unknown>
      resultMap[String(d.tool_call_id ?? '')] = {
        resultJSON: d.result as Record<string, unknown> | undefined,
        errorClass: ev.error_class,
      }
    } else if (ev.type === 'llm.turn.completed') {
      const d = ev.data as Record<string, unknown>
      const llmCallId = String(d.llm_call_id ?? '')
      const target = llmCallId ? turnByCallId.get(llmCallId) : null
      if (target) {
        const usage = d.usage as Record<string, unknown> | undefined
        if (usage) {
          target.inputTokens = usage.input_tokens as number | undefined
          target.outputTokens = usage.output_tokens as number | undefined
          target.cachedTokens = (usage.cached_tokens ?? usage.cache_read_input_tokens) as number | undefined
          target.cacheCreationTokens = usage.cache_creation_input_tokens as number | undefined
        }
        if (target === current) {
          finalizeCurrentTurn(extractCompletedAssistantText(d))
        } else if (!target.assistantText) {
          target.assistantText = extractCompletedAssistantText(d) ?? target.assistantText
        }
      }
    } else if (ev.type === 'run.completed' || ev.type === 'run.failed' || ev.type === 'run.cancelled') {
      finalizeCurrentTurn()
      current = null
    }
  }

  finalizeCurrentTurn()
  mergeTurnResults(turns, resultMap)
  return turns
}
