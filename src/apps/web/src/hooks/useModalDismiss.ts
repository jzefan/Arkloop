import { useEffect, useRef } from 'react'

type Options = {
  /** Whether the modal is currently open. When false, the hook is inert. */
  open: boolean
  /** Called when the user presses Escape or otherwise dismisses. */
  onDismiss: () => void
  /** Element to receive initial focus. If omitted, focuses the first focusable in the container. */
  initialFocus?: React.RefObject<HTMLElement | null>
}

const FOCUSABLE_SELECTOR = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
  '[contenteditable="true"]',
].join(',')

/**
 * Adds Escape-to-dismiss, focus-trap, and focus-restore to a modal/overlay.
 *
 * Returns a ref to attach to the modal container. While `open`, focus stays
 * inside the container; on Escape, `onDismiss` is called; on close, focus
 * returns to the element that was active when the modal opened.
 */
export function useModalDismiss({ open, onDismiss, initialFocus }: Options) {
  const containerRef = useRef<HTMLElement | null>(null)
  const previousActiveRef = useRef<Element | null>(null)

  useEffect(() => {
    if (!open) return
    previousActiveRef.current = document.activeElement
    const container = containerRef.current
    const target = initialFocus?.current
      ?? container?.querySelector<HTMLElement>(FOCUSABLE_SELECTOR)
      ?? container
    if (target instanceof HTMLElement) {
      // Defer to next frame so portal children are mounted and focusable.
      requestAnimationFrame(() => target.focus({ preventScroll: true }))
    }
    return () => {
      const prev = previousActiveRef.current
      if (prev instanceof HTMLElement) prev.focus({ preventScroll: true })
    }
  }, [open, initialFocus])

  useEffect(() => {
    if (!open) return
    const handleKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.stopPropagation()
        onDismiss()
        return
      }
      if (event.key !== 'Tab') return
      const container = containerRef.current
      if (!container) return
      const focusables = Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR))
        .filter((el) => !el.hasAttribute('disabled') && el.tabIndex !== -1)
      if (focusables.length === 0) {
        event.preventDefault()
        return
      }
      const first = focusables[0]
      const last = focusables[focusables.length - 1]
      const active = document.activeElement
      if (event.shiftKey && active === first) {
        event.preventDefault()
        last.focus()
      } else if (!event.shiftKey && active === last) {
        event.preventDefault()
        first.focus()
      }
    }
    document.addEventListener('keydown', handleKey, true)
    return () => document.removeEventListener('keydown', handleKey, true)
  }, [open, onDismiss])

  return containerRef
}
