import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import UserInputCard from '../components/UserInputCard'
import { LocaleProvider } from '../contexts/LocaleContext'
import type { UserInputRequest, UserInputResponse } from '../userInputTypes'

let container: HTMLDivElement
let root: ReturnType<typeof createRoot>

beforeEach(() => {
  container = document.createElement('div')
  document.body.appendChild(container)
  root = createRoot(container)
})

afterEach(() => {
  act(() => root.unmount())
  container.remove()
})

const singleSelect: UserInputRequest = {
  request_id: 'req_1',
  message: 'Which database?',
  requestedSchema: {
    properties: {
      db: {
        type: 'string' as const,
        title: 'Database',
        enum: ['postgres', 'mysql'],
        enumDescriptions: ['Open source relational database', 'Popular relational database'],
      },
    },
    required: ['db'],
  },
}

const multiField: UserInputRequest = {
  request_id: 'req_2',
  message: 'Configure project',
  requestedSchema: {
    properties: {
      db: {
        type: 'string' as const,
        title: 'Database',
        oneOf: [
          { const: 'pg', title: 'PostgreSQL' },
          { const: 'my', title: 'MySQL' },
        ],
      },
      features: {
        type: 'array' as const,
        title: 'Features',
        items: { type: 'string' as const, enum: ['auth', 'billing', 'search'] },
      },
    },
    required: ['db'],
  },
}

const requiredMultiSelect: UserInputRequest = {
  request_id: 'req_models',
  message: '请选择用于评估的模型。',
  requestedSchema: {
    _dismissLabel: '使用当前模型',
    properties: {
      models: {
        type: 'array' as const,
        title: '评估模型',
        description: '默认不选择模型；不选择并点击“使用当前模型”时，将沿用当前聊天模型。',
        items: { type: 'string' as const, enum: ['deepseek', 'qwen'] },
        minItems: 1,
      },
    },
    required: ['models'],
  },
}

function findBtn(text: string) {
  return Array.from(container.querySelectorAll('button')).find(
    (b) => b.textContent?.includes(text),
  )
}

function findRole(role: string, text: string) {
  return Array.from(container.querySelectorAll(`[role="${role}"]`)).find(
    (el) => el.textContent?.includes(text),
  )
}

function renderCard(
  request: UserInputRequest,
  onSubmit: (r: UserInputResponse) => void = vi.fn(),
  onDismiss: () => void = vi.fn(),
) {
  act(() => {
    root.render(
      <LocaleProvider>
        <UserInputCard request={request} onSubmit={onSubmit} onDismiss={onDismiss} />
      </LocaleProvider>,
    )
  })
}

describe('UserInputCard', () => {
  describe('rendering', () => {
    it('renders message and enum options', () => {
      renderCard(singleSelect)
      expect(container.textContent).toContain('Which database?')
      expect(container.textContent).toContain('postgres')
      expect(container.textContent).toContain('mysql')
      expect(container.textContent).toContain('Open source relational database')
    })

    it('renders oneOf options with titles', () => {
      renderCard(multiField)
      expect(container.textContent).toContain('Configure project')
      expect(container.textContent).toContain('PostgreSQL')
      expect(container.textContent).toContain('MySQL')
    })

    it('renders multiselect checkboxes', () => {
      renderCard(multiField)
      act(() => { (findRole('button', 'PostgreSQL') as HTMLElement).click() })
      expect(container.textContent).toContain('auth')
      expect(container.textContent).toContain('billing')
      expect(container.textContent).toContain('search')
      expect(container.querySelectorAll('[role="checkbox"]').length).toBe(3)
    })
  })

  describe('interaction', () => {
    it('single enum select immediately submits', () => {
      const onSubmit = vi.fn()
      renderCard(singleSelect, onSubmit)

      act(() => { (findRole('button', 'mysql') as HTMLElement).click() })

      expect(onSubmit).toHaveBeenCalledTimes(1)
      const response = onSubmit.mock.calls[0][0] as UserInputResponse
      expect(response.request_id).toBe('req_1')
      expect(response.answers.db).toBe('mysql')
    })

    it('multi-field requires submit button', () => {
      const onSubmit = vi.fn()
      renderCard(multiField, onSubmit)

      act(() => { (findRole('button', 'PostgreSQL') as HTMLElement).click() })
      expect(onSubmit).not.toHaveBeenCalled()

      const submitBtn = findBtn('提交') ?? findBtn('Submit')
      expect(submitBtn).toBeTruthy()
      act(() => { (submitBtn as HTMLElement).click() })
      expect(onSubmit).toHaveBeenCalledTimes(1)
    })

    it('multiselect toggles values', () => {
      const onSubmit = vi.fn()
      renderCard(multiField, onSubmit)

      // 选 db
      act(() => { (findRole('button', 'PostgreSQL') as HTMLElement).click() })

      // 选 features
      const checkboxes = container.querySelectorAll('[role="checkbox"]')
      act(() => { (checkboxes[0] as HTMLElement).click() })
      act(() => { (checkboxes[2] as HTMLElement).click() })

      const submitBtn = findBtn('提交') ?? findBtn('Submit')
      act(() => { submitBtn!.click() })

      const response = onSubmit.mock.calls[0][0] as UserInputResponse
      expect(response.answers.db).toBe('pg')
      expect(response.answers.features).toEqual(['auth', 'search'])
    })

    it('disables submit for required multiselect until an option is selected', () => {
      const onSubmit = vi.fn()
      renderCard(requiredMultiSelect, onSubmit)

      const submitBtn = findBtn('提交') ?? findBtn('Submit')
      expect(submitBtn).toBeTruthy()
      expect((submitBtn as HTMLButtonElement).disabled).toBe(true)
      expect(container.textContent).toContain('使用当前模型')
      expect(container.textContent).toContain('默认不选择模型')

      const checkboxes = container.querySelectorAll('[role="checkbox"]')
      act(() => { (checkboxes[0] as HTMLElement).click() })

      expect((submitBtn as HTMLButtonElement).disabled).toBe(false)
      act(() => { submitBtn!.click() })
      expect(onSubmit).toHaveBeenCalledTimes(1)
      const response = onSubmit.mock.calls[0][0] as UserInputResponse
      expect(response.answers.models).toEqual(['deepseek'])
    })
  })

  describe('dismiss', () => {
    it('calls onDismiss when skip button clicked', () => {
      const onDismiss = vi.fn()
      renderCard(multiField, vi.fn(), onDismiss)

      const skipBtn =
        container.querySelector('button[aria-label="跳过"]') ??
        container.querySelector('button[aria-label="Skip"]')
      expect(skipBtn).toBeTruthy()
      act(() => { (skipBtn as HTMLElement).click() })

      expect(onDismiss).toHaveBeenCalledTimes(1)
    })

    it('uses custom dismiss label when provided', () => {
      const onDismiss = vi.fn()
      renderCard(requiredMultiSelect, vi.fn(), onDismiss)

      const useCurrentBtn = findBtn('使用当前模型')
      expect(useCurrentBtn).toBeTruthy()
      act(() => { useCurrentBtn!.click() })

      expect(onDismiss).toHaveBeenCalledTimes(1)
    })

    it('calls onDismiss on ESC key', () => {
      const onDismiss = vi.fn()
      renderCard(singleSelect, vi.fn(), onDismiss)

      act(() => {
        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
      })

      expect(onDismiss).toHaveBeenCalledTimes(1)
    })
  })
})
