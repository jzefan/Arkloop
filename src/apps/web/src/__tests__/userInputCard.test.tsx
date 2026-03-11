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

const singleQuestion: UserInputRequest = {
  request_id: 'req_1',
  questions: [
    {
      id: 'q1',
      header: 'Select type',
      question: 'Which demo?',
      options: [
        { value: 'a', label: 'Option A', description: 'Description A', recommended: true },
        { value: 'b', label: 'Option B' },
      ],
      allow_other: false,
    },
  ],
}

const multiQuestion: UserInputRequest = {
  request_id: 'req_2',
  questions: [
    {
      id: 'q1',
      question: 'First?',
      options: [
        { value: 'x', label: 'X' },
        { value: 'y', label: 'Y' },
      ],
    },
    {
      id: 'q2',
      question: 'Second?',
      options: [
        { value: 'm', label: 'M' },
        { value: 'n', label: 'N' },
      ],
      allow_other: true,
    },
  ],
}

function findBtn(text: string) {
  return Array.from(container.querySelectorAll('button')).find(
    (b) => b.textContent?.includes(text),
  )
}

function findRole(text: string) {
  return Array.from(container.querySelectorAll('[role="button"]')).find(
    (el) => el.textContent?.includes(text),
  )
}

function findSubmitArrow() {
  return container.querySelector('[data-testid="user-input-submit"]') as HTMLButtonElement | null
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
    it('renders single question with header and options', () => {
      renderCard(singleQuestion)
      expect(container.textContent).toContain('Select type')
      expect(container.textContent).toContain('Which demo?')
      expect(container.textContent).toContain('Option A')
      expect(container.textContent).toContain('Option B')
    })

    it('renders first question in multi-question mode', () => {
      renderCard(multiQuestion)
      expect(container.textContent).toContain('First?')
      expect(container.textContent).toContain('X')
      expect(container.textContent).not.toContain('Second?')
    })

    it('shows recommended tag for recommended option', () => {
      renderCard(singleQuestion)
      expect(container.innerHTML).toContain('Option A')
      expect(container.innerHTML).toContain('推荐')
    })

    it('shows Other input after navigating to question with allow_other', () => {
      renderCard(multiQuestion)
      // q1: no Other input
      expect(container.querySelectorAll('input[type="text"]').length).toBe(0)

      // single-click X on q1 → auto-advances to q2
      act(() => { findRole('X')!.click() })

      expect(container.querySelectorAll('input[type="text"]').length).toBe(1)
    })
  })

  describe('interaction', () => {
    it('single click on option immediately submits (single question)', () => {
      const onSubmit = vi.fn()
      renderCard(singleQuestion, onSubmit)

      act(() => { findRole('Option B')!.click() })

      expect(onSubmit).toHaveBeenCalledTimes(1)
      const response = onSubmit.mock.calls[0][0] as UserInputResponse
      expect(response.request_id).toBe('req_1')
      expect(response.answers.q1).toEqual({ type: 'option', value: 'b' })
    })

    it('arrow button submits pre-selected recommended option', () => {
      const onSubmit = vi.fn()
      renderCard(singleQuestion, onSubmit)

      const arrow = findSubmitArrow()
      expect(arrow).toBeTruthy()
      expect(arrow!.disabled).toBe(false)
      act(() => { arrow!.click() })

      expect(onSubmit).toHaveBeenCalledTimes(1)
      const response = onSubmit.mock.calls[0][0] as UserInputResponse
      expect(response.answers.q1).toEqual({ type: 'option', value: 'a' })
    })

    it('multi-question: single click advances and final click submits', () => {
      const onSubmit = vi.fn()
      renderCard(multiQuestion, onSubmit)

      act(() => { findRole('Y')!.click() })
      expect(container.textContent).toContain('Second?')

      act(() => { findRole('M')!.click() })
      expect(onSubmit).toHaveBeenCalledTimes(1)
      const response = onSubmit.mock.calls[0][0] as UserInputResponse
      expect(response.answers.q1).toEqual({ type: 'option', value: 'y' })
      expect(response.answers.q2).toEqual({ type: 'option', value: 'm' })
    })

    it('arrow button is disabled when no answer selected', () => {
      const onSubmit = vi.fn()
      // Use a question with no recommended (so nothing pre-selected)
      const req: UserInputRequest = {
        request_id: 'req_x',
        questions: [{
          id: 'q1',
          question: 'Pick?',
          options: [{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }],
        }],
      }
      renderCard(req, onSubmit)

      const arrow = findSubmitArrow()
      expect(arrow!.disabled).toBe(true)
      act(() => { arrow!.click() })
      expect(onSubmit).not.toHaveBeenCalled()
    })
  })

  describe('dismiss', () => {
    it('calls onDismiss when skip button clicked', () => {
      const onDismiss = vi.fn()
      renderCard(singleQuestion, vi.fn(), onDismiss)

      const skipBtn = findBtn('跳过') ?? findBtn('Skip')
      expect(skipBtn).toBeTruthy()
      act(() => { skipBtn!.click() })

      expect(onDismiss).toHaveBeenCalledTimes(1)
    })

    it('calls onDismiss on ESC key', () => {
      const onDismiss = vi.fn()
      renderCard(singleQuestion, vi.fn(), onDismiss)

      act(() => {
        window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
      })

      expect(onDismiss).toHaveBeenCalledTimes(1)
    })

  })
})
