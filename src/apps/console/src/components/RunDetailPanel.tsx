import { useState, useEffect, useCallback, useRef } from 'react'
import { createPortal } from 'react-dom'
import { X, ChevronDown, ChevronRight } from 'lucide-react'
import type { GlobalRun, AdminRunDetail, AdminRunUsageItem, AdminRunUsageAggregate, RunEventRaw } from '../api/runs'
import { getAdminRunDetail, fetchRunEventsOnce } from '../api/runs'
import { TurnView, buildTurns } from './TurnView'
import { Badge, type BadgeVariant } from './Badge'
import { useLocale } from '../contexts/LocaleContext'

function statusVariant(status: string): BadgeVariant {
  switch (status) {
    case 'running': return 'warning'
    case 'completed': return 'success'
    case 'failed': return 'error'
    default: return 'neutral'
  }
}

function formatDuration(ms?: number): string {
  if (ms == null) return '—'
  const secs = Math.floor(ms / 1000)
  if (secs < 60) return `${secs}s`
  const mins = Math.floor(secs / 60)
  return `${mins}m ${secs % 60}s`
}

function formatCost(usd?: number): string {
  if (usd == null) return '—'
  const decimals = Math.abs(usd) < 0.01 ? 6 : 4
  return `$${usd.toFixed(decimals)}`
}

type MetaRowProps = {
  label: string
  value: string | undefined | null
  mono?: boolean
}

function MetaRow({ label, value, mono }: MetaRowProps) {
  if (!value) return null
  return (
    <div className="flex items-baseline gap-3 py-1">
      <span className="w-28 shrink-0 text-xs text-[var(--c-text-muted)]">{label}</span>
      <span className={['text-xs text-[var(--c-text-secondary)]', mono ? 'font-mono' : ''].join(' ')}>
        {value}
      </span>
    </div>
  )
}

type SectionProps = {
  title: string
  badge?: string
  defaultOpen?: boolean
  onOpen?: () => void
  children: React.ReactNode
}

function Section({ title, badge, defaultOpen = true, onOpen, children }: SectionProps) {
  const [open, setOpen] = useState(defaultOpen)
  const triggered = useRef(false)

  const handleToggle = useCallback(() => {
    setOpen((v) => {
      const next = !v
      if (next && !triggered.current && onOpen) {
        triggered.current = true
        onOpen()
      }
      return next
    })
  }, [onOpen])

  return (
    <div className="border-t border-[var(--c-border-console)]">
      <button
        onClick={handleToggle}
        className="flex w-full items-center gap-2 px-4 py-3 text-left transition-colors hover:bg-[var(--c-bg-sub)]"
      >
        <span className="text-[var(--c-text-muted)]">
          {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
        </span>
        <span className="text-xs font-semibold uppercase tracking-wider text-[var(--c-text-muted)]">
          {title}
        </span>
        {badge && (
          <span className="ml-1 text-xs text-[var(--c-text-muted)]">{badge}</span>
        )}
      </button>
      {open && <div className="px-4 pb-4">{children}</div>}
    </div>
  )
}

type Props = {
  run: GlobalRun | null
  accessToken: string
  onClose: () => void
  onOpenRun?: (run: GlobalRun) => void
}

export function RunDetailPanel({ run, accessToken, onClose, onOpenRun }: Props) {
  const [detail, setDetail] = useState<AdminRunDetail | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [events, setEvents] = useState<RunEventRaw[] | null>(null)
  const [eventsLoading, setEventsLoading] = useState(false)
  const { t } = useLocale()
  const rt = t.pages.runs

  // 面板打开时加载 summary
  useEffect(() => {
    if (!run) {
      setDetail(null)
      setEvents(null)
      return
    }
    setDetail(null)
    setEvents(null)
    setDetailLoading(true)
    getAdminRunDetail(run.run_id, accessToken)
      .then(setDetail)
      .catch(() => {/* 静默，面板仍可展示列表数据 */})
      .finally(() => setDetailLoading(false))
  }, [run, accessToken])

  // Conversation 展开时懒加载事件流
  const loadEvents = useCallback(() => {
    if (!run || events !== null) return
    setEventsLoading(true)
    fetchRunEventsOnce(run.run_id, accessToken)
      .then(setEvents)
      .catch(() => setEvents([]))
      .finally(() => setEventsLoading(false))
  }, [run, events, accessToken])

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() },
    [onClose],
  )
  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [handleKeyDown])

  if (!run) return null

  const d = detail
  const turns = events ? buildTurns(events) : []

  const conversationBadge =
    events !== null
      ? `${turns.length} turn${turns.length !== 1 ? 's' : ''}` +
        ((d?.events_stats.tool_calls ?? 0) > 0 ? ` · ${d!.events_stats.tool_calls} tool calls` : '') +
        ((d?.events_stats.provider_fallbacks ?? 0) > 0 ? ` · ${d!.events_stats.provider_fallbacks} fallbacks` : '')
      : d
        ? `${d.events_stats.llm_turns} turn${d.events_stats.llm_turns !== 1 ? 's' : ''}` +
          (d.events_stats.tool_calls > 0 ? ` · ${d.events_stats.tool_calls} tool calls` : '') +
          (d.events_stats.provider_fallbacks > 0 ? ` · ${d.events_stats.provider_fallbacks} fallbacks` : '')
        : undefined

  const rawEventsBadge = d ? `${d.events_stats.total} events` : undefined

  const usageChildren = d?.children ?? []
  const hasUsageBreakdown = usageChildren.length > 0
  const usageAggregate = d?.total_aggregate

  const selfUsageItem: AdminRunUsageItem = {
    run_id: run.run_id,
    org_id: run.org_id,
    thread_id: run.thread_id,
    parent_run_id: run.parent_run_id,
    status: run.status,
    persona_id: d?.persona_id ?? run.persona_id,
    model: d?.model ?? run.model,
    provider_kind: d?.provider_kind,
    credential_name: d?.credential_name,
    agent_config_name: d?.agent_config_name,
    duration_ms: d?.duration_ms ?? run.duration_ms,
    total_input_tokens: d?.total_input_tokens ?? run.total_input_tokens,
    total_output_tokens: d?.total_output_tokens ?? run.total_output_tokens,
    total_cost_usd: d?.total_cost_usd ?? run.total_cost_usd,
    cache_hit_rate: run.cache_hit_rate,
    credits_used: run.credits_used,
    created_at: run.created_at,
    completed_at: d?.completed_at ?? run.completed_at,
    failed_at: d?.failed_at ?? run.failed_at,
  }

  return createPortal(
    <>
      {/* 半透明遮罩 */}
      <div
        className="fixed inset-0 z-40 bg-black/30"
        onClick={onClose}
      />
      {/* 侧边栏 */}
      <div className="fixed inset-y-0 right-0 z-50 flex w-[500px] max-w-full flex-col border-l border-[var(--c-border)] bg-[var(--c-bg-deep2)] shadow-2xl">
        {/* 顶部标题栏 */}
        <div className="flex shrink-0 items-center justify-between border-b border-[var(--c-border-console)] px-4 py-3">
          <div className="flex items-center gap-2">
            <span className="font-mono text-xs text-[var(--c-text-muted)]" title={run.run_id}>
              {run.run_id.slice(0, 12)}…
            </span>
            <Badge variant={statusVariant(run.status)}>{run.status}</Badge>
            {(d?.duration_ms ?? run.duration_ms) != null && (
              <span className="text-xs text-[var(--c-text-muted)]">
                {formatDuration(d?.duration_ms ?? run.duration_ms)}
              </span>
            )}
          </div>
          <button
            onClick={onClose}
            className="rounded p-1 text-[var(--c-text-muted)] transition-colors hover:bg-[var(--c-bg-sub)] hover:text-[var(--c-text-secondary)]"
          >
            <X size={16} />
          </button>
        </div>

        <div className="flex-1 overflow-y-auto">
          {/* OVERVIEW */}
          <Section title={rt.sectionOverview} defaultOpen>
            {detailLoading && !d && (
              <p className="py-2 text-xs text-[var(--c-text-muted)]">{rt.loading}</p>
            )}
            <div className="divide-y divide-[var(--c-border-console)]">
              <div className="pb-2">
                <MetaRow label={rt.labelUser} value={
                  (d?.created_by_user_name ?? run.created_by_user_name)
                    ? `${d?.created_by_user_name ?? run.created_by_user_name}${(d?.created_by_email ?? run.created_by_email) ? `  ·  ${d?.created_by_email ?? run.created_by_email}` : ''}`
                    : (d?.created_by_user_id ?? run.created_by_user_id)
                } />
                <MetaRow label={rt.labelThread} value={run.thread_id} mono />
                <MetaRow label={rt.labelOrg} value={run.org_id} mono />
                <MetaRow label={rt.labelPersona} value={d?.persona_id ?? run.persona_id} />
              </div>
              <div className="pt-2">
                <MetaRow label={rt.labelAgentConfig} value={d?.agent_config_name} />
                <MetaRow label={rt.labelCredential} value={d?.credential_name} />
                <MetaRow label={rt.labelModel} value={d?.model ?? run.model} />
                <MetaRow
                  label={rt.labelTokens}
                  value={
                    (d?.total_input_tokens ?? run.total_input_tokens) != null
                      ? `${d?.total_input_tokens ?? run.total_input_tokens} in / ${d?.total_output_tokens ?? run.total_output_tokens ?? 0} out`
                      : undefined
                  }
                />
                <MetaRow label={rt.labelCost} value={formatCost(d?.total_cost_usd ?? run.total_cost_usd)} />
                {run.credits_used != null && (
                  <MetaRow label={rt.labelCreditsUsed} value={String(run.credits_used)} />
                )}
                {run.cache_hit_rate != null && (
                  <MetaRow
                    label={rt.labelCacheHit}
                    value={`${(run.cache_hit_rate * 100).toFixed(0)}%`}
                  />
                )}
              </div>
              <div className="pt-2">
                <MetaRow label={rt.labelCreated} value={new Date(run.created_at).toLocaleString()} />
                {(d?.completed_at ?? run.completed_at) && (
                  <MetaRow label={rt.labelCompleted} value={new Date((d?.completed_at ?? run.completed_at)!).toLocaleString()} />
                )}
                {(d?.failed_at ?? run.failed_at) && (
                  <MetaRow label={rt.labelFailedAt} value={new Date((d?.failed_at ?? run.failed_at)!).toLocaleString()} />
                )}
              </div>
            </div>
          </Section>

          {/* USAGE BREAKDOWN */}
          {hasUsageBreakdown && (
            <Section
              title={rt.sectionUsage}
              badge={`${usageChildren.length + 1} runs`}
              defaultOpen
            >
              <UsageBreakdownTable
                self={selfUsageItem}
                children={usageChildren}
                aggregate={usageAggregate}
                onOpenRun={onOpenRun}
              />
            </Section>
          )}

          {/* CONVERSATION — 默认折叠，展开时懒加载 */}
          <Section
            title={rt.sectionConversation}
            badge={conversationBadge}
            defaultOpen={false}
            onOpen={loadEvents}
          >
            {eventsLoading && (
              <p className="py-2 text-xs text-[var(--c-text-muted)]">{rt.loading}</p>
            )}
            {d?.user_prompt && (
              <UserPromptBlock prompt={d.user_prompt} label={rt.userPrompt} />
            )}
            {!eventsLoading && events !== null && turns.length === 0 && (
              <p className="py-2 text-xs text-[var(--c-text-muted)]">{rt.noConversation}</p>
            )}
            {turns.length > 0 && (
              <div className="space-y-3">
                {turns.map((turn, i) => (
                  <TurnView key={turn.llmCallId || i} turn={turn} index={i} />
                ))}
              </div>
            )}
          </Section>

          {/* RAW EVENTS — 调试用，始终折叠 */}
          <Section
            title={rt.sectionRawEvents}
            badge={rawEventsBadge}
            defaultOpen={false}
            onOpen={loadEvents}
          >
            {eventsLoading && (
              <p className="py-2 text-xs text-[var(--c-text-muted)]">{rt.loading}</p>
            )}
            {!eventsLoading && events !== null && events.length === 0 && (
              <p className="py-2 text-xs text-[var(--c-text-muted)]">{rt.noEvents}</p>
            )}
            {events && events.length > 0 && (
              <div className="space-y-1">
                {events.map((ev) => (
                  <RawEventRow key={ev.seq} event={ev} />
                ))}
              </div>
            )}
          </Section>
        </div>
      </div>
    </>,
    document.body,
  )
}

function toGlobalRun(item: AdminRunUsageItem): GlobalRun {
  return {
    run_id: item.run_id,
    org_id: item.org_id,
    thread_id: item.thread_id,
    status: item.status,
    model: item.model,
    persona_id: item.persona_id,
    parent_run_id: item.parent_run_id,
    total_input_tokens: item.total_input_tokens,
    total_output_tokens: item.total_output_tokens,
    total_cost_usd: item.total_cost_usd,
    duration_ms: item.duration_ms,
    cache_hit_rate: item.cache_hit_rate,
    credits_used: item.credits_used,
    created_at: item.created_at,
    completed_at: item.completed_at,
    failed_at: item.failed_at,
    created_by_user_id: undefined,
    created_by_user_name: undefined,
    created_by_email: undefined,
  }
}

type UsageBreakdownTableProps = {
  self: AdminRunUsageItem
  children: AdminRunUsageItem[]
  aggregate?: AdminRunUsageAggregate
  onOpenRun?: (run: GlobalRun) => void
}

function stageLabel(rt: ReturnType<typeof useLocale>['t']['pages']['runs'], item: AdminRunUsageItem, isSelf: boolean): string {
  if (isSelf) return rt.usageStageMain
  if (item.persona_id === 'search-output') return rt.usageStageFinal
  return rt.usageStageChild
}

function cacheLabel(item: AdminRunUsageItem): string {
  if (item.cache_hit_rate == null) return '—'
  return `${(item.cache_hit_rate * 100).toFixed(0)}%`
}

function cacheTitle(item: AdminRunUsageItem): string | undefined {
  const parts: string[] = []
  if (item.cache_read_tokens != null) parts.push(`read ${item.cache_read_tokens}`)
  if (item.cache_creation_tokens != null) parts.push(`write ${item.cache_creation_tokens}`)
  if (item.cached_tokens != null) parts.push(`cached ${item.cached_tokens}`)
  return parts.length > 0 ? parts.join(' · ') : undefined
}

function UsageBreakdownTable({ self, children, aggregate, onOpenRun }: UsageBreakdownTableProps) {
  const { t } = useLocale()
  const rt = t.pages.runs

  const rows: Array<{ item: AdminRunUsageItem; isSelf: boolean }> = [
    { item: self, isSelf: true },
    ...children.map((c) => ({ item: c, isSelf: false })),
  ]

  return (
    <div className="space-y-2">
      <div className="overflow-x-auto rounded-lg border border-[var(--c-border)]">
        <table className="min-w-[860px] w-full text-xs">
          <thead className="bg-[var(--c-bg-sub)] text-[var(--c-text-muted)]">
            <tr className="text-left">
              <th className="w-24 whitespace-nowrap px-3 py-2 font-medium">{rt.usageColStage}</th>
              <th className="min-w-[280px] whitespace-nowrap px-3 py-2 font-medium">{rt.usageColModel}</th>
              <th className="w-40 whitespace-nowrap px-3 py-2 font-medium">{rt.usageColTokens}</th>
              <th className="w-28 whitespace-nowrap px-3 py-2 font-medium">{rt.usageColCost}</th>
              <th className="w-24 whitespace-nowrap px-3 py-2 font-medium">{rt.usageColCache}</th>
              <th className="min-w-[260px] whitespace-nowrap px-3 py-2 font-medium">{rt.usageColRun}</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[var(--c-border-console)]">
            {rows.map(({ item, isSelf }) => {
              const inp = item.total_input_tokens ?? 0
              const out = item.total_output_tokens ?? 0
              const modelText = item.model ?? '—'
              const providerText = item.provider_kind ? ` · ${item.provider_kind}` : ''

              return (
                <tr key={item.run_id} className="bg-[var(--c-bg-deep2)]">
                  <td className="whitespace-nowrap px-3 py-2 text-[var(--c-text-secondary)]">
                    {stageLabel(rt, item, isSelf)}
                  </td>
                  <td className="px-3 py-2 text-[var(--c-text-secondary)]" title={modelText}>
                    <div className="truncate">
                      {modelText}
                      <span className="text-[var(--c-text-muted)]">{providerText}</span>
                    </div>
                    {(item.credential_name || item.agent_config_name) && (
                      <div
                        className="truncate text-[11px] text-[var(--c-text-muted)]"
                        title={item.credential_name ?? item.agent_config_name}
                      >
                        {item.credential_name ?? item.agent_config_name}
                      </div>
                    )}
                  </td>
                  <td className="whitespace-nowrap px-3 py-2 tabular-nums text-[var(--c-text-secondary)]">
                    {inp} / {out}
                  </td>
                  <td className="whitespace-nowrap px-3 py-2 tabular-nums text-[var(--c-text-secondary)]">
                    {formatCost(item.total_cost_usd)}
                  </td>
                  <td
                    className={[
                      'whitespace-nowrap px-3 py-2 tabular-nums',
                      item.cache_hit_rate != null ? 'text-[var(--c-status-success-text)]' : 'text-[var(--c-text-muted)]',
                    ].join(' ')}
                    title={cacheTitle(item)}
                  >
                    {cacheLabel(item)}
                  </td>
                  <td className="whitespace-nowrap px-3 py-2">
                    {onOpenRun ? (
                      <button
                        onClick={() => onOpenRun(toGlobalRun(item))}
                        className="font-mono text-[11px] text-[var(--c-text-muted)] hover:text-[var(--c-text-secondary)]"
                        title={item.run_id}
                      >
                        {item.run_id}
                      </button>
                    ) : (
                      <span className="font-mono text-[11px] text-[var(--c-text-muted)]" title={item.run_id}>
                        {item.run_id}
                      </span>
                    )}
                  </td>
                </tr>
              )
            })}

            {aggregate && (
              <tr className="bg-[var(--c-bg-sub)]">
                <td className="px-3 py-2 font-medium text-[var(--c-text-secondary)]">{rt.usageTotal}</td>
                <td className="px-3 py-2 text-[var(--c-text-muted)]" />
                <td className="px-3 py-2 tabular-nums font-medium text-[var(--c-text-secondary)]">
                  {(aggregate.total_input_tokens ?? 0)} / {(aggregate.total_output_tokens ?? 0)}
                </td>
                <td className="px-3 py-2 tabular-nums font-medium text-[var(--c-text-secondary)]">
                  {formatCost(aggregate.total_cost_usd)}
                </td>
                <td className="px-3 py-2 text-[var(--c-text-muted)]" />
                <td className="px-3 py-2 text-[var(--c-text-muted)]" />
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

type RawEventRowProps = { event: RunEventRaw }

function UserPromptBlock({ prompt, label }: { prompt: string; label: string }) {
  const [open, setOpen] = useState(false)
  const preview = prompt.slice(0, 100) + (prompt.length > 100 ? '…' : '')

  return (
    <div className="mb-3 rounded border border-[var(--c-border)] overflow-hidden">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-start gap-2 px-3 py-2 text-left transition-colors hover:bg-[var(--c-bg-sub)]"
      >
        <span className="shrink-0 rounded bg-[var(--c-bg-sub)] px-1.5 py-0.5 text-xs font-medium text-[var(--c-text-secondary)]">
          {label}
        </span>
        {!open && (
          <span className="truncate text-xs text-[var(--c-text-muted)]">{preview}</span>
        )}
      </button>
      {open && (
        <div className="border-t border-[var(--c-border)] bg-[var(--c-bg-deep2)] px-3 py-2">
          <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-[var(--c-text-secondary)]">
            {prompt}
          </pre>
        </div>
      )}
    </div>
  )
}

function RawEventRow({ event }: RawEventRowProps) {
  const [open, setOpen] = useState(false)
  const hasData = event.data && Object.keys(event.data).length > 0

  return (
    <div className="rounded border border-[var(--c-border)] overflow-hidden">
      <button
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-2 px-3 py-1.5 text-left transition-colors hover:bg-[var(--c-bg-sub)]"
        disabled={!hasData}
      >
        <span className="w-6 shrink-0 text-right font-mono text-xs text-[var(--c-text-muted)]">
          {event.seq}
        </span>
        <span className="text-xs font-medium text-[var(--c-text-secondary)]">{event.type}</span>
        {event.tool_name && (
          <span className="text-xs text-[var(--c-text-muted)]">{event.tool_name}</span>
        )}
        {event.error_class && (
          <span className="ml-auto text-xs text-red-500">{event.error_class}</span>
        )}
        <span className="ml-auto text-xs text-[var(--c-text-muted)]">
          {new Date(event.ts).toLocaleTimeString()}
        </span>
      </button>
      {open && hasData && (
        <div className="border-t border-[var(--c-border)] bg-[var(--c-bg-deep2)] px-3 py-2">
          <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-[var(--c-text-secondary)]">
            {JSON.stringify(event.data, null, 2)}
          </pre>
        </div>
      )}
    </div>
  )
}
