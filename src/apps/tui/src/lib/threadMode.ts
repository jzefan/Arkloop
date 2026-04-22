import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs"
import { homedir } from "node:os"
import { join } from "node:path"
import type { Thread } from "../api/types"
import type { EffortLevel } from "./effort"

export type ThreadMode = "chat" | "work"

interface TuiState {
  threadModes?: Record<string, ThreadMode>
  lastModel?: string
  modelEfforts?: Record<string, EffortLevel>
}

function statePath(): string {
  return join(homedir(), ".arkloop", "tui-state.json")
}

function readState(): TuiState {
  const path = statePath()
  if (!existsSync(path)) return {}
  try {
    return JSON.parse(readFileSync(path, "utf-8")) as TuiState
  } catch {
    return {}
  }
}

function writeState(next: TuiState): void {
  const path = statePath()
  mkdirSync(join(homedir(), ".arkloop"), { recursive: true })
  writeFileSync(path, JSON.stringify(next, null, 2))
}

export function readThreadMode(threadId: string): ThreadMode | null {
  if (!threadId) return null
  const mode = readState().threadModes?.[threadId]
  if (mode === "work") return "work"
  if (mode === "chat") return "chat"
  return null
}

export function writeThreadMode(threadId: string, mode: ThreadMode): void {
  if (!threadId) return
  const state = readState()
  state.threadModes = state.threadModes ?? {}
  state.threadModes[threadId] = mode
  writeState(state)
}

export function isWorkThread(thread: Pick<Thread, "id" | "mode">): boolean {
  if (thread.mode === "work") return true
  return readThreadMode(thread.id) === "work"
}

export function readLastModel(): string | null {
  const value = readState().lastModel
  return typeof value === "string" && value.trim() !== "" ? value : null
}

export function writeLastModel(model: string): void {
  if (!model) return
  const state = readState()
  state.lastModel = model
  writeState(state)
}

export function readModelEffort(model: string): EffortLevel | null {
  if (!model) return null
  const effort = readState().modelEfforts?.[model]
  return effort ?? null
}

export function writeModelEffort(model: string, effort: EffortLevel): void {
  if (!model) return
  const state = readState()
  state.modelEfforts = state.modelEfforts ?? {}
  state.modelEfforts[model] = effort
  writeState(state)
}
