import type { ReactNode } from 'react'

type Variant = 'success' | 'warning' | 'error' | 'neutral'

type Props = {
  variant: Variant
  children: ReactNode
}

const styles: Record<Variant, string> = {
  success: 'bg-[var(--c-status-ok-bg)] text-[var(--c-status-ok-text)]',
  warning: 'bg-[var(--c-status-warn-bg)] text-[var(--c-status-warn-text)]',
  error:   'bg-[var(--c-status-danger-bg)] text-[var(--c-status-danger-text)]',
  neutral: 'bg-[var(--c-bg-deep)] text-[var(--c-text-muted)]',
}

export function SettingsStatusBadge({ variant, children }: Props) {
  return (
    <span className={`inline-flex items-center rounded-full px-2.5 py-[2px] text-[12px] font-[450] leading-[16px] ${styles[variant]}`}>
      {children}
    </span>
  )
}
