import type { AgentUIEvent } from './agent-ui'

const SUPPRESSED_AFTER_BLOCK_EVENT_TYPES = new Set([
  'assistant-delta',
  'tool-call',
  'tool-result',
  'segment-start',
  'segment-end',
  'input-request',
])

export function getInjectionBlockMessage(event: AgentUIEvent): string | null {
  if (event.type !== 'security-block') return null
  if (!event.data || typeof event.data !== 'object') return 'message blocked: injection detected'

  const rawMessage = (event.data as { message?: unknown }).message
  return typeof rawMessage === 'string' && rawMessage.trim() !== ''
    ? rawMessage
    : 'message blocked: injection detected'
}

export function shouldSuppressLiveAgentEventAfterInjectionBlock(params: {
  activeRunId: string | null
  blockedRunId: string | null
  event: AgentUIEvent
}): boolean {
  const { activeRunId, blockedRunId, event } = params
  if (!activeRunId || !blockedRunId) return false
  if (blockedRunId !== activeRunId || event.streamId !== activeRunId) return false
  return SUPPRESSED_AFTER_BLOCK_EVENT_TYPES.has(event.type)
}
