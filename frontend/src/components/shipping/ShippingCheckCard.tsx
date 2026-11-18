import { useEffect, useRef, useState } from 'react'
import { Loader2, RefreshCw } from 'lucide-react'
import { toast } from 'sonner'
import client from '@/api/client'
import { useAuth } from '@/hooks/useAuth'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { ShippingCheckResult } from './ShippingCheckResult'
import { ShippingCheckBadge } from './ShippingCheckBadge'
import { ShippingEvidenceButton } from './ShippingEvidenceButton'
import {
  shippingSummary,
  shippingError,
  newShippingCheckRequestID,
  type ShippingEntry,
  type ShippingSourceDetails,
} from '@/lib/shipping-checks'
import type { Bill } from '@/types'

export function ShippingCheckCard({
  bill,
  onChecked,
}: {
  bill: Bill
  onChecked?: () => void | Promise<unknown>
}) {
  const { user } = useAuth()
  const [entries, setEntries] = useState<ShippingEntry[]>([])
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading')
  const [source, setSource] = useState<ShippingSourceDetails | null>(null)
  const [sourceState, setSourceState] = useState<'loading' | 'ready' | 'error'>(
    'loading',
  )
  const [revision, setRevision] = useState(0)
  const [checking, setChecking] = useState(false)
  const [checkError, setCheckError] = useState('')
  const requestID = useRef<string | null>(null)
  const relevant =
    bill.bill_type === 'purchase' &&
    ['shopee_shipped', 'lazada_email'].includes(bill.source)
  const canCheck = user?.role === 'admin' || user?.role === 'staff'
  useEffect(() => {
    if (!relevant) return
    let alive = true
    setState('loading')
    setSourceState('loading')
    void Promise.allSettled([
      client.get<{ data: ShippingEntry[] }>(
        `/api/bills/${bill.id}/shipping-checks`,
      ),
      client.get<ShippingSourceDetails>(
        `/api/bills/${bill.id}/shipping-details`,
      ),
    ]).then(([history, evidence]) => {
      if (!alive) return
      if (history.status === 'fulfilled') {
        setEntries(history.value.data.data)
        setState('ready')
      } else setState('error')
      if (evidence.status === 'fulfilled') {
        setSource(evidence.value.data)
        setSourceState('ready')
      } else {
        setSource(null)
        setSourceState('error')
      }
    })
    return () => {
      alive = false
    }
  }, [bill.id, relevant, revision, bill.raw_data?.shipping_check_summary])
  const check = async () => {
    setChecking(true)
    setCheckError('')
    try {
      requestID.current ||= newShippingCheckRequestID()
      await client.post(`/api/bills/${bill.id}/shipping-checks`, {
        request_id: requestID.current,
      })
      requestID.current = null
      setRevision((v) => v + 1)
      toast.success('บันทึกผลตรวจจัดส่งแล้ว')
      // A result is already committed even if the parent refresh fails. Keep the
      // history refresh independent so a temporary list refresh error is honest.
      try {
        await onChecked?.()
      } catch {
        toast.error('บันทึกผลแล้ว แต่โหลดข้อมูลบิลใหม่ไม่สำเร็จ กรุณารีเฟรช')
      }
    } catch (e) {
      setCheckError(shippingError(e))
    } finally {
      setChecking(false)
    }
  }
  if (!relevant) return null
  const summary = shippingSummary(bill.raw_data)
  return (
    <Card className="space-y-4 p-4 sm:p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="font-semibold">ตรวจสอบข้อมูลจัดส่ง</h2>
          <ShippingCheckBadge bill={bill} />
        </div>
        {canCheck && (
          <Button
            variant="outline"
            size="sm"
            disabled={
              checking ||
              sourceState !== 'ready' ||
              !source?.manual_check_available
            }
            onClick={() => void check()}
          >
            {checking ? (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            ) : (
              <RefreshCw className="mr-2 h-4 w-4" />
            )}
            {checking
              ? 'กำลังตรวจ…'
              : entries.length
                ? 'ตรวจจัดส่งอีกครั้ง'
                : 'ตรวจข้อมูลจัดส่ง'}
          </Button>
        )}
      </div>
      <p className="text-xs text-muted-foreground">
        ปุ่มตรวจใช้กติกาปัจจุบันของกล่องอีเมลและบันทึกผลรายบิล
        ผลตรวจนี้ไม่บล็อกการส่ง SML
      </p>
      {sourceState === 'loading' && (
        <p role="status" className="text-sm">
          กำลังอ่านข้อมูลจัดส่งจากหลักฐานอีเมล…
        </p>
      )}
      {sourceState === 'ready' && source && (
        <div className="space-y-2 rounded-md border bg-muted/20 p-3 text-sm">
          <h3 className="font-medium">ข้อมูลจัดส่งจากอีเมลคำสั่งซื้อเดิม</h3>
          <p className="break-words">
            <strong>ชื่อผู้รับ:</strong> {source.recipient || 'ไม่พบข้อมูล'}
          </p>
          <p className="break-words">
            <strong>ที่อยู่จัดส่ง:</strong> {source.address || 'ไม่พบข้อมูล'}
          </p>
          {source.reason && (
            <p className="text-muted-foreground">{source.reason}</p>
          )}
          {source.artifact_id && source.artifact_bill_id && (
            <ShippingEvidenceButton
              artifactID={source.artifact_id}
              billID={source.artifact_bill_id}
            />
          )}
          {canCheck && source.manual_check_reason && (
            <p className="text-warning">{source.manual_check_reason}</p>
          )}
        </div>
      )}
      {(sourceState === 'error' || state === 'error') && (
        <div role="alert" className="text-sm">
          {sourceState === 'error'
            ? 'อ่านข้อมูลจัดส่งไม่สำเร็จ'
            : 'โหลดประวัติไม่สำเร็จ'}{' '}
          <Button
            variant="outline"
            size="sm"
            onClick={() => setRevision((v) => v + 1)}
          >
            ลองใหม่
          </Button>
        </div>
      )}
      {checkError && (
        <p role="alert" className="text-sm text-destructive">
          {checkError}
        </p>
      )}
      {summary?.attention && (
        <p className="text-sm text-warning" role="status">
          มีข้อมูลจัดส่งที่ต้องตรวจสอบ โปรดดูรายละเอียดและประวัติด้านล่าง
          {summary.reason ? `: ${summary.reason}` : ''}
        </p>
      )}
      {state === 'loading' && (
        <p role="status" className="text-sm">
          กำลังโหลดผลตรวจ…
        </p>
      )}
      {state === 'ready' && entries.length === 0 && (
        <p className="text-sm text-muted-foreground">
          {summary?.status
            ? summary.reason || 'ยังไม่มีผลตรวจที่บันทึกสำเร็จ'
            : 'ยังไม่มีผลตรวจ บิลเก่าสามารถกดตรวจเป็นรายบิลได้ โดยไม่สร้างบิลหรือส่ง SML ซ้ำ'}
        </p>
      )}
      {state === 'ready' &&
        entries.map((entry, i) => (
          <details
            key={entry.id}
            open={i === 0}
            className="rounded-md border p-3"
          >
            <summary className="cursor-pointer text-sm">
              {i === 0 ? 'ผลล่าสุด' : 'ประวัติ'} ·{' '}
              {entry.result.trigger === 'manual'
                ? 'กดตรวจด้วยกติกาขณะนั้น'
                : 'ตรวจอัตโนมัติด้วยกติกาที่เก็บกับบิล'}{' '}
              · {new Date(entry.created_at).toLocaleString('th-TH')} ·{' '}
              {entry.policy.profile.name}
            </summary>
            <div className="mt-3">
              <ShippingCheckResult
                result={entry.result}
                artifactID={entry.artifact_id}
                artifactBillID={entry.artifact_bill_id}
              />
              <p className="mt-2 break-all text-xs text-muted-foreground">
                กติกา: {entry.result.version} / {entry.policy.revision} · อีเมล:{' '}
                {entry.message_id || 'ไม่มี Message-ID'}
                {entry.result.checked_by
                  ? ` · ผู้ตรวจ: ${entry.result.checked_by}`
                  : ''}
              </p>
            </div>
          </details>
        ))}
    </Card>
  )
}
