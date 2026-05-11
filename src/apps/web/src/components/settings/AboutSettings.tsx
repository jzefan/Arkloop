import { useEffect, useState } from 'react'
import { HardDrive } from 'lucide-react'
import {
  PRODUCT_BRAND_NAME,
  PRODUCT_DESKTOP_VERSION,
  PRODUCT_SOURCE_LABEL,
  PRODUCT_SOURCE_URL,
} from '@arkloop/shared'
import { getDesktopApi, type DesktopAdvancedOverview } from '@arkloop/shared/desktop'
import { useLocale } from '../../contexts/LocaleContext'
import { openExternal } from '../../openExternal'
import { readDeveloperMode, writeDeveloperMode } from '../../storage'
import { SettingsSection } from './_SettingsSection'
import { SettingsSectionHeader } from './_SettingsSectionHeader'
import { UpdateSettingsContent } from './UpdateSettings'
import { SettingsSwitch } from './_SettingsSwitch'

export function AboutSettings({ accessToken: _accessToken }: { accessToken: string }) {
  const { t } = useLocale()
  const ds = t.desktopSettings
  const api = getDesktopApi()
  const [devMode, setDevMode] = useState(() => readDeveloperMode())
  const [overview, setOverview] = useState<DesktopAdvancedOverview | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!api?.advanced) {
      setLoading(false)
      return
    }
    let active = true
    void api.advanced.getOverview()
      .then((data) => {
        if (active) setOverview(data)
      })
      .catch((err) => {
        if (active) setError(err instanceof Error ? err.message : t.requestFailed)
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [api, t.requestFailed])

  const appName = PRODUCT_BRAND_NAME
  const appVersion = PRODUCT_DESKTOP_VERSION
  const iconDataUrl = overview?.iconDataUrl ?? null

  return (
    <div className="flex flex-col gap-6">
      <SettingsSectionHeader title={ds.about} description={ds.aboutDesc} />

      <SettingsSection>
        <div className="flex flex-wrap items-start gap-4">
          <div
            className="flex h-14 w-14 items-center justify-center overflow-hidden rounded-2xl bg-[var(--c-bg-deep)]"
            style={{ border: '0.5px solid var(--c-border-subtle)' }}
          >
            {iconDataUrl ? (
              <img src={iconDataUrl} alt={appName} className="h-full w-full object-cover" />
            ) : (
              <HardDrive size={22} className="text-[var(--c-text-muted)]" />
            )}
          </div>
          <div className="min-w-[12rem] flex-1">
            <div className="text-lg font-semibold text-[var(--c-text-heading)]">{appName}</div>
            <div className="mt-0.5 text-sm text-[var(--c-text-secondary)]">
              {appVersion || (loading ? '...' : '')}
            </div>
          </div>
          <div className="flex basis-full items-center xl:ml-auto xl:basis-auto">
            <button
              type="button"
              onClick={() => openExternal(PRODUCT_SOURCE_URL)}
              className="text-sm font-medium text-[var(--c-accent)] underline-offset-2 transition-opacity hover:underline hover:opacity-80"
            >
              {PRODUCT_SOURCE_LABEL}
            </button>
          </div>
        </div>
      </SettingsSection>

      {error && (
        <SettingsSection>
          <p className="text-sm" style={{ color: 'var(--c-status-error)' }}>{error}</p>
        </SettingsSection>
      )}

      <SettingsSection>
        <UpdateSettingsContent />
      </SettingsSection>

      <SettingsSection>
        <div className="flex items-center justify-between">
          <div>
            <div className="text-sm font-medium text-[var(--c-text-primary)]">{ds.developerTitle}</div>
            <div className="text-xs text-[var(--c-text-muted)]">{ds.developerDesc}</div>
          </div>
          <SettingsSwitch
            checked={devMode}
            onChange={(next) => {
              setDevMode(next)
              writeDeveloperMode(next)
            }}
          />
        </div>
      </SettingsSection>
    </div>
  )
}
