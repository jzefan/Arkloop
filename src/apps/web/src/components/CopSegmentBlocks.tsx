import type { AssistantTurnSegment } from '../assistantTurnSegments'
import type { CodeExecution } from './CodeExecutionCard'
import type { CodeExecutionRef, FileOpRef, SubAgentRef, WebFetchRef, WebSource } from '../storage'
import type { WebSearchPhaseStep } from './cop-timeline/CopTimeline'
import { CopTimeline } from './cop-timeline/CopTimeline'
import { buildResolvedPool, buildSubSegments, buildThinkingOnlyFromItems, segmentLiveTitle } from '../copSubSegment'
import {
  copTimelinePayloadForSegment,
  splitCopItemsByTopLevelTools,
} from '../copSegmentTimeline'
import { TopLevelCopToolBlock, type TopLevelCopToolEntry } from './TopLevelCopToolBlock'

type CopSegment = Extract<AssistantTurnSegment, { type: 'cop' }>

type Props = {
  segment: CopSegment
  keyPrefix: string
  codeExecutions?: CodeExecutionRef[] | null
  fileOps?: FileOpRef[] | null
  webFetches?: WebFetchRef[] | null
  subAgents?: SubAgentRef[] | null
  searchSteps?: WebSearchPhaseStep[] | null
  sources: WebSource[]
  isComplete: boolean
  live?: boolean
  shimmer?: boolean
  thinkingHint?: string
  headerOverride?: string
  compactNarrativeEnd?: boolean
  onOpenCodeExecution?: (ce: CodeExecution) => void
  activeCodeExecutionId?: string
  onOpenSubAgent?: (agent: SubAgentRef) => void
  accessToken?: string
  baseUrl?: string
  typography?: 'default' | 'work'
}

function topLevelEntryForTool(
  entry: Extract<ReturnType<typeof splitCopItemsByTopLevelTools>[number], { kind: 'tool' }>,
  payload: ReturnType<typeof copTimelinePayloadForSegment>,
): TopLevelCopToolEntry | null {
  const id = entry.item.call.toolCallId
  const codeExecution = payload.codeExecutions?.find((item) => item.id === id)
  if (codeExecution) {
    return { kind: 'code', id, seq: entry.seq, item: codeExecution }
  }
  const todo = payload.todoWrites?.find((item) => item.id === id)
  if (todo) {
    return { kind: 'todo', id, seq: entry.seq, item: todo }
  }
  return null
}

export function CopSegmentBlocks({
  segment,
  keyPrefix,
  codeExecutions,
  fileOps,
  webFetches,
  subAgents,
  searchSteps,
  sources,
  isComplete,
  live,
  shimmer,
  thinkingHint,
  headerOverride,
  compactNarrativeEnd,
  onOpenCodeExecution,
  activeCodeExecutionId,
  onOpenSubAgent,
  accessToken,
  baseUrl,
  typography = 'default',
}: Props) {
  const splitEntries = splitCopItemsByTopLevelTools(segment.items)
  if (splitEntries.length === 0) return null

  const pools = { codeExecutions, fileOps, webFetches, subAgents, searchSteps, sources }
  const fullPayload = copTimelinePayloadForSegment(segment, pools)
  const timelineEntryCount = splitEntries.filter((entry) => entry.kind === 'timeline').length

  return (
    <>
      {splitEntries.map((entry, index) => {
        const entryLive = !!live && index === splitEntries.length - 1
        const entryComplete = isComplete || !entryLive

        if (entry.kind === 'tool') {
          const toolEntry = topLevelEntryForTool(entry, fullPayload)
          if (!toolEntry) return null
          return (
            <TopLevelCopToolBlock
              key={`${keyPrefix}-tool-${entry.id}`}
              entry={toolEntry}
              live={entryLive}
              onOpenCodeExecution={onOpenCodeExecution}
              activeCodeExecutionId={activeCodeExecutionId}
            />
          )
        }

        const timelineSegment: CopSegment = { ...segment, items: entry.items }
        const payload = copTimelinePayloadForSegment(timelineSegment, pools)
        const pool = buildResolvedPool(payload)
        const subSegments = buildSubSegments(entry.items)
        if (subSegments.length > 0 && entryLive) {
          const lastSeg = subSegments[subSegments.length - 1]!
          lastSeg.status = 'open'
          lastSeg.title = segmentLiveTitle(lastSeg.category)
        }

        const thinkingOnlyData = subSegments.length === 0 &&
          !payload.codeExecutions?.length &&
          !payload.subAgents?.length &&
          !payload.fileOps?.length &&
          !payload.webFetches?.length &&
          !payload.genericTools?.length &&
          !payload.todoWrites?.length
          ? buildThinkingOnlyFromItems(entry.items)
          : null

        const hasTimelineBody =
          subSegments.length > 0 ||
          thinkingOnlyData != null ||
          payload.steps.length > 0 ||
          payload.sources.length > 0 ||
          !!payload.fileOps?.length ||
          !!payload.webFetches?.length ||
          !!payload.genericTools?.length ||
          !!payload.subAgents?.length ||
          !!(payload.exploreGroups && payload.exploreGroups.length > 0)

        if (!hasTimelineBody) return null

        return (
          <CopTimeline
            key={`${keyPrefix}-timeline-${entry.id}`}
            segments={subSegments}
            pool={pool}
            thinkingOnly={thinkingOnlyData}
            thinkingHint={thinkingHint}
            headerOverride={timelineEntryCount === 1 ? headerOverride : undefined}
            isComplete={entryComplete}
            live={entryLive}
            shimmer={entryLive && !!shimmer}
            compactNarrativeEnd={compactNarrativeEnd}
            onOpenCodeExecution={onOpenCodeExecution}
            onOpenSubAgent={onOpenSubAgent}
            activeCodeExecutionId={activeCodeExecutionId}
            accessToken={accessToken}
            baseUrl={baseUrl}
            typography={typography}
          />
        )
      })}
    </>
  )
}
