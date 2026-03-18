import { useRef, useEffect } from 'react'
import { ArtifactIframe, type ArtifactIframeHandle, type ArtifactAction } from './ArtifactIframe'
import type { ArtifactRef } from '../storage'

export type StreamingArtifactEntry = {
  toolCallIndex: number
  toolCallId?: string
  toolName?: string
  argumentsBuffer: string
  title?: string
  filename?: string
  display?: 'inline' | 'panel'
  content?: string
  complete: boolean
  artifactRef?: ArtifactRef
}

type Props = {
  entry: StreamingArtifactEntry
  accessToken?: string
  onAction?: (action: ArtifactAction) => void
}

export function extractPartialArtifactFields(buffer: string): {
  title?: string
  filename?: string
  display?: string
  content?: string
} {
  return {
    title: extractJSONStringField(buffer, 'title'),
    filename: extractJSONStringField(buffer, 'filename'),
    display: extractJSONStringField(buffer, 'display'),
    content: extractJSONStringField(buffer, 'content') ?? extractJSONStringField(buffer, 'widget_code'),
  }
}

function extractJSONStringField(buffer: string, field: string): string | undefined {
  const start = buffer.search(new RegExp(`"${field}"\\s*:\\s*"`))
  if (start < 0) return undefined
  const keyToken = `"${field}"`
  const valueStart = buffer.indexOf('"', start + keyToken.length)
  if (valueStart < 0) return undefined
  return readJSONString(buffer, valueStart + 1)
}

function readJSONString(source: string, start: number): string {
  let result = ''
  let index = start

  while (index < source.length) {
    const char = source[index]
    if (char === '"') return result
    if (char !== '\\') {
      result += char
      index += 1
      continue
    }

    const next = source[index + 1]
    if (next == null) return result
    if (next === 'u') {
      const hex = source.slice(index + 2, index + 6)
      if (/^[0-9a-fA-F]{4}$/.test(hex)) {
        result += String.fromCharCode(Number.parseInt(hex, 16))
        index += 6
        continue
      }
      return result
    }

    result += decodeEscapedChar(next)
    index += 2
  }

  return result
}

function decodeEscapedChar(char: string): string {
  switch (char) {
    case 'n':
      return '\n'
    case 'r':
      return '\r'
    case 't':
      return '\t'
    case '"':
      return '"'
    case '\\':
      return '\\'
    case '/':
      return '/'
    case 'b':
      return '\b'
    case 'f':
      return '\f'
    default:
      return char
  }
}

export function ArtifactStreamBlock({ entry, accessToken, onAction }: Props) {
  const iframeRef = useRef<ArtifactIframeHandle>(null)
  const lastContentRef = useRef<string>('')

  useEffect(() => {
    if (!entry.content || entry.content === lastContentRef.current) return
    lastContentRef.current = entry.content
    if (entry.complete) {
      iframeRef.current?.finalizeContent(entry.content)
    } else {
      iframeRef.current?.setStreamingContent(entry.content)
    }
  }, [entry.content, entry.complete])

  // display=panel artifacts are not rendered inline during streaming;
  // they just show as a compact card
  if (entry.display === 'panel' && !entry.content) {
    return null
  }

  const isInline = entry.display !== 'panel'
  const title = entry.title || entry.filename || 'Artifact'

  if (entry.artifactRef && !isInline) {
    return null
  }

  // already have static artifact? render static iframe
  if (entry.artifactRef && isInline) {
    return (
      <div style={{ margin: '8px 0', maxWidth: '720px' }}>
        <div style={{
          fontSize: '12px',
          fontWeight: 500,
          color: 'var(--c-text-secondary)',
          marginBottom: '6px',
        }}>
          {title}
        </div>
        <ArtifactIframe
          mode="static"
          artifact={entry.artifactRef}
          accessToken={accessToken}
          onAction={onAction}
          frameTitle={title}
          style={{ minHeight: '300px' }}
        />
      </div>
    )
  }

  // streaming mode
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
        {!entry.complete && (
          <span style={{
            display: 'inline-block',
            width: '6px',
            height: '6px',
            borderRadius: '50%',
            background: 'var(--c-text-tertiary)',
            animation: '_fadeIn 0.6s ease infinite alternate',
          }} />
        )}
      </div>
      <ArtifactIframe
        ref={iframeRef}
        mode="streaming"
        onAction={onAction}
        frameTitle={title}
        style={{ minHeight: '200px' }}
      />
    </div>
  )
}
