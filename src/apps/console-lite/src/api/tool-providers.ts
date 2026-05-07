import { apiFetch } from './client'

export type ToolProviderItem = {
  group_name: string
  provider_name: string
  is_active: boolean
  key_prefix?: string
  base_url?: string
  requires_api_key: boolean
  requires_base_url: boolean
  configured: boolean
  config_json?: Record<string, unknown>
}

export type ToolProviderGroup = {
  group_name: string
  providers: ToolProviderItem[]
}

export type ToolDescriptionSource = 'default' | 'platform' | 'project'

export type ToolCatalogItem = {
  name: string
  label: string
  llm_description: string
  has_override: boolean
  description_source: ToolDescriptionSource
  is_disabled: boolean
}

export type ToolCatalogGroup = {
  group: string
  tools: ToolCatalogItem[]
}

const SCOPE = 'platform'

function scopedPath(path: string): string {
  const sep = path.includes('?') ? '&' : '?'
  return `${path}${sep}scope=${SCOPE}`
}

export async function loadToolProvidersAndCatalog(
  accessToken: string,
): Promise<{ providerGroups: ToolProviderGroup[]; catalogGroups: ToolCatalogGroup[] }> {
  const [providers, catalog] = await Promise.all([
    apiFetch<{ groups: ToolProviderGroup[] }>(scopedPath('/v1/tool-providers'), { accessToken }),
    apiFetch<{ groups: ToolCatalogGroup[] }>(scopedPath('/v1/tool-catalog'), { accessToken }),
  ])
  return { providerGroups: providers.groups, catalogGroups: catalog.groups }
}

export async function activateToolProvider(
  group: string, provider: string, accessToken: string,
): Promise<void> {
  await apiFetch<void>(scopedPath(`/v1/tool-providers/${group}/${provider}/activate`), {
    method: 'PUT', accessToken,
  })
}

export async function deactivateToolProvider(
  group: string, provider: string, accessToken: string,
): Promise<void> {
  await apiFetch<void>(scopedPath(`/v1/tool-providers/${group}/${provider}/deactivate`), {
    method: 'PUT', accessToken,
  })
}

export async function updateToolProviderCredential(
  group: string, provider: string, payload: Record<string, string>, accessToken: string,
): Promise<void> {
  await apiFetch<void>(scopedPath(`/v1/tool-providers/${group}/${provider}/credential`), {
    method: 'PUT', body: JSON.stringify(payload), accessToken,
  })
}

export async function clearToolProviderCredential(
  group: string, provider: string, accessToken: string,
): Promise<void> {
  await apiFetch<void>(scopedPath(`/v1/tool-providers/${group}/${provider}/credential`), {
    method: 'DELETE', accessToken,
  })
}

export async function updateToolProviderConfig(
  group: string, provider: string, configJSON: Record<string, unknown>, accessToken: string,
): Promise<void> {
  await apiFetch<void>(scopedPath(`/v1/tool-providers/${group}/${provider}/config`), {
    method: 'PUT', body: JSON.stringify(configJSON), accessToken,
  })
}

export async function updateToolDescription(
  toolName: string, description: string, accessToken: string,
): Promise<void> {
  await apiFetch<void>(scopedPath(`/v1/tool-catalog/${toolName}/description`), {
    method: 'PUT', body: JSON.stringify({ description }), accessToken,
  })
}

export async function deleteToolDescription(
  toolName: string, accessToken: string,
): Promise<void> {
  await apiFetch<void>(scopedPath(`/v1/tool-catalog/${toolName}/description`), {
    method: 'DELETE', accessToken,
  })
}

export async function updateToolDisabled(
  toolName: string, disabled: boolean, accessToken: string,
): Promise<void> {
  await apiFetch<void>(scopedPath(`/v1/tool-catalog/${toolName}/disabled`), {
    method: 'PUT', body: JSON.stringify({ disabled }), accessToken,
  })
}
