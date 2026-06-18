import client from '@/api/client'
import type {
  CreditCardReportFilter,
  CreditCardReportPreview,
  CreditCardReportRun,
  EmailPrintEvent,
} from '@/types'

export interface CreditCardReportRunsResponse {
  data: CreditCardReportRun[]
}

export interface CreditCardReportPrintResponse {
  data: EmailPrintEvent[]
  skipped: Array<{ group_id: string; reason: string }>
  summary: {
    event_count: number
    skipped_count: number
    group_count: number
  }
}

export async function previewCreditCardReport(filter: CreditCardReportFilter): Promise<CreditCardReportPreview> {
  const res = await client.get<CreditCardReportPreview>('/api/credit-card-reports/preview', {
    params: cleanCreditCardReportParams(filter),
  })
  return res.data
}

export async function listCreditCardReportRuns(): Promise<CreditCardReportRun[]> {
  const res = await client.get<CreditCardReportRunsResponse>('/api/credit-card-reports/runs')
  return res.data.data ?? []
}

export async function getCreditCardReportRun(id: string): Promise<CreditCardReportRun> {
  const res = await client.get<CreditCardReportRun>(`/api/credit-card-reports/runs/${id}`)
  return res.data
}

export async function createCreditCardReportRun(input: {
  report_name: string
  filters: CreditCardReportFilter
  selected_group_ids: string[]
}): Promise<CreditCardReportRun> {
  const res = await client.post<CreditCardReportRun>('/api/credit-card-reports/runs', input)
  return res.data
}

export async function exportCreditCardReportRun(id: string): Promise<{ blob: Blob; filename: string }> {
  const res = await client.get(`/api/credit-card-reports/runs/${id}/export.xlsx`, {
    responseType: 'blob',
  })
  const disposition = String(res.headers['content-disposition'] ?? '')
  const match = disposition.match(/filename="?([^";]+)"?/i)
  return {
    blob: res.data as Blob,
    filename: match?.[1] || `credit-card-report_${id}.xlsx`,
  }
}

export async function recordCreditCardReportPrintEvents(id: string): Promise<CreditCardReportPrintResponse> {
  const res = await client.post<CreditCardReportPrintResponse>(`/api/credit-card-reports/runs/${id}/print-events`)
  return res.data
}

function cleanCreditCardReportParams(filter: CreditCardReportFilter) {
  return {
    date_from: filter.date_from,
    date_to: filter.date_to,
    payment_method: filter.payment_method && filter.payment_method !== 'all' ? filter.payment_method : undefined,
    source: filter.source && filter.source !== 'all' ? filter.source : undefined,
    include_incomplete: filter.include_incomplete ? 'true' : undefined,
  }
}
