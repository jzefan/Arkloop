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
    }>
    set: (config: unknown) => Promise<{ ok: boolean }>
    getPath: () => Promise<string>
    onChanged: (callback: (config: unknown) => void) => () => void
  }
  sidecar: {
    getStatus: () => Promise<'stopped' | 'starting' | 'running' | 'crashed'>
    restart: () => Promise<string>
    onStatusChanged: (callback: (status: string) => void) => () => void
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
