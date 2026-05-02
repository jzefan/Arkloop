import { useMemo } from 'react'
import { silentRefresh } from '@arkloop/shared'
import { apiBaseUrl } from '@arkloop/shared/api'
import { isLocalMode } from '@arkloop/shared/desktop'
import { useAuth } from '../contexts/auth'
import { createArkloopAgentClient } from './arkloop-adapter'
import type { AgentClient } from './contract'

export function useAgentClient(): AgentClient {
  const { accessToken } = useAuth()
  const baseUrl = apiBaseUrl()

  return useMemo(() => createArkloopAgentClient({
    accessToken,
    baseUrl,
    refreshAccessToken: async () => {
      if (isLocalMode()) return accessToken
      return await silentRefresh()
    },
  }), [accessToken, baseUrl])
}
