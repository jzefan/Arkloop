import { apiFetch } from './client'

export type LlmProviderModel = {
  id: string
  provider_id: string
  model: string
  priority: number
  is_default: boolean
  tags: string[]
  when: Record<string, unknown>
  multiplier: number
  cost_per_1k_input?: number | null
  cost_per_1k_output?: number | null
  cost_per_1k_cache_write?: number | null
  cost_per_1k_cache_read?: number | null
}

export type LlmProvider = {
  id: string
  org_id: string
  provider: string
  name: string
  key_prefix: string | null
  base_url: string | null
  openai_api_mode: string | null
  advanced_json?: Record<string, unknown> | null
  created_at: string
  models: LlmProviderModel[]
}

export type CreateLlmProviderRequest = {
  name: string
  provider: string
  api_key: string
  base_url?: string
  openai_api_mode?: string
  advanced_json?: Record<string, unknown>
}

export type UpdateLlmProviderRequest = {
  name?: string
  provider?: string
  api_key?: string
  base_url?: string | null
  openai_api_mode?: string | null
  advanced_json?: Record<string, unknown> | null
}

export type CreateModelRequest = {
  model: string
  priority?: number
  is_default?: boolean
  tags?: string[]
  when?: Record<string, unknown>
  multiplier?: number
}

export type UpdateModelRequest = {
  model?: string
  priority?: number
  is_default?: boolean
  tags?: string[]
  when?: Record<string, unknown>
  multiplier?: number
}

export type AvailableModel = {
  id: string
  name: string
  configured: boolean
}

export async function listLlmProviders(accessToken: string): Promise<LlmProvider[]> {
  return apiFetch<LlmProvider[]>('/v1/llm-providers', { accessToken })
}

export async function createLlmProvider(
  req: CreateLlmProviderRequest,
  accessToken: string,
): Promise<LlmProvider> {
  return apiFetch<LlmProvider>('/v1/llm-providers', {
    method: 'POST',
    body: JSON.stringify(req),
    accessToken,
  })
}

export async function updateLlmProvider(
  id: string,
  req: UpdateLlmProviderRequest,
  accessToken: string,
): Promise<LlmProvider> {
  return apiFetch<LlmProvider>(`/v1/llm-providers/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(req),
    accessToken,
  })
}

export async function deleteLlmProvider(
  id: string,
  accessToken: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(`/v1/llm-providers/${id}`, {
    method: 'DELETE',
    accessToken,
  })
}

export async function createProviderModel(
  providerId: string,
  req: CreateModelRequest,
  accessToken: string,
): Promise<LlmProviderModel> {
  return apiFetch<LlmProviderModel>(`/v1/llm-providers/${providerId}/models`, {
    method: 'POST',
    body: JSON.stringify(req),
    accessToken,
  })
}

export async function updateProviderModel(
  providerId: string,
  modelId: string,
  req: UpdateModelRequest,
  accessToken: string,
): Promise<LlmProviderModel> {
  return apiFetch<LlmProviderModel>(`/v1/llm-providers/${providerId}/models/${modelId}`, {
    method: 'PATCH',
    body: JSON.stringify(req),
    accessToken,
  })
}

export async function deleteProviderModel(
  providerId: string,
  modelId: string,
  accessToken: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(`/v1/llm-providers/${providerId}/models/${modelId}`, {
    method: 'DELETE',
    accessToken,
  })
}

export async function listAvailableModels(
  providerId: string,
  accessToken: string,
): Promise<{ models: AvailableModel[] }> {
  return apiFetch<{ models: AvailableModel[] }>(`/v1/llm-providers/${providerId}/available-models`, {
    accessToken,
  })
}
