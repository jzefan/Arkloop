import type { ReactNode } from 'react'

type Props = {
  title: ReactNode
  actions?: ReactNode
}

export function PageHeader({ title, actions }: Props) {
  return (
    <header className="flex min-h-[46px] items-center gap-4 border-b border-[var(--c-border-console)] px-6 py-2">
      <h2 className="min-w-0 flex-1 truncate text-sm font-medium text-[var(--c-text-secondary)]">
        {title}
      </h2>
      {actions && (
        <div className="ml-auto flex min-w-0 shrink-0 items-center gap-2">{actions}</div>
      )}
    </header>
  )
}
