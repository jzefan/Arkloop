import type { RunEvent } from './sse'

const SUPPRESSED_AFTER_BLOCK_EVENT_TYPES = new Set([
  'message.delta',
  'tool.call',
  'tool.result',
  'run.segment.start',
  'run.segment.end',
  'run.input_requested',
])

export function getInjectionBlockMessage(event: RunEvent): string | null {
  if (event.type !== 'security.injection.blocked') return null
  if (!event.data || typeof event.data !== 'object') return 'message blocked: injection detected'

  const rawMessage = (event.data as { message?: unknown }).message
  return typeof rawMessage === 'string' && rawMessage.trim() !== ''
    ? rawMessage
    : 'message blocked: injection detected'
}

export function shouldSuppressLiveRunEventAfterInjectionBlock(params: {
  activeRunId: string | null
  blockedRunId: string | null
  event: RunEvent
}): boolean {
  const { activeRunId, blockedRunId, event } = params
  if (!activeRunId || !blockedRunId) return false
  if (blockedRunId !== activeRunId || event.run_id !== activeRunId) return false
  return SUPPRESSED_AFTER_BLOCK_EVENT_TYPES.has(event.type)
}
