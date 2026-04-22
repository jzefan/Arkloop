import { createSignal } from "solid-js"
import type { EffortLevel } from "../lib/effort"
import { readModelEffort, writeLastModel, writeModelEffort } from "../lib/threadMode"

export const [connected, setConnected] = createSignal(true)
export const [currentModel, setCurrentModel] = createSignal("")
export const [currentModelLabel, setCurrentModelLabel] = createSignal("")
export const [currentModelSupportsReasoning, setCurrentModelSupportsReasoning] = createSignal(false)
export const [currentModelContextLength, setCurrentModelContextLength] = createSignal<number | null>(null)
export const [currentPersona, setCurrentPersona] = createSignal("")
export const [currentPersonaLabel, setCurrentPersonaLabel] = createSignal("")
export const [currentEffort, setCurrentEffort] = createSignal<EffortLevel>("medium")
export const [overlay, setOverlay] = createSignal<"model" | "session" | "tool" | "effort" | null>(null)
export const [tokenUsage, setTokenUsage] = createSignal({ input: 0, output: 0, context: 0 })
export const [currentThreadId, setCurrentThreadId] = createSignal<string | null>(null)
let focusInputHandler: (() => void) | null = null

export function applyCurrentModel(model: string, label?: string, supportsReasoning = false, contextLength: number | null = null) {
  setCurrentModel(model)
  setCurrentModelLabel(label ?? model)
  setCurrentModelSupportsReasoning(supportsReasoning)
  setCurrentModelContextLength(contextLength)
  writeLastModel(model)
  if (supportsReasoning) {
    setCurrentEffort(readModelEffort(model) ?? "medium")
  }
}

export function applyCurrentPersona(persona: string, label?: string) {
  setCurrentPersona(persona)
  setCurrentPersonaLabel(label ?? persona)
}

export function applyCurrentEffort(effort: EffortLevel) {
  setCurrentEffort(effort)
  const model = currentModel()
  if (model) writeModelEffort(model, effort)
}

export function registerInputFocus(handler: (() => void) | null) {
  focusInputHandler = handler
}

export function focusInput() {
  focusInputHandler?.()
}
