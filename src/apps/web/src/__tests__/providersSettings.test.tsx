import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

let container: HTMLDivElement
let root: ReturnType<typeof createRoot> | null
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }
const originalActEnvironment = actEnvironment.IS_REACT_ACT_ENVIRONMENT

const listLlmProviders = vi.fn()
const listAvailableModels = vi.fn()
const deleteLlmProvider = vi.fn()
const patchProviderModel = vi.fn()

async function flushEffects() {
  await act(async () => {
    await Promise.resolve()
    await Promise.resolve()
  })
}

async function loadSubject() {
  vi.resetModules()
  vi.doMock('../api', async () => {
    const actual = await vi.importActual<typeof import('../api')>('../api')
    return {
      ...actual,
      listLlmProviders,
      listAvailableModels,
      createLlmProvider: vi.fn(),
      updateLlmProvider: vi.fn(),
      deleteLlmProvider,
      createProviderModel: vi.fn(),
      deleteProviderModel: vi.fn(),
      patchProviderModel,
      isApiError: () => false,
    }
  })

  const { ProvidersSettings } = await import('../components/settings/ProvidersSettings')
  const { LocaleProvider } = await import('../contexts/LocaleContext')
  return { ProvidersSettings, LocaleProvider }
}

beforeEach(() => {
  actEnvironment.IS_REACT_ACT_ENVIRONMENT = true
  container = document.createElement('div')
  document.body.appendChild(container)
  root = createRoot(container)

  listLlmProviders.mockReset()
  listAvailableModels.mockReset()
  deleteLlmProvider.mockReset()
  patchProviderModel.mockReset()
  listLlmProviders.mockResolvedValue([
    {
      id: 'provider-1',
      name: 'OpenRouter',
      provider: 'openai',
      openai_api_mode: 'responses',
      base_url: 'https://openrouter.ai/api/v1',
      advanced_json: {},
      models: [],
    },
  ])
  listAvailableModels.mockResolvedValue({
    models: [{ id: 'openai/gpt-4o-mini', name: 'GPT-4o mini', configured: false, type: 'chat' }],
  })
  deleteLlmProvider.mockResolvedValue(undefined)
  patchProviderModel.mockResolvedValue({})
})

afterEach(() => {
  if (root) {
    act(() => root!.unmount())
  }
  container.remove()
  root = null
  vi.doUnmock('../api')
  vi.resetModules()
  vi.clearAllMocks()
  if (originalActEnvironment === undefined) {
    delete actEnvironment.IS_REACT_ACT_ENVIRONMENT
  } else {
    actEnvironment.IS_REACT_ACT_ENVIRONMENT = originalActEnvironment
  }
})

describe('ProvidersSettings', () => {
  it('打开页面时不自动请求 available models，点击导入后才请求', async () => {
    const { ProvidersSettings, LocaleProvider } = await loadSubject()

    await act(async () => {
      root!.render(
        <LocaleProvider>
          <ProvidersSettings accessToken="token" />
        </LocaleProvider>,
      )
    })
    await flushEffects()

    expect(listLlmProviders).toHaveBeenCalledTimes(1)
    expect(listAvailableModels).not.toHaveBeenCalled()

    const importButton = container.querySelector('button.button-secondary')
    expect(importButton).toBeTruthy()

    await act(async () => {
      importButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    expect(listAvailableModels).toHaveBeenCalledTimes(1)
  })

  it('available models 拉取失败时只露出 Error 入口，详情放进弹层', async () => {
    listAvailableModels.mockRejectedValueOnce(new Error('provider request failed'))
    const { ProvidersSettings, LocaleProvider } = await loadSubject()

    await act(async () => {
      root!.render(
        <LocaleProvider>
          <ProvidersSettings accessToken="token" />
        </LocaleProvider>,
      )
    })
    await flushEffects()

    const importButton = container.querySelector('button.button-secondary')
    expect(importButton).toBeTruthy()

    await act(async () => {
      importButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    expect(container.textContent).toContain('Error')
    expect(container.textContent).not.toContain('provider request failed')

    const errorButton = Array.from(container.querySelectorAll('button')).find((button) => button.textContent === 'Error')
    expect(errorButton).toBeTruthy()

    await act(async () => {
      errorButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    expect(document.body.textContent).toContain('provider request failed')
  })

  it('删除一个供应商后可以继续删除下一个供应商', async () => {
    const firstProvider = {
      id: 'provider-1',
      name: 'DuoJie',
      provider: 'openai',
      openai_api_mode: 'responses',
      base_url: 'https://api.duojie.games',
      advanced_json: {},
      models: [],
    }
    const secondProvider = {
      id: 'provider-2',
      name: 'OpenRouter',
      provider: 'openai',
      openai_api_mode: 'responses',
      base_url: 'https://openrouter.ai/api/v1',
      advanced_json: {},
      models: [],
    }
    listLlmProviders
      .mockResolvedValueOnce([firstProvider, secondProvider])
      .mockResolvedValueOnce([secondProvider])
      .mockResolvedValueOnce([])

    const { ProvidersSettings, LocaleProvider } = await loadSubject()

    await act(async () => {
      root!.render(
        <LocaleProvider>
          <ProvidersSettings accessToken="token" />
        </LocaleProvider>,
      )
    })
    await flushEffects()

    await openProviderDeleteConfirm()
    await clickProviderDeleteConfirm()
    await flushEffects()

    expect(deleteLlmProvider).toHaveBeenNthCalledWith(1, 'token', 'provider-1')
    expect(container.textContent).toContain('OpenRouter')

    await openProviderDeleteConfirm()
    const secondDeleteButton = findProviderDeleteConfirm()
    expect(secondDeleteButton.disabled).toBe(false)

    await act(async () => {
      secondDeleteButton.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    expect(deleteLlmProvider).toHaveBeenNthCalledWith(2, 'token', 'provider-2')
  })

  it('本地只读供应商不显示写操作入口', async () => {
    listLlmProviders.mockResolvedValueOnce([
      {
        id: 'claude-code-local',
        name: 'Claude Code (Local)',
        provider: 'claude_code_local',
        source: 'local',
        read_only: true,
        auth_mode: 'api_key',
        openai_api_mode: null,
        base_url: null,
        advanced_json: {},
        models: [
          {
            id: 'model-1',
            provider_id: 'claude-code-local',
            model: 'claude-sonnet-4-6',
            priority: 0,
            is_default: true,
            show_in_picker: true,
            tags: [],
            when: {},
            multiplier: 1,
          },
        ],
      },
    ])

    const { ProvidersSettings, LocaleProvider } = await loadSubject()

    await act(async () => {
      root!.render(
        <LocaleProvider>
          <ProvidersSettings accessToken="token" />
        </LocaleProvider>,
      )
    })
    await flushEffects()

    const text = container.textContent ?? ''
    expect(text).toContain('Claude Code (Local)')
    expect(text).toMatch(/本地|Local/)
    expect(text).toMatch(/只读|Read-only/)
    expect(text).not.toMatch(/已启用|Enabled/)
    expect(text).not.toMatch(/测试|Test/)
    expect(text).not.toMatch(/添加模型|Add model/)
    expect(container.querySelector('input[type="password"]')).toBeNull()
    expect(container.querySelector('button.button-secondary')).toBeNull()
    const modelToggle = container.querySelector('input[type="checkbox"]') as HTMLInputElement | null
    expect(modelToggle).toBeTruthy()

    await act(async () => {
      modelToggle!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    expect(patchProviderModel).toHaveBeenCalledWith('token', 'claude-code-local', 'model-1', { show_in_picker: false })
  })
})

async function openProviderDeleteConfirm() {
  const trashButton = Array.from(container.querySelectorAll('button')).find((button) =>
    button.textContent?.trim() === '' && button.querySelector('svg'),
  )
  expect(trashButton).toBeTruthy()
  await act(async () => {
    trashButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  })
  await flushEffects()
}

function findProviderDeleteConfirm(): HTMLButtonElement {
  const button = Array.from(container.querySelectorAll('button')).find((item) =>
    item.textContent?.includes('删除供应商') || item.textContent?.includes('Delete provider'),
  ) as HTMLButtonElement | undefined
  expect(button).toBeTruthy()
  return button!
}

async function clickProviderDeleteConfirm() {
  const button = findProviderDeleteConfirm()
  await act(async () => {
    button.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  })
  await flushEffects()
}
