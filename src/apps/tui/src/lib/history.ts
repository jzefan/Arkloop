import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs"
import { homedir } from "node:os"
import { join } from "node:path"

const MAX_HISTORY = 500

function historyPath(): string {
  return join(homedir(), ".arkloop", "tui-history.json")
}

export function loadHistory(): string[] {
  const path = historyPath()
  if (!existsSync(path)) return []
  try {
    const data = JSON.parse(readFileSync(path, "utf-8"))
    if (Array.isArray(data)) return data.filter((s) => typeof s === "string")
    return []
  } catch {
    return []
  }
}

export function saveHistory(entries: string[]): void {
  const path = historyPath()
  mkdirSync(join(homedir(), ".arkloop"), { recursive: true })
  writeFileSync(path, JSON.stringify(entries))
}

export function addEntry(entries: string[], text: string): string[] {
  const trimmed = text.trim()
  if (!trimmed) return entries
  // dedupe: remove if last entry is identical
  const next = entries.at(-1) === trimmed ? entries : [...entries, trimmed]
  if (next.length > MAX_HISTORY) return next.slice(next.length - MAX_HISTORY)
  return next
}

// cursor: -1 = not browsing, 0 = most recent, N = older
// returns [newCursor, textToShow]

export function historyUp(
  entries: string[],
  cursor: number,
  currentDraft: string,
): [number, string] {
  if (entries.length === 0) return [cursor, currentDraft]
  if (cursor < 0) {
    // start browsing from most recent
    const idx = entries.length - 1
    return [0, entries[idx]]
  }
  const next = Math.min(cursor + 1, entries.length - 1)
  return [next, entries[entries.length - 1 - next]]
}

export function historyDown(
  entries: string[],
  cursor: number,
  draft: string,
): [number, string] {
  if (cursor < 0) return [-1, draft]
  if (cursor === 0) return [-1, draft]
  const next = cursor - 1
  return [next, entries[entries.length - 1 - next]]
}
