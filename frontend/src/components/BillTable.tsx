import dayjs from 'dayjs'
import { Badge } from '@/components/ui/badge'
import BillStatusBadge from '@/components/BillStatusBadge'
import { DataTable } from '@/components/common/DataTable'
import { billSourceLabel } from '@/lib/labels'
import {
  isShopeePurchaseBill,
  isShopeeSalesBill,
  money,
  rawNumber,
  rawString,
  shopeeOrderID,
  shopeePayableTotal,
} from '@/lib/shopeeBill'
import type { Bill } from '@/types'

interface Props {
  bills: Bill[]
  loading?: boolean
  onRowClick: (id: string) => void
  showShopeeStatusColumn?: boolean
  canManage?: boolean
  canPermanentDelete?: boolean
  onArchive?: (bill: Bill) => void
  onRestore?: (bill: Bill) => void
  onDelete?: (bill: Bill) => void
  onPermanentDelete?: (bill: Bill) => void
}

export default function BillTable({ bills, loading, onRowClick }: Props) {
  return (
    <DataTable<Bill>
      data={bills}
      loading={loading}
      onRowClick={(b) => onRowClick(b.id)}
      empty="ไม่พบรายการบิล"
      dense
      columns={[
        {
          key: 'doc',
          header: 'บิล / คำสั่งซื้อ',
          className: 'py-2',
          width: '52%',
          cell: (b) => {
            const displayDate = billDisplayDate(b)
            return (
              <div className="min-w-0 space-y-1">
                <div className="flex items-center gap-2">
                  {b.sml_doc_no ? (
                    <span className="font-mono text-xs font-medium text-foreground">
                      {b.sml_doc_no}
                    </span>
                  ) : (
                    <span className="font-mono text-xs text-foreground">
                      {b.id.slice(0, 8)}…
                    </span>
                  )}
                  {b.bill_type === 'purchase' && (
                    <Badge
                      variant="secondary"
                      className="h-5 bg-warning/15 px-1.5 text-[10px] font-medium text-warning hover:bg-warning/20"
                      title="ซื้อ -> ใบสั่งซื้อ"
                    >
                      บิลซื้อ
                    </Badge>
                  )}
                  {b.bill_type === 'sale' && (
                    <Badge
                      variant="secondary"
                      className="h-5 bg-primary/10 px-1.5 text-[10px] font-medium text-primary hover:bg-primary/15"
                      title={b.document_route === 'saleinvoice' ? 'ขาย -> ขายสินค้าและบริการ' : 'ขาย -> ใบสั่งขาย'}
                    >
                      {b.document_route === 'saleinvoice' ? 'ขายสินค้าฯ' : 'บิลขาย'}
                    </Badge>
                  )}
                  <span className="text-[11px] text-muted-foreground" title={displayDate.title}>
                    {displayDate.prefix && (
                      <span className="mr-1 text-[10px] font-medium text-info">{displayDate.prefix}</span>
                    )}
                    {displayDate.short}
                  </span>
                </div>
                <span className="block h-px w-0 overflow-hidden">
                  {displayDate.long}
                </span>
                {isShopeePurchaseBill(b) && (
                  <ShopeePurchaseSummary bill={b} />
                )}
                {isShopeeSalesBill(b) && (
                  <ShopeeSalesSummary bill={b} />
                )}
              </div>
            )
          },
        },
        {
          key: 'source',
          header: 'ช่องทาง',
          className: 'py-2',
          cell: (b) => {
            const inbox = emailInboxLabel(b)
            return (
              <div className="flex min-w-0 flex-col gap-1">
                <span className="inline-flex w-fit rounded-full bg-muted px-2 py-1 text-xs text-muted-foreground">
                  {billSourceLabel(b.source)}
                </span>
                {inbox && (
                  <span className="max-w-[180px] truncate text-[11px] text-muted-foreground" title={inbox}>
                    {inbox}
                  </span>
                )}
              </div>
            )
          },
        },
        {
          key: 'amount',
          header: 'ยอดชำระ',
          headerClassName: 'text-right',
          className: 'py-2 text-right',
          cell: (b) => {
            const payable = shopeePayableTotal(b)
            const fallback = b.total_amount ?? 0
            return (
              <div className="flex flex-col items-end gap-0.5">
                <span className="font-medium tabular-nums">
                  {money(payable ?? fallback)}
                </span>
                {payable != null && b.total_amount != null && payable !== b.total_amount && (
                  <span className="text-[10px] text-muted-foreground">
                    สินค้า {money(b.total_amount)}
                  </span>
                )}
              </div>
            )
          },
        },
        {
          key: 'status',
          header: 'สถานะ',
          headerClassName: 'text-center',
          className: 'py-2 text-center',
          cell: (b) => <BillStatusBadge status={b.status} />,
        },
      ]}
      rowClassName={(b) =>
        b.status === 'needs_review'
          ? 'bg-warning/[0.025]'
          : b.status === 'failed'
            ? 'bg-destructive/[0.025]'
            : ''
      }
    />
  )
}

function billDisplayDate(bill: Bill): { short: string; long: string; title: string; prefix: string } {
  const emailDate = rawString(bill.raw_data, 'email_date')
  const parsedEmailDate = emailDate ? dayjs(emailDate) : null
  if (parsedEmailDate?.isValid()) {
    return {
      short: parsedEmailDate.format('DD/MM/YY HH:mm'),
      long: parsedEmailDate.format('DD/MM/YYYY HH:mm'),
      title: `วันที่อีเมล: ${parsedEmailDate.format('DD/MM/YYYY HH:mm')}`,
      prefix: 'อีเมล',
    }
  }

  const created = dayjs(bill.created_at)
  return {
    short: created.format('DD/MM/YY HH:mm'),
    long: created.format('DD/MM/YYYY HH:mm'),
    title: `วันที่เข้าระบบ: ${created.format('DD/MM/YYYY HH:mm')}`,
    prefix: '',
  }
}

function emailInboxLabel(bill: Bill): string {
  const raw = bill.raw_data
  if (!raw) return ''
  const name = rawString(raw, 'imap_account_name')
  const user = rawString(raw, 'imap_username')
  if (name && user) return `${name} · ${user}`
  return name || user || ''
}

function ShopeeSalesSummary({ bill }: { bill: Bill }) {
  const raw = bill.raw_data
  const orderID = shopeeOrderID(raw)
  const orderDate = rawString(raw, 'order_datetime') || rawString(raw, 'doc_date')
  const buyer = rawString(raw, 'customer_name') || rawString(raw, 'buyer_username')
  const itemCount = rawNumber(raw, 'item_count') ?? bill.items?.length ?? null
  const status = rawString(raw, 'status')

  return (
    <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-0.5 text-[11px] leading-5 text-muted-foreground">
      {orderID && (
        <span className="min-w-0">
          เลขคำสั่งซื้อ{' '}
          <span className="font-mono text-foreground">{orderID}</span>
        </span>
      )}
      {orderDate && (
        <span>
          วันที่สั่งซื้อ: <span className="text-foreground">{orderDate}</span>
        </span>
      )}
      {buyer && (
        <span>
          ผู้ซื้อ: <span className="text-foreground">{buyer}</span>
        </span>
      )}
      {itemCount != null && (
        <span>
          <span className="tabular-nums text-foreground">{itemCount}</span> รายการ
        </span>
      )}
      {status && (
        <span>
          สถานะ: <span className="text-foreground">{status}</span>
        </span>
      )}
    </div>
  )
}

function ShopeePurchaseSummary({ bill }: { bill: Bill }) {
  const raw = bill.raw_data
  const orderID = shopeeOrderID(raw)
  const orderDate = rawString(raw, 'order_datetime')
  const seller = rawString(raw, 'seller_name')
  const itemCount = rawNumber(raw, 'item_count') ?? bill.items?.length ?? null

  return (
    <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-0.5 text-[11px] leading-5 text-muted-foreground">
      {orderID && (
        <span className="min-w-0">
          เลขคำสั่งซื้อ{' '}
          <span className="font-mono text-foreground">{orderID}</span>
        </span>
      )}
      {orderDate && (
        <span>
          วันที่สั่งซื้อ: <span className="text-foreground">{orderDate}</span>
        </span>
      )}
      {seller && (
        <span>
          ผู้ขาย: <span className="text-foreground">{seller}</span>
        </span>
      )}
      {itemCount != null && (
        <span>
          <span className="tabular-nums text-foreground">{itemCount}</span> รายการ
        </span>
      )}
    </div>
  )
}
