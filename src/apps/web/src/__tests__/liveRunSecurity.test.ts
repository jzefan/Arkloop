import { describe, expect, it } from 'vitest'

import { getInjectionBlockMessage, shouldSuppressLiveRunEventAfterInjectionBlock } from '../liveRunSecurity'
import type { RunEvent } from '../sse'

function makeRunEvent(overrides: Partial<RunEvent>): RunEvent {
  return {
    event_id: 'evt_1',
    run_id: 'run_1',
    seq: 1,
    ts: '2026-03-14T00:00:00Z',
    type: 'message.delta',
    data: {},
    ...overrides,
  }
}

describe('getInjectionBlockMessage', () => {
  it('uses explicit block message when present', () => {
    const event = makeRunEvent({
      type: 'security.injection.blocked',
      data: { message: 'blocked by semantic guard' },
    })

    expect(getInjectionBlockMessage(event)).toBe('blocked by semantic guard')
  })

  it('falls back to default block message', () => {
    const event = makeRunEvent({
      type: 'security.injection.blocked',
      data: {},
    })

    expect(getInjectionBlockMessage(event)).toBe('message blocked: injection detected')
  })
})

describe('shouldSuppressLiveRunEventAfterInjectionBlock', () => {
  it('suppresses late streaming events for the blocked active run', () => {
    const event = makeRunEvent({ type: 'message.delta' })

    expect(shouldSuppressLiveRunEventAfterInjectionBlock({
      activeRunId: 'run_1',
      blockedRunId: 'run_1',
      event,
    })).toBe(true)
  })

  it('keeps terminal lifecycle events visible after block', () => {
    const event = makeRunEvent({ type: 'run.cancelled' })

    expect(shouldSuppressLiveRunEventAfterInjectionBlock({
      activeRunId: 'run_1',
      blockedRunId: 'run_1',
      event,
    })).toBe(false)
  })

  it('does not suppress events for other runs', () => {
    const event = makeRunEvent({ run_id: 'run_2', type: 'message.delta' })

    expect(shouldSuppressLiveRunEventAfterInjectionBlock({
      activeRunId: 'run_1',
      blockedRunId: 'run_1',
      event,
    })).toBe(false)
  })
})
