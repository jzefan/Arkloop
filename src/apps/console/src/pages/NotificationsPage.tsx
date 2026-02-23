import { useState, useCallback, useEffect } from 'react'
import { useOutletContext } from 'react-router-dom'
import { RefreshCw, Bell, Check } from 'lucide-react'
import type { ConsoleOutletContext } from '../layouts/ConsoleLayout'
import { PageHeader } from '../components/PageHeader'
import { DataTable, type Column } from '../components/DataTable'
import { Badge } from '../components/Badge'
import { useToast } from '../components/useToast'
import {
  listNotifications,
  markNotificationRead,
  type Notification,
} from '../api/notifications'

function typeBadgeVariant(type: string) {
  switch (type) {
    case 'error': return 'error' as const
    case 'warning': return 'warning' as const
    case 'success': return 'success' as const
    default: return 'neutral' as const
  }
}

export function NotificationsPage() {
  const { accessToken, refreshUnreadCount } = useOutletContext<ConsoleOutletContext>()
  const { addToast } = useToast()

  const [notifications, setNotifications] = useState<Notification[]>([])
  const [loading, setLoading] = useState(false)
  const [markingIds, setMarkingIds] = useState<Set<string>>(new Set())

  const fetchNotifications = useCallback(async () => {
    setLoading(true)
    try {
      const resp = await listNotifications(accessToken)
      setNotifications(resp.data)
    } catch {
      addToast('Failed to load notifications', 'error')
    } finally {
      setLoading(false)
    }
  }, [accessToken, addToast])

  useEffect(() => {
    void fetchNotifications()
  }, [fetchNotifications])

  const handleMarkRead = useCallback(
    async (id: string) => {
      setMarkingIds((prev) => new Set(prev).add(id))
      try {
        await markNotificationRead(id, accessToken)
        setNotifications((prev) => prev.filter((n) => n.id !== id))
        refreshUnreadCount()
      } catch {
        addToast('Failed to mark notification as read', 'error')
      } finally {
        setMarkingIds((prev) => {
          const next = new Set(prev)
          next.delete(id)
          return next
        })
      }
    },
    [accessToken, addToast, refreshUnreadCount],
  )

  const columns: Column<Notification>[] = [
    {
      key: 'type',
      header: 'Type',
      render: (row) => <Badge variant={typeBadgeVariant(row.type)}>{row.type}</Badge>,
    },
    {
      key: 'title',
      header: 'Title',
      render: (row) => <span className="text-xs font-medium">{row.title}</span>,
    },
    {
      key: 'body',
      header: 'Body',
      render: (row) => (
        <span className="text-xs text-[var(--c-text-muted)]">{row.body || '--'}</span>
      ),
    },
    {
      key: 'created_at',
      header: 'Created At',
      render: (row) => (
        <span className="text-xs tabular-nums">
          {new Date(row.created_at).toLocaleString()}
        </span>
      ),
    },
    {
      key: 'actions',
      header: '',
      render: (row) => (
        <button
          onClick={() => void handleMarkRead(row.id)}
          disabled={markingIds.has(row.id)}
          className="flex items-center gap-1 rounded px-2 py-0.5 text-xs text-[var(--c-text-muted)] transition-colors hover:bg-[var(--c-bg-sub)] hover:text-[var(--c-text-secondary)] disabled:opacity-50"
        >
          <Check size={12} />
          Mark Read
        </button>
      ),
    },
  ]

  const actions = (
    <button
      onClick={() => void fetchNotifications()}
      disabled={loading}
      className="flex items-center gap-1.5 rounded-lg border border-[var(--c-border)] px-2.5 py-1.5 text-xs text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)] disabled:opacity-50"
    >
      <RefreshCw size={13} className={loading ? 'animate-spin' : ''} />
      Refresh
    </button>
  )

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader title="Notifications" actions={actions} />
      <div className="flex flex-1 flex-col overflow-auto">
        <DataTable
          columns={columns}
          data={notifications}
          rowKey={(row) => row.id}
          loading={loading}
          emptyMessage="No unread notifications"
          emptyIcon={<Bell size={28} />}
        />
      </div>
    </div>
  )
}
