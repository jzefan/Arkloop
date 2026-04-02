import { useState, useEffect, useCallback } from 'react'
import { FileText, RefreshCw, Settings, Database } from 'lucide-react'
import { PillToggle } from '@arkloop/shared'
import { SpinnerIcon } from '@arkloop/shared/components/auth-ui'
import { useLocale } from '../../contexts/LocaleContext'
import { getDesktopApi } from '@arkloop/shared/desktop'
import type { MemoryConfig } from '@arkloop/shared/desktop'
import { checkBridgeAvailable, bridgeClient, type ModuleStatus } from '../../api-bridge'
import { secondaryButtonSmCls, secondaryButtonBorderStyle } from '../buttonStyles'
import { SettingsSectionHeader } from './_SettingsSectionHeader'
import { MemoryConfigModal } from './MemoryConfigModal'

// ---------------------------------------------------------------------------
// Status dot — shows health on the provider card
// ---------------------------------------------------------------------------

type HealthStatus = 'ok' | 'warning' | 'error' | 'checking'

function statusDotColor(s: HealthStatus): string {
  switch (s) {
    case 'ok': return '#22c55e'
    case 'warning': return '#f59e0b'
    case 'error': return '#ef4444'
    default: return 'var(--c-text-muted)'
  }
}

// ---------------------------------------------------------------------------
// SnapshotView
// ---------------------------------------------------------------------------

function SnapshotView({ snapshot }: { snapshot: string }) {
  if (!snapshot) {
    return (
      <div
        className="flex flex-col items-center justify-center rounded-xl py-14"
        style={{ border: '1px solid var(--c-border-subtle)', background: 'var(--c-bg-menu)' }}
      >
        <FileText size={28} className="mb-3 text-[var(--c-text-muted)]" />
        <p className="text-sm text-[var(--c-text-muted)]">No memory snapshot available yet.</p>
      </div>
    )
  }
  return (
    <pre
      className="overflow-auto rounded-xl p-4 text-xs leading-relaxed text-[var(--c-text-secondary)] whitespace-pre-wrap"
      style={{ border: '1px solid var(--c-border-subtle)', background: 'var(--c-bg-menu)', maxHeight: 360 }}
    >
      {snapshot}
    </pre>
  )
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

type Props = { accessToken?: string }

export function MemorySettings({ accessToken }: Props) {
  const { t } = useLocale()
  const ds = t.desktopSettings

  const [memConfig, setMemConfigState] = useState<MemoryConfig | null>(null)
  const [snapshot, setSnapshot] = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [configModalOpen, setConfigModalOpen] = useState(false)

  // Runtime health probe (lightweight — no full Bridge UI, just status)
  const [health, setHealth] = useState<HealthStatus>('checking')
  const [healthLabel, setHealthLabel] = useState('')

  const api = getDesktopApi()

  const probeHealth = useCallback(async (cfg: MemoryConfig | null) => {
    const isConfigured = Boolean(cfg?.openviking?.vlmModel && cfg?.openviking?.embeddingModel)
    if (!isConfigured) {
      setHealth('error')
      setHealthLabel(ds.memoryNotConfiguredHint)
      return
    }
    try {
      const online = await checkBridgeAvailable()
      if (!online) {
        setHealth('error')
        setHealthLabel('Bridge Offline')
        return
      }
      const list = await bridgeClient.listModules()
      const ov = list.find((m) => m.id === 'openviking')
      if (!ov) {
        setHealth('warning')
        setHealthLabel(ds.memoryModuleNotInstalled)
        return
      }
      const bad: ModuleStatus[] = ['error', 'stopped', 'installed_disconnected']
      if (bad.includes(ov.status)) {
        setHealth(ov.status === 'error' ? 'error' : 'warning')
        switch (ov.status) {
          case 'error': setHealthLabel(ds.memoryModuleError); break
          case 'stopped': setHealthLabel(ds.memoryModuleStopped); break
          case 'installed_disconnected': setHealthLabel(ds.memoryModuleDisconnected); break
        }
        return
      }
      if (ov.status === 'running') {
        setHealth('ok')
        setHealthLabel(ds.memoryModuleRunning)
        return
      }
      setHealth('checking')
      setHealthLabel(ds.memoryModuleChecking)
    } catch {
      setHealth('error')
      setHealthLabel('Bridge Offline')
    }
  }, [ds])

  const loadData = useCallback(async (quiet = false) => {
    if (!api?.memory) { setLoading(false); return }
    if (!quiet) setLoading(true); else setRefreshing(true)
    try {
      const cfg = await api.memory.getConfig()
      setMemConfigState(cfg)
      void probeHealth(cfg)
      if (cfg.enabled) {
        const snap = await api.memory.getSnapshot()
        setSnapshot(snap.memory_block ?? '')
      }
    } catch { /* ignore */ } finally {
      setLoading(false); setRefreshing(false)
    }
  }, [api, probeHealth])

  useEffect(() => { void loadData() }, [loadData])

  const saveConfig = useCallback(async (next: MemoryConfig) => {
    if (!api?.memory) return
    await api.memory.setConfig(next)
    setMemConfigState(next)
  }, [api])

  // ---------------------------------------------------------------------------

  if (loading) {
    return (
      <div className="flex flex-col gap-4">
        <SettingsSectionHeader title={ds.memorySettingsTitle} description={ds.memorySettingsDesc} />
        <div className="flex items-center justify-center py-20"><SpinnerIcon /></div>
      </div>
    )
  }

  if (!api?.memory) {
    return (
      <div className="flex flex-col gap-4">
        <SettingsSectionHeader title={ds.memorySettingsTitle} description={ds.memorySettingsDesc} />
        <div
          className="rounded-xl bg-[var(--c-bg-menu)] py-16 text-center text-sm text-[var(--c-text-muted)]"
          style={{ border: '0.5px solid var(--c-border-subtle)' }}
        >
          Not available outside Desktop mode.
        </div>
      </div>
    )
  }

  const enabled = memConfig?.enabled ?? true
  const isConfigured = Boolean(memConfig?.openviking?.vlmModel && memConfig?.openviking?.embeddingModel)

  return (
    <div className="flex flex-col gap-6">
      <SettingsSectionHeader title={ds.memorySettingsTitle} description={ds.memorySettingsDesc} />

      {/* Enable Memory toggle */}
      <div
        className="flex items-center justify-between rounded-xl px-4 py-3"
        style={{ border: '1px solid var(--c-border-subtle)', background: 'var(--c-bg-menu)' }}
      >
        <div className="flex-1 pr-4">
          <p className="text-sm font-medium text-[var(--c-text-heading)]">{ds.memoryEnabled}</p>
          <p className="text-xs text-[var(--c-text-muted)]">{ds.memoryEnabledDesc}</p>
        </div>
        <PillToggle
          checked={enabled}
          onChange={(next) => { if (memConfig) void saveConfig({ ...memConfig, enabled: next }) }}
        />
      </div>

      {enabled && memConfig && (
        <>
          {/* OpenViking provider card */}
          <div
            className="rounded-xl transition-[border-color] duration-150"
            style={{
              border: '1.5px solid var(--c-accent)',
              background: 'var(--c-bg-menu)',
            }}
          >
            <div className="flex w-full items-start gap-3 rounded-xl p-4 text-left">
              <div
                className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center"
                style={{ color: 'var(--c-accent)' }}
              >
                <Database size={18} />
              </div>
              <div className="min-w-0 flex-1">
                <span className="text-sm font-medium text-[var(--c-text-heading)]">OpenViking</span>
                <p className="mt-0.5 text-xs leading-relaxed text-[var(--c-text-muted)]">
                  {ds.memoryOpenvikingProviderDesc}
                </p>
              </div>
            </div>
            <div className="flex items-center justify-end gap-3 border-t border-[var(--c-border-subtle)] px-4 py-3">
              <div className="flex items-center gap-2">
                <div
                  className="h-2 w-2 shrink-0 rounded-full"
                  style={{ background: statusDotColor(health) }}
                />
                <span
                  className="text-xs"
                  style={{ color: health === 'ok' ? 'var(--c-text-muted)' : statusDotColor(health) }}
                >
                  {healthLabel}
                </span>
              </div>
              <button
                type="button"
                onClick={() => setConfigModalOpen(true)}
                className={secondaryButtonSmCls}
                style={secondaryButtonBorderStyle}
              >
                <Settings size={14} />
                {ds.memoryConfigureButton}
              </button>
            </div>
          </div>

          {/* Auto-summarize toggle */}
          <div
            className="flex items-center justify-between rounded-xl px-4 py-3"
            style={{ border: '1px solid var(--c-border-subtle)', background: 'var(--c-bg-menu)' }}
          >
            <div className="flex-1 pr-4">
              <p className="text-sm font-medium text-[var(--c-text-heading)]">{ds.memoryAutoSummarizeLabel}</p>
              <p className="text-xs text-[var(--c-text-muted)]">{ds.memoryAutoSummarizeDesc}</p>
            </div>
            <PillToggle
              checked={memConfig.memoryCommitEachTurn !== false}
              onChange={(next) => void saveConfig({ ...memConfig, memoryCommitEachTurn: next })}
            />
          </div>

          <div className="border-t border-[var(--c-border-subtle)]" />

          {/* Snapshot section header */}
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <FileText size={15} className="text-[var(--c-text-secondary)]" />
              <h4 className="text-sm font-semibold text-[var(--c-text-heading)]">{ds.memorySnapshotTitle}</h4>
            </div>
            <button
              onClick={() => void loadData(true)}
              disabled={refreshing}
              className="shrink-0 rounded-lg p-1.5 text-[var(--c-text-muted)] transition-colors hover:text-[var(--c-text-secondary)] disabled:opacity-40"
            >
              <RefreshCw size={14} className={refreshing ? 'animate-spin' : ''} />
            </button>
          </div>

          {isConfigured && <SnapshotView snapshot={snapshot} />}
        </>
      )}

      <MemoryConfigModal
        open={configModalOpen}
        onClose={() => setConfigModalOpen(false)}
        accessToken={accessToken}
        memConfig={memConfig}
        onConfigSaved={(cfg) => { setMemConfigState(cfg); void probeHealth(cfg); void loadData(true) }}
      />
    </div>
  )
}
