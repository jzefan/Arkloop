import { describe, expect, it } from 'vitest'
import {
  applyAgentEventToWebSearchSteps,
  COMPLETED_SEARCHING_LABEL,
  DEFAULT_SEARCHING_LABEL,
  isWebSearchToolName,
  webSearchQueriesFromArguments,
  webSearchSourcesFromResult,
} from '../webSearchTimelineFromAgentEvent'
import type { AgentUIEvent } from '../agent-ui'

describe('isWebSearchToolName', () => {
  it('接受常见供应商/模型命名变体', () => {
    expect(isWebSearchToolName('web_search')).toBe(true)
    expect(isWebSearchToolName('WebSearch')).toBe(true)
    expect(isWebSearchToolName('web-search')).toBe(true)
    expect(isWebSearchToolName('web_search.tavily')).toBe(true)
    expect(isWebSearchToolName('other')).toBe(false)
  })
})

describe('webSearchQueriesFromArguments', () => {
  it('同时支持 query 与 queries', () => {
    expect(webSearchQueriesFromArguments({ query: 'a' })).toEqual(['a'])
    expect(webSearchQueriesFromArguments({ queries: ['b', 'c'] })).toEqual(['b', 'c'])
    expect(webSearchQueriesFromArguments({ query: 'a', queries: ['b'] })).toEqual(['a', 'b'])
  })
})

describe('webSearchSourcesFromResult', () => {
  it('提取 results 中的 sources', () => {
    expect(
      webSearchSourcesFromResult({
        results: [
          { title: 'A', url: 'https://a.test', snippet: 'aa' },
          { title: 'B', url: '' },
        ],
      }),
    ).toEqual([{ title: 'A', url: 'https://a.test', snippet: 'aa' }])
  })
})

describe('applyAgentEventToWebSearchSteps', () => {
  it('tool.call 与 tool.result 推进 searching 阶段', () => {
    const call: AgentUIEvent = {
      type: 'tool-call',
      order: 1,
      timestamp: '',
      id: 'e1',
      streamId: 'r',
      data: {
        tool_name: 'WebSearch',
        tool_call_id: 'c1',
        arguments: { queries: ['q1'] },
      },
    }
    const result: AgentUIEvent = {
      type: 'tool-result',
      order: 2,
      timestamp: '',
      id: 'e2',
      streamId: 'r',
      data: {
        tool_name: 'web_search',
        tool_call_id: 'c1',
        result: { results: [{ title: 't', url: 'https://x.test' }] },
      },
    }
    let steps = applyAgentEventToWebSearchSteps([], call)
    expect(steps).toHaveLength(1)
    expect(steps[0]?.kind).toBe('searching')
    expect(steps[0]?.label).toBe(DEFAULT_SEARCHING_LABEL)
    expect(steps[0]?.queries).toEqual(['q1'])
    steps = applyAgentEventToWebSearchSteps(steps, result)
    expect(steps).toHaveLength(1)
    expect(steps[0]?.label).toBe(COMPLETED_SEARCHING_LABEL)
    expect(steps[0]?.sources).toEqual([{ title: 't', url: 'https://x.test', snippet: undefined }])
    expect(steps[0]?.seq).toBe(1)
    expect(steps[0]?.resultSeq).toBe(2)
  })

  it('多次 search 时只给对应 call 绑定自己的 sources', () => {
    let steps = applyAgentEventToWebSearchSteps([], {
      type: 'tool-call',
      order: 10,
      timestamp: '',
      id: 'e1',
      streamId: 'r',
      data: { tool_name: 'web_search', tool_call_id: 's1', arguments: { query: 'first' } },
    })
    steps = applyAgentEventToWebSearchSteps(steps, {
      type: 'tool-call',
      order: 11,
      timestamp: '',
      id: 'e2',
      streamId: 'r',
      data: { tool_name: 'web_search', tool_call_id: 's2', arguments: { query: 'second' } },
    })
    steps = applyAgentEventToWebSearchSteps(steps, {
      type: 'tool-result',
      order: 20,
      timestamp: '',
      id: 'e3',
      streamId: 'r',
      data: {
        tool_name: 'web_search',
        tool_call_id: 's1',
        result: { results: [{ title: 'one', url: 'https://one.test' }] },
      },
    })

    expect(steps.find((step) => step.id === 's1')?.sources).toEqual([{ title: 'one', url: 'https://one.test', snippet: undefined }])
    expect(steps.find((step) => step.id === 's2')?.sources).toBeUndefined()
  })

  it('run.interrupted 也会把主会话搜索步骤收口为 done', () => {
    const active = applyAgentEventToWebSearchSteps([], {
      type: 'tool-call',
      order: 1,
      timestamp: '',
      id: 'e1',
      streamId: 'r',
      data: {
        tool_name: 'web_search',
        tool_call_id: 'host',
        arguments: { query: 'resume me' },
      },
    })
    const interrupted = applyAgentEventToWebSearchSteps(active, {
      type: 'run-interrupted',
      order: 2,
      timestamp: '',
      id: 'e2',
      streamId: 'r',
      data: {},
    })
    expect(interrupted).toHaveLength(1)
    expect(interrupted[0]?.status).toBe('done')
  })
})
