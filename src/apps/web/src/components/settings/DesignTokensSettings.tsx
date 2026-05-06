import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import {
  AlertTriangle,
  BookOpen,
  Brain,
  Check,
  Copy,
  Download,
  FileText,
  Layers,
  List,
  Plus,
  RefreshCw,
  Search,
  Settings,
  SlidersHorizontal,
  Trash2,
  X,
  Zap,
} from 'lucide-react'
import { ConfirmDialog, PillToggle } from '@arkloop/shared'
import { SettingsSectionHeader } from './_SettingsSectionHeader'
import { SettingsInput } from './_SettingsInput'
import { SettingsStatusBadge } from './_SettingsStatusBadge'
import { ProviderSelectCard } from './ProviderSelectCard'

type ButtonVariant = 'primary' | 'secondary' | 'danger'
type CanvasSize = 'narrow' | 'wide'
type StatusVariant = 'success' | 'warning' | 'error' | 'neutral'
type ToastVariant = 'success' | 'error' | 'warn' | 'neutral'

const pageShell = 'flex flex-col gap-8'
const specPanel = 'rounded-xl border-[0.5px] border-[var(--c-border-subtle)] bg-[var(--c-bg-menu)]'
const previewPanel = `${specPanel} p-5`
const labelCls = 'text-[11px] font-semibold uppercase tracking-wider text-[var(--c-text-muted)]'
const mutedText = 'text-sm leading-relaxed text-[var(--c-text-secondary)]'
const ruleText = 'text-xs leading-relaxed text-[var(--c-text-muted)]'
const controlRadius = 'rounded-[6.5px]'
const selectBorderColor = 'color-mix(in srgb, var(--c-border) 78%, var(--c-bg-input) 22%)'
const secondaryFrameBorder = 'color-mix(in srgb, var(--c-border) 91%, var(--c-text-primary) 9%)'
const dangerText = '#ea4d3c'
const dangerSurface = `color-mix(in srgb, var(--c-bg-menu) 96%, ${dangerText} 4%)`
const dangerBorder = `color-mix(in srgb, var(--c-border-subtle) 84%, ${dangerText} 16%)`

const toastText: Record<ToastVariant, string> = {
  success: 'text-[var(--c-status-ok-text)]',
  error: 'text-[#ea4d3c]',
  warn: 'text-[var(--c-status-warn-text)]',
  neutral: 'text-[var(--c-text-secondary)]',
}

const toastSurface: Record<ToastVariant, string> = {
  success: 'var(--c-bg-input)',
  error: 'var(--c-bg-input)',
  warn: 'var(--c-bg-input)',
  neutral: 'var(--c-bg-input)',
}

const toastBorder: Record<ToastVariant, string> = {
  success: secondaryFrameBorder,
  error: secondaryFrameBorder,
  warn: secondaryFrameBorder,
  neutral: secondaryFrameBorder,
}

const radiusScale = [
  { name: 'Control', value: '8px', use: 'button, icon button' },
  { name: 'Input', value: '10px', use: 'input, select, search' },
  { name: 'Row', value: '10px', use: 'setting row, resource row' },
  { name: 'Section', value: '14px', use: 'settings section' },
  { name: 'Modal', value: '18px', use: 'reader and resource modal' },
  { name: 'Pill', value: '999px', use: 'badge, segmented control' },
]

const densityScale = [
  { name: 'Control sm', value: '32px', use: 'toolbar action' },
  { name: 'Control md', value: '36px', use: 'default field' },
  { name: 'Row compact', value: '48px', use: 'resource list' },
  { name: 'Row standard', value: '60px', use: 'setting row' },
  { name: 'Canvas narrow', value: '760px', use: 'simple settings' },
  { name: 'Canvas wide', value: '1040px', use: 'management pages' },
]

const colorRoles = [
  { role: 'App', token: '--c-bg-page', use: 'root workspace background' },
  { role: 'Sidebar', token: '--c-bg-sidebar', use: 'left navigation surface' },
  { role: 'Surface', token: '--c-bg-menu', use: 'section, modal, card' },
  { role: 'Field', token: '--c-bg-input', use: 'input, select, search' },
  { role: 'Hover', token: '--c-bg-deep', use: 'row and button hover' },
  { role: 'Primary text', token: '--c-text-primary', use: 'body text' },
  { role: 'Secondary text', token: '--c-text-secondary', use: 'row descriptions' },
  { role: 'Muted text', token: '--c-text-muted', use: 'metadata, placeholder' },
  { role: 'Border subtle', token: '--c-border-subtle', use: 'default divider' },
  { role: 'Border strong', token: '--c-border', use: 'focus and selected' },
  { role: 'Action', token: '--c-btn-bg', use: 'primary button fill' },
  { role: 'Action text', token: '--c-btn-text', use: 'primary button label' },
  { role: 'Danger', token: '--c-status-danger-text', use: 'destructive intent' },
  { role: 'Warning', token: '--c-status-warn-text', use: 'recoverable issue' },
  { role: 'Success', token: '--c-status-ok-text', use: 'ready state' },
]

const pagePatterns = [
  {
    title: 'Simple Settings',
    description: '窄内容列、section 分组、row-based 控件。',
    icon: <SlidersHorizontal size={16} />,
    slots: ['Page header', 'SettingsSection', 'SettingRow', 'Inline control'],
  },
  {
    title: 'Resource List',
    description: '可搜索资源集合，使用紧凑 row 和低权重操作。',
    icon: <List size={16} />,
    slots: ['Toolbar', 'SearchInput', 'ResourceRow', 'Hover actions'],
  },
  {
    title: 'Document Reader',
    description: '阅读一个完整文本对象，正文宽度稳定。',
    icon: <BookOpen size={16} />,
    slots: ['Reader header', 'Metadata', 'Prose surface', 'Close action'],
  },
  {
    title: 'Master Detail',
    description: '左侧资源列表，右侧宽设置画布。',
    icon: <Layers size={16} />,
    slots: ['Resource rail', 'Detail header', 'Form section', 'Resource section'],
  },
]

const foundationRules = [
  {
    title: '颜色只按角色使用',
    body: '页面不直接选色。背景、输入、hover、selected、border、primary、danger 都从已有 CSS 变量取值。',
  },
  {
    title: '默认是柔和圆角矩形',
    body: '按钮、输入框、资源行、section 都保持 rounded-lg 到 rounded-xl；只有 badge、toggle、segmented control 接近 pill。',
  },
  {
    title: '阴影只给浮层和预览',
    body: '普通设置页依靠 border 和 surface 层级；dropdown、modal、mini preview 可以使用已有 shadow token。',
  },
  {
    title: '密度优先跟随 desktop',
    body: 'Provider list 38px，toolbar button 32px，普通输入 32px 到 36px，资源行约 52px。',
  },
  {
    title: '内容进入有标题容器',
    body: '设置内容必须被 section/card 包裹，并带有清晰标题；容器内部优先用间距组织，不依赖分割线。',
  },
]

const primitiveRules = [
  {
    title: 'Primary',
    body: '只给保存、添加、确认、连接这类主操作。一个局部区域里不要同时出现多个 primary。',
  },
  {
    title: 'Secondary',
    body: '用于测试、导入、取消、配置等普通操作。默认描边清楚，hover 时内部色块变深、边框退场。',
  },
  {
    title: 'Icon',
    body: '图标按钮也使用 secondary frame。刷新、设置、更多、关闭不能裸露在页面上。',
  },
  {
    title: 'Danger',
    body: '删除、清空、重置才使用 danger。常驻危险动作应保持低权重，确认态再变强。',
  },
]

const compositionRules = [
  {
    title: 'Simple Settings',
    body: '右侧进入 narrow canvas，内容不贴 sidebar；section 内部用 row 组织 label、说明和控件。',
  },
  {
    title: 'Master Detail',
    body: 'Provider、MCP、Tools 这类对象管理使用左资源列表 + 右详情，右详情保持 max-w-2xl 或 wide canvas。',
  },
  {
    title: 'Resource List',
    body: '多个对象用 list row，不直接堆大文本卡片。列表只展示标题、摘要、状态和 hover action。',
  },
  {
    title: 'Document Reader',
    body: '阅读完整 Impression 或单条 Note 时进入 reader/modal；完整正文不提前暴露在资源列表里。',
  },
]

const overlayRules = [
  {
    title: 'Modal',
    body: '需要阻断背景页面时使用；标题、关闭、表单和底部操作必须形成稳定结构。',
  },
  {
    title: 'Confirm dialog',
    body: '删除、清空、重置这类确认动作使用更小弹窗；危险操作只在确认态加强。',
  },
  {
    title: 'Overlay',
    body: '背景只负责降噪，不改变真实页面布局；弹出层的 surface 和边框必须来自系统变量。',
  },
]

function TokenPageSection({
  eyebrow,
  title,
  description,
  children,
}: {
  eyebrow: string
  title: string
  description: string
  children: ReactNode
}) {
  return (
    <section className="flex flex-col gap-4">
      <div className="max-w-3xl">
        <div className={labelCls}>{eyebrow}</div>
        <h3 className="mt-2 text-lg font-semibold text-[var(--c-text-heading)]">{title}</h3>
        <p className={`mt-1 ${mutedText}`}>{description}</p>
      </div>
      {children}
    </section>
  )
}

function SpecCard({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className={`${previewPanel} flex flex-col gap-4`}>
      <div className={labelCls}>{title}</div>
      {children}
    </div>
  )
}

function RuleCard({ title, body }: { title: string; body: string }) {
  return (
    <div className="rounded-xl border-[0.5px] border-[var(--c-border-subtle)] bg-[var(--c-bg-page)] p-4">
      <div className="text-sm font-medium text-[var(--c-text-heading)]">{title}</div>
      <p className={`mt-1 ${ruleText}`}>{body}</p>
    </div>
  )
}

function RuleGrid({ rules }: { rules: Array<{ title: string; body: string }> }) {
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      {rules.map((rule) => <RuleCard key={rule.title} {...rule} />)}
    </div>
  )
}

function SettingsButton({
  variant = 'secondary',
  children,
  icon,
  disabled,
  onClick,
}: {
  variant?: ButtonVariant
  children: ReactNode
  icon?: ReactNode
  disabled?: boolean
  onClick?: () => void
}) {
  const styles: Record<ButtonVariant, string> = {
    primary:
      'inline-flex h-[32px] items-center justify-center gap-1.5 rounded-[6.5px] px-3.5 text-sm font-[450] text-[var(--c-btn-text)] transition-[box-shadow] duration-150 hover:[box-shadow:inset_0_0_0_999px_rgba(255,255,255,0.07),0_0_0_0.2px_var(--c-btn-bg)] active:[box-shadow:inset_0_0_0_999px_rgba(0,0,0,0.04)] disabled:cursor-not-allowed disabled:opacity-40',
    secondary:
      'inline-flex h-[32px] items-center justify-center gap-1.5 rounded-[6.5px] border-[0.65px] [border-color:color-mix(in_srgb,var(--c-border)_91%,var(--c-text-primary)_9%)] bg-[var(--c-bg-input)] px-3.5 text-sm font-[450] text-[color-mix(in_srgb,var(--c-text-secondary)_72%,var(--c-text-primary)_28%)] [background-clip:padding-box] transition-colors duration-[180ms] hover:border-transparent hover:bg-[var(--c-bg-deep)] hover:text-[var(--c-text-primary)] disabled:cursor-not-allowed disabled:opacity-40',
    danger:
      'inline-flex h-[32px] items-center justify-center gap-1.5 rounded-[6.5px] border-[0.65px] [border-color:color-mix(in_srgb,var(--c-border)_91%,var(--c-text-primary)_9%)] bg-[var(--c-bg-input)] px-3.5 text-sm font-[450] text-[var(--c-text-muted)] [background-clip:padding-box] transition-colors duration-[180ms] hover:border-transparent hover:bg-[var(--c-bg-deep)] hover:text-[#ea4d3c] disabled:cursor-not-allowed disabled:opacity-40',
  }

  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      className={styles[variant]}
      style={variant === 'primary' ? { background: 'var(--c-btn-bg)' } : undefined}
    >
      {icon}
      <span className="truncate">{children}</span>
    </button>
  )
}

function IconButton({
  children,
  label,
  danger = false,
}: {
  children: ReactNode
  label: string
  danger?: boolean
}) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      className={[
        'inline-flex h-[32px] w-[32px] items-center justify-center rounded-[6.5px] border-[0.65px] [border-color:color-mix(in_srgb,var(--c-border)_91%,var(--c-text-primary)_9%)] bg-[var(--c-bg-input)] [background-clip:padding-box] text-[color-mix(in_srgb,var(--c-text-secondary)_72%,var(--c-text-primary)_28%)] transition-colors duration-[180ms] hover:border-transparent hover:bg-[var(--c-bg-deep)]',
        danger ? 'hover:text-[#ea4d3c]' : 'hover:text-[var(--c-text-primary)]',
      ].join(' ')}
    >
      {children}
    </button>
  )
}

function CopyStateButton() {
  const [copied, setCopied] = useState(false)
  const contentRef = useRef<HTMLSpanElement>(null)
  const [width, setWidth] = useState<number | null>(null)

  const handleCopyPreview = () => {
    setCopied(true)
    window.setTimeout(() => setCopied(false), 1200)
  }

  useLayoutEffect(() => {
    if (!contentRef.current) return
    setWidth(Math.ceil(contentRef.current.getBoundingClientRect().width))
  }, [copied])

  return (
    <button
      type="button"
      onClick={handleCopyPreview}
      className="inline-flex h-[32px] items-center justify-center overflow-hidden rounded-[6.5px] border-[0.65px] [border-color:color-mix(in_srgb,var(--c-border)_91%,var(--c-text-primary)_9%)] bg-[var(--c-bg-input)] text-sm font-[450] text-[color-mix(in_srgb,var(--c-text-secondary)_72%,var(--c-text-primary)_28%)] [background-clip:padding-box] transition-[width,background-color,border-color,color] duration-[220ms] ease-out hover:border-transparent hover:bg-[var(--c-bg-deep)] hover:text-[var(--c-text-primary)]"
      style={{ width: width === null ? undefined : width }}
    >
      <span
        ref={contentRef}
        className="inline-flex items-center justify-center gap-1.5 whitespace-nowrap px-3.5"
      >
        {copied ? <Check size={14} /> : <Copy size={14} />}
        {copied ? 'Copied' : 'Copy'}
      </span>
    </button>
  )
}

function SettingsCanvas({
  size = 'narrow',
  children,
}: {
  size?: CanvasSize
  children: ReactNode
}) {
  return (
    <div
      className={`mx-auto flex w-full flex-col gap-4 ${
        size === 'narrow' ? 'max-w-[760px]' : 'max-w-[1040px]'
      }`}
    >
      {children}
    </div>
  )
}

function ContractSection({
  title,
  description,
  actions,
  children,
}: {
  title: string
  description?: string
  actions?: ReactNode
  children: ReactNode
}) {
  return (
    <section className={specPanel}>
      <div className="flex items-start justify-between gap-4 border-b border-[var(--c-border-subtle)] px-4 py-3">
        <div className="min-w-0">
          <h4 className="text-sm font-semibold text-[var(--c-text-heading)]">{title}</h4>
          {description && <p className="mt-0.5 text-xs leading-relaxed text-[var(--c-text-muted)]">{description}</p>}
        </div>
        {actions && <div className="shrink-0">{actions}</div>}
      </div>
      <div>{children}</div>
    </section>
  )
}

function SettingRow({
  title,
  description,
  control,
  disabled,
}: {
  title: string
  description?: string
  control: ReactNode
  disabled?: boolean
}) {
  return (
    <div className={`flex min-h-[60px] items-center justify-between gap-4 px-4 py-4 ${disabled ? 'opacity-45' : ''}`}>
      <div className="min-w-0 flex-1">
        <div className="text-sm font-medium text-[var(--c-text-heading)]">{title}</div>
        {description && <div className="mt-0.5 text-xs leading-relaxed text-[var(--c-text-muted)]">{description}</div>}
      </div>
      <div className="shrink-0">{control}</div>
    </div>
  )
}

function SearchInput({ placeholder = 'Search' }: { placeholder?: string }) {
  return (
    <div className="relative">
      <Search size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-[var(--c-text-muted)]" />
      <TokenInput className="pl-8" placeholder={placeholder} />
    </div>
  )
}

function TokenInput({
  className,
  placeholder,
  defaultValue,
  type = 'text',
}: {
  className?: string
  placeholder?: string
  defaultValue?: string
  type?: string
}) {
  return (
    <input
      type={type}
      className={[
        'h-[32px] w-full border-[0.65px] [border-color:color-mix(in_srgb,var(--c-border)_64%,var(--c-bg-input)_36%)] bg-[var(--c-bg-input)] px-3 text-sm font-[450] text-[var(--c-text-primary)] outline-none placeholder:font-[350] placeholder:text-[var(--c-text-muted)] transition-colors duration-[180ms] hover:[border-color:color-mix(in_srgb,var(--c-border)_72%,var(--c-text-primary)_28%)] focus:[border-color:color-mix(in_srgb,var(--c-border)_72%,var(--c-text-primary)_28%)]',
        controlRadius,
        className,
      ].filter(Boolean).join(' ')}
      placeholder={placeholder}
      defaultValue={defaultValue}
    />
  )
}

function TokenSelect({
  value,
  options,
  onChange,
  triggerClassName,
}: {
  value: string
  options: Array<{ value: string; label: string }>
  onChange: (value: string) => void
  triggerClassName?: string
}) {
  const [open, setOpen] = useState(false)
  const [highlighted, setHighlighted] = useState(value)
  const menuRef = useRef<HTMLDivElement>(null)
  const buttonRef = useRef<HTMLButtonElement>(null)
  const currentLabel = options.find((option) => option.value === value)?.label ?? value

  useEffect(() => {
    if (!open) return
    setHighlighted(value)
    const handler = (event: MouseEvent) => {
      if (
        menuRef.current?.contains(event.target as Node) ||
        buttonRef.current?.contains(event.target as Node)
      ) return
      setOpen(false)
    }
    document.addEventListener('mousedown', handler)
    return () => document.removeEventListener('mousedown', handler)
  }, [open, value])

  return (
    <div className="relative">
      <button
        ref={buttonRef}
        type="button"
        onClick={() => setOpen((next) => !next)}
        className={[
          'flex h-[32px] w-full items-center justify-between border-[0.65px] bg-[var(--c-bg-input)] px-3 text-sm font-[450] text-[var(--c-text-primary)] [background-clip:padding-box] transition-colors duration-[180ms] hover:bg-[var(--c-bg-deep)]',
          controlRadius,
          triggerClassName,
        ].filter(Boolean).join(' ')}
        style={{ borderColor: selectBorderColor }}
      >
        <span className="truncate">{currentLabel}</span>
        <ChevronDownIcon />
      </button>

      {open && (
        <div
          ref={menuRef}
          className="dropdown-menu absolute left-0 top-[calc(100%+6px)] z-50 max-h-[220px] w-full overflow-y-auto border-[0.65px] bg-[var(--c-bg-menu)] p-1"
          style={{
            borderColor: selectBorderColor,
            borderRadius: 10,
            boxShadow: 'var(--c-dropdown-shadow)',
          }}
        >
          {options.map((option) => {
            const selected = option.value === value
            const active = option.value === highlighted
            return (
              <button
                key={option.value}
                type="button"
                onMouseEnter={() => setHighlighted(option.value)}
                onClick={() => { onChange(option.value); setOpen(false) }}
                className={[
                  'flex w-full items-center justify-between rounded-[6.5px] px-3 py-2 text-sm font-[450] transition-colors duration-[140ms]',
                  active ? 'bg-[var(--c-bg-deep)] text-[var(--c-text-primary)]' : 'bg-[var(--c-bg-menu)] text-[var(--c-text-secondary)]',
                ].join(' ')}
              >
                <span className="truncate">{option.label}</span>
                {selected && <Check size={13} className="ml-2 shrink-0" />}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}

function CapsuleSegmentedControl<T extends string>({
  value,
  options,
  onChange,
}: {
  value: T
  options: Array<{ value: T; label: string }>
  onChange: (value: T) => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const [pill, setPill] = useState({ left: 0, width: 0 })
  const [animate, setAnimate] = useState(false)

  useLayoutEffect(() => {
    const container = containerRef.current
    if (!container) return
    const measure = () => {
      const button = container.querySelector<HTMLButtonElement>(`[data-capsule="${value}"]`)
      if (!button) return
      setPill({ left: button.offsetLeft, width: button.offsetWidth })
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(container)
    return () => observer.disconnect()
  }, [value])

  useLayoutEffect(() => {
    const id = requestAnimationFrame(() => setAnimate(true))
    return () => cancelAnimationFrame(id)
  }, [])

  return (
    <div
      ref={containerRef}
      className="relative inline-flex w-fit gap-0.5 rounded-[7.5px] bg-[var(--c-bg-deep)] p-[2px]"
    >
      <span
        className="pointer-events-none absolute bottom-[2px] top-[2px] rounded-[6.25px] border-[0.5px] border-[var(--c-border-subtle)] bg-[var(--c-bg-menu)] shadow-[0_1px_2px_rgba(0,0,0,0.04)]"
        style={{
          left: pill.left,
          width: pill.width,
          transition: animate ? 'left 180ms cubic-bezier(0.22, 1, 0.36, 1), width 180ms cubic-bezier(0.22, 1, 0.36, 1)' : 'none',
        }}
      />
      {options.map((option) => {
        const active = option.value === value
        return (
          <button
            key={option.value}
            type="button"
            data-capsule={option.value}
            onClick={() => onChange(option.value)}
            className={[
              'group relative z-10 overflow-hidden rounded-[6.25px] px-[10px] py-[5px] text-[12.5px] font-[450] leading-[19px] transition-colors duration-[160ms]',
              active ? 'text-[var(--c-text-primary)]' : 'text-[var(--c-text-muted)] hover:text-[var(--c-text-primary)]',
            ].join(' ')}
            style={{
              minWidth: 0,
            }}
          >
            {!active && (
              <span className="pointer-events-none absolute inset-0 bg-transparent transition-[background-color,box-shadow] duration-[160ms] ease-out group-hover:bg-[color-mix(in_srgb,var(--c-bg-deep)_90%,var(--c-text-primary)_10%)] group-hover:shadow-[inset_0_0_0_0.5px_var(--c-border-subtle)] group-active:bg-[color-mix(in_srgb,var(--c-bg-deep)_84%,var(--c-text-primary)_16%)] group-active:shadow-[inset_0_1px_2px_rgba(0,0,0,0.08)]" />
            )}
            <span className="relative z-10">{option.label}</span>
          </button>
        )
      })}
    </div>
  )
}

function StatusBadge({ variant, children }: { variant: StatusVariant; children: ReactNode }) {
  return <SettingsStatusBadge variant={variant}>{children}</SettingsStatusBadge>
}

function SwatchCell({ role, token, use }: { role: string; token: string; use: string }) {
  return (
    <div className="rounded-xl border border-[var(--c-border-subtle)] bg-[var(--c-bg-menu)] p-3">
      <div className="h-10 rounded-lg border border-[var(--c-border-subtle)]" style={{ background: `var(${token})` }} />
      <div className="mt-2 text-sm font-medium text-[var(--c-text-heading)]">{role}</div>
      <div className="mt-0.5 truncate font-mono text-[10px] text-[var(--c-text-muted)]">{token}</div>
      <div className="mt-1 text-xs leading-relaxed text-[var(--c-text-muted)]">{use}</div>
    </div>
  )
}

function ScaleCard({ name, value, use }: { name: string; value: string; use: string }) {
  return (
    <div className="rounded-xl border border-[var(--c-border-subtle)] bg-[var(--c-bg-menu)] p-4">
      <div className="text-sm font-medium text-[var(--c-text-heading)]">{name}</div>
      <div className="mt-2 font-mono text-lg text-[var(--c-text-primary)]">{value}</div>
      <div className="mt-1 text-xs leading-relaxed text-[var(--c-text-muted)]">{use}</div>
    </div>
  )
}

function FoundationsPreview() {
  return (
    <TokenPageSection
      eyebrow="Foundations"
      title="基础视觉合约"
      description="这里先把颜色、圆角、密度和阴影的判断固定下来。页面只能解释现有系统，不能再造另一套视觉。"
    >
      <RuleGrid rules={foundationRules} />

      <div className={`${previewPanel} grid gap-4 lg:grid-cols-[1.15fr_0.85fr]`}>
        <div>
          <div className={labelCls}>Existing surface sample</div>
          <div className="mt-3 rounded-xl border border-[var(--c-border-subtle)] bg-[var(--c-bg-page)] p-5">
            <h4 className="text-xl font-semibold text-[var(--c-text-heading)]">Settings surface</h4>
            <p className="mt-2 max-w-2xl text-sm leading-relaxed text-[var(--c-text-secondary)]">
              Page background, menu surface, subtle borders, muted metadata, and primary actions should all come from shared tokens.
            </p>
          </div>
        </div>
        <div>
          <div className={labelCls}>Current shape sample</div>
          <div className="mt-3 flex h-[132px] items-center justify-center rounded-xl border border-[var(--c-border-subtle)] bg-[var(--c-bg-input)] p-5">
            <div className="flex h-20 w-full max-w-[260px] items-center justify-center rounded-[14px] border border-[var(--c-border)] bg-[var(--c-bg-menu)] text-sm font-medium text-[var(--c-text-heading)]">
              rounded-xl section
            </div>
          </div>
        </div>
      </div>

      <div className="grid gap-4 xl:grid-cols-[1.4fr_0.6fr]">
        <SpecCard title="Color roles">
          <div className="grid grid-cols-[repeat(auto-fill,minmax(150px,1fr))] gap-3">
            {colorRoles.map((item) => <SwatchCell key={item.token} {...item} />)}
          </div>
        </SpecCard>
        <div className="grid gap-4">
          <SpecCard title="Radius scale">
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-1">
              {radiusScale.map((item) => <ScaleCard key={item.name} {...item} />)}
            </div>
          </SpecCard>
          <SpecCard title="Density scale">
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-1">
              {densityScale.map((item) => <ScaleCard key={item.name} {...item} />)}
            </div>
          </SpecCard>
        </div>
      </div>
    </TokenPageSection>
  )
}

function CurrentSettingsMock() {
  const [autoCompact, setAutoCompact] = useState(true)
  const [model, setModel] = useState('default')

  return (
    <SettingsCanvas>
      <SettingsSectionHeader title="Context" description="A narrow settings page should sit inside the right content area." />
      <ContractSection title="Auto compact" description="Current row language: label and helper text on the left, control on the right.">
        <SettingRow
          title="Enable auto compact"
          description="Compress earlier turns when context usage grows."
          control={<PillToggle checked={autoCompact} onChange={setAutoCompact} />}
        />
        <div className="border-t border-[var(--c-border-subtle)]" />
        <SettingRow
          title="Tool model"
          description="Used by background utility tasks."
          control={(
            <div className="w-[240px]">
              <TokenSelect
                value={model}
                options={[
                  { value: 'default', label: 'Platform default' },
                  { value: 'fast', label: 'Fast model' },
                  { value: 'deep', label: 'Deep model' },
                ]}
                onChange={setModel}
              />
            </div>
          )}
        />
      </ContractSection>
    </SettingsCanvas>
  )
}

function MiniPreviewCard({
  title,
  subtitle,
  hoverSubtitle,
  preview,
  icon,
}: {
  title: string
  subtitle: string
  hoverSubtitle: string
  preview: ReactNode
  icon?: ReactNode
}) {
  const [hovered, setHovered] = useState(false)
  const [miniHovered, setMiniHovered] = useState(false)

  return (
    <div
      className="group/card cursor-pointer rounded-xl"
      style={{ border: '0.5px solid var(--c-border-subtle)', background: 'var(--c-bg-menu)' }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => { setHovered(false); setMiniHovered(false) }}
    >
      <div className="flex gap-4 p-4">
        <div
          className="shrink-0 overflow-hidden rounded-lg transition-shadow duration-200"
          style={{
            width: 120,
            height: 80,
            border: '0.5px solid var(--c-border-subtle)',
            background: 'var(--c-bg-page)',
            boxShadow: hovered
              ? '0 3px 6px -2px rgba(0,0,0,0.08), 1px 0 3px -2px rgba(0,0,0,0.03), -1px 0 3px -2px rgba(0,0,0,0.03)'
              : '0 1px 3px -1px rgba(0,0,0,0.04)',
          }}
          onMouseEnter={() => setMiniHovered(true)}
          onMouseLeave={() => setMiniHovered(false)}
        >
          <div
            className="overflow-hidden transition-all duration-200"
            style={{
              padding: '10px 0 0 12px',
              fontSize: 8,
              lineHeight: '11px',
              letterSpacing: '-0.01em',
              color: hovered ? 'var(--c-text-secondary)' : 'var(--c-text-tertiary)',
              maxHeight: 80,
              transformOrigin: 'top left',
              transform: miniHovered ? 'scale(1.12)' : 'scale(1)',
              WebkitMaskImage: 'linear-gradient(to bottom, black 40%, transparent 90%), linear-gradient(to left, transparent 0px, black 8px)',
              maskImage: 'linear-gradient(to bottom, black 40%, transparent 90%), linear-gradient(to left, transparent 0px, black 8px)',
              WebkitMaskComposite: 'source-in',
              maskComposite: 'intersect',
            }}
          >
            {preview}
          </div>
        </div>

        <div className="flex min-w-0 flex-1 flex-col justify-center overflow-hidden">
          <p className="flex items-center gap-1.5 text-sm text-[var(--c-text-heading)]" style={{ fontWeight: 450 }}>
            {icon}
            {title}
          </p>
          <div className="relative h-[18px] overflow-hidden">
            <p
              className="absolute inset-0 text-[11px] text-[var(--c-text-muted)] transition-all duration-150 ease-out"
              style={{
                transform: hovered ? 'translateX(-16px)' : 'translateX(0)',
                opacity: hovered ? 0 : 1,
              }}
            >
              {subtitle}
            </p>
            <p
              className="absolute inset-0 text-[11px] transition-all duration-150 ease-out"
              style={{
                color: 'var(--c-text-muted)',
                transform: hovered ? 'translateX(0)' : 'translateX(16px)',
                opacity: hovered ? 1 : 0,
              }}
            >
              {hoverSubtitle}
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}

function NotebookMock() {
  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <div className="flex flex-col gap-4">
        <div
          className="flex flex-col gap-3 rounded-xl p-4"
          style={{ border: '0.5px solid var(--c-border-subtle)', background: 'var(--c-bg-menu)' }}
        >
          <textarea
            value=""
            readOnly
            placeholder="Add a note..."
            rows={4}
            className="w-full resize-none rounded-lg px-3 py-2.5 text-sm text-[var(--c-text-primary)] placeholder:text-[var(--c-text-muted)] outline-none"
            style={{ border: '0.5px solid var(--c-border-subtle)', background: 'var(--c-bg-input)' }}
          />
          <button
            className="flex w-full items-center justify-center gap-1.5 rounded-lg py-2.5 text-sm font-medium transition-[filter] hover:[filter:brightness(1.08)] disabled:opacity-40"
            style={{ background: 'var(--c-btn-bg)', color: 'var(--c-btn-text)' }}
          >
            <Plus size={14} />
            Add
          </button>
        </div>
        <MiniPreviewCard
          title="Notebook"
          subtitle="8 entries"
          hoverSubtitle="View and edit"
          preview={
            <>
              Speaking rule
              {'\n'}Say less, avoid repeating.
              {'\n'}Mint milkshake
              {'\n'}Active group member with an AI pet named Mango Milk.
            </>
          }
        />
      </div>

      <div className="rounded-xl border-[0.5px] border-[var(--c-border-subtle)] bg-[var(--c-bg-menu)] p-4">
        <div className="flex flex-col gap-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Brain size={15} className="text-[var(--c-text-secondary)]" />
              <h4 className="text-sm font-semibold text-[var(--c-text-heading)]">Memory entries</h4>
              <span className="inline-flex items-center rounded-md bg-[var(--c-bg-deep)] px-2 py-0.5 text-xs font-medium text-[var(--c-text-muted)]">
                2
              </span>
            </div>
            <div className="flex items-center gap-1">
              <button className="shrink-0 rounded-lg p-1.5 text-[var(--c-text-muted)] transition-colors hover:text-[var(--c-text-secondary)]">
                <RefreshCw size={14} />
              </button>
              <button className="flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs text-red-400 transition-colors hover:bg-red-500/10">
                <Trash2 size={12} />
                Clear all
              </button>
            </div>
          </div>

          <div
            className="flex items-center gap-2 rounded-lg px-3 py-2"
            style={{ border: '0.5px solid var(--c-border-subtle)', background: 'var(--c-bg-input)' }}
          >
            <Search size={14} className="shrink-0 text-[var(--c-text-muted)]" />
            <input
              type="text"
              readOnly
              placeholder="Search notebook"
              className="min-w-0 flex-1 bg-transparent text-sm text-[var(--c-text-primary)] placeholder:text-[var(--c-text-muted)] outline-none"
            />
          </div>

          <div className="flex flex-col gap-2">
            {[
              ['Speaking rule', 'Say less, avoid repeating. Join a group conversation only when it adds real value.', 'general'],
              ['Mint milkshake', 'Active group member with an AI pet named Mango Milk.', 'profile'],
            ].map(([title, content, category]) => (
              <div
                key={title}
                className="group relative rounded-xl"
                style={{ border: '1px solid var(--c-border-subtle)', background: 'var(--c-bg-menu)' }}
              >
                <div className="flex items-start gap-3 px-4 py-3">
                  <div className="flex min-w-0 flex-1 flex-col gap-1">
                    <div className="notebook-entry-md prose-sm max-w-none text-sm text-[var(--c-text-primary)]">
                      <p className="m-0 font-medium text-[var(--c-text-heading)]">{title}</p>
                      <p className="m-0">{content}</p>
                    </div>
                  </div>
                  <div className="mt-0.5 flex shrink-0 flex-col items-end gap-1 opacity-0 transition-opacity duration-100 group-hover:opacity-100">
                    <div className="flex items-center gap-0.5">
                      <span className="inline-flex items-center rounded-md bg-[var(--c-bg-deep)] px-1.5 py-px text-[10px] font-medium leading-tight text-[var(--c-text-muted)]">
                        {category}
                      </span>
                    </div>
                    <div className="flex items-center gap-0.5">
                      <span className="text-[10px] text-[var(--c-text-muted)]">today</span>
                      <button className="rounded-lg p-1 text-[var(--c-text-muted)] transition-colors hover:text-red-400">
                        <Trash2 size={12} />
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

function ImpressionMock() {
  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Brain size={15} className="text-[var(--c-text-secondary)]" />
            <h4 className="text-sm font-semibold text-[var(--c-text-heading)]">Impression</h4>
          </div>
          <button
            type="button"
            className="inline-flex items-center justify-center gap-1.5 rounded-lg px-2.5 py-1 text-xs font-medium text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)]"
            style={{ border: '0.5px solid var(--c-border-subtle)' }}
          >
            <Check size={13} />
            Done
          </button>
        </div>
        <MiniPreviewCard
          title="Impression"
          subtitle="updated 3h ago"
          hoverSubtitle="View and edit"
          preview={
            <>
              User prefers quiet, direct replies.
              {'\n'}They care about coherent UI grammar.
              {'\n'}Provider settings are a core workflow.
            </>
          }
        />
      </div>

      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <FileText size={15} className="text-[var(--c-text-secondary)]" />
            <h4 className="text-sm font-semibold text-[var(--c-text-heading)]">Memories</h4>
          </div>
          <button
            type="button"
            className="inline-flex items-center justify-center gap-1.5 rounded-lg px-2.5 py-1 text-xs font-medium text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)]"
            style={{ border: '0.5px solid var(--c-border-subtle)' }}
          >
            <RefreshCw size={13} />
            Rebuild
          </button>
        </div>
        <MiniPreviewCard
          title="Memories"
          subtitle="7 memories"
          hoverSubtitle="View and edit"
          preview={
            <>
              UI grammar discussion
              {'\n'}Provider settings
              {'\n'}Notebook entries
              {'\n'}Design tokens
            </>
          }
        />
      </div>
    </div>
  )
}

function ProviderMasterDetailMock() {
  const [active, setActive] = useState('tokenflux')

  return (
    <div className="flex min-h-[560px] min-w-0 overflow-hidden border border-[var(--c-border-subtle)] bg-[var(--c-bg-page)]">
      <div
        className="flex w-[220px] shrink-0 flex-col overflow-hidden max-[1230px]:w-[180px] xl:w-[240px]"
        style={{ borderRight: '0.5px solid var(--c-border-subtle)' }}
      >
        <div className="flex-1 overflow-y-auto px-2 py-1">
          <div className="flex flex-col gap-[3px]">
            {[
              ['tokenflux', 'token flow mimo'],
              ['xiaomi', 'xiaomi mimo'],
              ['image', 'tokenflow image'],
              ['opencode', 'opencode go'],
              ['seefs', 'seefs'],
              ['deepseek', 'deepseek'],
              ['kimi', 'kimi'],
              ['openai', 'openai'],
              ['gemini', 'gemini'],
              ['didl', 'didl'],
              ['tokenflux-dev', 'tokenflux.dev'],
              ['cery', 'cery'],
              ['openrouter', 'openrouter'],
              ['minimax', 'minimax'],
            ].map(([key, label]) => (
              <button
                key={key}
                onClick={() => setActive(key)}
                className={[
                  'flex h-[38px] items-center gap-2 rounded-lg px-2.5 text-left text-[14px] font-medium transition-all duration-[120ms] active:scale-[0.96]',
                  active === key
                    ? 'rounded-[10px] bg-[var(--c-bg-deep)] text-[var(--c-text-heading)]'
                    : 'text-[var(--c-text-secondary)] hover:bg-[var(--c-bg-deep)] hover:text-[var(--c-text-heading)]',
                ].join(' ')}
              >
                <span className="min-w-0 flex-1 truncate">{label}</span>
                {(key === 'opencode' || key === 'tokenflux-dev') && (
                  <span className="shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-medium text-[var(--c-text-muted)]" style={{ background: 'var(--c-bg-sub)' }}>
                    本地
                  </span>
                )}
              </button>
            ))}
          </div>
        </div>
        <div className="border-t border-[var(--c-border-subtle)] px-3 py-3">
          <button
            className="flex h-10 w-full items-center justify-center gap-1.5 rounded-lg text-[13px] font-medium text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-deep)]"
            style={{ border: '0.5px solid var(--c-border-subtle)' }}
          >
            <Plus size={14} />
            添加供应商
          </button>
        </div>
      </div>

      <div className="min-w-0 flex-1 overflow-y-auto p-4 max-[1230px]:p-3 sm:p-5">
        <div className="mx-auto min-w-0 max-w-2xl space-y-6">
          <h3 className="text-base font-semibold text-[var(--c-text-primary)]">token flow mimo</h3>

          <div className="space-y-4">
            <div>
              <label className="mb-1 block text-xs font-medium text-[var(--c-text-tertiary)]">类型</label>
              <button
                type="button"
                className="flex w-full items-center justify-between rounded-lg bg-[var(--c-bg-input)] px-3 py-1.5 text-sm text-[var(--c-text-primary)] transition-colors hover:bg-[var(--c-bg-deep)]"
                style={{ border: '1px solid var(--c-border-subtle)' }}
              >
                <span className="truncate">OpenAI (Responses)</span>
                <ChevronDownIcon />
              </button>
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-[var(--c-text-tertiary)]">供应商名称</label>
              <SettingsInput value="token flow mimo" readOnly />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-[var(--c-text-tertiary)]">API Key</label>
              <SettingsInput type="password" value="sk-11bd2**************************************" readOnly />
              <p className="mt-1 text-xs text-[var(--c-text-muted)]">sk-11bd2********</p>
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-[var(--c-text-tertiary)]">Base URL</label>
              <SettingsInput value="https://tokenflux.dev/v1" readOnly />
            </div>
          </div>

          <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--c-border-subtle)] pb-4">
            <button className="inline-flex items-center gap-1.5 rounded-lg border border-[var(--c-border-subtle)] px-3 py-1.5 text-xs font-medium text-[var(--c-text-muted)] transition-colors duration-150 hover:border-red-500/30 hover:text-red-500">
              <Trash2 size={12} />
            </button>
            <button
              className="flex items-center justify-center rounded-lg px-4 py-1.5 text-sm font-medium text-[var(--c-btn-text)] transition-[filter] duration-150 hover:[filter:brightness(1.12)] active:[filter:brightness(0.95)]"
              style={{ background: 'var(--c-btn-bg)' }}
            >
              保存
            </button>
          </div>

          <div>
            <div className="flex flex-wrap items-center justify-between gap-3">
              <h4 className="text-sm font-medium text-[var(--c-text-primary)]">模型</h4>
              <div className="flex flex-wrap items-center gap-2">
                <button
                  type="button"
                  className="inline-flex h-8 items-center justify-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-[var(--c-text-muted)] transition-colors hover:border-red-500/30 hover:text-red-500"
                  style={{ border: '0.5px solid var(--c-border-subtle)' }}
                >
                  <Trash2 size={12} />
                </button>
                <button
                  type="button"
                  className="button-secondary inline-flex h-8 items-center justify-center gap-1.5 rounded-lg px-3 text-sm font-medium text-[var(--c-text-secondary)] transition-colors"
                  style={{ border: '0.5px solid var(--c-border-subtle)' }}
                >
                  <Download size={12} />
                </button>
                <button
                  type="button"
                  className="button-secondary inline-flex h-8 items-center justify-center gap-1.5 rounded-lg px-3 text-sm font-medium text-[var(--c-text-secondary)] transition-colors"
                  style={{ border: '0.5px solid var(--c-border-subtle)' }}
                >
                  <Zap size={12} />
                  测试
                </button>
                <button
                  className="button-primary inline-flex h-8 items-center justify-center gap-1.5 rounded-lg px-4 text-sm font-medium text-[var(--c-btn-text)] transition-[filter]"
                  style={{ background: 'var(--c-btn-bg)' }}
                >
                  添加模型
                </button>
              </div>
            </div>

            <div className="mt-3">
              <SettingsInput placeholder="搜索供应商..." />
            </div>

            <div className="mt-2 space-y-1 overflow-y-auto" style={{ maxHeight: '320px' }}>
              {['mimo-v2.5', 'mimo-v2.5-pro'].map((model) => (
                <div
                  key={model}
                  className="group flex flex-wrap items-center justify-between gap-2 rounded-lg border border-[var(--c-border-subtle)] px-4 py-2.5"
                  style={{ contentVisibility: 'auto', containIntrinsicBlockSize: '52px' }}
                >
                  <div className="min-w-0 flex flex-1 items-center gap-1.5">
                    <p className="truncate text-sm font-medium text-[var(--c-text-primary)]">{model}</p>
                  </div>
                  <div className="flex w-full shrink-0 items-center justify-end gap-1.5 sm:w-auto">
                    <PillToggle checked onChange={() => {}} />
                    <button className="rounded-md p-1.5 text-[var(--c-text-muted)] transition-colors duration-150 hover:bg-[var(--c-bg-sub)] hover:text-[var(--c-text-secondary)]">
                      <SlidersHorizontal size={14} />
                    </button>
                    <button className="rounded-md p-1.5 text-[var(--c-text-muted)] transition-colors duration-150 hover:bg-[var(--c-bg-sub)] hover:text-red-500">
                      <Trash2 size={14} />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

function ChevronDownIcon() {
  return (
    <svg className="ml-2 h-[13px] w-[13px] shrink-0 text-[var(--c-text-muted)]" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
      <path d="m6 9 6 6 6-6" />
    </svg>
  )
}

function PrimitivesPreview() {
  const [enabled, setEnabled] = useState(true)
  const [model, setModel] = useState('default')
  const [mode, setMode] = useState('system')

  return (
    <TokenPageSection
      eyebrow="Primitives"
      title="基础控件合约"
      description="按钮、输入、开关、状态和分段控件必须来自同一套权重规则，先在这里调，再进入真实页面。"
    >
      <RuleGrid rules={primitiveRules} />

      <div className="grid gap-4 lg:grid-cols-2">
        <SpecCard title="Buttons">
          <div className="flex flex-wrap items-center gap-2">
            <SettingsButton variant="primary">Save</SettingsButton>
            <SettingsButton variant="secondary" icon={<Plus size={14} />}>Add provider</SettingsButton>
            <SettingsButton variant="secondary" icon={<RefreshCw size={14} />}>Refresh</SettingsButton>
            <CopyStateButton />
            <SettingsButton variant="danger" icon={<Trash2 size={14} />}>Delete</SettingsButton>
            <SettingsButton disabled>Disabled</SettingsButton>
          </div>
          <div className="flex items-center gap-1">
            <IconButton label="Settings"><Settings size={15} /></IconButton>
            <IconButton label="Refresh"><RefreshCw size={15} /></IconButton>
            <IconButton label="Delete" danger><Trash2 size={15} /></IconButton>
          </div>
        </SpecCard>

        <SpecCard title="Inputs">
          <div className="grid gap-3 sm:grid-cols-2">
            <div>
              <div className="mb-1.5 text-xs font-medium text-[var(--c-text-secondary)]">Text field</div>
              <TokenInput placeholder="https://api.example.com/v1" />
            </div>
            <div>
              <div className="mb-1.5 text-xs font-medium text-[var(--c-text-secondary)]">Select</div>
              <TokenSelect
                value={model}
                options={[
                  { value: 'default', label: 'Platform default' },
                  { value: 'fast', label: 'Fast model' },
                  { value: 'deep', label: 'Deep model' },
                ]}
                onChange={setModel}
              />
            </div>
            <div className="sm:col-span-2">
              <div className="mb-1.5 text-xs font-medium text-[var(--c-text-secondary)]">Search field</div>
              <SearchInput placeholder="Search models" />
            </div>
          </div>
        </SpecCard>

        <SpecCard title="Switches and segmented controls">
          <div className="flex flex-wrap items-center gap-4">
            <div className="flex items-center gap-3">
              <PillToggle checked={enabled} onChange={setEnabled} />
              <span className="text-sm text-[var(--c-text-secondary)]">Enabled</span>
            </div>
            <CapsuleSegmentedControl
              options={[
                { value: 'system', label: 'System' },
                { value: 'light', label: 'Light' },
                { value: 'dark', label: 'Dark' },
              ]}
              value={mode}
              onChange={setMode}
            />
          </div>
        </SpecCard>

        <SpecCard title="Status badges">
          <div className="flex flex-wrap gap-2">
            <StatusBadge variant="success">ready</StatusBadge>
            <StatusBadge variant="neutral">local</StatusBadge>
            <StatusBadge variant="warning">missing</StatusBadge>
            <StatusBadge variant="error">error</StatusBadge>
          </div>
        </SpecCard>
      </div>
    </TokenPageSection>
  )
}

function CompositionsPreview() {
  return (
    <TokenPageSection
      eyebrow="Compositions"
      title="真实设置样本"
      description="这些样本贴近当前 Arkloop desktop 页面结构，用来检查 token 是否能解释真实界面。"
    >
      <RuleGrid rules={compositionRules} />

      <div className="grid gap-4">
        <SpecCard title="Simple settings sample">
          <CurrentSettingsMock />
        </SpecCard>

        <SpecCard title="Provider master-detail sample">
          <ProviderMasterDetailMock />
        </SpecCard>

        <SpecCard title="Notebook sample">
          <NotebookMock />
        </SpecCard>

        <SpecCard title="Impression and memories sample">
          <ImpressionMock />
        </SpecCard>

        <SpecCard title="Provider selection sample">
          <div className="flex flex-wrap gap-3">
            <ProviderSelectCard
              title="Nowledge"
              description="Structured notebook memory"
              selected
              onSelect={() => {}}
            />
            <ProviderSelectCard
              title="OpenViking"
              description="Semantic vector memory"
              selected={false}
              onSelect={() => {}}
            />
          </div>
        </SpecCard>
      </div>
    </TokenPageSection>
  )
}

function ModalActionButton({
  variant,
  children,
  onClick,
}: {
  variant: 'primary' | 'secondary'
  children: ReactNode
  onClick?: () => void
}) {
  const classes = variant === 'primary'
    ? 'min-w-[54px] bg-[var(--c-btn-bg)] text-[var(--c-btn-text)] hover:[box-shadow:inset_0_0_0_999px_rgba(255,255,255,0.07),0_0_0_0.2px_var(--c-btn-bg)] active:[box-shadow:inset_0_0_0_999px_rgba(0,0,0,0.04)]'
    : 'min-w-[54px] border-[0.65px] [border-color:color-mix(in_srgb,var(--c-border)_91%,var(--c-text-primary)_9%)] bg-[var(--c-bg-input)] text-[color-mix(in_srgb,var(--c-text-secondary)_72%,var(--c-text-primary)_28%)] hover:border-transparent hover:bg-[var(--c-bg-deep)] hover:text-[var(--c-text-primary)]'

  return (
    <button
      type="button"
      onClick={onClick}
      className={[
        'inline-flex h-[35px] items-center justify-center rounded-[7px] px-3 text-sm font-[450] transition-colors duration-[180ms]',
        classes,
      ].join(' ')}
    >
      {children}
    </button>
  )
}

function ProviderModalPreview({ open, onClose }: { open: boolean; onClose: () => void }) {
  const [providerType, setProviderType] = useState('responses')

  if (!open) return null

  const fieldLabelCls = 'mb-1 block text-[12px] font-medium text-[var(--c-text-secondary)]'
  const modalFieldCls = 'h-[35px] px-3.5 text-sm'

  return createPortal(
    <div
      className="overlay-fade-in fixed inset-0 z-[60] flex items-center justify-center"
      style={{ background: 'rgba(0, 0, 0, 0.58)' }}
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <div
        className="design-token-modal-enter flex w-[510px] flex-col rounded-[17px] p-7"
        style={{ background: 'var(--c-bg-menu)', border: '0.5px solid var(--c-border-subtle)' }}
      >
        <div className="flex items-center justify-between">
          <h3 className="text-[19px] font-semibold leading-none text-[var(--c-text-heading)]">添加供应商</h3>
          <button
            type="button"
            aria-label="关闭"
            onClick={onClose}
            className="-mr-2 flex h-8 w-8 items-center justify-center rounded-[7px] text-[color-mix(in_srgb,var(--c-border)_72%,var(--c-text-primary)_28%)] transition-colors duration-[160ms] hover:bg-[var(--c-bg-sub)] hover:text-[var(--c-text-primary)]"
          >
            <X size={16} />
          </button>
        </div>

        <div className="mt-6 grid grid-cols-2 gap-x-4 gap-y-4">
          <div>
            <label className={fieldLabelCls}>供应商名称</label>
            <TokenInput className={modalFieldCls} placeholder="My Provider" />
          </div>
          <div>
            <label className={fieldLabelCls}>类型</label>
            <TokenSelect
              value={providerType}
              onChange={setProviderType}
              triggerClassName={modalFieldCls}
              options={[
                { value: 'responses', label: 'OpenAI (Responses)' },
                { value: 'chat', label: 'OpenAI compatible' },
                { value: 'local', label: 'Local provider' },
              ]}
            />
          </div>
          <div className="col-span-2">
            <label className={fieldLabelCls}>API Key</label>
            <TokenInput className={modalFieldCls} type="password" placeholder="sk-..." />
          </div>
          <div className="col-span-2">
            <label className={fieldLabelCls}>Base URL</label>
            <TokenInput className={modalFieldCls} placeholder="https://api.openai.com/v1" />
          </div>
        </div>

        <div className="mt-7 flex items-center justify-end gap-2">
          <ModalActionButton variant="secondary" onClick={onClose}>取消</ModalActionButton>
          <ModalActionButton variant="primary">保存</ModalActionButton>
        </div>
      </div>
    </div>,
    document.body,
  )
}

function OverlaysPreview() {
  const [modalOpen, setModalOpen] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)

  return (
    <TokenPageSection
      eyebrow="Overlays"
      title="弹出层样式"
      description="弹窗和确认框在设置页里高频出现，先在这里统一 surface、overlay、圆角、阴影和进出场。"
    >
      <RuleGrid rules={overlayRules} />

      <SpecCard title="Dialog triggers">
        <div className="flex flex-wrap items-center gap-2">
          <SettingsButton variant="secondary" icon={<Plus size={14} />} onClick={() => setModalOpen(true)}>
            添加供应商
          </SettingsButton>
          <SettingsButton variant="danger" icon={<Trash2 size={14} />} onClick={() => setConfirmOpen(true)}>
            删除全部模型
          </SettingsButton>
        </div>
      </SpecCard>

      <ProviderModalPreview open={modalOpen} onClose={() => setModalOpen(false)} />
      <ConfirmDialog
        open={confirmOpen}
        onClose={() => setConfirmOpen(false)}
        onConfirm={() => setConfirmOpen(false)}
        title="删除全部模型"
        message="这会删除该供应商下的所有模型，确定继续？"
        confirmLabel="Delete all"
      />
    </TokenPageSection>
  )
}

function PageGrammarPreview() {
  return (
    <TokenPageSection
      eyebrow="Page Grammar"
      title="先选结构，再选样式"
      description="新页面先判断对象语义，再选对应 pattern。不要从按钮、卡片、颜色开始猜。"
    >
      <div className="grid gap-4 lg:grid-cols-2">
        {pagePatterns.map((pattern) => (
          <div key={pattern.title} className={`${previewPanel} flex flex-col gap-4`}>
            <div className="flex items-start gap-3">
              <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-[var(--c-bg-deep)] text-[var(--c-text-muted)]">
                {pattern.icon}
              </div>
              <div className="min-w-0">
                <h4 className="text-sm font-semibold text-[var(--c-text-heading)]">{pattern.title}</h4>
                <p className="mt-1 text-xs leading-relaxed text-[var(--c-text-muted)]">{pattern.description}</p>
              </div>
            </div>
            <div className="grid gap-2">
              {pattern.slots.map((slot) => (
                <div key={slot} className="flex h-8 items-center rounded-lg border border-[var(--c-border-subtle)] bg-[var(--c-bg-page)] px-3 text-xs text-[var(--c-text-secondary)]">
                  {slot}
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>

      <SpecCard title="Danger zone">
        <div
          className="rounded-xl border p-4"
          style={{ background: dangerSurface, borderColor: dangerBorder }}
        >
          <div className="flex flex-wrap items-center justify-between gap-4">
            <div className="flex min-w-0 items-start gap-3">
              <AlertTriangle size={16} className="mt-0.5 shrink-0" style={{ color: dangerText }} />
              <div>
                <div className="text-sm font-medium text-[var(--c-text-heading)]">Delete provider</div>
                <div className="mt-0.5 text-xs leading-relaxed text-[var(--c-text-muted)]">危险操作放在页面末尾，常驻态保持低权重。</div>
              </div>
            </div>
            <SettingsButton variant="danger" icon={<Trash2 size={14} />}>Delete</SettingsButton>
          </div>
        </div>
      </SpecCard>
    </TokenPageSection>
  )
}

type PreviewToast = {
  id: string
  variant: ToastVariant
  message: string
  exiting?: boolean
}

function ToastPreviewItem({
  variant,
  message,
  exiting = false,
  onDismiss,
}: {
  variant: ToastVariant
  message: string
  exiting?: boolean
  onDismiss?: () => void
}) {
  return (
    <div
      className={[
        'flex min-h-[32px] items-center gap-2 rounded-[6.5px] px-3.5 py-2 [background-clip:padding-box]',
        exiting ? 'toast-exit' : 'toast-enter',
      ].join(' ')}
      style={{
        border: `0.65px solid ${toastBorder[variant]}`,
        background: toastSurface[variant],
      }}
    >
      <span className={`flex-1 text-sm ${toastText[variant]}`}>{message}</span>
      <button
        type="button"
        onClick={onDismiss}
        className="-mr-1 flex h-6 w-6 shrink-0 items-center justify-center rounded-[6px] text-[color-mix(in_srgb,var(--c-border)_72%,var(--c-text-primary)_28%)] transition-colors duration-150 hover:bg-[var(--c-bg-sub)] hover:text-[var(--c-text-primary)]"
      >
        <X size={14} />
      </button>
    </div>
  )
}

function FeedbackPreview() {
  const [variant, setVariant] = useState<ToastVariant>('success')
  const messages: Record<ToastVariant, string> = {
    success: '已保存',
    error: '保存失败',
    warn: '需要重新连接',
    neutral: '设置已更新',
  }
  const [previewToasts, setPreviewToasts] = useState<PreviewToast[]>([
    { id: 'initial', variant: 'success', message: messages.success },
  ])

  const insertToast = () => {
    const id = `${Date.now()}-${Math.random()}`
    setPreviewToasts((current) => [
      ...current.filter((toast) => !toast.exiting).slice(-3),
      { id, variant, message: messages[variant] },
    ])
  }

  const dismissToast = (id: string) => {
    setPreviewToasts((current) => current.map((toast) => (
      toast.id === id ? { ...toast, exiting: true } : toast
    )))
    window.setTimeout(() => {
      setPreviewToasts((current) => current.filter((toast) => toast.id !== id))
    }, 200)
  }

  const dismissLatest = () => {
    const latest = [...previewToasts].reverse().find((toast) => !toast.exiting)
    if (latest) dismissToast(latest.id)
  }

  return (
    <TokenPageSection
      eyebrow="Global Feedback"
      title="Toast notifications"
      description="短暂反馈显示在右上角，直接使用 shared ToastProvider 的进入和退出动画。"
    >
      <div className="grid gap-4 lg:grid-cols-[0.9fr_1.1fr]">
        <SpecCard title="Variants">
          <div className="flex flex-wrap gap-2">
            {(['success', 'neutral', 'warn', 'error'] as ToastVariant[]).map((item) => (
              <SettingsButton
                key={item}
                variant={variant === item ? 'primary' : 'secondary'}
                onClick={() => setVariant(item)}
              >
                {item}
              </SettingsButton>
            ))}
          </div>
          <div className="mt-2 flex flex-col gap-2">
            <ToastPreviewItem variant="success" message="已保存" />
            <ToastPreviewItem variant="neutral" message="设置已更新" />
            <ToastPreviewItem variant="warn" message="需要重新连接" />
            <ToastPreviewItem variant="error" message="保存失败" />
          </div>
        </SpecCard>

        <SpecCard title="Top-right placement">
          <div className="flex flex-wrap gap-2">
            <SettingsButton variant="primary" onClick={insertToast}>
              插入通知
            </SettingsButton>
            <SettingsButton variant="secondary" onClick={dismissLatest}>
              退出最新
            </SettingsButton>
            <SettingsButton variant="secondary" onClick={() => setPreviewToasts([])}>
              清空
            </SettingsButton>
          </div>
          <div className="relative h-[220px] overflow-hidden rounded-xl border-[0.5px] border-[var(--c-border-subtle)] bg-[var(--c-bg-page)]">
            <div className="absolute left-4 top-4 h-8 w-28 rounded-lg bg-[var(--c-bg-sidebar)]" />
            <div className="absolute bottom-4 left-4 right-4 top-14 rounded-xl border-[0.5px] border-[var(--c-border-subtle)] bg-[var(--c-bg-menu)]" />
            <div className="absolute right-4 top-4 flex w-[220px] flex-col gap-2">
              {previewToasts.map((toast) => (
                <ToastPreviewItem
                  key={toast.id}
                  variant={toast.variant}
                  message={toast.message}
                  exiting={toast.exiting}
                  onDismiss={() => dismissToast(toast.id)}
                />
              ))}
            </div>
          </div>
          <div className="grid gap-2 text-xs text-[var(--c-text-muted)] sm:grid-cols-2">
            <div>Position: right-4 top-4</div>
            <div>Duration: 4000ms</div>
            <div>Enter: toast-slide-in</div>
            <div>Exit: toast-slide-out</div>
          </div>
        </SpecCard>
      </div>
    </TokenPageSection>
  )
}

export function DesignTokensSettings() {
  return (
    <div className={pageShell}>
      <SettingsSectionHeader
        title="Design Tokens"
        description="当前样式、真实样本和页面语法的可视化合约。"
      />

      <FoundationsPreview />
      <PrimitivesPreview />
      <CompositionsPreview />
      <OverlaysPreview />
      <FeedbackPreview />
      <PageGrammarPreview />
    </div>
  )
}
