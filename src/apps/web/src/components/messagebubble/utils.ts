import type { ArtifactRef } from '../../storage'

export function isDocumentArtifact(artifact: ArtifactRef): boolean {
  if (artifact.display === 'panel') return true
  return !artifact.mime_type.startsWith('image/') && artifact.mime_type !== 'text/html'
}

export function formatShortDate(dateStr: string): string {
  const d = new Date(dateStr)
  const month = d.toLocaleString('en-US', { month: 'short' })
  return `${month}. ${d.getDate()}`
}

export function formatFullDate(dateStr: string): string {
  const d = new Date(dateStr)
  return d.toLocaleString('en-US', {
    month: 'long',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    hour12: true,
  })
}

export function isArtifactReferenced(content: string, key: string): boolean {
  return content.includes(`artifact:${key}`)
}

export function getDomain(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, '')
  } catch {
    return url
  }
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

export const LIGHTBOX_ANIM_MS = 120

export const USER_TEXT_LINE_HEIGHT = 25.6 // 16px * 1.6
export const USER_TEXT_MAX_LINES = 9
export const USER_TEXT_COLLAPSED_HEIGHT = USER_TEXT_LINE_HEIGHT * USER_TEXT_MAX_LINES
export const USER_TEXT_FADE_HEIGHT = USER_TEXT_LINE_HEIGHT * 2
