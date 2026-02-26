import { apiFetch } from './client'

export type Org = {
  id: string
  slug: string
  name: string
  type: 'personal' | 'workspace'
  created_at: string
}

export type CreateWorkspaceRequest = {
  slug: string
  name: string
}

export async function listMyOrgs(accessToken: string): Promise<Org[]> {
  return apiFetch<Org[]>('/v1/orgs/me', { accessToken })
}

export async function createWorkspace(req: CreateWorkspaceRequest, accessToken: string): Promise<Org> {
  return apiFetch<Org>('/v1/orgs', {
    method: 'POST',
    body: JSON.stringify(req),
    accessToken,
  })
}
