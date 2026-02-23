import { apiFetch } from './client'

export type Notification = {
  id: string
  user_id: string
  org_id: string
  type: string
  title: string
  body: string
  payload: Record<string, unknown>
  read_at?: string
  created_at: string
}

export type ListNotificationsResponse = {
  data: Notification[]
}

export async function listNotifications(
  accessToken: string,
): Promise<ListNotificationsResponse> {
  return apiFetch<ListNotificationsResponse>('/v1/notifications', { accessToken })
}

export async function markNotificationRead(
  id: string,
  accessToken: string,
): Promise<{ ok: boolean }> {
  return apiFetch<{ ok: boolean }>(`/v1/notifications/${id}`, {
    method: 'PATCH',
    accessToken,
  })
}
