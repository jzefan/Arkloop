import { apiFetch } from './client'

export type LlmRoute = {
  id: string
  credential_id: string
  model: string
  priority: number
  is_default: boolean
  when: Record<string, unknown>
}

export type LlmCredential = {
  id: string
  org_id: string
  provider: string
  name: string
  key_prefix: string | null
  base_url: string | null
  openai_api_mode: string | null
  created_at: string
  routes: LlmRoute[]
}

export type CreateLlmRouteRequest = {
  model: string
  priority: number
  is_default: boolean
  when: Record<string, unknown>
}

export type CreateLlmCredentialRequest = {
  name: string
  provider: string
  api_key: string
  base_url?: string
  openai_api_mode?: string
  routes: CreateLlmRouteRequest[]
}

export async function listLlmCredentials(accessToken: string): Promise<LlmCredential[]> {
  return apiFetch<LlmCredential[]>('/v1/llm-credentials', { accessToken })
}

export async function createLlmCredential(
  req: CreateLlmCredentialRequest,
  accessToken: string,
): Promise<LlmCredential> {
  return apiFetch<LlmCredential>('/v1/llm-credentials', {
    method: 'POST',
    body: JSON.stringify(req),
    accessToken,
  })
}

export async function deleteLlmCredential(
  id: string,
  accessToken: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(`/v1/llm-credentials/${id}`, {
    method: 'DELETE',
    accessToken,
  })
}
