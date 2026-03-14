import { contextBridge, ipcRenderer } from 'electron'

export type ConnectionMode = 'local' | 'saas' | 'self-hosted'

export type AppConfig = {
  mode: ConnectionMode
  saas: { baseUrl: string }
  selfHosted: { baseUrl: string }
  local: { port: number }
  window: { width: number; height: number }
  onboarding_completed: boolean
}

export type SidecarStatus = 'stopped' | 'starting' | 'running' | 'crashed'

export type DownloadProgress = {
  phase: 'connecting' | 'downloading' | 'verifying' | 'done' | 'error'
  percent: number
  bytesDownloaded: number
  bytesTotal: number
  error?: string
}

export type SidecarVersionInfo = {
  current: string | null
  latest: string | null
  updateAvailable: boolean
}

export type RootfsStatus = 'not_installed' | 'downloading' | 'ready' | 'error'

export type RootfsProgress = {
  phase: 'connecting' | 'downloading' | 'extracting' | 'done' | 'error'
  percent: number
  bytesDownloaded: number
  bytesTotal: number
  error?: string
}

export type RootfsVersionInfo = {
  current: string | null
  latest: string | null
  updateAvailable: boolean
}

export type ArkloopDesktopApi = {
  isDesktop: true
  config: {
    get: () => Promise<AppConfig>
    set: (config: AppConfig) => Promise<{ ok: boolean }>
    getPath: () => Promise<string>
    onChanged: (callback: (config: AppConfig) => void) => () => void
  }
  sidecar: {
    getStatus: () => Promise<SidecarStatus>
    restart: () => Promise<SidecarStatus>
    download: () => Promise<{ ok: boolean }>
    isAvailable: () => Promise<boolean>
    checkUpdate: () => Promise<SidecarVersionInfo>
    onStatusChanged: (callback: (status: SidecarStatus) => void) => () => void
    onDownloadProgress: (callback: (progress: DownloadProgress) => void) => () => void
  }
  rootfs: {
    getStatus: () => Promise<RootfsStatus>
    isAvailable: () => Promise<boolean>
    getPath: () => Promise<string>
    checkVersion: () => Promise<RootfsVersionInfo>
    download: () => Promise<{ ok: boolean }>
    delete: () => Promise<{ ok: boolean }>
    onDownloadProgress: (callback: (progress: RootfsProgress) => void) => () => void
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

// 同步注入 __ARKLOOP_DESKTOP__, 必须在页面脚本执行前完成
const config = ipcRenderer.sendSync('arkloop:config:get-sync') as {
  mode: string
  saas: { baseUrl: string }
  selfHosted: { baseUrl: string }
  local: { port: number }
}

const isDevMode = process.env.ELECTRON_DEV === 'true'

let apiBaseUrl = ''
if (config.mode === 'local') {
  apiBaseUrl = isDevMode ? '' : `http://127.0.0.1:${config.local.port}`
} else if (config.mode === 'saas') {
  apiBaseUrl = config.saas.baseUrl
} else if (config.mode === 'self-hosted') {
  apiBaseUrl = config.selfHosted.baseUrl
}

contextBridge.exposeInMainWorld('__ARKLOOP_DESKTOP__', {
  apiBaseUrl,
  mode: config.mode,
})

const api: ArkloopDesktopApi = {
  isDesktop: true,

  config: {
    get: () => ipcRenderer.invoke('arkloop:config:get'),
    set: (config) => ipcRenderer.invoke('arkloop:config:set', config),
    getPath: () => ipcRenderer.invoke('arkloop:config:path'),
    onChanged: (callback) => {
      const handler = (_event: Electron.IpcRendererEvent, config: AppConfig) => callback(config)
      ipcRenderer.on('arkloop:config:changed', handler)
      return () => ipcRenderer.removeListener('arkloop:config:changed', handler)
    },
  },

  sidecar: {
    getStatus: () => ipcRenderer.invoke('arkloop:sidecar:status'),
    restart: () => ipcRenderer.invoke('arkloop:sidecar:restart'),
    download: () => ipcRenderer.invoke('arkloop:sidecar:download'),
    isAvailable: () => ipcRenderer.invoke('arkloop:sidecar:is-available'),
    checkUpdate: () => ipcRenderer.invoke('arkloop:sidecar:check-update'),
    onStatusChanged: (callback) => {
      const handler = (_event: Electron.IpcRendererEvent, status: SidecarStatus) => callback(status)
      ipcRenderer.on('arkloop:sidecar:status-changed', handler)
      return () => ipcRenderer.removeListener('arkloop:sidecar:status-changed', handler)
    },
    onDownloadProgress: (callback) => {
      const handler = (_event: Electron.IpcRendererEvent, progress: DownloadProgress) => callback(progress)
      ipcRenderer.on('arkloop:sidecar:download-progress', handler)
      return () => ipcRenderer.removeListener('arkloop:sidecar:download-progress', handler)
    },
  },

  rootfs: {
    getStatus: () => ipcRenderer.invoke('arkloop:rootfs:status'),
    isAvailable: () => ipcRenderer.invoke('arkloop:rootfs:available'),
    getPath: () => ipcRenderer.invoke('arkloop:rootfs:path'),
    checkVersion: () => ipcRenderer.invoke('arkloop:rootfs:check-version'),
    download: () => ipcRenderer.invoke('arkloop:rootfs:download'),
    delete: () => ipcRenderer.invoke('arkloop:rootfs:delete'),
    onDownloadProgress: (callback) => {
      const handler = (_event: Electron.IpcRendererEvent, progress: RootfsProgress) => callback(progress)
      ipcRenderer.on('arkloop:rootfs:download-progress', handler)
      return () => ipcRenderer.removeListener('arkloop:rootfs:download-progress', handler)
    },
  },

  onboarding: {
    getStatus: () => ipcRenderer.invoke('arkloop:onboarding:status'),
    complete: () => ipcRenderer.invoke('arkloop:onboarding:complete'),
  },

  app: {
    getVersion: () => ipcRenderer.invoke('arkloop:app:version'),
    quit: () => ipcRenderer.invoke('arkloop:app:quit'),
  },
}

contextBridge.exposeInMainWorld('arkloop', api)
