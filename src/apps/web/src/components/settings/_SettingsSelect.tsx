import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Check, ChevronDown } from 'lucide-react'
import type { CSSProperties } from 'react'

export type SettingsSelectOption = { value: string; label: string }

type Props = {
  value: string
  options: SettingsSelectOption[]
  onChange: (value: string) => void
  disabled?: boolean
  placeholder?: string
  triggerClassName?: string
}

export const settingsSelectBorderColor = 'color-mix(in srgb, var(--c-border) 78%, var(--c-bg-input) 22%)'

export function SettingsSelect({
  value,
  options,
  onChange,
  disabled,
  placeholder,
  triggerClassName,
}: Props) {
  const [open, setOpen] = useState(false)
  const [highlighted, setHighlighted] = useState(value)
  const [menuStyle, setMenuStyle] = useState<CSSProperties>({})
  const menuRef = useRef<HTMLDivElement>(null)
  const btnRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    if (!open) return
    setHighlighted(value)
    const handler = (e: MouseEvent) => {
      if (
        menuRef.current?.contains(e.target as Node) ||
        btnRef.current?.contains(e.target as Node)
      ) return
      setOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open])

  const handleOpen = () => {
    if (disabled) return
    if (!open && btnRef.current) {
      const rect = btnRef.current.getBoundingClientRect()
      setMenuStyle({
        position: 'fixed',
        top: rect.bottom + 6,
        left: rect.left,
        width: rect.width,
        zIndex: 9999,
      })
    }
    setOpen((v) => !v)
  }

  const currentLabel = options.find((o) => o.value === value)?.label ?? placeholder ?? value

  const menu = open ? (
    <div
      ref={menuRef}
      className="dropdown-menu"
      style={{
        ...menuStyle,
        border: `0.65px solid ${settingsSelectBorderColor}`,
        borderRadius: 10,
        padding: '4px',
        background: 'var(--c-bg-menu)',
        boxShadow: 'var(--c-dropdown-shadow)',
        maxHeight: '220px',
        overflowY: 'auto',
      }}
    >
      {options.map((opt) => {
        const selected = opt.value === value
        const active = opt.value === highlighted
        return (
          <button
            key={opt.value}
            type="button"
            onMouseEnter={() => setHighlighted(opt.value)}
            onClick={() => { onChange(opt.value); setOpen(false) }}
            className={[
              'flex w-full items-center justify-between rounded-[6.5px] px-3 py-2 text-sm font-[450] transition-colors duration-[140ms]',
              active
                ? 'bg-[var(--c-bg-deep)] text-[var(--c-text-primary)]'
                : 'bg-[var(--c-bg-menu)] text-[var(--c-text-secondary)]',
            ].join(' ')}
          >
            <span className="truncate">{opt.label}</span>
            {selected && <Check size={13} className="ml-2 shrink-0" />}
          </button>
        )
      })}
    </div>
  ) : null

  return (
    <div className="relative">
      <button
        ref={btnRef}
        type="button"
        disabled={disabled}
        onClick={handleOpen}
        className={[
          'flex h-[32px] w-full items-center justify-between rounded-[6.5px] border-[0.65px] bg-[var(--c-bg-input)] px-3 text-sm font-[450] text-[var(--c-text-primary)] [background-clip:padding-box] transition-colors duration-[180ms] hover:bg-[var(--c-bg-deep)] disabled:cursor-not-allowed disabled:opacity-50',
          triggerClassName,
        ].filter(Boolean).join(' ')}
        style={{ borderColor: settingsSelectBorderColor }}
      >
        <span className="truncate">{currentLabel}</span>
        <ChevronDown size={13} className="ml-2 shrink-0 text-[var(--c-text-muted)]" />
      </button>
      {menu && createPortal(menu, document.body)}
    </div>
  )
}
