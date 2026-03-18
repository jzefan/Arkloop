import { describe, expect, it } from 'vitest'
import { buildTurns, type RunEventRaw } from '@arkloop/shared/run-turns'

function makeEvent(params: {
  seq: number
  type: string
  data?: Record<string, unknown>
}): RunEventRaw {
  return {
    event_id: `evt_${params.seq}`,
    run_id: 'run_1',
    seq: params.seq,
    ts: '2026-03-18T00:00:00.000Z',
    type: params.type,
    data: params.data ?? {},
  }
}

describe('buildTurns', () => {
  it('应忽略 thinking channel，只保留正常 assistant 输出', () => {
    const turns = buildTurns([
      makeEvent({
        seq: 1,
        type: 'llm.request',
        data: {
          payload: {
            messages: [{ role: 'user', content: '帮我查一下' }],
          },
        },
      }),
      makeEvent({
        seq: 2,
        type: 'message.delta',
        data: { role: 'assistant', channel: 'thinking', content_delta: '先想一下' },
      }),
      makeEvent({
        seq: 3,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: '我先检查一下现有工具。' },
      }),
      makeEvent({
        seq: 4,
        type: 'run.completed',
        data: {},
      }),
    ])

    expect(turns).toHaveLength(1)
    expect(turns[0]?.assistantText).toBe('我先检查一下现有工具。')
  })
})
