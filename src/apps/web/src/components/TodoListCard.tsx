import { useState } from 'react'
import { ChevronDown, ChevronRight, Circle, CircleCheck, CircleDotDashed, CircleX, ListTodo } from 'lucide-react'
import type { TodoItemRef, TodoWriteRef } from '../copSegmentTimeline'
import { useLocale } from '../contexts/LocaleContext'

type Props = {
  todo: TodoWriteRef
}

function statusIcon(status: TodoItemRef['status']) {
  switch (status) {
    case 'completed':
      return <CircleCheck size={15} />
    case 'in_progress':
      return <CircleDotDashed size={15} />
    case 'cancelled':
      return <CircleX size={15} />
    case 'pending':
      return <Circle size={15} />
  }
}

function statusColor(status: TodoItemRef['status']): string {
  switch (status) {
    case 'completed':
      return 'var(--c-status-success-text)'
    case 'in_progress':
      return 'var(--c-text-primary)'
    case 'cancelled':
      return 'var(--c-status-error-text)'
    case 'pending':
      return 'var(--c-text-muted)'
  }
}

export function TodoListCard({ todo }: Props) {
  const { t } = useLocale()
  const [expanded, setExpanded] = useState(true)
  const completed = todo.todos.filter((item) => item.status === 'completed').length
  const total = todo.todos.length
  const failed = todo.status === 'failed'

  return (
    <div style={{ maxWidth: 'min(100%, 760px)', padding: '4px 0' }}>
      <div
        style={{
          borderRadius: 8,
          background: 'var(--c-attachment-bg)',
          border: '0.5px solid var(--c-border-subtle)',
          overflow: 'hidden',
        }}
      >
        <button
          type="button"
          aria-expanded={expanded}
          onClick={() => setExpanded((value) => !value)}
          style={{
            width: '100%',
            minWidth: 0,
            border: 'none',
            background: 'transparent',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            gap: 10,
            padding: '8px 10px',
            color: 'var(--c-text-secondary)',
            fontSize: 'var(--c-cop-row-font-size)',
            lineHeight: 'var(--c-cop-row-line-height)',
          }}
        >
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 7, minWidth: 0 }}>
            <ListTodo size={15} style={{ flexShrink: 0, color: failed ? 'var(--c-status-error-text)' : 'var(--c-text-muted)' }} />
            <span style={{ color: 'var(--c-text-primary)', fontWeight: 460 }}>{t.todoListTitle}</span>
            {total > 0 && (
              <span style={{ color: failed ? 'var(--c-status-error-text)' : 'var(--c-text-muted)', whiteSpace: 'nowrap' }}>
                {t.todoListProgress(completed, total)}
              </span>
            )}
          </span>
          {expanded
            ? <ChevronDown size={14} style={{ flexShrink: 0, color: 'var(--c-text-muted)' }} />
            : <ChevronRight size={14} style={{ flexShrink: 0, color: 'var(--c-text-muted)' }} />
          }
        </button>
        <div
          style={{
            display: 'grid',
            gridTemplateRows: expanded ? '1fr' : '0fr',
            transition: 'grid-template-rows 0.24s cubic-bezier(0.4, 0, 0.2, 1)',
          }}
        >
          <div style={{ minHeight: 0, overflow: 'hidden' }}>
            <div style={{ padding: '0 8px 8px' }}>
              {todo.todos.map((item) => {
                const muted = item.status === 'completed' || item.status === 'cancelled'
                return (
                  <div
                    key={item.id}
                    style={{
                      display: 'flex',
                      alignItems: 'flex-start',
                      gap: 8,
                      padding: '6px 2px',
                      color: muted ? 'var(--c-text-muted)' : 'var(--c-text-primary)',
                      borderTop: '0.5px solid var(--c-border-subtle)',
                    }}
                  >
                    <span style={{ display: 'inline-flex', paddingTop: 2, color: statusColor(item.status), flexShrink: 0 }}>
                      {statusIcon(item.status)}
                    </span>
                    <span
                      style={{
                        minWidth: 0,
                        flex: 1,
                        overflowWrap: 'anywhere',
                        fontSize: 'var(--c-cop-row-font-size)',
                        lineHeight: 'var(--c-cop-row-line-height)',
                        textDecoration: muted ? 'line-through' : 'none',
                        textDecorationColor: 'var(--c-text-muted)',
                      }}
                    >
                      {item.content}
                    </span>
                  </div>
                )
              })}
              {failed && todo.errorMessage && (
                <div
                  style={{
                    padding: '6px 2px 0',
                    color: 'var(--c-status-error-text)',
                    fontSize: 12,
                    lineHeight: '18px',
                    borderTop: total > 0 ? '0.5px solid var(--c-border-subtle)' : undefined,
                  }}
                >
                  {todo.errorMessage}
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
