// @vitest-environment jsdom

import { createElement } from 'react'
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AuthPage, type AuthApi, type AuthPageTranslations } from './AuthPage'

const t: AuthPageTranslations = {
  requestFailed: 'Request failed',
  loginMode: 'Login',
  enterYourPasswordTitle: 'Enter password',
  otpLoginTab: 'OTP',
  continueBtn: 'Continue',
  otpSendBtn: 'Send code',
  otpVerifyBtn: 'Verify',
  backBtn: 'Back',
  fieldIdentity: 'Username or email',
  identityPlaceholder: 'Username or email',
  editIdentity: 'Edit',
  fieldPassword: 'Password',
  enterPassword: 'Enter password',
  otpEmailPlaceholder: 'Email',
  otpCodePlaceholder: 'Code',
  useEmailOtpHint: 'Use email code',
  otpSendingCountdown: (n) => `Wait ${n}s`,
  registerMode: 'Register',
  creatingAccountHint: 'Creating account',
  enterUsername: 'Enter username',
  enterEmail: 'Enter email',
  registerPasswordHint: 'Use a password',
}

let root: Root | null = null
let host: HTMLDivElement | null = null

afterEach(() => {
  if (root) {
    act(() => root?.unmount())
  }
  root = null
  host?.remove()
  host = null
})

function changeInput(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
  setter?.call(input, value)
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

async function renderAuth(api: AuthApi) {
  host = document.createElement('div')
  document.body.append(host)
  root = createRoot(host)
  await act(async () => {
    root?.render(createElement(AuthPage, { onLoggedIn: vi.fn(), brandLabel: 'ArkLoop', locale: 'en', t, api }))
  })
  return host
}

describe('AuthPage', () => {
  it('does not let browser email autofill overwrite the register username field', async () => {
    const api: AuthApi = {
      login: vi.fn(),
      getCaptchaConfig: vi.fn().mockResolvedValue({ enabled: false, site_key: '' }),
      resolveIdentity: vi.fn().mockResolvedValue({
        next_step: 'register',
        prefill: { login: 'admin' },
      }),
    }
    const container = await renderAuth(api)

    const identityInput = container.querySelector('input[placeholder="Username or email"]') as HTMLInputElement
    await act(async () => {
      changeInput(identityInput, 'admin')
    })
    await act(async () => {
      container.querySelector('form')?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    })

    const usernameInput = container.querySelector('input[placeholder="Enter username"]') as HTMLInputElement
    expect(usernameInput.value).toBe('admin')
    expect(usernameInput.getAttribute('autocomplete')).toBe('off')
  })
})
