import { useCallback, useEffect, useState } from 'react'
import { createKnowledgeBase, getPlatformConfig, listKnowledgeScopes, type KnowledgeScope } from '../api/knowledge-bases'
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

// scopeOptionLabel renders the display_name with a small hierarchy-level hint.
function scopeOptionLabel(scope: KnowledgeScope): string {
  return `${scope.display_name} (${scope.type})`
}

export function CreateKBModal({ open, onClose, onCreated, accessToken, workspaceRef }: Props) {
  const { t } = useLocale()
  const tk = t.knowledgeBases
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [visibility, setVisibility] = useState<'workspace_member' | 'private'>('workspace_member')
  const [integrationMode, setIntegrationMode] = useState<'standalone' | 'exam'>('standalone')
  const [knowledgeScopeId, setKnowledgeScopeId] = useState('')
  const [knowledgeScopes, setKnowledgeScopes] = useState<KnowledgeScope[]>([])
  const [knowledgeScopesError, setKnowledgeScopesError] = useState('')
  const [linkedScopesEnabled, setLinkedScopesEnabled] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (!open) return
    getPlatformConfig(accessToken)
      .then((cfg) => setLinkedScopesEnabled(cfg.exam_integration_enabled))
      .catch(() => setLinkedScopesEnabled(false))
  }, [open, accessToken])

  useEffect(() => {
    if (!open || integrationMode !== 'exam' || !linkedScopesEnabled || knowledgeScopes.length > 0) return
    listKnowledgeScopes(accessToken)
      .then((items) => {
        setKnowledgeScopes(items)
        setKnowledgeScopesError('')
      })
      .catch((err) => {
        setKnowledgeScopesError(err instanceof Error ? err.message : String(err))
      })
  }, [open, integrationMode, linkedScopesEnabled, accessToken, knowledgeScopes.length])

  const handleSubmit = useCallback(async () => {
    const trimmedName = name.trim()
    if (!trimmedName || submitting) return
    if (integrationMode === 'exam' && !knowledgeScopeId.trim()) {
      setError('请选择课程范围')
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
        exam_scope_id: integrationMode === 'exam' ? knowledgeScopeId.trim() : undefined,
      })
      setName('')
      setDescription('')
      setVisibility('workspace_member')
      setIntegrationMode('standalone')
      setKnowledgeScopeId('')
      onCreated(kb.id)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSubmitting(false)
    }
  }, [accessToken, description, knowledgeScopeId, integrationMode, name, onClose, onCreated, submitting, visibility, workspaceRef])

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
        {linkedScopesEnabled && (
          <FormField label="题库来源">
            <div className="flex gap-2">
              <button type="button" onClick={() => setIntegrationMode('standalone')} className={integrationMode === 'standalone' ? radioActiveClassName : radioClassName}>
                <span className="text-[var(--c-text-primary)]">独立模式</span>
              </button>
              <button type="button" onClick={() => setIntegrationMode('exam')} className={integrationMode === 'exam' ? radioActiveClassName : radioClassName}>
                <span className="text-[var(--c-text-primary)]">绑定课程范围</span>
              </button>
            </div>
          </FormField>
        )}
        {integrationMode === 'exam' && (
          <FormField label="课程范围">
            <KnowledgeScopeSelect
              scopes={knowledgeScopes}
              value={knowledgeScopeId}
              onChange={setKnowledgeScopeId}
              loading={knowledgeScopes.length === 0 && knowledgeScopesError === ''}
              error={knowledgeScopesError}
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

type KnowledgeScopeSelectProps = {
  scopes: KnowledgeScope[]
  value: string
  onChange: (id: string) => void
  loading: boolean
  error: string
}

function KnowledgeScopeSelect({ scopes, value, onChange, loading, error }: KnowledgeScopeSelectProps) {
  if (error) {
    return <p className="text-xs text-[var(--c-status-error-text)]">加载课程范围失败：{error}</p>
  }
  if (loading) {
    return <p className="text-xs text-[var(--c-text-muted)]">加载课程范围中…</p>
  }
  if (scopes.length === 0) {
    return <p className="text-xs text-[var(--c-text-muted)]">未找到可绑定的课程范围</p>
  }
  return (
    <select
      value={value}
      onChange={(event) => onChange(event.target.value)}
      className={inputClassName}
    >
      <option value="">请选择课程范围…</option>
      {scopes.map((scope) => (
        <option key={scope.id} value={scope.id}>
          {scopeOptionLabel(scope)}
        </option>
      ))}
    </select>
  )
}
