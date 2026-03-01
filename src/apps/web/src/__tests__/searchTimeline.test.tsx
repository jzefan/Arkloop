import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { SearchTimeline } from '../components/SearchTimeline'
import type { WebSource } from '../storage'

function renderTimeline(params: {
  isComplete: boolean
  steps: { id: string; kind: 'planning' | 'searching' | 'reviewing' | 'finished'; label: string; status: 'active' | 'done'; queries?: string[] }[]
  sources: WebSource[]
}): string {
  return renderToStaticMarkup(
    <SearchTimeline steps={params.steps} sources={params.sources} isComplete={params.isComplete} />,
  )
}

describe('SearchTimeline', () => {
  it('isComplete=true 时应默认收起内容', () => {
    const html = renderTimeline({
      isComplete: true,
      steps: [
        { id: 's1', kind: 'planning', label: 'Plan step', status: 'done' },
        { id: 's2', kind: 'searching', label: 'Search step', status: 'done', queries: ['hello'] },
      ],
      sources: [{ title: 'Example', url: 'https://example.com' }],
    })

    expect(html).toContain('Reviewed 1 sources')
    expect(html).not.toContain('Plan step')
    expect(html).not.toContain('Search step')
  })

  it('isComplete=false 时应默认展开内容', () => {
    const html = renderTimeline({
      isComplete: false,
      steps: [
        { id: 's1', kind: 'planning', label: 'Plan step', status: 'done' },
        { id: 's2', kind: 'searching', label: 'Search step', status: 'active', queries: ['hello'] },
      ],
      sources: [{ title: 'Example', url: 'https://example.com' }],
    })

    expect(html).toContain('Plan step')
  })
})

