import { Badge } from '@/components/ui/badge'
import { shippingLabels, shippingSummary } from '@/lib/shipping-checks'
import type { Bill } from '@/types'
export function ShippingCheckBadge({ bill }: { bill: Bill }) {
  if (
    bill.bill_type !== 'purchase' ||
    !['shopee_shipped', 'lazada_email'].includes(bill.source)
  )
    return null
  const summary = shippingSummary(bill.raw_data)
  return (
    <Badge
      variant="outline"
      className={
        summary?.attention
          ? 'border-warning/40 text-warning'
          : 'text-muted-foreground'
      }
    >
      {summary?.attention && summary.status === 'match'
        ? 'มีประวัติจัดส่งผิดปกติ'
        : summary?.status
          ? shippingLabels[summary.status]
          : 'ยังไม่ได้ตรวจ'}
    </Badge>
  )
}
