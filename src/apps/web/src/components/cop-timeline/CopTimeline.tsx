import { useState, useEffect, useRef } from 'react'
import { motion, useReducedMotion } from 'framer-motion'
import { ChevronRight } from 'lucide-react'
import type { CodeExecution } from '../CodeExecutionCard'
import { CodeExecutionCard } from '../CodeExecutionCard'
import { ExecutionCard } from '../ExecutionCard'
import type { SubAgentRef } from '../../storage'
import { useLocale } from '../../contexts/LocaleContext'
import type { CopSubSegment, ResolvedPool } from '../../copSubSegment'
import { aggregateMainTitle } from '../../copSubSegment'
import { recordPerfCount, recordPerfValue } from '../../perfDebug'
import {
  COP_TIMELINE_THINKING_PLAIN_LINE_HEIGHT_PX,
  COP_TIMELINE_DOT_TOP,
  COP_TIMELINE_DOT_SIZE,
} from './utils'
import {
  useThinkingElapsedSeconds,
  formatThinkingHeaderLabel,
  CopTimelineHeaderLabel,
} from './CopTimelineHeader'
import { AssistantThinkingMarkdown } from './ThinkingBlock'
import { CopTimelineSegment } from './CopTimelineSegment'
import { CopTimelineUnifiedRow } from './CopUnifiedRow'

export type { WebSearchPhaseStep } from './types'

export function CopTimeline({
  segments,
  pool,
  thinkingOnly,
  thinkingHint,
  headerOverride,
  isComplete,
  live,
  shimmer,
  compactNarrativeEnd = false,
  onOpenCodeExecution,
  activeCodeExecutionId,
  onOpenSubAgent,
  accessToken,
  baseUrl,
  typography = 'default',
}: {
  segments: CopSubSegment[]
  pool: ResolvedPool
  thinkingOnly?: { markdown: string; live?: boolean; durationSec: number; startedAtMs?: number } | null
  thinkingHint?: string
  headerOverride?: string
  isComplete: boolean
  live?: boolean
  shimmer?: boolean
  compactNarrativeEnd?: boolean
  onOpenCodeExecution?: (ce: CodeExecution) => void
  activeCodeExecutionId?: string
  onOpenSubAgent?: (agent: SubAgentRef) => void
  accessToken?: string
  baseUrl?: string
  typography?: 'default' | 'work'
}) {
  const { t } = useLocale()
  const reduceMotion = useReducedMotion()

  const poolHasItems = pool.codeExecutions.size > 0 || pool.fileOps.size > 0 || pool.webFetches.size > 0 || pool.subAgents.size > 0 || pool.genericTools.size > 0 || pool.steps.size > 0
  const hasSegments = segments.length > 0 || poolHasItems
  const hasThinkingOnly = thinkingOnly != null && segments.length === 0 && !poolHasItems
  const anyThinking = thinkingOnly != null
  const thinkingLive = thinkingOnly?.live ?? false
  const anyThinkingLive = thinkingLive
  const timelineIsLive = !!live || anyThinkingLive
  const bodyHasContent = hasSegments || hasThinkingOnly
  const [collapsed, setCollapsed] = useState(() => {
    if (timelineIsLive && !isComplete) return hasThinkingOnly
    if (hasThinkingOnly && !isComplete) return false
    if (hasThinkingOnly && isComplete) return true
    return true
  })
  const [userToggled, setUserToggled] = useState(false)
  const prevLive = useRef(timelineIsLive)

  // Auto-collapse when live ends
  useEffect(() => {
    if (userToggled) return
    if (prevLive.current && !timelineIsLive && isComplete) {
      setCollapsed(true)
    }
    prevLive.current = timelineIsLive
  }, [timelineIsLive, isComplete, userToggled])

  // Auto-expand when new segment appears (live mode)
  useEffect(() => {
    if (userToggled) return
    if (timelineIsLive && !isComplete) {
      setCollapsed(hasThinkingOnly)
    }
    if (hasThinkingOnly && isComplete) {
      setCollapsed(true)
    }
  }, [hasThinkingOnly, isComplete, timelineIsLive, userToggled])

  const aggregatedDurationSec = thinkingOnly?.durationSec ?? 0
  const segmentThinkingStartedAtMs = thinkingOnly?.startedAtMs

  const pendingHasContent = hasSegments || hasThinkingOnly
  const pendingShowThinkingHeader = !!live && !anyThinking && !pendingHasContent && !!thinkingHint
  const thinkingTimerActive = anyThinkingLive || (anyThinking && !!live)
  const activeThinkingElapsed = useThinkingElapsedSeconds(thinkingTimerActive, segmentThinkingStartedAtMs)
  const thinkingLiveHeaderLabel = formatThinkingHeaderLabel(thinkingHint, activeThinkingElapsed, t)

  const shouldRender = hasSegments || hasThinkingOnly || !!thinkingHint

  const nonExecSegments = segments.filter((s) => s.category !== 'exec')
  const nonExecTimelineLive = !!live && nonExecSegments.some((s) => s.status === 'open')
  const hasNonExecBody = nonExecSegments.length > 0 || hasThinkingOnly || anyThinking || (!hasSegments && !!thinkingHint)

  const thoughtDurationLabel =
    aggregatedDurationSec > 0
      ? t.copTimelineThoughtForSeconds(aggregatedDurationSec)
      : t.copTimelineThinkingDoneNoDuration

  const headerPhaseKey: string = (anyThinkingLive || (anyThinking && !!live))
    ? 'thinking-live'
    : anyThinking
      ? 'thought'
      : pendingShowThinkingHeader
        ? 'thinking-pending'
        : nonExecTimelineLive
          ? 'live'
          : isComplete
            ? 'complete'
            : 'idle'

  const headerLabel = headerOverride ?? (() => {
    if (anyThinkingLive || (anyThinking && live)) return thinkingLiveHeaderLabel
    if (anyThinking && isComplete && !hasSegments) return thoughtDurationLabel
    if (pendingShowThinkingHeader) return `${thinkingHint}...`
    if (hasSegments) {
      const nonExecComplete = isComplete || (!!live && !nonExecTimelineLive)
      const aggregated = aggregateMainTitle(nonExecSegments, nonExecTimelineLive, nonExecComplete)
      if (aggregated) return aggregated
    }
    if (anyThinking) return thoughtDurationLabel
    if (isComplete) return 'Completed'
    return thinkingHint ? `${thinkingHint}...` : 'Working...'
  })()

  const seededStatusAnimation =
    nonExecTimelineLive || !!shimmer || headerPhaseKey === 'thinking-pending' || headerPhaseKey === 'thinking-live'
  const headerUsesIncrementalTypewriter = seededStatusAnimation

  const [hovered, setHovered] = useState(false)

  useEffect(() => {
    recordPerfCount('cop_timeline_render', 1, {
      segments: segments.length,
      thinkingOnly: !!hasThinkingOnly,
      live: !!live,
      collapsed,
    })
    recordPerfValue('cop_timeline_segments', segments.length, 'segments', {
      collapsed,
      live: !!live,
    })
  }, [collapsed, hasThinkingOnly, live, segments.length])

  if (!shouldRender) return null

  // exec items: 收集所有 exec segment 的 call items，保持原始 segment 顺序
  type ExecCallItem = Extract<import('../../assistantTurnSegments').CopBlockItem, { kind: 'call' }>
  const execCallItems: ExecCallItem[] = []
  for (const seg of segments) {
    if (seg.category !== 'exec') continue
    for (const item of seg.items) {
      if (item.kind === 'call') execCallItems.push(item)
    }
  }

  const renderExecItems = () =>
    execCallItems.map((callItem) => {
      const ce = pool.codeExecutions.get(callItem.call.toolCallId)
      if (!ce) return null
      return (
        <div key={callItem.call.toolCallId} style={{ padding: '4px 0' }}>
          {ce.language === 'shell'
            ? <ExecutionCard variant="shell" displayDescription={ce.displayDescription} code={ce.code} output={ce.output} status={ce.status} errorMessage={ce.errorMessage} smooth={!!live && ce.status === 'running'} />
            : <CodeExecutionCard language={ce.language} code={ce.code} output={ce.output} errorMessage={ce.errorMessage} status={ce.status} onOpen={onOpenCodeExecution ? () => onOpenCodeExecution(ce) : undefined} isActive={activeCodeExecutionId === ce.id} />
          }
        </div>
      )
    })

  // exec-only: 没有非 exec 内容，直接平铺
  if (!hasNonExecBody && execCallItems.length > 0) {
    return (
      <div className={`cop-timeline-root${typography === 'work' ? ' cop-timeline-root--work' : ''}`} style={{ maxWidth: '663px' }}>
        {renderExecItems()}
      </div>
    )
  }

  const toggleBody = () => {
    setUserToggled(true)
    setCollapsed((v) => !v)
  }

  return (
    <div className={`cop-timeline-root${typography === 'work' ? ' cop-timeline-root--work' : ''}`} style={{ maxWidth: '663px' }}>
      {hasNonExecBody && (
        <>
          <button
            type="button"
            onMouseEnter={() => setHovered(true)}
            onMouseLeave={() => setHovered(false)}
            onClick={toggleBody}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
              padding: '4px 0 2px',
              background: 'none',
              border: 'none',
              cursor: 'pointer',
              color: hovered ? 'var(--c-cop-row-hover-fg)' : 'var(--c-cop-row-fg)',
              fontSize: 'var(--c-cop-row-font-size)',
              fontWeight: 400,
              lineHeight: 'var(--c-cop-row-line-height)',
              transition: 'color 0.15s ease',
              maxWidth: '100%',
              minWidth: 0,
              alignSelf: 'stretch',
            }}
          >
            <CopTimelineHeaderLabel
              text={headerLabel}
              phaseKey={headerPhaseKey}
              shimmer={!!shimmer}
              incremental={headerUsesIncrementalTypewriter}
            />
            {(isComplete || live) && bodyHasContent && (
              <motion.div
                animate={{ rotate: collapsed ? 0 : 90 }}
                transition={{ duration: 0.2, ease: 'easeOut' }}
                style={{ display: 'flex', flexShrink: 0 }}
              >
                <ChevronRight size={13} />
              </motion.div>
            )}
          </button>

          <motion.div
            initial={false}
            animate={{ height: collapsed ? 0 : 'auto', opacity: collapsed ? 0 : 1 }}
            transition={!reduceMotion ? { duration: 0.24, ease: [0.4, 0, 0.2, 1] } : { duration: 0 }}
            style={{ overflow: collapsed ? 'hidden' : 'visible' }}
          >
            <div style={{ position: 'relative', paddingTop: '3px', paddingBottom: '3px', paddingLeft: (nonExecSegments.length > 1 || hasThinkingOnly) ? '24px' : undefined }}>
              {/* Thinking-only mode (no segments) */}
              {hasThinkingOnly && thinkingOnly && (() => {
                const showDone = isComplete && !thinkingOnly.live
                const multiItems = showDone
                return (
                  <>
                    <CopTimelineUnifiedRow
                      isFirst={true}
                      isLast={!showDone}
                      multiItems={multiItems}
                      dotColor={thinkingOnly.live && !isComplete ? 'var(--c-text-secondary)' : 'var(--c-border-mid)'}
                      dotTop={COP_TIMELINE_DOT_TOP}
                      paddingBottom={8}
                      horizontalMotion={false}
                    >
                      <div
                        style={{
                          paddingTop: Math.max(0, COP_TIMELINE_DOT_TOP + COP_TIMELINE_DOT_SIZE / 2 - COP_TIMELINE_THINKING_PLAIN_LINE_HEIGHT_PX / 2),
                        }}
                      >
                        <AssistantThinkingMarkdown
                          markdown={thinkingOnly.markdown}
                          live={!!thinkingOnly.live && !isComplete}
                          variant="timeline-plain"
                        />
                      </div>
                    </CopTimelineUnifiedRow>
                    {showDone && (
                      <CopTimelineUnifiedRow
                        isFirst={false}
                        isLast={true}
                        multiItems={multiItems}
                        dotColor="var(--c-text-muted)"
                        dotTop={COP_TIMELINE_DOT_TOP}
                        paddingBottom={0}
                        horizontalMotion={false}
                      >
                        <div
                          style={{
                            fontSize: 'var(--c-cop-row-font-size)',
                            color: 'var(--c-cop-row-fg)',
                            lineHeight: 'var(--c-cop-row-line-height)',
                            paddingTop: '3px',
                          }}
                        >
                          {t.copThinkingDone as string}
                        </div>
                      </CopTimelineUnifiedRow>
                    )}
                  </>
                )
              })()}

              {/* Non-exec segments */}
              {nonExecSegments.length === 1 ? (
                <CopTimelineSegment
                  segment={nonExecSegments[0]!}
                  pool={pool}
                  isLive={!!live && nonExecSegments[0]!.status === 'open'}
                  defaultExpanded={true}
                  hideHeader
                  compactNarrativeEnd={compactNarrativeEnd}
                  onOpenCodeExecution={onOpenCodeExecution}
                  activeCodeExecutionId={activeCodeExecutionId}
                  onOpenSubAgent={onOpenSubAgent}
                  accessToken={accessToken}
                  baseUrl={baseUrl}
                  typography={typography}
                />
              ) : (
                nonExecSegments.map((seg, index) => {
                const isLast = index === nonExecSegments.length - 1
                const segDotColor = seg.status === 'open'
                  ? 'var(--c-text-secondary)'
                  : 'var(--c-text-muted)'
                return (
                  <CopTimelineUnifiedRow
                    key={seg.id}
                    isFirst={index === 0}
                    isLast={isLast}
                    multiItems={nonExecSegments.length >= 2}
                    dotColor={segDotColor}
                    dotTop={8}
                    paddingBottom={7}
                    horizontalMotion={false}
                  >
                    <CopTimelineSegment
                      segment={seg}
                      pool={pool}
                      isLive={!!live && seg.status === 'open'}
                      defaultExpanded={isLast && (!isComplete || seg.status === 'open')}
                      compactNarrativeEnd={compactNarrativeEnd}
                      onOpenCodeExecution={onOpenCodeExecution}
                      activeCodeExecutionId={activeCodeExecutionId}
                      onOpenSubAgent={onOpenSubAgent}
                      accessToken={accessToken}
                      baseUrl={baseUrl}
                      typography={typography}
                    />
                  </CopTimelineUnifiedRow>
                )
              }))}
            </div>
          </motion.div>
        </>
      )}

      {/* Exec items: 始终在折叠体外，与 header/segment 平级 */}
      {renderExecItems()}
    </div>
  )
}
