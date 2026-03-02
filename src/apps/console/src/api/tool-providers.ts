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
}

export type ToolProviderGroup = {
  group_name: string
  providers: ToolProviderItem[]
}

export type ToolProvidersResponse = {
  groups: ToolProviderGroup[]
}

export type UpdateToolProviderCredentialPayload = {
  api_key?: string
  base_url?: string
}

export async function listToolProviders(accessToken: string): Promise<ToolProvidersResponse> {
  return apiFetch<ToolProvidersResponse>('/v1/tool-providers', { accessToken })
}

export async function activateToolProvider(
  group: string,
  provider: string,
  accessToken: string,
): Promise<void> {
  await apiFetch<void>(`/v1/tool-providers/${group}/${provider}/activate`, {
    method: 'PUT',
    accessToken,
  })
}

export async function deactivateToolProvider(
  group: string,
  provider: string,
  accessToken: string,
): Promise<void> {
  await apiFetch<void>(`/v1/tool-providers/${group}/${provider}/deactivate`, {
    method: 'PUT',
    accessToken,
  })
}

export async function updateToolProviderCredential(
  group: string,
  provider: string,
  payload: UpdateToolProviderCredentialPayload,
  accessToken: string,
): Promise<void> {
  await apiFetch<void>(`/v1/tool-providers/${group}/${provider}/credential`, {
    method: 'PUT',
    body: JSON.stringify(payload),
    accessToken,
  })
}

export async function clearToolProviderCredential(
  group: string,
  provider: string,
  accessToken: string,
): Promise<void> {
  await apiFetch<void>(`/v1/tool-providers/${group}/${provider}/credential`, {
    method: 'DELETE',
    accessToken,
  })
}

