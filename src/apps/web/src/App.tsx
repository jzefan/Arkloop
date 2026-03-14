import { useCallback, useEffect, useState } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import { AppLayout } from './layouts/AppLayout'
import { AuthPage } from './components/AuthPage'
import { WelcomePage } from './components/WelcomePage'
import { ChatPage } from './components/ChatPage'
import { SharePage } from './components/SharePage'
import { VerifyEmailPage } from './components/VerifyEmailPage'
import { OnboardingWizard } from './components/OnboardingWizard'
import {
  clearActiveThreadIdInStorage,
  writeAccessTokenToStorage,
  clearAccessTokenFromStorage,
} from './storage'
import { setUnauthenticatedHandler, setAccessTokenHandler, refreshAccessToken } from './api'
import { setClientApp } from '@arkloop/shared/api'
import { isLocalMode, isDesktop, getDesktopApi } from '@arkloop/shared/desktop'

function App() {
  const [accessToken, setAccessToken] = useState<string | null>(null)
  const [authChecked, setAuthChecked] = useState(false)
  const [onboardingDone, setOnboardingDone] = useState<boolean | null>(null)

  // Desktop: 检查 onboarding 状态
  useEffect(() => {
    if (!isDesktop()) {
      setOnboardingDone(true)
      return
    }
    const api = getDesktopApi()
    if (!api) {
      setOnboardingDone(true)
      return
    }
    api.onboarding.getStatus().then((s) => setOnboardingDone(s.completed)).catch(() => setOnboardingDone(true))
  }, [])

  useEffect(() => {
    const controller = new AbortController()

    setClientApp('web')
    setUnauthenticatedHandler(() => {
      clearAccessTokenFromStorage()
      clearActiveThreadIdInStorage()
      setAccessToken(null)
    })
    setAccessTokenHandler((token: string) => {
      writeAccessTokenToStorage(token)
      setAccessToken(token)
    })

    // Local 模式: Go 后端使用固定 token，跳过刷新流程
    if (isLocalMode()) {
      const desktopToken = 'desktop-local-token'
      writeAccessTokenToStorage(desktopToken)
      setAccessToken(desktopToken)
      setAuthChecked(true)
      return
    }

    const tryRefresh = (retries: number) => {
      refreshAccessToken(controller.signal)
        .then((resp) => {
          if (controller.signal.aborted) return
          writeAccessTokenToStorage(resp.access_token)
          setAccessToken(resp.access_token)
          setAuthChecked(true)
        })
        .catch((err) => {
          if (controller.signal.aborted) return
          const isNetwork = err instanceof TypeError || (err && typeof err === 'object' && 'code' in err)
          if (isNetwork && retries > 0) {
            setTimeout(() => tryRefresh(retries - 1), 2000)
            return
          }
          setAuthChecked(true)
        })
    }
    tryRefresh(3)

    return () => {
      controller.abort()
    }
  }, [])

  const handleLoggedIn = useCallback((token: string) => {
    clearActiveThreadIdInStorage()
    writeAccessTokenToStorage(token)
    setAccessToken(token)
    // accessToken 变化后路由树切换，/login 自动 redirect 到 /
  }, [])

  const handleLoggedOut = useCallback(() => {
    clearAccessTokenFromStorage()
    clearActiveThreadIdInStorage()
    setAccessToken(null)
  }, [])

  const handleOnboardingComplete = useCallback(() => {
    // config.mode 在 onboarding 中可能已变更，需要 reload 使 preload 重新注入 __ARKLOOP_DESKTOP__
    window.location.reload()
  }, [])

  if (onboardingDone === null) return null
  if (onboardingDone === false) return <OnboardingWizard onComplete={handleOnboardingComplete} />

  return (
    <Routes>
      <Route path="/verify" element={<VerifyEmailPage />} />
      <Route path="/s/:token" element={<SharePage />} />
      {!authChecked ? (
        <Route path="*" element={<div />} />
      ) : !accessToken ? (
        <>
          <Route path="/login" element={<AuthPage onLoggedIn={handleLoggedIn} />} />
          <Route path="/register" element={<Navigate to="/login" replace />} />
          <Route path="*" element={<Navigate to="/login" replace />} />
        </>
      ) : (
        <>
          <Route path="/login" element={<Navigate to="/" replace />} />
          <Route path="/register" element={<Navigate to="/" replace />} />
          <Route element={<AppLayout accessToken={accessToken} onLoggedOut={handleLoggedOut} />}>
            <Route index element={<WelcomePage />} />
            <Route path="search" element={<WelcomePage />} />
            <Route path="t/:threadId" element={<ChatPage />} />
            <Route path="t/:threadId/search" element={<ChatPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Route>
        </>
      )}
    </Routes>
  )
}

export default App
