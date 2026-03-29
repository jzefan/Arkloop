type Props = {
  checked: boolean
  onChange: (next: boolean) => void
  disabled?: boolean
}

/** 与 MemorySettings 等一致的胶囊开关 */
export function SettingsPillToggle({ checked, onChange, disabled }: Props) {
  return (
    <label
      className={`group relative inline-flex shrink-0 items-center ${disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'}`}
    >
      <input
        type="checkbox"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
        className="peer sr-only"
      />
      {/*
        轨道：1.1x（22px × 40px）
        基础描边：1px solid border-subtle（静止可见）
        激活：border-mid + 内侧 accent outline（跟随主色）
        hover：border-mid + accent outline（描边从小撑开）
      */}
      <span
        className={[
          'h-[22px] w-10 rounded-full transition-[background-color,border-color,outline-color,box-shadow] duration-200',
          !disabled && 'group-hover:shadow-[0_0_0_2px_var(--c-accent)]',
        ].filter(Boolean).join(' ')}
        style={{
          background: checked ? 'var(--c-btn-bg)' : 'var(--c-border-mid)',
          border: `1px solid ${checked ? 'var(--c-border-mid)' : 'var(--c-border-subtle)'}`,
        }}
      />
      {/* 圆点：18px，translate 18px（40 - 18 - 2*2 = 18） */}
      <span
        className="absolute left-[2px] top-[2px] h-[18px] w-[18px] rounded-full transition-transform duration-200 peer-checked:translate-x-[18px]"
        style={{ background: checked ? 'var(--c-btn-text)' : 'var(--c-bg-page)' }}
      />
    </label>
  )
}
