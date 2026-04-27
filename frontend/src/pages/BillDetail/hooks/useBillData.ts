import { useState, useEffect, useCallback } from 'react'
import { getBill, retryBill } from '@/hooks/useBills'
import type { Bill } from '@/types'

export interface UseBillDataReturn {
  bill: Bill | null
  loading: boolean
  retrying: boolean
  retryError: string | null
  handleRetry: () => Promise<void>
  setBill: React.Dispatch<React.SetStateAction<Bill | null>>
}

export function useBillData(id: string | undefined): UseBillDataReturn {
  const [bill, setBill] = useState<Bill | null>(null)
  const [loading, setLoading] = useState(true)
  const [retrying, setRetrying] = useState(false)
  const [retryError, setRetryError] = useState<string | null>(null)

  useEffect(() => {
    if (!id) return
    setLoading(true)
    getBill(id)
      .then(setBill)
      .catch(() => setBill(null))
      .finally(() => setLoading(false))
  }, [id])

  const handleRetry = useCallback(async () => {
    if (!id) return
    setRetrying(true)
    setRetryError(null)
    try {
      await retryBill(id)
      const updated = await getBill(id)
      setBill(updated)
    } catch {
      setRetryError('Retry ล้มเหลว — กรุณาลองใหม่อีกครั้ง')
    } finally {
      setRetrying(false)
    }
  }, [id])

  return { bill, loading, retrying, retryError, handleRetry, setBill }
}
