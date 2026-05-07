import { useState, useEffect } from 'react'
import { readDeveloperShowRunDetailButton, readDeveloperShowDebugPanel, type MessageAgentEvent } from '../storage'

export function useDevTools() {
  const [showRunDetailButton, setShowRunDetailButton] = useState(() => readDeveloperShowRunDetailButton())
  const [showDebugPanel, setShowDebugPanel] = useState(() => readDeveloperShowDebugPanel())
  const [runDetailPanelRunId, setRunDetailPanelRunId] = useState<string | null>(null)
  const [messageAgentEventsMap, setMessageAgentEventsMap] = useState<Map<string, MessageAgentEvent[]>>(new Map())

  useEffect(() => {
    const handleChange = (e: Event) => {
      setShowRunDetailButton((e as CustomEvent<boolean>).detail)
    }
    window.addEventListener('arkloop:developer_show_run_detail_button', handleChange)
    return () => window.removeEventListener('arkloop:developer_show_run_detail_button', handleChange)
  }, [])

  useEffect(() => {
    const handleChange = (e: Event) => {
      setShowDebugPanel((e as CustomEvent<boolean>).detail)
    }
    window.addEventListener('arkloop:developer_show_debug_panel', handleChange)
    return () => window.removeEventListener('arkloop:developer_show_debug_panel', handleChange)
  }, [])

  return {
    showRunDetailButton,
    showDebugPanel,
    runDetailPanelRunId,
    setRunDetailPanelRunId,
    messageAgentEventsMap,
    setMessageAgentEventsMap,
  } as const
}
