import type { ReactNode } from 'react'

type Props = {
  label: string
  error?: string
  children: ReactNode
}

export function FormField({ label, error, children }: Props) {
  return (
    <div className="flex flex-col gap-1.5">
      <label className="text-xs font-medium text-[var(--c-text-tertiary)]">{label}</label>
      {children}
      {error && <p className="text-xs text-[var(--c-status-error-text)]">{error}</p>}
    </div>
  )
}
