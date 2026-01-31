import { type RunEvent, type SSEClientState } from '../sse'

type RunEventsPanelProps = {
  events: RunEvent[]
  state: SSEClientState
  lastSeq: number
  error: Error | null
  onReconnect: () => void
  onClear: () => void
}

const STATE_LABELS: Record<SSEClientState, string> = {
  idle: '未连接',
  connecting: '连接中...',
  connected: '已连接',
  reconnecting: '重连中...',
  closed: '已断开',
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

function EventTypeTag({ type }: { type: string }) {
  const colorMap: Record<string, string> = {
    'run.started': 'bg-emerald-900/60 text-emerald-200',
    'run.completed': 'bg-blue-900/60 text-blue-200',
    'run.failed': 'bg-rose-900/60 text-rose-200',
    'message.delta': 'bg-violet-900/60 text-violet-200',
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
  onReconnect,
  onClear,
}: RunEventsPanelProps) {
  const canReconnect = state === 'closed' || state === 'error'

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
        {events.length === 0 ? (
          <div className="px-4 py-8 text-center text-sm text-slate-500">
            暂无事件
          </div>
        ) : (
          events.map(event => (
            <EventItem key={event.event_id} event={event} />
          ))
        )}
      </div>
    </div>
  )
}
