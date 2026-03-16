import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import App from '../App'
import { LocaleProvider } from '../contexts/LocaleContext'

const {
  restoreAccessSession,
  setUnauthenticatedHandler,
  setAccessTokenHandler,
  setClientApp,
} = vi.hoisted(() => ({
  restoreAccessSession: vi.fn(),
  setUnauthenticatedHandler: vi.fn(),
  setAccessTokenHandler: vi.fn(),
  setClientApp: vi.fn(),
}))

vi.mock('../api', () => ({
  restoreAccessSession,
  setUnauthenticatedHandler,
  setAccessTokenHandler,
}))

vi.mock('@arkloop/shared/api', () => ({
  setClientApp,
}))

describe('App loading state', () => {
  const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }
  const originalActEnvironment = actEnvironment.IS_REACT_ACT_ENVIRONMENT

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    restoreAccessSession.mockReset()
    setUnauthenticatedHandler.mockReset()
    setAccessTokenHandler.mockReset()
    setClientApp.mockReset()
    restoreAccessSession.mockReturnValue(new Promise(() => {}))
  })

  afterEach(() => {
    vi.clearAllMocks()
    if (originalActEnvironment === undefined) {
      delete actEnvironment.IS_REACT_ACT_ENVIRONMENT
    } else {
      actEnvironment.IS_REACT_ACT_ENVIRONMENT = originalActEnvironment
    }
  })

  it('等待刷新会话时应显示全屏加载页而不是空白', async () => {
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)

    await act(async () => {
      root.render(
        <LocaleProvider>
          <MemoryRouter initialEntries={['/t/thread-1']}>
            <App />
          </MemoryRouter>
        </LocaleProvider>,
      )
    })

    expect(restoreAccessSession).toHaveBeenCalledTimes(1)
    expect(container.textContent).toContain('Arkloop')
    expect(container.textContent).toContain('加载中...')

    act(() => {
      root.unmount()
    })
    container.remove()
  })
})
