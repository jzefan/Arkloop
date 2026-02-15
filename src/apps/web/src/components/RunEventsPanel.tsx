import { useMemo, useState } from 'react'
import { type RunEvent, type SSEClientState } from '../sse'

type RunEventsPanelProps = {
  events: RunEvent[]
  state: SSEClientState
  lastSeq: number
  error: Error | null
  allowReconnect?: boolean
  onReconnect: () => void
  onClear: () => void
}

const MAX_STREAM_PREVIEW_CHARS = 800

const STATE_LABELS: Record<SSEClientState, string> = {
  idle: '未连接',
  connecting: '连接中...',
  connected: '已连接',
  reconnecting: '重连中...',
  closed: '已关闭',
  error: '连接错误',
}

const STATE_COLORS: Record<SSEClientState, string> = {
  idle: 'bg-slate-500',
  connecting: 'bg-amber-500',
  connected: 'bg-emerald-500',
  reconnecting: 'bg-amber-500',
  closed: 'bg-slate-500',
  error: 'bg-rose-500',
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value != null && typeof value === 'object' && !Array.isArray(value)
}

function collapseRunEvents(events: RunEvent[]): RunEvent[] {
  const collapsed: RunEvent[] = []

  let pendingMessage: {
    first: RunEvent
    last: RunEvent
    deltaCount: number
    totalChars: number
    preview: string
    previewTruncated: boolean
  } | null = null

  let pendingChunks: {
    first: RunEvent
    last: RunEvent
    llmCallId: string
    chunkCount: number
    rawChars: number
    truncatedCount: number
    statusCodes: Set<number>
    providerKind?: string
    apiMode?: string
  } | null = null

  const flushMessage = () => {
    if (!pendingMessage) return
    const { first, last, deltaCount, totalChars, preview, previewTruncated } = pendingMessage
    collapsed.push({
      event_id: first.event_id,
      run_id: first.run_id,
      seq: first.seq,
      ts: first.ts,
      type: 'message.stream',
      tool_name: undefined,
      error_class: undefined,
      data: {
        seq_from: first.seq,
        seq_to: last.seq,
        delta_count: deltaCount,
        content_chars: totalChars,
        content_preview: preview,
        truncated: previewTruncated,
      },
    })
    pendingMessage = null
  }

  const flushChunks = () => {
    if (!pendingChunks) return
    const {
      first,
      last,
      llmCallId,
      chunkCount,
      rawChars,
      truncatedCount,
      statusCodes,
      providerKind,
      apiMode,
    } = pendingChunks

    collapsed.push({
      event_id: first.event_id,
      run_id: first.run_id,
      seq: first.seq,
      ts: first.ts,
      type: 'llm.response.stream',
      tool_name: undefined,
      error_class: undefined,
      data: {
        llm_call_id: llmCallId,
        provider_kind: providerKind,
        api_mode: apiMode,
        seq_from: first.seq,
        seq_to: last.seq,
        chunk_count: chunkCount,
        raw_chars: rawChars,
        truncated_chunks: truncatedCount,
        status_codes: Array.from(statusCodes).sort((a, b) => a - b),
      },
    })
    pendingChunks = null
  }

  for (const event of events) {
    if (event.type === 'message.delta') {
      flushChunks()
      if (!isRecord(event.data)) {
        flushMessage()
        collapsed.push(event)
        continue
      }
      const role = event.data.role
      if (role != null && role !== 'assistant') {
        flushMessage()
        collapsed.push(event)
        continue
      }
      const delta = typeof event.data.content_delta === 'string' ? event.data.content_delta : ''
      if (!delta) {
        continue
      }

      if (!pendingMessage) {
        pendingMessage = {
          first: event,
          last: event,
          deltaCount: 0,
          totalChars: 0,
          preview: '',
          previewTruncated: false,
        }
      }

      pendingMessage.last = event
      pendingMessage.deltaCount += 1
      pendingMessage.totalChars += delta.length
      if (!pendingMessage.previewTruncated) {
        const remaining = MAX_STREAM_PREVIEW_CHARS - pendingMessage.preview.length
        if (remaining > 0) {
          pendingMessage.preview += delta.slice(0, remaining)
          if (delta.length > remaining) {
            pendingMessage.previewTruncated = true
          }
        } else {
          pendingMessage.previewTruncated = true
        }
      }
      continue
    }

    if (event.type === 'llm.response.chunk') {
      flushMessage()
      if (!isRecord(event.data)) {
        flushChunks()
        collapsed.push(event)
        continue
      }

      const llmCallId = typeof event.data.llm_call_id === 'string' ? event.data.llm_call_id : ''
      if (!llmCallId) {
        flushChunks()
        collapsed.push(event)
        continue
      }

      const providerKind =
        typeof event.data.provider_kind === 'string' ? event.data.provider_kind : undefined
      const apiMode = typeof event.data.api_mode === 'string' ? event.data.api_mode : undefined
      const raw = typeof event.data.raw === 'string' ? event.data.raw : ''
      const truncated = typeof event.data.truncated === 'boolean' ? event.data.truncated : false
      const statusCode = typeof event.data.status_code === 'number' ? event.data.status_code : null

      if (!pendingChunks || pendingChunks.llmCallId !== llmCallId) {
        flushChunks()
        pendingChunks = {
          first: event,
          last: event,
          llmCallId,
          chunkCount: 0,
          rawChars: 0,
          truncatedCount: 0,
          statusCodes: new Set<number>(),
          providerKind,
          apiMode,
        }
      }

      pendingChunks.last = event
      pendingChunks.chunkCount += 1
      pendingChunks.rawChars += raw.length
      if (truncated) pendingChunks.truncatedCount += 1
      if (statusCode != null) pendingChunks.statusCodes.add(statusCode)
      continue
    }

    if (event.type === 'llm.request') {
      flushMessage()
      flushChunks()
      if (!isRecord(event.data)) {
        collapsed.push(event)
        continue
      }
      const llmCallId = typeof event.data.llm_call_id === 'string' ? event.data.llm_call_id : undefined
      const providerKind =
        typeof event.data.provider_kind === 'string' ? event.data.provider_kind : undefined
      const apiMode = typeof event.data.api_mode === 'string' ? event.data.api_mode : undefined
      const baseUrl = typeof event.data.base_url === 'string' ? event.data.base_url : undefined
      const path = typeof event.data.path === 'string' ? event.data.path : undefined
      const payload = event.data.payload
      const payloadSummary = isRecord(payload)
        ? { kind: 'object', keys: Object.keys(payload).length }
        : Array.isArray(payload)
          ? { kind: 'array', items: payload.length }
          : payload == null
            ? { kind: 'null' }
            : { kind: typeof payload }

      collapsed.push({
        ...event,
        type: 'llm.request.summary',
        data: {
          llm_call_id: llmCallId,
          provider_kind: providerKind,
          api_mode: apiMode,
          base_url: baseUrl,
          path,
          payload: payloadSummary,
        },
      })
      continue
    }

    flushMessage()
    flushChunks()
    collapsed.push(event)
  }

  flushMessage()
  flushChunks()
  return collapsed
}

function EventTypeTag({ type }: { type: string }) {
  const colorMap: Record<string, string> = {
    'run.started': 'bg-emerald-900/60 text-emerald-200',
    'run.completed': 'bg-blue-900/60 text-blue-200',
    'run.failed': 'bg-rose-900/60 text-rose-200',
    'message.delta': 'bg-violet-900/60 text-violet-200',
    'message.stream': 'bg-violet-900/60 text-violet-200',
    'llm.request': 'bg-slate-700 text-slate-200',
    'llm.request.summary': 'bg-slate-700 text-slate-200',
    'llm.response.chunk': 'bg-slate-700 text-slate-200',
    'llm.response.stream': 'bg-slate-700 text-slate-200',
    'tool.call': 'bg-amber-900/60 text-amber-200',
    'tool.result': 'bg-cyan-900/60 text-cyan-200',
  }

  const color = colorMap[type] ?? 'bg-slate-700 text-slate-300'

  return (
    <span className={`inline-block rounded px-2 py-0.5 text-xs font-medium ${color}`}>
      {type}
    </span>
  )
}

function EventItem({ event }: { event: RunEvent }) {
  const ts = new Date(event.ts).toLocaleTimeString('zh-CN', {
    hour12: false,
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    fractionalSecondDigits: 3,
  })

  const hasData = event.data && typeof event.data === 'object' && Object.keys(event.data as object).length > 0

  return (
    <div className="border-b border-slate-800 px-4 py-3 last:border-b-0">
      <div className="flex items-center gap-3">
        <span className="font-mono text-xs text-slate-500">#{event.seq}</span>
        <EventTypeTag type={event.type} />
        <span className="text-xs text-slate-500">{ts}</span>
      </div>

      {hasData ? (
        <pre className="mt-2 overflow-x-auto rounded bg-slate-950/50 p-2 text-xs text-slate-400">
          {JSON.stringify(event.data, null, 2)}
        </pre>
      ) : null}

      {event.tool_name ? (
        <div className="mt-1 text-xs text-slate-500">
          tool: <span className="text-slate-400">{event.tool_name}</span>
        </div>
      ) : null}

      {event.error_class ? (
        <div className="mt-1 text-xs text-rose-400">
          error: {event.error_class}
        </div>
      ) : null}
    </div>
  )
}

export function RunEventsPanel({
  events,
  state,
  lastSeq,
  error,
  allowReconnect = true,
  onReconnect,
  onClear,
}: RunEventsPanelProps) {
  const canReconnect = allowReconnect && (state === 'closed' || state === 'error')
  const [compact, setCompact] = useState(true)

  const displayEvents = useMemo(() => {
    if (!compact) return events
    return collapseRunEvents(events)
  }, [compact, events])

  return (
    <div className="rounded-2xl border border-slate-800 bg-slate-900/40 shadow-sm">
      <div className="flex items-center justify-between border-b border-slate-800 px-4 py-3">
        <div className="flex items-center gap-3">
          <h3 className="text-sm font-medium text-slate-200">事件流</h3>
          <div className="flex items-center gap-2">
            <span className={`h-2 w-2 rounded-full ${STATE_COLORS[state]}`} />
            <span className="text-xs text-slate-400">{STATE_LABELS[state]}</span>
          </div>
          <span className="text-xs text-slate-500">seq: {lastSeq}</span>
        </div>

        <div className="flex items-center gap-2">
          {canReconnect && (
            <button
              className="rounded-lg bg-indigo-600 px-3 py-1.5 text-xs font-medium text-white hover:bg-indigo-500"
              onClick={onReconnect}
              type="button"
            >
              重连
            </button>
          )}
          <button
            className="rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-1.5 text-xs text-slate-300 hover:bg-slate-950/60"
            onClick={() => setCompact((value) => !value)}
            type="button"
          >
            {compact ? '原始' : '精简'}
          </button>
          <button
            className="rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-1.5 text-xs text-slate-300 hover:bg-slate-950/60"
            onClick={onClear}
            type="button"
          >
            清空
          </button>
        </div>
      </div>

      {error && (
        <div className="border-b border-rose-900/40 bg-rose-950/30 px-4 py-2 text-xs text-rose-300">
          {error.message}
        </div>
      )}

      <div className="max-h-96 overflow-y-auto">
        {displayEvents.length === 0 ? (
          <div className="px-4 py-8 text-center text-sm text-slate-500">
            暂无事件
          </div>
        ) : (
          displayEvents.map(event => (
            <EventItem key={event.event_id} event={event} />
          ))
        )}
      </div>
    </div>
  )
}
