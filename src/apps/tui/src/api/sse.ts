import type { SSEEvent } from "./types"
import { isTerminalEvent } from "./types"
import type { ApiClient } from "./client"

export type EventHandler = (event: SSEEvent) => void

/** Parse SSE text stream into typed events */
export async function* parseSSE(response: Response): AsyncGenerator<SSEEvent> {
  const reader = response.body!.getReader()
  const decoder = new TextDecoder()
  let buffer = ""
  let currentId = 0
  let currentEvent = ""
  let currentData = ""

  while (true) {
    const { done, value } = await reader.read()
    if (done) break

    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split("\n")
    buffer = lines.pop()! // keep incomplete line

    for (const line of lines) {
      if (line.startsWith(":")) continue // comment

      if (line === "") {
        // end of event
        if (currentData) {
          try {
            const payload = JSON.parse(currentData) as Record<string, unknown>
            const innerData =
              payload.data && typeof payload.data === "object" && !Array.isArray(payload.data)
                ? payload.data as Record<string, unknown>
                : {}
            const event: SSEEvent = {
              seq:
                typeof payload.seq === "number"
                  ? payload.seq
                  : currentId,
              type:
                typeof payload.type === "string" && payload.type.trim() !== ""
                  ? payload.type
                  : currentEvent,
              eventId: typeof payload.event_id === "string" ? payload.event_id : "",
              runId: typeof payload.run_id === "string" ? payload.run_id : "",
              ts:
                typeof payload.ts === "string"
                  ? payload.ts
                  : typeof payload.created_at === "string"
                    ? payload.created_at
                    : "",
              data: innerData,
              toolName: typeof payload.tool_name === "string" ? payload.tool_name : undefined,
              errorClass: typeof payload.error_class === "string" ? payload.error_class : undefined,
            }
            if (event.type) {
              yield event
            }
          } catch {
            // skip malformed JSON
          }
        }
        currentId = 0
        currentEvent = ""
        currentData = ""
        continue
      }

      if (line.startsWith("id:")) {
        currentId = parseInt(line.slice(3).trim(), 10) || 0
      } else if (line.startsWith("event:")) {
        currentEvent = line.slice(6).trim()
      } else if (line.startsWith("data:")) {
        const chunk = line.slice(5)
        currentData = currentData ? `${currentData}\n${chunk}` : chunk
      }
    }
  }
}

const BACKOFF_BASE = 500
const BACKOFF_MAX = 3000
const MAX_RETRIES = 5

/** Stream a run's events with reconnection logic */
export async function streamRun(
  client: ApiClient,
  runId: string,
  onEvent: EventHandler,
): Promise<void> {
  const seen = new Set<number>()
  let lastSeq = 0
  let retries = 0

  while (retries < MAX_RETRIES) {
    const response = await client.streamEvents(runId, lastSeq)
    if (!response.ok) {
      throw new Error(`SSE stream failed: ${response.status}`)
    }

    let gotTerminal = false
    for await (const event of parseSSE(response)) {
      if (seen.has(event.seq)) continue
      seen.add(event.seq)
      if (event.seq > lastSeq) lastSeq = event.seq

      onEvent(event)

      if (isTerminalEvent(event.type)) {
        gotTerminal = true
        break
      }
    }

    if (gotTerminal) return

    // Stream ended without terminal event -- check run status
    const run = await client.getRun(runId)
    if (run.status !== "running" && run.status !== "cancelling") return

    // Exponential backoff
    const delay = Math.min(BACKOFF_BASE * 2 ** retries, BACKOFF_MAX)
    await new Promise(r => setTimeout(r, delay))
    retries++
  }
}
