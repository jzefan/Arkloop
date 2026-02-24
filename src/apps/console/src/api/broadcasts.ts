import { apiFetch } from './client'

export type Broadcast = {
  id: string
  type: string
  title: string
  body: string
  target_type: string
  target_id?: string
  payload: Record<string, unknown>
  status: string
  sent_count: number
  created_by: string
  created_at: string
}

export type ListBroadcastsParams = {
  limit?: number
  before_created_at?: string
  before_id?: string
}

export async function listBroadcasts(
  params: ListBroadcastsParams,
  accessToken: string,
): Promise<Broadcast[]> {
  const sp = new URLSearchParams()
  if (params.limit) sp.set('limit', String(params.limit))
  if (params.before_created_at) sp.set('before_created_at', params.before_created_at)
  if (params.before_id) sp.set('before_id', params.before_id)
  const qs = sp.toString()
  return apiFetch<Broadcast[]>(`/v1/admin/notifications/broadcasts${qs ? `?${qs}` : ''}`, { accessToken })
}

export type CreateBroadcastRequest = {
  type: string
  title: string
  body: string
  target: string
  payload?: Record<string, unknown>
}

export async function createBroadcast(
  body: CreateBroadcastRequest,
  accessToken: string,
): Promise<Broadcast> {
  return apiFetch<Broadcast>('/v1/admin/notifications/broadcasts', {
    method: 'POST',
    body: JSON.stringify(body),
    accessToken,
  })
}

export async function getBroadcast(
  id: string,
  accessToken: string,
): Promise<Broadcast> {
  return apiFetch<Broadcast>(`/v1/admin/notifications/broadcasts/${id}`, { accessToken })
}
