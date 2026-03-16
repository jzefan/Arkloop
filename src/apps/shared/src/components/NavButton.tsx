import type { ReactNode } from 'react'

type Props = {
  icon: ReactNode
  label: string
  active: boolean
  onClick: () => void
}

export function NavButton({ icon, label, active, onClick }: Props) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={[
        'flex h-[30px] items-center gap-[11px] rounded-[5px] px-2 py-[7px] text-sm font-medium transition-colors',
        active
          ? 'bg-[var(--c-bg-sub)] text-[var(--c-text-primary)]'
          : 'text-[var(--c-text-tertiary)] hover:bg-[var(--c-bg-sub)] hover:text-[var(--c-text-secondary)]',
      ].join(' ')}
    >
      <span className="flex h-[22px] w-[22px] shrink-0 items-center justify-center">
        {icon}
      </span>
      <span>{label}</span>
    </button>
  )
}
