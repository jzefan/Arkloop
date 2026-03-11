import { useState, useRef, useEffect } from 'react'
import {
  ChevronDown,
  Check,
  Globe,
  Monitor,
  ExternalLink,
} from 'lucide-react'
import { useLocale } from '../contexts/LocaleContext'

// -- types --

export type StepStatus = 'done' | 'active' | 'pending'

export type ProgressStep = {
  id: string
  label: string
  status: StepStatus
}

export type WorkspaceFile = {
  name: string
  label?: string
  ext?: string
}

export type Connector = {
  name: string
  icon: 'globe' | 'monitor'
}

export type ClawRightPanelProps = {
  steps: ProgressStep[]
  workspaceName: string
  files: WorkspaceFile[]
  connectors: Connector[]
}

// -- mock data --

const MOCK_STEPS: ProgressStep[] = [
  { id: '1', label: 'Analyze requirements', status: 'done' },
  { id: '2', label: 'Search codebase', status: 'done' },
  { id: '3', label: 'Implement changes', status: 'pending' },
]

const MOCK_FILES: WorkspaceFile[] = [
  { name: 'CLAUDE.md', label: 'Instructions', ext: 'md' },
  { name: 'sub-agent-architecture.md', ext: 'md' },
  { name: 'soul.md', ext: 'md' },
  { name: 'platform-agent-architecture.md', ext: 'md' },
  { name: 'navigation.ts', ext: 'ts' },
]

const MOCK_CONNECTORS: Connector[] = [
  { name: 'Web search', icon: 'globe' },
]

// -- animated height wrapper --

function AnimatedHeight({
  open,
  children,
}: {
  open: boolean
  children: React.ReactNode
}) {
  const contentRef = useRef<HTMLDivElement>(null)
  const [height, setHeight] = useState<number | undefined>(open ? undefined : 0)
  const firstRender = useRef(true)

  useEffect(() => {
    if (firstRender.current) {
      firstRender.current = false
      if (!open) setHeight(0)
      return
    }
    const el = contentRef.current
    if (!el) return
    if (open) {
      const h = el.scrollHeight
      setHeight(h)
      const timer = setTimeout(() => setHeight(undefined), 220)
      return () => clearTimeout(timer)
    } else {
      setHeight(el.scrollHeight)
      requestAnimationFrame(() => {
        requestAnimationFrame(() => setHeight(0))
      })
    }
  }, [open])

  return (
    <div
      ref={contentRef}
      style={{
        height: height === undefined ? 'auto' : height,
        transition: 'height 220ms ease',
        overflow: 'hidden',
      }}
    >
      {children}
    </div>
  )
}

// -- card wrapper --

function Card({
  title,
  badge,
  rightAction,
  defaultOpen = true,
  children,
}: {
  title: string
  badge?: string
  rightAction?: React.ReactNode
  defaultOpen?: boolean
  children: React.ReactNode
}) {
  const [open, setOpen] = useState(defaultOpen)

  return (
    <div
      style={{
        borderRadius: 12,
        border: '0.5px solid var(--c-claw-card-border)',
        background: 'var(--c-bg-page)',
        overflow: 'hidden',
      }}
    >
      <button
        onClick={() => setOpen((v) => !v)}
        style={{
          display: 'flex',
          width: '100%',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '14px 16px',
          background: 'transparent',
          border: 'none',
          cursor: 'pointer',
        }}
      >
        <span
          style={{
            fontSize: '14px',
            fontWeight: 600,
            color: 'var(--c-text-primary)',
            letterSpacing: '-0.2px',
          }}
        >
          {title}
        </span>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          {badge && (
            <span style={{ fontSize: '13px', color: 'var(--c-text-muted)' }}>
              {badge}
            </span>
          )}
          {rightAction}
          <span
            style={{
              color: 'var(--c-text-muted)',
              display: 'flex',
              transition: 'transform 200ms ease',
              transform: open ? 'rotate(0deg)' : 'rotate(-90deg)',
            }}
          >
            <ChevronDown size={16} />
          </span>
        </div>
      </button>
      <AnimatedHeight open={open}>
        <div style={{ padding: '0 16px 16px' }}>
          {children}
        </div>
      </AnimatedHeight>
    </div>
  )
}

// -- progress panel (1.5): vertical git-graph style --

function ProgressPanel({ steps }: { steps: ProgressStep[] }) {
  const { t } = useLocale()

  if (steps.length === 0) {
    return (
      <p style={{ fontSize: '13px', color: 'var(--c-text-muted)', margin: 0, lineHeight: 1.5 }}>
        {t.claw.progressEmpty}
      </p>
    )
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column' }}>
      {steps.map((step, i) => (
        <div key={step.id} style={{ display: 'flex', gap: 10 }}>
          {/* left rail: icon + vertical line */}
          <div
            style={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              width: 20,
              flexShrink: 0,
            }}
          >
            {step.status === 'done' ? (
              <div
                style={{
                  width: 20,
                  height: 20,
                  borderRadius: '50%',
                  border: '1.5px solid var(--c-claw-step-done)',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  flexShrink: 0,
                }}
              >
                <Check size={11} color="var(--c-claw-step-done)" strokeWidth={2.5} />
              </div>
            ) : (
              <div
                style={{
                  width: 20,
                  height: 20,
                  borderRadius: '50%',
                  border: '1.5px solid var(--c-claw-step-pending)',
                  flexShrink: 0,
                }}
              />
            )}
            {i < steps.length - 1 && (
              <div
                style={{
                  width: 1.5,
                  flex: 1,
                  minHeight: 12,
                  background: 'var(--c-claw-step-line)',
                }}
              />
            )}
          </div>
          {/* right: label */}
          <span
            style={{
              fontSize: '13px',
              color: step.status === 'done'
                ? 'var(--c-text-secondary)'
                : 'var(--c-text-muted)',
              lineHeight: '20px',
              paddingBottom: i < steps.length - 1 ? 8 : 0,
            }}
          >
            {step.label}
          </span>
        </div>
      ))}
    </div>
  )
}

// -- document icon with extension badge --

function DocIcon({ ext }: { ext?: string }) {
  const tag = ext?.toUpperCase() ?? ''
  return (
    <div
      style={{
        width: 22,
        height: 26,
        borderRadius: 3,
        border: '0.5px solid var(--c-claw-card-border)',
        background: 'var(--c-claw-file-bg)',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'flex-end',
        padding: '0 0 2px',
        flexShrink: 0,
        position: 'relative',
      }}
    >
      {/* fold corner */}
      <div
        style={{
          position: 'absolute',
          top: 0,
          right: 0,
          width: 6,
          height: 6,
          background: 'var(--c-bg-sub)',
          borderRadius: '0 0 0 1.5px',
        }}
      />
      {/* lines */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 1.5, alignItems: 'center', marginTop: 5 }}>
        <div style={{ width: 10, height: 0.5, background: 'var(--c-claw-step-pending)', borderRadius: 0.5 }} />
        <div style={{ width: 10, height: 0.5, background: 'var(--c-claw-step-pending)', borderRadius: 0.5 }} />
      </div>
      {tag && (
        <span
          style={{
            fontSize: '6px',
            fontWeight: 700,
            color: 'var(--c-text-muted)',
            letterSpacing: '0.2px',
            marginTop: 1,
            lineHeight: 1,
          }}
        >
          {tag}
        </span>
      )}
    </div>
  )
}

// -- working folder panel (1.6) --

function WorkingFolderPanel({ files }: { files: WorkspaceFile[] }) {
  const { t } = useLocale()

  if (files.length === 0) {
    return (
      <p style={{ fontSize: '13px', color: 'var(--c-text-muted)', margin: 0, lineHeight: 1.5 }}>
        {t.claw.workingFolderEmpty}
      </p>
    )
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column' }}>
      {files.map((f) => (
        <div
          key={f.name}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            padding: '6px 2px',
            borderRadius: 6,
            cursor: 'default',
          }}
        >
          <DocIcon ext={f.ext} />
          <span
            style={{
              fontSize: '13px',
              color: 'var(--c-text-secondary)',
              lineHeight: 1.3,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {f.label ? `${f.label} \u00B7 ${f.name}` : f.name}
          </span>
        </div>
      ))}
    </div>
  )
}

// -- context panel (1.7) --

function ConnectorIcon({ icon }: { icon: Connector['icon'] }) {
  const style = { color: 'var(--c-text-muted)' }
  switch (icon) {
    case 'globe': return <Globe size={15} style={style} />
    case 'monitor': return <Monitor size={15} style={style} />
    default: return <Globe size={15} style={style} />
  }
}

function ContextPanel({ connectors }: { connectors: Connector[] }) {
  const { t } = useLocale()

  if (connectors.length === 0) {
    return (
      <p style={{ fontSize: '13px', color: 'var(--c-text-muted)', margin: 0, lineHeight: 1.5 }}>
        {t.claw.contextEmpty}
      </p>
    )
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
      <span style={{ fontSize: '12px', color: 'var(--c-text-muted)', marginBottom: 2 }}>
        {t.claw.toolsCalled}
      </span>
      {connectors.map((c) => (
        <div
          key={c.name}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 10,
            padding: '6px 2px',
            borderRadius: 6,
            cursor: 'default',
          }}
        >
          <div
            style={{
              width: 26,
              height: 26,
              borderRadius: 6,
              border: '0.5px solid var(--c-claw-card-border)',
              background: 'var(--c-claw-file-bg)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              flexShrink: 0,
              overflow: 'hidden',
            }}
          >
            <ConnectorIcon icon={c.icon} />
          </div>
          <span style={{ fontSize: '13px', color: 'var(--c-text-secondary)' }}>
            {c.name}
          </span>
        </div>
      ))}
    </div>
  )
}

// -- main component --

const clawPanelWidth = 300

export function ClawRightPanel({
  steps = MOCK_STEPS,
  workspaceName = 'Arkloop',
  files = MOCK_FILES,
  connectors = MOCK_CONNECTORS,
}: Partial<ClawRightPanelProps>) {
  const { t } = useLocale()
  const doneCount = steps.filter((s) => s.status === 'done').length
  const hasFiles = files.length > 0
  const folderTitle = hasFiles ? workspaceName : t.claw.workingFolder

  return (
    <div
      style={{
        width: clawPanelWidth,
        height: '100%',
        flexShrink: 0,
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        background: 'var(--c-bg-page)',
      }}
    >
      <div
        style={{
          flex: 1,
          overflowY: 'auto',
          padding: '8px 10px',
          display: 'flex',
          flexDirection: 'column',
          gap: 8,
        }}
      >
        <Card
          title={t.claw.progress}
          badge={steps.length > 0 ? `${doneCount} of ${steps.length}` : undefined}
          defaultOpen
        >
          <ProgressPanel steps={steps} />
        </Card>

        <Card
          title={folderTitle}
          rightAction={
            hasFiles ? (
              <ExternalLink
                size={14}
                style={{ color: 'var(--c-text-muted)' }}
                onClick={(e) => e.stopPropagation()}
              />
            ) : undefined
          }
          defaultOpen
        >
          <WorkingFolderPanel files={files} />
        </Card>

        <Card title={t.claw.context} defaultOpen>
          <ContextPanel connectors={connectors} />
        </Card>
      </div>
    </div>
  )
}
