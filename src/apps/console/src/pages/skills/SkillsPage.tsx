import { useCallback, useEffect, useMemo, useState } from 'react'
import { useOutletContext } from 'react-router-dom'
import { Loader2, Save } from 'lucide-react'
import type { ConsoleOutletContext } from '../../layouts/ConsoleLayout'
import { PageHeader } from '../../components/PageHeader'
import { FormField } from '../../components/FormField'
import { useToast } from '../../components/useToast'
import { isApiError } from '../../api'
import { useLocale } from '../../contexts/LocaleContext'
import { deletePlatformSetting, getPlatformSetting, setPlatformSetting } from '../../api/platform-settings'

const SETTINGS = {
  apiKey: 'skills.market.skillsmp_api_key',
  baseURL: 'skills.market.skillsmp_base_url',
} as const

const DEFAULT_BASE_URL = 'https://skillsmp.com'

export function SkillsPage() {
  const { accessToken } = useOutletContext<ConsoleOutletContext>()
  const { addToast } = useToast()
  const { t } = useLocale()
  const tc = t.pages.skills

  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [apiKey, setApiKey] = useState('')
  const [baseURL, setBaseURL] = useState(DEFAULT_BASE_URL)
  const [savedApiKey, setSavedApiKey] = useState('')
  const [savedBaseURL, setSavedBaseURL] = useState(DEFAULT_BASE_URL)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [apiKeySetting, baseURLSetting] = await Promise.all([
        getPlatformSetting(SETTINGS.apiKey, accessToken).catch(() => null),
        getPlatformSetting(SETTINGS.baseURL, accessToken).catch(() => null),
      ])
      const nextApiKey = apiKeySetting?.value?.trim() ?? ''
      const nextBaseURL = baseURLSetting?.value?.trim() || DEFAULT_BASE_URL
      setApiKey(nextApiKey)
      setBaseURL(nextBaseURL)
      setSavedApiKey(nextApiKey)
      setSavedBaseURL(nextBaseURL)
    } catch (err) {
      addToast(isApiError(err) ? err.message : tc.toastLoadFailed, 'error')
    } finally {
      setLoading(false)
    }
  }, [accessToken, addToast, tc.toastLoadFailed])

  useEffect(() => {
    void load()
  }, [load])

  const dirty = useMemo(() => {
    return apiKey !== savedApiKey || baseURL !== savedBaseURL
  }, [apiKey, baseURL, savedApiKey, savedBaseURL])

  const handleSave = useCallback(async () => {
    setSaving(true)
    try {
      const ops: Promise<unknown>[] = [
        setPlatformSetting(SETTINGS.baseURL, baseURL.trim() || DEFAULT_BASE_URL, accessToken),
      ]
      if (apiKey.trim()) {
        ops.push(setPlatformSetting(SETTINGS.apiKey, apiKey.trim(), accessToken))
      } else if (savedApiKey) {
        ops.push(deletePlatformSetting(SETTINGS.apiKey, accessToken))
      }
      await Promise.all(ops)
      setSavedApiKey(apiKey.trim())
      setSavedBaseURL(baseURL.trim() || DEFAULT_BASE_URL)
      addToast(tc.toastSaved, 'success')
    } catch (err) {
      addToast(isApiError(err) ? err.message : tc.toastSaveFailed, 'error')
    } finally {
      setSaving(false)
    }
  }, [accessToken, addToast, apiKey, baseURL, savedApiKey, tc.toastSaved, tc.toastSaveFailed])

  const inputCls = 'w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-bg-deep2)] px-3 py-1.5 text-sm text-[var(--c-text-primary)] focus:outline-none'

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader
        title={tc.title}
        actions={(
          <button
            onClick={handleSave}
            disabled={saving || loading || !dirty}
            className="flex items-center gap-1.5 rounded-lg bg-[var(--c-bg-tag)] px-3 py-1.5 text-xs font-medium text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)] disabled:opacity-50"
          >
            {saving ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}
            {tc.save}
          </button>
        )}
      />

      <div className="flex flex-1 flex-col gap-6 overflow-auto p-6">
        {loading ? (
          <div className="flex items-center justify-center py-12">
            <Loader2 size={18} className="animate-spin text-[var(--c-text-muted)]" />
          </div>
        ) : (
          <div className="max-w-2xl space-y-5 rounded-xl border border-[var(--c-border-console)] bg-[var(--c-bg-card)] p-5">
            <div>
              <h3 className="text-sm font-medium text-[var(--c-text-primary)]">{tc.sectionMarket}</h3>
              <p className="mt-1 text-xs text-[var(--c-text-muted)]">{tc.hint}</p>
            </div>

            <FormField label={tc.fieldApiKey}>
              <input
                type="password"
                value={apiKey}
                onChange={(event) => setApiKey(event.target.value)}
                className={inputCls}
                placeholder="sk_live_..."
              />
            </FormField>

            <FormField label={tc.fieldBaseUrl}>
              <input
                type="text"
                value={baseURL}
                onChange={(event) => setBaseURL(event.target.value)}
                className={inputCls}
                placeholder={DEFAULT_BASE_URL}
              />
            </FormField>

            <p className="text-xs text-[var(--c-text-muted)]">{tc.fieldBaseUrlHint}</p>
          </div>
        )}
      </div>
    </div>
  )
}
