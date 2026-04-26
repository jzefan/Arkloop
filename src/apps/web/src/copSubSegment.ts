import type { CopBlockItem } from './assistantTurnSegments'
import { normalizeToolName, compactCommandLine } from './toolPresentation'

export type CopSegmentCategory = 'explore' | 'exec' | 'edit' | 'agent' | 'fetch' | 'generic'

export type CopSubSegment = {
  id: string
  category: CopSegmentCategory
  status: 'open' | 'closed'
  items: CopBlockItem[]
  seq: number
  title: string
}

const EXPLORE_NAMES = new Set(['read_file', 'grep', 'glob', 'load_tools', 'load_skill', 'lsp'])
const EXEC_NAMES = new Set(['exec_command', 'python_execute', 'continue_process', 'terminate_process'])
const EDIT_NAMES = new Set(['edit', 'edit_file', 'write_file'])
const AGENT_NAMES = new Set([
  'spawn_agent', 'acp_agent', 'spawn_acp',
  'send_input', 'wait_agent', 'resume_agent', 'close_agent', 'interrupt_agent',
  'send_acp', 'wait_acp', 'close_acp', 'interrupt_acp',
])
const FETCH_NAMES = new Set(['web_fetch'])
const MUTATING_LSP = new Set(['rename'])

export function categoryForTool(toolName: string): CopSegmentCategory {
  const n = normalizeToolName(toolName)
  if (EXPLORE_NAMES.has(n)) {
    if (n === 'lsp' && MUTATING_LSP.has(toolName)) return 'edit'
    return 'explore'
  }
  if (EXEC_NAMES.has(n)) return 'exec'
  if (EDIT_NAMES.has(n)) return 'edit'
  if (AGENT_NAMES.has(n)) return 'agent'
  if (FETCH_NAMES.has(n)) return 'fetch'
  return 'generic'
}

export function segmentLiveTitle(cat: CopSegmentCategory): string {
  switch (cat) {
    case 'explore': return 'Exploring code...'
    case 'exec': return 'Running...'
    case 'edit': return 'Editing...'
    case 'agent': return 'Agent running...'
    case 'fetch': return 'Fetching...'
    case 'generic': return 'Working...'
  }
}

function basename(path: string): string {
  const normalized = path.replace(/\\/g, '/')
  return normalized.split('/').filter(Boolean).pop() ?? path
}

export function segmentCompletedTitle(seg: CopSubSegment): string {
  const calls = seg.items
    .filter((i): i is Extract<CopBlockItem, { kind: 'call' }> => i.kind === 'call')
    .map((i) => i.call)
  if (calls.length === 0) return 'Completed'

  switch (seg.category) {
    case 'explore': {
      const readCalls = calls.filter((c) => normalizeToolName(c.toolName) === 'read_file')
      const readPaths = new Set(readCalls.map((c) => {
        const path = c.arguments?.file_path as string | undefined
          ?? (c.arguments?.source as { file_path?: string } | undefined)?.file_path
        return path || c.toolCallId
      }))
      const searchCount = calls.filter((c) => {
        const n = normalizeToolName(c.toolName)
        return n === 'grep' || n === 'lsp'
      }).length
      const globCount = calls.filter((c) => normalizeToolName(c.toolName) === 'glob').length
      const parts: string[] = []
      if (readPaths.size > 0) parts.push(`Read ${readPaths.size} file${readPaths.size === 1 ? '' : 's'}`)
      if (searchCount > 0) parts.push(`${searchCount} search${searchCount === 1 ? '' : 'es'}`)
      if (globCount > 0) parts.push(`Listed ${globCount} file${globCount === 1 ? '' : 's'}`)
      return parts.length > 0 ? parts.join(', ') : 'Explored code'
    }
    case 'exec': {
      const n = calls.length
      return `${n} step${n === 1 ? '' : 's'} completed`
    }
    case 'edit': {
      const editCall = calls[0]
      const filePath = (editCall?.arguments?.file_path as string | undefined) ?? ''
      return filePath ? `Edited ${basename(filePath)}` : 'Edit completed'
    }
    case 'agent': {
      const n = calls.length
      return n === 1 ? 'Agent completed' : `${n} agent tasks completed`
    }
    case 'fetch': return 'Fetch completed'
    case 'generic': {
      if (calls.length === 1) {
        const t = calls[0]!.toolName
        // Map known generic tool names to readable labels
        const label: Record<string, string> = {
          todo_write: 'Updated todos',
          todo_read: 'Read todos',
        }
        return label[t] ?? t
      }
      return `${calls.length} steps completed`
    }
  }
}

// 聚合统计：跨 segment 收集所有 tool call 的分类计数
export type AggregatedCallStats = {
  readPaths: Set<string>
  searchCount: number
  globCount: number
  editPaths: string[]
  execCount: number
  agentCount: number
  fetchCount: number
  genericCount: number
  byToolName: Map<string, number>
}

type CallItem = Extract<CopBlockItem, { kind: 'call' }>

function getReadPath(c: CallItem['call']): string {
  const direct = c.arguments?.file_path as string | undefined
  if (typeof direct === 'string' && direct) return direct
  const source = c.arguments?.source as { file_path?: string } | undefined
  if (source && typeof source.file_path === 'string' && source.file_path) return source.file_path
  return c.toolCallId
}

function getEditPath(c: CallItem['call']): string {
  const p = c.arguments?.file_path
  return typeof p === 'string' ? p : ''
}

export function aggregateCallStats(calls: ReadonlyArray<CallItem['call']>): AggregatedCallStats {
  const stats: AggregatedCallStats = {
    readPaths: new Set<string>(),
    searchCount: 0,
    globCount: 0,
    editPaths: [],
    execCount: 0,
    agentCount: 0,
    fetchCount: 0,
    genericCount: 0,
    byToolName: new Map<string, number>(),
  }
  for (const c of calls) {
    const n = normalizeToolName(c.toolName)
    stats.byToolName.set(n, (stats.byToolName.get(n) ?? 0) + 1)
    const cat = categoryForTool(c.toolName)
    if (n === 'read_file') {
      stats.readPaths.add(getReadPath(c))
      continue
    }
    if (n === 'grep') { stats.searchCount += 1; continue }
    if (n === 'glob') { stats.globCount += 1; continue }
    if (n === 'lsp') {
      // rename 算 edit，其余算 search
      const op = typeof c.arguments?.operation === 'string' ? c.arguments.operation : ''
      if (MUTATING_LSP.has(op)) {
        const fp = typeof c.arguments?.file_path === 'string' ? c.arguments.file_path : ''
        stats.editPaths.push(fp ? basename(fp) : c.toolCallId)
      } else {
        stats.searchCount += 1
      }
      continue
    }
    if (cat === 'edit') {
      const fp = getEditPath(c)
      stats.editPaths.push(fp ? basename(fp) : c.toolCallId)
      continue
    }
    if (cat === 'exec') { stats.execCount += 1; continue }
    if (cat === 'agent') { stats.agentCount += 1; continue }
    if (cat === 'fetch') { stats.fetchCount += 1; continue }
    stats.genericCount += 1
  }
  return stats
}

function truncate(value: string, max: number): string {
  return value.length > max ? `${value.slice(0, max)}…` : value
}

function lspProgressive(args: Record<string, unknown>): string {
  const op = typeof args.operation === 'string' ? args.operation : ''
  const query = typeof args.query === 'string' ? args.query : ''
  const filePath = typeof args.file_path === 'string' ? args.file_path : ''
  const subject = query || (filePath ? basename(filePath) : '')
  switch (op) {
    case 'definition': return subject ? `Finding definition in ${subject}` : 'Finding definition'
    case 'references': return subject ? `Finding references in ${subject}` : 'Finding references'
    case 'hover': return subject ? `Inspecting symbol in ${subject}` : 'Inspecting symbol'
    case 'document_symbols': return subject ? `Listing symbols in ${subject}` : 'Listing symbols'
    case 'workspace_symbols': return query ? `Searching symbols for ${truncate(query, 36)}` : 'Searching symbols'
    case 'type_definition': return subject ? `Finding type definition in ${subject}` : 'Finding type definition'
    case 'implementation': return subject ? `Finding implementations in ${subject}` : 'Finding implementations'
    case 'diagnostics': return subject ? `Checking diagnostics in ${subject}` : 'Checking diagnostics'
    case 'rename': return subject ? `Renaming symbol in ${subject}` : 'Renaming symbol'
    default: return op ? `Running LSP ${op}` : 'Running LSP'
  }
}

export function presentToProgressive(
  toolNameInput: string,
  args: Record<string, unknown> = {},
  displayDescription?: string,
): string {
  const dd = (displayDescription ?? '').trim()
  if (dd) return dd
  const toolName = normalizeToolName(toolNameInput)
  switch (toolName) {
    case 'read_file': {
      const path = (typeof args.file_path === 'string' && args.file_path)
        || ((args.source as { file_path?: string } | undefined)?.file_path ?? '')
      return path ? `Reading ${truncate(basename(path), 48)}` : 'Reading file'
    }
    case 'grep': {
      const pattern = typeof args.pattern === 'string' ? args.pattern : ''
      return pattern ? `Searching ${truncate(pattern, 48)}` : 'Searching code'
    }
    case 'glob': {
      const pattern = typeof args.pattern === 'string' ? args.pattern : ''
      return pattern ? `Listing ${truncate(pattern, 48)}` : 'Listing files'
    }
    case 'edit':
    case 'edit_file': {
      const fp = typeof args.file_path === 'string' ? args.file_path : ''
      return fp ? `Editing ${truncate(basename(fp), 48)}` : 'Editing file'
    }
    case 'write_file': {
      const fp = typeof args.file_path === 'string' ? args.file_path : ''
      return fp ? `Writing ${truncate(basename(fp), 48)}` : 'Writing file'
    }
    case 'exec_command':
    case 'continue_process':
    case 'terminate_process':
    case 'python_execute': {
      const cmd = (typeof args.cmd === 'string' && args.cmd)
        || (typeof args.command === 'string' && args.command)
        || (typeof args.code === 'string' && args.code) || ''
      if (cmd) return truncate(compactCommandLine(cmd.split('\n')[0]!.trim()), 72)
      return 'Running command'
    }
    case 'load_tools': return 'Loading tools'
    case 'load_skill': return 'Loading skill'
    case 'lsp': return lspProgressive(args)
    default: return `${toolName}...`
  }
}

function pluralize(n: number, singular: string, plural: string): string {
  return n === 1 ? singular : plural
}

function formatStatsParts(stats: AggregatedCallStats): string {
  const parts: string[] = []
  if (stats.editPaths.length === 1) parts.push(`Edited ${stats.editPaths[0]}`)
  else if (stats.editPaths.length > 1) parts.push(`Edited ${stats.editPaths.length} files`)
  if (stats.readPaths.size > 0) parts.push(`Read ${stats.readPaths.size} ${pluralize(stats.readPaths.size, 'file', 'files')}`)
  if (stats.searchCount > 0) parts.push(`${stats.searchCount} ${pluralize(stats.searchCount, 'search', 'searches')}`)
  if (stats.globCount > 0) parts.push(`Listed ${stats.globCount} ${pluralize(stats.globCount, 'file', 'files')}`)
  if (stats.execCount > 0) parts.push(`Ran ${stats.execCount} ${pluralize(stats.execCount, 'command', 'commands')}`)
  if (stats.agentCount > 0) parts.push(`${stats.agentCount} agent ${pluralize(stats.agentCount, 'task', 'tasks')}`)
  if (stats.fetchCount > 0) parts.push(`${stats.fetchCount} ${pluralize(stats.fetchCount, 'fetch', 'fetches')}`)
  return parts.join(', ')
}

function formatSingleCategoryTitle(cat: CopSegmentCategory, stats: AggregatedCallStats, total: number): string {
  switch (cat) {
    case 'explore': {
      const parts = formatStatsParts(stats)
      return parts || 'Explored code'
    }
    case 'exec':
      return `${stats.execCount} ${pluralize(stats.execCount, 'step', 'steps')} completed`
    case 'edit': {
      if (stats.editPaths.length === 1) return `Edited ${stats.editPaths[0]}`
      if (stats.editPaths.length > 1) return `Edited ${stats.editPaths.length} files`
      return 'Edit completed'
    }
    case 'agent':
      return stats.agentCount === 1 ? 'Agent completed' : `${stats.agentCount} agent tasks completed`
    case 'fetch':
      return stats.fetchCount === 1 ? 'Fetch completed' : `${stats.fetchCount} fetches completed`
    case 'generic':
      return `${total} ${pluralize(total, 'step', 'steps')} completed`
  }
}

export function aggregateMainTitle(
  segments: ReadonlyArray<CopSubSegment>,
  isLive: boolean,
  isComplete: boolean,
): string {
  if (segments.length === 0) return ''

  const collectCalls = (segs: ReadonlyArray<CopSubSegment>): CallItem['call'][] => {
    const out: CallItem['call'][] = []
    for (const s of segs) {
      for (const it of s.items) {
        if (it.kind === 'call') out.push(it.call)
      }
    }
    return out
  }

  if (isLive && !isComplete) {
    // 找最后一个 open（或最后一段）的最后一个 call
    let openSeg: CopSubSegment | null = null
    for (let i = segments.length - 1; i >= 0; i--) {
      if (segments[i]!.status === 'open') { openSeg = segments[i]!; break }
    }
    if (!openSeg) openSeg = segments[segments.length - 1]!
    let lastCall: CallItem['call'] | null = null
    for (let i = openSeg.items.length - 1; i >= 0; i--) {
      const it = openSeg.items[i]!
      if (it.kind === 'call') { lastCall = it.call; break }
    }
    const current = lastCall
      ? presentToProgressive(lastCall.toolName, lastCall.arguments, lastCall.displayDescription)
      : segmentLiveTitle(openSeg.category).replace(/\.\.\.$/, '')
    const closedSegs = segments.filter((s) => s !== openSeg && s.status === 'closed')
    const closedCalls = collectCalls(closedSegs)
    if (closedCalls.length === 0) return `${current}...`
    const stats = aggregateCallStats(closedCalls)
    const history = formatStatsParts(stats)
    return history ? `${history} · ${current}...` : `${current}...`
  }

  // complete 态
  const allCalls = collectCalls(segments)
  if (allCalls.length === 0) {
    // 退化路径：没有真实 call（例如 segments 仅作为 title 占位），沿用旧 segment.title 行为
    if (segments.length > 1) return `${segments.length} steps completed`
    return segments[0]!.title || 'Completed'
  }
  const stats = aggregateCallStats(allCalls)
  const cats = Array.from(new Set(segments.map((s) => s.category)))
  if (cats.length === 1) return formatSingleCategoryTitle(cats[0]!, stats, allCalls.length)
  const parts = formatStatsParts(stats)
  return parts || `${allCalls.length} ${pluralize(allCalls.length, 'step', 'steps')} completed`
}

export function buildSubSegments(items: CopBlockItem[]): CopSubSegment[] {
  const segments: CopSubSegment[] = []
  let currentItems: CopBlockItem[] = []
  let currentCat: CopSegmentCategory | null = null
  let pendingLead: CopBlockItem[] = [] // think/text before any tool call

  const closeCurrent = () => {
    if (currentItems.length === 0 && pendingLead.length === 0) return
    if (currentCat == null) return
    const allItems = [...pendingLead, ...currentItems]
    if (allItems.length === 0) return
    const seg: CopSubSegment = {
      id: `seg-${segments.length}`,
      category: currentCat,
      status: 'closed',
      items: allItems,
      seq: allItems[0]?.seq ?? 0,
      title: segmentLiveTitle(currentCat),
    }
    seg.title = segmentCompletedTitle(seg)
    segments.push(seg)
    currentItems = []
    currentCat = null
    pendingLead = []
  }

  for (const item of items) {
    if (item.kind === 'call') {
      const cat = categoryForTool(item.call.toolName)
      if (currentCat !== null && cat !== currentCat) {
        closeCurrent()
      }
      if (currentCat === null) {
        currentCat = cat
        currentItems = []
      }
      currentItems.push(item)
    } else {
      // thinking or assistant_text
      if (currentCat !== null) {
        currentItems.push(item)
      } else {
        pendingLead.push(item)
      }
    }
  }

  closeCurrent()

  return segments
}

// -- Resolved pool builder --

import type { CopTimelinePayload } from './copSegmentTimeline'

function mapById<T extends { id: string }>(arr: T[]): Map<string, T> {
  const m = new Map<string, T>()
  for (const item of arr) m.set(item.id, item)
  return m
}

export type ResolvedPool = {
  codeExecutions: Map<string, import('./storage').CodeExecutionRef>
  fileOps: Map<string, import('./storage').FileOpRef>
  webFetches: Map<string, import('./storage').WebFetchRef>
  subAgents: Map<string, import('./storage').SubAgentRef>
  genericTools: Map<string, import('./copSegmentTimeline').GenericToolCallRef>
  steps: Map<string, import('./components/cop-timeline/types').WebSearchPhaseStep>
  sources: import('./storage').WebSource[]
}

export function buildResolvedPool(payload: CopTimelinePayload): ResolvedPool {
  const fileOps = new Map<string, import('./storage').FileOpRef>()
  for (const op of payload.fileOps ?? []) fileOps.set(op.id, op)
  for (const group of payload.exploreGroups ?? []) {
    for (const op of group.items) fileOps.set(op.id, op)
  }
  return {
    codeExecutions: mapById(payload.codeExecutions ?? []),
    fileOps,
    webFetches: mapById(payload.webFetches ?? []),
    subAgents: mapById(payload.subAgents ?? []),
    genericTools: mapById(payload.genericTools ?? []),
    steps: mapById(payload.steps),
    sources: payload.sources,
  }
}

function emptyMap<K extends string, V>(): Map<K, V> {
  return new Map<K, V>()
}

export const EMPTY_POOL: ResolvedPool = {
  codeExecutions: emptyMap(),
  fileOps: emptyMap(),
  webFetches: emptyMap(),
  subAgents: emptyMap(),
  genericTools: emptyMap(),
  steps: emptyMap(),
  sources: [],
}

export function buildFallbackSegments(tools: {
  codeExecutions?: Array<{ id: string; seq?: number }> | null
  subAgents?: Array<{ id: string; sourceTool?: string; seq?: number }> | null
  fileOps?: Array<{ id: string; toolName: string; seq?: number }> | null
  webFetches?: Array<{ id: string; seq?: number }> | null
}): CopSubSegment[] {
  const segs: CopSubSegment[] = []
  const allTools: Array<{ id: string; toolName: string; seq?: number }> = []
  if (tools.codeExecutions) {
    for (const c of tools.codeExecutions) allTools.push({ id: c.id, toolName: 'exec_command', seq: c.seq })
  }
  if (tools.subAgents) {
    for (const a of tools.subAgents) allTools.push({ id: a.id, toolName: a.sourceTool ?? 'spawn_agent', seq: a.seq })
  }
  if (tools.fileOps) {
    for (const f of tools.fileOps) allTools.push({ id: f.id, toolName: f.toolName, seq: f.seq })
  }
  if (tools.webFetches) {
    for (const w of tools.webFetches) allTools.push({ id: w.id, toolName: 'web_fetch', seq: w.seq })
  }
  allTools.sort((a, b) => (a.seq ?? 0) - (b.seq ?? 0))
  if (allTools.length === 0) return segs

  const items: CopBlockItem[] = allTools.map((t) => ({
    kind: 'call' as const,
    call: { toolCallId: t.id, toolName: t.toolName, arguments: {} as Record<string, unknown> },
    seq: t.seq ?? 0,
  }))

  segs.push({
    id: 'fallback-tools',
    category: 'generic',
    status: 'closed',
    items,
    seq: items[0]?.seq ?? 0,
    title: `${allTools.length} step${allTools.length === 1 ? '' : 's'} completed`,
  })
  return segs
}

export function buildThinkingOnlyFromItems(items: { kind: string; content?: string; seq: number; startedAtMs?: number; endedAtMs?: number }[]): { markdown: string; live?: boolean; durationSec: number; startedAtMs?: number } | null {
  let markdown = ''
  let live = false
  let startedAtMs: number | undefined
  let endedAtMs: number | undefined
  for (const item of items) {
    if (item.kind === 'thinking' && item.content) {
      markdown += item.content
      if (item.endedAtMs == null) live = true
      if (startedAtMs == null && item.startedAtMs != null) startedAtMs = item.startedAtMs
      if (item.endedAtMs != null) endedAtMs = item.endedAtMs
    }
  }
  if (!markdown.trim()) return null
  const durationSec = startedAtMs != null && endedAtMs != null
    ? Math.max(0, Math.round((endedAtMs - startedAtMs) / 1000))
    : 0
  return { markdown, live, durationSec, startedAtMs }
}
