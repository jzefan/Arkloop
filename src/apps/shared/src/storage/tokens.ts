function canUseLocalStorage(): boolean {
  try {
    return typeof localStorage !== 'undefined'
  } catch {
    return false
  }
}

const LEGACY_ACCESS_TOKEN_KEY = 'arkloop:web:access_token'
const LEGACY_REFRESH_TOKEN_WEB_KEY = 'arkloop:web:refresh_token'
const LEGACY_REFRESH_TOKEN_CONSOLE_KEY = 'arkloop:console:refresh_token'

let accessToken: string | null = null

function clearLegacyTokens(): void {
  if (!canUseLocalStorage()) return
  try {
    localStorage.removeItem(LEGACY_ACCESS_TOKEN_KEY)
    localStorage.removeItem(LEGACY_REFRESH_TOKEN_WEB_KEY)
    localStorage.removeItem(LEGACY_REFRESH_TOKEN_CONSOLE_KEY)
  } catch {
    // ignore
  }
}

clearLegacyTokens()

export function readAccessToken(): string | null {
  return accessToken?.trim() ? accessToken : null
}

export function writeAccessToken(token: string): void {
  const trimmed = token.trim()
  if (!trimmed) return
  accessToken = trimmed
}

export function clearAccessToken(): void {
  accessToken = null
}

export function canUseStorage(): boolean {
  return canUseLocalStorage()
}
