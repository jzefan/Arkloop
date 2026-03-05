import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act } from 'react'
import { createRoot } from 'react-dom/client'

import { ArtifactHtmlPreview } from '../components/ArtifactHtmlPreview'
import type { ArtifactRef } from '../storage'

function flushMicrotasks(): Promise<void> {
  return Promise.resolve()
    .then(() => Promise.resolve())
    .then(() => Promise.resolve())
}

describe('ArtifactHtmlPreview', () => {
  const originalCreateObjectURL = (URL as any).createObjectURL
  const originalRevokeObjectURL = (URL as any).revokeObjectURL
  const originalFetch = globalThis.fetch
  const originalActEnvironment = (globalThis as any).IS_REACT_ACT_ENVIRONMENT

  beforeEach(() => {
    ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = true
    ;(URL as any).createObjectURL = vi.fn(() => 'blob:mock')
    ;(URL as any).revokeObjectURL = vi.fn()
    globalThis.fetch = vi.fn(async () => ({
      ok: true,
      blob: async () => new Blob(['<html></html>'], { type: 'text/html' }),
    })) as any
  })

  afterEach(() => {
    if (originalCreateObjectURL) {
      ;(URL as any).createObjectURL = originalCreateObjectURL
    } else {
      delete (URL as any).createObjectURL
    }
    if (originalRevokeObjectURL) {
      ;(URL as any).revokeObjectURL = originalRevokeObjectURL
    } else {
      delete (URL as any).revokeObjectURL
    }
    globalThis.fetch = originalFetch
    if (originalActEnvironment === undefined) {
      delete (globalThis as any).IS_REACT_ACT_ENVIRONMENT
    } else {
      ;(globalThis as any).IS_REACT_ACT_ENVIRONMENT = originalActEnvironment
    }
    vi.restoreAllMocks()
  })

  it('只接受当前 iframe 自身发来的 resize 消息', async () => {
    const artifact: ArtifactRef = {
      key: 'artifact-key',
      filename: 'index.html',
      size: 10,
      mime_type: 'text/html',
    }

    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)

    await act(async () => {
      root.render(<ArtifactHtmlPreview artifact={artifact} accessToken="token" />)
    })
    await act(async () => {
      await flushMicrotasks()
    })

    const iframe = container.querySelector('iframe') as HTMLIFrameElement | null
    expect(iframe).not.toBeNull()
    if (!iframe) return

    const iframeWindow = {} as any
    Object.defineProperty(iframe, 'contentWindow', { value: iframeWindow, configurable: true })

    const badSource = new MessageEvent('message', {
      data: { type: 'arkloop-iframe-resize', height: 123 },
    })
    Object.defineProperty(badSource, 'source', { value: window, configurable: true })
    window.dispatchEvent(badSource)
    expect(iframe.style.height).not.toBe('123px')

    const goodSource = new MessageEvent('message', {
      data: { type: 'arkloop-iframe-resize', height: 456 },
    })
    Object.defineProperty(goodSource, 'source', { value: iframeWindow, configurable: true })
    window.dispatchEvent(goodSource)
    expect(iframe.style.height).toBe('456px')

    act(() => {
      root.unmount()
    })
    container.remove()
  })
})
