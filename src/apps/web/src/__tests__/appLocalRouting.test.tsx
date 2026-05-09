import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ToastProvider } from '@arkloop/shared'

import App from '../App'
import { LocaleProvider } from '../contexts/LocaleContext'

const {
  setUnauthenticatedHandler,
  setAccessTokenHandler,
  setSessionExpiredHandler,
  restoreAccessSession,
  createLocalSession,
  setClientApp,
} = vi.hoisted(() => ({
  setUnauthenticatedHandler: vi.fn(),
  setAccessTokenHandler: vi.fn(),
  setSessionExpiredHandler: vi.fn(),
  restoreAccessSession: vi.fn(),
  createLocalSession: vi.fn(),
  setClientApp: vi.fn(),
}))

vi.mock('../api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api')>()
  return {
    ...actual,
    setUnauthenticatedHandler,
    setAccessTokenHandler,
    setSessionExpiredHandler,
    restoreAccessSession,
    createLocalSession,
  }
})

vi.mock('@arkloop/shared/api', () => ({
  setClientApp,
}))

vi.mock('@arkloop/shared/desktop', () => ({
  isLocalMode: () => true,
  isDesktop: () => false,
  getDesktopApi: () => null,
  getDesktopAccessToken: () => '',
}))


vi.mock('../components/AuthPage', () => ({
  AuthPage: () => <div data-testid="auth-page">auth-page</div>,
}))

vi.mock('../components/HeadlessSetupPage', () => ({
  HeadlessSetupPage: () => <div data-testid="headless-setup-page">headless-setup-page</div>,
}))

describe('App local login routing', () => {
  const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }
  const originalActEnvironment = actEnvironment.IS_REACT_ACT_ENVIRONMENT

  beforeEach(() => {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    setUnauthenticatedHandler.mockReset()
    setAccessTokenHandler.mockReset()
    setSessionExpiredHandler.mockReset()
    restoreAccessSession.mockReset()
    createLocalSession.mockReset()
    setClientApp.mockReset()
  })

  afterEach(() => {
    vi.clearAllMocks()
    if (originalActEnvironment === undefined) {
      delete actEnvironment.IS_REACT_ACT_ENVIRONMENT
    } else {
      actEnvironment.IS_REACT_ACT_ENVIRONMENT = originalActEnvironment
    }
  })

  it('local mode unauthenticated /login renders AuthPage instead of HeadlessSetupPage', async () => {
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)

    await act(async () => {
      root.render(
        <LocaleProvider>
          <ToastProvider>
            <MemoryRouter initialEntries={['/login']}>
              <App />
            </MemoryRouter>
          </ToastProvider>
        </LocaleProvider>,
      )
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(container.querySelector('[data-testid="auth-page"]')).not.toBeNull()
    expect(container.querySelector('[data-testid="headless-setup-page"]')).toBeNull()

    act(() => {
      root.unmount()
    })
    container.remove()
  })

  it('renders HeadlessSetupPage on /setup', async () => {
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)

    await act(async () => {
      root.render(
        <LocaleProvider>
          <ToastProvider>
            <MemoryRouter initialEntries={['/setup']}>
              <App />
            </MemoryRouter>
          </ToastProvider>
        </LocaleProvider>,
      )
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(container.querySelector('[data-testid="headless-setup-page"]')).not.toBeNull()
    expect(container.querySelector('[data-testid="auth-page"]')).toBeNull()

    act(() => {
      root.unmount()
    })
    container.remove()
  })
})
