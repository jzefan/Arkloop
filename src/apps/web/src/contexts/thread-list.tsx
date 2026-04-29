import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { silentRefresh } from '@arkloop/shared'
import { isLocalMode } from '@arkloop/shared/desktop'
import {
  isApiError,
  listThreads,
  streamThreadRunStateEvents,
  type CollaborationMode,
  type ThreadResponse,
} from '../api'
import {
  readAppModeFromStorage,
  readGtdEnabled,
  readGtdInboxThreadIds,
  readThreadModes,
  writeGtdInboxThreadIds,
  writeThreadMode,
  type AppMode,
} from '../storage'
import { useAuth } from './auth'

export interface ThreadListContextValue {
  threads: ThreadResponse[]
  runningThreadIds: Set<string>
  completedUnreadThreadIds: Set<string>
  privateThreadIds: Set<string>
  isPrivateMode: boolean
  pendingIncognitoMode: boolean
  addThread: (thread: ThreadResponse) => void
  upsertThread: (thread: ThreadResponse) => void
  removeThread: (threadId: string) => void
  updateTitle: (threadId: string, title: string) => void
  updateCollaborationMode: (threadId: string, collaborationMode: CollaborationMode, revision?: number) => void
  markRunning: (threadId: string) => void
  markIdle: (threadId: string) => void
  markCompletionRead: (threadId: string) => void
  togglePrivateMode: () => void
  setPendingIncognito: (v: boolean) => void
  getFilteredThreads: (appMode: AppMode) => ThreadResponse[]
}

const ThreadListContext = createContext<ThreadListContextValue | null>(null)
const THREAD_RUN_STATE_RECONNECT_DELAY_MS = 1000

export function ThreadListProvider({ children }: { children: ReactNode }) {
  const { accessToken } = useAuth()
  const mountedRef = useRef(true)

  const [threads, setThreads] = useState<ThreadResponse[]>([])
  const [threadModes, setThreadModes] = useState<Record<string, AppMode>>(() => readThreadModes())
  const [runningThreadIds, setRunningThreadIds] = useState<Set<string>>(new Set())
  const runningThreadIdsRef = useRef<Set<string>>(new Set())
  const [completedUnreadThreadIds, setCompletedUnreadThreadIds] = useState<Set<string>>(new Set())
  const [privateThreadIds, setPrivateThreadIds] = useState<Set<string>>(new Set())
  const [isPrivateMode, setIsPrivateMode] = useState(false)
  const [pendingIncognitoMode, setPendingIncognitoMode] = useState(false)

  const replaceRunningThreadIds = useCallback((next: Set<string>) => {
    runningThreadIdsRef.current = next
    setRunningThreadIds(next)
  }, [])

  const addCompletedUnread = useCallback((threadIds: Iterable<string>) => {
    setCompletedUnreadThreadIds((prev) => {
      let next: Set<string> | null = null
      for (const threadId of threadIds) {
        if (prev.has(threadId)) continue
        next ??= new Set(prev)
        next.add(threadId)
      }
      return next ?? prev
    })
  }, [])

  const clearCompletedUnread = useCallback((threadIds: Iterable<string>) => {
    setCompletedUnreadThreadIds((prev) => {
      let next: Set<string> | null = null
      for (const threadId of threadIds) {
        if (!prev.has(threadId)) continue
        next ??= new Set(prev)
        next.delete(threadId)
      }
      return next ?? prev
    })
  }, [])

  const applyThreadState = useCallback((threadId: string, activeRunId: string | null, title?: string | null) => {
    const isRunning = activeRunId != null
    const wasRunning = runningThreadIdsRef.current.has(threadId)
    if (wasRunning && !isRunning) {
      addCompletedUnread([threadId])
    } else if (isRunning) {
      clearCompletedUnread([threadId])
    }
    if (wasRunning !== isRunning) {
      const next = new Set(runningThreadIdsRef.current)
      if (isRunning) {
        next.add(threadId)
      } else {
        next.delete(threadId)
      }
      replaceRunningThreadIds(next)
    }

    setThreads((prev) => {
      const idx = prev.findIndex((t) => t.id === threadId)
      if (idx < 0) return prev
      const current = prev[idx]
      const nextTitle = title === undefined ? current.title : title
      const updated = current.active_run_id === activeRunId && current.title === nextTitle
        ? current
        : { ...current, active_run_id: activeRunId, title: nextTitle }
      if (activeRunId != null && idx > 0) {
        return [updated, ...prev.slice(0, idx), ...prev.slice(idx + 1)]
      }
      if (updated === current) return prev
      const next = prev.slice()
      next[idx] = updated
      return next
    })
  }, [addCompletedUnread, clearCompletedUnread, replaceRunningThreadIds])

  const syncThreadList = useCallback(async (token: string) => {
    const items = await listThreads(token, { limit: 200 })
    if (!mountedRef.current) return
    const nextRunning = new Set(items.filter((t) => t.active_run_id != null).map((t) => t.id))
    const completedThreadIds = Array.from(runningThreadIdsRef.current).filter((threadId) => !nextRunning.has(threadId))
    setThreads(items)
    replaceRunningThreadIds(nextRunning)
    if (completedThreadIds.length > 0) addCompletedUnread(completedThreadIds)
    if (nextRunning.size > 0) clearCompletedUnread(nextRunning)
  }, [addCompletedUnread, clearCompletedUnread, replaceRunningThreadIds])

  useEffect(() => {
    mountedRef.current = true
    return () => { mountedRef.current = false }
  }, [])

  useEffect(() => {
    if (!accessToken) return
    let stopped = false
    let streamController: AbortController | null = null
    let retryTimer: ReturnType<typeof setTimeout> | null = null
    let streamAccessToken = accessToken

    const clearRetryTimer = () => {
      if (retryTimer === null) return
      clearTimeout(retryTimer)
      retryTimer = null
    }

    const scheduleReconnect = (connect: () => void) => {
      clearRetryTimer()
      retryTimer = setTimeout(connect, THREAD_RUN_STATE_RECONNECT_DELAY_MS)
    }

    const connect = () => {
      if (stopped) return
      const controller = new AbortController()
      streamController = controller
      let shouldReconnect = true

      void (async () => {
        await syncThreadList(streamAccessToken)
        if (stopped || controller.signal.aborted) return
        await streamThreadRunStateEvents(streamAccessToken, {
          signal: controller.signal,
          onEvent: (event) => {
            if (stopped) return
            applyThreadState(event.thread_id, event.active_run_id, event.title)
          },
        })
      })()
        .catch((err: unknown) => {
          if (controller.signal.aborted) return
          if (isApiError(err) && err.status === 401 && !isLocalMode()) {
            return silentRefresh()
              .then((token) => { streamAccessToken = token })
              .catch(() => { shouldReconnect = false })
          }
          if (isApiError(err) && (err.status === 401 || err.status === 403)) {
            shouldReconnect = false
          }
        })
        .finally(() => {
          if (stopped || controller.signal.aborted || !shouldReconnect) return
          scheduleReconnect(connect)
        })
    }

    connect()

    return () => {
      stopped = true
      clearRetryTimer()
      streamController?.abort()
    }
  }, [accessToken, applyThreadState, syncThreadList])

  const addThread = useCallback((thread: ThreadResponse) => {
    if (thread.is_private) {
      setPrivateThreadIds((prev) => new Set(prev).add(thread.id))
    } else {
      const mode = readAppModeFromStorage()
      writeThreadMode(thread.id, mode)
      setThreadModes((prev) => (prev[thread.id] === mode ? prev : { ...prev, [thread.id]: mode }))
      if (readGtdEnabled() && mode === 'chat') {
        const inboxIds = readGtdInboxThreadIds()
        inboxIds.add(thread.id)
        writeGtdInboxThreadIds(inboxIds)
      }
    }
    setThreads((prev) => {
      if (prev.some((t) => t.id === thread.id)) return prev
      return [thread, ...prev]
    })
  }, [])

  const upsertThread = useCallback((thread: ThreadResponse) => {
    if (thread.is_private) {
      setPrivateThreadIds((prev) => new Set(prev).add(thread.id))
    }
    setThreads((prev) => {
      const idx = prev.findIndex((t) => t.id === thread.id)
      if (idx < 0) return [thread, ...prev]
      return prev.map((item, currentIndex) => (currentIndex === idx ? { ...item, ...thread } : item))
    })
  }, [])

  const removeThread = useCallback((threadId: string) => {
    setThreads((prev) => prev.filter((t) => t.id !== threadId))
    clearCompletedUnread([threadId])
    if (runningThreadIdsRef.current.has(threadId)) {
      const next = new Set(runningThreadIdsRef.current)
      next.delete(threadId)
      replaceRunningThreadIds(next)
    }
  }, [clearCompletedUnread, replaceRunningThreadIds])

  const updateTitle = useCallback((threadId: string, title: string) => {
    setThreads((prev) =>
      prev.map((t) => (t.id === threadId ? { ...t, title } : t)),
    )
  }, [])

  const updateCollaborationMode = useCallback((threadId: string, collaborationMode: CollaborationMode, revision?: number) => {
    setThreads((prev) =>
      prev.map((t) => {
        if (t.id !== threadId) return t
        if (revision !== undefined && revision <= (t.collaboration_mode_revision ?? 0)) return t
        return {
          ...t,
          collaboration_mode: collaborationMode,
          collaboration_mode_revision: revision ?? t.collaboration_mode_revision,
        }
      }),
    )
  }, [])

  const markRunning = useCallback((threadId: string) => {
    clearCompletedUnread([threadId])
    if (!runningThreadIdsRef.current.has(threadId)) {
      replaceRunningThreadIds(new Set(runningThreadIdsRef.current).add(threadId))
    }
    setThreads((prev) => {
      const idx = prev.findIndex((t) => t.id === threadId)
      if (idx <= 0) return prev
      const thread = prev[idx]
      return [thread, ...prev.slice(0, idx), ...prev.slice(idx + 1)]
    })
  }, [clearCompletedUnread, replaceRunningThreadIds])

  const markIdle = useCallback((threadId: string) => {
    applyThreadState(threadId, null)
  }, [applyThreadState])

  const markCompletionRead = useCallback((threadId: string) => {
    clearCompletedUnread([threadId])
  }, [clearCompletedUnread])

  const togglePrivateMode = useCallback(() => {
    setIsPrivateMode((prev) => !prev)
  }, [])

  const threadsByMode = useMemo<Record<AppMode, ThreadResponse[]>>(() => {
    const grouped: Record<AppMode, ThreadResponse[]> = {
      chat: [],
      work: [],
    }
    for (const thread of threads) {
      if (thread.is_private) continue
      const mode = threadModes[thread.id] ?? 'chat'
      grouped[mode].push(thread)
    }
    return grouped
  }, [threadModes, threads])

  const getFilteredThreads = useCallback(
    (appMode: AppMode): ThreadResponse[] => threadsByMode[appMode],
    [threadsByMode],
  )

  const value = useMemo<ThreadListContextValue>(() => ({
    threads,
    runningThreadIds,
    completedUnreadThreadIds,
    privateThreadIds,
    isPrivateMode,
    pendingIncognitoMode,
    addThread,
    upsertThread,
    removeThread,
    updateTitle,
    updateCollaborationMode,
    markRunning,
    markIdle,
    markCompletionRead,
    togglePrivateMode,
    setPendingIncognito: setPendingIncognitoMode,
    getFilteredThreads,
  }), [
    threads,
    runningThreadIds,
    completedUnreadThreadIds,
    privateThreadIds,
    isPrivateMode,
    pendingIncognitoMode,
    addThread,
    upsertThread,
    removeThread,
    updateTitle,
    updateCollaborationMode,
    markRunning,
    markIdle,
    markCompletionRead,
    togglePrivateMode,
    getFilteredThreads,
  ])

  return (
    <ThreadListContext.Provider value={value}>
      {children}
    </ThreadListContext.Provider>
  )
}

export function ThreadListContextBridge({
  value,
  children,
}: {
  value: ThreadListContextValue
  children: ReactNode
}) {
  return (
    <ThreadListContext.Provider value={value}>
      {children}
    </ThreadListContext.Provider>
  )
}

export function useThreadList(): ThreadListContextValue {
  const ctx = useContext(ThreadListContext)
  if (!ctx) throw new Error('useThreadList must be used within ThreadListProvider')
  return ctx
}
