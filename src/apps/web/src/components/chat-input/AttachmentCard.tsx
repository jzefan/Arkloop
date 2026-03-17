import { useState, useEffect } from 'react'
import { X } from 'lucide-react'
import type { Attachment } from '../ChatInput'

export const BAR_COUNT = 52

export function hasTransferFiles(dataTransfer?: DataTransfer | null): boolean {
  if (!dataTransfer) return false
  const types = Array.from(dataTransfer.types ?? [])
  if (types.includes('Files')) return true
  if ((dataTransfer.files?.length ?? 0) > 0) return true
  if (Array.from(dataTransfer.items ?? []).some((item) => item.kind === 'file')) return true
  // Electron: clipboard images from screenshots/apps may only expose image/* types
  if (types.some((t) => t.startsWith('image/'))) return true
  return false
}

export function extractFilesFromTransfer(dataTransfer?: DataTransfer | null): File[] {
  if (!dataTransfer) return []
  const files: File[] = []
  const seenTypes = new Set<string>()

  const items = Array.from(dataTransfer.items ?? [])

  // Prefer items API (supports clipboard images in Electron)
  const itemFiles = items
    .filter((item) => item.kind === 'file')
    .map((item) => item.getAsFile())
    .filter((f): f is File => f != null)

  const dtFiles = Array.from(dataTransfer.files ?? [])

  const allFiles = itemFiles.length > 0 ? itemFiles : dtFiles

  if (allFiles.length > 0) {
    for (const file of allFiles) {
      const prefix = file.type.split('/')[0]
      if (prefix === 'image') {
        if (seenTypes.has('image')) continue
        seenTypes.add('image')
      }
      files.push(file)
    }
    return files
  }

  // Electron fallback: clipboard image items may be typed image/* with kind 'file'
  // but getAsFile() returned null. Try to build a Blob from the DataTransferItem.
  // This handles cases where the clipboard image kind check passes but file is null.
  for (const item of items) {
    if (!item.type.startsWith('image/')) continue
    if (seenTypes.has('image')) continue
    const file = item.getAsFile()
    if (file) {
      seenTypes.add('image')
      files.push(file)
    }
  }

  return files
}

export function isEditableElement(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  if (target.isContentEditable) return true
  const tagName = target.tagName
  return tagName === 'INPUT' || tagName === 'TEXTAREA' || tagName === 'SELECT'
}

export function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export function AttachmentCard({ attachment, onRemove }: { attachment: Attachment; onRemove: () => void }) {
  const [imageLoaded, setImageLoaded] = useState(false)
  const [lineCount, setLineCount] = useState<number | null>(null)
  const [cardHovered, setCardHovered] = useState(false)
  const isImage = attachment.mime_type.startsWith('image/')

  useEffect(() => {
    if (isImage) return
    const reader = new FileReader()
    reader.onload = (e) => {
      const text = e.target?.result as string
      setLineCount(text.split('\n').length)
    }
    reader.readAsText(attachment.file)
  }, [attachment.file, isImage])

  const ext = attachment.name.includes('.')
    ? attachment.name.split('.').pop()!.toUpperCase()
    : ''
  const uploading = attachment.status === 'uploading'
  const ready = !uploading && (isImage ? imageLoaded : lineCount !== null)

  return (
    <div style={{ position: 'relative', flexShrink: 0 }}
      onMouseEnter={() => setCardHovered(true)}
      onMouseLeave={() => setCardHovered(false)}
    >
      <div
        style={{
          width: '120px',
          height: '120px',
          borderRadius: '10px',
          background: 'var(--c-attachment-bg)',
          overflow: 'hidden',
          borderWidth: '0.7px',
          borderStyle: 'solid',
          borderColor: cardHovered ? 'var(--c-attachment-border-hover)' : 'var(--c-attachment-border)',
          transition: 'border-color 0.2s ease',
        }}
      >
        {!ready && (
          <div style={{
            position: 'absolute', inset: 0, padding: '10px',
            display: 'flex', flexDirection: 'column', gap: '8px',
          }}>
            <div className="attachment-shimmer" style={{ width: '80%', height: '10px', borderRadius: '5px' }} />
            <div className="attachment-shimmer" style={{ width: '55%', height: '10px', borderRadius: '5px' }} />
            <div style={{ flex: 1 }} />
            <div className="attachment-shimmer" style={{ width: '30%', height: '10px', borderRadius: '5px' }} />
          </div>
        )}

        {isImage ? (
          <img
            src={attachment.preview_url}
            alt={attachment.name}
            onLoad={() => setImageLoaded(true)}
            style={{
              width: '100%',
              height: '100%',
              objectFit: 'cover',
              opacity: ready ? 1 : 0,
              transition: 'opacity 0.2s ease',
              display: 'block',
            }}
          />
        ) : (
          <div style={{
            padding: '10px',
            display: 'flex', flexDirection: 'column',
            height: '100%',
            opacity: ready ? 1 : 0,
            transition: 'opacity 0.2s ease',
          }}>
            <span style={{
              color: 'var(--c-text-heading)',
              fontSize: '12px',
              fontWeight: 300,
              lineHeight: '1.35',
              wordBreak: 'break-all',
              display: '-webkit-box',
              WebkitLineClamp: 3,
              WebkitBoxOrient: 'vertical',
              overflow: 'hidden',
            }}>
              {attachment.name}
            </span>
            {lineCount !== null && (
              <span style={{ color: 'var(--c-text-muted)', fontSize: '11px', marginTop: '3px' }}>
                {lineCount} lines
              </span>
            )}
            <div style={{ flex: 1 }} />
            {ext && (
              <span style={{
                alignSelf: 'flex-start',
                padding: '2px 6px',
                borderRadius: '5px',
                background: 'var(--c-attachment-bg)',
                border: '0.5px solid var(--c-attachment-badge-border)',
                color: 'var(--c-text-secondary)',
                fontSize: '10px',
                fontWeight: 500,
              }}>
                {ext}
              </span>
            )}
          </div>
        )}
      </div>

      <button
        type="button"
        className="attachment-close-btn"
        onClick={(e) => { e.stopPropagation(); onRemove() }}
        style={{
          position: 'absolute',
          top: '-5px',
          left: '-5px',
          width: '18px',
          height: '18px',
          borderRadius: '50%',
          background: 'var(--c-attachment-close-bg)',
          border: '0.5px solid var(--c-attachment-close-border)',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          cursor: 'pointer',
          opacity: cardHovered ? 1 : 0,
          transition: 'opacity 0.15s ease',
          pointerEvents: cardHovered ? 'auto' : 'none',
          zIndex: 1,
        }}
      >
        <X size={9} />
      </button>
    </div>
  )
}
