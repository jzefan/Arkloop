import { useState, useCallback, useEffect } from 'react'
import { useOutletContext } from 'react-router-dom'
import { RefreshCw, AlertTriangle } from 'lucide-react'
import type { ConsoleOutletContext } from '../../layouts/ConsoleLayout'
import { PageHeader } from '../../components/PageHeader'
import { EmptyState } from '../../components/EmptyState'
import { useToast } from '../../components/useToast'
import { useLocale } from '../../contexts/LocaleContext'
import { listReports, type Report } from '../../api/reports'

const PAGE_SIZE = 50

const CATEGORY_COLORS: Record<string, string> = {
  inaccurate: 'bg-amber-500/15 text-amber-400',
  out_of_date: 'bg-blue-500/15 text-blue-400',
  too_short: 'bg-purple-500/15 text-purple-400',
  too_long: 'bg-purple-500/15 text-purple-400',
  harmful_or_offensive: 'bg-red-500/15 text-red-400',
  wrong_sources: 'bg-orange-500/15 text-orange-400',
}

function CategoryBadge({ category }: { category: string }) {
  const color = CATEGORY_COLORS[category] ?? 'bg-gray-500/15 text-gray-400'
  return (
    <span className={`inline-block rounded-md px-1.5 py-0.5 text-[11px] font-medium ${color}`}>
      {category.replace(/_/g, ' ')}
    </span>
  )
}

function truncateId(id: string): string {
  return id.length > 8 ? id.slice(0, 8) : id
}

export function ReportsPage() {
  const { accessToken } = useOutletContext<ConsoleOutletContext>()
  const { addToast } = useToast()
  const { t } = useLocale()
  const p = t.pages.reports

  const [reports, setReports] = useState<Report[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [offset, setOffset] = useState(0)

  const fetchReports = useCallback(
    async (currentOffset: number) => {
      setLoading(true)
      try {
        const resp = await listReports(
          { limit: PAGE_SIZE, offset: currentOffset },
          accessToken,
        )
        setReports(resp.data)
        setTotal(resp.total)
      } catch {
        addToast(p.toastLoadFailed, 'error')
      } finally {
        setLoading(false)
      }
    },
    [accessToken, addToast, p.toastLoadFailed],
  )

  useEffect(() => {
    void fetchReports(offset)
  }, [fetchReports, offset])

  const handleRefresh = useCallback(() => {
    void fetchReports(offset)
  }, [fetchReports, offset])

  const totalPages = Math.ceil(total / PAGE_SIZE)
  const currentPage = Math.floor(offset / PAGE_SIZE) + 1

  const actions = (
    <button
      onClick={handleRefresh}
      disabled={loading}
      className="flex items-center gap-1.5 rounded-lg border border-[var(--c-border)] px-2.5 py-1.5 text-xs text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)] disabled:opacity-50"
    >
      <RefreshCw size={13} className={loading ? 'animate-spin' : ''} />
      {p.refresh}
    </button>
  )

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader title={p.title} actions={actions} />

      <div className="flex flex-1 flex-col overflow-auto">
        {loading ? (
          <div className="flex flex-1 items-center justify-center py-16">
            <p className="text-sm text-[var(--c-text-muted)]">Loading...</p>
          </div>
        ) : reports.length === 0 ? (
          <EmptyState icon={<AlertTriangle size={28} />} message={p.empty} />
        ) : (
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-[var(--c-border-console)]">
                <th className="whitespace-nowrap px-4 py-2.5 text-xs font-medium text-[var(--c-text-muted)]">{p.colCreatedAt}</th>
                <th className="whitespace-nowrap px-4 py-2.5 text-xs font-medium text-[var(--c-text-muted)]">{p.colReporter}</th>
                <th className="whitespace-nowrap px-4 py-2.5 text-xs font-medium text-[var(--c-text-muted)]">{p.colThread}</th>
                <th className="whitespace-nowrap px-4 py-2.5 text-xs font-medium text-[var(--c-text-muted)]">{p.colCategories}</th>
                <th className="whitespace-nowrap px-4 py-2.5 text-xs font-medium text-[var(--c-text-muted)]">{p.colFeedback}</th>
              </tr>
            </thead>
            <tbody>
              {reports.map((r) => (
                <tr
                  key={r.id}
                  className="border-b border-[var(--c-border-console)] transition-colors hover:bg-[var(--c-bg-sub)]"
                >
                  <td className="whitespace-nowrap px-4 py-2.5 text-[var(--c-text-secondary)]">
                    <span className="text-xs tabular-nums">
                      {new Date(r.created_at).toLocaleString()}
                    </span>
                  </td>
                  <td className="whitespace-nowrap px-4 py-2.5 text-[var(--c-text-secondary)]">
                    <span className="text-xs">{r.reporter_email}</span>
                  </td>
                  <td className="whitespace-nowrap px-4 py-2.5 text-[var(--c-text-secondary)]">
                    <span className="font-mono text-xs" title={r.thread_id}>
                      {truncateId(r.thread_id)}
                    </span>
                  </td>
                  <td className="px-4 py-2.5">
                    <div className="flex flex-wrap gap-1">
                      {r.categories.map((c) => (
                        <CategoryBadge key={c} category={c} />
                      ))}
                    </div>
                  </td>
                  <td className="max-w-[300px] truncate px-4 py-2.5 text-[var(--c-text-secondary)]">
                    <span className="text-xs">{r.feedback ?? '--'}</span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {totalPages > 1 && (
        <div className="flex items-center justify-between border-t border-[var(--c-border-console)] px-4 py-2">
          <span className="text-xs text-[var(--c-text-muted)]">
            {offset + 1}–{Math.min(offset + PAGE_SIZE, total)} of {total}
          </span>
          <div className="flex gap-2">
            <button
              onClick={() => setOffset((prev) => Math.max(0, prev - PAGE_SIZE))}
              disabled={currentPage <= 1}
              className="rounded border border-[var(--c-border)] px-2.5 py-1 text-xs text-[var(--c-text-secondary)] disabled:opacity-40 hover:bg-[var(--c-bg-sub)]"
            >
              {p.prev}
            </button>
            <span className="flex items-center text-xs text-[var(--c-text-muted)]">
              {currentPage} / {totalPages}
            </span>
            <button
              onClick={() => setOffset((prev) => prev + PAGE_SIZE)}
              disabled={currentPage >= totalPages}
              className="rounded border border-[var(--c-border)] px-2.5 py-1 text-xs text-[var(--c-text-secondary)] disabled:opacity-40 hover:bg-[var(--c-bg-sub)]"
            >
              {p.next}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
