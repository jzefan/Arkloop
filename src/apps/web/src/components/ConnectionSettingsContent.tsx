import { useState, useEffect, useCallback } from 'react'
import { Wifi, HardDrive, Cloud, Server, RefreshCw, CheckCircle, XCircle } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { getDesktopApi } from '@arkloop/shared/desktop'
import type { ConnectionMode } from '@arkloop/shared/desktop'
import { useLocale } from '../contexts/LocaleContext'

type SidecarStatus = 'stopped' | 'starting' | 'running' | 'crashed'

type ModeCardProps = {
  mode: ConnectionMode
  icon: LucideIcon
  label: string
  desc: string
  selected: boolean
  onSelect: () => void
}

function ModeCard({ icon: Icon, label, desc, selected, onSelect }: ModeCardProps) {
  return (
    <button
      type="button"
      onClick={onSelect}
      className="flex items-start gap-3 rounded-xl p-4 text-left transition-colors"
      style={{
        border: selected
          ? '1.5px solid var(--c-accent)'
          : '1px solid var(--c-border-subtle)',
        background: selected ? 'var(--c-bg-deep)' : 'var(--c-bg-page)',
      }}
    >
      <div
        className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg"
        style={{
          background: selected ? 'var(--c-accent)' : 'var(--c-bg-sub)',
          color: selected ? '#fff' : 'var(--c-text-secondary)',
        }}
      >
        <Icon size={18} />
      </div>
      <div className="min-w-0 flex-1">
        <div className="text-sm font-medium text-[var(--c-text-heading)]">{label}</div>
        <div className="mt-0.5 text-xs text-[var(--c-text-muted)]">{desc}</div>
      </div>
      <div
        className="mt-1 h-4 w-4 shrink-0 rounded-full"
        style={{
          border: selected ? '5px solid var(--c-accent)' : '1.5px solid var(--c-border-subtle)',
          background: selected ? '#fff' : 'transparent',
        }}
      />
    </button>
  )
}

function StatusBadge({ status, t }: { status: SidecarStatus; t: Record<string, string> }) {
  const map: Record<SidecarStatus, { color: string; label: string }> = {
    running:  { color: '#22c55e', label: t.running },
    stopped:  { color: '#94a3b8', label: t.stopped },
    starting: { color: '#f59e0b', label: t.starting },
    crashed:  { color: '#ef4444', label: t.crashed },
  }
  const { color, label } = map[status]
  return (
    <span className="inline-flex items-center gap-1.5 text-xs">
      <span className="h-2 w-2 rounded-full" style={{ background: color }} />
      <span style={{ color }}>{label}</span>
    </span>
  )
}

export function ConnectionSettingsContent() {
  const { t } = useLocale()
  const ct = t.connection
  const api = getDesktopApi()

  const [mode, setMode] = useState<ConnectionMode>('local')
  const [saasUrl, setSaasUrl] = useState('')
  const [selfHostedUrl, setSelfHostedUrl] = useState('')
  const [sidecarStatus, setSidecarStatus] = useState<SidecarStatus>('stopped')
  const [testResult, setTestResult] = useState<'connected' | 'failed' | null>(null)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!api) return
    api.config.get().then((cfg) => {
      setMode(cfg.mode)
      setSaasUrl(cfg.saas.baseUrl)
      setSelfHostedUrl(cfg.selfHosted.baseUrl)
    })
    api.sidecar.getStatus().then(setSidecarStatus)
    const unsub = api.sidecar.onStatusChanged((s) => setSidecarStatus(s as SidecarStatus))
    return unsub
  }, [api])

  const handleSave = useCallback(async () => {
    if (!api) return
    setSaving(true)
    try {
      const current = await api.config.get()
      await api.config.set({
        ...current,
        mode,
        saas: { baseUrl: saasUrl },
        selfHosted: { baseUrl: selfHostedUrl },
      })
    } finally {
      setSaving(false)
    }
  }, [api, mode, saasUrl, selfHostedUrl])

  const handleTest = useCallback(async () => {
    setTestResult(null)
    let url: string
    if (mode === 'local') {
      const cfg = await api?.config.get()
      url = `http://127.0.0.1:${cfg?.local.port ?? 19001}`
    } else if (mode === 'saas') {
      url = saasUrl
    } else {
      url = selfHostedUrl
    }
    try {
      const resp = await fetch(`${url}/healthz`, { signal: AbortSignal.timeout(5000) })
      setTestResult(resp.ok ? 'connected' : 'failed')
    } catch {
      setTestResult('failed')
    }
  }, [api, mode, saasUrl, selfHostedUrl])

  const handleRestart = useCallback(async () => {
    if (!api) return
    await api.sidecar.restart()
  }, [api])

  if (!api) return null

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-2">
        <span className="text-sm font-medium text-[var(--c-text-heading)]">{ct.title}</span>
        <div className="flex flex-col gap-2">
          <ModeCard
            mode="local" icon={HardDrive}
            label={ct.local} desc={ct.localDesc}
            selected={mode === 'local'} onSelect={() => setMode('local')}
          />
          <ModeCard
            mode="saas" icon={Cloud}
            label={ct.saas} desc={ct.saasDesc}
            selected={mode === 'saas'} onSelect={() => setMode('saas')}
          />
          <ModeCard
            mode="self-hosted" icon={Server}
            label={ct.selfHosted} desc={ct.selfHostedDesc}
            selected={mode === 'self-hosted'} onSelect={() => setMode('self-hosted')}
          />
        </div>
      </div>

      {mode === 'local' && (
        <div className="flex flex-col gap-3">
          <div className="flex items-center justify-between">
            <span className="text-sm text-[var(--c-text-secondary)]">{ct.status}</span>
            <div className="flex items-center gap-2">
              <StatusBadge status={sidecarStatus} t={ct} />
              <button
                type="button"
                onClick={handleRestart}
                className="flex h-6 items-center gap-1 rounded-md px-2 text-xs text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)]"
              >
                <RefreshCw size={12} />
                <span>{ct.restart}</span>
              </button>
            </div>
          </div>
        </div>
      )}

      {mode === 'saas' && (
        <div className="flex flex-col gap-2">
          <label className="text-sm text-[var(--c-text-secondary)]">{ct.baseUrl}</label>
          <input
            type="url"
            value={saasUrl}
            onChange={(e) => setSaasUrl(e.target.value)}
            className="h-9 rounded-lg px-3 text-sm text-[var(--c-text-primary)] outline-none"
            style={{ border: '0.5px solid var(--c-border-subtle)', background: 'var(--c-bg-page)' }}
          />
        </div>
      )}

      {mode === 'self-hosted' && (
        <div className="flex flex-col gap-2">
          <label className="text-sm text-[var(--c-text-secondary)]">{ct.baseUrl}</label>
          <input
            type="url"
            value={selfHostedUrl}
            onChange={(e) => setSelfHostedUrl(e.target.value)}
            placeholder="https://your-server.com"
            className="h-9 rounded-lg px-3 text-sm text-[var(--c-text-primary)] outline-none"
            style={{ border: '0.5px solid var(--c-border-subtle)', background: 'var(--c-bg-page)' }}
          />
        </div>
      )}

      <div className="flex items-center gap-2">
        <button
          type="button"
          onClick={handleSave}
          disabled={saving}
          className="flex h-8 items-center rounded-lg px-4 text-sm font-medium text-white transition-colors"
          style={{ background: 'var(--c-accent)' }}
        >
          {ct.save}
        </button>
        <button
          type="button"
          onClick={handleTest}
          className="flex h-8 items-center gap-1.5 rounded-lg px-3 text-sm text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)]"
          style={{ border: '0.5px solid var(--c-border-subtle)' }}
        >
          <Wifi size={13} />
          <span>{ct.testConnection}</span>
        </button>
        {testResult === 'connected' && (
          <span className="flex items-center gap-1 text-xs" style={{ color: '#22c55e' }}>
            <CheckCircle size={13} /> {ct.connected}
          </span>
        )}
        {testResult === 'failed' && (
          <span className="flex items-center gap-1 text-xs" style={{ color: '#ef4444' }}>
            <XCircle size={13} /> {ct.failed}
          </span>
        )}
      </div>
    </div>
  )
}
