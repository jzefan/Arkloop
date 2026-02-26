import { apiFetch } from './client'

export type Skill = {
  id: string
  org_id: string | null
  skill_key: string
  version: string
  display_name: string
  description?: string
  prompt_md: string
  tool_allowlist: string[]
  budgets: Record<string, unknown>
  is_active: boolean
  created_at: string
  preferred_credential?: string
  executor_type: string
  executor_config: Record<string, unknown>
}

export type CreateSkillRequest = {
  skill_key: string
  version: string
  display_name: string
  description?: string
  prompt_md: string
  tool_allowlist?: string[]
  budgets?: Record<string, unknown>
  is_active?: boolean
  preferred_credential?: string
  executor_type?: string
  executor_config?: Record<string, unknown>
}

export type PatchSkillRequest = {
  display_name?: string
  description?: string
  prompt_md?: string
  tool_allowlist?: string[]
  budgets?: Record<string, unknown>
  is_active?: boolean
  preferred_credential?: string
  executor_type?: string
  executor_config?: Record<string, unknown>
}

export async function listSkills(accessToken: string): Promise<Skill[]> {
  return apiFetch<Skill[]>('/v1/skills', { accessToken })
}

export async function createSkill(
  req: CreateSkillRequest,
  accessToken: string,
): Promise<Skill> {
  return apiFetch<Skill>('/v1/skills', {
    method: 'POST',
    body: JSON.stringify(req),
    accessToken,
  })
}

export async function patchSkill(
  id: string,
  req: PatchSkillRequest,
  accessToken: string,
): Promise<Skill> {
  return apiFetch<Skill>(`/v1/skills/${id}`, {
    method: 'PATCH',
    body: JSON.stringify(req),
    accessToken,
  })
}
