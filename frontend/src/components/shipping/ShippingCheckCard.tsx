import { useEffect, useState } from 'react'
import client from '@/api/client'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { ShippingCheckResult } from './ShippingCheckResult'
import { ShippingCheckBadge } from './ShippingCheckBadge'
import { shippingSummary, type ShippingEntry } from '@/lib/shipping-checks'
import type { Bill } from '@/types'
export function ShippingCheckCard({ bill }: { bill: Bill }) {
  const [entries, setEntries] = useState<ShippingEntry[]>([])
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading')
  const [revision, setRevision] = useState(0)
  const relevant =
    bill.bill_type === 'purchase' &&
    ['shopee_shipped', 'lazada_email'].includes(bill.source)
  useEffect(() => {
    if (!relevant) return
    let alive = true
    setState('loading')
    client
      .get<{ data: ShippingEntry[] }>(`/api/bills/${bill.id}/shipping-checks`)
      .then((r) => {
        if (alive) {
          setEntries(r.data.data)
          setState('ready')
        }
      })
      .catch(() => {
        if (alive) setState('error')
      })
    return () => {
      alive = false
    }
  }, [bill.id, relevant, revision, bill.raw_data?.shipping_check_summary])
  if (!relevant) return null
  const summary = shippingSummary(bill.raw_data)
  return (
    <Card className="space-y-3 p-4 sm:p-5">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="font-semibold">ตรวจสอบข้อมูลจัดส่ง</h2>
        <ShippingCheckBadge bill={bill} />
      </div>
      <p className="text-xs text-muted-foreground">
        ตรวจจากอีเมลตามกติกาที่บันทึกไว้เมื่อสร้างบิล ผลตรวจนี้ไม่บล็อกการส่ง
        SML
      </p>
      {state === 'loading' && <p role="status">กำลังโหลดผลตรวจ…</p>}
      {state === 'error' && (
        <div role="alert">
          โหลดประวัติไม่สำเร็จ{' '}
          <Button
            variant="outline"
            size="sm"
            onClick={() => setRevision((v) => v + 1)}
          >
            ลองใหม่
          </Button>
        </div>
      )}
      {summary?.attention && (
        <p className="text-sm text-warning" role="status">
          มีข้อมูลจัดส่งที่ต้องตรวจสอบ โปรดดูรายละเอียดและประวัติด้านล่าง
          {summary.reason ? `: ${summary.reason}` : ''}
        </p>
      )}
      {state === 'ready' && entries.length === 0 && (
        <p className="text-sm text-muted-foreground">
          {summary?.status
            ? summary.reason || 'ยังไม่มีผลตรวจที่บันทึกสำเร็จ'
            : 'บิลนี้ยังไม่ได้ตรวจ ระบบตรวจอัตโนมัติเฉพาะบิลใหม่ที่เปิดใช้งานแล้ว'}
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
              {new Date(entry.created_at).toLocaleString('th-TH')} ·{' '}
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
              </p>
            </div>
          </details>
        ))}
    </Card>
  )
}
