type NavItem<T extends string = string> = {
  key: T
  label: string
}

type Props<T extends string = string> = {
  items: NavItem<T>[]
  value: T
  onChange: (key: T) => void
  className?: string
}

export function SidebarNav<T extends string = string>({
  items,
  value,
  onChange,
  className,
}: Props<T>) {
  return (
    <div className={['w-[160px] shrink-0 overflow-y-auto border-r border-[var(--c-border-console)] p-2', className ?? ''].join(' ')}>
      <div className="flex flex-col gap-[3px]">
        {items.map((item) => (
          <button
            key={item.key}
            type="button"
            onClick={() => onChange(item.key)}
            className={[
              'flex h-[30px] items-center rounded-[5px] px-3 text-sm font-medium transition-colors',
              item.key === value
                ? 'bg-[var(--c-bg-sub)] text-[var(--c-text-primary)]'
                : 'text-[var(--c-text-tertiary)] hover:bg-[var(--c-bg-sub)] hover:text-[var(--c-text-secondary)]',
            ].join(' ')}
          >
            {item.label}
          </button>
        ))}
      </div>
    </div>
  )
}
