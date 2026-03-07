import { useState } from 'react'
import { ChevronRight, Check, X as XIcon, Loader2 } from 'lucide-react'
import { useLocale } from '../contexts/LocaleContext'

type Props = {
  code?: string
  output?: string
  exitCode?: number
  isStreaming?: boolean
}

function extractCommandPreview(code: string | undefined): string {
  if (!code) return ''
  const first = code.split('\n')[0].trim()
  return first.length > 72 ? first.slice(0, 72) + '...' : first
}

type Status = 'running' | 'success' | 'failed'

function resolveStatus(exitCode: number | undefined, isStreaming: boolean): Status {
  if (exitCode != null) return exitCode === 0 ? 'success' : 'failed'
  if (isStreaming) return 'running'
  return 'success'
}

export function ShellExecutionBlock({ code, output, exitCode, isStreaming = false }: Props) {
  const [expanded, setExpanded] = useState(false)
  const { t } = useLocale()

  const status = resolveStatus(exitCode, isStreaming)
  const preview = extractCommandPreview(code)
  const expandable = !!(code || output)

  return (
    <div
      style={{
        borderRadius: '7px',
        border: '0.5px solid var(--c-border-subtle)',
        background: 'var(--c-bg-page)',
        width: 'fit-content',
        maxWidth: '100%',
      }}
    >
      {/* header */}
      <button
        type="button"
        onClick={() => expandable && setExpanded((p) => !p)}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
          padding: '6px 10px',
          background: 'none',
          border: 'none',
          cursor: expandable ? 'pointer' : 'default',
          width: '100%',
        }}
      >
        <ChevronRight
          size={13}
          color="var(--c-text-muted)"
          strokeWidth={2}
          style={{
            flexShrink: 0,
            transition: 'transform 200ms ease',
            transform: expanded ? 'rotate(90deg)' : 'rotate(0deg)',
          }}
        />
        <span
          style={{
            fontSize: '12px',
            color: 'var(--c-text-secondary)',
            fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
            whiteSpace: 'nowrap',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
          }}
        >
          {preview || t.shellRan}
        </span>
        <StatusBadge status={status} />
      </button>

      {/* collapsible body */}
      <div
        style={{
          display: 'grid',
          gridTemplateRows: expanded ? '1fr' : '0fr',
          transition: 'grid-template-rows 200ms ease',
        }}
      >
        <div style={{ overflow: 'hidden' }}>
          <div style={{ padding: '0 10px 8px' }}>
            {output && output.trim() ? (
              <pre
                style={{
                  margin: 0,
                  color: status === 'failed' ? '#ef4444' : 'var(--c-text-tertiary)',
                  fontSize: '11px',
                  lineHeight: '1.5',
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-word',
                  maxHeight: '240px',
                  overflowY: 'auto',
                  fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
                }}
              >
                {output}
              </pre>
            ) : status !== 'running' ? (
              <span
                style={{
                  fontSize: '11px',
                  color: 'var(--c-text-muted)',
                  fontStyle: 'italic',
                }}
              >
                {t.shellNoOutput}
              </span>
            ) : null}
          </div>
        </div>
      </div>
    </div>
  )
}

function StatusBadge({ status }: { status: Status }) {
  const { t } = useLocale()

  if (status === 'running') {
    return (
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: '3px', fontSize: '11px', color: 'var(--c-text-muted)', flexShrink: 0 }}>
        <Loader2 size={11} className="animate-spin" />
      </span>
    )
  }
  if (status === 'failed') {
    return (
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: '3px', fontSize: '11px', color: '#ef4444', flexShrink: 0 }}>
        <XIcon size={11} strokeWidth={2.5} />
        {t.shellFailed}
      </span>
    )
  }
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: '3px', fontSize: '11px', color: 'var(--c-text-muted)', flexShrink: 0 }}>
      <Check size={11} strokeWidth={2.5} />
      {t.shellSuccess}
    </span>
  )
}
