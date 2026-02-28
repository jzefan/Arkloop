import { useState, useEffect } from 'react'
import { ChevronDown, ChevronRight, Loader2, Check, Search, BookOpen, Sparkles } from 'lucide-react'
import type { WebSource } from '../storage'

export type SearchStep = {
  id: string
  kind: 'planning' | 'searching' | 'reviewing' | 'finished'
  label: string
  status: 'active' | 'done'
  queries?: string[]
}

type Props = {
  steps: SearchStep[]
  sources: WebSource[]
  isComplete: boolean
}

function getDomain(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, '')
  } catch {
    return url
  }
}

function isHttpUrl(url: string): boolean {
  try {
    const p = new URL(url)
    return p.protocol === 'http:' || p.protocol === 'https:'
  } catch {
    return false
  }
}

function StepIcon({ kind, status }: { kind: SearchStep['kind']; status: SearchStep['status'] }) {
  const size = 14
  if (status === 'active') {
    return <Loader2 size={size} className="animate-spin" style={{ color: 'var(--c-accent)' }} />
  }
  switch (kind) {
    case 'planning':
      return <Sparkles size={size} style={{ color: 'var(--c-text-muted)' }} />
    case 'searching':
      return <Search size={size} style={{ color: 'var(--c-text-muted)' }} />
    case 'reviewing':
      return <BookOpen size={size} style={{ color: 'var(--c-text-muted)' }} />
    case 'finished':
      return <Check size={size} style={{ color: 'var(--c-accent)' }} />
  }
}

function QueryPill({ text }: { text: string }) {
  return (
    <span
      style={{
        display: 'inline-block',
        padding: '2px 8px',
        borderRadius: '4px',
        background: 'var(--c-bg-sub)',
        border: '0.5px solid var(--c-border-subtle)',
        fontSize: '12px',
        color: 'var(--c-text-secondary)',
        lineHeight: '18px',
      }}
    >
      {text}
    </span>
  )
}

function SourceCard({ source }: { source: WebSource }) {
  if (!isHttpUrl(source.url)) return null
  const domain = getDomain(source.url)
  return (
    <a
      href={source.url}
      target="_blank"
      rel="noopener noreferrer"
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        padding: '6px 10px',
        borderRadius: '6px',
        background: 'var(--c-bg-sub)',
        border: '0.5px solid var(--c-border-subtle)',
        textDecoration: 'none',
        color: 'inherit',
        transition: 'border-color 0.15s',
        minWidth: 0,
      }}
    >
      <img
        src={`https://www.google.com/s2/favicons?sz=16&domain=${domain}`}
        alt=""
        width={14}
        height={14}
        style={{ flexShrink: 0, borderRadius: '2px' }}
      />
      <span
        style={{
          fontSize: '12px',
          color: 'var(--c-text-primary)',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          flex: 1,
        }}
      >
        {source.title || domain}
      </span>
      <span style={{ fontSize: '11px', color: 'var(--c-text-muted)', flexShrink: 0 }}>{domain}</span>
    </a>
  )
}

export function SearchTimeline({ steps, sources, isComplete }: Props) {
  const [collapsed, setCollapsed] = useState(isComplete)

  // 完成时自动折叠
  useEffect(() => {
    if (isComplete) setCollapsed(true)
  }, [isComplete])

  if (steps.length === 0) return null

  const headerLabel = isComplete
    ? `Reviewed ${sources.length} sources`
    : steps[steps.length - 1]?.label || 'Searching...'

  return (
    <div style={{ maxWidth: '663px' }}>
      {/* 可折叠标题 */}
      <button
        type="button"
        onClick={() => setCollapsed((p) => !p)}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
          padding: '6px 0',
          background: 'none',
          border: 'none',
          cursor: 'pointer',
          color: 'var(--c-text-secondary)',
          fontSize: '13px',
          fontWeight: 500,
        }}
      >
        {!isComplete ? (
          <Loader2 size={13} className="animate-spin" style={{ flexShrink: 0, color: 'var(--c-accent)' }} />
        ) : collapsed ? (
          <ChevronRight size={13} style={{ flexShrink: 0 }} />
        ) : (
          <ChevronDown size={13} style={{ flexShrink: 0 }} />
        )}
        <span>{headerLabel}</span>
      </button>

      {/* 时间轴 */}
      {!collapsed && (
        <div style={{ paddingLeft: '4px', marginTop: '4px' }}>
          {steps.map((step, idx) => {
            const isLast = idx === steps.length - 1
            return (
              <div
                key={step.id}
                style={{
                  display: 'flex',
                  gap: '10px',
                  position: 'relative',
                  paddingBottom: isLast ? 0 : '12px',
                }}
              >
                {/* 竖线 + 图标 */}
                <div
                  style={{
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    width: '16px',
                    flexShrink: 0,
                  }}
                >
                  <div style={{ marginTop: '2px' }}>
                    <StepIcon kind={step.kind} status={step.status} />
                  </div>
                  {!isLast && (
                    <div
                      style={{
                        flex: 1,
                        width: '1px',
                        background: 'var(--c-border-subtle)',
                        marginTop: '4px',
                      }}
                    />
                  )}
                </div>

                {/* 内容 */}
                <div style={{ flex: 1, minWidth: 0, paddingTop: '1px' }}>
                  <div style={{ fontSize: '13px', color: 'var(--c-text-primary)', lineHeight: '18px' }}>
                    {step.label}
                  </div>

                  {/* 搜索关键词 pills */}
                  {step.kind === 'searching' && step.queries && step.queries.length > 0 && (
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: '4px', marginTop: '6px' }}>
                      {step.queries.map((q) => (
                        <QueryPill key={q} text={q} />
                      ))}
                    </div>
                  )}

                  {/* 来源卡片 */}
                  {step.kind === 'reviewing' && step.status === 'done' && sources.length > 0 && (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '4px', marginTop: '6px' }}>
                      {sources.slice(0, 6).map((s, i) => (
                        <SourceCard key={`${s.url}-${i}`} source={s} />
                      ))}
                      {sources.length > 6 && (
                        <span style={{ fontSize: '12px', color: 'var(--c-text-muted)', paddingLeft: '2px' }}>
                          +{sources.length - 6} more
                        </span>
                      )}
                    </div>
                  )}
                </div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
