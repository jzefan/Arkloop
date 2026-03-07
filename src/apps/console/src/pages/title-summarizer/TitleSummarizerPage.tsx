import { useState, useEffect, useCallback, useMemo } from 'react'
import { useOutletContext } from 'react-router-dom'
import type { ConsoleOutletContext } from '../../layouts/ConsoleLayout'
import { PageHeader } from '../../components/PageHeader'
import { FormField } from '../../components/FormField'
import { useToast } from '../../components/useToast'
import { isApiError } from '../../api'
import { useLocale } from '../../contexts/LocaleContext'
import { listLlmProviders } from '../../api/llm-providers'
import { getPlatformSetting, setPlatformSetting, deletePlatformSetting } from '../../api/platform-settings'

const SETTING_KEY = 'title_summarizer.model'

type SelectorOption = {
  value: string
  label: string
}

export function TitleSummarizerPage() {
  const { accessToken } = useOutletContext<ConsoleOutletContext>()
  const { addToast } = useToast()
  const { t } = useLocale()
  const tc = t.pages.titleSummarizer

  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [selectedModel, setSelectedModel] = useState('')
  const [options, setOptions] = useState<SelectorOption[]>([])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [providers, setting] = await Promise.all([
        listLlmProviders(accessToken),
        getPlatformSetting(SETTING_KEY, accessToken).catch(() => null),
      ])

      const nextOptions = providers.flatMap((provider) =>
        (provider.models ?? []).map((model) => ({
          value: `${provider.name}^${model.model}`,
          label: `${provider.name} · ${model.model}`,
        })),
      )

      setOptions(nextOptions)
      setSelectedModel(setting?.value ?? '')
    } catch {
      addToast(tc.toastLoadFailed, 'error')
    } finally {
      setLoading(false)
    }
  }, [accessToken, addToast, tc.toastLoadFailed])

  useEffect(() => {
    void load()
  }, [load])

  const mergedOptions = useMemo(() => {
    if (!selectedModel.trim() || options.some((item) => item.value === selectedModel)) {
      return options
    }
    return [{ value: selectedModel, label: selectedModel }, ...options]
  }, [options, selectedModel])

  const handleSave = useCallback(async () => {
    setSaving(true)
    try {
      if (selectedModel.trim()) {
        await setPlatformSetting(SETTING_KEY, selectedModel.trim(), accessToken)
      } else {
        await deletePlatformSetting(SETTING_KEY, accessToken).catch(() => {})
      }
      addToast(tc.toastSaved, 'success')
    } catch (err) {
      addToast(isApiError(err) ? err.message : tc.toastSaveFailed, 'error')
    } finally {
      setSaving(false)
    }
  }, [accessToken, addToast, selectedModel, tc])

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
          <FormField label={tc.fieldModel}>
            <select
              value={selectedModel}
              onChange={(e) => setSelectedModel(e.target.value)}
              className={inputCls}
            >
              <option value="">{tc.modelNone}</option>
              {mergedOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </FormField>
        )}
      </div>
    </div>
  )
}
