import { useState, useEffect, useCallback, Fragment } from 'react'
import { useOutletContext } from 'react-router-dom'
import { Loader2, RefreshCw, ChevronDown, ChevronRight } from 'lucide-react'
import type { ConsoleOutletContext } from '../../layouts/ConsoleLayout'
import { PageHeader } from '../../components/PageHeader'
import { Badge } from '../../components/Badge'
import { useToast } from '@arkloop/shared'
import { isApiError } from '../../api'
import { useLocale } from '../../contexts/LocaleContext'
import { getPlatformSetting, setPlatformSetting } from '../../api/platform-settings'
import { listAuditLogs, type AuditLog } from '../../api/audit'

const KEY_REGEX_ENABLED = 'security.injection_scan.regex_enabled'
const KEY_TRUST_SOURCE_ENABLED = 'security.injection_scan.trust_source_enabled'
const KEY_SEMANTIC_ENABLED = 'security.injection_scan.semantic_enabled'
const AUDIT_ACTION = 'security.injection_detected'
const AUDIT_PAGE_SIZE = 30

type Layer = {
  id: string
  nameKey: 'layerRegex' | 'layerSemantic' | 'layerTrustSource'
  descKey: 'layerRegexDesc' | 'layerSemanticDesc' | 'layerTrustSourceDesc'
  settingsKey: string | null
}

const LAYERS: Layer[] = [
  { id: 'regex', nameKey: 'layerRegex', descKey: 'layerRegexDesc', settingsKey: KEY_REGEX_ENABLED },
  { id: 'trust-source', nameKey: 'layerTrustSource', descKey: 'layerTrustSourceDesc', settingsKey: KEY_TRUST_SOURCE_ENABLED },
  { id: 'semantic', nameKey: 'layerSemantic', descKey: 'layerSemanticDesc', settingsKey: KEY_SEMANTIC_ENABLED },
]

type Tab = 'layers' | 'audit'

function truncateId(id: string): string {
  return id.length > 8 ? id.slice(0, 8) : id
}

function AuditTab({ accessToken }: { accessToken: string }) {
  const { addToast } = useToast()
  const { t } = useLocale()
  const tp = t.pages.promptInjection

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
      addToast(tp.toastLoadFailed, 'error')
    } finally {
      setLoading(false)
    }
  }, [accessToken, addToast, tp.toastLoadFailed])

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
        <p className="text-sm text-[var(--c-text-muted)]">{tp.auditEmpty}</p>
      </div>
    )
  }

  return (
    <div className="flex flex-col">
      <div className="mb-3 flex items-center justify-between">
        <span className="text-xs text-[var(--c-text-muted)]">{total} events</span>
        <button
          onClick={() => fetchLogs(offset)}
          className="flex items-center gap-1 rounded-lg border border-[var(--c-border)] px-2.5 py-1.5 text-xs text-[var(--c-text-secondary)] hover:bg-[var(--c-bg-sub)]"
        >
          <RefreshCw size={13} />
        </button>
      </div>
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-[var(--c-border-console)]">
            <th className="w-6 px-3 py-2.5" />
            <th className="whitespace-nowrap px-4 py-2.5 text-xs font-medium text-[var(--c-text-muted)]">{tp.auditColTime}</th>
            <th className="whitespace-nowrap px-4 py-2.5 text-xs font-medium text-[var(--c-text-muted)]">{tp.auditColRunId}</th>
            <th className="whitespace-nowrap px-4 py-2.5 text-xs font-medium text-[var(--c-text-muted)]">{tp.auditColCount}</th>
            <th className="whitespace-nowrap px-4 py-2.5 text-xs font-medium text-[var(--c-text-muted)]">{tp.auditColPatterns}</th>
          </tr>
        </thead>
        <tbody>
          {logs.map(log => {
            const expanded = expandedIds.has(log.id)
            const meta = log.metadata as Record<string, unknown>
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
                  <td className="w-6 px-3 py-2.5 text-[var(--c-text-muted)]">
                    {hasDetail && (expanded ? <ChevronDown size={13} /> : <ChevronRight size={13} />)}
                  </td>
                  <td className="whitespace-nowrap px-4 py-2.5 text-xs tabular-nums text-[var(--c-text-secondary)]">
                    {new Date(log.created_at).toLocaleString()}
                  </td>
                  <td className="whitespace-nowrap px-4 py-2.5 text-[var(--c-text-secondary)]">
                    <span className="font-mono text-xs" title={log.target_id ?? ''}>
                      {log.target_id ? truncateId(log.target_id) : '--'}
                    </span>
                  </td>
                  <td className="whitespace-nowrap px-4 py-2.5 text-xs text-[var(--c-text-secondary)]">
                    {count}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-[var(--c-text-secondary)]">
                    {patterns.slice(0, 3).map(p => p.pattern_id ?? p.category).join(', ')}
                    {patterns.length > 3 && ` +${patterns.length - 3}`}
                  </td>
                </tr>
                {expanded && (
                  <tr className="bg-[var(--c-bg-deep2)]">
                    <td colSpan={5} className="px-6 py-3">
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
        <div className="flex items-center justify-between border-t border-[var(--c-border-console)] px-4 py-2">
          <span className="text-xs text-[var(--c-text-muted)]">
            {offset + 1}–{Math.min(offset + AUDIT_PAGE_SIZE, total)} / {total}
          </span>
          <div className="flex gap-2">
            <button
              onClick={() => setOffset(p => Math.max(0, p - AUDIT_PAGE_SIZE))}
              disabled={currentPage <= 1}
              className="rounded border border-[var(--c-border)] px-2.5 py-1 text-xs text-[var(--c-text-secondary)] disabled:opacity-40 hover:bg-[var(--c-bg-sub)]"
            >
              Prev
            </button>
            <span className="flex items-center text-xs text-[var(--c-text-muted)]">{currentPage} / {totalPages}</span>
            <button
              onClick={() => setOffset(p => p + AUDIT_PAGE_SIZE)}
              disabled={currentPage >= totalPages}
              className="rounded border border-[var(--c-border)] px-2.5 py-1 text-xs text-[var(--c-text-secondary)] disabled:opacity-40 hover:bg-[var(--c-bg-sub)]"
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

export function PromptInjectionPage() {
  const { accessToken } = useOutletContext<ConsoleOutletContext>()
  const { addToast } = useToast()
  const { t } = useLocale()
  const tp = t.pages.promptInjection

  const [activeTab, setActiveTab] = useState<Tab>('layers')
  const [loading, setLoading] = useState(true)
  const [toggling, setToggling] = useState('')
  const [settings, setSettings] = useState<Record<string, boolean>>({})

  const loadSettings = useCallback(async () => {
    setLoading(true)
    try {
      const [regexResult, trustResult, semanticResult] = await Promise.all([
        getPlatformSetting(KEY_REGEX_ENABLED, accessToken).catch(() => ({ value: 'true' })),
        getPlatformSetting(KEY_TRUST_SOURCE_ENABLED, accessToken).catch(() => ({ value: 'true' })),
        getPlatformSetting(KEY_SEMANTIC_ENABLED, accessToken).catch(() => ({ value: 'true' })),
      ])
      setSettings({
        [KEY_REGEX_ENABLED]: regexResult.value === 'true',
        [KEY_TRUST_SOURCE_ENABLED]: trustResult.value === 'true',
        [KEY_SEMANTIC_ENABLED]: semanticResult.value === 'true',
      })
    } catch (err) {
      addToast(isApiError(err) ? err.message : tp.toastLoadFailed, 'error')
    } finally {
      setLoading(false)
    }
  }, [accessToken, addToast, tp.toastLoadFailed])

  useEffect(() => { loadSettings() }, [loadSettings])

  const handleToggle = useCallback(async (key: string, current: boolean) => {
    if (toggling) return
    setToggling(key)
    try {
      await setPlatformSetting(key, String(!current), accessToken)
      setSettings(prev => ({ ...prev, [key]: !current }))
      addToast(tp.toastUpdated, 'success')
    } catch (err) {
      addToast(isApiError(err) ? err.message : tp.toastFailed, 'error')
    } finally {
      setToggling('')
    }
  }, [toggling, accessToken, addToast, tp.toastUpdated, tp.toastFailed])

  const tabs: { key: Tab; label: string }[] = [
    { key: 'layers', label: tp.tabLayers },
    { key: 'audit', label: tp.tabAudit },
  ]

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader title={tp.title} />
      <div className="flex-1 overflow-y-auto p-6">
        <p className="mb-4 text-sm text-[var(--c-text-secondary)]">{tp.description}</p>

        <div className="mb-6 flex gap-1 border-b border-[var(--c-border-console)]">
          {tabs.map(tab => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={`px-4 py-2 text-sm transition-colors ${
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
              {LAYERS.map(layer => {
                const enabled = layer.settingsKey ? settings[layer.settingsKey] ?? true : false
                const comingSoon = !layer.settingsKey
                const isToggling = toggling === layer.settingsKey

                return (
                  <div
                    key={layer.id}
                    className="flex items-center justify-between rounded-lg border border-[var(--c-border-console)] bg-[var(--c-bg-card)] px-5 py-4"
                  >
                    <div className="flex flex-col gap-1">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium text-[var(--c-text-primary)]">
                          {tp[layer.nameKey]}
                        </span>
                        {comingSoon ? (
                          <Badge variant="neutral">{tp.statusComingSoon}</Badge>
                        ) : (
                          <Badge variant={enabled ? 'success' : 'warning'}>
                            {enabled ? tp.statusEnabled : tp.statusDisabled}
                          </Badge>
                        )}
                      </div>
                      <span className="text-xs text-[var(--c-text-muted)]">
                        {tp[layer.descKey]}
                      </span>
                    </div>

                    {!comingSoon && (
                      <button
                        onClick={() => handleToggle(layer.settingsKey!, enabled)}
                        disabled={isToggling || loading}
                        className={`relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full transition-colors duration-200 ${
                          enabled
                            ? 'bg-[var(--c-status-success)]'
                            : 'bg-[var(--c-border-console)]'
                        } ${(isToggling || loading) ? 'opacity-50 cursor-not-allowed' : ''}`}
                      >
                        {isToggling ? (
                          <Loader2 size={12} className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 animate-spin text-white" />
                        ) : (
                          <span
                            className={`inline-block h-4 w-4 rounded-full bg-white shadow transition-transform duration-200 ${
                              enabled ? 'translate-x-[22px]' : 'translate-x-[3px]'
                            }`}
                          />
                        )}
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
