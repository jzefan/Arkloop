import { useCallback, useEffect, useState } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import { AppLayout } from './layouts/AppLayout'
import { AuthPage } from './components/AuthPage'
import { WelcomePage } from './components/WelcomePage'
import { ChatPage } from './components/ChatPage'
import { SharePage } from './components/SharePage'
import { VerifyEmailPage } from './components/VerifyEmailPage'
import {
  clearActiveThreadIdInStorage,
  writeAccessTokenToStorage,
  clearAccessTokenFromStorage,
} from './storage'
import { setUnauthenticatedHandler, setAccessTokenHandler, refreshAccessToken } from './api'

function App() {
  const [accessToken, setAccessToken] = useState<string | null>(null)
  const [authChecked, setAuthChecked] = useState(false)

  useEffect(() => {
    setUnauthenticatedHandler(() => {
      clearAccessTokenFromStorage()
      clearActiveThreadIdInStorage()
      setAccessToken(null)
    })
    setAccessTokenHandler((token: string) => {
      writeAccessTokenToStorage(token)
      setAccessToken(token)
    })

    refreshAccessToken()
      .then((resp) => {
        writeAccessTokenToStorage(resp.access_token)
        setAccessToken(resp.access_token)
      })
      .catch(() => {})
      .finally(() => {
        setAuthChecked(true)
      })
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
