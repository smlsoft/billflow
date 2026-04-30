import { ArrowLeft } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import dayjs from 'dayjs'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import BillStatusBadge from '@/components/BillStatusBadge'
import type { Bill } from '@/types'
import { SOURCE_LABELS } from '../utils/formatters'

interface Props {
  bill: Bill
}

function InfoRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
        {label}
      </span>
      <span className="text-sm text-foreground">{value || '—'}</span>
    </div>
  )
}

export function BillHeader({ bill }: Props) {
  const navigate = useNavigate()
  const rawData = bill.raw_data as Record<string, unknown> | null
  const isPurchase = bill.bill_type === 'purchase'

  return (
    <div className="space-y-3">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="gap-1.5 text-muted-foreground hover:text-foreground -ml-2 h-8"
        onClick={() => navigate(-1)}
      >
        <ArrowLeft className="h-4 w-4" />
        กลับ
      </Button>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between gap-3 pb-3 border-b">
          <div className="flex items-center gap-2">
            <h2 className="font-mono text-lg font-bold tracking-tight">
              {bill.sml_doc_no ?? bill.id.slice(0, 8)}
            </h2>
            {isPurchase && (
              <Badge
                variant="secondary"
                className="bg-warning/15 text-warning hover:bg-warning/20"
                title="Purchase Order"
              >
                ใบสั่งซื้อ/สั่งจอง
              </Badge>
            )}
          </div>
          <BillStatusBadge status={bill.status} />
        </CardHeader>

        <CardContent className="pt-5">
          <div className="grid grid-cols-2 gap-x-8 gap-y-4 md:grid-cols-3">
            <InfoRow
              label={isPurchase ? 'ผู้ขาย (Supplier)' : 'ลูกค้า'}
              value={(rawData?.customer_name as string) || '—'}
            />
            <InfoRow
              label="เบอร์โทร"
              value={(rawData?.customer_phone as string) || '—'}
            />
            <InfoRow
              label="Platform"
              value={SOURCE_LABELS[bill.source] ?? bill.source}
            />
            <InfoRow
              label="วันที่สร้าง"
              value={dayjs(bill.created_at).format('DD/MM/YYYY HH:mm')}
            />
            {bill.sent_at && (
              <InfoRow
                label="ส่ง SML เมื่อ"
                value={dayjs(bill.sent_at).format('DD/MM/YYYY HH:mm')}
              />
            )}
            {bill.ai_confidence != null && (
              <InfoRow
                label="AI Confidence"
                value={`${Math.round(bill.ai_confidence * 100)}%`}
              />
            )}
          </div>

          {/* Failure details moved to BillFailureCard (rendered alongside this
              header by the parent) — gives the error room to breathe + a
              copy button without cluttering the meta grid. */}
        </CardContent>
      </Card>
    </div>
  )
}
