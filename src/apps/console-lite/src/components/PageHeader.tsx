import type { ReactNode } from 'react'

type Props = {
  title: string
  actions?: ReactNode
}

export function PageHeader({ title, actions }: Props) {
  return (
    <header className="flex min-h-[46px] items-center justify-between border-b border-[var(--c-border-console)] px-6">
      <h2 className="text-sm font-medium text-[var(--c-text-secondary)]">{title}</h2>
      {actions && <div className="flex items-center gap-2">{actions}</div>}
    </header>
  )
}
