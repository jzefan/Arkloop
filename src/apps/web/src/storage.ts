const ACCESS_TOKEN_KEY = 'arkloop:web:access_token'
const REFRESH_TOKEN_KEY = 'arkloop:web:refresh_token'
const ACTIVE_THREAD_ID_KEY = 'arkloop:web:active_thread_id'
const LOCALE_KEY = 'arkloop:web:locale'
const THEME_KEY = 'arkloop:web:theme'
const SELECTED_TIER_KEY = 'arkloop:web:selected_tier'

export type Theme = 'system' | 'light' | 'dark'

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

export function readAccessTokenFromStorage(): string | null {
  if (!canUseLocalStorage()) return null
  try {
    const raw = localStorage.getItem(ACCESS_TOKEN_KEY)
    return raw?.trim() ? raw : null
  } catch {
    return null
  }
}

export function writeAccessTokenToStorage(token: string): void {
  if (!canUseLocalStorage() || !token.trim()) return
  try {
    localStorage.setItem(ACCESS_TOKEN_KEY, token)
  } catch {
    // ignore
  }
}

export function clearAccessTokenFromStorage(): void {
  if (!canUseLocalStorage()) return
  try {
    localStorage.removeItem(ACCESS_TOKEN_KEY)
  } catch {
    // ignore
  }
}

export function readRefreshTokenFromStorage(): string | null {
  if (!canUseLocalStorage()) return null
  try {
    const raw = localStorage.getItem(REFRESH_TOKEN_KEY)
    return raw?.trim() ? raw : null
  } catch {
    return null
  }
}

export function writeRefreshTokenToStorage(token: string): void {
  if (!canUseLocalStorage() || !token.trim()) return
  try {
    localStorage.setItem(REFRESH_TOKEN_KEY, token)
  } catch {
    // ignore
  }
}

export function clearRefreshTokenFromStorage(): void {
  if (!canUseLocalStorage()) return
  try {
    localStorage.removeItem(REFRESH_TOKEN_KEY)
  } catch {
    // ignore
  }
}

export function readActiveThreadIdFromStorage(): string | null {
  if (!canUseLocalStorage()) return null
  try {
    const raw = localStorage.getItem(ACTIVE_THREAD_ID_KEY)
    if (!raw) return null
    return raw.trim() ? raw : null
  } catch {
    return null
  }
}

export function writeActiveThreadIdToStorage(threadId: string): void {
  if (!canUseLocalStorage()) return
  if (!threadId.trim()) return
  try {
    localStorage.setItem(ACTIVE_THREAD_ID_KEY, threadId)
  } catch {
    // 忽略存储失败
  }
}

export function clearActiveThreadIdInStorage(): void {
  if (!canUseLocalStorage()) return
  try {
    localStorage.removeItem(ACTIVE_THREAD_ID_KEY)
  } catch {
    // 忽略存储失败
  }
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

export function readLocaleFromStorage(): import('./locales').Locale {
  if (!canUseLocalStorage()) return 'zh'
  try {
    const raw = localStorage.getItem(LOCALE_KEY)
    if (raw === 'zh' || raw === 'en') return raw
    return 'zh'
  } catch {
    return 'zh'
  }
}

export function writeLocaleToStorage(locale: import('./locales').Locale): void {
  if (!canUseLocalStorage()) return
  try {
    localStorage.setItem(LOCALE_KEY, locale)
  } catch {
    // 忽略存储失败
  }
}

export function readThemeFromStorage(): Theme {
  if (!canUseLocalStorage()) return 'system'
  try {
    const raw = localStorage.getItem(THEME_KEY)
    if (raw === 'system' || raw === 'light' || raw === 'dark') return raw
    return 'system'
  } catch {
    return 'system'
  }
}

export function writeThemeToStorage(theme: Theme): void {
  if (!canUseLocalStorage()) return
  try {
    localStorage.setItem(THEME_KEY, theme)
  } catch {
    // 忽略存储失败
  }
}

export type SelectedTier = 'Auto' | 'Lite' | 'Pro' | 'Ultra' | 'Search'

export function readSelectedTierFromStorage(): SelectedTier {
  if (!canUseLocalStorage()) return 'Lite'
  try {
    const raw = localStorage.getItem(SELECTED_TIER_KEY)
    if (raw === 'Auto' || raw === 'Lite' || raw === 'Pro' || raw === 'Ultra' || raw === 'Search') return raw
    return 'Lite'
  } catch {
    return 'Lite'
  }
}

export function writeSelectedTierToStorage(tier: SelectedTier): void {
  if (!canUseLocalStorage()) return
  try {
    localStorage.setItem(SELECTED_TIER_KEY, tier)
  } catch {
    // 忽略存储失败
  }
}

export type WebSource = {
  title: string
  url: string
  snippet?: string
}

function messageSourcesKey(messageId: string): string {
  return `arkloop:web:msg_sources:${messageId}`
}

export function readMessageSources(messageId: string): WebSource[] | null {
  if (!canUseLocalStorage() || !messageId) return null
  try {
    const raw = localStorage.getItem(messageSourcesKey(messageId))
    if (!raw) return null
    return JSON.parse(raw) as WebSource[]
  } catch {
    return null
  }
}

export function writeMessageSources(messageId: string, sources: WebSource[]): void {
  if (!canUseLocalStorage() || !messageId || sources.length === 0) return
  try {
    localStorage.setItem(messageSourcesKey(messageId), JSON.stringify(sources))
  } catch { /* ignore */ }
}

const SEARCH_THREAD_IDS_KEY = 'arkloop:web:search_thread_ids'

export function addSearchThreadId(threadId: string): void {
  if (!canUseLocalStorage()) return
  try {
    const raw = localStorage.getItem(SEARCH_THREAD_IDS_KEY)
    const ids: string[] = raw ? (JSON.parse(raw) as string[]) : []
    if (ids.includes(threadId)) return
    ids.push(threadId)
    if (ids.length > 500) ids.splice(0, ids.length - 500)
    localStorage.setItem(SEARCH_THREAD_IDS_KEY, JSON.stringify(ids))
  } catch { /* ignore */ }
}

export function isSearchThreadId(threadId: string): boolean {
  if (!canUseLocalStorage()) return false
  try {
    const raw = localStorage.getItem(SEARCH_THREAD_IDS_KEY)
    if (!raw) return false
    return (JSON.parse(raw) as string[]).includes(threadId)
  } catch { return false }
}
