import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  agentUIEventFromChunk,
  type AgentClient,
  type AgentStreamState,
  type AgentUIEvent,
  type AgentUIMessageChunk,
} from '../agent-ui'
import { clearLastSeqInStorage, readLastSeqFromStorage, writeLastSeqToStorage } from '../storage'
import { emitStreamDebug } from '../streamDebug'

type ActiveChunkStream = {
  abortController: AbortController
  reader: ReadableStreamDefaultReader<AgentUIMessageChunk>
}

export type UseAgentStreamOptions = {
  runId: string
  client: AgentClient
}

export type UseAgentStreamResult = {
  events: AgentUIEvent[]
  state: AgentStreamState
  lastSeq: number
  error: Error | null
  subscribeEvents: (listener: () => void) => () => void
  connect: () => void
  disconnect: () => void
  reconnect: () => void
  clearEvents: () => void
  reset: () => void
}

export function useAgentStream(options: UseAgentStreamOptions): UseAgentStreamResult {
  const { runId, client: agentClient } = options

  const [state, setState] = useState<AgentStreamState>('idle')
  const [error, setError] = useState<Error | null>(null)

  const clientRef = useRef<ActiveChunkStream | null>(null)
  const eventsRef = useRef<AgentUIEvent[]>([])
  const eventListenersRef = useRef(new Set<() => void>())
  const seenOrdersRef = useRef<Set<number>>(new Set())
  const cursorRef = useRef(0)
  const connectedRunIdRef = useRef('')
  const lastStorageWriteRef = useRef(0)
  const storageTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const notifyEventListeners = useCallback(() => {
    for (const listener of eventListenersRef.current) {
      listener()
    }
  }, [])

  const handleEvent = useCallback((event: AgentUIEvent) => {
    if (seenOrdersRef.current.has(event.order)) return
    seenOrdersRef.current.add(event.order)

    eventsRef.current.push(event)

    if (typeof event.order === 'number' && event.order >= 0) {
      cursorRef.current = event.order

      const now = Date.now()
      if (now - lastStorageWriteRef.current > 1000) {
        lastStorageWriteRef.current = now
        writeLastSeqToStorage(runId, event.order)
        if (storageTimerRef.current) {
          clearTimeout(storageTimerRef.current)
          storageTimerRef.current = null
        }
      } else if (!storageTimerRef.current) {
        storageTimerRef.current = setTimeout(() => {
          storageTimerRef.current = null
          lastStorageWriteRef.current = Date.now()
          writeLastSeqToStorage(runId, cursorRef.current)
        }, 1000)
      }
    }
    notifyEventListeners()
  }, [notifyEventListeners, runId])

  const handleStateChange = useCallback((newState: AgentStreamState) => {
    setState(newState)
    emitStreamDebug('agent-stream:state', {
      runId,
      state: newState,
      cursor: cursorRef.current,
    }, 'agent-stream')
    if (newState === 'connected') {
      setError(null)
    }
  }, [runId])

  const handleError = useCallback((err: Error) => {
    setError(err)
    emitStreamDebug('agent-stream:error', {
      runId,
      name: err.name,
      message: err.message,
      cursor: cursorRef.current,
    }, 'agent-stream')
  }, [runId])

  const closeActiveStream = useCallback(() => {
    const active = clientRef.current
    if (!active) return
    clientRef.current = null
    active.abortController.abort()
    void active.reader.cancel().catch(() => {})
  }, [])

  const connect = useCallback(() => {
    if (!runId) return

    closeActiveStream()
    setError(null)

    if (connectedRunIdRef.current !== runId) {
      connectedRunIdRef.current = runId
      cursorRef.current = 0
      eventsRef.current = []
      seenOrdersRef.current.clear()
      notifyEventListeners()
    }

    const stored = readLastSeqFromStorage(runId)
    const nextCursor = Math.max(cursorRef.current, stored)
    cursorRef.current = nextCursor
    emitStreamDebug('agent-stream:connect', {
      runId,
      cursor: nextCursor,
    }, 'agent-stream')

    const abortController = new AbortController()
    const stream = agentClient.openMessageChunkStream(runId, {
      cursor: cursorRef.current,
      live: true,
      onStateChange: handleStateChange,
      onError: handleError,
      signal: abortController.signal,
    })
    const reader = stream.getReader()
    clientRef.current = { abortController, reader }

    void (async () => {
      try {
        while (true) {
          const { done, value } = await reader.read()
          if (done) break
          const event = agentUIEventFromChunk(value)
          if (event) handleEvent(event)
        }
      } catch (err) {
        if (abortController.signal.aborted) return
        handleError(err instanceof Error ? err : new Error(String(err)))
      }
    })()
  }, [agentClient, closeActiveStream, handleError, handleEvent, handleStateChange, notifyEventListeners, runId])

  const disconnect = useCallback(() => {
    closeActiveStream()
    setError(null)
    emitStreamDebug('agent-stream:disconnect', {
      runId,
      cursor: cursorRef.current,
    }, 'agent-stream')
  }, [closeActiveStream, runId])

  const reconnect = useCallback(() => {
    setError(null)
    emitStreamDebug('agent-stream:reconnect', {
      runId,
      cursor: cursorRef.current,
    }, 'agent-stream')
    connect()
  }, [connect, runId])

  const clearEvents = useCallback(() => {
    eventsRef.current = []
    notifyEventListeners()
  }, [notifyEventListeners])

  const reset = useCallback(() => {
    clearLastSeqInStorage(runId)
    cursorRef.current = 0
    eventsRef.current = []
    seenOrdersRef.current.clear()
    setError(null)
    setState('idle')
    notifyEventListeners()
  }, [notifyEventListeners, runId])

  const subscribeEvents = useCallback((listener: () => void) => {
    eventListenersRef.current.add(listener)
    return () => {
      eventListenersRef.current.delete(listener)
    }
  }, [])

  useEffect(() => {
    return () => {
      disconnect()
    }
  }, [disconnect])

  useEffect(() => {
    return () => {
      if (storageTimerRef.current) {
        clearTimeout(storageTimerRef.current)
        storageTimerRef.current = null
      }
      if (cursorRef.current > 0) {
        writeLastSeqToStorage(runId, cursorRef.current)
      }
    }
  }, [runId])

  return useMemo(() => ({
    get events() {
      return eventsRef.current
    },
    state,
    get lastSeq() {
      return cursorRef.current
    },
    error,
    subscribeEvents,
    connect,
    disconnect,
    reconnect,
    clearEvents,
    reset,
  }), [state, error, subscribeEvents, connect, disconnect, reconnect, clearEvents, reset])
}
