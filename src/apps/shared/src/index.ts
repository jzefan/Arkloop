export {
  TRACE_ID_HEADER,
  ApiError,
  isApiError,
  apiFetch,
  refreshAccessToken,
  initApiClient,
  setUnauthenticatedHandler,
  setAccessTokenHandler,
  apiBaseUrl,
  buildUrl,
  readJsonSafely,
} from './api/client'
export type { ErrorEnvelope } from './api/client'

export type { LoginRequest, LoginResponse } from './api/types'

export {
  readAccessToken,
  writeAccessToken,
  clearAccessToken,
  readRefreshToken,
  writeRefreshToken,
  clearRefreshToken,
  canUseStorage,
} from './storage/tokens'
export type { AppId } from './storage/tokens'

export { ThemeProvider, useTheme } from './contexts/ThemeContext'
export type { Theme } from './contexts/ThemeContext'

export { createLocaleContext } from './contexts/LocaleContext'
export type { Locale } from './contexts/LocaleContext'
