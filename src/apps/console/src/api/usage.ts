import { apiFetch } from './client'

export type UsageSummary = {
  org_id: string
  year: number
  month: number
  total_input_tokens: number
  total_output_tokens: number
  total_cost_usd: number
  record_count: number
}

export type DailyUsage = {
  date: string
  input_tokens: number
  output_tokens: number
  cost_usd: number
  record_count: number
}

export type ModelUsage = {
  model: string
  input_tokens: number
  output_tokens: number
  cost_usd: number
  record_count: number
}

export async function getOrgUsage(
  orgId: string,
  year: number,
  month: number,
  accessToken: string,
): Promise<UsageSummary> {
  return apiFetch<UsageSummary>(
    `/v1/orgs/${encodeURIComponent(orgId)}/usage?year=${year}&month=${month}`,
    { accessToken },
  )
}

export async function getOrgDailyUsage(
  orgId: string,
  start: string,
  end: string,
  accessToken: string,
): Promise<DailyUsage[]> {
  return apiFetch<DailyUsage[]>(
    `/v1/orgs/${encodeURIComponent(orgId)}/usage/daily?start=${start}&end=${end}`,
    { accessToken },
  )
}

export async function getOrgUsageByModel(
  orgId: string,
  year: number,
  month: number,
  accessToken: string,
): Promise<ModelUsage[]> {
  return apiFetch<ModelUsage[]>(
    `/v1/orgs/${encodeURIComponent(orgId)}/usage/by-model?year=${year}&month=${month}`,
    { accessToken },
  )
}

export async function getGlobalDailyUsage(
  start: string,
  end: string,
  accessToken: string,
): Promise<DailyUsage[]> {
  return apiFetch<DailyUsage[]>(
    `/v1/admin/usage/daily?start=${start}&end=${end}`,
    { accessToken },
  )
}

export async function getGlobalUsageSummary(
  year: number,
  month: number,
  accessToken: string,
): Promise<UsageSummary> {
  return apiFetch<UsageSummary>(
    `/v1/admin/usage/summary?year=${year}&month=${month}`,
    { accessToken },
  )
}

export async function getGlobalUsageByModel(
  year: number,
  month: number,
  accessToken: string,
): Promise<ModelUsage[]> {
  return apiFetch<ModelUsage[]>(
    `/v1/admin/usage/by-model?year=${year}&month=${month}`,
    { accessToken },
  )
}
