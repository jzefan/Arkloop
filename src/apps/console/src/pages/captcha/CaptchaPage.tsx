import { useState, useEffect, useCallback } from 'react'
import { useOutletContext } from 'react-router-dom'
import { Loader2, Save } from 'lucide-react'
import type { ConsoleOutletContext } from '../../layouts/ConsoleLayout'
import { PageHeader } from '../../components/PageHeader'
import { Badge } from '../../components/Badge'
import { useToast } from '../../components/useToast'
import { isApiError } from '../../api'
import { useLocale } from '../../contexts/LocaleContext'
import { getPlatformSetting, setPlatformSetting, deletePlatformSetting } from '../../api/platform-settings'

const KEY_SITE_KEY    = 'turnstile.site_key'
const KEY_SECRET_KEY  = 'turnstile.secret_key'
const KEY_ALLOWED_HOST = 'turnstile.allowed_host'

export function CaptchaPage() {
  const { accessToken } = useOutletContext<ConsoleOutletContext>()
  const { addToast } = useToast()
  const { t } = useLocale()
  const tc = t.pages.captcha

  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)

  const [siteKey, setSiteKey] = useState('')
  const [secretKey, setSecretKey] = useState('')
  const [secretKeySet, setSecretKeySet] = useState(false)
  const [allowedHost, setAllowedHost] = useState('')

  const [savedSiteKey, setSavedSiteKey] = useState('')
  const [savedAllowedHost, setSavedAllowedHost] = useState('')

  // configured = site key is set (secret key never exposed to UI)
  const [configured, setConfigured] = useState(false)
  const [source, setSource] = useState<'db' | 'env' | 'none'>('none')

  const loadAll = useCallback(async () => {
    setLoading(true)
    try {
      const results = await Promise.allSettled([
        getPlatformSetting(KEY_SITE_KEY, accessToken),
        getPlatformSetting(KEY_SECRET_KEY, accessToken),
        getPlatformSetting(KEY_ALLOWED_HOST, accessToken),
      ])

      const sk = results[0].status === 'fulfilled' ? results[0].value.value : ''
      const secretSet = results[1].status === 'fulfilled' && results[1].value.value !== ''
      const ah = results[2].status === 'fulfilled' ? results[2].value.value : ''

      setSiteKey(sk)
      setSavedSiteKey(sk)
      setSecretKeySet(secretSet)
      setAllowedHost(ah)
      setSavedAllowedHost(ah)
      setSecretKey('')
      setConfigured(sk !== '' && secretSet)
      setSource(sk !== '' || secretSet ? 'db' : 'none')
    } catch (err) {
      addToast(isApiError(err) ? err.message : tc.toastLoadFailed, 'error')
    } finally {
      setLoading(false)
    }
  }, [accessToken, addToast, tc.toastLoadFailed])

  useEffect(() => { void loadAll() }, [loadAll])

  const isDirty =
    siteKey !== savedSiteKey ||
    secretKey !== '' ||
    allowedHost !== savedAllowedHost

  const handleSave = async () => {
    setSaving(true)
    try {
      const ops: Promise<unknown>[] = [
        setPlatformSetting(KEY_SITE_KEY, siteKey.trim(), accessToken),
      ]
      if (secretKey !== '') {
        ops.push(setPlatformSetting(KEY_SECRET_KEY, secretKey.trim(), accessToken))
      }
      ops.push(
        allowedHost.trim() === ''
          ? deletePlatformSetting(KEY_ALLOWED_HOST, accessToken).catch(() => {})
          : setPlatformSetting(KEY_ALLOWED_HOST, allowedHost.trim(), accessToken),
      )
      await Promise.all(ops)

      setSavedSiteKey(siteKey.trim())
      setSavedAllowedHost(allowedHost.trim())
      if (secretKey !== '') setSecretKeySet(true)
      setSecretKey('')
      setConfigured(siteKey.trim() !== '' && (secretKeySet || secretKey !== ''))
      setSource(siteKey.trim() !== '' ? 'db' : 'none')
      addToast(tc.toastSaved, 'success')
    } catch (err) {
      addToast(isApiError(err) ? err.message : tc.toastSaveFailed, 'error')
    } finally {
      setSaving(false)
    }
  }

  const inputClass =
    'w-full rounded-md border border-[var(--c-border-console)] bg-[var(--c-bg-input)] px-3 py-1.5 text-sm text-[var(--c-text-primary)] outline-none focus:border-[var(--c-border-focus)]'
  const labelClass = 'mb-1 block text-xs font-medium text-[var(--c-text-secondary)]'

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader title={tc.title} />
      <div className="flex-1 overflow-y-auto p-6">
        {loading ? (
          <div className="flex items-center justify-center py-16">
            <Loader2 size={20} className="animate-spin text-[var(--c-text-muted)]" />
          </div>
        ) : (
          <div className="mx-auto max-w-xl space-y-6">

            {/* Status */}
            <div className="rounded-lg border border-[var(--c-border-console)] bg-[var(--c-bg-card)] p-5">
              <div className="flex items-center justify-between">
                <h3 className="text-sm font-medium text-[var(--c-text-primary)]">
                  {tc.statusTitle}
                </h3>
                <div className="flex items-center gap-2">
                  {source !== 'none' && (
                    <Badge variant="neutral">
                      {source === 'db' ? tc.sourceDb : tc.sourceEnv}
                    </Badge>
                  )}
                  <Badge variant={configured ? 'success' : 'warning'}>
                    {configured ? tc.statusConfigured : tc.statusNotConfigured}
                  </Badge>
                </div>
              </div>
            </div>

            {/* Configuration */}
            <div className="rounded-lg border border-[var(--c-border-console)] bg-[var(--c-bg-card)] p-5">
              <h3 className="text-sm font-medium text-[var(--c-text-primary)]">
                {tc.configTitle}
              </h3>
              <div className="mt-4 space-y-4">
                <div>
                  <label className={labelClass}>{tc.fieldSiteKey}</label>
                  <input
                    type="text"
                    className={inputClass}
                    value={siteKey}
                    onChange={e => setSiteKey(e.target.value)}
                    placeholder="0x4AAAAAAA..."
                    autoComplete="off"
                  />
                </div>
                <div>
                  <label className={labelClass}>{tc.fieldSecretKey}</label>
                  <input
                    type="password"
                    className={inputClass}
                    value={secretKey}
                    onChange={e => setSecretKey(e.target.value)}
                    placeholder={secretKeySet ? tc.fieldSecretKeySet : tc.fieldSecretKeyPlaceholder}
                    autoComplete="new-password"
                  />
                </div>
                <div>
                  <label className={labelClass}>{tc.fieldAllowedHost}</label>
                  <input
                    type="text"
                    className={inputClass}
                    value={allowedHost}
                    onChange={e => setAllowedHost(e.target.value)}
                    placeholder={tc.fieldAllowedHostPlaceholder}
                    autoComplete="off"
                  />
                  <p className="mt-1 text-xs text-[var(--c-text-muted)]">
                    {tc.fieldAllowedHostHint}
                  </p>
                </div>
                <div className="border-t border-[var(--c-border-console)] pt-4">
                  <button
                    onClick={handleSave}
                    disabled={saving || !isDirty}
                    className="inline-flex items-center gap-1.5 rounded-md border border-[var(--c-border-console)] px-3 py-1.5 text-xs font-medium text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)] disabled:opacity-50"
                  >
                    {saving ? <Loader2 size={12} className="animate-spin" /> : <Save size={12} />}
                    {tc.save}
                  </button>
                </div>
              </div>
            </div>

          </div>
        )}
      </div>
    </div>
  )
}
