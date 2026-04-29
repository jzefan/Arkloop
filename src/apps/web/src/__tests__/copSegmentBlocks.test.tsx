import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { CopSegmentBlocks } from '../components/CopSegmentBlocks'
import { LocaleProvider } from '../contexts/LocaleContext'
import type { AssistantTurnSegment } from '../assistantTurnSegments'

const originalMatchMedia = window.matchMedia
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }
const originalActEnvironment = actEnvironment.IS_REACT_ACT_ENVIRONMENT

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

async function renderBlocks(segment: Extract<AssistantTurnSegment, { type: 'cop' }>) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  await act(async () => {
    root.render(
      <LocaleProvider>
        <CopSegmentBlocks
          segment={segment}
          keyPrefix="test"
          fileOps={[{ id: 'read-1', toolName: 'read_file', label: 'Read app.tsx', status: 'success', seq: 2, filePath: 'app.tsx', displayKind: 'read' }]}
          sources={[]}
          isComplete
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

describe('CopSegmentBlocks', () => {
  it('renders exec_command as a top-level sibling, not inside CopTimeline', async () => {
    const { container, cleanup } = await renderBlocks({
      type: 'cop',
      title: null,
      items: [
        { kind: 'call', call: { toolCallId: 'cmd-1', toolName: 'exec_command', arguments: { command: 'pwd' } }, seq: 1 },
        { kind: 'call', call: { toolCallId: 'read-1', toolName: 'read', arguments: { file_path: 'app.tsx' } }, seq: 2 },
      ],
    })
    try {
      const timeline = container.querySelector('.cop-timeline-root')
      expect(container.textContent).toContain('pwd')
      expect(timeline).not.toBeNull()
      expect(timeline?.textContent).not.toContain('pwd')
    } finally {
      cleanup()
    }
  })

  it('renders todo_write as a top-level sibling, not inside CopTimeline', async () => {
    const { container, cleanup } = await renderBlocks({
      type: 'cop',
      title: null,
      items: [
        {
          kind: 'call',
          call: {
            toolCallId: 'todo-1',
            toolName: 'todo_write',
            arguments: {
              todos: [
                { id: 'a', content: 'Write focused test', status: 'completed' },
                { id: 'b', content: 'Wire the renderer', status: 'pending' },
              ],
            },
          },
          seq: 1,
        },
        { kind: 'call', call: { toolCallId: 'read-1', toolName: 'read', arguments: { file_path: 'app.tsx' } }, seq: 2 },
      ],
    })
    try {
      const timeline = container.querySelector('.cop-timeline-root')
      expect(container.textContent).toContain('Write focused test')
      expect(container.textContent).toContain('1 of 2 Done')
      expect(timeline).not.toBeNull()
      expect(timeline?.textContent).not.toContain('Write focused test')
    } finally {
      cleanup()
    }
  })
})
