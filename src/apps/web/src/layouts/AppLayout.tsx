import { memo, useCallback } from 'react'
import { Outlet, useNavigate, useLocation } from 'react-router-dom'
import { isDesktop } from '@arkloop/shared/desktop'
import { LoadingPage } from '@arkloop/shared'
import { Sidebar } from '../components/Sidebar'
import { DesktopTitleBar } from '../components/DesktopTitleBar'
import { SettingsModal } from '../components/SettingsModal'
import { DesktopSettings } from '../components/DesktopSettings'
import { ChatsSearchModal } from '../components/ChatsSearchModal'
import { NotificationsPanel } from '../components/NotificationsPanel'
import { EmailVerificationGate } from '../components/EmailVerificationGate'
import { useLocale } from '../contexts/LocaleContext'
import { getMe } from '../api'
import { writeSelectedPersonaKeyToStorage, DEFAULT_PERSONA_KEY } from '../storage'
import { useAuth } from '../contexts/auth'
import { useThreadList } from '../contexts/thread-list'
import { useAppUI } from '../contexts/app-ui'
import { useCredits } from '../contexts/credits'

type LayoutMainProps = {
  desktop: boolean
  isSearchOpen: boolean
  filteredThreads: import('../api').ThreadResponse[]
  onSearchClose: () => void
  onMeUpdated: (m: import('../api').MeResponse) => void
  onTrySkill: (prompt: string) => void
}

const LayoutMain = memo(function LayoutMain({
  desktop,
  isSearchOpen,
  filteredThreads,
  onSearchClose,
  onMeUpdated,
  onTrySkill,
}: LayoutMainProps) {
  const { me, accessToken, logout } = useAuth()
  const { setCreditsBalance } = useCredits()
  const {
    settingsOpen, settingsInitialTab, desktopSettingsSection,
    closeSettings, notificationsOpen, closeNotifications, markNotificationRead,
  } = useAppUI()

  return (
    <>
      {settingsOpen && !desktop && (
        <SettingsModal
          me={me}
          accessToken={accessToken}
          initialTab={settingsInitialTab}
          onClose={closeSettings}
          onLogout={logout}
          onCreditsChanged={setCreditsBalance}
          onMeUpdated={onMeUpdated}
          onTrySkill={onTrySkill}
        />
      )}

      {isSearchOpen && (
        <ChatsSearchModal threads={filteredThreads} accessToken={accessToken} onClose={onSearchClose} />
      )}

      {desktop && settingsOpen ? (
        <DesktopSettings
          me={me}
          accessToken={accessToken}
          initialSection={desktopSettingsSection}
          onClose={closeSettings}
          onLogout={logout}
          onMeUpdated={onMeUpdated}
          onTrySkill={onTrySkill}
        />
      ) : (
        <main className="relative flex min-w-0 flex-1 flex-col overflow-y-auto" style={{ scrollbarGutter: 'stable' }}>
          <Outlet />
          {notificationsOpen && (
            <NotificationsPanel accessToken={accessToken} onClose={closeNotifications} onMarkedRead={markNotificationRead} />
          )}
        </main>
      )}
    </>
  )
})

export function AppLayout() {
  const { me, meLoaded, accessToken, logout, updateMe } = useAuth()
  const {
    isPrivateMode, pendingIncognitoMode,
    privateThreadIds, removeThread,
    togglePrivateMode,
    getFilteredThreads,
  } = useThreadList()
  const {
    sidebarCollapsed, sidebarHiddenByWidth,
    isSearchMode, appMode, availableAppModes,
    toggleSidebar, closeSettings,
    exitSearchMode, closeNotifications,
    setAppMode, queueSkillPrompt, triggerTitleBarIncognitoClick,
  } = useAppUI()
  useCredits()
  const { t } = useLocale()
  const navigate = useNavigate()
  const location = useLocation()
  const desktop = isDesktop()

  const isSearchOpen = location.pathname.endsWith('/search')

  const handleDesktopTitleBarIncognitoClick = useCallback(() => {
    triggerTitleBarIncognitoClick(togglePrivateMode)
  }, [triggerTitleBarIncognitoClick, togglePrivateMode])

  const handleNewThread = useCallback(() => {
    if (isSearchMode) writeSelectedPersonaKeyToStorage(DEFAULT_PERSONA_KEY)
    exitSearchMode()
    closeNotifications()
    if (desktop) closeSettings()
    navigate('/')
  }, [isSearchMode, exitSearchMode, closeNotifications, desktop, closeSettings, navigate])

  const handleCloseSearch = useCallback(() => {
    const basePath = location.pathname.replace(/\/search$/, '') || '/'
    navigate(basePath)
  }, [location.pathname, navigate])

  const handleTrySkill = useCallback((prompt: string) => {
    closeSettings()
    navigate('/')
    queueSkillPrompt(prompt)
  }, [closeSettings, navigate, queueSkillPrompt])

  const handleThreadDeleted = useCallback((deletedId: string) => {
    removeThread(deletedId)
    if (location.pathname === `/t/${deletedId}` || location.pathname.startsWith(`/t/${deletedId}/`)) {
      navigate('/')
    }
  }, [removeThread, location.pathname, navigate])

  const handleBeforeNavigateToThread = useCallback(() => {
    closeSettings()
  }, [closeSettings])

  const filteredThreads = getFilteredThreads(appMode)

  if (!meLoaded) return <LoadingPage label={t.loading} />

  if (me !== null && !me.email_verified && me.email_verification_required && me.email) {
    return (
      <EmailVerificationGate
        accessToken={accessToken}
        email={me.email}
        onVerified={() => { getMe(accessToken).then(updateMe).catch(() => {}) }}
        onPollVerified={() => { getMe(accessToken).then(updateMe).catch(() => {}) }}
        onLogout={logout}
      />
    )
  }

  const currentThreadId = location.pathname.match(/^\/t\/([^/]+)/)?.[1] ?? null
  const titleBarIncognitoActive =
    isPrivateMode || pendingIncognitoMode ||
    (currentThreadId != null && privateThreadIds.has(currentThreadId))

  return (
    <div className="flex h-screen flex-col overflow-hidden bg-[var(--c-bg-page)]">
      {desktop && (
        <DesktopTitleBar
          sidebarCollapsed={sidebarCollapsed}
          onToggleSidebar={toggleSidebar}
          appMode={appMode}
          onSetAppMode={setAppMode}
          availableModes={availableAppModes}
          showIncognitoToggle={appMode !== 'work'}
          isPrivateMode={titleBarIncognitoActive}
          onTogglePrivateMode={handleDesktopTitleBarIncognitoClick}
        />
      )}

      <div className="flex min-h-0 flex-1">
        {!sidebarHiddenByWidth && (
          <Sidebar
            onNewThread={handleNewThread}
            onThreadDeleted={handleThreadDeleted}
            beforeNavigateToThread={handleBeforeNavigateToThread}
          />
        )}

        <LayoutMain
          desktop={desktop}
          isSearchOpen={isSearchOpen}
          filteredThreads={filteredThreads}
          onSearchClose={handleCloseSearch}
          onMeUpdated={updateMe}
          onTrySkill={handleTrySkill}
        />
      </div>
    </div>
  )
}
