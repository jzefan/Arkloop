import { useEffect, useCallback, useRef, type ReactNode, type MouseEvent } from 'react'
import { createPortal } from 'react-dom'
import { X } from 'lucide-react'

type Props = {
  open: boolean
  onClose: () => void
  title?: string
  children: ReactNode
  width?: string
  /** Accessible label when no `title` is rendered. */
  ariaLabel?: string
}

const FOCUSABLE = 'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"]),[contenteditable="true"]'

export function Modal({ open, onClose, title, children, width = '480px', ariaLabel }: Props) {
  const overlayRef = useRef<HTMLDivElement>(null)
  const dialogRef = useRef<HTMLDivElement>(null)
  const previousActiveRef = useRef<Element | null>(null)

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation()
        onClose()
        return
      }
      if (e.key !== 'Tab') return
      const dialog = dialogRef.current
      if (!dialog) return
      const focusables = Array.from(dialog.querySelectorAll<HTMLElement>(FOCUSABLE))
        .filter((el) => !el.hasAttribute('disabled') && el.tabIndex !== -1)
      if (focusables.length === 0) {
        e.preventDefault()
        return
      }
      const first = focusables[0]
      const last = focusables[focusables.length - 1]
      const active = document.activeElement
      if (e.shiftKey && active === first) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && active === last) {
        e.preventDefault()
        first.focus()
      }
    },
    [onClose],
  )

  useEffect(() => {
    if (!open) return
    previousActiveRef.current = document.activeElement
    document.addEventListener('keydown', handleKeyDown, true)
    requestAnimationFrame(() => {
      const dialog = dialogRef.current
      if (!dialog) return
      const target = dialog.querySelector<HTMLElement>(FOCUSABLE) ?? dialog
      if (target instanceof HTMLElement) target.focus({ preventScroll: true })
    })
    return () => {
      document.removeEventListener('keydown', handleKeyDown, true)
      const prev = previousActiveRef.current
      if (prev instanceof HTMLElement) prev.focus({ preventScroll: true })
    }
  }, [open, handleKeyDown])

  const handleOverlayClick = useCallback(
    (e: MouseEvent<HTMLDivElement>) => {
      if (e.target === overlayRef.current) onClose()
    },
    [onClose],
  )

  if (!open) return null

  return createPortal(
    <div
      ref={overlayRef}
      onClick={handleOverlayClick}
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-label={title ?? ariaLabel}
        tabIndex={-1}
        className="modal-enter flex max-h-[85vh] flex-col rounded-[14px]"
        style={{
          background: 'var(--c-bg-page)',
          border: '0.5px solid var(--c-border-subtle)',
          width: `min(${width}, calc(100vw - 40px))`,
        }}
      >
        {title && (
          <div className="flex items-center justify-between px-5 py-4" style={{ borderBottom: '0.5px solid var(--c-border-subtle)' }}>
            <h3 className="text-[15px] font-semibold text-[var(--c-text-heading)]">{title}</h3>
            <button
              type="button"
              aria-label="Close"
              onClick={onClose}
              className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-[var(--c-text-muted)] transition-colors hover:bg-[var(--c-bg-deep)] hover:text-[var(--c-text-secondary)]"
            >
              <X size={16} />
            </button>
          </div>
        )}
        <div className="flex-1 overflow-y-auto px-5 py-4">{children}</div>
      </div>
    </div>,
    document.body,
  )
}
