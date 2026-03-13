import { useState, useEffect, useCallback, Fragment } from 'react'
import { useOutletContext } from 'react-router-dom'
import { ShieldAlert, Loader2, RefreshCw, ChevronDown, ChevronRight } from 'lucide-react'
import type { LiteOutletContext } from '../layouts/LiteLayout'
import { PageHeader } from '../components/PageHeader'
import { useToast } from '@arkloop/shared'
import { useLocale } from '../contexts/LocaleContext'
import {
  listPlatformSettings,
  updatePlatformSetting,
  isApiError,
} from '../api'
import { listAuditLogs, type AuditLog } from '../api/audit'

const KEY_REGEX_ENABLED = 'security.injection_scan.regex_enabled'
const KEY_TRUST_SOURCE_ENABLED = 'security.injection_scan.trust_source_enabled'
const KEY_SEMANTIC_ENABLED = 'security.injection_scan.semantic_enabled'
const AUDIT_ACTION = 'security.injection_detected'
const AUDIT_PAGE_SIZE = 30

type Layer = {
  id: string
  nameKey: 'layerRegex' | 'layerSemantic' | 'layerTrustSource'
  descKey: 'layerRegexDesc' | 'layerSemanticDesc' | 'layerTrustSourceDesc'
  settingKey: string | null
}

const LAYERS: Layer[] = [
  { id: 'regex', nameKey: 'layerRegex', descKey: 'layerRegexDesc', settingKey: KEY_REGEX_ENABLED },
  { id: 'trust-source', nameKey: 'layerTrustSource', descKey: 'layerTrustSourceDesc', settingKey: KEY_TRUST_SOURCE_ENABLED },
  { id: 'semantic', nameKey: 'layerSemantic', descKey: 'layerSemanticDesc', settingKey: KEY_SEMANTIC_ENABLED },
]

type Tab = 'layers' | 'audit'

function truncateId(id: string): string {
  return id.length > 8 ? id.slice(0, 8) : id
}

function AuditTab({ accessToken }: { accessToken: string }) {
  const { addToast } = useToast()
  const { t } = useLocale()
  const ts = t.security

  const [logs, setLogs] = useState<AuditLog[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [offset, setOffset] = useState(0)
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set())

  const fetchLogs = useCallback(async (currentOffset: number) => {
    setLoading(true)
    try {
      const resp = await listAuditLogs(
        { action: AUDIT_ACTION, limit: AUDIT_PAGE_SIZE, offset: currentOffset },
        accessToken,
      )
      setLogs(resp.data)
      setTotal(resp.total)
    } catch {
      addToast(ts.toastLoadFailed, 'error')
    } finally {
      setLoading(false)
    }
  }, [accessToken, addToast, ts.toastLoadFailed])

  useEffect(() => { fetchLogs(offset) }, [fetchLogs, offset])

  const toggleExpand = useCallback((id: string) => {
    setExpandedIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const totalPages = Math.ceil(total / AUDIT_PAGE_SIZE)
  const currentPage = Math.floor(offset / AUDIT_PAGE_SIZE) + 1

  if (loading) {
    return (
      <div className="flex items-center justify-center py-16">
        <Loader2 size={20} className="animate-spin text-[var(--c-text-muted)]" />
      </div>
    )
  }

  if (logs.length === 0) {
    return (
      <div className="flex items-center justify-center py-16">
        <p className="text-sm text-[var(--c-text-muted)]">{ts.auditEmpty}</p>
      </div>
    )
  }

  return (
    <div className="flex flex-col">
      <div className="mb-3 flex items-center justify-between">
        <span className="text-xs text-[var(--c-text-muted)]">{total} events</span>
        <button
          onClick={() => fetchLogs(offset)}
          className="flex items-center gap-1 rounded-lg border border-[var(--c-border-console)] px-2 py-1 text-xs text-[var(--c-text-muted)] hover:bg-[var(--c-bg-sub)]"
        >
          <RefreshCw size={12} />
        </button>
      </div>
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-[var(--c-border-console)]">
            <th className="w-6 px-2 py-2" />
            <th className="whitespace-nowrap px-3 py-2 text-xs font-medium text-[var(--c-text-muted)]">{ts.auditColTime}</th>
            <th className="whitespace-nowrap px-3 py-2 text-xs font-medium text-[var(--c-text-muted)]">{ts.auditColRunId}</th>
            <th className="whitespace-nowrap px-3 py-2 text-xs font-medium text-[var(--c-text-muted)]">{ts.auditColCount}</th>
            <th className="whitespace-nowrap px-3 py-2 text-xs font-medium text-[var(--c-text-muted)]">{ts.auditColPatterns}</th>
          </tr>
        </thead>
        <tbody>
          {logs.map(log => {
            const expanded = expandedIds.has(log.id)
            const meta = log.metadata
            const count = (meta?.detection_count as number) ?? 0
            const patterns = (meta?.patterns as Array<Record<string, string>>) ?? []
            const hasDetail = patterns.length > 0

            return (
              <Fragment key={log.id}>
                <tr
                  onClick={() => hasDetail && toggleExpand(log.id)}
                  className={[
                    'border-b border-[var(--c-border-console)] transition-colors hover:bg-[var(--c-bg-sub)]',
                    hasDetail ? 'cursor-pointer' : '',
                  ].join(' ')}
                >
                  <td className="w-6 px-2 py-2 text-[var(--c-text-muted)]">
                    {hasDetail && (expanded ? <ChevronDown size={12} /> : <ChevronRight size={12} />)}
                  </td>
                  <td className="whitespace-nowrap px-3 py-2 text-xs tabular-nums text-[var(--c-text-secondary)]">
                    {new Date(log.created_at).toLocaleString()}
                  </td>
                  <td className="whitespace-nowrap px-3 py-2 text-[var(--c-text-secondary)]">
                    <span className="font-mono text-xs" title={log.target_id ?? ''}>
                      {log.target_id ? truncateId(log.target_id) : '--'}
                    </span>
                  </td>
                  <td className="whitespace-nowrap px-3 py-2 text-xs text-[var(--c-text-secondary)]">{count}</td>
                  <td className="px-3 py-2 text-xs text-[var(--c-text-secondary)]">
                    {patterns.slice(0, 3).map(p => p.pattern_id ?? p.category).join(', ')}
                    {patterns.length > 3 && ` +${patterns.length - 3}`}
                  </td>
                </tr>
                {expanded && (
                  <tr className="bg-[var(--c-bg-deep2)]">
                    <td colSpan={5} className="px-4 py-3">
                      <pre className="overflow-auto rounded-md bg-[var(--c-bg-deep3,var(--c-bg-tag))] p-3 text-xs leading-relaxed text-[var(--c-text-secondary)]">
                        {JSON.stringify(meta, null, 2)}
                      </pre>
                    </td>
                  </tr>
                )}
              </Fragment>
            )
          })}
        </tbody>
      </table>
      {totalPages > 1 && (
        <div className="flex items-center justify-between border-t border-[var(--c-border-console)] px-3 py-2">
          <span className="text-xs text-[var(--c-text-muted)]">
            {offset + 1}–{Math.min(offset + AUDIT_PAGE_SIZE, total)} / {total}
          </span>
          <div className="flex gap-2">
            <button
              onClick={() => setOffset(p => Math.max(0, p - AUDIT_PAGE_SIZE))}
              disabled={currentPage <= 1}
              className="rounded border border-[var(--c-border-console)] px-2 py-0.5 text-xs text-[var(--c-text-secondary)] disabled:opacity-40"
            >
              Prev
            </button>
            <span className="flex items-center text-xs text-[var(--c-text-muted)]">{currentPage} / {totalPages}</span>
            <button
              onClick={() => setOffset(p => p + AUDIT_PAGE_SIZE)}
              disabled={currentPage >= totalPages}
              className="rounded border border-[var(--c-border-console)] px-2 py-0.5 text-xs text-[var(--c-text-secondary)] disabled:opacity-40"
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

export function SecurityPage() {
  const { accessToken } = useOutletContext<LiteOutletContext>()
  const { addToast } = useToast()
  const { t } = useLocale()
  const ts = t.security

  const [activeTab, setActiveTab] = useState<Tab>('layers')
  const [values, setValues] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(true)
  const [toggling, setToggling] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const list = await listPlatformSettings(accessToken)
      const map: Record<string, string> = {}
      for (const s of list) map[s.key] = s.value
      setValues(map)
    } catch (err) {
      if (isApiError(err)) addToast(ts.toastLoadFailed, 'error')
    } finally {
      setLoading(false)
    }
  }, [accessToken, addToast, ts.toastLoadFailed])

  useEffect(() => { void load() }, [load])

  const toggle = useCallback(async (key: string, current: boolean) => {
    setToggling(key)
    try {
      await updatePlatformSetting(key, String(!current), accessToken)
      setValues((prev) => ({ ...prev, [key]: String(!current) }))
      addToast(ts.toastUpdated, 'success')
    } catch (err) {
      if (isApiError(err)) addToast(ts.toastFailed, 'error')
    } finally {
      setToggling(null)
    }
  }, [accessToken, addToast, ts.toastUpdated, ts.toastFailed])

  const isEnabled = (key: string) => values[key] === 'true'

  const tabs: { key: Tab; label: string }[] = [
    { key: 'layers', label: ts.tabLayers },
    { key: 'audit', label: ts.tabAudit },
  ]

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader
        title={
          <span className="flex items-center gap-2">
            <ShieldAlert size={15} />
            {ts.title}
          </span>
        }
      />

      <div className="flex-1 overflow-y-auto p-6">
        <p className="mb-4 text-xs text-[var(--c-text-muted)]">{ts.description}</p>

        <div className="mb-5 flex gap-1 border-b border-[var(--c-border-console)]">
          {tabs.map(tab => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={`px-3 py-1.5 text-xs transition-colors ${
                activeTab === tab.key
                  ? 'border-b-2 border-[var(--c-text-primary)] font-medium text-[var(--c-text-primary)]'
                  : 'text-[var(--c-text-muted)] hover:text-[var(--c-text-secondary)]'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        {activeTab === 'layers' && (
          loading ? (
            <div className="flex items-center justify-center py-16">
              <Loader2 size={20} className="animate-spin text-[var(--c-text-muted)]" />
            </div>
          ) : (
            <div className="flex flex-col gap-3">
              {LAYERS.map((layer) => {
                const configurable = layer.settingKey !== null
                const enabled = configurable && isEnabled(layer.settingKey!)
                const busy = toggling === layer.settingKey

                return (
                  <div
                    key={layer.id}
                    className="flex items-center justify-between rounded-lg border border-[var(--c-border-console)] px-4 py-3"
                  >
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium text-[var(--c-text-primary)]">
                          {ts[layer.nameKey]}
                        </span>
                        {!configurable && (
                          <span className="rounded bg-[var(--c-bg-tag)] px-1.5 py-0.5 text-[10px] font-medium text-[var(--c-text-muted)]">
                            {ts.statusComingSoon}
                          </span>
                        )}
                        {configurable && (
                          <span
                            className={[
                              'rounded px-1.5 py-0.5 text-[10px] font-medium',
                              enabled
                                ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
                                : 'bg-[var(--c-bg-tag)] text-[var(--c-text-muted)]',
                            ].join(' ')}
                          >
                            {enabled ? ts.statusEnabled : ts.statusDisabled}
                          </span>
                        )}
                      </div>
                      <p className="mt-1 text-xs text-[var(--c-text-muted)]">
                        {ts[layer.descKey]}
                      </p>
                    </div>

                    {configurable && (
                      <button
                        disabled={busy}
                        onClick={() => void toggle(layer.settingKey!, enabled)}
                        className={[
                          'relative ml-4 h-5 w-9 shrink-0 rounded-full transition-colors',
                          enabled ? 'bg-emerald-500' : 'bg-[var(--c-bg-tag)]',
                          busy ? 'opacity-50' : 'cursor-pointer',
                        ].join(' ')}
                      >
                        <span
                          className={[
                            'absolute top-0.5 h-4 w-4 rounded-full bg-white shadow transition-transform',
                            enabled ? 'translate-x-[18px]' : 'translate-x-0.5',
                          ].join(' ')}
                        />
                      </button>
                    )}
                  </div>
                )
              })}
            </div>
          )
        )}

        {activeTab === 'audit' && <AuditTab accessToken={accessToken} />}
      </div>
    </div>
  )
}
