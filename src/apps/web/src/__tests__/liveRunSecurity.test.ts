import { describe, expect, it } from 'vitest'

import { getInjectionBlockMessage, shouldSuppressLiveAgentEventAfterInjectionBlock } from '../liveRunSecurity'
import type { AgentUIEvent } from '../agent-ui'

function makeRunEvent(overrides: Partial<AgentUIEvent> & {
  run_id?: string
  type?: string
  data?: unknown
}): AgentUIEvent {
  return {
    id: 'evt_1',
    streamId: overrides.run_id ?? overrides.streamId ?? 'run_1',
    order: overrides.order ?? 1,
    timestamp: overrides.timestamp ?? '2026-03-14T00:00:00Z',
    type: overrides.type ?? overrides.type ?? 'assistant-delta',
    data: overrides.data ?? overrides.data ?? {},
    ...overrides,
  }
}

describe('getInjectionBlockMessage', () => {
  it('uses explicit block message when present', () => {
    const event = makeRunEvent({
      type: 'security-block',
      data: { message: 'blocked by semantic guard' },
    })

    expect(getInjectionBlockMessage(event)).toBe('blocked by semantic guard')
  })

  it('falls back to default block message', () => {
    const event = makeRunEvent({
      type: 'security-block',
      data: {},
    })

    expect(getInjectionBlockMessage(event)).toBe('message blocked: injection detected')
  })
})

describe('shouldSuppressLiveAgentEventAfterInjectionBlock', () => {
  it('suppresses late streaming events for the blocked active run', () => {
    const event = makeRunEvent({ type: 'assistant-delta' })

    expect(shouldSuppressLiveAgentEventAfterInjectionBlock({
      activeRunId: 'run_1',
      blockedRunId: 'run_1',
      event,
    })).toBe(true)
  })

  it('keeps terminal lifecycle events visible after block', () => {
    const event = makeRunEvent({ type: 'run-cancelled' })

    expect(shouldSuppressLiveAgentEventAfterInjectionBlock({
      activeRunId: 'run_1',
      blockedRunId: 'run_1',
      event,
    })).toBe(false)
  })

  it('does not suppress events for other runs', () => {
    const event = makeRunEvent({ run_id: 'run_2', type: 'assistant-delta' })

    expect(shouldSuppressLiveAgentEventAfterInjectionBlock({
      activeRunId: 'run_1',
      blockedRunId: 'run_1',
      event,
    })).toBe(false)
  })
})
