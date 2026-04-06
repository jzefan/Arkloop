import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { isDesktop } from '@arkloop/shared/desktop'
import type { SettingsTab } from '../components/SettingsModal'
import type { DesktopSettingsKey } from '../components/DesktopSettings'
import {
  readAppModeFromStorage,
  writeAppModeToStorage,
  type AppMode,
} from '../storage'
import { beginPerfTrace, endPerfTrace, recordPerfDuration } from '../perfDebug'
import { useAuth } from './auth'

export interface AppUIContextValue {
  sidebarCollapsed: boolean
  sidebarHiddenByWidth: boolean
  rightPanelOpen: boolean
  isSearchMode: boolean
  settingsOpen: boolean
  settingsInitialTab: SettingsTab
  desktopSettingsSection: DesktopSettingsKey
  notificationsOpen: boolean
  notificationVersion: number
  appMode: AppMode
  availableAppModes: AppMode[]
  pendingSkillPrompt: string | null

  toggleSidebar: () => void
  setRightPanelOpen: (open: boolean) => void
  enterSearchMode: () => void
  exitSearchMode: () => void
  openSettings: (tab?: SettingsTab | 'voice') => void
  closeSettings: () => void
  openNotifications: () => void
  closeNotifications: () => void
  markNotificationRead: () => void
  setAppMode: (mode: AppMode) => void
  queueSkillPrompt: (prompt: string) => void
  consumeSkillPrompt: () => void
  setTitleBarIncognitoClick: (fn: (() => void) | null) => void
  triggerTitleBarIncognitoClick: (fallback: () => void) => void
}

const AppUIContext = createContext<AppUIContextValue | null>(null)

export function AppUIProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate()
  const location = useLocation()
  const { me } = useAuth()
  const desktop = isDesktop()
  const usesHashRouting = window.location.protocol === 'file:'

  const [sidebarCollapsed, setSidebarCollapsed] = useState(() => window.innerWidth < 1200)
  const [sidebarHiddenByWidth, setSidebarHiddenByWidth] = useState(() => window.innerWidth < 900)
  const collapsedByWidthRef = useRef(window.innerWidth < 1200)
  const [rightPanelOpen, setRightPanelOpen] = useState(false)
  const [isSearchMode, setIsSearchMode] = useState(false)
  const isSearchModeRef = useRef(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [settingsInitialTab, setSettingsInitialTab] = useState<SettingsTab>('account')
  const [desktopSettingsSection, setDesktopSettingsSection] = useState<DesktopSettingsKey>('general')
  const [notificationsOpen, setNotificationsOpen] = useState(
    () => new URLSearchParams(location.search).has('notices'),
  )
  const [notificationVersion, setNotificationVersion] = useState(0)
  const [appMode, setAppModeState] = useState<AppMode>(readAppModeFromStorage)
  const [pendingSkillPrompt, setPendingSkillPrompt] = useState<string | null>(null)

  const settingsOpenTraceRef = useRef<ReturnType<typeof beginPerfTrace>>(null)
  const titleBarIncognitoRef = useRef<(() => void) | null>(null)

  const availableAppModes: AppMode[] = useMemo(
    () => (desktop || me?.work_enabled !== false) ? ['chat', 'work'] : ['chat'],
    [desktop, me?.work_enabled],
  )

  // -- helpers --

  const replaceQueryState = useCallback((params: URLSearchParams) => {
    const qs = params.toString()
    const basePath = window.location.pathname
    const hash = usesHashRouting ? window.location.hash : ''
    const next = `${basePath}${qs ? `?${qs}` : ''}${hash}`
    window.history.replaceState(window.history.state, '', next)
  }, [usesHashRouting])

  const pushSearchModeState = useCallback(() => {
    const basePath = window.location.pathname
    const next = usesHashRouting ? `${basePath}${window.location.search}${window.location.hash}` : '/'
    window.history.pushState({ searchMode: true }, '', next)
  }, [usesHashRouting])

  // -- actions --

  const toggleSidebar = useCallback(() => {
    setSidebarCollapsed((v) => !v)
  }, [])

  const enterSearchMode = useCallback(() => {
    pushSearchModeState()
    setIsSearchMode(true)
  }, [pushSearchModeState])

  const exitSearchMode = useCallback(() => {
    setIsSearchMode(false)
  }, [])

  const openSettings = useCallback((tab: SettingsTab | 'voice' = 'account') => {
    if (desktop) {
      const keyMap: Record<string, DesktopSettingsKey> = {
        account: 'general',
        settings: 'general',
        skills: 'skills',
        models: 'providers',
        agents: 'personas',
        channels: 'channels',
        connection: 'advanced',
        voice: 'advanced',
      }
      const section = keyMap[tab] ?? 'general'
      recordPerfDuration('desktop_settings_open_request', 0, {
        source: 'sidebar',
        requestedTab: tab,
        section,
        pathname: location.pathname,
      })
      settingsOpenTraceRef.current = beginPerfTrace('desktop_settings_open', {
        source: 'sidebar',
        requestedTab: tab,
        section,
        pathname: location.pathname,
      })
      setDesktopSettingsSection(section)
      setSettingsOpen(true)
      return
    }
    setSettingsInitialTab(tab as SettingsTab)
    setSettingsOpen(true)
  }, [desktop, location.pathname])

  const closeSettings = useCallback(() => {
    setSettingsOpen(false)
  }, [])

  const openNotifications = useCallback(() => {
    setNotificationsOpen(true)
    const params = new URLSearchParams(window.location.search)
    if (!params.has('notices')) {
      params.set('notices', '')
      replaceQueryState(params)
    }
  }, [replaceQueryState])

  const closeNotifications = useCallback(() => {
    setNotificationsOpen(false)
    const params = new URLSearchParams(window.location.search)
    if (params.has('notices')) {
      params.delete('notices')
      replaceQueryState(params)
    }
  }, [replaceQueryState])

  const markNotificationRead = useCallback(() => {
    setNotificationVersion((v) => v + 1)
  }, [])

  const handleSetAppMode = useCallback((mode: AppMode) => {
    writeAppModeToStorage(mode)
    setAppModeState(mode)
    if (/^\/t\//.test(location.pathname)) {
      navigate('/')
    }
  }, [location.pathname, navigate])

  const queueSkillPrompt = useCallback((prompt: string) => {
    setPendingSkillPrompt(prompt)
  }, [])

  const consumeSkillPrompt = useCallback(() => {
    setPendingSkillPrompt(null)
  }, [])

  const setTitleBarIncognitoClick = useCallback((fn: (() => void) | null) => {
    titleBarIncognitoRef.current = fn
  }, [])

  const triggerTitleBarIncognitoClick = useCallback((fallback: () => void) => {
    const fn = titleBarIncognitoRef.current
    if (fn) fn()
    else fallback()
  }, [])

  // -- effects --

  // 同步 ref，使 popstate 回调始终拿到最新值
  useEffect(() => { isSearchModeRef.current = isSearchMode }, [isSearchMode])

  // window resize：折叠/隐藏侧边栏
  useEffect(() => {
    let raf = 0
    const handler = () => {
      cancelAnimationFrame(raf)
      raf = requestAnimationFrame(() => {
        const w = window.innerWidth
        const hidden = w < 900
        setSidebarHiddenByWidth((prev) => (prev === hidden ? prev : hidden))
        const narrow = w < 1200
        if (narrow && !collapsedByWidthRef.current) {
          collapsedByWidthRef.current = true
          setSidebarCollapsed(true)
        } else if (!narrow && collapsedByWidthRef.current) {
          collapsedByWidthRef.current = false
          setSidebarCollapsed(false)
        }
      })
    }
    window.addEventListener('resize', handler)
    return () => {
      window.removeEventListener('resize', handler)
      cancelAnimationFrame(raf)
    }
  }, [])

  // 离开首页时退出搜索模式
  useEffect(() => {
    if (location.pathname === '/') return
    const id = requestAnimationFrame(() => setIsSearchMode(false))
    return () => cancelAnimationFrame(id)
  }, [location.pathname])

  // 路由切换时重置右侧面板
  useEffect(() => {
    const id = requestAnimationFrame(() => {
      setRightPanelOpen(false)
      if (notificationsOpen) closeNotifications()
    })
    return () => cancelAnimationFrame(id)
  }, [location.pathname]) // eslint-disable-line react-hooks/exhaustive-deps

  // Desktop：导航到会话时关闭设置
  useEffect(() => {
    if (!(desktop && settingsOpen && /^\/t\//.test(location.pathname))) return
    const id = requestAnimationFrame(() => setSettingsOpen(false))
    return () => cancelAnimationFrame(id)
  }, [location.pathname]) // eslint-disable-line react-hooks/exhaustive-deps

  // Desktop：监听外部 open-settings 事件
  useEffect(() => {
    if (!desktop) return
    const handler = () => {
      settingsOpenTraceRef.current = beginPerfTrace('desktop_settings_open', {
        source: 'window-event',
        requestedSection: 'general',
        pathname: location.pathname,
      })
      setDesktopSettingsSection('general')
      setSettingsOpen(true)
    }
    window.addEventListener('arkloop:app:open-settings', handler as EventListener)
    return () => window.removeEventListener('arkloop:app:open-settings', handler as EventListener)
  }, [desktop, location.pathname])

  // Desktop：settings 可见时结束 perf trace
  useEffect(() => {
    if (!(desktop && settingsOpen)) return
    endPerfTrace(settingsOpenTraceRef.current, {
      phase: 'visible',
      section: desktopSettingsSection,
      pathname: location.pathname,
    })
    settingsOpenTraceRef.current = null
  }, [desktop, settingsOpen, desktopSettingsSection, location.pathname])

  // popstate：退出搜索模式
  useEffect(() => {
    const onPopState = () => {
      if (isSearchModeRef.current) setIsSearchMode(false)
    }
    window.addEventListener('popstate', onPopState)
    return () => window.removeEventListener('popstate', onPopState)
  }, [])

  const value = useMemo<AppUIContextValue>(() => ({
    sidebarCollapsed,
    sidebarHiddenByWidth,
    rightPanelOpen,
    isSearchMode,
    settingsOpen,
    settingsInitialTab,
    desktopSettingsSection,
    notificationsOpen,
    notificationVersion,
    appMode,
    availableAppModes,
    pendingSkillPrompt,
    toggleSidebar,
    setRightPanelOpen,
    enterSearchMode,
    exitSearchMode,
    openSettings,
    closeSettings,
    openNotifications,
    closeNotifications,
    markNotificationRead,
    setAppMode: handleSetAppMode,
    queueSkillPrompt,
    consumeSkillPrompt,
    setTitleBarIncognitoClick,
    triggerTitleBarIncognitoClick,
  }), [
    sidebarCollapsed,
    sidebarHiddenByWidth,
    rightPanelOpen,
    isSearchMode,
    settingsOpen,
    settingsInitialTab,
    desktopSettingsSection,
    notificationsOpen,
    notificationVersion,
    appMode,
    availableAppModes,
    pendingSkillPrompt,
    toggleSidebar,
    setRightPanelOpen,
    enterSearchMode,
    exitSearchMode,
    openSettings,
    closeSettings,
    openNotifications,
    closeNotifications,
    markNotificationRead,
    handleSetAppMode,
    queueSkillPrompt,
    consumeSkillPrompt,
    setTitleBarIncognitoClick,
    triggerTitleBarIncognitoClick,
  ])

  return (
    <AppUIContext.Provider value={value}>
      {children}
    </AppUIContext.Provider>
  )
}

export function AppUIContextBridge({
  value,
  children,
}: {
  value: AppUIContextValue
  children: ReactNode
}) {
  return (
    <AppUIContext.Provider value={value}>
      {children}
    </AppUIContext.Provider>
  )
}

export function useAppUI(): AppUIContextValue {
  const ctx = useContext(AppUIContext)
  if (!ctx) throw new Error('useAppUI must be used within AppUIProvider')
  return ctx
}
