import { apiFetch } from './client'

export type Report = {
  id: string
  thread_id: string
  reporter_id: string
  reporter_email: string
  categories: string[]
  feedback: string | null
  created_at: string
}

export type ListReportsResponse = {
  data: Report[]
  total: number
}

export type ListReportsParams = {
  limit?: number
  offset?: number
}

export async function listReports(
  params: ListReportsParams,
  accessToken: string,
): Promise<ListReportsResponse> {
  const qs = new URLSearchParams()
  if (params.limit != null) qs.set('limit', String(params.limit))
  if (params.offset != null) qs.set('offset', String(params.offset))
  const query = qs.toString()
  return apiFetch<ListReportsResponse>(
    `/v1/admin/reports${query ? `?${query}` : ''}`,
    { accessToken },
  )
}
