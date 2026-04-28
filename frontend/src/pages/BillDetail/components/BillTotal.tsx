import { RefreshCw, Send } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import type { Bill } from '@/types'

interface Props {
  bill: Bill
  total: number
  retrying: boolean
  onRetry: () => void
}

export function BillTotal({ bill, total, retrying, onRetry }: Props) {
  const canSend =
    bill.status === 'failed' ||
    bill.status === 'pending' ||
    bill.status === 'needs_review'
  const isPurchase = bill.bill_type === 'purchase'
  const isFailed = bill.status === 'failed'

  return (
    <Card>
      <CardContent className="flex items-center justify-between gap-4 py-4">
        <div>
          <div className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
            ยอดรวมทั้งหมด
          </div>
          <div className="mt-0.5 text-2xl font-bold tabular-nums tracking-tight">
            ฿{total.toLocaleString()}
          </div>
        </div>

        {canSend && (
          <Button
            type="button"
            onClick={onRetry}
            disabled={retrying}
            variant={isFailed ? 'destructive' : 'default'}
            className="gap-2 shrink-0"
          >
            {retrying ? (
              <RefreshCw className="h-4 w-4 animate-spin" />
            ) : isFailed ? (
              <RefreshCw className="h-4 w-4" />
            ) : (
              <Send className="h-4 w-4" />
            )}
            {retrying
              ? 'กำลังส่ง...'
              : isFailed
                ? `⚠️ ลองส่งใหม่${isPurchase ? ' (ใบสั่งซื้อ/สั่งจอง)' : ''}`
                : `ยืนยันและส่งไปยัง SML${isPurchase ? ' (ใบสั่งซื้อ/สั่งจอง)' : ''}`}
          </Button>
        )}
      </CardContent>
    </Card>
  )
}
