import { useState, useEffect, useCallback } from 'react'
import { toast } from 'sonner'
import { getBill, retryBill } from '@/hooks/useBills'
import type { RetryBillPayload } from '@/hooks/useBills'
import type { Bill } from '@/types'

export interface UseBillDataReturn {
  bill: Bill | null
  loading: boolean
  retrying: boolean
  retryError: string | null
  reloadBill: () => Promise<Bill | null>
  handleRetry: () => Promise<void>
  handleRetryWithOverride: (body: RetryBillPayload) => Promise<void>
  setBill: React.Dispatch<React.SetStateAction<Bill | null>>
}

export function useBillData(id: string | undefined): UseBillDataReturn {
  const [bill, setBill] = useState<Bill | null>(null)
  const [loading, setLoading] = useState(true)
  const [retrying, setRetrying] = useState(false)
  const [retryError, setRetryError] = useState<string | null>(null)

  const reloadBill = useCallback(async () => {
    if (!id) return null
    const updated = await getBill(id)
    setBill(updated)
    return updated
  }, [id])

  useEffect(() => {
    if (!id) return
    setLoading(true)
    reloadBill()
      .catch(() => setBill(null))
      .finally(() => setLoading(false))
  }, [id, reloadBill])

  const doRetry = useCallback(
    async (body?: RetryBillPayload) => {
      if (!id) return
      setRetrying(true)
      setRetryError(null)
      try {
        await retryBill(id, body)
        const updated = await reloadBill()
        toast.success('ส่ง SML สำเร็จ', {
          description: updated?.sml_doc_no ? `Doc: ${updated.sml_doc_no}` : undefined,
        })
      } catch (err) {
        try {
          await reloadBill()
        } catch {
          // Keep the existing bill in view if the follow-up refresh also fails.
        }
        const message =
          err instanceof Error && err.message
            ? err.message
            : 'Retry ล้มเหลว — กรุณาลองใหม่อีกครั้ง'
        setRetryError(message)
        toast.error('ส่ง SML ไม่สำเร็จ', {
          description: 'ดูรายละเอียดในการ์ด Error ด้านบน',
        })
      } finally {
        setRetrying(false)
      }
    },
    [id, reloadBill],
  )

  const handleRetry = useCallback(() => doRetry(), [doRetry])

  const handleRetryWithOverride = useCallback(
    (body: RetryBillPayload) => doRetry(body),
    [doRetry],
  )

  return { bill, loading, retrying, retryError, reloadBill, handleRetry, handleRetryWithOverride, setBill }
}
