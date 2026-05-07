import type { Attachment } from '../components/ChatInput'

export const MAX_IMAGE_ATTACHMENTS = 5

export function filterAttachableFilesForImageLimit(current: Attachment[], files: File[]): File[] {
  let imageSlots = Math.max(0, MAX_IMAGE_ATTACHMENTS - current.filter((item) => item.mime_type.startsWith('image/')).length)
  const out: File[] = []
  for (const file of files) {
    const isImage = file.type.startsWith('image/')
    if (isImage) {
      if (imageSlots <= 0) continue
      imageSlots -= 1
    }
    out.push(file)
  }
  return out
}
