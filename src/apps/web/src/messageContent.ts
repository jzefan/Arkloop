import type {
  UploadedThreadAttachment,
} from './api'
import type {
  AgentCreateMessageRequest,
  AgentMessage,
  AgentMessageContent,
  AgentMessageContentPart,
} from './agent-ui'

export function extractLegacyFilesFromContent(content: string): { text: string; fileNames: string[] } {
  const fileNames: string[] = []
  const text = content
    .replace(/<file name="([^"]+)" encoding="[^"]+">[\s\S]*?<\/file>/g, (_, name: string) => {
      fileNames.push(name)
      return ''
    })
    .trim()
  return { text, fileNames }
}

export function messageTextContent(message: Pick<AgentMessage, 'content' | 'contentJson'>): string {
  if (message.contentJson?.parts?.length) {
    return message.contentJson.parts
      .filter((part): part is Extract<AgentMessageContentPart, { type: 'text' }> => part.type === 'text')
      .map((part) => part.text)
      .join('\n\n')
      .trim()
  }
  return extractLegacyFilesFromContent(message.content).text
}

export function messageAttachmentParts(message: Pick<AgentMessage, 'content' | 'contentJson'>): AgentMessageContentPart[] {
  if (message.contentJson?.parts?.length) {
    return message.contentJson.parts.filter((part) => part.type === 'image' || part.type === 'file')
  }
  return []
}

export function buildMessageRequest(text: string, uploads: UploadedThreadAttachment[]): AgentCreateMessageRequest {
  const parts: AgentMessageContentPart[] = []
  if (text.trim()) {
    parts.push({ type: 'text', text: text.trim() })
  }
  for (const item of uploads) {
    const attachment = {
      key: item.key,
      filename: item.filename,
      mediaType: item.mime_type,
      size: item.size,
    }
    if (item.kind === 'image') {
      parts.push({ type: 'image', attachment })
      continue
    }
    parts.push({ type: 'file', attachment, extractedText: item.extracted_text ?? '' })
  }
  if (parts.length === 0) {
    return { content: text.trim() }
  }
  return {
    content: text.trim() || undefined,
    contentJson: { parts },
  }
}

export function hasMessageAttachments(message: Pick<AgentMessage, 'content' | 'contentJson'>): boolean {
  return messageAttachmentParts(message).length > 0 || extractLegacyFilesFromContent(message.content).fileNames.length > 0
}

export function isImagePart(part: AgentMessageContentPart): part is Extract<AgentMessageContentPart, { type: 'image' }> {
  return part.type === 'image'
}

export function isFilePart(part: AgentMessageContentPart): part is Extract<AgentMessageContentPart, { type: 'file' }> {
  return part.type === 'file'
}

export function ensureContent(value?: AgentMessageContent): AgentMessageContent | undefined {
  if (!value?.parts?.length) return undefined
  return value
}

const PASTED_FILENAME_RE = /^pasted-\d+\.txt$/

export function isPastedFile(filename: string): boolean {
  return PASTED_FILENAME_RE.test(filename)
}
