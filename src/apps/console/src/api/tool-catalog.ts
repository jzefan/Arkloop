import { apiFetch } from './client'

export type ToolCatalogItem = {
  name: string
  description: string
  has_override: boolean
}

export type ToolCatalogGroup = {
  group: string
  tools: ToolCatalogItem[]
}

function withScope(path: string, scope?: string): string {
  const s = (scope ?? '').trim()
  if (!s) return path
  const sep = path.includes('?') ? '&' : '?'
  return `${path}${sep}scope=${encodeURIComponent(s)}`
}

export async function updateToolDescription(
  toolName: string,
  description: string,
  accessToken: string,
  scope?: string,
): Promise<void> {
  await apiFetch<void>(withScope(`/v1/tool-catalog/${toolName}/description`, scope), {
    method: 'PUT',
    body: JSON.stringify({ description }),
    accessToken,
  })
}

export async function deleteToolDescription(
  toolName: string,
  accessToken: string,
  scope?: string,
): Promise<void> {
  await apiFetch<void>(withScope(`/v1/tool-catalog/${toolName}/description`, scope), {
    method: 'DELETE',
    accessToken,
  })
}
