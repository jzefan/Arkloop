import { useCallback, useEffect, useState } from 'react'
import { createKnowledgeBase, getPlatformConfig, listExamScopes, type ExamScope } from '../api/knowledge-bases'
import { useLocale } from '../contexts/LocaleContext'
import { FormField } from './FormField'
import { Modal } from './Modal'

type Props = {
  open: boolean
  onClose: () => void
  onCreated: (kbId: string) => void
  accessToken: string
  workspaceRef?: string
}

const inputClassName = 'w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-bg-input)] px-3 py-2 text-sm text-[var(--c-text-primary)] placeholder:text-[var(--c-text-muted)] outline-none transition-colors focus:border-[var(--c-border-focus)]'
const radioClassName = 'flex cursor-pointer items-center gap-2 rounded-lg border border-[var(--c-border)] px-3 py-2 text-sm transition-colors hover:bg-[var(--c-bg-sub)]'
const radioActiveClassName = 'flex cursor-pointer items-center gap-2 rounded-lg border border-[var(--c-border-focus)] bg-[var(--c-bg-sub)] px-3 py-2 text-sm'

// scopeOptionLabel renders the display_name with a small chip showing the
// scope type (major/direction/topic) so teachers know which hierarchy level
// they are binding the KB to.
function scopeOptionLabel(scope: ExamScope): string {
  return `${scope.display_name} (${scope.type})`
}

export function CreateKBModal({ open, onClose, onCreated, accessToken, workspaceRef }: Props) {
  const { t } = useLocale()
  const tk = t.knowledgeBases
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [visibility, setVisibility] = useState<'workspace_member' | 'private'>('workspace_member')
  const [integrationMode, setIntegrationMode] = useState<'standalone' | 'exam'>('standalone')
  const [examScopeId, setExamScopeId] = useState('')
  const [examScopes, setExamScopes] = useState<ExamScope[]>([])
  const [examScopesError, setExamScopesError] = useState('')
  const [examEnabled, setExamEnabled] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) return
    getPlatformConfig(accessToken)
      .then((cfg) => setExamEnabled(cfg.exam_integration_enabled))
      .catch(() => setExamEnabled(false))
  }, [open, accessToken])

  // Fetch exam scopes lazily when the teacher switches to exam mode.
  useEffect(() => {
    if (!open || integrationMode !== 'exam' || !examEnabled || examScopes.length > 0) return
    listExamScopes(accessToken)
      .then((items) => {
        setExamScopes(items)
        setExamScopesError('')
      })
      .catch((err) => {
        setExamScopesError(err instanceof Error ? err.message : String(err))
      })
  }, [open, integrationMode, examEnabled, accessToken, examScopes.length])

  const handleSubmit = useCallback(async () => {
    const trimmedName = name.trim()
    if (!trimmedName || submitting) return
    if (integrationMode === 'exam' && !examScopeId.trim()) {
      setError('请选择 exam 范围')
      return
    }
    setSubmitting(true)
    setError('')
    try {
      const kb = await createKnowledgeBase(accessToken, {
        name: trimmedName,
        workspace_ref: workspaceRef,
        description: description.trim(),
        visibility,
        integration_mode: integrationMode,
        exam_scope_id: integrationMode === 'exam' ? examScopeId.trim() : undefined,
      })
      setName('')
      setDescription('')
      setVisibility('workspace_member')
      setIntegrationMode('standalone')
      setExamScopeId('')
      onCreated(kb.id)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSubmitting(false)
    }
  }, [accessToken, description, examScopeId, integrationMode, name, onClose, onCreated, submitting, visibility, workspaceRef])

  return (
    <Modal open={open} onClose={onClose} title={tk.createTitle} width="460px">
      <div className="flex flex-col gap-4">
        <FormField label={tk.nameLabel}>
          <input
            type="text"
            value={name}
            onChange={(event) => {
              setName(event.target.value)
              setError('')
            }}
            className={inputClassName}
            placeholder={tk.namePlaceholder}
            autoFocus
          />
        </FormField>
        <FormField label={tk.descriptionLabel}>
          <textarea
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            rows={3}
            className={inputClassName}
          />
        </FormField>
        <FormField label="可见性">
          <div className="flex gap-2">
            <button type="button" onClick={() => setVisibility('workspace_member')} className={visibility === 'workspace_member' ? radioActiveClassName : radioClassName}>
              <span className="text-[var(--c-text-primary)]">工作区全员可见</span>
            </button>
            <button type="button" onClick={() => setVisibility('private')} className={visibility === 'private' ? radioActiveClassName : radioClassName}>
              <span className="text-[var(--c-text-primary)]">仅自己</span>
            </button>
          </div>
        </FormField>
        {examEnabled && (
          <FormField label="集成模式">
            <div className="flex gap-2">
              <button type="button" onClick={() => setIntegrationMode('standalone')} className={integrationMode === 'standalone' ? radioActiveClassName : radioClassName}>
                <span className="text-[var(--c-text-primary)]">独立模式</span>
              </button>
              <button type="button" onClick={() => setIntegrationMode('exam')} className={integrationMode === 'exam' ? radioActiveClassName : radioClassName}>
                <span className="text-[var(--c-text-primary)]">绑定 exam 范围</span>
              </button>
            </div>
          </FormField>
        )}
        {integrationMode === 'exam' && (
          <FormField label="exam 范围">
            <ExamScopeSelect
              scopes={examScopes}
              value={examScopeId}
              onChange={setExamScopeId}
              loading={examScopes.length === 0 && examScopesError === ''}
              error={examScopesError}
            />
            <p className="mt-1 text-xs text-[var(--c-text-muted)]">KB 创建后无法切换模式或范围</p>
          </FormField>
        )}
        {error && <p className="text-xs text-[var(--c-status-error-text)]">{error}</p>}
        <div className="mt-2 flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            disabled={submitting}
            className="rounded-lg border border-[var(--c-border)] px-3.5 py-1.5 text-sm text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)] disabled:opacity-50"
          >
            {t.common.cancel}
          </button>
          <button
            type="button"
            onClick={handleSubmit}
            disabled={!name.trim() || submitting}
            className="rounded-lg bg-[var(--c-bg-tag)] px-3.5 py-1.5 text-sm font-medium text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)] disabled:opacity-50"
          >
            {submitting ? t.common.loading : tk.create}
          </button>
        </div>
      </div>
    </Modal>
  )
}

type ExamScopeSelectProps = {
  scopes: ExamScope[]
  value: string
  onChange: (id: string) => void
  loading: boolean
  error: string
}

// ExamScopeSelect renders a native <select> with display_name + a parenthetical
// type chip (major/direction/topic) on each option so teachers can see which
// hierarchy level they are binding to. Stays minimal — a custom popover with
// chip badges per option can come later if the dropdown grows large.
function ExamScopeSelect({ scopes, value, onChange, loading, error }: ExamScopeSelectProps) {
  if (error) {
    return <p className="text-xs text-[var(--c-status-error-text)]">加载 exam 范围失败：{error}</p>
  }
  if (loading) {
    return <p className="text-xs text-[var(--c-text-muted)]">加载 exam 范围中…</p>
  }
  if (scopes.length === 0) {
    return <p className="text-xs text-[var(--c-text-muted)]">未找到可绑定的 exam 范围</p>
  }
  return (
    <select
      value={value}
      onChange={(event) => onChange(event.target.value)}
      className={inputClassName}
    >
      <option value="">请选择 exam 范围…</option>
      {scopes.map((scope) => (
        <option key={scope.id} value={scope.id}>
          {scopeOptionLabel(scope)}
        </option>
      ))}
    </select>
  )
}
