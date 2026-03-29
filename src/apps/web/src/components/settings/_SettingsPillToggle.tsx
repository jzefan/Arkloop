import { useState } from 'react'

type Props = {
  checked: boolean
  onChange: (next: boolean) => void
  disabled?: boolean
}

/** 与 MemorySettings 等一致的胶囊开关 */
export function SettingsPillToggle({ checked, onChange, disabled }: Props) {
  const [hovered, setHovered] = useState(false)
  const showRing = hovered && !disabled

  return (
    <label
      className={`relative inline-flex shrink-0 items-center ${disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'}`}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
    >
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
        className="peer sr-only"
      />
      <span
        className="h-5 w-9 rounded-full transition-[background-color,box-shadow] duration-200"
        style={{
          background: checked ? 'var(--c-btn-bg)' : 'var(--c-border-mid)',
          boxShadow: showRing ? '0 0 0 1.5px var(--c-accent)' : '0 0 0 0px var(--c-accent)',
        }}
      />
      <span
        className="absolute left-0.5 top-0.5 h-4 w-4 rounded-full transition-transform duration-200 peer-checked:translate-x-4"
        style={{ background: checked ? 'var(--c-btn-text)' : 'var(--c-bg-page)' }}
      />
    </label>
  )
}
