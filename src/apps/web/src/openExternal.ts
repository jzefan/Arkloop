declare global {
  interface Window {
    arkloop?: {
      app?: {
        openExternal?: (url: string) => Promise<void>
      }
    }
  }
}

type AnchorClickEvent = {
  preventDefault: () => void
}

function isHttpExternalUrl(url: string): boolean {
  try {
    const parsed = new URL(url)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

export function openExternal(url: string): void {
  if (window.arkloop?.app?.openExternal) {
    void window.arkloop.app.openExternal(url)
  } else {
    window.open(url, '_blank', 'noopener,noreferrer')
  }
}

export function handleExternalAnchorClick(event: AnchorClickEvent, href?: string): void {
  if (!href || !isHttpExternalUrl(href)) return
  event.preventDefault()
  openExternal(href)
}
