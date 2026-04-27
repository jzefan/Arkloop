import { describe, expect, it } from 'vitest'
import type { CopBlockItem } from '../assistantTurnSegments'
import { aggregateMainTitle, buildSubSegments, categoryForTool, presentToProgressive } from '../copSubSegment'

function toolCall(
  id: string,
  toolName: string,
  seq: number,
  args: Record<string, unknown> = {},
): CopBlockItem {
  return {
    kind: 'call',
    call: {
      toolCallId: id,
      toolName,
      arguments: args,
    },
    seq,
  }
}

describe('copSubSegment web search titles', () => {
  it('把 web_search 归为搜索分类，而不是 generic', () => {
    expect(categoryForTool('web_search')).toBe('search')
    expect(categoryForTool('web_search.tavily')).toBe('search')
  })

  it('live adaptive title 使用搜索语义和 query，不裸露工具名', () => {
    const segments = buildSubSegments([
      toolCall('ws1', 'web_search', 1, { query: 'rust crate niche' }),
    ])
    const openSegment = {
      ...segments[0]!,
      status: 'open' as const,
      title: 'Searching...',
    }

    expect(presentToProgressive('web_search', { query: 'rust crate niche' })).toBe('Searching for rust crate niche')
    expect(aggregateMainTitle([openSegment], true, false)).toBe('Searching for rust crate niche...')
    expect(aggregateMainTitle([openSegment], true, false)).not.toContain('web_search')
  })

  it('完成态保留搜索标题，不退化成 step completed', () => {
    const segments = buildSubSegments([
      toolCall('ws1', 'web_search', 1, { query: 'rust crate niche' }),
    ])

    expect(segments[0]?.title).toBe('Searched for rust crate niche')
    expect(aggregateMainTitle(segments, false, true)).toBe('Searched for rust crate niche')
    expect(aggregateMainTitle(segments, false, true)).not.toBe('1 step completed')
  })

  it('多次搜索完成态显示搜索数量，不退化成 steps completed', () => {
    const segments = buildSubSegments([
      toolCall('ws1', 'web_search', 1, { query: 'rust crate niche' }),
      toolCall('ws2', 'web_search', 2, { query: 'python library rewrite' }),
    ])

    expect(aggregateMainTitle(segments, false, true)).toBe('Searched for rust crate niche +1')
    expect(aggregateMainTitle(segments, false, true)).not.toBe('2 steps completed')
  })
})
