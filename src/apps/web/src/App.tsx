import arkloopMark from './assets/arkloop.svg'
import {
  cancelRun,
  createMessage,
  createRun,
  createThread,
  getMe,
  isApiError,
  listMessages,
  listThreadRuns,
  listThreads,
  login,
  logout,
  register,
  type MessageResponse,
  type MeResponse,
  type ThreadResponse,
} from './api'
import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from 'react'
import { useSSE } from './hooks/useSSE'
import { LlmDebugPanel } from './components/LlmDebugPanel'
import { RunEventsPanel } from './components/RunEventsPanel'
import { SSEApiError } from './sse'
import { selectFreshRunEvents } from './runEventProcessing'
import {
  clearActiveThreadIdInStorage,
  readActiveThreadIdFromStorage,
  writeActiveThreadIdToStorage,
} from './storage'

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
  if (error instanceof SSEApiError) {
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

function AuthCard({
  onLoggedIn,
}: {
  onLoggedIn: (accessToken: string) => void
}) {
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [loginValue, setLoginValue] = useState('')
  const [password, setPassword] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<AppError | null>(null)

  const canSubmit = useMemo(() => {
    if (submitting) return false
    if (loginValue.trim().length === 0) return false
    if (password.length === 0) return false
    if (mode === 'register' && displayName.trim().length === 0) return false
    if (mode === 'register' && password.length < 8) return false
    return true
  }, [loginValue, password, displayName, submitting, mode])

  const onSubmit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (!canSubmit) return
    setSubmitting(true)
    setError(null)
    try {
      if (mode === 'login') {
        const resp = await login({ login: loginValue, password })
        onLoggedIn(resp.access_token)
      } else {
        const resp = await register({
          login: loginValue,
          password,
          display_name: displayName,
        })
        onLoggedIn(resp.access_token)
      }
    } catch (err) {
      setError(normalizeError(err))
    } finally {
      setSubmitting(false)
    }
  }

  const switchMode = () => {
    setMode(mode === 'login' ? 'register' : 'login')
    setError(null)
  }

  return (
    <div className="rounded-2xl border border-slate-800 bg-slate-900/40 p-6 shadow-sm">
      <div className="flex items-center justify-between">
        <h2 className="text-base font-semibold text-slate-100">
          {mode === 'login' ? '登录' : '注册'}
        </h2>
        <button
          className="text-sm text-indigo-400 hover:text-indigo-300"
          onClick={switchMode}
          type="button"
        >
          {mode === 'login' ? '没有账号？注册' : '已有账号？登录'}
        </button>
      </div>

      <form className="mt-6 space-y-4" onSubmit={onSubmit}>
        {mode === 'register' && (
          <label className="block">
            <div className="text-sm text-slate-300">显示名称</div>
            <input
              className="mt-1 w-full rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-slate-100 placeholder:text-slate-500 focus:outline-none focus:ring-2 focus:ring-indigo-500/50"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              placeholder="输入显示名称"
              autoComplete="name"
            />
          </label>
        )}

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
          <div className="text-sm text-slate-300">
            密码{mode === 'register' && <span className="text-slate-500">（至少8位）</span>}
          </div>
          <input
            className="mt-1 w-full rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-slate-100 placeholder:text-slate-500 focus:outline-none focus:ring-2 focus:ring-indigo-500/50"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            type="password"
            placeholder="输入密码"
            autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
          />
        </label>

        <button
          className="inline-flex w-full items-center justify-center rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-50"
          type="submit"
          disabled={!canSubmit}
        >
          {submitting
            ? mode === 'login'
              ? '登录中...'
              : '注册中...'
            : mode === 'login'
              ? '登录'
              : '注册'}
        </button>
      </form>

      {error ? <ErrorCallout error={error} /> : null}
    </div>
  )
}

type ThreadId = string
type RunId = string

function _threadTitle(thread: ThreadResponse): string {
  const title = (thread.title ?? '').trim()
  return title.length > 0 ? title : '未命名会话'
}

function _deriveThreadTitleFromFirstMessage(content: string): string {
  const cleaned = content.trim().replace(/\s+/g, ' ')
  if (!cleaned) return '新会话'
  return cleaned.length > 40 ? `${cleaned.slice(0, 40)}…` : cleaned
}

function ChatMvp({
  accessToken,
  onLoggedOut,
}: {
  accessToken: string
  onLoggedOut: () => void
}) {
  const baseUrl = (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? ''

  const [me, setMe] = useState<MeResponse | null>(null)
  const [threads, setThreads] = useState<ThreadResponse[]>([])
  const [threadsLoading, setThreadsLoading] = useState(false)

  const [activeThreadId, setActiveThreadId] = useState<ThreadId | null>(() => {
    return readActiveThreadIdFromStorage()
  })
  const [messages, setMessages] = useState<MessageResponse[]>([])
  const [messagesLoading, setMessagesLoading] = useState(false)

  const [draft, setDraft] = useState('')
  const [assistantDraft, setAssistantDraft] = useState('')

  const [activeRunId, setActiveRunId] = useState<RunId | null>(null)
  const [sending, setSending] = useState(false)
  const [cancelSubmitting, setCancelSubmitting] = useState(false)
  const [showEvents, setShowEvents] = useState(false)
  const [showDebug, setShowDebug] = useState(false)

  const [error, setError] = useState<AppError | null>(null)

  const sse = useSSE({
    runId: activeRunId ?? '',
    accessToken,
    baseUrl,
  })

  const activeThread = useMemo(() => {
    if (!activeThreadId) return null
    return threads.find((item) => item.id === activeThreadId) ?? null
  }, [threads, activeThreadId])

  const isStreaming = activeRunId != null
  const canCancel =
    activeRunId != null &&
    (sse.state === 'connecting' || sse.state === 'connected' || sse.state === 'reconnecting')

  const refreshThreads = useCallback(async () => {
    setThreadsLoading(true)
    setError(null)
    try {
      const items = await listThreads(accessToken, { limit: 200 })
      setThreads(items)

      if (activeThreadId && !items.some((item) => item.id === activeThreadId)) {
        clearActiveThreadIdInStorage()
        setActiveThreadId(null)
      }
    } catch (err) {
      setError(normalizeError(err))
    } finally {
      setThreadsLoading(false)
    }
  }, [accessToken, activeThreadId])

  const refreshMessages = useCallback(async () => {
    if (!activeThreadId) return
    setMessagesLoading(true)
    setError(null)
    try {
      const items = await listMessages(accessToken, activeThreadId)
      setMessages(items)
    } catch (err) {
      setError(normalizeError(err))
    } finally {
      setMessagesLoading(false)
    }
  }, [accessToken, activeThreadId])

  const selectThread = useCallback(
    (threadId: ThreadId) => {
      writeActiveThreadIdToStorage(threadId)
      setActiveThreadId(threadId)
    },
    [],
  )

  const handleLogout = useCallback(async () => {
    setError(null)
    try {
      await logout(accessToken)
    } catch (err) {
      const normalized = normalizeError(err)
      if (isApiError(err) && err.status === 401) {
        onLoggedOut()
        return
      }
      setError(normalized)
      return
    }
    onLoggedOut()
  }, [accessToken, onLoggedOut])

  const handleNewThread = useCallback(async () => {
    if (sending) return
    setSending(true)
    setError(null)
    try {
      const thread = await createThread(accessToken, { title: '新会话' })
      setThreads((prev) => [thread, ...prev])
      setMessages([])
      setAssistantDraft('')
      setActiveRunId(null)
      selectThread(thread.id)
    } catch (err) {
      setError(normalizeError(err))
    } finally {
      setSending(false)
    }
  }, [accessToken, selectThread, sending])

  const loadThreadStateRequestIdRef = useRef(0)
  useEffect(() => {
    if (!activeThreadId) {
      setMessages([])
      setActiveRunId(null)
      setAssistantDraft('')
      return
    }

    const requestId = ++loadThreadStateRequestIdRef.current
    setMessagesLoading(true)
    setError(null)

    void (async () => {
      try {
        const [items, runs] = await Promise.all([
          listMessages(accessToken, activeThreadId),
          listThreadRuns(accessToken, activeThreadId, 1),
        ])
        if (loadThreadStateRequestIdRef.current !== requestId) return
        setMessages(items)
        const latest = runs[0]
        setActiveRunId(latest?.status === 'running' ? latest.run_id : null)
      } catch (err) {
        if (loadThreadStateRequestIdRef.current !== requestId) return
        setError(normalizeError(err))
      } finally {
        if (loadThreadStateRequestIdRef.current === requestId) {
          setMessagesLoading(false)
        }
      }
    })()
  }, [accessToken, activeThreadId])

  useEffect(() => {
    setAssistantDraft('')
    setCancelSubmitting(false)
    setShowEvents(false)
    setShowDebug(false)
    setActiveRunId(null)
    sse.disconnect()
    sse.clearEvents()
  }, [activeThreadId]) // 切 thread 时清理 run 订阅

  useEffect(() => {
    if (!activeRunId) return
    sse.reset()
    sse.connect()
    return () => {
      sse.disconnect()
    }
  }, [activeRunId]) // 故意不依赖 sse 避免循环

  const processedEventCountRef = useRef(0)
  useEffect(() => {
    if (!activeRunId) return
    processedEventCountRef.current = 0
    setAssistantDraft('')
    setCancelSubmitting(false)
  }, [activeRunId])

  useEffect(() => {
    if (!activeRunId) return
    const { fresh, nextProcessedCount } = selectFreshRunEvents({
      events: sse.events,
      activeRunId,
      processedCount: processedEventCountRef.current,
    })
    processedEventCountRef.current = nextProcessedCount

    for (const event of fresh) {
      if (event.type === 'message.delta') {
        const data = event.data
        if (!data || typeof data !== 'object') continue
        const obj = data as { content_delta?: unknown; role?: unknown }
        if (obj.role != null && obj.role !== 'assistant') continue
        if (typeof obj.content_delta !== 'string') continue
        if (!obj.content_delta) continue
        setAssistantDraft((prev) => prev + obj.content_delta)
        continue
      }

      if (event.type === 'run.completed') {
        sse.disconnect()
        setActiveRunId(null)
        setAssistantDraft('')
        void refreshMessages()
        continue
      }

      if (event.type === 'run.cancelled') {
        sse.disconnect()
        setActiveRunId(null)
        const data = event.data
        const traceId =
          data && typeof data === 'object' && typeof (data as { trace_id?: unknown }).trace_id === 'string'
            ? String((data as { trace_id?: unknown }).trace_id)
            : undefined
        setError({ message: '已停止生成', traceId })
        continue
      }

      if (event.type === 'run.failed') {
        sse.disconnect()
        setActiveRunId(null)
        const data = event.data
        if (data && typeof data === 'object') {
          const obj = data as { message?: unknown; error_class?: unknown }
          setError({
            message: typeof obj.message === 'string' ? obj.message : '运行失败',
            code: typeof obj.error_class === 'string' ? obj.error_class : undefined,
          })
        } else {
          setError({ message: '运行失败' })
        }
      }
    }
  }, [activeRunId, refreshMessages, sse.events])

  useEffect(() => {
    void refreshThreads()
  }, [refreshThreads])

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const resp = await getMe(accessToken)
        if (cancelled) return
        setMe(resp)
      } catch (err) {
        if (cancelled) return
        if (isApiError(err) && err.status === 401) {
          onLoggedOut()
          return
        }
        setError(normalizeError(err))
      }
    })()
    return () => {
      cancelled = true
    }
  }, [accessToken, onLoggedOut])

  useEffect(() => {
    if (!sse.error) return
    if (sse.error instanceof SSEApiError && sse.error.status === 401) {
      onLoggedOut()
    }
  }, [onLoggedOut, sse.error])

  const handleSend = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (sending || isStreaming) return
    const content = draft.trim()
    if (!content) return

    setSending(true)
    setError(null)

    try {
      let threadId = activeThreadId
      if (!threadId) {
        const title = _deriveThreadTitleFromFirstMessage(content)
        const thread = await createThread(accessToken, { title })
        threadId = thread.id

        const message = await createMessage(accessToken, threadId, { content })
        const run = await createRun(accessToken, threadId)

        setThreads((prev) => [thread, ...prev])
        writeActiveThreadIdToStorage(threadId)
        setActiveThreadId(threadId)
        setMessages([message])
        setDraft('')
        setAssistantDraft('')
        setShowEvents(false)
        setActiveRunId(run.run_id)
        return
      }

      const message = await createMessage(accessToken, threadId, { content })
      setMessages((prev) => [...prev, message])
      setDraft('')
      setAssistantDraft('')
      setShowEvents(false)

      const run = await createRun(accessToken, threadId)
      setActiveRunId(run.run_id)
    } catch (err) {
      if (isApiError(err) && err.status === 401) {
        onLoggedOut()
        return
      }
      setError(normalizeError(err))
    } finally {
      setSending(false)
    }
  }

  const handleCancel = async () => {
    if (!activeRunId || cancelSubmitting) return
    setCancelSubmitting(true)
    setError(null)
    try {
      await cancelRun(accessToken, activeRunId)
    } catch (err) {
      setError(normalizeError(err))
      setCancelSubmitting(false)
    }
  }

  const terminalSseError = useMemo(() => {
    if (!sse.error) return null
    return normalizeError(sse.error)
  }, [sse.error])

  const selectedThreadLabel = activeThread ? _threadTitle(activeThread) : '请选择会话'

  return (
    <div className="mt-10 grid grid-cols-12 gap-6">
      <aside className="col-span-4 rounded-2xl border border-slate-800 bg-slate-900/40 shadow-sm">
        <div className="flex items-start justify-between gap-4 border-b border-slate-800 px-4 py-4">
          <div>
            <div className="text-sm font-semibold text-slate-100">会话</div>
            <div className="mt-1 text-xs text-slate-400">
              {me ? `你好，${me.display_name}` : '加载用户中...'}
            </div>
          </div>
          <button
            className="rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-xs text-slate-200 hover:bg-slate-950/60 disabled:cursor-not-allowed disabled:opacity-50"
            onClick={handleLogout}
            type="button"
            disabled={sending}
          >
            退出登录
          </button>
        </div>

        <div className="flex items-center gap-2 px-4 py-3">
          <button
            className="rounded-lg bg-indigo-600 px-3 py-2 text-xs font-medium text-white hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-50"
            onClick={handleNewThread}
            type="button"
            disabled={sending}
          >
            新建
          </button>
          <button
            className="rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-xs text-slate-200 hover:bg-slate-950/60 disabled:cursor-not-allowed disabled:opacity-50"
            onClick={refreshThreads}
            type="button"
            disabled={threadsLoading}
          >
            {threadsLoading ? '刷新中...' : '刷新'}
          </button>
        </div>

        <div className="max-h-[65vh] overflow-y-auto px-2 pb-2">
          {threads.length === 0 && !threadsLoading ? (
            <div className="px-2 py-10 text-center text-sm text-slate-500">
              暂无会话
            </div>
          ) : (
            <div className="space-y-1">
              {threads.map((thread) => {
                const isActive = thread.id === activeThreadId
                return (
                  <button
                    key={thread.id}
                    className={[
                      'w-full rounded-xl px-3 py-2 text-left',
                      isActive
                        ? 'bg-indigo-600/20 text-indigo-100 ring-1 ring-indigo-500/40'
                        : 'hover:bg-slate-950/30',
                    ].join(' ')}
                    onClick={() => selectThread(thread.id)}
                    type="button"
                  >
                    <div className="truncate text-sm">{_threadTitle(thread)}</div>
                    <div className="mt-1 truncate font-mono text-[11px] text-slate-500">
                      {thread.id}
                    </div>
                  </button>
                )
              })}
            </div>
          )}
        </div>
      </aside>

      <section className="col-span-8 flex h-[75vh] flex-col rounded-2xl border border-slate-800 bg-slate-900/40 shadow-sm">
        <div className="flex items-center justify-between gap-4 border-b border-slate-800 px-4 py-4">
          <div>
            <div className="text-sm font-semibold text-slate-100">{selectedThreadLabel}</div>
            {activeRunId ? (
              <div className="mt-1 text-xs text-slate-400">
                {canCancel ? '生成中' : '运行中断开'} · seq: {sse.lastSeq} · {sse.state}
              </div>
            ) : (
              <div className="mt-1 text-xs text-slate-400">准备就绪</div>
            )}
          </div>

          <div className="flex items-center gap-2">
            {(sse.state === 'closed' || sse.state === 'error') && activeRunId ? (
              <button
                className="rounded-lg bg-indigo-600 px-3 py-2 text-xs font-medium text-white hover:bg-indigo-500"
                onClick={sse.reconnect}
                type="button"
              >
                重连
              </button>
            ) : null}

            {activeRunId ? (
              <button
                className="rounded-lg border border-rose-900/50 bg-rose-950/30 px-3 py-2 text-xs text-rose-200 hover:bg-rose-950/50 disabled:cursor-not-allowed disabled:opacity-50"
                onClick={handleCancel}
                type="button"
                disabled={!canCancel || cancelSubmitting}
              >
                {cancelSubmitting ? '停止中...' : '停止'}
              </button>
            ) : null}

            {activeRunId ? (
              <button
                className="rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-xs text-slate-200 hover:bg-slate-950/60"
                onClick={() => setShowEvents((v) => !v)}
                type="button"
              >
                {showEvents ? '隐藏事件' : '显示事件'}
              </button>
            ) : null}

            {activeRunId || sse.events.length > 0 ? (
              <button
                className="rounded-lg border border-slate-700 bg-slate-950/40 px-3 py-2 text-xs text-slate-200 hover:bg-slate-950/60"
                onClick={() => setShowDebug((v) => !v)}
                type="button"
              >
                {showDebug ? '隐藏调试' : '调试'}
              </button>
            ) : null}
          </div>
        </div>

        <div className="flex-1 flex overflow-hidden">
          <div className="flex-1 space-y-4 overflow-y-auto px-4 py-4">
            {!activeThreadId ? (
              <div className="rounded-xl border border-dashed border-slate-800 bg-slate-950/20 px-4 py-10 text-center text-sm text-slate-500">
                选择一个会话，或点击“新建”开始聊天
              </div>
            ) : messagesLoading ? (
              <div className="px-2 py-10 text-center text-sm text-slate-500">
                加载消息中...
              </div>
            ) : (
              <>
                {messages.map((msg) => (
                  <div
                    key={msg.id}
                    className={[
                      'max-w-[85%] whitespace-pre-wrap rounded-2xl px-4 py-3 text-sm',
                      msg.role === 'user'
                        ? 'ml-auto bg-indigo-600/20 text-indigo-50 ring-1 ring-indigo-500/30'
                        : 'mr-auto bg-slate-950/40 text-slate-100 ring-1 ring-slate-800',
                    ].join(' ')}
                  >
                    {msg.content}
                  </div>
                ))}

                {assistantDraft ? (
                  <div className="mr-auto max-w-[85%] whitespace-pre-wrap rounded-2xl bg-slate-950/40 px-4 py-3 text-sm text-slate-100 ring-1 ring-slate-800">
                    {assistantDraft}
                  </div>
                ) : null}

                {terminalSseError ? <ErrorCallout error={terminalSseError} /> : null}
              </>
            )}
          </div>

          {showDebug && (activeRunId || sse.events.length > 0) ? (
            <div className="w-[440px] shrink-0 overflow-y-auto border-l border-slate-800 p-4">
              <LlmDebugPanel events={sse.events} onClear={sse.clearEvents} />
            </div>
          ) : null}
        </div>

        {showEvents && activeRunId ? (
          <div className="border-t border-slate-800 p-4">
            <RunEventsPanel
              events={sse.events}
              state={sse.state}
              lastSeq={sse.lastSeq}
              error={sse.error}
              onReconnect={sse.reconnect}
              onClear={sse.clearEvents}
            />
          </div>
        ) : null}

        <div className="border-t border-slate-800 px-4 py-4">
          <form className="flex items-start gap-3" onSubmit={handleSend}>
            <textarea
              className="min-h-[44px] w-full resize-none rounded-xl border border-slate-700 bg-slate-950/40 px-3 py-2 text-sm text-slate-100 placeholder:text-slate-500 focus:outline-none focus:ring-2 focus:ring-indigo-500/50 disabled:cursor-not-allowed disabled:opacity-50"
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              placeholder={activeThreadId ? '输入消息...' : '输入消息（将自动新建会话）...'}
              disabled={sending || isStreaming}
              rows={2}
            />
            <button
              className="rounded-xl bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-500 disabled:cursor-not-allowed disabled:opacity-50"
              type="submit"
              disabled={sending || isStreaming || !draft.trim()}
            >
              {sending ? '发送中...' : isStreaming ? '生成中' : '发送'}
            </button>
          </form>

          {error ? <ErrorCallout error={error} /> : null}
        </div>
      </section>
    </div>
  )
}

function App() {
  const [accessToken, setAccessToken] = useState<string | null>(null)

  const handleLoggedIn = useCallback((token: string) => {
    setAccessToken(token)
  }, [])

  const handleLoggedOut = useCallback(() => {
    clearActiveThreadIdInStorage()
    setAccessToken(null)
  }, [])

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100">
      <main className="mx-auto max-w-6xl px-6 py-16">
        <div className="flex items-center gap-4">
          <img src={arkloopMark} alt="Arkloop" className="h-10 w-10" />
          <h1 className="text-4xl font-semibold tracking-tight">Arkloop Web</h1>
        </div>

        <div className="mt-10 space-y-6">
          {accessToken ? (
            <ChatMvp accessToken={accessToken} onLoggedOut={handleLoggedOut} />
          ) : (
            <AuthCard onLoggedIn={handleLoggedIn} />
          )}
        </div>
      </main>
    </div>
  )
}

export default App
