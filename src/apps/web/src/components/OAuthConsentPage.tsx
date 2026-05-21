import { useEffect, useMemo, useState } from 'react'

/**
 * /oauth/consent page.
 *
 * The OAuth authorize endpoint redirects here when the current user has not
 * yet granted (or has revoked) consent for the requesting OAuth client.
 *
 * Lifecycle:
 *   1. Pull metadata from GET /v1/auth/oauth/consent?<original authorize query>
 *      → renders the client name + list of scopes + their human descriptions
 *   2. "Allow" form-POSTs the same query parameters back to /v1/auth/oauth/consent;
 *      the server records consent, issues a code, and 302s to the client's
 *      redirect_uri — the browser handles the navigation natively.
 *   3. "Deny" navigates the user straight to redirect_uri with the standard
 *      `error=access_denied&state=<state>` parameters, skipping the server.
 */
type ConsentMeta = {
  client: { client_id: string; name: string }
  scopes_requested: string[]
  scope_descriptions: Record<string, string>
}

export function OAuthConsentPage() {
  const params = useMemo(() => new URLSearchParams(window.location.search), [])
  const [meta, setMeta] = useState<ConsentMeta | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    fetch(`/v1/auth/oauth/consent?${params.toString()}`, {
      credentials: 'include',
      headers: { Accept: 'application/json' },
    })
      .then(async (r) => {
        if (!r.ok) {
          throw new Error((await r.json()).error_description ?? r.statusText)
        }
        return r.json() as Promise<ConsentMeta>
      })
      .then((data) => {
        if (!cancelled) setMeta(data)
      })
      .catch((err) => {
        if (!cancelled) setError(String(err))
      })
    return () => {
      cancelled = true
    }
  }, [params])

  const handleDeny = () => {
    const redirectURI = params.get('redirect_uri') ?? ''
    if (!redirectURI) return
    const u = new URL(redirectURI)
    u.searchParams.set('error', 'access_denied')
    if (params.get('state')) u.searchParams.set('state', params.get('state')!)
    window.location.replace(u.toString())
  }

  if (error) {
    return (
      <div style={pageStyle}>
        <div style={cardStyle}>
          <h1 style={{ color: 'var(--c-status-error)', marginBottom: 12 }}>授权失败</h1>
          <p style={{ color: 'var(--c-text-secondary)' }}>{error}</p>
        </div>
      </div>
    )
  }

  if (!meta) {
    return (
      <div style={pageStyle}>
        <div style={cardStyle}>加载中…</div>
      </div>
    )
  }

  return (
    <div style={pageStyle}>
      <div style={cardStyle}>
        <h1 style={{ marginBottom: 8 }}>授权访问</h1>
        <p style={{ color: 'var(--c-text-secondary)', marginBottom: 24 }}>
          <strong>{meta.client.name}</strong> 申请访问你的 ArkLoop 账号
        </p>
        <ul style={{ listStyle: 'none', padding: 0, marginBottom: 24 }}>
          {meta.scopes_requested.map((scope) => (
            <li key={scope} style={scopeItemStyle}>
              <span style={{ fontFamily: 'monospace', fontWeight: 600 }}>{scope}</span>
              <span style={{ color: 'var(--c-text-secondary)', fontSize: '0.9em' }}>
                {meta.scope_descriptions[scope] ?? '(未知权限)'}
              </span>
            </li>
          ))}
        </ul>
        <form method="POST" action={`/v1/auth/oauth/consent`} style={{ display: 'flex', gap: 12 }}>
          {/* Forward all authorize-time params verbatim so the server can issue
              the code with identical context. */}
          {Array.from(params.entries()).map(([k, v]) => (
            <input key={k} type="hidden" name={k} value={v} />
          ))}
          <button type="button" onClick={handleDeny} style={denyButtonStyle}>
            拒绝
          </button>
          <button type="submit" style={allowButtonStyle}>
            允许
          </button>
        </form>
      </div>
    </div>
  )
}

const pageStyle: React.CSSProperties = {
  minHeight: '100vh',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  background: 'var(--c-bg-page)',
  padding: 16,
}

const cardStyle: React.CSSProperties = {
  background: 'var(--c-bg-sub)',
  border: '1px solid var(--c-border)',
  borderRadius: 12,
  padding: 32,
  maxWidth: 460,
  width: '100%',
  color: 'var(--c-text-primary)',
}

const scopeItemStyle: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: 4,
  padding: '10px 12px',
  background: 'var(--c-bg-deep)',
  borderRadius: 6,
  marginBottom: 8,
}

const denyButtonStyle: React.CSSProperties = {
  flex: 1,
  padding: '10px 16px',
  borderRadius: 6,
  border: '1px solid var(--c-border)',
  background: 'transparent',
  color: 'var(--c-text-primary)',
  cursor: 'pointer',
}

const allowButtonStyle: React.CSSProperties = {
  flex: 1,
  padding: '10px 16px',
  borderRadius: 6,
  border: 'none',
  background: 'var(--c-text-primary)',
  color: 'var(--c-bg-page)',
  cursor: 'pointer',
  fontWeight: 600,
}
