import type { AgentUIEvent, AgentUIMessageChunk } from './contract'

export function agentUIEventFromChunk(chunk: AgentUIMessageChunk): AgentUIEvent | null {
  if (chunk.type !== 'data-agent-event') return null
  return chunk.data as AgentUIEvent
}

export async function readAgentUIEvents(stream: ReadableStream<AgentUIMessageChunk>): Promise<AgentUIEvent[]> {
  const reader = stream.getReader()
  const events: AgentUIEvent[] = []
  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      const event = agentUIEventFromChunk(value)
      if (event) events.push(event)
    }
  } finally {
    reader.releaseLock()
  }
  return events
}
