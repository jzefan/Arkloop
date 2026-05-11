import { useCallback, useEffect, useState } from 'react'
import { Search } from 'lucide-react'
import { listAdminUsers, type AdminUser } from '../../api-admin'
import { useLocale } from '../../contexts/LocaleContext'

type Props = {
  accessToken: string
}

export function UsersSettings({ accessToken }: Props) {
  const { t } = useLocale()
  const ut = t.users
  const [users, setUsers] = useState<AdminUser[]>([])
  const [loading, setLoading] = useState(true)
  const [query, setQuery] = useState('')

  const fetchUsers = useCallback(async (q?: string) => {
    setLoading(true)
    try {
      const result = await listAdminUsers(accessToken, { q: q || undefined, limit: 50 })
      setUsers(result)
    } catch {
      setUsers([])
    } finally {
      setLoading(false)
    }
  }, [accessToken])

  useEffect(() => { void fetchUsers() }, [fetchUsers])

  const handleSearch = () => { void fetchUsers(query) }

  return (
    <div className="flex flex-col gap-4 p-4">
      <h2 className="text-[16px] font-medium text-[var(--c-text-heading)]">{ut.title}</h2>

      <div className="flex items-center gap-2">
        <div className="relative flex-1">
          <Search size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--c-text-muted)]" />
          <input
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') handleSearch() }}
            placeholder={ut.searchPlaceholder}
            className="h-8 w-full rounded-lg border border-[var(--c-border)] bg-[var(--c-bg-input)] pl-8 pr-3 text-[13px] text-[var(--c-text-primary)] placeholder:text-[var(--c-text-muted)] focus:border-[var(--c-border-mid)] focus:outline-none"
          />
        </div>
      </div>

      {loading ? (
        <div className="flex items-center justify-center py-8 text-[13px] text-[var(--c-text-muted)]">...</div>
      ) : users.length === 0 ? (
        <div className="flex items-center justify-center py-8 text-[13px] text-[var(--c-text-muted)]">{ut.noUsers}</div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-[var(--c-border)]">
          <table className="w-full text-[13px]">
            <thead>
              <tr className="border-b border-[var(--c-border)] bg-[var(--c-bg-sub)]">
                <th className="px-3 py-2 text-left font-medium text-[var(--c-text-secondary)]">{ut.username}</th>
                <th className="px-3 py-2 text-left font-medium text-[var(--c-text-secondary)]">{ut.email}</th>
                <th className="px-3 py-2 text-left font-medium text-[var(--c-text-secondary)]">{ut.status}</th>
                <th className="px-3 py-2 text-left font-medium text-[var(--c-text-secondary)]">{ut.createdAt}</th>
              </tr>
            </thead>
            <tbody>
              {users.map((user) => (
                <tr key={user.id} className="border-b border-[var(--c-border-subtle)] last:border-b-0">
                  <td className="px-3 py-2 text-[var(--c-text-primary)]">{user.username}</td>
                  <td className="px-3 py-2 text-[var(--c-text-secondary)]">{user.email ?? '—'}</td>
                  <td className="px-3 py-2">
                    <span className={`inline-block rounded px-1.5 py-0.5 text-[11px] font-medium ${
                      user.status === 'active'
                        ? 'bg-[var(--c-status-success)]/10 text-[var(--c-status-success)]'
                        : 'bg-[var(--c-status-error)]/10 text-[var(--c-status-error)]'
                    }`}>
                      {user.status === 'active' ? ut.active : ut.suspended}
                    </span>
                  </td>
                  <td className="px-3 py-2 text-[var(--c-text-tertiary)]">
                    {new Date(user.created_at).toLocaleDateString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
