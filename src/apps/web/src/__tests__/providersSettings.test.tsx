import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

let container: HTMLDivElement
let root: ReturnType<typeof createRoot> | null
const actEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }
const originalActEnvironment = actEnvironment.IS_REACT_ACT_ENVIRONMENT

const listLlmProviders = vi.fn()
const listAvailableModels = vi.fn()
const createLlmProvider = vi.fn()
const updateLlmProvider = vi.fn()
const createProviderModel = vi.fn()
const deleteLlmProvider = vi.fn()
const copyLlmProvider = vi.fn()
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
      createLlmProvider,
      updateLlmProvider,
      deleteLlmProvider,
      copyLlmProvider,
      createProviderModel,
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
  createLlmProvider.mockReset()
  updateLlmProvider.mockReset()
  createProviderModel.mockReset()
  deleteLlmProvider.mockReset()
  copyLlmProvider.mockReset()
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
  createLlmProvider.mockResolvedValue({
    id: 'provider-created',
    name: 'Created Provider',
    provider: 'openai',
    openai_api_mode: 'responses',
    base_url: 'https://created.example/v1',
    advanced_json: {},
    models: [],
  })
  updateLlmProvider.mockResolvedValue({
    id: 'provider-1',
    name: 'OpenRouter',
    provider: 'openai',
    openai_api_mode: 'responses',
    base_url: 'https://openrouter.ai/api/v1',
    advanced_json: {},
    models: [],
  })
  createProviderModel.mockResolvedValue({
    id: 'model-created',
    provider_id: 'provider-created',
    model: 'openai/gpt-4o-mini',
    priority: 0,
    is_default: false,
    show_in_picker: false,
    tags: [],
    when: {},
    multiplier: 1,
  })
  deleteLlmProvider.mockResolvedValue(undefined)
  copyLlmProvider.mockResolvedValue({})
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

    await openProviderCard('OpenRouter')

    const importButton = findButton(document.body, /导入模型|Import models/)
    expect(importButton).toBeTruthy()

    await act(async () => {
      importButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    expect(listAvailableModels).toHaveBeenCalled()
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

    await openProviderCard('OpenRouter')

    const importButton = findButton(document.body, /导入模型|Import models/)
    expect(importButton).toBeTruthy()

    await act(async () => {
      importButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    expect(document.body.textContent).toContain('Error')
    expect(document.body.textContent).not.toContain('provider request failed')

    const errorButton = Array.from(document.body.querySelectorAll('button')).find((button) => button.textContent === 'Error')
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

    await openProviderCard('DuoJie')
    await openProviderDeleteConfirm()
    await clickProviderDeleteConfirm()
    await flushEffects()

    expect(deleteLlmProvider).toHaveBeenNthCalledWith(1, 'token', 'provider-1')
    expect(container.textContent).toContain('OpenRouter')

    await openProviderCard('OpenRouter')
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
    expect(text).not.toMatch(/测试|Test/)
    expect(text).not.toMatch(/添加模型|Add model/)

    await openProviderCard('Claude Code (Local)')

    expect(document.body.querySelector('input[type="password"]')).toBeNull()
    expect(document.body.querySelector('button.button-secondary')).toBeNull()
    const modelToggle = document.body.querySelector('input[type="checkbox"]') as HTMLInputElement | null
    expect(modelToggle).toBeTruthy()

    await act(async () => {
      modelToggle!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    expect(patchProviderModel).toHaveBeenCalledWith('token', 'claude-code-local', 'model-1', { show_in_picker: false })
  })

  it('复制供应商时调用后端 clone 并刷新列表', async () => {
    const sourceProvider = {
      id: 'provider-1',
      name: 'OpenRouter',
      provider: 'openai',
      openai_api_mode: 'responses',
      base_url: 'https://openrouter.ai/api/v1',
      advanced_json: {},
      models: [],
    }
    const copiedProvider = {
      ...sourceProvider,
      id: 'provider-2',
      name: 'OpenRouter copy',
    }
    listLlmProviders
      .mockResolvedValueOnce([sourceProvider])
      .mockResolvedValueOnce([copiedProvider, sourceProvider])

    const { ProvidersSettings, LocaleProvider } = await loadSubject()

    await act(async () => {
      root!.render(
        <LocaleProvider>
          <ProvidersSettings accessToken="token" />
        </LocaleProvider>,
      )
    })
    await flushEffects()

    const copyButton = container.querySelector('button[aria-label="复制供应商"], button[aria-label="Copy provider"]')
    expect(copyButton).toBeTruthy()

    await act(async () => {
      copyButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    expect(copyLlmProvider).toHaveBeenCalledWith('token', 'provider-1')
    expect(listLlmProviders).toHaveBeenCalledTimes(2)
    expect(container.textContent).toContain('OpenRouter copy')
  })

  it('新增供应商后自动打开详情并触发一次导入模型', async () => {
    const createdProvider = {
      id: 'provider-created',
      name: 'Created Provider',
      provider: 'openai',
      openai_api_mode: 'responses',
      base_url: 'https://created.example/v1',
      advanced_json: {},
      models: [],
    }
    createLlmProvider.mockResolvedValueOnce(createdProvider)
    listLlmProviders
      .mockResolvedValue([createdProvider])
      .mockResolvedValueOnce([])
      .mockResolvedValueOnce([createdProvider])

    const { ProvidersSettings, LocaleProvider } = await loadSubject()

    await act(async () => {
      root!.render(
        <LocaleProvider>
          <ProvidersSettings accessToken="token" />
        </LocaleProvider>,
      )
    })
    await flushEffects()

    const addButton = findButton(container, /添加供应商|Add provider/)
    expect(addButton).toBeTruthy()
    await act(async () => {
      addButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const inputs = Array.from(document.body.querySelectorAll('input')) as HTMLInputElement[]
    const nameInput = inputs.find((input) => input.placeholder === 'My Provider')
    const apiKeyInput = inputs.find((input) => input.type === 'password')
    expect(nameInput).toBeTruthy()
    expect(apiKeyInput).toBeTruthy()

    await act(async () => {
      setInputValue(nameInput!, 'Created Provider')
      setInputValue(apiKeyInput!, 'sk-created')
    })
    await flushEffects()

    const saveButton = findButton(document.body, /保存|Save/)
    expect(saveButton).toBeTruthy()
    await act(async () => {
      saveButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()
    await flushEffects()

    expect(createLlmProvider).toHaveBeenCalledWith('token', expect.objectContaining({
      name: 'Created Provider',
      api_key: 'sk-created',
    }))
    expect(document.body.textContent).toContain('Created Provider')
    expect(listAvailableModels).toHaveBeenCalledWith('token', 'provider-created')
    expect(createProviderModel).toHaveBeenCalledWith('token', 'provider-created', expect.objectContaining({
      model: 'openai/gpt-4o-mini',
    }))
  })

  it('新增 DeepSeek 供应商时不提交 openai_api_mode', async () => {
    const createdProvider = {
      id: 'provider-deepseek',
      name: 'deepseek官方',
      provider: 'deepseek',
      openai_api_mode: null,
      base_url: 'https://api.deepseek.com',
      advanced_json: {},
      models: [],
    }
    createLlmProvider.mockResolvedValueOnce(createdProvider)
    listLlmProviders.mockResolvedValueOnce([]).mockResolvedValueOnce([createdProvider])

    const { ProvidersSettings, LocaleProvider } = await loadSubject()

    await act(async () => {
      root!.render(
        <LocaleProvider>
          <ProvidersSettings accessToken="token" />
        </LocaleProvider>,
      )
    })
    await flushEffects()

    const addButton = findButton(container, /添加供应商|Add provider/)
    expect(addButton).toBeTruthy()
    await act(async () => {
      addButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const vendorButton = Array.from(document.body.querySelectorAll('button')).find((button) => button.textContent?.includes('OpenAI'))
    expect(vendorButton).toBeTruthy()
    await act(async () => {
      vendorButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const deepseekOption = Array.from(document.body.querySelectorAll('button')).find((button) => button.textContent?.includes('DeepSeek'))
    expect(deepseekOption).toBeTruthy()
    await act(async () => {
      deepseekOption!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const inputs = Array.from(document.body.querySelectorAll('input')) as HTMLInputElement[]
    const nameInput = inputs.find((input) => input.placeholder === 'My Provider')
    const apiKeyInput = inputs.find((input) => input.type === 'password')
    expect(nameInput).toBeTruthy()
    expect(apiKeyInput).toBeTruthy()

    await act(async () => {
      setInputValue(nameInput!, 'deepseek官方')
      setInputValue(apiKeyInput!, 'sk-deepseek')
    })
    await flushEffects()

    const saveButton = findButton(document.body, /保存|Save/)
    expect(saveButton).toBeTruthy()
    await act(async () => {
      saveButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    expect(createLlmProvider).toHaveBeenCalledWith('token', expect.objectContaining({
      name: 'deepseek官方',
      provider: 'deepseek',
      api_key: 'sk-deepseek',
      openai_api_mode: undefined,
    }))
  })

  it('新增 ZENMAX 供应商时只显示单一类型且不提交 zenmax_protocol', async () => {
    const createdProvider = {
      id: 'provider-zenmax',
      name: 'ZENMAX 官方',
      provider: 'zenmax',
      openai_api_mode: undefined,
      base_url: 'https://zenmux.ai/api/v1',
      advanced_json: {},
      models: [],
    }
    createLlmProvider.mockResolvedValueOnce(createdProvider)
    listLlmProviders.mockResolvedValueOnce([]).mockResolvedValueOnce([createdProvider])

    const { ProvidersSettings, LocaleProvider } = await loadSubject()

    await act(async () => {
      root!.render(
        <LocaleProvider>
          <ProvidersSettings accessToken="token" />
        </LocaleProvider>,
      )
    })
    await flushEffects()

    const addButton = findButton(container, /添加供应商|Add provider/)
    expect(addButton).toBeTruthy()
    await act(async () => {
      addButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const vendorButton = Array.from(document.body.querySelectorAll('button')).find((button) => button.textContent?.includes('OpenAI'))
    expect(vendorButton).toBeTruthy()
    await act(async () => {
      vendorButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const zenmaxOptions = Array.from(document.body.querySelectorAll('button')).filter((button) => button.textContent?.includes('ZENMAX'))
    expect(zenmaxOptions).toHaveLength(1)
    await act(async () => {
      zenmaxOptions[0]!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    expect(document.body.textContent).not.toContain('Claude')
    expect(document.body.textContent).not.toContain('Gemini')

    const inputs = Array.from(document.body.querySelectorAll('input')) as HTMLInputElement[]
    const nameInput = inputs.find((input) => input.placeholder === 'My Provider')
    const apiKeyInput = inputs.find((input) => input.type === 'password')
    expect(nameInput).toBeTruthy()
    expect(apiKeyInput).toBeTruthy()

    await act(async () => {
      setInputValue(nameInput!, 'ZENMAX 官方')
      setInputValue(apiKeyInput!, 'sk-zenmax')
    })
    await flushEffects()

    const saveButton = findButton(document.body, /保存|Save/)
    expect(saveButton).toBeTruthy()
    await act(async () => {
      saveButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    expect(createLlmProvider).toHaveBeenCalledWith('token', expect.objectContaining({
      name: 'ZENMAX 官方',
      provider: 'zenmax',
      api_key: 'sk-zenmax',
      advanced_json: expect.not.objectContaining({
        zenmax_protocol: expect.anything(),
      }),
    }))
  })

  it('新增 Doubao 供应商时不提交 openai_api_mode', async () => {
    const createdProvider = {
      id: 'provider-doubao',
      name: 'doubao官方',
      provider: 'doubao',
      openai_api_mode: null,
      base_url: 'https://ark.cn-beijing.volces.com/api/v3',
      advanced_json: {},
      models: [],
    }
    createLlmProvider.mockResolvedValueOnce(createdProvider)
    listLlmProviders.mockResolvedValueOnce([]).mockResolvedValueOnce([createdProvider])

    const { ProvidersSettings, LocaleProvider } = await loadSubject()

    await act(async () => {
      root!.render(
        <LocaleProvider>
          <ProvidersSettings accessToken="token" />
        </LocaleProvider>,
      )
    })
    await flushEffects()

    const addButton = findButton(container, /添加供应商|Add provider/)
    expect(addButton).toBeTruthy()
    await act(async () => {
      addButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const vendorButton = Array.from(document.body.querySelectorAll('button')).find((button) => button.textContent?.includes('OpenAI'))
    expect(vendorButton).toBeTruthy()
    await act(async () => {
      vendorButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const doubaoOption = Array.from(document.body.querySelectorAll('button')).find((button) => button.textContent?.includes('Doubao'))
    expect(doubaoOption).toBeTruthy()
    await act(async () => {
      doubaoOption!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const inputs = Array.from(document.body.querySelectorAll('input')) as HTMLInputElement[]
    const nameInput = inputs.find((input) => input.placeholder === 'My Provider')
    const apiKeyInput = inputs.find((input) => input.type === 'password')
    expect(nameInput).toBeTruthy()
    expect(apiKeyInput).toBeTruthy()

    await act(async () => {
      setInputValue(nameInput!, 'doubao官方')
      setInputValue(apiKeyInput!, 'sk-doubao')
    })
    await flushEffects()

    const saveButton = findButton(document.body, /保存|Save/)
    expect(saveButton).toBeTruthy()
    await act(async () => {
      saveButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    expect(createLlmProvider).toHaveBeenCalledWith('token', expect.objectContaining({
      name: 'doubao官方',
      provider: 'doubao',
      api_key: 'sk-doubao',
      openai_api_mode: undefined,
    }))
  })

  it('新增 Qwen 供应商时不提交 openai_api_mode', async () => {
    const createdProvider = {
      id: 'provider-qwen',
      name: 'qwen官方',
      provider: 'qwen',
      openai_api_mode: null,
      base_url: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
      advanced_json: {},
      models: [],
    }
    createLlmProvider.mockResolvedValueOnce(createdProvider)
    listLlmProviders.mockResolvedValueOnce([]).mockResolvedValueOnce([createdProvider])

    const { ProvidersSettings, LocaleProvider } = await loadSubject()

    await act(async () => {
      root!.render(
        <LocaleProvider>
          <ProvidersSettings accessToken="token" />
        </LocaleProvider>,
      )
    })
    await flushEffects()

    const addButton = findButton(container, /添加供应商|Add provider/)
    expect(addButton).toBeTruthy()
    await act(async () => {
      addButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const vendorButton = Array.from(document.body.querySelectorAll('button')).find((button) => button.textContent?.includes('OpenAI'))
    expect(vendorButton).toBeTruthy()
    await act(async () => {
      vendorButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const qwenOption = Array.from(document.body.querySelectorAll('button')).find((button) => button.textContent?.includes('Qwen'))
    expect(qwenOption).toBeTruthy()
    await act(async () => {
      qwenOption!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const inputs = Array.from(document.body.querySelectorAll('input')) as HTMLInputElement[]
    const nameInput = inputs.find((input) => input.placeholder === 'My Provider')
    const apiKeyInput = inputs.find((input) => input.type === 'password')
    expect(nameInput).toBeTruthy()
    expect(apiKeyInput).toBeTruthy()

    await act(async () => {
      setInputValue(nameInput!, 'qwen官方')
      setInputValue(apiKeyInput!, 'sk-qwen')
    })
    await flushEffects()

    const saveButton = findButton(document.body, /保存|Save/)
    expect(saveButton).toBeTruthy()
    await act(async () => {
      saveButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    expect(createLlmProvider).toHaveBeenCalledWith('token', expect.objectContaining({
      name: 'qwen官方',
      provider: 'qwen',
      api_key: 'sk-qwen',
      openai_api_mode: undefined,
    }))
  })

  it('新增 Yuanbao 供应商时不提交 openai_api_mode', async () => {
    const createdProvider = {
      id: 'provider-yuanbao',
      name: 'yuanbao官方',
      provider: 'yuanbao',
      openai_api_mode: null,
      base_url: 'https://api.hunyuan.cloud.tencent.com/v1',
      advanced_json: {},
      models: [],
    }
    createLlmProvider.mockResolvedValueOnce(createdProvider)
    listLlmProviders.mockResolvedValueOnce([]).mockResolvedValueOnce([createdProvider])

    const { ProvidersSettings, LocaleProvider } = await loadSubject()

    await act(async () => {
      root!.render(
        <LocaleProvider>
          <ProvidersSettings accessToken="token" />
        </LocaleProvider>,
      )
    })
    await flushEffects()

    const addButton = findButton(container, /添加供应商|Add provider/)
    expect(addButton).toBeTruthy()
    await act(async () => {
      addButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const vendorButton = Array.from(document.body.querySelectorAll('button')).find((button) => button.textContent?.includes('OpenAI'))
    expect(vendorButton).toBeTruthy()
    await act(async () => {
      vendorButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const yuanbaoOption = Array.from(document.body.querySelectorAll('button')).find((button) => button.textContent?.includes('Yuanbao'))
    expect(yuanbaoOption).toBeTruthy()
    await act(async () => {
      yuanbaoOption!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const inputs = Array.from(document.body.querySelectorAll('input')) as HTMLInputElement[]
    const nameInput = inputs.find((input) => input.placeholder === 'My Provider')
    const apiKeyInput = inputs.find((input) => input.type === 'password')
    expect(nameInput).toBeTruthy()
    expect(apiKeyInput).toBeTruthy()

    await act(async () => {
      setInputValue(nameInput!, 'yuanbao官方')
      setInputValue(apiKeyInput!, 'sk-yuanbao')
    })
    await flushEffects()

    const saveButton = findButton(document.body, /保存|Save/)
    expect(saveButton).toBeTruthy()
    await act(async () => {
      saveButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    expect(createLlmProvider).toHaveBeenCalledWith('token', expect.objectContaining({
      name: 'yuanbao官方',
      provider: 'yuanbao',
      api_key: 'sk-yuanbao',
      openai_api_mode: undefined,
    }))
  })

  it('新增 Kimi 供应商时不提交 openai_api_mode', async () => {
    const createdProvider = {
      id: 'provider-kimi',
      name: 'kimi官方',
      provider: 'kimi',
      openai_api_mode: null,
      base_url: 'https://api.moonshot.cn/v1',
      advanced_json: {},
      models: [],
    }
    createLlmProvider.mockResolvedValueOnce(createdProvider)
    listLlmProviders.mockResolvedValueOnce([]).mockResolvedValueOnce([createdProvider])

    const { ProvidersSettings, LocaleProvider } = await loadSubject()

    await act(async () => {
      root!.render(
        <LocaleProvider>
          <ProvidersSettings accessToken="token" />
        </LocaleProvider>,
      )
    })
    await flushEffects()

    const addButton = findButton(container, /添加供应商|Add provider/)
    expect(addButton).toBeTruthy()
    await act(async () => {
      addButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const vendorButton = Array.from(document.body.querySelectorAll('button')).find((button) => button.textContent?.includes('OpenAI'))
    expect(vendorButton).toBeTruthy()
    await act(async () => {
      vendorButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const kimiOption = Array.from(document.body.querySelectorAll('button')).find((button) => button.textContent?.includes('Kimi'))
    expect(kimiOption).toBeTruthy()
    await act(async () => {
      kimiOption!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const inputs = Array.from(document.body.querySelectorAll('input')) as HTMLInputElement[]
    const nameInput = inputs.find((input) => input.placeholder === 'My Provider')
    const apiKeyInput = inputs.find((input) => input.type === 'password')
    expect(nameInput).toBeTruthy()
    expect(apiKeyInput).toBeTruthy()

    await act(async () => {
      setInputValue(nameInput!, 'kimi官方')
      setInputValue(apiKeyInput!, 'sk-kimi')
    })
    await flushEffects()

    const saveButton = findButton(document.body, /保存|Save/)
    expect(saveButton).toBeTruthy()
    await act(async () => {
      saveButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    expect(createLlmProvider).toHaveBeenCalledWith('token', expect.objectContaining({
      name: 'kimi官方',
      provider: 'kimi',
      api_key: 'sk-kimi',
      openai_api_mode: undefined,
    }))
  })

  it('新增供应商时在高级选项保存 Headers', async () => {
    const { ProvidersSettings, LocaleProvider } = await loadSubject()

    await act(async () => {
      root!.render(
        <LocaleProvider>
          <ProvidersSettings accessToken="token" />
        </LocaleProvider>,
      )
    })
    await flushEffects()

    const addButton = findButton(container, /添加供应商|Add provider/)
    expect(addButton).toBeTruthy()
    await act(async () => {
      addButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const advancedButton = findButton(document.body, /高级选项|高级配置|Advanced/)
    expect(advancedButton).toBeTruthy()
    await act(async () => {
      advancedButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const addHeaderButton = findButton(document.body, /添加 Header|Add header/)
    expect(addHeaderButton).toBeTruthy()
    await act(async () => {
      addHeaderButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    const inputs = Array.from(document.body.querySelectorAll('input')) as HTMLInputElement[]
    const nameInput = inputs.find((input) => input.placeholder === 'My Provider')
    const apiKeyInput = inputs.find((input) => input.type === 'password')
    const headerNameInput = inputs.find((input) => /Header 名称|Header name/.test(input.placeholder))
    const headerValueInput = inputs.find((input) => /Header 值|Header value/.test(input.placeholder))
    expect(nameInput).toBeTruthy()
    expect(apiKeyInput).toBeTruthy()
    expect(headerNameInput).toBeTruthy()
    expect(headerValueInput).toBeTruthy()

    await act(async () => {
      setInputValue(nameInput!, 'Created Provider')
      setInputValue(apiKeyInput!, 'sk-created')
      setInputValue(headerNameInput!, 'X-Provider')
      setInputValue(headerValueInput!, 'secret')
    })
    await flushEffects()

    const saveButton = findButton(document.body, /保存|Save/)
    expect(saveButton).toBeTruthy()
    await act(async () => {
      saveButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    })
    await flushEffects()

    expect(createLlmProvider).toHaveBeenCalledWith('token', expect.objectContaining({
      advanced_json: expect.objectContaining({
        openviking_extra_headers: { 'X-Provider': 'secret' },
      }),
    }))
  })
})

async function openProviderCard(name: string) {
  const button = Array.from(container.querySelectorAll('[role="button"], button')).find((item) =>
    item.textContent?.includes(name),
  ) as HTMLElement | undefined
  expect(button).toBeTruthy()
  await act(async () => {
    button!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  })
  await flushEffects()
}

function findButton(rootElement: ParentNode, pattern: RegExp): HTMLButtonElement | undefined {
  return Array.from(rootElement.querySelectorAll('button')).find((button) =>
    pattern.test(button.textContent ?? ''),
  ) as HTMLButtonElement | undefined
}

async function openProviderDeleteConfirm() {
  const trashButton = document.body.querySelector('button[aria-label="删除供应商"], button[aria-label="Delete provider"]')
  expect(trashButton).toBeTruthy()
  await act(async () => {
    trashButton!.dispatchEvent(new MouseEvent('click', { bubbles: true }))
  })
  await flushEffects()
}

function findProviderDeleteConfirm(): HTMLButtonElement {
  const button = Array.from(document.body.querySelectorAll('button')).find((item) =>
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

function setInputValue(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
  setter?.call(input, value)
  input.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText', data: value }))
}
