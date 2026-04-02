import { useState } from 'react'
import { Bug } from 'lucide-react'
import { DebugPanel } from './DebugPanel'

export function DebugTrigger() {
  const [open, setOpen] = useState(false)
  const [hovered, setHovered] = useState(false)

  const bg = open
    ? 'var(--c-btn-bg)'
    : hovered
      ? 'var(--c-bg-sub)'
      : 'var(--c-bg-page)'

  const fg = open
    ? 'var(--c-btn-text)'
    : hovered
      ? 'var(--c-text-secondary)'
      : 'var(--c-text-muted)'

  return (
    <>
      <button
        onClick={() => setOpen(!open)}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        className="fixed right-4 bottom-4 z-[9998] flex items-center justify-center rounded-full"
        style={{
          width: 36,
          height: 36,
          background: bg,
          color: fg,
          border: '0.5px solid var(--c-border-subtle)',
          transition: 'background 150ms, color 150ms',
        }}
      >
        <Bug size={16} />
      </button>
      <DebugPanel open={open} onClose={() => setOpen(false)} />
    </>
  )
}
