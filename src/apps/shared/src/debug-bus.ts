export type DebugEntry = {
  ts: number
  type: string
  data: unknown
  source?: string
}

type Subscriber = (entry: DebugEntry) => void

const MAX_BUFFER = 500

let buffer: DebugEntry[] = []
let subscribers: Subscriber[] = []

export const debugBus = {
  emit(entry: DebugEntry) {
    buffer.push(entry)
    if (buffer.length > MAX_BUFFER) {
      buffer = buffer.slice(buffer.length - MAX_BUFFER)
    }
    for (const fn of subscribers) fn(entry)
  },

  subscribe(fn: Subscriber): () => void {
    subscribers.push(fn)
    return () => {
      subscribers = subscribers.filter((s) => s !== fn)
    }
  },

  snapshot(): readonly DebugEntry[] {
    return buffer
  },

  clear() {
    buffer = []
  },
}
