import { useCallback, useEffect, useState } from 'react'
import { Download, ExternalLink } from 'lucide-react'
import { apiBaseUrl } from '@arkloop/shared/api'
import type { ArtifactRef } from '../storage'

type Props = {
  artifact: ArtifactRef
  accessToken: string
  pathPrefix?: string
}

function blobPart(bytes: Uint8Array): BlobPart {
  const copy = new Uint8Array(bytes.length)
  copy.set(bytes)
  return copy.buffer
}

export function ArtifactVideo({ artifact, accessToken, pathPrefix = '/v1/artifacts' }: Props) {
  const [blobUrl, setBlobUrl] = useState<string | null>(null)
  const [progress, setProgress] = useState<number | null>(null)
  const [error, setError] = useState(false)

  useEffect(() => {
    let cancelled = false
    const url = `${apiBaseUrl()}${pathPrefix}/${artifact.key}`

    const load = async () => {
      let res: Response
      try {
        res = await fetch(url, { headers: { Authorization: `Bearer ${accessToken}` } })
        if (!res.ok) throw new Error(`${res.status}`)
      } catch {
        if (!cancelled) setError(true)
        return
      }
      const contentLength = Number(res.headers.get('content-length') ?? '0')
      const reader = res.body!.getReader()
      const chunks: Uint8Array[] = []
      let received = 0
      try {
        while (true) {
          const { done, value } = await reader.read()
          if (done) break
          if (cancelled) return
          chunks.push(value)
          received += value.length
          if (contentLength > 0 && !cancelled) {
            setProgress(Math.round((received / contentLength) * 100))
          }
        }
      } catch {
        if (!cancelled) setError(true)
        return
      }
      if (!cancelled) {
        const mimeType = res.headers.get('content-type') ?? undefined
        const blob = new Blob(chunks.map(blobPart), { type: mimeType })
        setBlobUrl(URL.createObjectURL(blob))
        setProgress(null)
      }
    }

    void load()
    return () => { cancelled = true }
  }, [artifact.key, accessToken, pathPrefix])

  useEffect(() => {
    return () => { if (blobUrl) URL.revokeObjectURL(blobUrl) }
  }, [blobUrl])

  const handleDownload = useCallback(() => {
    if (!blobUrl) return
    const a = document.createElement('a')
    a.href = blobUrl
    a.download = artifact.filename
    a.click()
  }, [blobUrl, artifact.filename])

  if (error) return null

  return (
    <div
      style={{
        width: 'min(100%, 620px)',
        border: '0.5px solid var(--c-border-subtle)',
        borderRadius: 8,
        padding: 8,
        background: 'var(--c-bg-sub)',
        margin: '8px 0',
      }}
    >
      {blobUrl ? (
        <video
          src={blobUrl}
          controls
          playsInline
          preload="metadata"
          style={{
            width: '100%',
            maxHeight: '70vh',
            display: 'block',
            borderRadius: 6,
            background: 'var(--c-bg-deep)',
          }}
        />
      ) : (
        <div
          className="attachment-shimmer"
          style={{ width: '100%', aspectRatio: '16 / 9', borderRadius: 6, background: 'var(--c-bg-deep)', position: 'relative', overflow: 'hidden' }}
        >
          {progress !== null && (
            <div
              style={{
                position: 'absolute',
                bottom: 0,
                left: 0,
                right: 0,
                height: 3,
                background: 'var(--c-accent)',
                transformOrigin: 'left center',
                transform: `scaleX(${Math.min(progress, 100) / 100})`,
                transition: 'transform 0.15s cubic-bezier(0.22, 1, 0.36, 1)',
                borderRadius: '0 2px 2px 0',
                willChange: 'transform',
              }}
            />
          )}
        </div>
      )}
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 8 }}>
        <a
          href={blobUrl ?? undefined}
          target="_blank"
          rel="noopener noreferrer"
          draggable={false}
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            gap: 8,
            minWidth: 0,
            padding: '7px 12px',
            borderRadius: 8,
            border: '0.5px solid var(--c-border-subtle)',
            background: 'var(--c-bg-page)',
            color: 'var(--c-text-primary)',
            fontSize: 13,
            textDecoration: 'none',
          }}
        >
          <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{artifact.filename}</span>
          <ExternalLink size={14} style={{ color: 'var(--c-text-icon)', flexShrink: 0 }} />
        </a>
        <button
          type="button"
          onClick={handleDownload}
          disabled={!blobUrl}
          title="Download"
          style={{
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            width: 34,
            height: 34,
            borderRadius: 8,
            border: '0.5px solid var(--c-border-subtle)',
            background: 'var(--c-bg-page)',
            color: 'var(--c-text-icon)',
            cursor: blobUrl ? 'pointer' : 'default',
            opacity: blobUrl ? 1 : 0.5,
          }}
        >
          <Download size={15} />
        </button>
      </div>
    </div>
  )
}
