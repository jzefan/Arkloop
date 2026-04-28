import type { RunReasoningMode, UploadedThreadAttachment } from './api'

export type QueuedPrompt = {
  id: string
  text: string
  attachments: UploadedThreadAttachment[]
  personaKey?: string
  modelOverride?: string
  reasoningMode?: RunReasoningMode
  workDir?: string
  createdAt: number
}

export type CreateQueuedPromptInput = {
  text: string
  attachments?: UploadedThreadAttachment[]
  personaKey?: string
  modelOverride?: string
  reasoningMode?: RunReasoningMode
  workDir?: string
}

export function createQueuedPrompt(input: CreateQueuedPromptInput): QueuedPrompt {
  return {
    id: `queued_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 9)}`,
    text: input.text.trim(),
    attachments: input.attachments ?? [],
    personaKey: input.personaKey,
    modelOverride: input.modelOverride,
    reasoningMode: input.reasoningMode,
    workDir: input.workDir,
    createdAt: Date.now(),
  }
}
