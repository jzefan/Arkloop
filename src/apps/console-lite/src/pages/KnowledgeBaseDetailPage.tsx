import { useCallback, useEffect, useMemo, useRef, useState, type ChangeEvent } from 'react'
import { useNavigate, useOutletContext, useParams } from 'react-router-dom'
import { ArrowLeft, FileText, RefreshCw, Search, Trash2, Upload } from 'lucide-react'
import {
  deleteDocument,
  getKnowledgeBase,
  listDocuments,
  searchKnowledgeBase,
  uploadDocument,
  type KBDocument,
  type KBDocumentStatus,
  type KnowledgeBase,
  type SearchHit,
} from '../api/knowledge-bases'
import { Badge, type BadgeVariant } from '../components/Badge'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { DataTable, type Column } from '../components/DataTable'
import { ErrorCallout } from '../components/ErrorCallout'
import { PageHeader } from '../components/PageHeader'
import type { LiteOutletContext } from '../layouts/LiteLayout'
import { useLocale } from '../contexts/LocaleContext'

const pollIntervalMs = 3000
const terminalStatuses = new Set<KBDocumentStatus>(['ready', 'failed'])

const inputClassName = 'rounded-lg border border-[var(--c-border)] bg-[var(--c-bg-input)] px-3 py-1.5 text-sm text-[var(--c-text-primary)] placeholder:text-[var(--c-text-muted)] outline-none transition-colors focus:border-[var(--c-border-focus)]'

function statusVariant(status: KBDocumentStatus): BadgeVariant {
  switch (status) {
    case 'ready':
      return 'success'
    case 'failed':
      return 'error'
    case 'queued':
      return 'neutral'
    default:
      return 'warning'
  }
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`
}

function formatDate(value: string, locale: 'zh' | 'en'): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString(locale === 'zh' ? 'zh-CN' : 'en')
}

export function KnowledgeBaseDetailPage() {
  const { accessToken } = useOutletContext<LiteOutletContext>()
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const { locale, t } = useLocale()
  const tk = t.knowledgeBases
  const [kb, setKB] = useState<KnowledgeBase | null>(null)
  const [docs, setDocs] = useState<KBDocument[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [uploading, setUploading] = useState(false)
  const [pendingDelete, setPendingDelete] = useState<KBDocument | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [query, setQuery] = useState('')
  const [k, setK] = useState(8)
  const [hits, setHits] = useState<SearchHit[]>([])
  const [searching, setSearching] = useState(false)
  const [searchError, setSearchError] = useState('')

  const refresh = useCallback(async () => {
    if (!id) return
    setError('')
    try {
      const [kbResult, docsResult] = await Promise.all([
        getKnowledgeBase(accessToken, id),
        listDocuments(accessToken, id),
      ])
      setKB(kbResult)
      setDocs(docsResult)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [accessToken, id])

  useEffect(() => {
    void refresh()
  }, [refresh])

  useEffect(() => {
    if (!docs.some((doc) => !terminalStatuses.has(doc.status))) return
    const handle = window.setInterval(() => void refresh(), pollIntervalMs)
    return () => window.clearInterval(handle)
  }, [docs, refresh])

  const handleUpload = useCallback(async (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (!file || !id || uploading) return
    setUploading(true)
    setError('')
    try {
      await uploadDocument(accessToken, id, file)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setUploading(false)
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }, [accessToken, id, refresh, uploading])

  const handleDelete = useCallback(async () => {
    if (!pendingDelete || !id || deleting) return
    setDeleting(true)
    setError('')
    try {
      await deleteDocument(accessToken, id, pendingDelete.id)
      setPendingDelete(null)
      await refresh()
      setHits((prev) => prev.filter((hit) => hit.document_ref !== pendingDelete.original_filename))
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setDeleting(false)
    }
  }, [accessToken, deleting, id, pendingDelete, refresh])

  const handleSearch = useCallback(async () => {
    const trimmed = query.trim()
    if (!id || !trimmed || searching) return
    setSearching(true)
    setSearchError('')
    try {
      setHits(await searchKnowledgeBase(accessToken, id, trimmed, Math.min(50, Math.max(1, k))))
    } catch (err) {
      setSearchError(err instanceof Error ? err.message : String(err))
      setHits([])
    } finally {
      setSearching(false)
    }
  }, [accessToken, id, k, query, searching])

  const columns = useMemo<Column<KBDocument>[]>(() => [
    {
      key: 'filename',
      header: tk.colFilename,
      cellClassName: 'min-w-[240px]',
      render: (doc) => (
        <div className="flex min-w-0 items-center gap-2">
          <FileText size={14} className="shrink-0 text-[var(--c-text-muted)]" />
          <span className="truncate" title={doc.original_filename}>{doc.original_filename}</span>
        </div>
      ),
    },
    {
      key: 'status',
      header: tk.colStatus,
      render: (doc) => (
        <div className="flex min-w-0 items-center gap-2">
          <Badge variant={statusVariant(doc.status)}>{tk.status[doc.status]}</Badge>
          {doc.error_message && (
            <span className="truncate text-xs text-[var(--c-status-error-text)]" title={doc.error_message}>
              {doc.error_message}
            </span>
          )}
        </div>
      ),
    },
    {
      key: 'chunks',
      header: tk.colChunks,
      cellClassName: 'whitespace-nowrap tabular-nums',
      render: (doc) => String(typeof doc.parse_meta?.chunk_count === 'number' ? doc.parse_meta.chunk_count : '--'),
    },
    {
      key: 'size',
      header: tk.colSize,
      cellClassName: 'whitespace-nowrap',
      render: (doc) => formatBytes(doc.size_bytes),
    },
    {
      key: 'created',
      header: tk.colCreated,
      cellClassName: 'whitespace-nowrap',
      render: (doc) => formatDate(doc.created_at, locale),
    },
    {
      key: 'actions',
      header: tk.colActions,
      cellClassName: 'w-[72px] whitespace-nowrap text-right',
      render: (doc) => (
        <button
          type="button"
          onClick={(event) => {
            event.stopPropagation()
            setPendingDelete(doc)
          }}
          className="inline-flex h-7 w-7 items-center justify-center rounded-md text-[var(--c-text-tertiary)] transition-colors hover:bg-[var(--c-bg-sub)] hover:text-[var(--c-status-error-text)]"
          title={t.common.delete}
        >
          <Trash2 size={14} />
        </button>
      ),
    },
  ], [locale, t.common.delete, tk])

  const actions = (
    <>
      <button
        type="button"
        onClick={() => navigate('/knowledge-bases')}
        className="flex h-7 items-center gap-1 rounded-lg border border-[var(--c-border)] px-2.5 text-[11px] font-medium text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)]"
      >
        <ArrowLeft size={12} />
        {tk.back}
      </button>
      <button
        type="button"
        onClick={() => void refresh()}
        className="flex h-7 items-center gap-1 rounded-lg bg-[var(--c-bg-tag)] px-2.5 text-[11px] font-medium text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)]"
      >
        <RefreshCw size={12} />
        {tk.refresh}
      </button>
    </>
  )

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader title={kb?.name ?? tk.detailTitle} actions={actions} />
      <div className="flex flex-1 flex-col gap-5 overflow-auto px-6 py-4">
        {error && <ErrorCallout error={{ message: error }} locale={locale} requestFailedText={t.requestFailed} />}
        {loading ? (
          <p className="py-12 text-center text-sm text-[var(--c-text-muted)]">{t.common.loading}</p>
        ) : !kb ? (
          <p className="py-12 text-center text-sm text-[var(--c-text-muted)]">{tk.notFound}</p>
        ) : (
          <>
            <section className="flex flex-col gap-3">
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <h3 className="text-sm font-semibold text-[var(--c-text-primary)]">{tk.docsTitle}</h3>
                  <p className="mt-1 truncate text-xs text-[var(--c-text-muted)]">{kb.description || tk.noDescription}</p>
                </div>
                <div>
                  <input
                    ref={fileInputRef}
                    type="file"
                    accept=".txt,.md"
                    onChange={handleUpload}
                    disabled={uploading}
                    className="hidden"
                    id="kb-doc-upload"
                  />
                  <label
                    htmlFor="kb-doc-upload"
                    className="flex h-8 cursor-pointer items-center gap-1 rounded-lg bg-[var(--c-bg-tag)] px-3 text-xs font-medium text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)]"
                  >
                    <Upload size={13} />
                    {uploading ? tk.uploading : tk.upload}
                  </label>
                </div>
              </div>
              <DataTable
                columns={columns}
                data={docs}
                rowKey={(doc) => doc.id}
                emptyMessage={tk.docsEmpty}
                emptyIcon={<FileText size={24} />}
                tableClassName="min-w-[960px]"
              />
            </section>

            <section className="border-t border-[var(--c-border-console)] pt-5">
              <div className="mb-3 flex items-center gap-2">
                <Search size={15} className="text-[var(--c-text-muted)]" />
                <h3 className="text-sm font-semibold text-[var(--c-text-primary)]">{tk.searchTitle}</h3>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <input
                  type="text"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter') void handleSearch()
                  }}
                  placeholder={tk.searchPlaceholder}
                  className={`${inputClassName} min-w-[260px] flex-1`}
                />
                <input
                  type="number"
                  min={1}
                  max={50}
                  value={k}
                  onChange={(event) => setK(Number(event.target.value) || 8)}
                  className={`${inputClassName} w-[80px]`}
                />
                <button
                  type="button"
                  onClick={() => void handleSearch()}
                  disabled={searching || !query.trim()}
                  className="h-8 rounded-lg bg-[var(--c-bg-tag)] px-3 text-xs font-medium text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)] disabled:opacity-50"
                >
                  {searching ? t.common.loading : tk.search}
                </button>
              </div>
              {searchError && (
                <div className="mt-3">
                  <ErrorCallout error={{ message: searchError }} locale={locale} requestFailedText={t.requestFailed} />
                </div>
              )}
              <div className="mt-4 flex flex-col gap-2">
                {hits.map((hit, index) => (
                  <article
                    key={`${hit.document_ref}-${hit.ordinal}-${index}`}
                    className="rounded-lg border border-[var(--c-border-console)] bg-[var(--c-bg-deep2)] p-3"
                  >
                    <div className="mb-1 flex flex-wrap items-center gap-2 text-[11px] text-[var(--c-text-muted)]">
                      <span>{hit.document_ref} · {tk.paragraph(hit.ordinal)}</span>
                      <span>{tk.score}: {hit.score.toFixed(3)}</span>
                      {hit.heading_path.length > 0 && <span>{hit.heading_path.join(' / ')}</span>}
                    </div>
                    <p className="text-sm leading-6 text-[var(--c-text-secondary)]">
                      {hit.text.slice(0, 240)}{hit.text.length > 240 ? '...' : ''}
                    </p>
                  </article>
                ))}
              </div>
            </section>
          </>
        )}
      </div>
      <ConfirmDialog
        open={pendingDelete !== null}
        onClose={() => setPendingDelete(null)}
        onConfirm={handleDelete}
        title={tk.docDeleteTitle}
        message={pendingDelete ? tk.docDeleteConfirm(pendingDelete.original_filename) : ''}
        confirmLabel={t.common.delete}
        loading={deleting}
      />
    </div>
  )
}
