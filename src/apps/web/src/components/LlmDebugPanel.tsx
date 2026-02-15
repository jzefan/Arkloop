import { type RunEvent } from '../sse'

type LlmDebugPanelProps = {
  events: RunEvent[]
  onClear: () => void
}

type LlmRequest = {
  seq: number
  ts: string
  providerKind?: string
  apiMode?: string
  baseUrl?: string
  path?: string
  payload?: unknown
}

type LlmChunk = {
  seq: number
  ts: string
  raw?: string
  json?: unknown
  statusCode?: number
  truncated?: boolean
}

type LlmCallGroup = {
  llmCallId: string
  request?: LlmRequest
  chunks: LlmChunk[]
}

const MAX_RENDERED_CHUNKS = 200

function isRecord(value: unknown): value is Record<string, unknown> {
  return value != null && typeof value === 'object' && !Array.isArray(value)
}

function parseLlmGroups(events: RunEvent[]): LlmCallGroup[] {
  const groups = new Map<string, LlmCallGroup>()
  const order: string[] = []

  const ensureGroup = (llmCallId: string): LlmCallGroup => {
    const existing = groups.get(llmCallId)
    if (existing) return existing
    const group: LlmCallGroup = { llmCallId, chunks: [] }
    groups.set(llmCallId, group)
    order.push(llmCallId)
    return group
  }

  for (const event of events) {
    if (event.type !== 'llm.request' && event.type !== 'llm.response.chunk') continue
    if (!isRecord(event.data)) continue

    const llmCallId = typeof event.data.llm_call_id === 'string' ? event.data.llm_call_id : null
    if (!llmCallId) continue

    const group = ensureGroup(llmCallId)

    if (event.type === 'llm.request') {
      group.request = {
        seq: event.seq,
        ts: event.ts,
        providerKind: typeof event.data.provider_kind === 'string' ? event.data.provider_kind : undefined,
        apiMode: typeof event.data.api_mode === 'string' ? event.data.api_mode : undefined,
        baseUrl: typeof event.data.base_url === 'string' ? event.data.base_url : undefined,
        path: typeof event.data.path === 'string' ? event.data.path : undefined,
        payload: event.data.payload,
      }
      continue
    }

    group.chunks.push({
      seq: event.seq,
      ts: event.ts,
      raw: typeof event.data.raw === 'string' ? event.data.raw : undefined,
      json: event.data.json,
      statusCode: typeof event.data.status_code === 'number' ? event.data.status_code : undefined,
      truncated: typeof event.data.truncated === 'boolean' ? event.data.truncated : undefined,
    })
  }

  return order.map((id) => groups.get(id)!).filter(Boolean)
}

function formatJson(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2)
  } catch {
    return String(value)
  }
}

function payloadSummary(value: unknown): string {
  if (value == null) return 'null'
  if (Array.isArray(value)) return `array(${value.length})`
  if (isRecord(value)) return `object(${Object.keys(value).length})`
  return typeof value
}

export function LlmDebugPanel({ events, onClear }: LlmDebugPanelProps) {
  const groups = parseLlmGroups(events)

  return (
    <div className="rounded-2xl border border-slate-800 bg-slate-900/40 shadow-sm">
      <div className="flex items-center justify-between border-b border-slate-800 px-4 py-3">
        <div className="flex items-center gap-3">
          <h3 className="text-sm font-medium text-slate-200">LLM 调试</h3>
          <span className="text-xs text-slate-500">calls: {groups.length}</span>
        </div>
        <button
          className="rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-1.5 text-xs text-slate-300 hover:bg-slate-950/60"
          onClick={onClear}
          type="button"
        >
          清空
        </button>
      </div>

      <div className="max-h-[55vh] overflow-y-auto p-4">
        {groups.length === 0 ? (
          <div className="rounded-xl border border-dashed border-slate-800 bg-slate-950/20 px-4 py-6 text-sm text-slate-500">
            未收到 <span className="font-mono">llm.*</span> 调试事件。后端需开启环境变量{' '}
            <span className="font-mono">ARKLOOP_LLM_DEBUG_EVENTS=1</span>。
          </div>
        ) : (
          <div className="space-y-3">
            {groups.map((group) => {
              const request = group.request
              const title = request
                ? `${request.providerKind ?? '-'} / ${request.apiMode ?? '-'} ${request.path ?? ''}`
                : '未记录 request'

              const chunks =
                group.chunks.length > MAX_RENDERED_CHUNKS
                  ? group.chunks.slice(-MAX_RENDERED_CHUNKS)
                  : group.chunks
              const clipped = group.chunks.length > chunks.length
              const rawChars = chunks.reduce((acc, chunk) => acc + (chunk.raw?.length ?? 0), 0)
              const truncatedChunks = chunks.reduce((acc, chunk) => acc + (chunk.truncated ? 1 : 0), 0)

              const chunkLines = chunks
                .map((chunk) => {
                  const tag = chunk.truncated ? ' (truncated)' : ''
                  const code = chunk.statusCode != null ? ` [${chunk.statusCode}]` : ''
                  return `#${chunk.seq}${code}${tag}\n${chunk.raw ?? ''}`
                })
                .join('\n\n')

              return (
                <details
                  key={group.llmCallId}
                  className="rounded-xl border border-slate-800 bg-slate-950/20"
                >
                  <summary className="cursor-pointer select-none px-4 py-3 text-sm text-slate-200">
                    <span className="font-mono text-xs text-slate-500">#{request?.seq ?? '-'}</span>{' '}
                    <span className="ml-2">{title}</span>
                    <span className="ml-2 text-xs text-slate-500">chunks: {group.chunks.length}</span>
                  </summary>

                  <div className="space-y-3 border-t border-slate-800 px-4 py-3">
                    {request ? (
                      <div>
                        <div className="text-xs font-medium text-slate-300">input</div>
                        <div className="mt-1 text-xs text-slate-500">
                          llm_call_id: <span className="font-mono text-slate-400">{group.llmCallId}</span>
                          {request.baseUrl ? (
                            <>
                              {' '}
                              base_url: <span className="font-mono text-slate-400">{request.baseUrl}</span>
                            </>
                          ) : null}
                        </div>
                        <details className="mt-2 rounded bg-slate-950/30 px-3 py-2">
                          <summary className="cursor-pointer select-none text-xs text-slate-400">
                            查看 payload（{payloadSummary(request.payload)}）
                          </summary>
                          <pre className="mt-2 overflow-x-auto rounded bg-slate-950/50 p-2 text-xs text-slate-400">
                            {formatJson(request.payload)}
                          </pre>
                        </details>
                      </div>
                    ) : null}

                    <div>
                      <div className="flex items-center justify-between">
                        <div className="text-xs font-medium text-slate-300">output</div>
                        {clipped ? (
                          <div className="text-xs text-slate-500">
                            仅显示最近 {MAX_RENDERED_CHUNKS} 条
                          </div>
                        ) : null}
                      </div>
                      <div className="mt-1 text-xs text-slate-500">
                        chunk_count: <span className="font-mono text-slate-400">{group.chunks.length}</span>{' '}
                        raw_chars: <span className="font-mono text-slate-400">{rawChars}</span>{' '}
                        truncated: <span className="font-mono text-slate-400">{truncatedChunks}</span>
                      </div>

                      <details className="mt-2 rounded bg-slate-950/30 px-3 py-2">
                        <summary className="cursor-pointer select-none text-xs text-slate-400">
                          查看原始 raw（按 chunk 分段）
                        </summary>
                        <pre className="mt-2 overflow-x-auto whitespace-pre-wrap rounded bg-slate-950/50 p-2 text-xs text-slate-400">
                          {chunkLines || '暂无 chunk'}
                        </pre>
                      </details>

                      {chunks.some((c) => c.json != null) ? (
                        <details className="mt-2 rounded bg-slate-950/30 px-3 py-2">
                          <summary className="cursor-pointer select-none text-xs text-slate-400">
                            查看解析后的 JSON
                          </summary>
                          <pre className="mt-2 overflow-x-auto rounded bg-slate-950/50 p-2 text-xs text-slate-400">
                            {formatJson(chunks.map((c) => ({ seq: c.seq, json: c.json })))}
                          </pre>
                        </details>
                      ) : null}
                    </div>
                  </div>
                </details>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}
