import { useState, useEffect, useCallback } from 'react'
import client from '../api/client'
import type { Bill, BillListResponse } from '../types'

interface BillsFilter {
  page?: number
  per_page?: number
  status?: string
  source?: string
  search?: string
  date_from?: string
  date_to?: string
}

export function useBills(filter: BillsFilter = {}) {
  const [data, setData] = useState<BillListResponse | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const fetch = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const params = new URLSearchParams()
      if (filter.page) params.set('page', String(filter.page))
      if (filter.per_page) params.set('per_page', String(filter.per_page))
      if (filter.status) params.set('status', filter.status)
      if (filter.source) params.set('source', filter.source)
      if (filter.search) params.set('search', filter.search)
      if (filter.date_from) params.set('date_from', filter.date_from)
      if (filter.date_to) params.set('date_to', filter.date_to)
      const res = await client.get<BillListResponse>(`/api/bills?${params}`)
      setData(res.data)
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : 'Failed to fetch bills')
    } finally {
      setLoading(false)
    }
  }, [JSON.stringify(filter)])

  useEffect(() => { fetch() }, [fetch])

  return { data, loading, error, refetch: fetch }
}

export async function getBill(id: string): Promise<Bill> {
  const res = await client.get<Bill>(`/api/bills/${id}`)
  return res.data
}

export async function retryBill(id: string): Promise<void> {
  await client.post(`/api/bills/${id}/retry`)
}
