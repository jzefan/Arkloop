import { useCallback, useEffect, useRef, useState } from 'react'
import { createSSEClient, type RunEvent, type SSEClient, type SSEClientState } from '../sse'

function lastSeqStorageKey(runId: string): string {
  return `arkloop:sse:last_seq:${runId}`
}

function readLastSeqFromStorage(runId: string): number {
  if (!runId) return 0
  try {
    const raw = localStorage.getItem(lastSeqStorageKey(runId))
    if (!raw) return 0
    const parsed = Number.parseInt(raw, 10)
    return Number.isFinite(parsed) && parsed >= 0 ? parsed : 0
  } catch {
    return 0
  }
}

function writeLastSeqToStorage(runId: string, seq: number): void {
  if (!runId) return
  if (!Number.isFinite(seq) || seq < 0) return
  try {
    localStorage.setItem(lastSeqStorageKey(runId), String(seq))
  } catch {
    // 忽略存储失败（无痕模式/禁用存储等）
  }
}

function clearLastSeqInStorage(runId: string): void {
  if (!runId) return
  try {
    localStorage.removeItem(lastSeqStorageKey(runId))
  } catch {
    // 忽略存储失败
  }
}

export type UseSSEOptions = {
  runId: string
  accessToken: string
  baseUrl?: string
}

export type UseSSEResult = {
  events: RunEvent[]
  state: SSEClientState
  lastSeq: number
  error: Error | null
  connect: () => void
  disconnect: () => void
  reconnect: () => void
  clearEvents: () => void
  reset: () => void
}

/**
 * SSE 订阅 Hook
 * 管理 run 事件流的订阅与状态
 */
export function useSSE(options: UseSSEOptions): UseSSEResult {
  const { runId, accessToken, baseUrl = '' } = options

  const [events, setEvents] = useState<RunEvent[]>([])
  const [state, setState] = useState<SSEClientState>('idle')
  const [lastSeq, setLastSeq] = useState(0)
  const [error, setError] = useState<Error | null>(null)

  const clientRef = useRef<SSEClient | null>(null)
  const seenSeqsRef = useRef<Set<number>>(new Set())
  const cursorRef = useRef(0)
  const connectedRunIdRef = useRef('')

  // 构建 SSE URL
  const normalizedBaseUrl = baseUrl.replace(/\/$/, '')
  const sseUrl = `${normalizedBaseUrl}/v1/runs/${runId}/events`

  const handleEvent = useCallback((event: RunEvent) => {
    // 去重：避免重连时重复展示
    if (seenSeqsRef.current.has(event.seq)) return
    seenSeqsRef.current.add(event.seq)

    setEvents(prev => [...prev, event])

    if (typeof event.seq === 'number' && event.seq >= 0) {
      cursorRef.current = event.seq
      writeLastSeqToStorage(runId, event.seq)
      setLastSeq(event.seq)
    }
  }, [runId])

  const handleStateChange = useCallback((newState: SSEClientState) => {
    setState(newState)
  }, [])

  const handleError = useCallback((err: Error) => {
    setError(err)
  }, [])

  const connect = useCallback(() => {
    if (!runId || !accessToken) return

    if (clientRef.current) {
      clientRef.current.close()
    }

    setError(null)

    if (connectedRunIdRef.current !== runId) {
      connectedRunIdRef.current = runId
      cursorRef.current = 0
      setLastSeq(0)
      setEvents([])
      seenSeqsRef.current.clear()
    }

    const stored = readLastSeqFromStorage(runId)
    const nextCursor = Math.max(cursorRef.current, stored)
    cursorRef.current = nextCursor
    setLastSeq(nextCursor)

    const client = createSSEClient({
      url: sseUrl,
      accessToken,
      afterSeq: cursorRef.current,
      follow: true,
      onEvent: handleEvent,
      onStateChange: handleStateChange,
      onError: handleError,
    })

    clientRef.current = client
    void client.connect()
  }, [sseUrl, accessToken, runId, handleEvent, handleStateChange, handleError])

  const disconnect = useCallback(() => {
    clientRef.current?.close()
    clientRef.current = null
  }, [])

  const reconnect = useCallback(() => {
    setError(null)
    void clientRef.current?.reconnect()
  }, [])

  const clearEvents = useCallback(() => {
    setEvents([])
  }, [])

  const reset = useCallback(() => {
    clearLastSeqInStorage(runId)
    cursorRef.current = 0
    setLastSeq(0)
    setEvents([])
    seenSeqsRef.current.clear()
    setError(null)
    setState('idle')
  }, [runId])

  // 组件卸载时断开连接
  useEffect(() => {
    return () => {
      disconnect()
    }
  }, [disconnect])

  return {
    events,
    state,
    lastSeq,
    error,
    connect,
    disconnect,
    reconnect,
    clearEvents,
    reset,
  }
}
