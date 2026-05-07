import { useEffect, useMemo, useState } from 'react'
import { FolderOpen, Loader2, RotateCcw } from 'lucide-react'
import { getDesktopApi } from '@arkloop/shared/desktop'
import type { DesktopConfig } from '@arkloop/shared/desktop'
import { useLocale } from '../../contexts/LocaleContext'
import { secondaryButtonBorderStyle } from '../buttonStyles'

export function WorkspaceSettings() {
  const { t } = useLocale()
  const labels = t.desktopSettings
  const desktopApi = useMemo(() => getDesktopApi(), [])
  const [config, setConfig] = useState<DesktopConfig | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!desktopApi?.config) return
    let cancelled = false
    void desktopApi.config.get()
      .then((next) => {
        if (!cancelled) setConfig(next)
      })
      .catch(() => {
        if (!cancelled) setError(labels.workspaceLoadFailed ?? 'Failed to load workspace settings')
      })
    return () => {
      cancelled = true
    }
  }, [desktopApi, labels.workspaceLoadFailed])

  useEffect(() => {
    if (!desktopApi?.config) return
    return desktopApi.config.onChanged((next) => setConfig(next))
  }, [desktopApi])

  const saveRoot = async (root?: string) => {
    if (!desktopApi?.config || !config) return
    setSaving(true)
    setError('')
    try {
      await desktopApi.config.set({
        ...config,
        workspace: root ? { root } : {},
      })
    } catch {
      setError(labels.workspaceSaveFailed ?? 'Failed to save workspace settings')
    } finally {
      setSaving(false)
    }
  }

  const handleChoose = async () => {
    const folder = await desktopApi?.dialog?.openFolder()
    if (!folder) return
    await saveRoot(folder)
  }

  const currentRoot = config?.workspace?.root ?? ''

  return (
    <div className="mx-auto max-w-2xl space-y-5">
      <div>
        <h3 className="text-sm font-medium text-[var(--c-text-heading)]">
          {labels.workspaceTitle ?? 'Working Directory'}
        </h3>
        <p className="mt-1 text-xs text-[var(--c-text-muted)]">
          {labels.workspaceSubtitle ?? 'Choose the folder AI tools use as their read/write workspace.'}
        </p>
      </div>

      <div
        className="rounded-xl px-5 py-4"
        style={{ border: '0.5px solid var(--c-border-subtle)', background: 'var(--c-bg-menu)' }}
      >
        <div className="flex items-center justify-between gap-4">
          <div className="min-w-0">
            <div className="text-sm font-medium text-[var(--c-text-primary)]">
              {labels.workspaceCurrent ?? 'Current directory'}
            </div>
            <div className="mt-1 truncate text-xs text-[var(--c-text-muted)]">
              {currentRoot || (labels.workspaceDefault ?? 'Default')}
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <button
              type="button"
              onClick={() => void handleChoose()}
              disabled={saving || !desktopApi?.dialog}
              className="button-secondary inline-flex h-8 items-center justify-center gap-1.5 rounded-lg px-3 text-sm font-medium text-[var(--c-text-secondary)] transition-colors disabled:cursor-not-allowed disabled:opacity-50"
              style={secondaryButtonBorderStyle}
            >
              {saving ? <Loader2 size={14} className="animate-spin" /> : <FolderOpen size={14} />}
              {labels.workspaceChoose ?? 'Open folder'}
            </button>
            {currentRoot && (
              <button
                type="button"
                onClick={() => void saveRoot(undefined)}
                disabled={saving}
                className="button-secondary inline-flex h-8 items-center justify-center gap-1.5 rounded-lg px-3 text-sm font-medium text-[var(--c-text-secondary)] transition-colors disabled:cursor-not-allowed disabled:opacity-50"
                style={secondaryButtonBorderStyle}
              >
                <RotateCcw size={14} />
                {labels.workspaceReset ?? 'Reset'}
              </button>
            )}
          </div>
        </div>
        <p className="mt-3 text-xs text-[var(--c-text-tertiary)]">
          {labels.workspacePermissionHint ?? 'Reads stay inside this folder. Writes ask for permission; choosing session allow keeps the approval until the app restarts or the folder changes.'}
        </p>
      </div>

      {error && <p className="text-xs text-[var(--c-status-error-text)]">{error}</p>}
    </div>
  )
}
