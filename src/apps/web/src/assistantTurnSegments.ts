import {
  assistantTurnPlainText,
  buildAssistantTurnFromEvents as buildSharedAssistantTurnFromEvents,
  copSegmentCalls,
  createEmptyAssistantTurnFoldState,
  drainAssistantTurnForPersist,
  finalizeAssistantTurnFoldState,
  foldAssistantTurnEvent as foldSharedAssistantTurnEvent,
  requestAssistantTurnThinkingBreak,
  snapshotAssistantTurn,
  type AssistantTurnEvent,
  type AssistantTurnFoldState,
  type AssistantTurnSegment,
  type AssistantTurnUi,
  type CopBlockItem,
  type TurnToolCallRef,
} from '../../shared/src/assistantTurn'
import type { AgentUIEvent } from './agent-ui/contract'

export {
  assistantTurnPlainText,
  copSegmentCalls,
  createEmptyAssistantTurnFoldState,
  drainAssistantTurnForPersist,
  finalizeAssistantTurnFoldState,
  requestAssistantTurnThinkingBreak,
  snapshotAssistantTurn,
  type AssistantTurnEvent,
  type AssistantTurnFoldState,
  type AssistantTurnSegment,
  type AssistantTurnUi,
  type CopBlockItem,
  type TurnToolCallRef,
}

function toAssistantTurnEventType(type: string): string {
  switch (type) {
    case 'assistant-delta':
      return 'message.delta'
    case 'tool-call':
      return 'tool.call'
    case 'tool-result':
      return 'tool.result'
    case 'segment-start':
      return 'run.segment.start'
    case 'segment-end':
      return 'run.segment.end'
    default:
      return type
  }
}

function toAssistantTurnEvent(event: AgentUIEvent): AssistantTurnEvent {
  return {
    event_id: event.id,
    run_id: event.streamId,
    seq: event.order,
    ts: event.timestamp,
    type: toAssistantTurnEventType(event.type),
    data: event.data,
    tool_name: event.toolName,
    error_class: event.errorCode,
  }
}

export function foldAssistantTurnEvent(
  state: AssistantTurnFoldState,
  event: AgentUIEvent,
): void {
  foldSharedAssistantTurnEvent(state, toAssistantTurnEvent(event))
}

export function buildAssistantTurnFromAgentEvents(
  events: readonly AgentUIEvent[],
): AssistantTurnUi {
  return buildSharedAssistantTurnFromEvents(events.map(toAssistantTurnEvent))
}
