import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate, useOutletContext } from 'react-router-dom'
import { BookOpen, Plus, RefreshCw, Trash2 } from 'lucide-react'
import {
  deleteKnowledgeBase,
  getDefaultKBWorkspace,
  listKnowledgeBases,
  type KnowledgeBase,
} from '../api/knowledge-bases'
import { Badge } from '../components/Badge'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { CreateKBModal } from '../components/CreateKBModal'
import { DataTable, type Column } from '../components/DataTable'
import { ErrorCallout } from '../components/ErrorCallout'
import { PageHeader } from '../components/PageHeader'
import type { LiteOutletContext } from '../layouts/LiteLayout'
import { useLocale } from '../contexts/LocaleContext'

function formatDate(value: string, locale: 'zh' | 'en'): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString(locale === 'zh' ? 'zh-CN' : 'en')
}

function shortRef(value: string): string {
  return value.length > 18 ? `${value.slice(0, 18)}...` : value
}

export function KnowledgeBasesPage() {
  const { accessToken } = useOutletContext<LiteOutletContext>()
  const navigate = useNavigate()
  const { locale, t } = useLocale()
  const tk = t.knowledgeBases
  const [workspaceRef, setWorkspaceRef] = useState('')
  const [items, setItems] = useState<KnowledgeBase[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [pendingDelete, setPendingDelete] = useState<KnowledgeBase | null>(null)
  const [deleting, setDeleting] = useState(false)

  const refresh = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const ws = workspaceRef || await getDefaultKBWorkspace(accessToken)
      setWorkspaceRef(ws)
      setItems(await listKnowledgeBases(accessToken, ws))
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [accessToken, workspaceRef])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const handleDelete = useCallback(async () => {
    if (!pendingDelete || deleting) return
    setDeleting(true)
    setError('')
    try {
      await deleteKnowledgeBase(accessToken, pendingDelete.id)
      setPendingDelete(null)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setDeleting(false)
    }
  }, [accessToken, deleting, pendingDelete, refresh])

  const columns = useMemo<Column<KnowledgeBase>[]>(() => [
    {
      key: 'name',
      header: tk.colName,
      cellClassName: 'min-w-[220px]',
      render: (row) => (
        <div className="flex min-w-0 flex-col gap-1">
          <button
            type="button"
            onClick={() => navigate(`/knowledge-bases/${row.id}`)}
            className="truncate text-left text-sm font-medium text-[var(--c-text-primary)] hover:text-[var(--c-accent)]"
          >
            {row.name}
          </button>
          {row.description && (
            <span className="line-clamp-1 text-xs text-[var(--c-text-muted)]">{row.description}</span>
          )}
        </div>
      ),
    },
    {
      key: 'workspace',
      header: tk.colWorkspace,
      cellClassName: 'whitespace-nowrap font-mono text-[11px]',
      render: (row) => <span title={row.workspace_ref}>{shortRef(row.workspace_ref)}</span>,
    },
    {
      key: 'docs',
      header: tk.colDocs,
      cellClassName: 'whitespace-nowrap tabular-nums',
      render: (row) => String(row.document_count ?? 0),
    },
    {
      key: 'mode',
      header: tk.colMode,
      render: (row) => (
        <div className="flex flex-wrap gap-1">
          {row.visibility === 'private' && <Badge variant="neutral">私有</Badge>}
          <Badge variant={row.integration_mode === 'exam' ? 'success' : 'neutral'}>{row.integration_mode === 'exam' ? '已绑定 exam 范围' : 'standalone'}</Badge>
        </div>
      ),
    },
    {
      key: 'created',
      header: tk.colCreated,
      cellClassName: 'whitespace-nowrap',
      render: (row) => formatDate(row.created_at, locale),
    },
    {
      key: 'actions',
      header: tk.colActions,
      cellClassName: 'w-[96px] whitespace-nowrap text-right',
      render: (row) => (
        <button
          type="button"
          onClick={(event) => {
            event.stopPropagation()
            setPendingDelete(row)
          }}
          className="inline-flex h-7 w-7 items-center justify-center rounded-md text-[var(--c-text-tertiary)] transition-colors hover:bg-[var(--c-bg-sub)] hover:text-[var(--c-status-error-text)]"
          title={t.common.delete}
        >
          <Trash2 size={14} />
        </button>
      ),
    },
  ], [locale, navigate, t.common.delete, tk])

  const actions = (
    <>
      <button
        type="button"
        onClick={() => void refresh()}
        disabled={loading}
        className="flex h-7 items-center gap-1 rounded-lg bg-[var(--c-bg-tag)] px-2.5 text-[11px] font-medium text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)] disabled:opacity-50"
      >
        <RefreshCw size={12} className={loading ? 'animate-spin' : ''} />
        {tk.refresh}
      </button>
      <button
        type="button"
        onClick={() => setCreateOpen(true)}
        className="flex h-7 items-center gap-1 rounded-lg bg-[var(--c-bg-tag)] px-2.5 text-[11px] font-medium text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)]"
      >
        <Plus size={12} />
        {tk.create}
      </button>
    </>
  )

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader title={tk.title} actions={actions} />
      <div className="flex flex-1 flex-col overflow-hidden">
        {error && (
          <div className="px-6 pt-4">
            <ErrorCallout error={{ message: error }} locale={locale} requestFailedText={t.requestFailed} />
          </div>
        )}
        <div className="flex-1 overflow-auto px-6 py-4">
          <div className="mb-3 flex items-center justify-between gap-3">
            <p className="truncate text-xs text-[var(--c-text-muted)]">
              {workspaceRef ? tk.workspaceHint(shortRef(workspaceRef)) : tk.workspaceResolving}
            </p>
          </div>
          <DataTable
            columns={columns}
            data={items}
            rowKey={(row) => row.id}
            loading={loading}
            loadingLabel={t.common.loading}
            emptyMessage={tk.empty}
            emptyIcon={<BookOpen size={24} />}
            onRowClick={(row) => navigate(`/knowledge-bases/${row.id}`)}
            tableClassName="min-w-[900px]"
          />
        </div>
      </div>
      <CreateKBModal
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={(id) => navigate(`/knowledge-bases/${id}`)}
        accessToken={accessToken}
        workspaceRef={workspaceRef || undefined}
      />
      <ConfirmDialog
        open={pendingDelete !== null}
        onClose={() => setPendingDelete(null)}
        onConfirm={handleDelete}
        title={tk.deleteTitle}
        message={pendingDelete ? tk.deleteConfirm(pendingDelete.name) : ''}
        confirmLabel={t.common.delete}
        loading={deleting}
      />
    </div>
  )
}
