import { useState, useEffect, useCallback, useRef } from 'react'
import { X, Check } from 'lucide-react'
import { listNotifications, markNotificationRead, type NotificationItem } from '../api'
import { useLocale } from '../contexts/LocaleContext'

type Props = {
  accessToken: string
  onClose: () => void
  onMarkedRead: () => void
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  const y = d.getFullYear()
  const m = d.getMonth() + 1
  const day = d.getDate()
  return `${y}年${m}月${day}日`
}

export function NotificationsPanel({ accessToken, onClose, onMarkedRead }: Props) {
  const { t } = useLocale()
  const [items, setItems] = useState<NotificationItem[]>([])
  const [loading, setLoading] = useState(true)
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    return () => { mountedRef.current = false }
  }, [])

  const fetchNotifications = useCallback(async () => {
    try {
      const resp = await listNotifications(accessToken, 'broadcast')
      if (mountedRef.current) {
        setItems(resp.data ?? [])
      }
    } catch {
      // 静默处理
    } finally {
      if (mountedRef.current) setLoading(false)
    }
  }, [accessToken])

  useEffect(() => {
    void fetchNotifications()
  }, [fetchNotifications])

  const handleMarkRead = useCallback(async (id: string) => {
    try {
      await markNotificationRead(accessToken, id)
      setItems((prev) => prev.filter((n) => n.id !== id))
      onMarkedRead()
    } catch {
      // 静默处理
    }
  }, [accessToken, onMarkedRead])

  return (
    <div className="absolute inset-0 z-30 flex flex-col overflow-hidden bg-[var(--c-bg-page)]">
      {/* 关闭按钮 */}
      <div className="flex items-center justify-end px-4 py-3">
        <button
          onClick={onClose}
          className="flex h-8 w-8 items-center justify-center rounded-lg text-[var(--c-text-tertiary)] transition-colors hover:bg-[var(--c-bg-deep)] hover:text-[var(--c-text-secondary)]"
        >
          <X size={20} />
        </button>
      </div>

      {/* 标题 */}
      <div className="flex flex-col items-center gap-6 px-4 pb-6">
        <h1 className="text-2xl font-semibold text-[var(--c-text-primary)]">
          {t.notificationsTitle}
        </h1>
      </div>

      {/* 通知列表 */}
      <div className="flex-1 overflow-y-auto">
        <div className="mx-auto w-full max-w-[720px] px-6">
          {loading ? (
            <div className="flex items-center justify-center py-20">
              <span className="text-sm text-[var(--c-text-muted)]">{t.loading}</span>
            </div>
          ) : items.length === 0 ? (
            <div className="flex items-center justify-center py-20">
              <span className="text-sm text-[var(--c-text-muted)]">{t.notificationsEmpty}</span>
            </div>
          ) : (
            items.map((n) => (
              <div
                key={n.id}
                className="flex items-start gap-8 border-b border-[var(--c-border)] py-6"
              >
                <span className="mt-0.5 shrink-0 text-sm text-[var(--c-text-muted)]">
                  {formatDate(n.created_at)}
                </span>
                <div className="min-w-0 flex-1">
                  <p className="text-base font-semibold text-[var(--c-text-primary)]">{n.title}</p>
                  {n.body && (
                    <p className="mt-1.5 text-sm text-[var(--c-text-muted)]">{n.body}</p>
                  )}
                </div>
                <button
                  onClick={() => void handleMarkRead(n.id)}
                  className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-[var(--c-text-muted)] transition-colors hover:bg-[var(--c-bg-deep)] hover:text-[var(--c-text-secondary)]"
                  title={t.notificationsMarkRead}
                >
                  <Check size={16} />
                </button>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
