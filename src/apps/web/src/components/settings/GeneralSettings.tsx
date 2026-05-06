import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { Monitor, LogOut, HelpCircle, ArrowUpRight, Loader2, X, Pencil, Check, RefreshCw } from 'lucide-react'
import type { MeResponse } from '../../api'
import { updateMe } from '../../api'
import { useLocale } from '../../contexts/LocaleContext'
import { getDesktopApi, getDesktopAppVersion, isLocalMode, type AppUpdaterState } from '@arkloop/shared/desktop'
import { openExternal } from '../../openExternal'
import { LanguageContent } from './AppearanceSettings'
import { TimeZoneSettings } from './TimeZoneSettings'
import { SettingsButton, SettingsIconButton } from './_SettingsButton'
import { SettingsInput } from './_SettingsInput'

type Props = {
  me: MeResponse | null
  accessToken: string
  onLogout: () => void
  onMeUpdated?: (me: MeResponse) => void
}

function getAppUpdaterApi() {
  return getDesktopApi()?.appUpdater ?? null
}

function isAppUpdaterBusy(state: AppUpdaterState | null) {
  return state?.phase === 'checking' || state?.phase === 'downloading'
}

function GeneralSection({
  title,
  children,
}: {
  title: string
  children: ReactNode
}) {
  return (
    <section className="flex flex-col gap-2.5">
      <h3 className="pl-2.5 text-[13px] font-normal text-[var(--c-text-secondary)]">{title}</h3>
      {children}
    </section>
  )
}

function GeneralCard({ children }: { children: ReactNode }) {
  return (
    <div className="overflow-hidden rounded-xl border border-[var(--c-border-subtle)] bg-[var(--c-bg-menu)]">
      {children}
    </div>
  )
}

function GeneralRow({
  title,
  description,
  control,
}: {
  title: string
  description?: ReactNode
  control: ReactNode
}) {
  return (
    <div className="relative grid gap-3 px-5 py-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center sm:gap-6 [&+&]:before:absolute [&+&]:before:left-5 [&+&]:before:right-5 [&+&]:before:top-0 [&+&]:before:h-px [&+&]:before:bg-[var(--c-border-subtle)] [&+&]:before:content-['']">
      <div className="min-w-0">
        <div className="text-[13px] font-medium text-[var(--c-text-primary)]">{title}</div>
        {description && (
          <div className="mt-1 text-xs leading-5 text-[var(--c-text-tertiary)]">{description}</div>
        )}
      </div>
      <div className="min-w-0 sm:justify-self-end">{control}</div>
    </div>
  )
}

export function GeneralSettings({ me, accessToken, onLogout, onMeUpdated }: Props) {
  const { t, locale, setLocale } = useLocale()
  const ds = t.desktopSettings
  const docsUrl = locale === 'en' ? 'https://arkloop.io/en/docs/guide' : 'https://arkloop.io/zh/docs/guide'
  const localMode = isLocalMode()
  const [editingName, setEditingName] = useState(false)
  const [draftName, setDraftName] = useState(me?.username ?? '')
  const [savingName, setSavingName] = useState(false)
  const [nameError, setNameError] = useState('')
  const [appUpdateState, setAppUpdateState] = useState<AppUpdaterState | null>(null)
  const [checkingUpdate, setCheckingUpdate] = useState(false)
  const [updateError, setUpdateError] = useState('')
  const nameInputRef = useRef<HTMLInputElement>(null)
  const localVersion = getDesktopAppVersion() ?? ''

  useEffect(() => {
    if (!editingName) setDraftName(me?.username ?? '')
  }, [editingName, me?.username])

  useEffect(() => {
    if (!editingName) return
    const frame = requestAnimationFrame(() => {
      nameInputRef.current?.focus()
      nameInputRef.current?.select()
    })
    return () => cancelAnimationFrame(frame)
  }, [editingName])

  useEffect(() => {
    const api = getAppUpdaterApi()
    if (!api) return
    let active = true
    void api.getState().then((state) => {
      if (active) setAppUpdateState(state)
    }).catch(() => {})
    const unsub = api.onState((state) => {
      setAppUpdateState(state)
    })
    return () => {
      active = false
      unsub()
    }
  }, [])

  const displayName = me?.username ?? '?'
  const userInitial = displayName.charAt(0).toUpperCase()
  const trimmedDraftName = draftName.trim()
  const nameSaveDisabled = savingName || !trimmedDraftName

  const startNameEdit = () => {
    if (!me || savingName) return
    setDraftName(me.username)
    setNameError('')
    setEditingName(true)
  }

  const cancelNameEdit = () => {
    if (savingName) return
    setDraftName(me?.username ?? '')
    setNameError('')
    setEditingName(false)
  }

  const saveNameEdit = async () => {
    if (!me || nameSaveDisabled) return
    if (trimmedDraftName === me.username) {
      cancelNameEdit()
      return
    }
    setSavingName(true)
    setNameError('')
    try {
      const updated = await updateMe(accessToken, { username: trimmedDraftName })
      onMeUpdated?.({
        ...me,
        username: updated.username,
        timezone: updated.timezone === undefined ? me.timezone : updated.timezone,
      })
      setEditingName(false)
    } catch {
      setNameError(t.requestFailed)
    } finally {
      setSavingName(false)
    }
  }

  const checkUpdates = useCallback(async () => {
    const api = getAppUpdaterApi()
    if (!api) return
    setCheckingUpdate(true)
    setUpdateError('')
    try {
      const state = await api.check()
      setAppUpdateState(state)
    } catch (e) {
      setUpdateError(e instanceof Error ? e.message : t.requestFailed)
    } finally {
      setCheckingUpdate(false)
    }
  }, [t.requestFailed])

  const handleDownloadApp = useCallback(async () => {
    const api = getAppUpdaterApi()
    if (!api) return
    setUpdateError('')
    try {
      const state = await api.download()
      setAppUpdateState(state)
    } catch (e) {
      setUpdateError(e instanceof Error ? e.message : t.requestFailed)
    }
  }, [t.requestFailed])

  const handleInstallApp = useCallback(async () => {
    const api = getAppUpdaterApi()
    if (!api) return
    setUpdateError('')
    try {
      await api.install()
    } catch (e) {
      setUpdateError(e instanceof Error ? e.message : t.requestFailed)
    }
  }, [t.requestFailed])

  const appVersion = appUpdateState?.currentVersion ?? localVersion
  const appBusy = checkingUpdate || isAppUpdaterBusy(appUpdateState)
  const updateStateText = (() => {
    if (!appUpdateState) return ''
    switch (appUpdateState.phase) {
      case 'checking':
        return ds.appUpdateChecking
      case 'available':
        return appUpdateState.latestVersion ? `${ds.appUpdateAvailable} · ${appUpdateState.latestVersion}` : ds.appUpdateAvailable
      case 'not-available':
        return ds.appUpdateLatest
      case 'downloading':
        return `${ds.appUpdateDownloading} ${appUpdateState.progressPercent}%`
      case 'downloaded':
        return ds.appUpdateReady
      case 'error':
        return appUpdateState.error ?? ds.appUpdateError
      default:
        return ''
    }
  })()

  const updateControl = (() => {
    if (appBusy) {
      return (
        <SettingsButton disabled icon={<Loader2 size={14} className="animate-spin" />} className="min-w-[132px]">
          {updateStateText || ds.appUpdateChecking}
        </SettingsButton>
      )
    }
    if (appUpdateState?.phase === 'available') {
      return (
        <SettingsButton variant="primary" onClick={handleDownloadApp}>
          {ds.appUpdateDownload}
        </SettingsButton>
      )
    }
    if (appUpdateState?.phase === 'downloaded') {
      return (
        <SettingsButton variant="primary" onClick={handleInstallApp}>
          {ds.appUpdateInstall}
        </SettingsButton>
      )
    }
    return (
      <SettingsButton
        onClick={() => void checkUpdates()}
        disabled={!getAppUpdaterApi()}
        icon={<RefreshCw size={14} />}
      >
        {ds.checkForUpdates}
      </SettingsButton>
    )
  })()

  return (
    <div className="mx-auto flex w-full max-w-[740px] flex-col gap-7 pb-8">
      <div>
        <h2 className="pl-1.5 text-[22px] font-semibold tracking-normal text-[var(--c-text-heading)]">
          {ds.general}
        </h2>
      </div>

      <GeneralSection title={ds.profileSection}>
        <GeneralCard>
          <div className="group flex items-center gap-4 px-5 py-4">
            <div
              className="flex h-12 w-12 shrink-0 items-center justify-center rounded-full text-base font-semibold"
              style={{ background: 'var(--c-avatar-bg)', color: 'var(--c-avatar-text)' }}
            >
              {userInitial}
            </div>
            <div className="flex min-w-0 flex-1 flex-col">
              {editingName ? (
                <div className="mb-1 flex min-w-0 flex-col gap-1.5">
                  <div className="flex min-w-0 items-center gap-1.5">
                    <SettingsInput
                      ref={nameInputRef}
                      type="text"
                      value={draftName}
                      onChange={(e) => {
                        setDraftName(e.target.value)
                        setNameError('')
                      }}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') void saveNameEdit()
                        if (e.key === 'Escape') cancelNameEdit()
                      }}
                      disabled={savingName}
                      maxLength={256}
                      className="min-w-0 max-w-[280px] flex-1"
                    />
                    <SettingsIconButton
                      label={t.profileSave}
                      onClick={() => void saveNameEdit()}
                      disabled={nameSaveDisabled}
                    >
                      {savingName ? <Loader2 size={14} className="animate-spin" /> : <Check size={14} />}
                    </SettingsIconButton>
                    <SettingsIconButton
                      label={t.models.cancel}
                      onClick={cancelNameEdit}
                      disabled={savingName}
                    >
                      <X size={14} />
                    </SettingsIconButton>
                  </div>
                  {nameError && (
                    <span className="text-xs text-[var(--c-status-error-text)]">{nameError}</span>
                  )}
                </div>
              ) : (
                <div className="flex min-w-0 items-center gap-1.5">
                  <span className="truncate text-base font-semibold text-[var(--c-text-heading)]">
                    {displayName === '?' ? t.loading : displayName}
                  </span>
                  {localMode && me && (
                    <button
                      type="button"
                      onClick={startNameEdit}
                      className="pointer-events-none flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-[var(--c-text-tertiary)] opacity-0 transition-[opacity,background-color,color] hover:bg-[var(--c-bg-deep)] hover:text-[var(--c-text-secondary)] group-hover:pointer-events-auto group-hover:opacity-100 focus-visible:pointer-events-auto focus-visible:opacity-100"
                      title={t.editProfile}
                      aria-label={t.editProfile}
                    >
                      <Pencil size={14} />
                    </button>
                  )}
                </div>
              )}
              {localMode ? (
                <span className="flex items-center gap-1 text-xs text-[var(--c-text-tertiary)]">
                  <Monitor size={11} />
                  {ds.localModeLabel ?? 'Local'}
                </span>
              ) : me?.email ? (
                <span className="truncate text-xs text-[var(--c-text-tertiary)]">{me.email}</span>
              ) : null}
            </div>
          </div>
        </GeneralCard>
      </GeneralSection>

      <GeneralSection title={ds.languageRegionSection}>
        <GeneralCard>
          <GeneralRow
            title={t.language}
            control={(
              <LanguageContent
                locale={locale}
                setLocale={setLocale}
                label={t.language}
                showLabel={false}
                triggerClassName="h-9"
              />
            )}
          />
          <GeneralRow
            title={t.timezone}
            control={(
              <TimeZoneSettings
                me={me}
                accessToken={accessToken}
                onMeUpdated={onMeUpdated}
                showLabel={false}
              />
            )}
          />
        </GeneralCard>
      </GeneralSection>

      <GeneralSection title={ds.about}>
        <GeneralCard>
          <GeneralRow
            title={ds.appUpdateVersion}
            description={updateError || updateStateText || undefined}
            control={(
              <span className="flex h-[32px] max-w-[140px] items-center justify-end truncate rounded-[6.5px] bg-[var(--c-bg-input)] px-2.5 text-sm font-[450] tabular-nums text-[var(--c-text-primary)]">
                {appVersion || '-'}
              </span>
            )}
          />
          <GeneralRow
            title={ds.checkForUpdates}
            control={updateControl}
          />
        </GeneralCard>
      </GeneralSection>

      <GeneralSection title={ds.supportSection}>
        <GeneralCard>
          <GeneralRow
            title={t.getHelp}
            control={(
              <SettingsButton
                type="button"
                onClick={() => openExternal(docsUrl)}
                icon={<HelpCircle size={14} />}
              >
                <span className="inline-flex items-center gap-1">
                  {t.getHelp}
                  <ArrowUpRight size={11} />
                </span>
              </SettingsButton>
            )}
          />
          {!isLocalMode() && (
            <GeneralRow
              title={t.logout}
              control={(
                <SettingsButton
                  variant="danger"
                  onClick={onLogout}
                  icon={<LogOut size={14} />}
                >
                  {t.logout}
                </SettingsButton>
              )}
            />
          )}
        </GeneralCard>
      </GeneralSection>
    </div>
  )
}
