import { useState, useCallback, useEffect } from 'react'
import { useOutletContext } from 'react-router-dom'
import { BarChart3 } from 'lucide-react'
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
  PieChart, Pie, Cell, Legend,
} from 'recharts'
import type { ConsoleOutletContext } from '../../layouts/ConsoleLayout'
import { PageHeader } from '../../components/PageHeader'
import { useToast } from '../../components/useToast'
import { useLocale } from '../../contexts/LocaleContext'
import {
  getGlobalUsageSummary,
  getGlobalDailyUsage,
  getGlobalUsageByModel,
  type UsageSummary,
  type DailyUsage,
  type ModelUsage,
} from '../../api/usage'
import { isApiError } from '../../api'

const MONTHS = [
  'January', 'February', 'March', 'April',
  'May', 'June', 'July', 'August',
  'September', 'October', 'November', 'December',
]

const PIE_COLORS = [
  '#60a5fa', '#34d399', '#fbbf24', '#f87171',
  '#a78bfa', '#f472b6', '#38bdf8', '#fb923c',
]

function buildYearOptions(): number[] {
  const current = new Date().getUTCFullYear()
  const years: number[] = []
  for (let y = 2024; y <= current + 1; y++) years.push(y)
  return years
}

function formatNumber(n: number): string {
  return n.toLocaleString()
}

function formatCost(n: number): string {
  return n.toFixed(4)
}

function dateRangeForMonth(year: number, month: number): { start: string; end: string } {
  const s = new Date(Date.UTC(year, month - 1, 1))
  const e = new Date(Date.UTC(year, month, 0))
  const pad = (v: number) => String(v).padStart(2, '0')
  return {
    start: `${s.getUTCFullYear()}-${pad(s.getUTCMonth() + 1)}-${pad(s.getUTCDate())}`,
    end: `${e.getUTCFullYear()}-${pad(e.getUTCMonth() + 1)}-${pad(e.getUTCDate())}`,
  }
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-1.5 rounded-xl border border-[var(--c-border)] bg-[var(--c-bg-sub)] px-5 py-4">
      <span className="text-xs text-[var(--c-text-muted)]">{label}</span>
      <span className="text-2xl font-semibold tabular-nums text-[var(--c-text-primary)]">{value}</span>
    </div>
  )
}

type QueryResult = {
  summary: UsageSummary
  daily: DailyUsage[]
  byModel: ModelUsage[]
}

export function UsagePage() {
  const { accessToken } = useOutletContext<ConsoleOutletContext>()
  const { addToast } = useToast()
  const { t } = useLocale()
  const tc = t.pages.usage

  const now = new Date()
  const [year, setYear] = useState(now.getUTCFullYear())
  const [month, setMonth] = useState(now.getUTCMonth() + 1)
  const [loading, setLoading] = useState(false)
  const [result, setResult] = useState<QueryResult | null>(null)

  const years = buildYearOptions()

  const loadData = useCallback(async (y: number, m: number) => {
    setLoading(true)
    try {
      const range = dateRangeForMonth(y, m)
      const [summary, daily, byModel] = await Promise.all([
        getGlobalUsageSummary(y, m, accessToken),
        getGlobalDailyUsage(range.start, range.end, accessToken),
        getGlobalUsageByModel(y, m, accessToken),
      ])
      setResult({ summary, daily, byModel })
    } catch (err) {
      addToast(isApiError(err) ? err.message : tc.toastLoadFailed, 'error')
    } finally {
      setLoading(false)
    }
  }, [accessToken, addToast, tc.toastLoadFailed])

  useEffect(() => {
    void loadData(year, month)
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const handleQuery = useCallback(() => {
    void loadData(year, month)
  }, [year, month, loadData])

  const selectCls =
    'rounded-lg border border-[var(--c-border)] bg-[var(--c-bg-deep2)] px-2.5 py-1.5 text-xs text-[var(--c-text-secondary)] focus:outline-none'

  const filterBar = (
    <div className="flex items-center gap-2 flex-wrap">
      <select
        value={year}
        onChange={(e) => setYear(Number(e.target.value))}
        className={selectCls}
      >
        {years.map((y) => (
          <option key={y} value={y}>{y}</option>
        ))}
      </select>

      <select
        value={month}
        onChange={(e) => setMonth(Number(e.target.value))}
        className={selectCls}
      >
        {MONTHS.map((name, i) => (
          <option key={i + 1} value={i + 1}>{name}</option>
        ))}
      </select>

      <button
        onClick={handleQuery}
        disabled={loading}
        className="flex items-center gap-1.5 rounded-lg bg-[var(--c-bg-tag)] px-3 py-1.5 text-xs font-medium text-[var(--c-text-secondary)] transition-colors hover:bg-[var(--c-bg-sub)] disabled:opacity-50"
      >
        {loading ? '...' : tc.queryButton}
      </button>
    </div>
  )

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <PageHeader title={tc.title} actions={filterBar} />

      <div className="flex flex-1 flex-col overflow-auto p-4 gap-6">
        {result ? (
          <>
            <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
              <StatCard label={tc.cardInputTokens} value={formatNumber(result.summary.total_input_tokens)} />
              <StatCard label={tc.cardOutputTokens} value={formatNumber(result.summary.total_output_tokens)} />
              <StatCard label={tc.cardCostUSD} value={formatCost(result.summary.total_cost_usd)} />
              <StatCard label={tc.cardRecordCount} value={formatNumber(result.summary.record_count)} />
            </div>

            <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
              <div className="rounded-xl border border-[var(--c-border)] bg-[var(--c-bg-sub)] p-4">
                <h3 className="mb-3 text-sm font-medium text-[var(--c-text-secondary)]">{tc.chartDailyTitle}</h3>
                {result.daily.length > 0 ? (
                  <ResponsiveContainer width="100%" height={260}>
                    <LineChart data={result.daily}>
                      <CartesianGrid strokeDasharray="3 3" stroke="var(--c-border)" />
                      <XAxis
                        dataKey="date"
                        tickFormatter={(v: string) => v.slice(5)}
                        stroke="var(--c-text-muted)"
                        tick={{ fontSize: 11 }}
                      />
                      <YAxis stroke="var(--c-text-muted)" tick={{ fontSize: 11 }} />
                      <Tooltip
                        contentStyle={{
                          backgroundColor: 'var(--c-bg-menu)',
                          border: '1px solid var(--c-border)',
                          borderRadius: 8,
                          fontSize: 12,
                          color: 'var(--c-text-primary)',
                        }}
                      />
                      <Line type="monotone" dataKey="input_tokens" name={tc.cardInputTokens} stroke="#60a5fa" strokeWidth={2} dot={false} />
                      <Line type="monotone" dataKey="output_tokens" name={tc.cardOutputTokens} stroke="#34d399" strokeWidth={2} dot={false} />
                    </LineChart>
                  </ResponsiveContainer>
                ) : (
                  <p className="flex h-[260px] items-center justify-center text-xs text-[var(--c-text-muted)]">{tc.chartNoData}</p>
                )}
              </div>

              <div className="rounded-xl border border-[var(--c-border)] bg-[var(--c-bg-sub)] p-4">
                <h3 className="mb-3 text-sm font-medium text-[var(--c-text-secondary)]">{tc.chartModelTitle}</h3>
                {result.byModel.length > 0 ? (
                  <ResponsiveContainer width="100%" height={260}>
                    <PieChart>
                      <Pie
                        data={result.byModel.map((m) => ({
                          name: m.model,
                          value: m.input_tokens + m.output_tokens,
                        }))}
                        cx="50%"
                        cy="50%"
                        innerRadius={50}
                        outerRadius={90}
                        paddingAngle={2}
                        dataKey="value"
                        label={({ name, percent }) =>
                          `${name ?? ''} ${((percent ?? 0) * 100).toFixed(0)}%`
                        }
                      >
                        {result.byModel.map((_, i) => (
                          <Cell key={i} fill={PIE_COLORS[i % PIE_COLORS.length]} />
                        ))}
                      </Pie>
                      <Tooltip
                        formatter={(value) => formatNumber(value as number)}
                        contentStyle={{
                          backgroundColor: 'var(--c-bg-menu)',
                          border: '1px solid var(--c-border)',
                          borderRadius: 8,
                          fontSize: 12,
                          color: 'var(--c-text-primary)',
                        }}
                      />
                      <Legend wrapperStyle={{ fontSize: 11, color: 'var(--c-text-muted)' }} />
                    </PieChart>
                  </ResponsiveContainer>
                ) : (
                  <p className="flex h-[260px] items-center justify-center text-xs text-[var(--c-text-muted)]">{tc.chartNoData}</p>
                )}
              </div>
            </div>
          </>
        ) : loading ? (
          <div className="flex flex-1 items-center justify-center text-[var(--c-text-muted)]">
            <p className="text-sm">...</p>
          </div>
        ) : (
          <div className="flex flex-1 flex-col items-center justify-center gap-3 text-[var(--c-text-muted)]">
            <BarChart3 size={28} />
            <p className="text-sm">{tc.emptyHint}</p>
          </div>
        )}
      </div>
    </div>
  )
}
