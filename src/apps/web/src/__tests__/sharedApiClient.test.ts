import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

type LoginResponse = {
  access_token: string
  token_type: string
}

function jsonResponse(payload: LoginResponse): Response {
  return new Response(JSON.stringify(payload), {
    status: 200,
    headers: {
      'Content-Type': 'application/json',
    },
  })
}

describe('shared auth client', () => {
  beforeEach(() => {
    vi.resetModules()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('会复用同一个 refresh 请求，并允许已取消的调用方退出等待', async () => {
    let resolveFetch!: (value: Response) => void
    const fetchMock = vi.fn(() => new Promise<Response>((resolve) => {
      resolveFetch = resolve
    }))
    vi.stubGlobal('fetch', fetchMock)

    const { refreshAccessToken } = await import('@arkloop/shared')
    const controller = new AbortController()

    const first = refreshAccessToken(controller.signal)
    const second = refreshAccessToken()

    expect(fetchMock).toHaveBeenCalledTimes(1)

    controller.abort()
    await expect(first).rejects.toMatchObject({ name: 'AbortError' })

    resolveFetch(jsonResponse({ access_token: 'token-1', token_type: 'bearer' }))

    await expect(second).resolves.toEqual({ access_token: 'token-1', token_type: 'bearer' })
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('启动恢复会对瞬时故障做重试', async () => {
    const fetchMock = vi
      .fn()
      .mockRejectedValueOnce(new TypeError('fetch failed'))
      .mockRejectedValueOnce(new TypeError('fetch failed'))
      .mockResolvedValueOnce(jsonResponse({ access_token: 'token-2', token_type: 'bearer' }))
    vi.stubGlobal('fetch', fetchMock)

    const { restoreAccessSession } = await import('@arkloop/shared')

    await expect(restoreAccessSession({ retries: 2, retryDelayMs: 0 })).resolves.toEqual({
      access_token: 'token-2',
      token_type: 'bearer',
    })
    expect(fetchMock).toHaveBeenCalledTimes(3)
  })
})
