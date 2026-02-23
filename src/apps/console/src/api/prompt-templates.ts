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

export async function listPromptTemplates(accessToken: string): Promise<PromptTemplate[]> {
  return apiFetch<PromptTemplate[]>('/v1/prompt-templates', { accessToken })
}
