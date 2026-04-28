import dayjs from 'dayjs'
import { Badge } from '@/components/ui/badge'
import BillStatusBadge from '@/components/BillStatusBadge'
import { DataTable } from '@/components/common/DataTable'
import type { Bill } from '@/types'

const SOURCE_LABELS: Record<string, string> = {
  line:           'LINE',
  email:          'Email',
  lazada:         'Lazada',
  shopee:         'Shopee Excel',
  shopee_email:   'Shopee Order',
  shopee_shipped: 'Shopee Shipped',
  manual:         'Manual',
}

interface Props {
  bills: Bill[]
  loading?: boolean
  onRowClick: (id: string) => void
}

export default function BillTable({ bills, loading, onRowClick }: Props) {
  return (
    <DataTable<Bill>
      data={bills}
      loading={loading}
      onRowClick={(b) => onRowClick(b.id)}
      empty="ไม่พบรายการบิล"
      columns={[
        {
          key: 'doc',
          header: 'เลขบิล',
          cell: (b) => (
            <div className="flex items-center gap-2">
              {b.sml_doc_no ? (
                <span className="font-mono text-xs font-medium text-foreground">
                  {b.sml_doc_no}
                </span>
              ) : (
                <span className="font-mono text-xs text-muted-foreground">
                  {b.id.slice(0, 8)}…
                </span>
              )}
              {b.bill_type === 'purchase' && (
                <Badge
                  variant="secondary"
                  className="h-5 bg-warning/15 px-1.5 text-[10px] font-medium text-warning hover:bg-warning/20"
                  title="ใบสั่งซื้อ/สั่งจอง (Purchase Order)"
                >
                  ใบสั่งซื้อ/สั่งจอง
                </Badge>
              )}
            </div>
          ),
        },
        {
          key: 'source',
          header: 'ช่องทาง',
          cell: (b) => (
            <span className="text-xs text-muted-foreground">
              {SOURCE_LABELS[b.source] ?? b.source}
            </span>
          ),
        },
        {
          key: 'date',
          header: 'วันที่',
          cell: (b) => (
            <span className="text-xs tabular-nums text-muted-foreground">
              {dayjs(b.created_at).format('DD/MM/YY HH:mm')}
            </span>
          ),
        },
        {
          key: 'amount',
          header: 'ยอดรวม',
          headerClassName: 'text-right',
          className: 'text-right',
          cell: (b) => (
            <span className="font-medium tabular-nums">
              ฿{(b.total_amount ?? 0).toLocaleString()}
            </span>
          ),
        },
        {
          key: 'status',
          header: 'สถานะ',
          headerClassName: 'text-center',
          className: 'text-center',
          cell: (b) => <BillStatusBadge status={b.status} />,
        },
      ]}
    />
  )
}
