import { useCallback, useState } from 'react'
import { Routes, Route, Navigate, useNavigate } from 'react-router-dom'
import { AppLayout } from './layouts/AppLayout'
import { AuthPage } from './components/AuthPage'
import { WelcomePage } from './components/WelcomePage'
import { ChatPage } from './components/ChatPage'
import {
  clearActiveThreadIdInStorage,
  readAccessTokenFromStorage,
  writeAccessTokenToStorage,
  clearAccessTokenFromStorage,
} from './storage'

function App() {
  const [accessToken, setAccessToken] = useState<string | null>(() => readAccessTokenFromStorage())
  const navigate = useNavigate()

  const handleLoggedIn = useCallback((token: string) => {
    clearActiveThreadIdInStorage()
    writeAccessTokenToStorage(token)
    setAccessToken(token)
    navigate('/', { replace: true })
  }, [navigate])

  const handleLoggedOut = useCallback(() => {
    clearAccessTokenFromStorage()
    clearActiveThreadIdInStorage()
    setAccessToken(null)
  }, [])

  if (!accessToken) {
    return <AuthPage onLoggedIn={handleLoggedIn} />
  }

  return (
    <Routes>
      <Route
        element={<AppLayout accessToken={accessToken} onLoggedOut={handleLoggedOut} />}
      >
        <Route index element={<WelcomePage />} />
        <Route path="search" element={<WelcomePage />} />
        <Route path="t/:threadId" element={<ChatPage />} />
        <Route path="t/:threadId/search" element={<ChatPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}

export default App
