import { useEffect, useRef } from 'react'

declare global {
  interface Window {
    turnstile?: {
      render: (container: string | HTMLElement, options: TurnstileOptions) => string
      remove: (widgetId: string) => void
      reset: (widgetId: string) => void
    }
  }
}

interface TurnstileOptions {
  sitekey: string
  callback: (token: string) => void
  'expired-callback'?: () => void
  'error-callback'?: () => void
  theme?: 'light' | 'dark' | 'auto'
  size?: 'normal' | 'compact' | 'flexible'
}

interface TurnstileProps {
  siteKey: string
  onSuccess: (token: string) => void
  onExpire?: () => void
}

export function Turnstile({ siteKey, onSuccess, onExpire }: TurnstileProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const widgetIdRef = useRef<string | null>(null)
  // 用 ref 持有 callback，避免 effect 依赖引用变化导致 widget 反复 mount/unmount
  const onSuccessRef = useRef(onSuccess)
  const onExpireRef = useRef(onExpire)
  useEffect(() => {
    onSuccessRef.current = onSuccess
    onExpireRef.current = onExpire
  })

  useEffect(() => {
    if (!containerRef.current || !siteKey) return

    const mount = () => {
      if (!containerRef.current || !window.turnstile) return
      widgetIdRef.current = window.turnstile.render(containerRef.current, {
        sitekey: siteKey,
        size: 'flexible',
        callback: (token) => onSuccessRef.current(token),
        'expired-callback': () => onExpireRef.current?.(),
        'error-callback': () => onExpireRef.current?.(),
      })
    }

    if (window.turnstile) {
      mount()
    } else {
      const interval = setInterval(() => {
        if (window.turnstile) {
          clearInterval(interval)
          mount()
        }
      }, 100)
      return () => clearInterval(interval)
    }

    return () => {
      if (widgetIdRef.current && window.turnstile) {
        window.turnstile.remove(widgetIdRef.current)
        widgetIdRef.current = null
      }
    }
  // 仅依赖 siteKey，callback 变化不重建 widget
  }, [siteKey])

  return <div ref={containerRef} style={{ width: '100%' }} />
}
