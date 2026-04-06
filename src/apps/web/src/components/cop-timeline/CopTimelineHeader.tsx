import { useState, useEffect } from 'react'
import { useLocale } from '../../contexts/LocaleContext'
import { useTypewriter } from '../../hooks/useTypewriter'

export const COP_HEADER_TRANSITION_RETAIN_MS = 320
export const COP_SUMMARY_TRANSITION_RETAIN_MS = 100

export function useThinkingElapsedSeconds(active: boolean, startedAtMs?: number): number {
  const readElapsed = () => {
    if (!active || !startedAtMs) return 0
    return Math.max(1, Math.round((Date.now() - startedAtMs) / 1000))
  }
  const [elapsed, setElapsed] = useState(readElapsed)

  useEffect(() => {
    if (!active || !startedAtMs) {
      if (!active) setElapsed(0)
      return
    }
    setElapsed(readElapsed())
    const id = setInterval(() => {
      setElapsed(readElapsed())
    }, 1000)
    return () => clearInterval(id)
  }, [active, startedAtMs])

  return elapsed
}

export function formatThinkingHeaderLabel(thinkingHint: string | undefined, elapsedSeconds: number, t: ReturnType<typeof useLocale>['t']): string {
  if (thinkingHint && thinkingHint.trim() !== '') {
    return `${thinkingHint} for ${elapsedSeconds}s`
  }
  return t.copTimelineThinkingForSeconds(elapsedSeconds)
}

export function CopTimelineHeaderLabel({
  text,
  phaseKey,
  shimmer,
  typewriter,
}: {
  text: string
  phaseKey: string
  shimmer?: boolean
  typewriter?: boolean
}) {
  const displayed = useTypewriter(text, !typewriter)
  return (
    <span
      data-phase={phaseKey}
      className={shimmer ? 'thinking-shimmer' : undefined}
    >
      {typewriter ? displayed : text}
    </span>
  )
}
