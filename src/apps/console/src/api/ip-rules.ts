import { apiFetch } from './client'

export type IPRule = {
  id: string
  project_id: string // TODO(migrate): backend still sends org_id
  type: 'allowlist' | 'blocklist'
  cidr: string
  note?: string
  created_at: string
}

export type CreateIPRuleRequest = {
  type: 'allowlist' | 'blocklist'
  cidr: string
  note?: string
}

export async function listIPRules(accessToken: string): Promise<IPRule[]> {
  return apiFetch<IPRule[]>('/v1/ip-rules', { accessToken })
}

export async function createIPRule(
  req: CreateIPRuleRequest,
  accessToken: string,
): Promise<IPRule> {
  return apiFetch<IPRule>('/v1/ip-rules', {
    method: 'POST',
    body: JSON.stringify(req),
    accessToken,
  })
}

export async function deleteIPRule(id: string, accessToken: string): Promise<void> {
  await apiFetch<void>(`/v1/ip-rules/${id}`, {
    method: 'DELETE',
    accessToken,
  })
}
