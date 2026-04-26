import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { CopTimeline } from '../components/cop-timeline/CopTimeline'
import { CopTimelineHeaderLabel } from '../components/cop-timeline/CopTimelineHeader'
import type { CopSubSegment, ResolvedPool } from '../copSubSegment'
import { EMPTY_POOL } from '../copSubSegment'
import { LocaleProvider } from '../contexts/LocaleContext'

vi.mock('../hooks/useTypewriter', () => ({
  useTypewriter: (text: string) => text,
}))

vi.mock('../hooks/useIncrementalTypewriter', () => ({
  useIncrementalTypewriter: (text: string) => text,
}))

globalThis.scrollTo = (() => {}) as typeof globalThis.scrollTo

const originalMatchMedia = window.matchMedia
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }
const originalActEnvironment = actEnvironment.IS_REACT_ACT_ENVIRONMENT

function reducedMotionMatchMedia(query: string) {
  return {
    matches: query === '(prefers-reduced-motion: reduce)',
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(() => false),
  }
}

function defaultMatchMedia(query: string) {
  return {
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(() => false),
  }
}

beforeEach(() => {
  actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
  window.matchMedia = vi.fn(defaultMatchMedia)
})

afterEach(() => {
  window.matchMedia = originalMatchMedia
  if (originalActEnvironment === undefined) {
    delete actEnvironment.IS_REACT_ACT_ENVIRONMENT
  } else {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = originalActEnvironment
  }
})

function makePool(overrides?: Partial<ResolvedPool>): ResolvedPool {
  return { ...EMPTY_POOL, ...overrides }
}

function makeSeg(overrides?: Partial<CopSubSegment>): CopSubSegment {
  return {
    id: 'seg1',
    category: 'exec',
    status: 'closed',
    items: [],
    seq: 0,
    title: 'Test segment',
    ...overrides,
  }
}

function renderTimeline(props: Parameters<typeof CopTimeline>[0]): string {
  const prev = window.matchMedia
  window.matchMedia = vi.fn(reducedMotionMatchMedia)
  try {
    return renderToStaticMarkup(
      <LocaleProvider>
        <CopTimeline {...props} />
      </LocaleProvider>,
    )
  } finally {
    window.matchMedia = prev
  }
}

async function renderTimelineDom(props: Parameters<typeof CopTimeline>[0]) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  await act(async () => {
    root.render(
      <LocaleProvider>
        <CopTimeline {...props} />
      </LocaleProvider>,
    )
  })
  return {
    container,
    rerender: async (next: Parameters<typeof CopTimeline>[0]) => {
      await act(async () => {
        root.render(
          <LocaleProvider>
            <CopTimeline {...next} />
          </LocaleProvider>,
        )
      })
    },
    cleanup: () => {
      act(() => { root.unmount() })
      container.remove()
    },
  }
}

async function renderHeaderLabelDom(params: { text: string; phaseKey?: string; incremental?: boolean }) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  await act(async () => {
    root.render(
      <LocaleProvider>
        <CopTimelineHeaderLabel
          text={params.text}
          phaseKey={params.phaseKey ?? 'test'}
          incremental={params.incremental}
        />
      </LocaleProvider>,
    )
  })
  return {
    container,
    cleanup: () => {
      act(() => { root.unmount() })
      container.remove()
    },
  }
}

describe('CopTimeline', () => {
  describe('shouldRender gate', () => {
    it('empty segments, no thinkingOnly, no thinkingHint -> renders null', () => {
      const html = renderTimeline({
        segments: [],
        pool: EMPTY_POOL,
        isComplete: false,
      })
      expect(html).toBe('')
    })

    it('thinkingHint + live with no segments -> renders pending header', async () => {
      const { container, cleanup } = await renderTimelineDom({
        segments: [],
        pool: EMPTY_POOL,
        isComplete: false,
        live: true,
        thinkingHint: 'Planning next moves',
      })
      try {
        const span = container.querySelector('[data-phase="thinking-pending"]')
        expect(span).not.toBeNull()
        expect(span?.textContent).toContain('Planning next moves')
      } finally {
        cleanup()
      }
    })

    it('non-empty pool with no segments still renders', () => {
      const pool = makePool({
        codeExecutions: new Map([['ce1', { id: 'ce1', code: 'echo hi' } as unknown as import('../storage').CodeExecutionRef]]),
      })
      const html = renderTimeline({
        segments: [],
        pool,
        isComplete: true,
      })
      expect(html).not.toBe('')
    })
  })

  describe('thinking-only mode', () => {
    it('complete thinkingOnly collapses body by default', () => {
      const html = renderTimeline({
        segments: [],
        pool: EMPTY_POOL,
        thinkingOnly: { markdown: 'Let me think', durationSec: 3, live: false },
        isComplete: true,
      })
      expect(html).toContain('data-phase="thought"')
      // framer-motion SSR renders collapsed state as height:0
      expect(html).toContain('height:0')
    })

    it('complete thinkingOnly with durationSec > 0 shows thoughtDuration label', () => {
      const html = renderTimeline({
        segments: [],
        pool: EMPTY_POOL,
        thinkingOnly: { markdown: 'some thinking', durationSec: 5, live: false },
        isComplete: true,
      })
      expect(html).toContain('Thought for 5s')
    })

    it('complete thinkingOnly with durationSec=0 shows "Thought"', () => {
      const html = renderTimeline({
        segments: [],
        pool: EMPTY_POOL,
        thinkingOnly: { markdown: 'think', durationSec: 0, live: false },
        isComplete: true,
      })
      expect(html).toContain('Thought')
    })

    it('live thinkingOnly shows thinking-live phase key', async () => {
      const { container, cleanup } = await renderTimelineDom({
        segments: [],
        pool: EMPTY_POOL,
        thinkingOnly: { markdown: 'live thought', durationSec: 0, live: true },
        isComplete: false,
        live: true,
      })
      try {
        const span = container.querySelector('[data-phase="thinking-live"]')
        expect(span).not.toBeNull()
      } finally {
        cleanup()
      }
    })

    it('live thinkingOnly with hint shows hint in header', async () => {
      const { container, cleanup } = await renderTimelineDom({
        segments: [],
        pool: EMPTY_POOL,
        thinkingOnly: { markdown: 'live thought', durationSec: 0, live: true, startedAtMs: Date.now() },
        isComplete: false,
        live: true,
        thinkingHint: 'Working through this',
      })
      try {
        const span = container.querySelector('[data-phase="thinking-live"]')
        expect(span?.textContent).toContain('Working through this')
      } finally {
        cleanup()
      }
    })
  })

  describe('segment rendering', () => {
    it('single exec segment with closed status renders', () => {
      const seg = makeSeg({
        id: 'seg1',
        category: 'exec',
        status: 'closed',
        title: '1 steps completed',
        items: [{ kind: 'call', call: { toolCallId: 'ce1', toolName: 'exec_command', arguments: {} }, seq: 0 }],
      })
      const html = renderTimeline({
        segments: [seg],
        pool: EMPTY_POOL,
        isComplete: true,
      })
      expect(html).not.toBe('')
      expect(html).toContain('cop-timeline-root')
    })

    it('multiple segments rendered in order', () => {
      const seg1 = makeSeg({ id: 'seg1', title: 'First segment', seq: 0, status: 'closed' })
      const seg2 = makeSeg({ id: 'seg2', title: 'Second segment', seq: 1, status: 'closed', category: 'edit' })
      const html = renderTimeline({
        segments: [seg1, seg2],
        pool: EMPTY_POOL,
        isComplete: true,
      })
      const idx1 = html.indexOf('First segment')
      const idx2 = html.indexOf('Second segment')
      expect(idx1).toBeGreaterThan(-1)
      expect(idx2).toBeGreaterThan(-1)
      expect(idx1).toBeLessThan(idx2)
    })

    it('segment with status=open shows its title', () => {
      const seg = makeSeg({
        id: 'seg1',
        category: 'explore',
        status: 'open',
        title: 'Exploring code...',
        items: [],
      })
      const html = renderTimeline({
        segments: [seg],
        pool: EMPTY_POOL,
        isComplete: false,
        live: true,
      })
      expect(html).toContain('Exploring code...')
    })

    it('segment with status=closed shows its completed title', () => {
      const seg = makeSeg({
        id: 'seg1',
        category: 'exec',
        status: 'closed',
        title: '2 steps completed',
        items: [
          { kind: 'call', call: { toolCallId: 'c1', toolName: 'exec_command', arguments: {} }, seq: 0 },
          { kind: 'call', call: { toolCallId: 'c2', toolName: 'exec_command', arguments: {} }, seq: 1 },
        ],
      })
      const html = renderTimeline({
        segments: [seg],
        pool: EMPTY_POOL,
        isComplete: true,
      })
      expect(html).toContain('2 steps completed')
    })
  })

  describe('header label and phaseKey', () => {
    it('isComplete with segments -> data-phase="complete"', () => {
      const seg = makeSeg({ id: 's1', status: 'closed', title: '1 steps completed' })
      const html = renderTimeline({
        segments: [seg],
        pool: EMPTY_POOL,
        isComplete: true,
      })
      expect(html).toContain('data-phase="complete"')
    })

    it('headerOverride overrides auto-generated label', () => {
      const seg = makeSeg({ id: 's1', status: 'closed', title: '1 steps completed' })
      const html = renderTimeline({
        segments: [seg],
        pool: EMPTY_POOL,
        isComplete: true,
        headerOverride: 'Custom Header Text',
      })
      expect(html).toContain('Custom Header Text')
    })

    it('live with open segments -> data-phase="live"', () => {
      const seg = makeSeg({ id: 's1', status: 'open', title: 'Working...' })
      const html = renderTimeline({
        segments: [seg],
        pool: EMPTY_POOL,
        isComplete: false,
        live: true,
      })
      expect(html).toContain('data-phase="live"')
    })

    it('live with open last segment -> header shows that segment title', () => {
      const seg1 = makeSeg({ id: 's1', status: 'closed', title: 'Read 1 file', seq: 0 })
      const seg2 = makeSeg({ id: 's2', status: 'open', title: 'Running...', seq: 1, category: 'exec' })
      const html = renderTimeline({
        segments: [seg1, seg2],
        pool: EMPTY_POOL,
        isComplete: false,
        live: true,
      })
      expect(html).toContain('Running...')
    })

    it('thinkingOnly + live -> data-phase="thinking-live"', () => {
      const html = renderTimeline({
        segments: [],
        pool: EMPTY_POOL,
        thinkingOnly: { markdown: 'Hmm', durationSec: 0, live: true },
        isComplete: false,
        live: true,
      })
      expect(html).toContain('data-phase="thinking-live"')
    })

    it('thinkingOnly + complete -> data-phase="thought"', () => {
      const html = renderTimeline({
        segments: [],
        pool: EMPTY_POOL,
        thinkingOnly: { markdown: 'Hmm', durationSec: 2, live: false },
        isComplete: true,
      })
      expect(html).toContain('data-phase="thought"')
    })

    it('thinkingHint pending (no segments, no thinkingOnly, live) -> data-phase="thinking-pending"', () => {
      const html = renderTimeline({
        segments: [],
        pool: EMPTY_POOL,
        isComplete: false,
        live: true,
        thinkingHint: 'Planning next steps',
      })
      expect(html).toContain('data-phase="thinking-pending"')
      expect(html).toContain('Planning next steps...')
    })

    it('complete with N segments -> header shows step count', () => {
      const seg1 = makeSeg({ id: 's1', status: 'closed', title: 'Step 1', seq: 0 })
      const seg2 = makeSeg({ id: 's2', status: 'closed', title: 'Step 2', seq: 1, category: 'edit' })
      const html = renderTimeline({
        segments: [seg1, seg2],
        pool: EMPTY_POOL,
        isComplete: true,
      })
      expect(html).toContain('2 steps completed')
    })

    it('complete with 1 segment uses segment title', () => {
      const seg = makeSeg({ id: 's1', status: 'closed', title: 'Step 1', seq: 0 })
      const html = renderTimeline({
        segments: [seg],
        pool: EMPTY_POOL,
        isComplete: true,
      })
      expect(html).toContain('Step 1')
      expect(html).not.toContain('1 step completed')
    })
  })

  describe('collapse and expand behavior', () => {
    it('complete timeline starts collapsed', async () => {
      const seg = makeSeg({ id: 's1', status: 'closed', title: '1 step completed', items: [
        { kind: 'call', call: { toolCallId: 'c1', toolName: 'exec_command', arguments: {} }, seq: 0 },
      ]})
      const { container, cleanup } = await renderTimelineDom({
        segments: [seg],
        pool: EMPTY_POOL,
        isComplete: true,
      })
      try {
        const root = container.querySelector('.cop-timeline-root')
        expect(root).not.toBeNull()
        const motionDiv = container.querySelector('.cop-timeline-root > div:last-child') as HTMLElement | null
        expect(motionDiv?.style.overflow).toBe('hidden')
      } finally {
        cleanup()
      }
    })

    it('user click on header expands a collapsed timeline', async () => {
      const seg = makeSeg({ id: 's1', status: 'closed', title: '1 step completed', items: [
        { kind: 'call', call: { toolCallId: 'c1', toolName: 'exec_command', arguments: {} }, seq: 0 },
      ]})
      const { container, cleanup } = await renderTimelineDom({
        segments: [seg],
        pool: EMPTY_POOL,
        isComplete: true,
      })
      try {
        const button = container.querySelector('button')!
        // collapsed: overflow hidden
        const motionBefore = container.querySelector('.cop-timeline-root > div:last-child') as HTMLElement | null
        expect(motionBefore?.style.overflow).toBe('hidden')

        await act(async () => { button.click() })

        // expanded: overflow visible
        const motionAfter = container.querySelector('.cop-timeline-root > div:last-child') as HTMLElement | null
        expect(motionAfter?.style.overflow).toBe('visible')
      } finally {
        cleanup()
      }
    })

    it('transition from live to complete auto-collapses', async () => {
      const seg = makeSeg({ id: 's1', status: 'open', title: 'Running...', items: [
        { kind: 'call', call: { toolCallId: 'c1', toolName: 'exec_command', arguments: {} }, seq: 0 },
      ]})
      const { container, rerender, cleanup } = await renderTimelineDom({
        segments: [seg],
        pool: EMPTY_POOL,
        isComplete: false,
        live: true,
      })
      try {
        // live = expanded: overflow visible
        const motionBefore = container.querySelector('.cop-timeline-root > div:last-child') as HTMLElement | null
        expect(motionBefore?.style.overflow).toBe('visible')

        const closedSeg = { ...seg, status: 'closed' as const, title: '1 step completed' }
        await rerender({
          segments: [closedSeg],
          pool: EMPTY_POOL,
          isComplete: true,
          live: false,
        })

        // complete = collapsed: overflow hidden
        const motionAfter = container.querySelector('.cop-timeline-root > div:last-child') as HTMLElement | null
        expect(motionAfter?.style.overflow).toBe('hidden')
      } finally {
        cleanup()
      }
    })
  })

  describe('mixed thinking + segments', () => {
    it('thinkingOnly + segments complete -> step count shown in header', () => {
      const seg = makeSeg({ id: 's1', status: 'closed', title: '1 step completed' })
      const html = renderTimeline({
        segments: [seg],
        pool: EMPTY_POOL,
        thinkingOnly: { markdown: 'Thought about it', durationSec: 4, live: false },
        isComplete: true,
      })
      expect(html).toContain('1 step completed')
      expect(html).toContain('data-phase="thought"')
    })

    it('thinkingOnly + live segments -> thinking-live phase', () => {
      const seg = makeSeg({ id: 's1', status: 'open', title: 'Running...' })
      const html = renderTimeline({
        segments: [seg],
        pool: EMPTY_POOL,
        thinkingOnly: { markdown: 'thinking...', durationSec: 0, live: true },
        isComplete: false,
        live: true,
      })
      expect(html).toContain('data-phase="thinking-live"')
    })

    it('thinkingOnly + multiple segments complete -> N steps label', () => {
      const seg1 = makeSeg({ id: 's1', status: 'closed', seq: 0 })
      const seg2 = makeSeg({ id: 's2', status: 'closed', seq: 1, category: 'edit' })
      const seg3 = makeSeg({ id: 's3', status: 'closed', seq: 2, category: 'fetch' })
      const html = renderTimeline({
        segments: [seg1, seg2, seg3],
        pool: EMPTY_POOL,
        thinkingOnly: { markdown: 'thought', durationSec: 2, live: false },
        isComplete: true,
      })
      expect(html).toContain('3 steps completed')
    })
  })

  describe('edge cases', () => {
    it('segment with no pool data still renders', () => {
      const seg = makeSeg({
        id: 's1',
        category: 'generic',
        status: 'closed',
        title: 'todo_write',
        items: [{ kind: 'call', call: { toolCallId: 'g1', toolName: 'todo_write', arguments: {} }, seq: 0 }],
      })
      const html = renderTimeline({
        segments: [seg],
        pool: EMPTY_POOL,
        isComplete: true,
      })
      expect(html).not.toBe('')
    })

    it('headerOverride takes priority over thinkingOnly label', () => {
      const html = renderTimeline({
        segments: [],
        pool: EMPTY_POOL,
        thinkingOnly: { markdown: 'some thought', durationSec: 5, live: false },
        isComplete: true,
        headerOverride: 'Override wins',
      })
      expect(html).toContain('Override wins')
      expect(html).not.toContain('Thought for 5s')
    })
  })
})

describe('CopTimelineHeaderLabel', () => {
  it('renders text with data-phase attribute', async () => {
    const { container, cleanup } = await renderHeaderLabelDom({
      text: 'Thinking for 3s',
      phaseKey: 'thinking-live',
    })
    try {
      const span = container.querySelector('[data-phase="thinking-live"]')
      expect(span).not.toBeNull()
      expect(span?.textContent).toBe('Thinking for 3s')
    } finally {
      cleanup()
    }
  })

  it('renders with shimmer class when shimmer=true', () => {
    const html = renderToStaticMarkup(
      <LocaleProvider>
        <CopTimelineHeaderLabel text="Working..." phaseKey="live" shimmer />
      </LocaleProvider>,
    )
    expect(html).toContain('thinking-shimmer')
  })

  it('renders without shimmer class when shimmer=false', () => {
    const html = renderToStaticMarkup(
      <LocaleProvider>
        <CopTimelineHeaderLabel text="Done" phaseKey="complete" shimmer={false} />
      </LocaleProvider>,
    )
    expect(html).not.toContain('thinking-shimmer')
  })
})
