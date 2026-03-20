/** 与 worker ACP Bridge 写入的 delegate_layer 一致 */
export const ACP_DELEGATE_LAYER = 'acp' as const

export function isACPDelegateEventData(data: unknown): boolean {
  if (!data || typeof data !== 'object' || Array.isArray(data)) return false
  return (data as Record<string, unknown>).delegate_layer === ACP_DELEGATE_LAYER
}
