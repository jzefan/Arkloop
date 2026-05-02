import { beforeEach, describe, expect, it, vi } from 'vitest'

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

import { createArkloopAgentClient, readAgentUIEvents, type AgentUIMessageChunk } from '../agent-ui'

beforeEach(() => {
  mockedCreateMessage.mockReset()
  mockedCreateRun.mockReset()
  mockedListMessages.mockReset()
  mockedListRunEvents.mockReset()
  mockedCreateSSEClient.mockReset()
})

async function readChunks(stream: ReadableStream<AgentUIMessageChunk>): Promise<AgentUIMessageChunk[]> {
  const reader = stream.getReader()
  const chunks: AgentUIMessageChunk[] = []
  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      chunks.push(value)
    }
  } finally {
    reader.releaseLock()
  }
  return chunks
}

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

  it('过滤不可见 message.delta 时仍保留 agent event', async () => {
    const agentClient = createArkloopAgentClient({ accessToken: 'token' })
    mockedListRunEvents.mockResolvedValue([
      {
        event_id: 'evt_1',
        run_id: 'run_1',
        seq: 1,
        ts: '2026-01-01T00:00:00Z',
        type: 'message.delta',
        data: { role: 'assistant', content_delta: '' },
      },
    ])

    const chunks = await readChunks(agentClient.openMessageChunkStream('run_1', { live: false }))

    expect(chunks).toContainEqual(expect.objectContaining({
      type: 'data-agent-event',
      id: 'evt_1',
      data: expect.objectContaining({
        type: 'assistant-delta',
        data: { role: 'assistant', delta: '' },
      }),
    }))
    expect(chunks.some((chunk) => chunk.type === 'text-delta')).toBe(false)
    expect(chunks.some((chunk) => chunk.type === 'reasoning-delta')).toBe(false)
  })

  it('tool-only stream 不创建空 text/reasoning part', async () => {
    const agentClient = createArkloopAgentClient({ accessToken: 'token' })
    mockedListRunEvents.mockResolvedValue([
      {
        event_id: 'evt_1',
        run_id: 'run_1',
        seq: 1,
        ts: '2026-01-01T00:00:00Z',
        type: 'tool.call',
        tool_name: 'web_search',
        data: { tool_call_id: 'call_1', tool_name: 'web_search', arguments: { query: 'arkloop' } },
      },
      {
        event_id: 'evt_2',
        run_id: 'run_1',
        seq: 2,
        ts: '2026-01-01T00:00:01Z',
        type: 'run.completed',
        data: {},
      },
    ])

    const chunks = await readChunks(agentClient.openMessageChunkStream('run_1', { live: false }))

    expect(chunks.some((chunk) => chunk.type === 'text-start')).toBe(false)
    expect(chunks.some((chunk) => chunk.type === 'reasoning-start')).toBe(false)
    expect(chunks).toContainEqual({
      type: 'tool-input-available',
      toolCallId: 'call_1',
      toolName: 'web_search',
      input: { query: 'arkloop' },
    })
  })

  it('tool.call.delta 进入 UIMessageChunk tool input 生命周期', async () => {
    const agentClient = createArkloopAgentClient({ accessToken: 'token' })
    mockedListRunEvents.mockResolvedValue([
      {
        event_id: 'evt_1',
        run_id: 'run_1',
        seq: 1,
        ts: '2026-01-01T00:00:00Z',
        type: 'tool.call.delta',
        data: {
          tool_call_index: 0,
          tool_call_id: 'call_1',
          tool_name: 'show_widget',
          arguments_delta: '{"title"',
        },
      },
      {
        event_id: 'evt_2',
        run_id: 'run_1',
        seq: 2,
        ts: '2026-01-01T00:00:01Z',
        type: 'tool.call.delta',
        data: {
          tool_call_index: 0,
          tool_call_id: 'call_1',
          tool_name: 'show_widget',
          arguments_delta: ':"Chart"}',
        },
      },
      {
        event_id: 'evt_3',
        run_id: 'run_1',
        seq: 3,
        ts: '2026-01-01T00:00:02Z',
        type: 'tool.call',
        tool_name: 'show_widget',
        data: { tool_call_id: 'call_1', tool_name: 'show_widget', arguments: { title: 'Chart' } },
      },
    ])

    const chunks = await readChunks(agentClient.openMessageChunkStream('run_1', { live: false }))

    expect(chunks.filter((chunk) => chunk.type === 'tool-input-start')).toEqual([{
      type: 'tool-input-start',
      toolCallId: 'call_1',
      toolName: 'show_widget',
      dynamic: true,
    }])
    expect(chunks.filter((chunk) => chunk.type === 'tool-input-delta')).toEqual([
      { type: 'tool-input-delta', toolCallId: 'call_1', inputTextDelta: '{"title"' },
      { type: 'tool-input-delta', toolCallId: 'call_1', inputTextDelta: ':"Chart"}' },
    ])
    expect(chunks).toContainEqual({
      type: 'tool-input-available',
      toolCallId: 'call_1',
      toolName: 'show_widget',
      input: { title: 'Chart' },
    })
  })

  it('tool.call.delta 缺少 call id 时不创建临时 tool part', async () => {
    const agentClient = createArkloopAgentClient({ accessToken: 'token' })
    mockedListRunEvents.mockResolvedValue([
      {
        event_id: 'evt_1',
        run_id: 'run_1',
        seq: 1,
        ts: '2026-01-01T00:00:00Z',
        type: 'tool.call.delta',
        data: {
          tool_call_index: 0,
          tool_name: 'show_widget',
          arguments_delta: '{"title"',
        },
      },
      {
        event_id: 'evt_2',
        run_id: 'run_1',
        seq: 2,
        ts: '2026-01-01T00:00:01Z',
        type: 'tool.call',
        tool_name: 'show_widget',
        data: { tool_call_id: 'call_1', tool_name: 'show_widget', arguments: { title: 'Chart' } },
      },
    ])

    const chunks = await readChunks(agentClient.openMessageChunkStream('run_1', { live: false }))

    expect(chunks.some((chunk) =>
      (chunk.type === 'tool-input-start' || chunk.type === 'tool-input-delta') &&
      chunk.toolCallId.startsWith('tool-index-'),
    )).toBe(false)
    expect(chunks).toContainEqual({
      type: 'tool-input-start',
      toolCallId: 'call_1',
      toolName: 'show_widget',
      dynamic: true,
    })
    expect(chunks).toContainEqual({
      type: 'tool-input-delta',
      toolCallId: 'call_1',
      inputTextDelta: '{"title"',
    })
    expect(chunks).toContainEqual({
      type: 'tool-input-available',
      toolCallId: 'call_1',
      toolName: 'show_widget',
      input: { title: 'Chart' },
    })
  })
})
