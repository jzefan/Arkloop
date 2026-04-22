import { batch, createSignal } from "solid-js"
import { createStore, produce } from "solid-js/store"
import {
  createEmptyAssistantTurnFoldState,
  foldAssistantTurnEvent,
  snapshotAssistantTurn,
  type AssistantTurnFoldState,
  type AssistantTurnUi,
} from "../lib/assistantTurn"
import { buildTurns } from "../lib/runTurns"
import type { LlmTurn, RunEventRaw } from "../lib/runTurns"

export interface PendingToolCall {
  runId: string
  toolCallId: string
  toolName: string
  args: Record<string, unknown>
}

const [historyTurns, setHistoryTurns] = createStore<LlmTurn[]>([])
const [liveRunEvents, setLiveRunEvents] = createStore<RunEventRaw[]>([])
const [liveAssistantTurn, setLiveAssistantTurn] = createSignal<AssistantTurnUi | null>(null)
const [streaming, setStreaming] = createSignal(false)
const [pendingUserInput, setPendingUserInput] = createSignal<string | null>(null)
const [pendingToolCall, setPendingToolCall] = createSignal<PendingToolCall | null>(null)
const [error, setError] = createSignal<string | null>(null)
const [debugInfo, setDebugInfo] = createSignal("")
let assistantTurnFoldState: AssistantTurnFoldState = createEmptyAssistantTurnFoldState()

export {
  historyTurns, setHistoryTurns,
  liveRunEvents, setLiveRunEvents,
  liveAssistantTurn, setLiveAssistantTurn,
  streaming, setStreaming,
  pendingUserInput, setPendingUserInput,
  pendingToolCall, setPendingToolCall,
  error, setError,
  debugInfo, setDebugInfo,
}

export function liveTurns(): LlmTurn[] {
  return buildTurns([...liveRunEvents])
}

export function allTurns(): LlmTurn[] {
  return [...historyTurns, ...liveTurns()]
}

export function startLiveTurn(input: string) {
  if (liveRunEvents.length > 0 || pendingUserInput() !== null || liveAssistantTurn() !== null) {
    commitLiveTurns()
  }
  batch(() => {
    setPendingUserInput(input)
    setLiveRunEvents([])
    assistantTurnFoldState = createEmptyAssistantTurnFoldState()
    setLiveAssistantTurn(null)
  })
}

export function appendRunEvent(event: RunEventRaw) {
  setLiveRunEvents(produce((items) => {
    items.push(event)
  }))
  if (event.type === "message.delta" || event.type === "tool.call" || event.type === "tool.result") {
    foldAssistantTurnEvent(assistantTurnFoldState, event)
    setLiveAssistantTurn(snapshotAssistantTurn(assistantTurnFoldState))
  }
}

export function commitLiveTurns() {
  const turns = liveTurns()
  batch(() => {
    if (turns.length > 0) {
      setHistoryTurns(produce((items) => {
        items.push(...turns)
      }))
    }
    setLiveRunEvents([])
    assistantTurnFoldState = createEmptyAssistantTurnFoldState()
    setLiveAssistantTurn(null)
    setPendingUserInput(null)
  })
}

export function clearChat() {
  setHistoryTurns([])
  setLiveRunEvents([])
  assistantTurnFoldState = createEmptyAssistantTurnFoldState()
  setLiveAssistantTurn(null)
  setStreaming(false)
  setPendingUserInput(null)
  setPendingToolCall(null)
  setError(null)
  setDebugInfo("")
}
