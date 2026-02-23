import { useState, useCallback, useEffect } from 'react'
import { useOutletContext } from 'react-router-dom'
import { FileText, Plus, Pencil, Trash2 } from 'lucide-react'
import type { ConsoleOutletContext } from '../../layouts/ConsoleLayout'
import { PageHeader } from '../../components/PageHeader'
import { DataTable, type Column } from '../../components/DataTable'
import { Badge } from '../../components/Badge'
import { Modal } from '../../components/Modal'
import { FormField } from '../../components/FormField'
import { ConfirmDialog } from '../../components/ConfirmDialog'
import { useToast } from '../../components/useToast'
import { isApiError } from '../../api'
import { useLocale } from '../../contexts/LocaleContext'
import {
  listPromptTemplates,
  createPromptTemplate,
  updatePromptTemplate,
  deletePromptTemplate,
  type PromptTemplate,
} from '../../api/prompt-templates'

type FormState = {
  name: string
  content: string
  variables: string
  is_default: boolean
}

function emptyForm(): FormState {
  return { name: '', content: '', variables: '', is_default: false }
}

function templateToForm(pt: PromptTemplate): FormState {
  return {
    name: pt.name,
    content: pt.content,
    variables: pt.variables.join(', '),
    is_default: pt.is_default,
  }
}

function parseCommaSeparated(value: string): string[] {
  return value
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
}

type DeleteTarget = { id: string; name: string }

export function PromptTemplatesPage() {
  const { accessToken } = useOutletContext<ConsoleOutletContext>()
  const { addToast } = useToast()
  const { t } = useLocale()
  const tc = t.pages.promptTemplates

  const [templates, setTemplates] = useState<PromptTemplate[]>([])
  const [loading, setLoading] = useState(false)

  const [modalOpen, setModalOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<PromptTemplate | null>(null)
  const [form, setForm] = useState<FormState>(emptyForm)
  const [formError, setFormError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null)
  const [deleting, setDeleting] = useState(false)

  const fetchAll = useCallback(async () => {
    setLoading(true)
    try {
      const list = await listPromptTemplates(accessToken)
      setTemplates(list)
    } catch {
      addToast(tc.toastLoadFailed, 'error')
    } finally {
      setLoading(false)
    }
  }, [accessToken, addToast, tc.toastLoadFailed])

  useEffect(() => {
    void fetchAll()
  }, [fetchAll])

  const handleOpenCreate = useCallback(() => {
    setEditTarget(null)
    setForm(emptyForm())
    setFormError('')
    setModalOpen(true)
  }, [])

  const handleOpenEdit = useCallback((pt: PromptTemplate) => {
    setEditTarget(pt)
    setForm(templateToForm(pt))
    setFormError('')
    setModalOpen(true)
  }, [])

  const handleCloseModal = useCallback(() => {
    if (submitting) return
    setModalOpen(false)
  }, [submitting])

  const handleFormField = useCallback(
    <K extends keyof FormState>(key: K, value: FormState[K]) => {
      setForm((prev) => ({ ...prev, [key]: value }))
      setFormError('')
    },
    [],
  )

  const handleSubmit = useCallback(async () => {
    const name = form.name.trim()
    if (!name) {
      setFormError(tc.errRequired)
      return
    }

    setSubmitting(true)
    setFormError('')
    try {
      if (editTarget) {
        await updatePromptTemplate(
          editTarget.id,
          { name, content: form.content, is_default: form.is_default },
          accessToken,
        )
        addToast(tc.toastUpdated, 'success')
      } else {
        await createPromptTemplate(
          {
            name,
            content: form.content,
            variables: parseCommaSeparated(form.variables),
            is_default: form.is_default,
          },
          accessToken,
        )
        addToast(tc.toastCreated, 'success')
      }
      setModalOpen(false)
      await fetchAll()
    } catch (err) {
      if (isApiError(err)) {
        setFormError(err.message)
      } else {
        setFormError(tc.toastSaveFailed)
      }
    } finally {
      setSubmitting(false)
    }
  }, [form, editTarget, accessToken, fetchAll, addToast, tc])

  const handleDelete = useCallback(async () => {
    if (!deleteTarget) return
    setDeleting(true)
    try {
      await deletePromptTemplate(deleteTarget.id, accessToken)
      setDeleteTarget(null)
      await fetchAll()
      addToast(tc.toastDeleted, 'success')
    } catch {
      addToast(tc.toastDeleteFailed, 'error')
    } finally {
      setDeleting(false)
    }
  }, [deleteTarget, accessToken, fetchAll, addToast, tc])

  const columns: Column<PromptTemplate>[] = [
    {
      key: 'name',
      header: tc.colName,
      render: (row) => (
        <span className="font-medium text-[var(--c-text-primary)]">{row.name}</span>
      ),
    },
    {
      key: 'is_default',
      header: tc.colIsDefault,
      render: (row) =>
        row.is_default ? (
          <Badge variant="success">default</Badge>
        ) : (
          <span className="text-[var(--c-text-muted)]">--</span>
        ),
    },
    {
      key: 'version',
      header: tc.colVersion,
      render: (row) => (
        <span className="tabular-nums text-xs">{row.version}</span>
      ),
    },
    {
      key: 'variables',
      header: tc.colVariablesCount,
      render: (row) => (
        <span className="tabular-nums text-xs">{row.variables.length}</span>
      ),
    },
    {
      key: 'created_at',
      header: tc.colCreatedAt,
      render: (row) => (
        <span className="tabular-nums text-xs">
          {new Date(row.created_at).toLocaleString()}
        </span>
      ),
    },
    {
      key: 'actions',
      header: '',
      render: (row) => (
        <div className="flex items-center gap-1">
          <button
            onClick={(e) => {
              e.stopPropagation()
              handleOpenEdit(row)
            }}
            className="flex items-center justify-center rounded p-1 text-[var(--c-text-muted)] transition-colors hover:bg-[var(--c-bg-sub)] hover:text-[var(--c-text-secondary)]"
            title={tc.modalTitleEdit}
          >
            <Pencil size={13} />
          </button>
          <button
            onClick={(e) => {
              e.stopPropagation()
              setDeleteTarget({ id: row.id, name: row.name })
            }}
            className="flex items-center justify-center rounded p-1 text-[var(--c-text-muted)] transition-colors hover:bg-[var(--c-bg-sub)] hover:text-[var(--c-status-error-text)]"
            title={tc.deleteConfirm}
          >
            <Trash2 size={13} />
          </button>
        </div>
      ),
    },
  ]

  const actions = (
    <button
      onClick={handleOpenCreate}
      className="flex items-center gap-1.5 rounded-lg bg-[var(--c-bg-tag)] px-3 py-1.5 text-xs font-medium text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)]"
    >
      <Plus size={13} />
      {tc.addTemplate}
    </button>
  )

  const inputCls =
    'rounded-lg border border-[var(--c-border)] bg-[var(--c-bg-deep2)] px-3 py-1.5 text-sm text-[var(--c-text-primary)] placeholder:text-[var(--c-text-muted)] focus:outline-none'

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader title={tc.title} actions={actions} />

      <div className="flex flex-1 flex-col overflow-auto">
        <DataTable
          columns={columns}
          data={templates}
          rowKey={(row) => row.id}
          loading={loading}
          emptyMessage={tc.empty}
          emptyIcon={<FileText size={28} />}
        />
      </div>

      <Modal
        open={modalOpen}
        onClose={handleCloseModal}
        title={editTarget ? tc.modalTitleEdit : tc.modalTitleCreate}
        width="560px"
      >
        <div className="flex flex-col gap-4">
          <FormField label={tc.fieldName}>
            <input
              type="text"
              value={form.name}
              onChange={(e) => handleFormField('name', e.target.value)}
              placeholder="my-template"
              className={inputCls}
            />
          </FormField>

          <FormField label={tc.fieldContent}>
            <textarea
              value={form.content}
              onChange={(e) => handleFormField('content', e.target.value)}
              rows={6}
              placeholder="You are a helpful assistant. {{instruction}}"
              className="rounded-lg border border-[var(--c-border)] bg-[var(--c-bg-deep2)] px-3 py-1.5 text-sm font-mono text-[var(--c-text-primary)] placeholder:text-[var(--c-text-muted)] focus:outline-none resize-none"
            />
          </FormField>

          {/* Variables 仅创建时可编辑，编辑时后端不支持更新 */}
          <FormField label={tc.fieldVariables}>
            <input
              type="text"
              value={form.variables}
              onChange={(e) => handleFormField('variables', e.target.value)}
              placeholder="instruction, context"
              disabled={editTarget !== null}
              className={`${inputCls} disabled:opacity-50`}
            />
          </FormField>

          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id="prompt-template-is-default"
              checked={form.is_default}
              onChange={(e) => handleFormField('is_default', e.target.checked)}
              className="h-3.5 w-3.5 rounded"
            />
            <label
              htmlFor="prompt-template-is-default"
              className="text-sm text-[var(--c-text-secondary)]"
            >
              {tc.fieldIsDefault}
            </label>
          </div>

          {formError && (
            <p className="text-xs text-[var(--c-status-error-text)]">{formError}</p>
          )}

          <div className="flex justify-end gap-2 border-t border-[var(--c-border)] pt-3">
            <button
              onClick={handleCloseModal}
              disabled={submitting}
              className="rounded-lg border border-[var(--c-border)] px-3.5 py-1.5 text-sm text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)] disabled:opacity-50"
            >
              {tc.cancel}
            </button>
            <button
              onClick={handleSubmit}
              disabled={submitting}
              className="rounded-lg bg-[var(--c-bg-tag)] px-3.5 py-1.5 text-sm font-medium text-[var(--c-text-primary)] transition-colors hover:bg-[var(--c-bg-sub)] disabled:opacity-50"
            >
              {submitting ? '...' : editTarget ? tc.save : tc.create}
            </button>
          </div>
        </div>
      </Modal>

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        title={tc.deleteTitle}
        message={tc.deleteMessage(deleteTarget?.name ?? '')}
        confirmLabel={tc.deleteConfirm}
        loading={deleting}
      />
    </div>
  )
}
