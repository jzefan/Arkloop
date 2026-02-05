import type { RunEvent } from './sse'

export function selectFreshRunEvents(params: {
  events: RunEvent[]
  activeRunId: string
  processedCount: number
}): { fresh: RunEvent[]; nextProcessedCount: number } {
  const { events, activeRunId } = params
  const normalizedProcessedCount = params.processedCount > events.length ? 0 : params.processedCount

  if (events.length <= normalizedProcessedCount) {
    return { fresh: [], nextProcessedCount: normalizedProcessedCount }
  }

  const slice = events.slice(normalizedProcessedCount)
  return {
    fresh: slice.filter((event) => event.run_id === activeRunId),
    nextProcessedCount: events.length,
  }
}

