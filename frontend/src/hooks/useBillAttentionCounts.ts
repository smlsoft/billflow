import { useCallback, useEffect, useState } from 'react'
import client from '@/api/client'
import { WORK_QUEUE_CHANGED_EVENT } from '@/lib/work-queue-events'

export interface BillAttentionCounts {
  shipping: number
  email: number
}

export function useBillAttentionCounts(enabled: boolean) {
  const [counts, setCounts] = useState<BillAttentionCounts | null>(null)
  const [error, setError] = useState(false)
  const [revision, setRevision] = useState(0)
  const refresh = useCallback(() => setRevision((v) => v + 1), [])
  useEffect(() => {
    if (!enabled) return
    let alive = true
    let inFlight = false
    const abort = new AbortController()
    const load = async () => {
      if (inFlight || document.visibilityState === 'hidden') return
      inFlight = true
      try {
        const { data } = await client.get<BillAttentionCounts>(
          '/api/bills/attention-counts',
          { signal: abort.signal },
        )
        if (
          ![data.shipping, data.email].every(
            (v) => Number.isSafeInteger(v) && v >= 0,
          )
        )
          throw new Error('invalid counts')
        if (alive) {
          setCounts(data)
          setError(false)
        }
      } catch {
        if (alive) {
          setCounts(null)
          setError(true)
        }
      } finally {
        inFlight = false
      }
    }
    void load()
    const reload = () => {
      void load()
    }
    const timer = window.setInterval(reload, 30_000)
    window.addEventListener('focus', reload)
    window.addEventListener(WORK_QUEUE_CHANGED_EVENT, reload)
    document.addEventListener('visibilitychange', reload)
    return () => {
      alive = false
      abort.abort()
      window.clearInterval(timer)
      window.removeEventListener('focus', reload)
      window.removeEventListener(WORK_QUEUE_CHANGED_EVENT, reload)
      document.removeEventListener('visibilitychange', reload)
    }
  }, [enabled, revision])
  return { counts, error, refresh }
}
