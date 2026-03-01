import { useState, useEffect, useCallback } from 'react'
import { useOutletContext } from 'react-router-dom'
import type { ConsoleOutletContext } from '../../layouts/ConsoleLayout'
import { PageHeader } from '../../components/PageHeader'
import { FormField } from '../../components/FormField'
import { useToast } from '../../components/useToast'
import { isApiError } from '../../api'
import { useLocale } from '../../contexts/LocaleContext'
import { listAgentConfigs, type AgentConfig } from '../../api/agent-configs'
import { getPlatformSetting, setPlatformSetting, deletePlatformSetting } from '../../api/platform-settings'

const SETTING_KEY = 'title_summarizer.agent_config_id'

export function TitleSummarizerPage() {
  const { accessToken } = useOutletContext<ConsoleOutletContext>()
  const { addToast } = useToast()
  const { t } = useLocale()
  const tc = t.pages.titleSummarizer

  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [agentConfigs, setAgentConfigs] = useState<AgentConfig[]>([])
  const [selectedAgentId, setSelectedAgentId] = useState<string>('')

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [configs, setting] = await Promise.all([
        listAgentConfigs(accessToken),
        getPlatformSetting(SETTING_KEY, accessToken).catch(() => null),
      ])
      setAgentConfigs(configs)
      setSelectedAgentId(setting?.value ?? '')
    } catch {
      addToast(tc.toastLoadFailed, 'error')
    } finally {
      setLoading(false)
    }
  }, [accessToken, addToast, tc.toastLoadFailed])

  useEffect(() => {
    void load()
  }, [load])

  const handleSave = useCallback(async () => {
    setSaving(true)
    try {
      if (selectedAgentId) {
        await setPlatformSetting(SETTING_KEY, selectedAgentId, accessToken)
      } else {
        await deletePlatformSetting(SETTING_KEY, accessToken).catch(() => {})
      }
      addToast(tc.toastSaved, 'success')
    } catch (err) {
      addToast(isApiError(err) ? err.message : tc.toastSaveFailed, 'error')
    } finally {
      setSaving(false)
    }
  }, [selectedAgentId, accessToken, addToast, tc])

  const inputCls =
    'w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-bg-deep2)] px-3 py-1.5 text-sm text-[var(--c-text-primary)] focus:outline-none'

  const headerActions = (
    <button
      onClick={handleSave}
      disabled={saving || loading}
      className="flex items-center gap-1.5 rounded-lg bg-[var(--c-bg-tag)] px-3 py-1.5 text-xs font-medium text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)] disabled:opacity-50"
    >
      {saving ? '...' : tc.save}
    </button>
  )

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader title={tc.title} actions={headerActions} />

      <div className="flex flex-1 flex-col gap-6 overflow-auto p-6">
        {loading ? (
          <div className="flex items-center justify-center py-12">
            <span className="text-sm text-[var(--c-text-muted)]">...</span>
          </div>
        ) : (
          <FormField label={tc.fieldAgent}>
            <select
              value={selectedAgentId}
              onChange={(e) => setSelectedAgentId(e.target.value)}
              className={inputCls}
            >
              <option value="">{tc.agentNone}</option>
              {agentConfigs.map((ac) => (
                <option key={ac.id} value={ac.id}>
                  {ac.name}{ac.model ? ` (${ac.model})` : ''}
                </option>
              ))}
            </select>
            <p className="text-xs text-[var(--c-text-muted)]">{tc.fieldAgentHint}</p>
          </FormField>
        )}
      </div>
    </div>
  )
}
