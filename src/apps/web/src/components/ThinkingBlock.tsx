import { useState } from 'react'
import { ChevronDown, ChevronRight, Loader2 } from 'lucide-react'
import { MarkdownRenderer } from './MarkdownRenderer'

type Props = {
  kind: string
  label: string
  mode: 'visible' | 'collapsed' | 'hidden'
  content: string
  isStreaming?: boolean
}

export function ThinkingBlock({ kind: _kind, label, mode, content, isStreaming }: Props) {
  const [expanded, setExpanded] = useState(false)

  if (mode === 'hidden') return null

  if (mode === 'visible') {
    return (
      <div style={{ maxWidth: '663px' }}>
        <MarkdownRenderer content={content} disableMath />
      </div>
    )
  }

  // collapsed mode
  return (
    <div
      style={{
        borderRadius: '8px',
        border: '0.5px solid var(--c-border-subtle)',
        background: 'var(--c-bg-sub)',
        overflow: 'hidden',
        maxWidth: '663px',
      }}
    >
      <button
        type="button"
        onClick={() => setExpanded((prev) => !prev)}
        style={{
          width: '100%',
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
          padding: '8px 12px',
          background: 'none',
          border: 'none',
          cursor: 'pointer',
          color: 'var(--c-text-secondary)',
          fontSize: '13px',
        }}
      >
        {isStreaming ? (
          <Loader2 size={13} className="animate-spin" style={{ flexShrink: 0, color: 'var(--c-text-muted)' }} />
        ) : expanded ? (
          <ChevronDown size={13} style={{ flexShrink: 0 }} />
        ) : (
          <ChevronRight size={13} style={{ flexShrink: 0 }} />
        )}
        <span style={{ textAlign: 'left' }}>{label}</span>
      </button>

      {expanded && content && (
        <div
          style={{
            padding: '0 12px 10px',
            borderTop: '0.5px solid var(--c-border-subtle)',
            paddingTop: '8px',
          }}
        >
          <MarkdownRenderer content={content} disableMath />
        </div>
      )}
    </div>
  )
}
