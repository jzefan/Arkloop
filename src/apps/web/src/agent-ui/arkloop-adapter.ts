import {
  cancelRun,
  createMessage,
  createRun,
  editMessage,
  listMessages,
  listRunEvents,
  provideInput,
  retryMessage,
  type MessageContent,
  type MessageContentPart,
  type MessageResponse,
  type RunEvent,
} from '../api'
import { createSSEClient } from '../sse'
import type {
  AgentClient,
  AgentCreateMessageRequest,
  AgentMessage,
  AgentMessageAttachmentRef,
  AgentMessageContent,
  AgentMessageContentPart,
  AgentUIMessagePart,
  AgentRun,
  AgentUIEvent,
  AgentOpenMessageChunkStreamOptions,
  AgentUIMessageChunk,
} from './contract'

export type CreateArkloopAgentClientOptions = {
  accessToken: string
  baseUrl?: string
  refreshAccessToken?: () => Promise<string>
}

function buildStreamUrl(baseUrl: string | undefined, streamId: string): string {
  const normalizedBaseUrl = (baseUrl ?? '').replace(/\/$/, '')
  return `${normalizedBaseUrl}/v1/runs/${streamId}/events`
}

function toAgentRun(run: { run_id: string; trace_id: string }): AgentRun {
  return {
    id: run.run_id,
    traceId: run.trace_id,
  }
}

function toAgentAttachment(attachment: {
  key: string
  filename: string
  mime_type: string
  size: number
}): AgentMessageAttachmentRef {
  return {
    key: attachment.key,
    filename: attachment.filename,
    mediaType: attachment.mime_type,
    size: attachment.size,
  }
}

function toAgentContentPart(part: MessageContentPart): AgentMessageContentPart {
  if (part.type === 'text') return part
  if (part.type === 'image') {
    return { type: 'image', attachment: toAgentAttachment(part.attachment) }
  }
  return {
    type: 'file',
    attachment: toAgentAttachment(part.attachment),
    extractedText: part.extracted_text,
  }
}

function toArkloopAttachment(attachment: AgentMessageAttachmentRef): {
  key: string
  filename: string
  mime_type: string
  size: number
} {
  return {
    key: attachment.key,
    filename: attachment.filename,
    mime_type: attachment.mediaType,
    size: attachment.size,
  }
}

function toArkloopContentPart(part: AgentMessageContentPart): MessageContentPart {
  if (part.type === 'text') return part
  if (part.type === 'image') {
    return { type: 'image', attachment: toArkloopAttachment(part.attachment) }
  }
  return {
    type: 'file',
    attachment: toArkloopAttachment(part.attachment),
    extracted_text: part.extractedText,
  }
}

function toAgentContent(content: MessageContent | undefined): AgentMessageContent | undefined {
  if (!content?.parts?.length) return undefined
  return { parts: content.parts.map(toAgentContentPart) }
}

function toArkloopContent(content: AgentMessageContent | undefined): MessageContent | undefined {
  if (!content?.parts?.length) return undefined
  return { parts: content.parts.map(toArkloopContentPart) }
}

function toArkloopCreateMessageRequest(request: AgentCreateMessageRequest): {
  content?: string
  content_json?: MessageContent
} {
  return {
    content: request.content,
    content_json: toArkloopContent(request.contentJson),
  }
}

function toAgentMessage(message: MessageResponse): AgentMessage {
  const contentJson = toAgentContent(message.content_json)
  const parts: AgentUIMessagePart[] = contentJson?.parts.flatMap<AgentUIMessagePart>((part) => {
    if (part.type === 'text') return [{ type: 'text' as const, text: part.text, state: 'done' as const }]
    const mediaType = part.attachment.mediaType
    return [{
      type: 'file' as const,
      mediaType,
      filename: part.attachment.filename,
      url: `attachment:${part.attachment.key}`,
    }]
  }) ?? (message.content ? [{ type: 'text' as const, text: message.content, state: 'done' as const }] : [])

  return {
    id: message.id,
    role: message.role === 'system' || message.role === 'user' || message.role === 'assistant'
      ? message.role
      : 'assistant',
    content: message.content,
    contentJson,
    createdAt: message.created_at,
    streamId: message.run_id,
    metadata: {
      createdAt: message.created_at,
      streamId: message.run_id,
    },
    parts,
  }
}

function toAgentUIEventType(type: string): AgentUIEvent['type'] {
  switch (type) {
    case 'message.delta':
      return 'assistant-delta'
    case 'tool.call.delta':
      return 'tool-input-delta'
    case 'tool.call':
      return 'tool-call'
    case 'tool.result':
      return 'tool-result'
    case 'terminal.stdout_delta':
    case 'terminal.stderr_delta':
      return 'terminal-delta'
    case 'run.segment.start':
      return 'segment-start'
    case 'run.segment.end':
      return 'segment-end'
    case 'run.context_compact':
      return 'context-compact'
    case 'run.input_requested':
      return 'input-request'
    case 'run.completed':
      return 'run-completed'
    case 'run.failed':
      return 'run-failed'
    case 'run.cancelled':
      return 'run-cancelled'
    case 'run.interrupted':
      return 'run-interrupted'
    case 'security.injection.blocked':
      return 'security-block'
    case 'thread.title.updated':
      return 'thread-title'
    case 'thread.collaboration_mode.updated':
    case 'thread.collaboration.updated':
      return 'thread-collaboration'
    case 'todo.updated':
      return 'todo-updated'
    default:
      return type
  }
}

function toAgentUIEvent(event: RunEvent): AgentUIEvent {
  const terminalStream = event.type === 'terminal.stdout_delta'
    ? 'stdout'
    : event.type === 'terminal.stderr_delta' ? 'stderr' : undefined
  const data = terminalStream && event.data && typeof event.data === 'object'
    ? { ...(event.data as Record<string, unknown>), stream: terminalStream }
    : event.data

  return {
    id: event.event_id,
    streamId: event.run_id,
    order: event.seq,
    timestamp: event.ts,
    type: toAgentUIEventType(event.type),
    data,
    toolName: event.tool_name,
    errorCode: event.error_class,
  }
}

function readStringField(value: unknown, field: string): string | undefined {
  if (!value || typeof value !== 'object') return undefined
  const raw = (value as Record<string, unknown>)[field]
  return typeof raw === 'string' ? raw : undefined
}

function runEventToMessageChunks(event: RunEvent): AgentUIMessageChunk[] {
  const uiEvent = toAgentUIEvent(event)
  const chunks: AgentUIMessageChunk[] = [{
    type: 'data-agent-event',
    id: uiEvent.id,
    data: uiEvent,
    transient: true,
  }]

  if (event.type === 'message.delta') {
    const delta = readStringField(event.data, 'content_delta')
    const role = readStringField(event.data, 'role')
    const channel = readStringField(event.data, 'channel')
    if (!delta || (role && role !== 'assistant')) return []
    const id = channel === 'thinking' ? 'reasoning' : 'text'
    chunks.push(channel === 'thinking'
      ? { type: 'reasoning-delta', id, delta }
      : { type: 'text-delta', id, delta })
    return chunks
  }

  if (event.type === 'tool.call') {
    const toolCallId = readStringField(event.data, 'tool_call_id') ?? event.event_id
    const toolName = event.tool_name ?? readStringField(event.data, 'tool_name') ?? 'tool'
    const input = event.data && typeof event.data === 'object'
      ? (event.data as Record<string, unknown>).arguments
      : undefined
    chunks.push({ type: 'tool-input-available', toolCallId, toolName, input })
    return chunks
  }

  if (event.type === 'tool.result') {
    const toolCallId = readStringField(event.data, 'tool_call_id') ?? event.event_id
    const output = event.data && typeof event.data === 'object'
      ? (event.data as Record<string, unknown>).result
      : event.data
    if (event.error_class) {
      chunks.push({ type: 'tool-output-error', toolCallId, errorText: event.error_class })
      return chunks
    }
    chunks.push({ type: 'tool-output-available', toolCallId, output })
    return chunks
  }

  if (event.type === 'run.completed') {
    chunks.push({ type: 'finish', finishReason: 'stop' })
    return chunks
  }
  if (event.type === 'run.failed') {
    chunks.push({ type: 'finish', finishReason: 'error' })
    return chunks
  }
  if (event.type === 'run.cancelled' || event.type === 'run.interrupted') {
    chunks.push({ type: 'abort', reason: toAgentUIEventType(event.type) })
    return chunks
  }

  return chunks
}

function enqueueStreamStart(controller: ReadableStreamDefaultController<AgentUIMessageChunk>): void {
  controller.enqueue({ type: 'start' })
  controller.enqueue({ type: 'text-start', id: 'text' })
  controller.enqueue({ type: 'reasoning-start', id: 'reasoning' })
}

function enqueueStreamEnd(controller: ReadableStreamDefaultController<AgentUIMessageChunk>): void {
  controller.enqueue({ type: 'reasoning-end', id: 'reasoning' })
  controller.enqueue({ type: 'text-end', id: 'text' })
}

function runEventsToMessageChunkStream(eventsPromise: Promise<RunEvent[]>): ReadableStream<AgentUIMessageChunk> {
  return new ReadableStream<AgentUIMessageChunk>({
    async start(controller) {
      enqueueStreamStart(controller)
      try {
        const events = await eventsPromise
        for (const event of events) {
          for (const chunk of runEventToMessageChunks(event)) {
            controller.enqueue(chunk)
          }
        }
        enqueueStreamEnd(controller)
        controller.close()
      } catch (error) {
        controller.enqueue({
          type: 'error',
          errorText: error instanceof Error ? error.message : String(error),
        })
        controller.close()
      }
    },
  })
}

export function createArkloopAgentClient({
  accessToken,
  baseUrl,
  refreshAccessToken,
}: CreateArkloopAgentClientOptions): AgentClient {
  return {
    listMessages: async (threadId, limit) => (
      await (limit == null
        ? listMessages(accessToken, threadId)
        : listMessages(accessToken, threadId, limit))
    ).map(toAgentMessage),
    createMessage: async (input) => toAgentMessage(await createMessage(
      accessToken,
      input.threadId,
      toArkloopCreateMessageRequest(input.request),
    )),
    createRun: async (input) => toAgentRun(await createRun(
      accessToken,
      input.threadId,
      input.personaId,
      input.modelOverride,
      input.workDir,
      input.reasoningMode,
      input.options,
    )),
    editMessage: async (input) => toAgentRun(await editMessage(
      accessToken,
      input.threadId,
      input.messageId,
      input.content,
      toArkloopContent(input.contentJson),
      input.personaId,
      input.modelOverride,
      input.workDir,
      input.reasoningMode,
    )),
    retryMessage: async (input) => toAgentRun(await retryMessage(
      accessToken,
      input.threadId,
      input.messageId,
      input.personaId,
      input.modelOverride,
      input.workDir,
      input.reasoningMode,
    )),
    cancelRun: async (streamId, lastSeenSequence) => {
      await cancelRun(accessToken, streamId, lastSeenSequence)
    },
    provideInput: async (streamId, value) => {
      await provideInput(accessToken, streamId, value)
    },
    openMessageChunkStream: (
      streamId: string,
      options?: AgentOpenMessageChunkStreamOptions,
    ) => {
      if (options?.live === false) {
        return runEventsToMessageChunkStream(listRunEvents(accessToken, streamId, {
          afterSeq: options.cursor,
          follow: false,
        }))
      }

      let client: ReturnType<typeof createSSEClient> | null = null
      let closed = false
      const abort = () => {
        closed = true
        client?.close()
      }

      return new ReadableStream<AgentUIMessageChunk>({
        start(controller) {
          enqueueStreamStart(controller)
          const signal = options?.signal
          if (signal?.aborted) {
            abort()
            controller.close()
            return
          }
          signal?.addEventListener('abort', abort, { once: true })

          client = createSSEClient({
            url: buildStreamUrl(baseUrl, streamId),
            accessToken,
            afterSeq: options?.cursor,
            follow: options?.live ?? true,
            maxRetries: options?.maxRetries,
            retryDelayMs: options?.retryDelayMs,
            maxRetryDelayMs: options?.maxRetryDelayMs,
            readTimeoutMs: options?.readTimeoutMs,
            maxAuthRetries: options?.maxAuthRetries,
            onTokenRefresh: refreshAccessToken,
            onStateChange: options?.onStateChange,
            onError: (error) => {
              options?.onError?.(error)
              if (!closed) {
                controller.enqueue({
                  type: 'error',
                  errorText: error.message,
                })
              }
            },
            onEvent: (event) => {
              if (closed) return
              for (const chunk of runEventToMessageChunks(event)) {
                controller.enqueue(chunk)
              }
            },
          })

          void client.connect()
            .catch((error: unknown) => {
              if (closed) return
              const err = error instanceof Error ? error : new Error(String(error))
              options?.onError?.(err)
              controller.enqueue({ type: 'error', errorText: err.message })
            })
            .finally(() => {
              signal?.removeEventListener('abort', abort)
              if (closed) return
              closed = true
              enqueueStreamEnd(controller)
              controller.close()
            })
        },
        cancel() {
          abort()
        },
      })
    },
  }
}
