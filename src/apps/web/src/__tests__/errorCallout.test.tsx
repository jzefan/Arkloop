import { describe, expect, it, vi } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import type { ReactElement } from 'react'
import { LocaleProvider } from '../contexts/LocaleContext'
import { ErrorCallout, RunErrorNotice } from '../components/ErrorCallout'

vi.mock('../storage', async () => {
  const actual = await vi.importActual<typeof import('../storage')>('../storage')
  return {
    ...actual,
    readLocaleFromStorage: vi.fn(() => 'zh'),
    writeLocaleToStorage: vi.fn(),
  }
})

function renderWithLocale(ui: ReactElement): string {
  return renderToStaticMarkup(<LocaleProvider>{ui}</LocaleProvider>)
}

describe('ErrorCallout', () => {
  it('应将错误码映射为用户可读文案，并默认收起详情', () => {
    const html = renderWithLocale(
      <ErrorCallout error={{ message: 'invalid credentials', code: 'auth.invalid_credentials' }} />,
    )

    expect(html).toContain('账号或密码错误')
    expect(html).not.toContain('invalid credentials')
    expect(html).not.toContain('auth.invalid_credentials')
  })

  it('将本地模型用量限制显示为明确提示', () => {
    const html = renderWithLocale(
      <ErrorCallout error={{ message: 'Claude Code usage limit reached', code: 'provider.usage_limit' }} />,
    )

    expect(html).toContain('本地模型用量额度已用尽')
    expect(html).not.toContain('所需工具未配置')
  })

  it('run error notice 默认展开完整错误信息并提供关闭入口', () => {
    const html = renderWithLocale(
      <RunErrorNotice
        error={{
          message: 'provider request failed',
          code: 'provider.non_retryable',
          traceId: 'trace-1',
          details: {
            upstream: 'anthropic',
            provider_error_body: 'HTTP/2.0 400 Bad Request\r\nConnection: close\r\nAlt-Svc: h3=":443"',
          },
        }}
        onDismiss={() => {}}
      />,
    )

    expect(html).toContain('模型服务商请求失败')
    expect(html).toContain('原始信息: provider request failed')
    expect(html).toContain('错误码: provider.non_retryable')
    expect(html).toContain('Trace ID: trace-1')
    expect(html).toContain('upstream: anthropic')
    expect(html).toContain('provider_error_body: HTTP/2.0 400 Bad Request Connection: close Alt-Svc: h3=&quot;:443&quot;')
    expect(html).toContain('aria-label="收起"')
    expect(html).toContain('aria-label="关闭"')
    expect(html).toContain('#ea4d3c')
    expect(html).not.toContain('详情')
  })
})
