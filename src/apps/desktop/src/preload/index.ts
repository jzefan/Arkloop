import { contextBridge, ipcRenderer } from 'electron'

export type ConnectionMode = 'local' | 'saas' | 'self-hosted'

export type AppConfig = {
  mode: ConnectionMode
  saas: { baseUrl: string }
  selfHosted: { baseUrl: string }
  local: { port: number }
  window: { width: number; height: number }
}

export type SidecarStatus = 'stopped' | 'starting' | 'running' | 'crashed'

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
    onStatusChanged: (callback: (status: SidecarStatus) => void) => () => void
  }
  app: {
    getVersion: () => Promise<string>
    quit: () => Promise<void>
  }
}

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
    onStatusChanged: (callback) => {
      const handler = (_event: Electron.IpcRendererEvent, status: SidecarStatus) => callback(status)
      ipcRenderer.on('arkloop:sidecar:status-changed', handler)
      return () => ipcRenderer.removeListener('arkloop:sidecar:status-changed', handler)
    },
  },

  app: {
    getVersion: () => ipcRenderer.invoke('arkloop:app:version'),
    quit: () => ipcRenderer.invoke('arkloop:app:quit'),
  },
}

contextBridge.exposeInMainWorld('arkloop', api)
