import { useEffect, useLayoutEffect, useRef, useCallback } from 'react'
import { debugBus } from '@arkloop/shared'

const DEBUG_PANEL_KEY = 'arkloop:web:developer_show_debug_panel'
const EMIT_INTERVAL_MS = 500

function isDebugEnabled(): boolean {
  try {
    return localStorage.getItem(DEBUG_PANEL_KEY) === 'true'
  } catch {
    return false
  }
}

export function useInputPerfDebug() {
  const enabledRef = useRef(isDebugEnabled())
  const renderCountRef = useRef(0)
  const onChangeDurationsRef = useRef<number[]>([])
  const commitDurationsRef = useRef<number[]>([])
  const longTasksRef = useRef<number[]>([])
  const keystrokePaintRef = useRef<number[]>([])
  const fpsFramesRef = useRef(0)
  const fpsRef = useRef(0)
  const rafRef = useRef(0)
  const timerRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const lastFpsTimeRef = useRef(0)
  const renderStartRef = useRef(0)

  useLayoutEffect(() => {
    if (!enabledRef.current) return
    renderCountRef.current += 1
    renderStartRef.current = performance.now()
  })

  // measure render-to-commit (useEffect fires after commit)
  useEffect(() => {
    if (!enabledRef.current || renderStartRef.current === 0) return
    commitDurationsRef.current.push(performance.now() - renderStartRef.current)
  })

  // long task detection via PerformanceObserver
  useEffect(() => {
    if (typeof PerformanceObserver === 'undefined') return
    let observer: PerformanceObserver
    try {
      observer = new PerformanceObserver((list) => {
        if (!enabledRef.current) return
        for (const entry of list.getEntries()) {
          longTasksRef.current.push(entry.duration)
        }
      })
      observer.observe({ type: 'longtask', buffered: false })
    } catch {
      return
    }
    return () => observer.disconnect()
  }, [])

  // fps counter + periodic emit
  useEffect(() => {
    const clearSamples = () => {
      renderCountRef.current = 0
      onChangeDurationsRef.current = []
      commitDurationsRef.current = []
      longTasksRef.current = []
      keystrokePaintRef.current = []
      fpsFramesRef.current = 0
      fpsRef.current = 0
      lastFpsTimeRef.current = 0
    }

    const fpsLoop = (now: number) => {
      if (!enabledRef.current) {
        rafRef.current = 0
        return
      }
      fpsFramesRef.current++
      const elapsed = now - lastFpsTimeRef.current
      if (elapsed >= 1000) {
        fpsRef.current = Math.round((fpsFramesRef.current * 1000) / elapsed)
        fpsFramesRef.current = 0
        lastFpsTimeRef.current = now
      }
      rafRef.current = requestAnimationFrame(fpsLoop)
    }

    const emitSamples = () => {
      if (!enabledRef.current) return
      const onChange = onChangeDurationsRef.current
      const commits = commitDurationsRef.current
      const longTasks = longTasksRef.current
      const paints = keystrokePaintRef.current

      const avg = (arr: number[]) => arr.length > 0 ? arr.reduce((a, b) => a + b, 0) / arr.length : 0
      const max = (arr: number[]) => arr.length > 0 ? Math.max(...arr) : 0

      debugBus.emit({
        ts: Date.now(),
        type: 'perf:chat-input',
        source: 'input-perf',
        data: {
          fps: fpsRef.current,
          renders: renderCountRef.current,
          onChangeCount: onChange.length,
          onChangeAvgMs: +avg(onChange).toFixed(2),
          onChangeMaxMs: +max(onChange).toFixed(2),
          commitAvgMs: +avg(commits).toFixed(2),
          commitMaxMs: +max(commits).toFixed(2),
          longTasks: longTasks.length,
          longTaskMaxMs: +max(longTasks).toFixed(1),
          paintAvgMs: +avg(paints).toFixed(1),
          paintMaxMs: +max(paints).toFixed(1),
        },
      })
      renderCountRef.current = 0
      onChangeDurationsRef.current = []
      commitDurationsRef.current = []
      longTasksRef.current = []
      keystrokePaintRef.current = []
    }

    const start = () => {
      if (rafRef.current === 0) {
        rafRef.current = requestAnimationFrame(fpsLoop)
      }
      if (timerRef.current === null) {
        timerRef.current = setInterval(emitSamples, EMIT_INTERVAL_MS)
      }
    }

    const stop = () => {
      if (rafRef.current !== 0) {
        cancelAnimationFrame(rafRef.current)
        rafRef.current = 0
      }
      if (timerRef.current) {
        clearInterval(timerRef.current)
        timerRef.current = null
      }
      clearSamples()
    }

    const handler = (e: Event) => {
      enabledRef.current = !!(e as CustomEvent<boolean>).detail
      if (enabledRef.current) start()
      else stop()
    }

    if (enabledRef.current) start()
    window.addEventListener('arkloop:developer_show_debug_panel', handler as EventListener)

    return () => {
      window.removeEventListener('arkloop:developer_show_debug_panel', handler as EventListener)
      stop()
    }
  }, [])

  // wrap onChange to measure callback + keystroke-to-paint latency
  const wrapOnChange = useCallback(<T extends (val: string) => void>(fn: T): T => {
    return ((val: string) => {
      if (!enabledRef.current) {
        fn(val)
        return
      }
      const t0 = performance.now()
      fn(val)
      onChangeDurationsRef.current.push(performance.now() - t0)
      // measure time from keystroke to next paint
      requestAnimationFrame(() => {
        keystrokePaintRef.current.push(performance.now() - t0)
      })
    }) as T
  }, [])

  return { wrapOnChange }
}
