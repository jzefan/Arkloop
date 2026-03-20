type Props = {
  checked: boolean
  onChange: (next: boolean) => void
  disabled?: boolean
}

/** 与 MemorySettings 等一致的胶囊开关 */
export function SettingsPillToggle({ checked, onChange, disabled }: Props) {
  return (
    <label
      className={`relative inline-flex shrink-0 items-center ${disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'}`}
    >
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
        className="peer sr-only"
      />
      <span
        className="h-5 w-9 rounded-full transition-colors"
        style={{ background: checked ? 'var(--c-btn-bg)' : 'var(--c-border-mid)' }}
      />
      <span
        className="absolute left-0.5 top-0.5 h-4 w-4 rounded-full transition-transform peer-checked:translate-x-4"
        style={{ background: checked ? 'var(--c-btn-text)' : 'var(--c-bg-page)' }}
      />
    </label>
  )
}
