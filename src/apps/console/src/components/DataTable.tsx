import { useState, useCallback, type ReactNode } from 'react'
import { ArrowUpDown, ArrowUp, ArrowDown } from 'lucide-react'
import { EmptyState } from './EmptyState'

export type Column<T> = {
  key: string
  header: string
  render: (row: T) => ReactNode
  sortable?: boolean
}

type SortState = {
  key: string
  direction: 'asc' | 'desc'
} | null

type Props<T> = {
  columns: Column<T>[]
  data: T[]
  rowKey: (row: T) => string
  loading?: boolean
  emptyMessage?: string
  emptyIcon?: ReactNode
  onSort?: (key: string, direction: 'asc' | 'desc') => void
}

export function DataTable<T>({
  columns,
  data,
  rowKey,
  loading = false,
  emptyMessage,
  emptyIcon,
  onSort,
}: Props<T>) {
  const [sort, setSort] = useState<SortState>(null)

  const handleSort = useCallback(
    (key: string) => {
      setSort((prev) => {
        const direction = prev?.key === key && prev.direction === 'asc' ? 'desc' : 'asc'
        onSort?.(key, direction)
        return { key, direction }
      })
    },
    [onSort],
  )

  if (loading) {
    return (
      <div className="flex flex-1 items-center justify-center py-16">
        <p className="text-sm text-[var(--c-text-muted)]">Loading...</p>
      </div>
    )
  }

  if (data.length === 0) {
    return <EmptyState icon={emptyIcon} message={emptyMessage} />
  }

  return (
    <div className="overflow-auto">
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-[var(--c-border-console)]">
            {columns.map((col) => (
              <th
                key={col.key}
                className={[
                  'whitespace-nowrap px-4 py-2.5 text-xs font-medium text-[var(--c-text-muted)]',
                  col.sortable ? 'cursor-pointer select-none' : '',
                ].join(' ')}
                onClick={col.sortable ? () => handleSort(col.key) : undefined}
              >
                <span className="inline-flex items-center gap-1">
                  {col.header}
                  {col.sortable && (
                    <span className="opacity-40">
                      {sort?.key === col.key ? (
                        sort.direction === 'asc' ? <ArrowUp size={12} /> : <ArrowDown size={12} />
                      ) : (
                        <ArrowUpDown size={12} />
                      )}
                    </span>
                  )}
                </span>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {data.map((row) => (
            <tr
              key={rowKey(row)}
              className="border-b border-[var(--c-border-console)] transition-colors hover:bg-[var(--c-bg-sub)]"
            >
              {columns.map((col) => (
                <td key={col.key} className="whitespace-nowrap px-4 py-2.5 text-[var(--c-text-secondary)]">
                  {col.render(row)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
