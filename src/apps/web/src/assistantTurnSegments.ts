import { isACPDelegateEventData } from '@arkloop/shared'
import type { RunEvent } from './sse'

export type TurnToolCallRef = {
  toolCallId: string
  toolName: string
  arguments: Record<string, unknown>
  result?: unknown
  errorClass?: string
}

export type AssistantTurnSegment =
  | { type: 'text'; content: string }
  | { type: 'cop'; title: string | null; calls: TurnToolCallRef[] }

export type AssistantTurnUi = { segments: AssistantTurnSegment[] }

/** SSE 递增折叠用状态（事件按 seq 递增到达，禁止对 live 路径全量 sort）。 */
export type AssistantTurnFoldState = {
  segments: AssistantTurnSegment[]
  currentCop: { type: 'cop'; title: string | null; calls: TurnToolCallRef[] } | null
}

const TIMELINE_TITLE_TOOL = 'timeline_title'

function pickToolName(data: unknown): string {
  if (!data || typeof data !== 'object') return ''
  const raw = (data as { tool_name?: unknown }).tool_name
  return typeof raw === 'string' ? raw : ''
}

function pickToolCallId(event: RunEvent): string {
  if (!event.data || typeof event.data !== 'object') return event.event_id
  const raw = (event.data as { tool_call_id?: unknown }).tool_call_id
  return typeof raw === 'string' && raw.trim() !== '' ? raw : event.event_id
}

function sortRunEvents(events: readonly RunEvent[]): RunEvent[] {
  return [...events].sort((left, right) => left.seq - right.seq || left.ts.localeCompare(right.ts))
}

function extractArguments(data: unknown): Record<string, unknown> {
  if (!data || typeof data !== 'object') return {}
  const raw = (data as { arguments?: unknown }).arguments
  if (raw && typeof raw === 'object' && !Array.isArray(raw)) {
    return { ...(raw as Record<string, unknown>) }
  }
  return {}
}

function extractResultPayload(event: RunEvent): unknown {
  if (!event.data || typeof event.data !== 'object') return undefined
  return (event.data as { result?: unknown }).result
}

function copIsEmpty(cop: { title: string | null; calls: TurnToolCallRef[] }): boolean {
  return cop.title == null && cop.calls.length === 0
}

function cloneTurnToolCall(c: TurnToolCallRef): TurnToolCallRef {
  return {
    toolCallId: c.toolCallId,
    toolName: c.toolName,
    arguments: { ...c.arguments },
    result: c.result,
    errorClass: c.errorClass,
  }
}

function cloneSegment(s: AssistantTurnSegment): AssistantTurnSegment {
  if (s.type === 'text') return { type: 'text', content: s.content }
  return {
    type: 'cop',
    title: s.title,
    calls: s.calls.map(cloneTurnToolCall),
  }
}

/** 结束 run 时收尾并取出不可变快照，清空 fold state。 */
export function drainAssistantTurnForPersist(state: AssistantTurnFoldState): AssistantTurnUi {
  finalizeAssistantTurnFoldState(state)
  const turn: AssistantTurnUi = { segments: state.segments.map(cloneSegment) }
  state.segments = []
  state.currentCop = null
  return turn
}

export function createEmptyAssistantTurnFoldState(): AssistantTurnFoldState {
  return { segments: [], currentCop: null }
}

function flushCopToSegments(
  segments: AssistantTurnSegment[],
  currentCop: AssistantTurnFoldState['currentCop'],
): void {
  if (currentCop == null) return
  if (!copIsEmpty(currentCop)) {
    segments.push({
      type: 'cop',
      title: currentCop.title,
      calls: currentCop.calls.map(cloneTurnToolCall),
    })
  }
}

/** 将当前 open cop 结束前推入 segments 的不可变快照（供 React state）。 */
export function snapshotAssistantTurn(state: AssistantTurnFoldState): AssistantTurnUi {
  const segments = state.segments.map(cloneSegment)
  flushCopToSegments(segments, state.currentCop)
  return { segments }
}

/** 处理单条事件（仅 message.delta / tool.call / tool.result）；可变 state。 */
export function foldAssistantTurnEvent(state: AssistantTurnFoldState, event: RunEvent): void {
  const { segments } = state
  let { currentCop } = state

  const flushCop = () => {
    if (currentCop == null) return
    if (!copIsEmpty(currentCop)) {
      segments.push({
        type: 'cop',
        title: currentCop.title,
        calls: currentCop.calls.map(cloneTurnToolCall),
      })
    }
    currentCop = null
  }

  const appendAssistantDelta = (delta: string) => {
    if (delta === '') return
    // 流式里工具批之间常插 \n 或仅空白 delta，若 flush 会把同一轮 tool 拆成两段 cop
    if (delta.trim() === '') {
      const last = segments[segments.length - 1]
      if (last?.type === 'text') last.content += delta
      return
    }
    flushCop()
    const last = segments[segments.length - 1]
    if (last?.type === 'text') {
      last.content += delta
    } else {
      segments.push({ type: 'text', content: delta })
    }
  }

  const ensureCop = () => {
    if (currentCop == null) {
      currentCop = { type: 'cop', title: null, calls: [] }
    }
  }

  const attachResultToCop = (toolCallId: string, toolName: string, result: unknown, errorClass?: string) => {
    if (!currentCop) return
    const call = currentCop.calls.find((c) => c.toolCallId === toolCallId)
    if (call) {
      call.result = result
      if (errorClass) call.errorClass = errorClass
      return
    }
    currentCop.calls.push({
      toolCallId,
      toolName: toolName || 'unknown',
      arguments: {},
      result,
      errorClass,
    })
  }

  if (event.type === 'message.delta') {
    if (isACPDelegateEventData(event.data)) return
    const obj = event.data as { content_delta?: unknown; role?: unknown; channel?: unknown }
    const delta = obj.content_delta
    const isAssistant =
      (obj.role == null || obj.role === 'assistant') &&
      obj.channel !== 'thinking' &&
      typeof delta === 'string'
    if (isAssistant) {
      appendAssistantDelta(delta)
    }
    state.currentCop = currentCop
    return
  }

  if (event.type === 'tool.call') {
    if (isACPDelegateEventData(event.data)) return
    const toolName = pickToolName(event.data)
    if (toolName === TIMELINE_TITLE_TOOL) {
      ensureCop()
      const args = extractArguments(event.data)
      const labelRaw = args.label
      const label = typeof labelRaw === 'string' ? labelRaw.trim() : ''
      if (label !== '' && currentCop) {
        currentCop.title = label
      }
      state.currentCop = currentCop
      return
    }
    ensureCop()
    currentCop!.calls.push({
      toolCallId: pickToolCallId(event),
      toolName,
      arguments: extractArguments(event.data),
    })
    state.currentCop = currentCop
    return
  }

  if (event.type === 'tool.result') {
    if (isACPDelegateEventData(event.data)) return
    const toolName = pickToolName(event.data)
    const toolCallId = pickToolCallId(event)
    const result = extractResultPayload(event)
    const err =
      typeof event.error_class === 'string' && event.error_class.trim() !== ''
        ? event.error_class
        : undefined
    attachResultToCop(toolCallId, toolName, result, err)
    state.currentCop = currentCop
  }
}

/** run 结束时关闭未决 cop（仍在同一 state 上操作，再 snapshot）。 */
export function finalizeAssistantTurnFoldState(state: AssistantTurnFoldState): void {
  if (state.currentCop == null) return
  if (!copIsEmpty(state.currentCop)) {
    state.segments.push({
      type: 'cop',
      title: state.currentCop.title,
      calls: state.currentCop.calls.map(cloneTurnToolCall),
    })
  }
  state.currentCop = null
}

/** 从一次 run 的事件流构建 assistant turn（重放时按 seq 排序）。 */
export function buildAssistantTurnFromRunEvents(events: readonly RunEvent[]): AssistantTurnUi {
  const state = createEmptyAssistantTurnFoldState()
  for (const event of sortRunEvents(events)) {
    foldAssistantTurnEvent(state, event)
  }
  finalizeAssistantTurnFoldState(state)
  return { segments: state.segments.map(cloneSegment) }
}

/** 将所有 text 段拼接为单一字符串（复制、与 message.content 对照）。 */
export function assistantTurnPlainText(turn: AssistantTurnUi): string {
  let out = ''
  for (const s of turn.segments) {
    if (s.type === 'text') out += s.content
  }
  return out
}
