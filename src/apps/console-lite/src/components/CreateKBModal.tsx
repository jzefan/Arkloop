import { useCallback, useState } from 'react'
import { createKnowledgeBase } from '../api/knowledge-bases'
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

export function CreateKBModal({ open, onClose, onCreated, accessToken, workspaceRef }: Props) {
  const { t } = useLocale()
  const tk = t.knowledgeBases
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  const handleSubmit = useCallback(async () => {
    const trimmedName = name.trim()
    if (!trimmedName || submitting) return
    setSubmitting(true)
    setError('')
    try {
      const kb = await createKnowledgeBase(accessToken, {
        name: trimmedName,
        workspace_ref: workspaceRef,
        description: description.trim(),
      })
      setName('')
      setDescription('')
      onCreated(kb.id)
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSubmitting(false)
    }
  }, [accessToken, description, name, onClose, onCreated, submitting, workspaceRef])

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
