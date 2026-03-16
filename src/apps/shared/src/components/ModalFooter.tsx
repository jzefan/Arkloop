import type { ReactNode } from 'react'

type Props = {
  onCancel: () => void
  onConfirm: () => void
  cancelLabel?: string
  confirmLabel?: string
  loading?: boolean
  confirmDisabled?: boolean
  confirmVariant?: 'primary' | 'destructive'
  children?: ReactNode
}

export function ModalFooter({
  onCancel,
  onConfirm,
  cancelLabel = 'Cancel',
  confirmLabel = 'Save',
  loading = false,
  confirmDisabled = false,
  confirmVariant = 'primary',
  children,
}: Props) {
  const confirmCls =
    confirmVariant === 'destructive'
      ? 'rounded-lg bg-red-600 px-3.5 py-1.5 text-sm font-medium text-white transition-colors hover:bg-red-700 disabled:opacity-50'
      : 'rounded-lg bg-[var(--c-bg-tag)] px-3.5 py-1.5 text-sm font-medium text-[var(--c-text-primary)] transition-colors hover:bg-[var(--c-bg-sub)] disabled:opacity-50'

  return (
    <div className="flex items-center justify-end gap-2 border-t border-[var(--c-border)] pt-3">
      {children}
      <button
        type="button"
        onClick={onCancel}
        disabled={loading}
        className="rounded-lg border border-[var(--c-border)] px-3.5 py-1.5 text-sm text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)] disabled:opacity-50"
      >
        {cancelLabel}
      </button>
      <button
        type="button"
        onClick={onConfirm}
        disabled={loading || confirmDisabled}
        className={confirmCls}
      >
        {loading ? '…' : confirmLabel}
      </button>
    </div>
  )
}
