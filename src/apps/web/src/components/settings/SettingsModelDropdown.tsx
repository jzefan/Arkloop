import { useEffect, useRef, useState } from 'react'
import { ChevronDown } from 'lucide-react'

export type SettingsModelOption = { value: string; label: string }

export function SettingsModelDropdown({
  value,
  options,
  placeholder,
  disabled,
  onChange,
}: {
  value: string
  options: SettingsModelOption[]
  placeholder: string
  disabled: boolean
  onChange: (v: string) => void
}) {
  const [open, setOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)
  const btnRef = useRef<HTMLButtonElement>(null)

  const currentLabel = options.find((o) => o.value === value)?.label ?? (value || placeholder)

  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      if (
        menuRef.current?.contains(e.target as Node)
        || btnRef.current?.contains(e.target as Node)
      ) {
        return
      }
      setOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  return (
    <div className="relative">
      <button
        ref={btnRef}
        type="button"
        disabled={disabled}
        onClick={() => setOpen((v) => !v)}
        className="flex h-9 w-full items-center justify-between rounded-lg px-3 text-sm transition-colors hover:bg-[var(--c-bg-deep)]"
        style={{
          border: '0.5px solid var(--c-border-subtle)',
          background: 'var(--c-bg-page)',
          color: 'var(--c-text-secondary)',
        }}
      >
        <span className="truncate">{currentLabel}</span>
        <ChevronDown size={13} className="ml-2 shrink-0" />
      </button>

      {open && (
        <div
          ref={menuRef}
          className="dropdown-menu absolute left-0 top-[calc(100%+4px)] z-50"
          style={{
            border: '0.5px solid var(--c-border-subtle)',
            borderRadius: '10px',
            padding: '4px',
            background: 'var(--c-bg-menu)',
            width: '100%',
            boxShadow: 'var(--c-dropdown-shadow)',
            maxHeight: '220px',
            overflowY: 'auto',
          }}
        >
          <button
            type="button"
            onClick={() => { onChange(''); setOpen(false) }}
            className="flex w-full items-center px-3 py-2 text-sm transition-colors bg-[var(--c-bg-menu)] hover:bg-[var(--c-bg-deep)]"
            style={{
              borderRadius: '8px',
              fontWeight: !value ? 600 : 400,
              color: !value ? 'var(--c-text-heading)' : 'var(--c-text-secondary)',
            }}
          >
            {placeholder}
          </button>
          {options.map(({ value: v, label }) => (
            <button
              key={v}
              type="button"
              onClick={() => { onChange(v); setOpen(false) }}
              className="flex w-full items-center px-3 py-2 text-sm transition-colors bg-[var(--c-bg-menu)] hover:bg-[var(--c-bg-deep)]"
              style={{
                borderRadius: '8px',
                fontWeight: value === v ? 600 : 400,
                color: value === v ? 'var(--c-text-heading)' : 'var(--c-text-secondary)',
              }}
            >
              {label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
