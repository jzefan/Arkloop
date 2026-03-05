import { describe, expect, it } from 'vitest'
import { buildMessageThinkingFromRunEvents, selectFreshRunEvents } from '../runEventProcessing'
import type { RunEvent } from '../sse'

function makeRunEvent(params: {
  runId: string
  seq: number
  type: string
  data?: unknown
}): RunEvent {
  return {
    event_id: `evt_${params.seq}`,
    run_id: params.runId,
    seq: params.seq,
    ts: '2024-01-01T00:00:00.000Z',
    type: params.type,
    data: params.data ?? {},
  }
}

describe('selectFreshRunEvents', () => {
  it('应忽略旧 run 的尾部事件，避免误触发断开', () => {
    const events = [makeRunEvent({ runId: 'run_1', seq: 1, type: 'run.completed' })]

    const result = selectFreshRunEvents({
      events,
      activeRunId: 'run_2',
      processedCount: 0,
    })

    expect(result.fresh).toEqual([])
    expect(result.nextProcessedCount).toBe(1)
  })

  it('应在 events 被清空后重置 processedCount', () => {
    const result = selectFreshRunEvents({
      events: [],
      activeRunId: 'run_1',
      processedCount: 10,
    })

    expect(result.fresh).toEqual([])
    expect(result.nextProcessedCount).toBe(0)
  })

  it('应只返回当前 run 的新事件，并推进游标到末尾', () => {
    const events = [
      makeRunEvent({ runId: 'run_1', seq: 1, type: 'run.started' }),
      makeRunEvent({
        runId: 'run_2',
        seq: 2,
        type: 'message.delta',
        data: { content_delta: 'hi', role: 'assistant' },
      }),
      makeRunEvent({ runId: 'run_2', seq: 3, type: 'run.completed' }),
    ]

    const result = selectFreshRunEvents({
      events,
      activeRunId: 'run_2',
      processedCount: 0,
    })

    expect(result.fresh.map((item) => item.seq)).toEqual([2, 3])
    expect(result.nextProcessedCount).toBe(3)
  })

  it('应从 processedCount 之后开始取新事件', () => {
    const events = [
      makeRunEvent({ runId: 'run_1', seq: 1, type: 'run.started' }),
      makeRunEvent({ runId: 'run_1', seq: 2, type: 'message.delta' }),
    ]

    const result = selectFreshRunEvents({
      events,
      activeRunId: 'run_1',
      processedCount: 1,
    })

    expect(result.fresh.map((item) => item.seq)).toEqual([2])
    expect(result.nextProcessedCount).toBe(2)
  })

  it('processedCount 超过 events.length 时重置为 0', () => {
    const events = [
      makeRunEvent({ runId: 'run_1', seq: 1, type: 'run.started' }),
      makeRunEvent({ runId: 'run_1', seq: 2, type: 'message.delta' }),
    ]

    const result = selectFreshRunEvents({
      events,
      activeRunId: 'run_1',
      processedCount: 999,
    })

    expect(result.fresh.map((item) => item.seq)).toEqual([1, 2])
    expect(result.nextProcessedCount).toBe(2)
  })
})

describe('buildMessageThinkingFromRunEvents', () => {
  it('应提取顶层 thinking 文本', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'message.delta',
        data: { role: 'assistant', channel: 'thinking', content_delta: 'A' },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 2,
        type: 'message.delta',
        data: { role: 'assistant', channel: 'thinking', content_delta: 'B' },
      }),
    ]

    const snapshot = buildMessageThinkingFromRunEvents(events)
    expect(snapshot).not.toBeNull()
    expect(snapshot?.thinkingText).toBe('AB')
    expect(snapshot?.segments).toEqual([])
  })

  it('应提取 segment 文本并过滤 hidden 段', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'run.segment.start',
        data: { segment_id: 'seg_1', kind: 'planning_round', display: { mode: 'collapsed', label: 'Plan' } },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 2,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: 'P1' },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 3,
        type: 'run.segment.end',
        data: { segment_id: 'seg_1' },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 4,
        type: 'run.segment.start',
        data: { segment_id: 'seg_2', kind: 'planning_round', display: { mode: 'hidden', label: 'Hidden' } },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 5,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: 'H1' },
      }),
    ]

    const snapshot = buildMessageThinkingFromRunEvents(events)
    expect(snapshot).not.toBeNull()
    expect(snapshot?.segments).toHaveLength(1)
    expect(snapshot?.segments[0]).toMatchObject({
      segmentId: 'seg_1',
      label: 'Plan',
      content: 'P1',
    })
  })

  it('没有 thinking 内容时应返回 null', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: 'Final answer' },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 2,
        type: 'run.completed',
      }),
    ]

    const snapshot = buildMessageThinkingFromRunEvents(events)
    expect(snapshot).toBeNull()
  })

  it('segment.start 缺少 segment_id 时跳过', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'run.segment.start',
        data: { kind: 'planning_round', display: { mode: 'collapsed', label: 'NoID' } },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 2,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: 'orphaned delta' },
      }),
    ]

    const snapshot = buildMessageThinkingFromRunEvents(events)
    expect(snapshot).toBeNull()
  })

  it('非 assistant role 的 message.delta 被过滤', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'run.segment.start',
        data: { segment_id: 'seg_1', kind: 'planning_round', display: { mode: 'collapsed', label: 'Plan' } },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 2,
        type: 'message.delta',
        data: { role: 'user', content_delta: 'user message' },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 3,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: 'valid' },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 4,
        type: 'run.segment.end',
        data: { segment_id: 'seg_1' },
      }),
    ]

    const snapshot = buildMessageThinkingFromRunEvents(events)
    expect(snapshot).not.toBeNull()
    expect(snapshot?.segments[0].content).toBe('valid')
  })

  it('空 content_delta 被过滤', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'message.delta',
        data: { role: 'assistant', channel: 'thinking', content_delta: '' },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 2,
        type: 'message.delta',
        data: { role: 'assistant', channel: 'thinking', content_delta: 'real' },
      }),
    ]

    const snapshot = buildMessageThinkingFromRunEvents(events)
    expect(snapshot).not.toBeNull()
    expect(snapshot?.thinkingText).toBe('real')
  })

  it('segment.end 的 segment_id 不匹配当前活跃 segment 时不关闭', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'run.segment.start',
        data: { segment_id: 'seg_1', kind: 'planning_round', display: { mode: 'collapsed', label: 'Plan' } },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 2,
        type: 'run.segment.end',
        data: { segment_id: 'seg_wrong' },
      }),
      makeRunEvent({
        runId: 'run_1',
        seq: 3,
        type: 'message.delta',
        data: { role: 'assistant', content_delta: 'still in seg_1' },
      }),
    ]

    const snapshot = buildMessageThinkingFromRunEvents(events)
    expect(snapshot).not.toBeNull()
    expect(snapshot?.segments).toHaveLength(1)
    expect(snapshot?.segments[0].content).toBe('still in seg_1')
  })

  it('content_delta 非 string 类型被过滤', () => {
    const events = [
      makeRunEvent({
        runId: 'run_1',
        seq: 1,
        type: 'message.delta',
        data: { role: 'assistant', channel: 'thinking', content_delta: 123 },
      }),
    ]

    const snapshot = buildMessageThinkingFromRunEvents(events)
    expect(snapshot).toBeNull()
  })
})
