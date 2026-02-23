import { apiFetch } from './client'

export type PromptTemplate = {
  id: string
  org_id: string
  name: string
  content: string
  variables: string[]
  is_default: boolean
  version: number
  published_at?: string
  created_at: string
}

export type CreatePromptTemplateRequest = {
  name: string
  content: string
  variables: string[]
  is_default: boolean
}

export type UpdatePromptTemplateRequest = {
  name?: string
  content?: string
  is_default?: boolean
}

export async function listPromptTemplates(accessToken: string): Promise<PromptTemplate[]> {
  return apiFetch<PromptTemplate[]>('/v1/prompt-templates', { accessToken })
}

export async function createPromptTemplate(
  req: CreatePromptTemplateRequest,
  accessToken: string,
): Promise<PromptTemplate> {
  return apiFetch<PromptTemplate>('/v1/prompt-templates', {
    method: 'POST',
    body: JSON.stringify(req),
    accessToken,
  })
}

export async function updatePromptTemplate(
  id: string,
  req: UpdatePromptTemplateRequest,
  accessToken: string,
): Promise<PromptTemplate> {
  return apiFetch<PromptTemplate>(`/v1/prompt-templates/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(req),
    accessToken,
  })
}

export async function deletePromptTemplate(
  id: string,
  accessToken: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(`/v1/prompt-templates/${id}`, {
    method: 'DELETE',
    accessToken,
  })
}
