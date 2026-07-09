import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export interface DailyCashbackRule {
  id: number
  name: string
  enabled: boolean
  min_amount: number
  max_amount?: number | null
  rate_percent: number
  sort_order: number
  created_at: string
  updated_at: string
}

export interface DailyCashbackRulePayload {
  name: string
  enabled: boolean
  min_amount: number
  max_amount?: number | null
  rate_percent: number
  sort_order: number
}

export interface DailyCashbackRecord {
  id: number
  user_id: number
  user_email: string
  username: string
  rule_id?: number | null
  rule_name?: string
  business_date: string
  spend_amount: number
  rate_percent: number
  cashback_amount: number
  balance_after?: number | null
  status: string
  applied_at: string
}

export interface DailyCashbackRunResult {
  business_date: string
  matched_users: number
  applied_users: number
  skipped_users: number
  total_spend: number
  total_cashback: number
}

export interface ListRecordsParams {
  page?: number
  page_size?: number
  search?: string
  business_date?: string
}

export async function listRules(): Promise<DailyCashbackRule[]> {
  const { data } = await apiClient.get<DailyCashbackRule[]>('/admin/daily-cashback/rules')
  return data
}

export async function createRule(payload: DailyCashbackRulePayload): Promise<DailyCashbackRule> {
  const { data } = await apiClient.post<DailyCashbackRule>('/admin/daily-cashback/rules', payload)
  return data
}

export async function updateRule(id: number, payload: DailyCashbackRulePayload): Promise<DailyCashbackRule> {
  const { data } = await apiClient.put<DailyCashbackRule>(`/admin/daily-cashback/rules/${id}`, payload)
  return data
}

export async function deleteRule(id: number): Promise<{ id: number }> {
  const { data } = await apiClient.delete<{ id: number }>(`/admin/daily-cashback/rules/${id}`)
  return data
}

export async function listRecords(params: ListRecordsParams = {}): Promise<PaginatedResponse<DailyCashbackRecord>> {
  const { data } = await apiClient.get<PaginatedResponse<DailyCashbackRecord>>('/admin/daily-cashback/records', {
    params: {
      page: params.page ?? 1,
      page_size: params.page_size ?? 20,
      search: params.search || undefined,
      business_date: params.business_date || undefined,
    },
  })
  return data
}

export async function runForDate(businessDate: string): Promise<DailyCashbackRunResult> {
  const { data } = await apiClient.post<DailyCashbackRunResult>('/admin/daily-cashback/run', {
    business_date: businessDate,
  })
  return data
}

export const dailyCashbackAPI = {
  listRules,
  createRule,
  updateRule,
  deleteRule,
  listRecords,
  runForDate,
}

export default dailyCashbackAPI
