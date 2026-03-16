import { useState, useEffect, useCallback } from 'react'
import {
  Shield, ShieldCheck, Download, Trash2, Loader2,
  AlertTriangle, Check, Monitor, FolderOpen, ChevronDown, ChevronRight,
} from 'lucide-react'
import { useLocale } from '../../contexts/LocaleContext'
import { getDesktopApi, isDesktop } from '@arkloop/shared/desktop'
import { SettingsSectionHeader } from './_SettingsSectionHeader'

type IsolationMode = 'trusted' | 'vm'

type IsolationConfig = {
  mode: IsolationMode
  vmResources?: { cpuCount?: number; memoryMiB?: number }
  vmKernelPath?: string
  vmRootfsPath?: string
  vmInitrdPath?: string
}

type VmImageStatus = 'not_installed' | 'downloading' | 'ready' | 'error' | 'unsupported' | 'custom'

type VmProgress = {
  phase: string
  percent: number
  bytesDownloaded: number
  bytesTotal: number
  error?: string
}

const isMac = typeof navigator !== 'undefined' && /Mac/.test(navigator.userAgent)

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

const inputCls =
  'w-full rounded-lg border border-[var(--c-border-subtle)] bg-[var(--c-bg-input)] px-3 py-2 text-sm ' +
  'text-[var(--c-text-primary)] outline-none placeholder:text-[var(--c-text-muted)] ' +
  'focus:border-[var(--c-border)] transition-colors duration-150'

export function IsolationSettings() {
  const { t } = useLocale()
  const iso = t.isolation

  const [config, setConfig] = useState<IsolationConfig>({ mode: 'trusted' })
  const [vmStatus, setVmStatus] = useState<VmImageStatus>('not_installed')
  const [downloading, setDownloading] = useState(false)
  const [progress, setProgress] = useState<VmProgress | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showDevPaths, setShowDevPaths] = useState(false)
  const [devKernel, setDevKernel] = useState('')
  const [devRootfs, setDevRootfs] = useState('')
  const [devInitrd, setDevInitrd] = useState('')
  const [devInstalling, setDevInstalling] = useState(false)

  const loadState = useCallback(async () => {
    if (!isDesktop()) return
    const api = getDesktopApi()
    if (!api?.isolation || !api?.vm) return
    try {
      const [cfg, st] = await Promise.all([
        api.isolation.getConfig(),
        api.vm.getStatus(),
      ])
      setConfig(cfg)
      setVmStatus(st)
      // Pre-fill dev path fields from saved config
      if (cfg.vmKernelPath) setDevKernel(cfg.vmKernelPath)
      if (cfg.vmRootfsPath) setDevRootfs(cfg.vmRootfsPath)
      if (cfg.vmInitrdPath) setDevInitrd(cfg.vmInitrdPath)
      // Auto-open dev panel if custom paths are saved
      if (cfg.vmKernelPath || cfg.vmRootfsPath) setShowDevPaths(true)
    } catch {}
  }, [])

  useEffect(() => {
    loadState()
  }, [loadState])

  useEffect(() => {
    if (!isDesktop()) return
    const api = getDesktopApi()
    if (!api?.vm) return
    const unsub = api.vm.onDownloadProgress((p: VmProgress) => {
      setProgress(p)
      if (p.phase === 'done') {
        setDownloading(false)
        setVmStatus('ready')
        setProgress(null)
      }
      if (p.phase === 'error') {
        setDownloading(false)
        setVmStatus('error')
        setError(p.error ?? 'Download failed')
      }
    })
    return unsub
  }, [])

  const vmReady = vmStatus === 'ready' || vmStatus === 'custom'

  const handleModeChange = async (mode: IsolationMode) => {
    if (!isDesktop()) return
    const api = getDesktopApi()
    if (!api?.isolation) return
    if (mode === 'vm' && !vmReady) return

    setSaving(true)
    setError(null)
    try {
      const next: IsolationConfig = { ...config, mode }
      await api.isolation.setConfig(next)
      setConfig(next)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  const handleDownload = async () => {
    if (!isDesktop()) return
    const api = getDesktopApi()
    if (!api?.vm) return
    setDownloading(true)
    setError(null)
    setProgress(null)
    try {
      await api.vm.download()
    } catch (e) {
      setDownloading(false)
      setError(e instanceof Error ? e.message : 'Download failed')
    }
  }

  const handleDelete = async () => {
    if (!isDesktop()) return
    const api = getDesktopApi()
    if (!api?.vm) return
    if (config.mode === 'vm') {
      await handleModeChange('trusted')
    }
    setError(null)
    try {
      await api.vm.delete()
      setVmStatus('not_installed')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Delete failed')
    }
  }

  const handleInstallLocal = async () => {
    if (!devKernel.trim() || !devRootfs.trim()) {
      setError('Kernel path and rootfs path are both required')
      return
    }
    if (!isDesktop()) return
    const api = getDesktopApi()
    if (!api?.vm || !api?.isolation) return

    setDevInstalling(true)
    setError(null)
    try {
      await api.vm.installLocal(devKernel.trim(), devRootfs.trim(), devInitrd.trim() || undefined)
      // Save paths into the isolation config
      const next: IsolationConfig = {
        ...config,
        vmKernelPath: devKernel.trim(),
        vmRootfsPath: devRootfs.trim(),
        vmInitrdPath: devInitrd.trim() || undefined,
      }
      await api.isolation.setConfig(next)
      setConfig(next)
      setVmStatus('custom')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Installation failed')
    } finally {
      setDevInstalling(false)
    }
  }

  const handleClearLocalPaths = async () => {
    if (!isDesktop()) return
    const api = getDesktopApi()
    if (!api?.isolation) return
    const next: IsolationConfig = {
      ...config,
      mode: 'trusted',
      vmKernelPath: undefined,
      vmRootfsPath: undefined,
      vmInitrdPath: undefined,
    }
    await api.isolation.setConfig(next)
    setConfig(next)
    setDevKernel('')
    setDevRootfs('')
    setDevInitrd('')
    setVmStatus('not_installed')
  }

  if (!isDesktop() || !isMac) {
    return (
      <div className="space-y-6">
        <SettingsSectionHeader title={iso.title} description={iso.unsupported} />
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <SettingsSectionHeader title={iso.title} description={iso.description} />

      {error && (
        <div className="flex items-start gap-2 rounded-lg bg-red-500/10 px-3 py-2.5 text-sm text-red-400">
          <AlertTriangle size={16} className="mt-0.5 shrink-0" />
          <span className="whitespace-pre-wrap break-all">{error}</span>
        </div>
      )}

      {/* Mode selection */}
      <div className="space-y-2">
        <ModeCard
          active={config.mode === 'trusted'}
          icon={<Monitor size={20} />}
          title={iso.trustedTitle}
          description={iso.trustedDesc}
          onClick={() => handleModeChange('trusted')}
          disabled={saving}
        />
        <ModeCard
          active={config.mode === 'vm'}
          icon={<ShieldCheck size={20} />}
          title={iso.vmTitle}
          description={iso.vmDesc}
          onClick={() => handleModeChange('vm')}
          disabled={saving || !vmReady}
          badge={!vmReady ? iso.vmImagesRequired : undefined}
        />
      </div>

      {/* VM Images: Download from releases */}
      <div className="space-y-3">
        <h4 className="text-sm font-medium text-[var(--c-text-heading)]">{iso.vmImagesTitle}</h4>

        <div className="rounded-xl border border-[var(--c-border-subtle)] bg-[var(--c-bg-card)] p-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className={`flex h-9 w-9 items-center justify-center rounded-lg ${
                vmStatus === 'ready' ? 'bg-green-500/15 text-green-500'
                : vmStatus === 'custom' ? 'bg-blue-500/15 text-blue-400'
                : 'bg-[var(--c-bg-deep)] text-[var(--c-text-muted)]'
              }`}>
                <Shield size={18} />
              </div>
              <div>
                <div className="text-sm font-medium text-[var(--c-text-heading)]">
                  {iso.vmKernelRootfs}
                </div>
                <div className="text-xs text-[var(--c-text-secondary)]">
                  {vmStatus === 'ready' ? iso.vmInstalled
                    : vmStatus === 'custom' ? iso.vmCustomInstalled
                    : vmStatus === 'downloading' ? iso.vmDownloading
                    : vmStatus === 'error' ? iso.vmDownloadError
                    : iso.vmNotInstalled}
                </div>
              </div>
            </div>

            <div className="flex items-center gap-2">
              {vmReady && (
                <button
                  onClick={handleDelete}
                  className="flex h-8 items-center gap-1.5 rounded-lg border border-[var(--c-border-subtle)] px-3 text-xs font-medium text-[var(--c-text-secondary)] transition-colors hover:border-red-500/40 hover:text-red-400"
                >
                  <Trash2 size={13} />
                  {iso.vmDelete}
                </button>
              )}
              {!vmReady && !downloading && (
                <button
                  onClick={handleDownload}
                  className="flex h-8 items-center gap-1.5 rounded-lg bg-[var(--c-accent)] px-3 text-xs font-medium text-white transition-opacity hover:opacity-90"
                >
                  <Download size={13} />
                  {iso.vmDownload}
                </button>
              )}
              {downloading && (
                <div className="flex items-center gap-2 text-xs text-[var(--c-text-secondary)]">
                  <Loader2 size={14} className="animate-spin" />
                  {progress ? `${progress.percent}%` : iso.vmConnecting}
                </div>
              )}
            </div>
          </div>

          {downloading && progress && progress.phase === 'downloading' && (
            <div className="mt-3">
              <div className="h-1.5 overflow-hidden rounded-full bg-[var(--c-bg-deep)]">
                <div
                  className="h-full rounded-full bg-[var(--c-accent)] transition-[width] duration-300"
                  style={{ width: `${progress.percent}%` }}
                />
              </div>
              <div className="mt-1 flex justify-between text-[10px] text-[var(--c-text-muted)]">
                <span>{formatBytes(progress.bytesDownloaded)} / {formatBytes(progress.bytesTotal)}</span>
                <span>{progress.percent}%</span>
              </div>
            </div>
          )}

          {downloading && progress && progress.phase === 'extracting' && (
            <div className="mt-3 flex items-center gap-2 text-xs text-[var(--c-text-secondary)]">
              <Loader2 size={13} className="animate-spin" />
              {iso.vmExtracting}
            </div>
          )}
        </div>
      </div>

      {/* Developer: Use local files */}
      <div className="rounded-xl border border-[var(--c-border-subtle)] overflow-hidden">
        <button
          onClick={() => setShowDevPaths(!showDevPaths)}
          className="flex w-full items-center justify-between px-4 py-3 text-sm font-medium text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)] transition-colors"
        >
          <div className="flex items-center gap-2">
            <FolderOpen size={15} />
            {iso.devLocalPathsTitle}
          </div>
          {showDevPaths ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </button>

        {showDevPaths && (
          <div className="border-t border-[var(--c-border-subtle)] bg-[var(--c-bg-card)] px-4 py-4 space-y-3">
            <p className="text-xs text-[var(--c-text-secondary)]">{iso.devLocalPathsDesc}</p>

            <div className="space-y-2">
              <label className="block text-xs font-medium text-[var(--c-text-secondary)]">
                {iso.devKernelPath}
              </label>
              <input
                type="text"
                value={devKernel}
                onChange={e => setDevKernel(e.target.value)}
                placeholder="/tmp/vz-test/vmlinux"
                className={inputCls}
              />
            </div>

            <div className="space-y-2">
              <label className="block text-xs font-medium text-[var(--c-text-secondary)]">
                {iso.devRootfsPath}
              </label>
              <input
                type="text"
                value={devRootfs}
                onChange={e => setDevRootfs(e.target.value)}
                placeholder="/tmp/vz-test/rootfs.ext4"
                className={inputCls}
              />
            </div>

            <div className="space-y-2">
              <label className="block text-xs font-medium text-[var(--c-text-secondary)]">
                {iso.devInitrdPath} <span className="text-[var(--c-text-muted)]">({iso.devOptional})</span>
              </label>
              <input
                type="text"
                value={devInitrd}
                onChange={e => setDevInitrd(e.target.value)}
                placeholder="/tmp/vz-test/initramfs-custom.gz"
                className={inputCls}
              />
            </div>

            <div className="flex items-center gap-2 pt-1">
              <button
                onClick={handleInstallLocal}
                disabled={devInstalling || !devKernel.trim() || !devRootfs.trim()}
                className="flex h-8 items-center gap-1.5 rounded-lg bg-[var(--c-accent)] px-3 text-xs font-medium text-white disabled:opacity-50 transition-opacity hover:opacity-90"
              >
                {devInstalling ? <Loader2 size={13} className="animate-spin" /> : <Check size={13} />}
                {iso.devApply}
              </button>
              {vmStatus === 'custom' && (
                <button
                  onClick={handleClearLocalPaths}
                  className="flex h-8 items-center gap-1.5 rounded-lg border border-[var(--c-border-subtle)] px-3 text-xs font-medium text-[var(--c-text-secondary)] transition-colors hover:border-red-500/40 hover:text-red-400"
                >
                  <Trash2 size={13} />
                  {iso.devClear}
                </button>
              )}
            </div>
          </div>
        )}
      </div>

      <p className="text-xs text-[var(--c-text-muted)]">{iso.note}</p>
    </div>
  )
}

function ModeCard({
  active, icon, title, description, onClick, disabled, badge,
}: {
  active: boolean
  icon: React.ReactNode
  title: string
  description: string
  onClick: () => void
  disabled?: boolean
  badge?: string
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className={[
        'flex w-full items-start gap-3 rounded-xl border p-4 text-left transition-colors',
        active
          ? 'border-[var(--c-accent)] bg-[var(--c-accent)]/5'
          : 'border-[var(--c-border-subtle)] bg-[var(--c-bg-card)] hover:border-[var(--c-border)]',
        disabled && !active ? 'cursor-not-allowed opacity-50' : '',
      ].join(' ')}
    >
      <div className={`mt-0.5 shrink-0 ${active ? 'text-[var(--c-accent)]' : 'text-[var(--c-text-muted)]'}`}>
        {icon}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-[var(--c-text-heading)]">{title}</span>
          {active && <Check size={14} className="text-[var(--c-accent)]" />}
          {badge && (
            <span className="rounded-full bg-amber-500/15 px-1.5 py-0.5 text-[10px] font-medium text-amber-500">
              {badge}
            </span>
          )}
        </div>
        <p className="mt-0.5 text-xs text-[var(--c-text-secondary)]">{description}</p>
      </div>
    </button>
  )
}
