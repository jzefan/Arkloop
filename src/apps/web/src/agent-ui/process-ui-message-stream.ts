import type {
  AgentReasoningUIPart,
  AgentTextUIPart,
  AgentToolUIPart,
  AgentUIMessage,
  AgentUIMessageChunk,
} from './contract'

export type StreamingAgentUIMessageState<UI_MESSAGE extends AgentUIMessage = AgentUIMessage> = {
  message: UI_MESSAGE
  activeTextParts: Record<string, AgentTextUIPart>
  activeReasoningParts: Record<string, AgentReasoningUIPart>
  partialToolInputs: Record<string, { toolName: string; text: string }>
}

export function createStreamingAgentUIMessageState<UI_MESSAGE extends AgentUIMessage>({
  lastMessage,
  messageId,
}: {
  lastMessage?: UI_MESSAGE
  messageId: string
}): StreamingAgentUIMessageState<UI_MESSAGE> {
  return {
    message: lastMessage?.role === 'assistant'
      ? lastMessage
      : ({
          id: messageId,
          role: 'assistant',
          parts: [],
        } as unknown as UI_MESSAGE),
    activeTextParts: {},
    activeReasoningParts: {},
    partialToolInputs: {},
  }
}

function mergeMetadata(current: unknown, incoming: unknown): unknown {
  if (!incoming || typeof incoming !== 'object') return current ?? incoming
  if (!current || typeof current !== 'object') return incoming
  return { ...current, ...incoming }
}

function findToolPart(parts: AgentUIMessage['parts'], toolCallId: string): AgentToolUIPart | undefined {
  return parts.find((part): part is AgentToolUIPart => (
    part.type === 'dynamic-tool' && part.toolCallId === toolCallId
  ))
}

function findToolPartByApprovalId(parts: AgentUIMessage['parts'], approvalId: string): AgentToolUIPart | undefined {
  return parts.find((part): part is AgentToolUIPart => (
    part.type === 'dynamic-tool' && part.approval?.id === approvalId
  ))
}

function parsePartialInput(text: string): unknown {
  try {
    return JSON.parse(text) as unknown
  } catch {
    return undefined
  }
}

function upsertToolPart(
  state: StreamingAgentUIMessageState,
  next: Omit<AgentToolUIPart, 'type'> & Record<string, unknown>,
): void {
  const current = findToolPart(state.message.parts, next.toolCallId)
  if (!current) {
    state.message.parts.push({ type: 'dynamic-tool', ...next } as AgentToolUIPart)
    return
  }
  Object.assign(current, next)
}

export function processAgentUIMessageStream<UI_MESSAGE extends AgentUIMessage>({
  stream,
  state,
  onError,
  onUpdate,
}: {
  stream: ReadableStream<AgentUIMessageChunk>
  state: StreamingAgentUIMessageState<UI_MESSAGE>
  onError?: (error: Error) => void
  onUpdate?: (state: StreamingAgentUIMessageState<UI_MESSAGE>) => void
}): ReadableStream<AgentUIMessageChunk> {
  return stream.pipeThrough(new TransformStream<AgentUIMessageChunk, AgentUIMessageChunk>({
    transform(chunk, controller) {
      switch (chunk.type) {
        case 'start':
          if (chunk.messageId) state.message.id = chunk.messageId
          state.message.metadata = mergeMetadata(
            state.message.metadata,
            chunk.messageMetadata,
          ) as UI_MESSAGE['metadata']
          break
        case 'message-metadata':
          state.message.metadata = mergeMetadata(
            state.message.metadata,
            chunk.messageMetadata,
          ) as UI_MESSAGE['metadata']
          break
        case 'finish':
          state.message.metadata = mergeMetadata(
            state.message.metadata,
            chunk.messageMetadata,
          ) as UI_MESSAGE['metadata']
          break
        case 'text-start': {
          const part: AgentTextUIPart = {
            type: 'text',
            text: '',
            state: 'streaming',
            providerMetadata: chunk.providerMetadata,
          }
          state.activeTextParts[chunk.id] = part
          state.message.parts.push(part)
          break
        }
        case 'text-delta': {
          const part = state.activeTextParts[chunk.id]
          if (part) {
            part.text += chunk.delta
            part.providerMetadata = chunk.providerMetadata ?? part.providerMetadata
          }
          break
        }
        case 'text-end': {
          const part = state.activeTextParts[chunk.id]
          if (part) {
            part.state = 'done'
            part.providerMetadata = chunk.providerMetadata ?? part.providerMetadata
            delete state.activeTextParts[chunk.id]
          }
          break
        }
        case 'reasoning-start': {
          const part: AgentReasoningUIPart = {
            type: 'reasoning',
            text: '',
            state: 'streaming',
            providerMetadata: chunk.providerMetadata,
          }
          state.activeReasoningParts[chunk.id] = part
          state.message.parts.push(part)
          break
        }
        case 'reasoning-delta': {
          const part = state.activeReasoningParts[chunk.id]
          if (part) {
            part.text += chunk.delta
            part.providerMetadata = chunk.providerMetadata ?? part.providerMetadata
          }
          break
        }
        case 'reasoning-end': {
          const part = state.activeReasoningParts[chunk.id]
          if (part) {
            part.state = 'done'
            part.providerMetadata = chunk.providerMetadata ?? part.providerMetadata
            delete state.activeReasoningParts[chunk.id]
          }
          break
        }
        case 'custom':
          state.message.parts.push({
            type: 'custom',
            kind: chunk.kind,
            providerMetadata: chunk.providerMetadata,
          })
          break
        case 'tool-input-start':
          state.partialToolInputs[chunk.toolCallId] = { toolName: chunk.toolName, text: '' }
          upsertToolPart(state, {
            toolCallId: chunk.toolCallId,
            toolName: chunk.toolName,
            title: chunk.title,
            providerExecuted: chunk.providerExecuted,
            callProviderMetadata: chunk.providerMetadata,
            state: 'input-streaming',
            input: undefined,
          })
          break
        case 'tool-input-delta': {
          const partial = state.partialToolInputs[chunk.toolCallId]
          if (partial) {
            partial.text += chunk.inputTextDelta
            upsertToolPart(state, {
              toolCallId: chunk.toolCallId,
              toolName: partial.toolName,
              state: 'input-streaming',
              input: parsePartialInput(partial.text),
            })
          }
          break
        }
        case 'tool-input-available':
          delete state.partialToolInputs[chunk.toolCallId]
          upsertToolPart(state, {
            toolCallId: chunk.toolCallId,
            toolName: chunk.toolName,
            title: chunk.title,
            providerExecuted: chunk.providerExecuted,
            callProviderMetadata: chunk.providerMetadata,
            state: 'input-available',
            input: chunk.input,
          })
          break
        case 'tool-input-error': {
          const current = findToolPart(state.message.parts, chunk.toolCallId)
          upsertToolPart(state, {
            toolCallId: chunk.toolCallId,
            toolName: chunk.toolName,
            title: chunk.title ?? current?.title,
            providerExecuted: chunk.providerExecuted ?? current?.providerExecuted,
            state: 'output-error',
            input: current && 'input' in current ? current.input : undefined,
            rawInput: chunk.input,
            errorText: chunk.errorText,
            callProviderMetadata: chunk.providerMetadata,
          })
          break
        }
        case 'tool-approval-request': {
          const current = findToolPart(state.message.parts, chunk.toolCallId)
          if (current && 'input' in current) {
            Object.assign(current, {
              state: 'approval-requested',
              approval: {
                id: chunk.approvalId,
                ...(chunk.isAutomatic ? { isAutomatic: true } : {}),
              },
            })
          }
          break
        }
        case 'tool-approval-response': {
          const current = findToolPartByApprovalId(state.message.parts, chunk.approvalId)
          if (current && 'input' in current) {
            Object.assign(current, {
              state: 'approval-responded',
              providerExecuted: chunk.providerExecuted ?? current.providerExecuted,
              callProviderMetadata: chunk.providerMetadata ?? current.callProviderMetadata,
              approval: {
                id: chunk.approvalId,
                approved: chunk.approved,
                ...(chunk.reason ? { reason: chunk.reason } : {}),
                ...(current.approval?.isAutomatic ? { isAutomatic: true } : {}),
              },
            })
          }
          break
        }
        case 'tool-output-available': {
          const current = findToolPart(state.message.parts, chunk.toolCallId)
          upsertToolPart(state, {
            toolCallId: chunk.toolCallId,
            toolName: current?.toolName ?? 'tool',
            providerExecuted: chunk.providerExecuted ?? current?.providerExecuted,
            state: 'output-available',
            input: current && 'input' in current ? current.input : undefined,
            output: chunk.output,
            preliminary: chunk.preliminary,
            callProviderMetadata: current?.callProviderMetadata,
            resultProviderMetadata: chunk.providerMetadata,
            approval: current?.approval?.approved === true ? current.approval : undefined,
          })
          break
        }
        case 'tool-output-error': {
          const current = findToolPart(state.message.parts, chunk.toolCallId)
          upsertToolPart(state, {
            toolCallId: chunk.toolCallId,
            toolName: current?.toolName ?? 'tool',
            providerExecuted: chunk.providerExecuted ?? current?.providerExecuted,
            state: 'output-error',
            input: current && 'input' in current ? current.input : undefined,
            rawInput: current && 'rawInput' in current ? current.rawInput : undefined,
            errorText: chunk.errorText,
            callProviderMetadata: current?.callProviderMetadata,
            resultProviderMetadata: chunk.providerMetadata,
            approval: current?.approval?.approved === true ? current.approval : undefined,
          })
          break
        }
        case 'tool-output-denied': {
          const current = findToolPart(state.message.parts, chunk.toolCallId)
          upsertToolPart(state, {
            toolCallId: chunk.toolCallId,
            toolName: current?.toolName ?? 'tool',
            providerExecuted: current?.providerExecuted,
            state: 'output-denied',
            input: current && 'input' in current ? current.input : undefined,
            callProviderMetadata: current?.callProviderMetadata,
            approval: current?.approval?.approved === false
              ? current.approval
              : { id: chunk.toolCallId, approved: false },
          })
          break
        }
        case 'source-url':
        case 'source-document':
        case 'file':
        case 'reasoning-file':
          state.message.parts.push(chunk)
          break
        case 'start-step':
          state.message.parts.push({ type: 'step-start' })
          break
        case 'error':
          onError?.(new Error(chunk.errorText))
          break
        default:
          if (chunk.type.startsWith('data-')) {
            const dataChunk = chunk as { type: `data-${string}`; id?: string; data: unknown; transient?: boolean }
            if (dataChunk.transient) break
            state.message.parts.push({
              type: dataChunk.type,
              id: dataChunk.id,
              data: dataChunk.data,
            })
          }
      }

      onUpdate?.(state)
      controller.enqueue(chunk)
    },
  }))
}
