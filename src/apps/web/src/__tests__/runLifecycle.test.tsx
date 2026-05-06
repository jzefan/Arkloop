import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { RunLifecycleProvider, useRunLifecycle } from '../contexts/run-lifecycle'
import type { AgentClient } from '../agent-ui'

const mockedOpenEventStream = vi.hoisted(() => vi.fn())
const mockedUseAgentClient = vi.hoisted(() => vi.fn())

vi.mock('../agent-ui', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../agent-ui')>()
  return {
    ...actual,
    useAgentClient: mockedUseAgentClient,
  }
})

vi.mock('../contexts/chat-session', () => ({
  useChatSession: () => ({ threadId: 'thread_1', isSearchThread: false }),
}))

vi.mock('../streamDebug', () => ({
  emitStreamDebug: vi.fn(),
}))

vi.mock('../storage', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../storage')>()
  return {
    ...actual,
    readLastSeqFromStorage: vi.fn(() => 0),
    writeLastSeqToStorage: vi.fn(),
    clearLastSeqInStorage: vi.fn(),
  }
})

function pendingEventStream(): ReadableStream<never> {
  return new ReadableStream<never>()
}

function createMockAgentClient(): AgentClient {
  return {
    listMessages: vi.fn(),
    createMessage: vi.fn(),
    createRun: vi.fn(),
    editMessage: vi.fn(),
    retryMessage: vi.fn(),
    cancelRun: vi.fn(),
    provideInput: vi.fn(),
    openEventStream: mockedOpenEventStream,
    openMessageChunkStream: vi.fn(),
  }
}

function StartRunProbe() {
  const lifecycle = useRunLifecycle()
  return (
    <button type="button" onClick={() => lifecycle.setActiveRunId('run_1')}>
      start
    </button>
  )
}

describe('RunLifecycleProvider', () => {
  beforeEach(() => {
    mockedOpenEventStream.mockReset()
    mockedOpenEventStream.mockReturnValue(pendingEventStream())
    mockedUseAgentClient.mockReset()
    mockedUseAgentClient.mockReturnValue(createMockAgentClient())
  })

  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('activeRunId 启动时只打开一条 event stream', async () => {
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)

    await act(async () => {
      root.render(
        <RunLifecycleProvider>
          <StartRunProbe />
        </RunLifecycleProvider>,
      )
    })

    await act(async () => {
      container.querySelector('button')?.dispatchEvent(new MouseEvent('click', { bubbles: true }))
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(mockedOpenEventStream).toHaveBeenCalledTimes(1)
    expect(mockedOpenEventStream).toHaveBeenCalledWith('run_1', expect.objectContaining({
      cursor: 0,
      live: true,
    }))

    act(() => root.unmount())
  })
})
