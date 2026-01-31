import arkloopMark from './assets/arkloop.svg'
import { getMe, isApiError, login, type MeResponse } from './api'
import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react'

type AppError = {
  message: string
  traceId?: string
  code?: string
}

function normalizeError(error: unknown): AppError {
  if (isApiError(error)) {
    return {
      message: error.message,
      traceId: error.traceId,
      code: error.code,
    }
  }
  if (error instanceof Error) {
    return { message: error.message }
  }
  return { message: '请求失败' }
}

function ErrorCallout({ error }: { error: AppError }) {
  return (
    <div className="mt-4 rounded-lg border border-rose-900/40 bg-rose-950/40 px-4 py-3 text-sm">
      <div className="font-medium text-rose-200">请求失败</div>
      <div className="mt-1 text-rose-100/90">{error.message}</div>
      <div className="mt-2 space-y-1 text-rose-100/80">
        {error.code ? <div>code: {error.code}</div> : null}
        {error.traceId ? <div>trace_id: {error.traceId}</div> : null}
      </div>
    </div>
  )
}

function LoginCard({
  onLoggedIn,
}: {
  onLoggedIn: (accessToken: string) => void
}) {
  const [loginValue, setLoginValue] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<AppError | null>(null)

  const canSubmit = useMemo(() => {
    if (submitting) return false
    return loginValue.trim().length > 0 && password.length > 0
  }, [loginValue, password, submitting])

  const onSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (!canSubmit) return
    setSubmitting(true)
    setError(null)
    try {
      const resp = await login({ login: loginValue, password })
      onLoggedIn(resp.access_token)
    } catch (err) {
      setError(normalizeError(err))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className="rounded-2xl border border-slate-800 bg-slate-900/40 p-6 shadow-sm">
      <h2 className="text-base font-semibold text-slate-100">登录</h2>

      <form className="mt-6 space-y-4" onSubmit={onSubmit}>
        <label className="block">
          <div className="text-sm text-slate-300">登录名</div>
          <input
            className="mt-1 w-full rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-slate-100 placeholder:text-slate-500 focus:outline-none focus:ring-2 focus:ring-indigo-500/50"
            value={loginValue}
            onChange={(e) => setLoginValue(e.target.value)}
            placeholder="输入登录名"
            autoComplete="username"
          />
        </label>

        <label className="block">
          <div className="text-sm text-slate-300">密码</div>
          <input
            className="mt-1 w-full rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-slate-100 placeholder:text-slate-500 focus:outline-none focus:ring-2 focus:ring-indigo-500/50"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            type="password"
            placeholder="输入密码"
            autoComplete="current-password"
          />
        </label>

        <button
          className="inline-flex w-full items-center justify-center rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-50"
          type="submit"
          disabled={!canSubmit}
        >
          {submitting ? '登录中…' : '登录'}
        </button>
      </form>

      {error ? <ErrorCallout error={error} /> : null}
    </div>
  )
}

function MeCard({
  accessToken,
  onLogout,
}: {
  accessToken: string
  onLogout: () => void
}) {
  const [me, setMe] = useState<MeResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<AppError | null>(null)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const resp = await getMe(accessToken)
      setMe(resp)
    } catch (err) {
      setError(normalizeError(err))
    } finally {
      setLoading(false)
    }
  }, [accessToken])

  useEffect(() => {
    void refresh()
  }, [refresh])

  return (
    <div className="rounded-2xl border border-slate-800 bg-slate-900/40 p-6 shadow-sm">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-base font-semibold text-slate-100">当前用户</h2>
        </div>
        <button
          className="rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-sm text-slate-200 hover:bg-slate-950/60 disabled:cursor-not-allowed disabled:opacity-50"
          onClick={onLogout}
          type="button"
        >
          退出登录
        </button>
      </div>

      <div className="mt-6">
        <div className="flex items-center gap-3">
          <button
            className="rounded-lg bg-slate-200 px-3 py-2 text-sm font-medium text-slate-950 hover:bg-white disabled:cursor-not-allowed disabled:opacity-50"
            onClick={refresh}
            type="button"
            disabled={loading}
          >
            {loading ? '刷新中…' : '刷新'}
          </button>
        </div>

        {me ? (
          <div className="mt-4 rounded-lg border border-slate-800 bg-slate-950/30 px-4 py-3 text-sm">
            <div className="text-slate-300">id</div>
            <div className="mt-1 font-mono text-slate-100">{me.id}</div>
            <div className="mt-4 text-slate-300">display_name</div>
            <div className="mt-1 text-slate-100">{me.display_name}</div>
          </div>
        ) : null}

        {error ? <ErrorCallout error={error} /> : null}
      </div>
    </div>
  )
}

function App() {
  const [accessToken, setAccessToken] = useState<string | null>(null)

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100">
      <main className="mx-auto max-w-3xl px-6 py-16">
        <div className="flex items-center gap-4">
          <img src={arkloopMark} alt="Arkloop" className="h-10 w-10" />
          <h1 className="text-4xl font-semibold tracking-tight">Arkloop Web</h1>
        </div>

        <div className="mt-10">
          {accessToken ? (
            <MeCard
              accessToken={accessToken}
              onLogout={() => setAccessToken(null)}
            />
          ) : (
            <LoginCard onLoggedIn={(t) => setAccessToken(t)} />
          )}
        </div>
      </main>
    </div>
  )
}

export default App
