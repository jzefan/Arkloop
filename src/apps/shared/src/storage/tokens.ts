function canUseLocalStorage(): boolean {
  try {
    return typeof localStorage !== 'undefined'
  } catch {
    return false
  }
}

export type AppId = 'web' | 'console'

const ACCESS_TOKEN_KEY = 'arkloop:web:access_token'

function refreshTokenKey(app: AppId): string {
  return `arkloop:${app}:refresh_token`
}

export function readAccessToken(): string | null {
  if (!canUseLocalStorage()) return null
  try {
    const raw = localStorage.getItem(ACCESS_TOKEN_KEY)
    return raw?.trim() ? raw : null
  } catch {
    return null
  }
}

export function writeAccessToken(token: string): void {
  if (!canUseLocalStorage() || !token.trim()) return
  try {
    localStorage.setItem(ACCESS_TOKEN_KEY, token)
  } catch {
    // ignore
  }
}

export function clearAccessToken(): void {
  if (!canUseLocalStorage()) return
  try {
    localStorage.removeItem(ACCESS_TOKEN_KEY)
  } catch {
    // ignore
  }
}

export function readRefreshToken(app: AppId): string | null {
  if (!canUseLocalStorage()) return null
  try {
    const raw = localStorage.getItem(refreshTokenKey(app))
    return raw?.trim() ? raw : null
  } catch {
    return null
  }
}

export function writeRefreshToken(app: AppId, token: string): void {
  if (!canUseLocalStorage() || !token.trim()) return
  try {
    localStorage.setItem(refreshTokenKey(app), token)
  } catch {
    // ignore
  }
}

export function clearRefreshToken(app: AppId): void {
  if (!canUseLocalStorage()) return
  try {
    localStorage.removeItem(refreshTokenKey(app))
  } catch {
    // ignore
  }
}

export function canUseStorage(): boolean {
  return canUseLocalStorage()
}
