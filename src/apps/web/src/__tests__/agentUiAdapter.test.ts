import { describe, expect, it, vi } from 'vitest'

const mockedCreateMessage = vi.hoisted(() => vi.fn())
const mockedCreateRun = vi.hoisted(() => vi.fn())
const mockedListMessages = vi.hoisted(() => vi.fn())
const mockedListRunEvents = vi.hoisted(() => vi.fn())
const mockedCreateSSEClient = vi.hoisted(() => vi.fn())

vi.mock('../api', () => ({
  createMessage: mockedCreateMessage,
  createRun: mockedCreateRun,
  listMessages: mockedListMessages,
  listRunEvents: mockedListRunEvents,
}))

vi.mock('../sse', () => ({
  createSSEClient: mockedCreateSSEClient,
}))

import { createArkloopAgentClient, readAgentUIEvents } from '../agent-ui'

describe('createArkloopAgentClient', () => {
  it('委托消息与 run API，并映射为 agent UI contract', async () => {
    const message = {
      id: 'msg_1',
      role: 'user',
      content: 'hello',
      created_at: '2026-01-01T00:00:00Z',
    }
    const run = { run_id: 'run_1', trace_id: 'trace_1' }
    const agentClient = createArkloopAgentClient({ accessToken: 'token' })
    mockedListMessages.mockResolvedValue([message])
    mockedCreateMessage.mockResolvedValue(message)
    mockedCreateRun.mockResolvedValue(run)

    await expect(agentClient.listMessages('thread_1', 25)).resolves.toEqual([expect.objectContaining({
      id: 'msg_1',
      role: 'user',
      content: 'hello',
      createdAt: '2026-01-01T00:00:00Z',
      parts: [{ type: 'text', text: 'hello', state: 'done' }],
    })])
    await expect(agentClient.createMessage({
      threadId: 'thread_1',
      request: { content: 'hello' },
    })).resolves.toEqual(expect.objectContaining({ id: 'msg_1', createdAt: '2026-01-01T00:00:00Z' }))
    await expect(agentClient.createRun({
      threadId: 'thread_1',
      personaId: 'persona_1',
      modelOverride: 'model_1',
      workDir: '/tmp/work',
      reasoningMode: 'medium',
      options: { resumeFromRunId: 'run_0' },
    })).resolves.toEqual({ id: 'run_1', traceId: 'trace_1' })
    expect(mockedListMessages).toHaveBeenCalledWith('token', 'thread_1', 25)
    expect(mockedCreateMessage).toHaveBeenCalledWith('token', 'thread_1', { content: 'hello' })
    expect(mockedCreateRun).toHaveBeenCalledWith(
      'token',
      'thread_1',
      'persona_1',
      'model_1',
      '/tmp/work',
      'medium',
      { resumeFromRunId: 'run_0' },
    )
  })

  it('将 Arkloop SSE 映射为 Agent UI chunk stream', async () => {
    const refreshAccessToken = vi.fn(async () => 'fresh-token')
    const agentClient = createArkloopAgentClient({
      accessToken: 'token',
      baseUrl: 'http://api.test/',
      refreshAccessToken,
    })
    mockedCreateSSEClient.mockImplementation((options) => ({
      connect: vi.fn(async () => {
        options.onEvent({
          event_id: 'evt_1',
          run_id: 'run_1',
          seq: 4,
          ts: '2026-01-01T00:00:00Z',
          type: 'run.completed',
          data: {},
        })
      }),
      close: vi.fn(),
    }))

    const events = await readAgentUIEvents(agentClient.openMessageChunkStream('run_1', {
      cursor: 3,
      live: true,
    }))

    expect(events).toEqual([{
      id: 'evt_1',
      streamId: 'run_1',
      order: 4,
      timestamp: '2026-01-01T00:00:00Z',
      type: 'run-completed',
      data: {},
      toolName: undefined,
      errorCode: undefined,
    }])

    expect(mockedCreateSSEClient).toHaveBeenCalledWith(expect.objectContaining({
      url: 'http://api.test/v1/runs/run_1/events',
      accessToken: 'token',
      afterSeq: 3,
      follow: true,
      onTokenRefresh: refreshAccessToken,
    }))
  })
})
