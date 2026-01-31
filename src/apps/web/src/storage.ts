export type RunDemoSession = {
  threadId: string
  runId: string
  traceId: string
}

const RUN_DEMO_SESSION_KEY = 'arkloop:web:run_demo_session'

function canUseLocalStorage(): boolean {
  try {
    return typeof localStorage !== 'undefined'
  } catch {
    return false
  }
}

function lastSeqStorageKey(runId: string): string {
  return `arkloop:sse:last_seq:${runId}`
}

export function readLastSeqFromStorage(runId: string): number {
  if (!runId || !canUseLocalStorage()) return 0
  try {
    const raw = localStorage.getItem(lastSeqStorageKey(runId))
    if (!raw) return 0
    const parsed = Number.parseInt(raw, 10)
    return Number.isFinite(parsed) && parsed >= 0 ? parsed : 0
  } catch {
    return 0
  }
}

export function writeLastSeqToStorage(runId: string, seq: number): void {
  if (!runId || !canUseLocalStorage()) return
  if (!Number.isFinite(seq) || seq < 0) return
  try {
    localStorage.setItem(lastSeqStorageKey(runId), String(seq))
  } catch {
    // 忽略存储失败（无痕模式/禁用存储等）
  }
}

export function clearLastSeqInStorage(runId: string): void {
  if (!runId || !canUseLocalStorage()) return
  try {
    localStorage.removeItem(lastSeqStorageKey(runId))
  } catch {
    // 忽略存储失败
  }
}

export function readRunDemoSession(): RunDemoSession | null {
  if (!canUseLocalStorage()) return null
  try {
    const raw = localStorage.getItem(RUN_DEMO_SESSION_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as unknown
    if (!parsed || typeof parsed !== 'object') return null

    const obj = parsed as Partial<RunDemoSession>
    if (!obj.runId || typeof obj.runId !== 'string' || obj.runId.length === 0) return null
    if (!obj.threadId || typeof obj.threadId !== 'string' || obj.threadId.length === 0) return null
    if (!obj.traceId || typeof obj.traceId !== 'string' || obj.traceId.length === 0) return null

    return { runId: obj.runId, threadId: obj.threadId, traceId: obj.traceId }
  } catch {
    return null
  }
}

export function writeRunDemoSession(session: RunDemoSession): void {
  if (!canUseLocalStorage()) return
  try {
    localStorage.setItem(RUN_DEMO_SESSION_KEY, JSON.stringify(session))
  } catch {
    // 忽略存储失败
  }
}

export function clearRunDemoSession(): void {
  if (!canUseLocalStorage()) return
  try {
    localStorage.removeItem(RUN_DEMO_SESSION_KEY)
  } catch {
    // 忽略存储失败
  }
}

