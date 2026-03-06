import { apiFetch } from './client'

export type DashboardData = {
  total_runs: number
  runs_today: number
  total_input_tokens: number
  total_output_tokens: number
}

export type DailyUsage = {
  date: string
  input_tokens: number
  output_tokens: number
}

export async function getDashboard(accessToken: string): Promise<DashboardData> {
  return apiFetch<DashboardData>('/v1/admin/dashboard', { accessToken })
}

export async function getDailyUsage(
  start: string,
  end: string,
  accessToken: string,
): Promise<DailyUsage[]> {
  return apiFetch<DailyUsage[]>(
    `/v1/admin/usage/daily?start=${start}&end=${end}`,
    { accessToken },
  )
}
