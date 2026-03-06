import { Modal } from './Modal'
import { useLocale } from '../contexts/LocaleContext'

type Props = {
  open: boolean
  onClose: () => void
  onConfirm: () => void
  title?: string
  message: string
  confirmLabel?: string
  loading?: boolean
}

export function ConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  message,
  confirmLabel,
  loading = false,
}: Props) {
  const { t } = useLocale()
  return (
    <Modal open={open} onClose={onClose} title={title ?? t.common.confirm} width="400px">
      <p className="text-sm text-[var(--c-text-secondary)]">{message}</p>
      <div className="mt-5 flex justify-end gap-2">
        <button
          onClick={onClose}
          className="rounded-lg border border-[var(--c-border)] px-3.5 py-1.5 text-sm text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)]"
        >
          {t.common.cancel}
        </button>
        <button
          onClick={onConfirm}
          disabled={loading}
          className="rounded-lg bg-red-600 px-3.5 py-1.5 text-sm font-medium text-white transition-colors hover:bg-red-700 disabled:opacity-50"
        >
          {loading ? '...' : (confirmLabel ?? t.common.confirm)}
        </button>
      </div>
    </Modal>
  )
}
