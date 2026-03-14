export type ConnectionMode = 'local' | 'saas' | 'self-hosted'

export type AppConfig = {
  mode: ConnectionMode
  saas: { baseUrl: string }
  selfHosted: { baseUrl: string }
  local: { port: number }
  window: { width: number; height: number }
  onboarding_completed: boolean
}

export const DEFAULT_CONFIG: AppConfig = {
  mode: 'local',
  saas: { baseUrl: 'https://api.arkloop.com' },
  selfHosted: { baseUrl: '' },
  local: { port: 19001 },
  window: { width: 1280, height: 800 },
  onboarding_completed: false,
}
