export type ConnectionMode = 'local' | 'saas' | 'self-hosted'

export type ArkloopDesktopApi = {
  isDesktop: true
  config: {
    get: () => Promise<{
      mode: ConnectionMode
      saas: { baseUrl: string }
      selfHosted: { baseUrl: string }
      local: { port: number }
      window: { width: number; height: number }
      onboarding_completed: boolean
    }>
    set: (config: unknown) => Promise<{ ok: boolean }>
    getPath: () => Promise<string>
    onChanged: (callback: (config: unknown) => void) => () => void
  }
  sidecar: {
    getStatus: () => Promise<'stopped' | 'starting' | 'running' | 'crashed'>
    restart: () => Promise<string>
    download: () => Promise<{ ok: boolean }>
    isAvailable: () => Promise<boolean>
    checkUpdate: () => Promise<{ current: string | null; latest: string | null; updateAvailable: boolean }>
    onStatusChanged: (callback: (status: string) => void) => () => void
    onDownloadProgress: (callback: (progress: { phase: string; percent: number; bytesDownloaded: number; bytesTotal: number; error?: string }) => void) => () => void
  }
  onboarding: {
    getStatus: () => Promise<{ completed: boolean }>
    complete: () => Promise<{ ok: boolean }>
  }
  app: {
    getVersion: () => Promise<string>
    quit: () => Promise<void>
  }
}

export function isDesktop(): boolean {
  return !!(globalThis as Record<string, unknown>).arkloop
}

export function getDesktopApi(): ArkloopDesktopApi | null {
  const api = (globalThis as Record<string, unknown>).arkloop as ArkloopDesktopApi | undefined
  return api?.isDesktop ? api : null
}

export function getDesktopMode(): ConnectionMode | null {
  const info = (globalThis as Record<string, unknown>).__ARKLOOP_DESKTOP__ as
    | { mode?: ConnectionMode }
    | undefined
  return info?.mode ?? null
}

export function isLocalMode(): boolean {
  return getDesktopMode() === 'local'
}
