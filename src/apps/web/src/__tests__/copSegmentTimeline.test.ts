import { describe, expect, it } from 'vitest'
import { copTimelinePayloadForSegment, toolCallIdsInCopTimelines } from '../copSegmentTimeline'

const call = (id: string, name: string, seq: number) =>
  ({ kind: 'call' as const, call: { toolCallId: id, toolName: name, arguments: {} }, seq })

describe('copTimelinePayloadForSegment', () => {
  it('无匹配富数据时仍返回空壳，供 COP 标题行挂载', () => {
    const r = copTimelinePayloadForSegment(
      { type: 'cop', title: null, items: [call('x', 'search_tools', 1)] },
      { sources: [] },
    )
    expect(r).toEqual({ steps: [], sources: [] })
  })

  it('按 tool_call_id 筛出代码执行', () => {
    const r = copTimelinePayloadForSegment(
      {
        type: 'cop',
        title: 't',
        items: [
          call('a', 'python_execute', 2),
          call('b', 'unknown', 3),
        ],
      },
      {
        codeExecutions: [
          { id: 'a', language: 'python', code: '1', status: 'success', seq: 2 },
          { id: 'z', language: 'python', code: '2', status: 'success', seq: 1 },
        ],
        sources: [],
      },
    )
    expect(r.codeExecutions).toEqual([{ id: 'a', language: 'python', code: '1', status: 'success', seq: 2 }])
    expect(r.steps).toEqual([])
  })

  it('含 searching 步骤时附带 sources', () => {
    const r = copTimelinePayloadForSegment(
      {
        type: 'cop',
        title: null,
        items: [call('ws1', 'web_search', 1)],
      },
      {
        searchSteps: [
          { id: 'ws1', kind: 'searching', label: 'q', status: 'done', seq: 1 },
        ],
        sources: [{ title: 'u', url: 'https://u.test' }],
      },
    )
    expect(r.sources).toHaveLength(1)
  })

  it('toolCallIdsInCopTimelines 汇总 COP 时间轴已占用的 id', () => {
    const ids = toolCallIdsInCopTimelines(
      {
        segments: [
          {
            type: 'cop',
            title: null,
            items: [call('fo1', 'search_tools', 1)],
          },
        ],
      },
      {
        fileOps: [{ id: 'fo1', toolName: 'search_tools', label: 'x', status: 'success' }],
        sources: [],
      },
    )
    expect(ids.has('fo1')).toBe(true)
  })
})
