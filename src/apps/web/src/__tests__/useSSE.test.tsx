import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useAgentStream, type UseAgentStreamResult } from '../hooks/useAgentStream'
import type { AgentClient } from '../agent-ui'

const mockedReadLastSeqFromStorage = vi.hoisted(() => vi.fn())
const mockedWriteLastSeqToStorage = vi.hoisted(() => vi.fn())
const mockedClearLastSeqInStorage = vi.hoisted(() => vi.fn())

vi.mock('../storage', () => ({
  readLastSeqFromStorage: mockedReadLastSeqFromStorage,
  writeLastSeqToStorage: mockedWriteLastSeqToStorage,
  clearLastSeqInStorage: mockedClearLastSeqInStorage,
}))

vi.mock('../streamDebug', () => ({
  emitStreamDebug: vi.fn(),
}))

const mockedOpenMessageChunkStream = vi.fn()

function pendingStream(cancel: () => void = vi.fn()): ReadableStream<never> {
  return new ReadableStream<never>({
    cancel,
  })
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
    openMessageChunkStream: mockedOpenMessageChunkStream,
  }
}

function HookProbe({
  runId,
  client,
  onSnapshot,
}: {
  runId: string
  client: AgentClient
  onSnapshot: (value: UseAgentStreamResult) => void
}) {
  const value = useAgentStream({ runId, client })
  onSnapshot(value)
  return null
}

describe('useAgentStream', () => {
  const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }
  const originalActEnvironment = actEnvironment.IS_REACT_ACT_ENVIRONMENT

  beforeEach(() => {
    vi.clearAllMocks()
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
  })

  afterEach(() => {
    if (originalActEnvironment === undefined) {
      delete actEnvironment.IS_REACT_ACT_ENVIRONMENT
    } else {
      actEnvironment.IS_REACT_ACT_ENVIRONMENT = originalActEnvironment
    }
  })

  it('切换 runId 后应关闭旧 client，并使用新 run 的 last seq 建连', async () => {
    const firstCancel = vi.fn()
    const secondCancel = vi.fn()
    const agentClient = createMockAgentClient()
    mockedOpenMessageChunkStream
      .mockReturnValueOnce(pendingStream(firstCancel))
      .mockReturnValueOnce(pendingStream(secondCancel))
    mockedReadLastSeqFromStorage.mockImplementation((runId: string) => (
      runId === 'run-1' ? 7 : 3
    ))

    let latest: UseAgentStreamResult | null = null
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)

    await act(async () => {
      root.render(<HookProbe runId="run-1" client={agentClient} onSnapshot={(value) => { latest = value }} />)
    })

    await act(async () => {
      latest?.connect()
      await Promise.resolve()
    })

    expect(mockedOpenMessageChunkStream).toHaveBeenNthCalledWith(1, 'run-1', expect.objectContaining({
      cursor: 7,
      live: true,
    }))

    await act(async () => {
      root.render(<HookProbe runId="run-2" client={agentClient} onSnapshot={(value) => { latest = value }} />)
    })

    await act(async () => {
      latest?.connect()
      await Promise.resolve()
    })

    expect(firstCancel).toHaveBeenCalledTimes(1)
    expect(mockedOpenMessageChunkStream).toHaveBeenNthCalledWith(2, 'run-2', expect.objectContaining({
      cursor: 3,
      live: true,
    }))

    act(() => root.unmount())
    expect(secondCancel).toHaveBeenCalledTimes(1)
    container.remove()
  })

  it('reconnect 应重建当前 run 的 chunk stream', async () => {
    const firstCancel = vi.fn()
    const secondCancel = vi.fn()
    const agentClient = createMockAgentClient()
    mockedOpenMessageChunkStream
      .mockReturnValueOnce(pendingStream(firstCancel))
      .mockReturnValueOnce(pendingStream(secondCancel))
    mockedReadLastSeqFromStorage.mockReturnValue(0)

    let latest: UseAgentStreamResult | null = null
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)

    await act(async () => {
      root.render(<HookProbe runId="run-1" client={agentClient} onSnapshot={(value) => { latest = value }} />)
    })
    await act(async () => {
      latest?.connect()
      await Promise.resolve()
    })

    await act(async () => {
      latest?.reconnect()
      await Promise.resolve()
    })

    expect(firstCancel).toHaveBeenCalledTimes(1)
    expect(mockedOpenMessageChunkStream).toHaveBeenNthCalledWith(2, 'run-1', expect.objectContaining({
      cursor: 0,
      live: true,
    }))

    act(() => root.unmount())
    expect(secondCancel).toHaveBeenCalledTimes(1)
    container.remove()
  })
})
