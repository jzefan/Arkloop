import { useState } from 'react'
import { ChevronDown, ChevronRight, Loader2, Code2, Terminal } from 'lucide-react'
import { MarkdownRenderer } from './MarkdownRenderer'

export type CodeExecution = {
  id: string
  language: 'python' | 'shell'
  code?: string
  output?: string
  exitCode?: number
}

export function CodeExecutionCard({ language, code, output, exitCode }: {
  language: 'python' | 'shell'
  code?: string
  output?: string
  exitCode?: number
}) {
  const [expanded, setExpanded] = useState(false)
  const isPython = language === 'python'
  const hasDetail = !!(code || output)
  const failed = exitCode != null && exitCode !== 0

  return (
    <div
      style={{
        borderRadius: '8px',
        border: '0.5px solid var(--c-border-subtle)',
        background: 'var(--c-bg-page)',
        width: 'fit-content',
        minWidth: expanded ? '100%' : undefined,
        maxWidth: '100%',
      }}
    >
      <button
        type="button"
        onClick={() => hasDetail && setExpanded((p) => !p)}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '10px',
          padding: '8px 12px',
          background: 'none',
          border: 'none',
          cursor: hasDetail ? 'pointer' : 'default',
          width: '100%',
        }}
      >
        <div
          style={{
            width: '34px',
            height: '34px',
            borderRadius: '7px',
            background: isPython ? '#0e9f8e' : '#6366f1',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            flexShrink: 0,
          }}
        >
          {isPython
            ? <Code2 size={17} color="#fff" strokeWidth={2} />
            : <Terminal size={17} color="#fff" strokeWidth={2} />
          }
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: '1px', textAlign: 'left' }}>
          <span style={{ fontSize: '13px', fontWeight: 500, color: 'var(--c-text-primary)', lineHeight: '16px' }}>
            {isPython ? 'Python' : 'Shell'}
          </span>
          <span style={{ fontSize: '11px', color: 'var(--c-text-muted)', lineHeight: '14px' }}>
            Code
          </span>
        </div>
        {hasDetail && (
          <div style={{ marginLeft: 'auto', color: 'var(--c-text-muted)', flexShrink: 0 }}>
            {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          </div>
        )}
      </button>

      {expanded && hasDetail && (
        <div style={{ borderTop: '0.5px solid var(--c-border-subtle)', padding: '8px 12px 10px', overflow: 'auto' }}>
          {code && (
            <MarkdownRenderer
              content={'```' + (isPython ? 'python' : 'bash') + '\n' + code + '\n```'}
              disableMath
            />
          )}
          {output && (
            <div style={{ marginTop: code ? '8px' : '0' }}>
              <div style={{ fontSize: '11px', color: 'var(--c-text-muted)', marginBottom: '4px', fontWeight: 500 }}>
                {failed ? 'stderr' : 'stdout'}
              </div>
              <pre
                style={{
                  margin: 0,
                  padding: '8px 10px',
                  borderRadius: '6px',
                  background: 'var(--c-bg-deep)',
                  color: failed ? '#ef4444' : 'var(--c-text-secondary)',
                  fontSize: '12px',
                  lineHeight: '1.5',
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-word',
                  maxHeight: '300px',
                  overflow: 'auto',
                }}
              >
                {output}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

type Props = {
  kind: string
  label: string
  mode: 'visible' | 'collapsed' | 'hidden'
  content: string
  isStreaming?: boolean
  codeExecutions?: CodeExecution[]
}

export function ThinkingBlock({ kind: _kind, label, mode, content, isStreaming, codeExecutions }: Props) {
  const [expanded, setExpanded] = useState(false)

  if (mode === 'hidden') return null

  if (mode === 'visible') {
    return (
      <div style={{ maxWidth: '663px' }}>
        <MarkdownRenderer content={content} disableMath />
        {codeExecutions && codeExecutions.length > 0 && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', marginTop: '10px' }}>
            {codeExecutions.map((ce) => (
              <CodeExecutionCard key={ce.id} language={ce.language} code={ce.code} output={ce.output} exitCode={ce.exitCode} />
            ))}
          </div>
        )}
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

      {expanded && (content || (codeExecutions && codeExecutions.length > 0)) && (
        <div
          style={{
            padding: '0 12px 10px',
            borderTop: '0.5px solid var(--c-border-subtle)',
            paddingTop: '8px',
          }}
        >
          {content && <MarkdownRenderer content={content} disableMath />}
          {codeExecutions && codeExecutions.length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', marginTop: content ? '10px' : '0' }}>
              {codeExecutions.map((ce) => (
                <CodeExecutionCard key={ce.id} language={ce.language} code={ce.code} output={ce.output} exitCode={ce.exitCode} />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
