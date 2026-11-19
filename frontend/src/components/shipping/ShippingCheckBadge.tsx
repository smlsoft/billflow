import { Check, Clock } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import {
  shippingLabels,
  shippingSummary,
  type ShippingStatus,
} from '@/lib/shipping-checks'
import type { Bill } from '@/types'

export function ShippingStatusBadge({
  status,
  attention,
}: {
  status?: ShippingStatus
  attention?: boolean
}) {
  if (attention) {
    return (
      <Badge variant="outline" className="border-warning/40 text-warning">
        {status === 'match'
          ? 'มีประวัติจัดส่งผิดปกติ'
          : status
            ? shippingLabels[status]
            : 'ยังไม่ได้ตรวจ'}
      </Badge>
    )
  }
  if (status === 'match') {
    return (
      <Badge
        variant="outline"
        className="gap-1 border-emerald-300 bg-emerald-100 text-emerald-900 dark:border-emerald-700 dark:bg-emerald-950 dark:text-emerald-200"
      >
        <Check aria-hidden="true" className="h-3.5 w-3.5 shrink-0" />
        {shippingLabels.match}
      </Badge>
    )
  }
  if (!status) {
    return (
      <Badge
        variant="outline"
        className="gap-1 border-slate-300 bg-slate-100 text-slate-800 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-100"
      >
        <Clock aria-hidden="true" className="h-3.5 w-3.5 shrink-0" />
        ยังไม่ได้ตรวจ
      </Badge>
    )
  }
  return <Badge variant="outline">{shippingLabels[status]}</Badge>
}

export function ShippingCheckBadge({ bill }: { bill: Bill }) {
  if (
    bill.bill_type !== 'purchase' ||
    !['shopee_shipped', 'lazada_email'].includes(bill.source)
  )
    return null
  const summary = shippingSummary(bill.raw_data)
  return (
    <ShippingStatusBadge status={summary?.status} attention={summary?.attention} />
  )
}
