import { useState, useEffect, useCallback, useRef } from 'react'
import { createPortal } from 'react-dom'
import { X, Download, ExternalLink, Copy, ZoomIn, ZoomOut, RotateCcw } from 'lucide-react'
import { apiBaseUrl } from '@arkloop/shared/api'
import type { ArtifactRef } from '../storage'

const ANIM_MS = 120

type Props = {
  artifact: ArtifactRef
  accessToken: string
  pathPrefix?: string
}

export function ArtifactImage({ artifact, accessToken, pathPrefix = '/v1/artifacts' }: Props) {
  const [blobUrl, setBlobUrl] = useState<string | null>(null)
  const [error, setError] = useState(false)
  const [loading, setLoading] = useState(true)
  const [visible, setVisible] = useState(false)
  const [show, setShow] = useState(false)
  const [scale, setScale] = useState(1)
  const [offset, setOffset] = useState({ x: 0, y: 0 })
  const [dragging, setDragging] = useState(false)
  const closingTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const dragStartRef = useRef<{ x: number; y: number; offsetX: number; offsetY: number } | null>(null)

  useEffect(() => {
    let cancelled = false
    const url = `${apiBaseUrl()}${pathPrefix}/${artifact.key}`

    fetch(url, {
      headers: { Authorization: `Bearer ${accessToken}` },
    })
      .then((res) => {
        if (!res.ok) throw new Error(`${res.status}`)
        return res.blob()
      })
      .then((blob) => {
        if (cancelled) return
        const url = URL.createObjectURL(blob)
        setBlobUrl(url)
        setLoading(false)
      })
      .catch(() => {
        if (cancelled) return
        setError(true)
        setLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [artifact.key, accessToken, pathPrefix])

  useEffect(() => {
    return () => {
      if (blobUrl) URL.revokeObjectURL(blobUrl)
    }
  }, [blobUrl])

  useEffect(() => {
    return () => {
      if (closingTimer.current) clearTimeout(closingTimer.current)
    }
  }, [])

  const openLightbox = useCallback(() => {
    if (closingTimer.current) clearTimeout(closingTimer.current)
    setScale(1)
    setOffset({ x: 0, y: 0 })
    setVisible(true)
    requestAnimationFrame(() => requestAnimationFrame(() => setShow(true)))
  }, [])

  const closeLightbox = useCallback(() => {
    setShow(false)
    closingTimer.current = setTimeout(() => setVisible(false), ANIM_MS)
  }, [])

  useEffect(() => {
    if (!visible) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') closeLightbox()
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [visible, closeLightbox])

  const handleOverlayClick = useCallback(
    (e: React.MouseEvent) => {
      if (e.target === e.currentTarget) closeLightbox()
    },
    [closeLightbox],
  )

  const handleDownload = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation()
      if (!blobUrl) return
      const a = document.createElement('a')
      a.href = blobUrl
      a.download = artifact.filename
      a.click()
    },
    [blobUrl, artifact.filename],
  )

  const handleCopyImage = useCallback(
    async (e: React.MouseEvent) => {
      e.stopPropagation()
      if (!blobUrl || !navigator.clipboard?.write) return
      try {
        const res = await fetch(blobUrl)
        const blob = await res.blob()
        const mime = blob.type && blob.type !== '' ? blob.type : 'image/png'
        await navigator.clipboard.write([new ClipboardItem({ [mime]: blob })])
      } catch {
        // Clipboard support depends on browser permissions and image MIME support.
      }
    },
    [blobUrl],
  )

  const zoomBy = useCallback((delta: number) => {
    setScale((current) => Math.min(6, Math.max(0.25, Number((current + delta).toFixed(2)))))
  }, [])

  const resetView = useCallback((e?: React.MouseEvent) => {
    e?.stopPropagation()
    setScale(1)
    setOffset({ x: 0, y: 0 })
  }, [])

  const handleWheel = useCallback((e: React.WheelEvent) => {
    e.preventDefault()
    zoomBy(e.deltaY > 0 ? -0.2 : 0.2)
  }, [zoomBy])

  const startDrag = useCallback((e: React.MouseEvent) => {
    e.stopPropagation()
    if (scale <= 1) return
    setDragging(true)
    dragStartRef.current = { x: e.clientX, y: e.clientY, offsetX: offset.x, offsetY: offset.y }
  }, [offset.x, offset.y, scale])

  const moveDrag = useCallback((e: React.MouseEvent) => {
    if (!dragging || !dragStartRef.current) return
    const start = dragStartRef.current
    setOffset({ x: start.offsetX + e.clientX - start.x, y: start.offsetY + e.clientY - start.y })
  }, [dragging])

  const endDrag = useCallback(() => {
    setDragging(false)
    dragStartRef.current = null
  }, [])

  if (error) return null
  if (loading) {
    return (
      <div
        style={{
          width: '100%',
          maxWidth: 'min(60%, 480px)',
          height: '200px',
          borderRadius: '10px',
          background: 'var(--c-bg-sub)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'var(--c-text-tertiary)',
          fontSize: '13px',
        }}
      />
    )
  }

  const transition = `all ${ANIM_MS}ms ease-out`

  return (
    <>
      <div
        style={{
          display: 'inline-block',
          maxWidth: 'min(60%, 520px)',
          border: '0.5px solid var(--c-border-subtle)',
          borderRadius: '12px',
          padding: '8px',
        }}
      >
        <img loading="lazy" decoding="async"
          src={blobUrl!}
          alt={artifact.filename}
          draggable={false}
          onClick={openLightbox}
          style={{
            maxWidth: '100%',
            width: '100%',
            display: 'block',
            borderRadius: '6px',
            cursor: 'default',
          }}
        />
      </div>
      {visible && createPortal(
        <div
          onClick={handleOverlayClick}
          style={{
            position: 'fixed',
            inset: 0,
            zIndex: 9999,
            background: show ? 'var(--c-lightbox-overlay)' : 'transparent',
            backdropFilter: show ? 'blur(12px)' : 'blur(0px)',
            WebkitBackdropFilter: show ? 'blur(12px)' : 'blur(0px)',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            justifyContent: 'center',
            cursor: 'default',
            transition,
          }}
        >
          <button
            onClick={closeLightbox}
            className="flex h-7 w-7 items-center justify-center rounded-lg transition-colors hover:bg-[var(--c-bg-deep)]"
            style={{
              position: 'absolute',
              top: 16,
              right: 16,
              border: 'none',
              background: 'transparent',
              color: 'var(--c-text-muted)',
              cursor: 'pointer',
              opacity: show ? 1 : 0,
              transition,
            }}
          >
            <X size={16} />
          </button>

          <div
            onWheel={handleWheel}
            onMouseMove={moveDrag}
            onMouseUp={endDrag}
            onMouseLeave={endDrag}
            style={{
              maxWidth: '90vw',
              maxHeight: 'calc(90vh - 64px)',
              overflow: 'hidden',
              cursor: scale > 1 ? (dragging ? 'grabbing' : 'grab') : 'zoom-in',
              opacity: show ? 1 : 0,
              transition: `opacity ${ANIM_MS}ms ease-out`,
            }}
          >
            <img loading="lazy" decoding="async"
              src={blobUrl!}
              alt={artifact.filename}
              draggable={false}
              onMouseDown={startDrag}
              onDoubleClick={(e) => {
                e.stopPropagation()
                if (scale === 1) zoomBy(1)
                else resetView()
              }}
              style={{
                maxWidth: '90vw',
                maxHeight: 'calc(90vh - 64px)',
                borderRadius: '8px',
                display: 'block',
                transform: `${show ? 'scale(1)' : 'scale(0.94)'} translate(${offset.x / Math.max(scale, 1)}px, ${offset.y / Math.max(scale, 1)}px) scale(${scale})`,
                transformOrigin: 'center',
                transition: dragging ? 'none' : transition,
              }}
            />
          </div>

          <div
            onClick={(e) => e.stopPropagation()}
            style={{
              marginTop: 16,
              display: 'flex',
              alignItems: 'center',
              gap: 6,
              cursor: 'default',
              transform: show ? 'translateY(0)' : 'translateY(6px)',
              opacity: show ? 1 : 0,
              transition,
            }}
          >
            <a
              href={blobUrl!}
              target="_blank"
              rel="noopener noreferrer"
              draggable={false}
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                gap: 8,
                padding: '8px 14px',
                borderRadius: 10,
                border: '0.5px solid var(--c-border-subtle)',
                background: 'var(--c-bg-sub)',
                color: 'var(--c-text-primary)',
                fontSize: 14,
                textDecoration: 'none',
                fontFamily: 'inherit',
                transition: 'background 150ms',
              }}
              onMouseEnter={(e) => {
                ;(e.currentTarget as HTMLAnchorElement).style.background = 'var(--c-bg-deep)'
              }}
              onMouseLeave={(e) => {
                ;(e.currentTarget as HTMLAnchorElement).style.background = 'var(--c-bg-sub)'
              }}
            >
              <span
                style={{
                  maxWidth: 220,
                  overflow: 'hidden',
                  textOverflow: 'ellipsis',
                  whiteSpace: 'nowrap',
                }}
              >
                {artifact.filename}
              </span>
              <ExternalLink size={14} style={{ color: 'var(--c-text-icon)', flexShrink: 0 }} />
            </a>
            <button
              onClick={handleCopyImage}
              title="Copy image"
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                justifyContent: 'center',
                width: 36,
                height: 36,
                borderRadius: 10,
                border: '0.5px solid var(--c-border-subtle)',
                background: 'var(--c-bg-sub)',
                color: 'var(--c-text-icon)',
                cursor: 'pointer',
                fontFamily: 'inherit',
                transition: 'background 150ms',
              }}
              onMouseEnter={(e) => {
                ;(e.currentTarget as HTMLButtonElement).style.background = 'var(--c-bg-deep)'
              }}
              onMouseLeave={(e) => {
                ;(e.currentTarget as HTMLButtonElement).style.background = 'var(--c-bg-sub)'
              }}
            >
              <Copy size={16} />
            </button>
            <button
              onClick={(e) => { e.stopPropagation(); zoomBy(-0.25) }}
              title="Zoom out"
              style={mediaButtonStyle}
            >
              <ZoomOut size={16} />
            </button>
            <button
              onClick={(e) => { e.stopPropagation(); zoomBy(0.25) }}
              title="Zoom in"
              style={mediaButtonStyle}
            >
              <ZoomIn size={16} />
            </button>
            <button
              onClick={resetView}
              title="Reset view"
              style={mediaButtonStyle}
            >
              <RotateCcw size={16} />
            </button>
            <button
              onClick={handleDownload}
              title="Download"
              style={{
                display: 'inline-flex',
                alignItems: 'center',
                justifyContent: 'center',
                width: 36,
                height: 36,
                borderRadius: 10,
                border: '0.5px solid var(--c-border-subtle)',
                background: 'var(--c-bg-sub)',
                color: 'var(--c-text-icon)',
                cursor: 'pointer',
                fontFamily: 'inherit',
                transition: 'background 150ms',
              }}
              onMouseEnter={(e) => {
                ;(e.currentTarget as HTMLButtonElement).style.background = 'var(--c-bg-deep)'
              }}
              onMouseLeave={(e) => {
                ;(e.currentTarget as HTMLButtonElement).style.background = 'var(--c-bg-sub)'
              }}
            >
              <Download size={16} />
            </button>
          </div>
        </div>,
        document.body,
      )}
    </>
  )
}

const mediaButtonStyle: React.CSSProperties = {
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  width: 36,
  height: 36,
  borderRadius: 10,
  border: '0.5px solid var(--c-border-subtle)',
  background: 'var(--c-bg-sub)',
  color: 'var(--c-text-icon)',
  cursor: 'pointer',
  fontFamily: 'inherit',
  transition: 'background 150ms',
}
