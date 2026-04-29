import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { bridgeClient } from '../api-bridge'

type DesktopGlobals = typeof globalThis & {
  __ARKLOOP_DESKTOP__?: {
    getAccessToken?: () => string | null
    getBridgeBaseUrl?: () => string | null
  }
}

const globals = globalThis as DesktopGlobals

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

function streamResponse(body: string): Response {
  const encoder = new TextEncoder()
  const stream = new ReadableStream({
    start(controller) {
      controller.enqueue(encoder.encode(body))
      controller.close()
    },
  })

  return new Response(stream, {
    status: 200,
    headers: { 'Content-Type': 'text/event-stream' },
  })
}

describe('bridge client', () => {
  beforeEach(() => {
    globals.__ARKLOOP_DESKTOP__ = {
      getAccessToken: () => 'desktop-token',
      getBridgeBaseUrl: () => 'http://bridge.test',
    }
  })

  afterEach(() => {
    vi.restoreAllMocks()
    delete globals.__ARKLOOP_DESKTOP__
  })

  it('只给受保护 bridge 请求携带 Authorization', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(jsonResponse({ status: 'ok' }))
      .mockResolvedValueOnce(jsonResponse([]))

    await bridgeClient.healthz()
    await bridgeClient.listModules()

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      'http://bridge.test/healthz',
      expect.not.objectContaining({
        headers: expect.anything(),
      }),
    )
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      'http://bridge.test/v1/modules',
      expect.objectContaining({
        headers: { Authorization: 'Bearer desktop-token' },
      }),
    )
  })

  it('没有 desktop token 时不构造空 Bearer', async () => {
    globals.__ARKLOOP_DESKTOP__ = {
      getAccessToken: () => null,
      getBridgeBaseUrl: () => 'http://bridge.test',
    }
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(jsonResponse([]))

    await bridgeClient.listModules()

    expect(fetchMock).toHaveBeenCalledWith(
      'http://bridge.test/v1/modules',
      expect.not.objectContaining({
        headers: expect.anything(),
      }),
    )
  })

  it('空 desktop bridge base URL 时使用默认 bridge 地址', async () => {
    globals.__ARKLOOP_DESKTOP__ = {
      getAccessToken: () => 'desktop-token',
      getBridgeBaseUrl: () => '',
    }
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(jsonResponse([]))

    await bridgeClient.listModules()

    expect(fetchMock).toHaveBeenCalledWith(
      'http://localhost:19003/v1/modules',
      expect.objectContaining({
        headers: { Authorization: 'Bearer desktop-token' },
      }),
    )
  })

  it('streamOperation 用 fetch 解析 status 事件', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      streamResponse(
        'event: log\n' +
          'data: booting\n\n' +
          'event: status\n' +
          'data: {"status":"completed"}\n\n',
      ),
    )
    const logs: string[] = []

    const result = await new Promise<{ status: string; error?: string }>((resolve) => {
      bridgeClient.streamOperation('operation-1', (line) => logs.push(line), resolve)
    })

    expect(fetchMock).toHaveBeenCalledWith(
      'http://bridge.test/v1/operations/operation-1/stream',
      expect.objectContaining({
        headers: { Authorization: 'Bearer desktop-token' },
      }),
    )
    expect(logs).toEqual(['booting'])
    expect(result).toEqual({ status: 'completed' })
  })

  it('streamOperation 连接结束但没有 status 时返回失败', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      streamResponse('event: log\n' + 'data: booting\n\n'),
    )

    const result = await new Promise<{ status: string; error?: string }>((resolve) => {
      bridgeClient.streamOperation('operation-1', () => {}, resolve)
    })

    expect(result.status).toBe('failed')
    expect(result.error).toContain('ended before status')
  })
})
