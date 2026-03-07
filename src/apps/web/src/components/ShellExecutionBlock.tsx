import { useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
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

const expandTransition = { duration: 0.25, ease: [0.4, 0, 0.2, 1] as const }

export function ShellExecutionBlock({ code, output, exitCode, isStreaming = false }: Props) {
  const [expanded, setExpanded] = useState(false)
  const [hovered, setHovered] = useState(false)
  const { t } = useLocale()

  const status = resolveStatus(exitCode, isStreaming)
  const preview = extractCommandPreview(code)
  const expandable = !!(code || output || status === 'running')

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        width: 'fit-content',
        maxWidth: '100%',
      }}
    >
      <button
        type="button"
        onClick={() => expandable && setExpanded((p) => !p)}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        title={code?.trim() || undefined}
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: '5px',
          padding: '6px 10px 6px 7px',
          borderRadius: '6px',
          background: expanded || hovered ? 'var(--c-bg-sub)' : 'transparent',
          border: 'none',
          cursor: expandable ? 'pointer' : 'default',
          width: 'fit-content',
          maxWidth: '100%',
          transition: 'background 160ms ease',
        }}
      >
        <motion.div
          animate={{ rotate: expanded ? 90 : 0 }}
          transition={{ duration: 0.2, ease: 'easeOut' }}
          style={{ display: 'flex', flexShrink: 0 }}
        >
          <ChevronRight
            size={13}
            color={hovered || expanded ? 'var(--c-text-secondary)' : 'var(--c-text-muted)'}
            strokeWidth={2}
          />
        </motion.div>
        <span
          style={{
            fontSize: '11px',
            color: expanded ? 'var(--c-text-primary)' : 'var(--c-text-secondary)',
            fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
            whiteSpace: 'nowrap',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            lineHeight: '16px',
          }}
        >
          {preview || t.shellRan}
        </span>
        <StatusBadge status={status} />
      </button>

      <AnimatePresence initial={false}>
        {expanded && (
          <motion.div
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: 'auto', opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={expandTransition}
            style={{ overflow: 'hidden' }}
          >
            <div
              style={{
                marginLeft: '18px',
                marginTop: '1px',
                borderRadius: '6px',
                border: '0.5px solid var(--c-border-subtle)',
                background: 'var(--c-bg-page)',
                padding: '3px 8px',
                maxWidth: 'min(100%, 720px)',
              }}
            >
              {output && output.trim() ? (
                <pre
                  style={{
                    margin: 0,
                    color: status === 'failed' ? '#ef4444' : 'var(--c-text-tertiary)',
                    fontSize: '10.5px',
                    lineHeight: '1.4',
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-word',
                    maxHeight: '240px',
                    overflowY: 'auto',
                    fontFamily: 'ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace',
                  }}
                >
                  {output.trimEnd()}
                </pre>
              ) : status === 'running' ? (
                <div
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    minHeight: '20px',
                    padding: '2px 0',
                  }}
                >
                  <Loader2 size={12} className="animate-spin" style={{ color: 'var(--c-text-muted)' }} />
                </div>
              ) : (
                <div
                  style={{
                    display: 'block',
                    fontSize: '10.5px',
                    lineHeight: '13px',
                    color: 'var(--c-text-muted)',
                    fontStyle: 'italic',
                  }}
                >
                  {t.shellNoOutput}
                </div>
              )}
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  )
}

function StatusBadge({ status }: { status: Status }) {
  const { t } = useLocale()

  if (status === 'running') {
    return (
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: '2px', fontSize: '10px', color: 'var(--c-text-muted)', flexShrink: 0 }}>
        <Loader2 size={10} className="animate-spin" />
      </span>
    )
  }
  if (status === 'failed') {
    return (
      <span style={{ display: 'inline-flex', alignItems: 'center', gap: '2px', fontSize: '10px', color: '#ef4444', flexShrink: 0 }}>
        <XIcon size={10} strokeWidth={2.5} />
        {t.shellFailed}
      </span>
    )
  }
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: '2px', fontSize: '10px', color: 'var(--c-text-muted)', flexShrink: 0 }}>
      <Check size={10} strokeWidth={2.5} />
      {t.shellSuccess}
    </span>
  )
}
