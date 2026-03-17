import { useRef, useEffect } from 'react'

type Props = {
  html: string
  title: string
  complete: boolean
}

export function WidgetBlock({ html, title, complete }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const lastHtmlRef = useRef('')

  useEffect(() => {
    const el = containerRef.current
    if (!el || html === lastHtmlRef.current) return
    lastHtmlRef.current = html
    el.innerHTML = html
    if (complete) {
      executeScripts(el)
    }
  }, [html, complete])

  return (
    <div style={{ margin: '8px 0', maxWidth: '720px' }}>
      <div style={{
        fontSize: '12px',
        fontWeight: 500,
        color: 'var(--c-text-secondary)',
        marginBottom: '6px',
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
      }}>
        {title}
        {!complete && (
          <span style={{
            width: '6px',
            height: '6px',
            borderRadius: '50%',
            background: 'var(--c-text-tertiary)',
            display: 'inline-block',
            animation: '_fadeIn 0.6s ease infinite alternate',
          }} />
        )}
      </div>
      <div
        ref={containerRef}
        style={{
          border: '0.5px solid var(--c-border-subtle)',
          borderRadius: '10px',
          padding: '16px',
          background: 'var(--c-bg-deep)',
          overflow: 'auto',
        }}
      />
    </div>
  )
}

function executeScripts(container: HTMLElement): void {
  container.querySelectorAll('script').forEach((old) => {
    const s = document.createElement('script')
    if ((old as HTMLScriptElement).src) {
      s.src = (old as HTMLScriptElement).src
    } else {
      s.textContent = old.textContent
    }
    old.replaceWith(s)
  })
}
