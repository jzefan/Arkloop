import { useCallback, useEffect, useRef, useState } from 'react'
import { ArtifactIframe, type ArtifactAction, type ArtifactIframeHandle } from './ArtifactIframe'

type Props = {
  html: string
  title: string
  complete: boolean
  onAction?: (action: ArtifactAction) => void
}

export function WidgetBlock({ html, title, complete, onAction }: Props) {
  const iframeRef = useRef<ArtifactIframeHandle>(null)
  const lastRenderRef = useRef<{ html: string; complete: boolean } | null>(null)
  const [runtimeError, setRuntimeError] = useState<string | null>(null)

  useEffect(() => {
    if (!html) return
    const previous = lastRenderRef.current
    if (previous?.html === html && previous.complete === complete) return
    lastRenderRef.current = { html, complete }
    if (complete) {
      iframeRef.current?.finalizeContent(html)
      return
    }
    iframeRef.current?.setStreamingContent(html)
  }, [html, complete])

  useEffect(() => {
    setRuntimeError(null)
  }, [html])

  const handleAction = useCallback((action: ArtifactAction) => {
    if (action.type === 'error') {
      setRuntimeError(action.message)
    }
    onAction?.(action)
  }, [onAction])

  return (
    <div style={{ margin: '2px 0 4px', maxWidth: '720px' }}>
      <ArtifactIframe
        ref={iframeRef}
        mode="streaming"
        frameTitle={title}
        onAction={handleAction}
        style={{
          minHeight: '120px',
          border: 'none',
          borderRadius: '0',
          background: 'transparent',
        }}
      />
      {runtimeError && (
        <div style={{
          marginTop: '6px',
          fontSize: '12px',
          color: 'var(--c-status-error-text)',
        }}>
          {runtimeError}
        </div>
      )}
    </div>
  )
}
