import dayjs from 'dayjs'
import type { MouseEvent } from 'react'
import { Archive, CreditCard, Loader2, Mail, MoreHorizontal, Printer, RotateCcw, Sparkles, Store, Trash2, UserCog } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { BillSourceBadge } from '@/components/BillSourceBadge'
import BillStatusBadge from '@/components/BillStatusBadge'
import { DataTable } from '@/components/common/DataTable'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  isShopeePurchaseBill,
  isShopeeSalesBill,
  money,
  rawNumber,
  rawString,
  shopeeOrderID,
  shopeePayableTotal,
} from '@/lib/shopeeBill'
import { cn } from '@/lib/utils'
import type { Bill, ShopeeOrderEvent } from '@/types'

interface Props {
  bills: Bill[]
  loading?: boolean
  onRowClick: (id: string) => void
  showShopeeStatusColumn?: boolean
  canManage?: boolean
  canPermanentDelete?: boolean
  virtualize?: boolean
  onArchive?: (bill: Bill) => void
  onRestore?: (bill: Bill) => void
  onDelete?: (bill: Bill) => void
  onPermanentDelete?: (bill: Bill) => void
  canUpdatePurchaseCreditor?: boolean
  purchaseCreditorLoadingBillId?: string | null
  onUpdatePurchaseCreditor?: (bill: Bill) => void
  canUpdatePrintPaymentMethod?: boolean
  printPaymentMethodLoadingBillId?: string | null
  onUpdatePrintPaymentMethod?: (bill: Bill) => void
  printLoadingMessageID?: string | null
  onPrintEmail?: (bill: Bill) => void
}

export default function BillTable({
  bills,
  loading,
  onRowClick,
  showShopeeStatusColumn = true,
  canManage = true,
  canPermanentDelete = false,
  virtualize = false,
  onArchive,
  onRestore,
  onDelete,
  onPermanentDelete,
  canUpdatePurchaseCreditor = false,
  purchaseCreditorLoadingBillId = null,
  onUpdatePurchaseCreditor,
  canUpdatePrintPaymentMethod = false,
  printPaymentMethodLoadingBillId = null,
  onUpdatePrintPaymentMethod,
  printLoadingMessageID = null,
  onPrintEmail,
}: Props) {
  return (
    <DataTable<Bill>
      data={bills}
      loading={loading}
      onRowClick={(b) => onRowClick(b.id)}
      getRowKey={(b) => b.id}
      empty="ไม่พบรายการบิล"
      dense
      virtualize={virtualize}
      virtualizeThreshold={100}
      virtualRowHeight={64}
      virtualMaxHeight={620}
      columns={[
        {
          key: 'doc',
          header: 'บิล / คำสั่งซื้อ',
          className: 'px-3 py-1.5',
          width: '42%',
          cell: (b) => {
            const displayDate = billDisplayDate(b)
            const creditor = purchaseCreditorLabel(b)
            return (
              <div className="min-w-0 space-y-1">
                <div className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5">
                  {b.sml_doc_no ? (
                    <span
                      className="min-w-0 max-w-[390px] truncate text-[13px] font-semibold leading-5 text-foreground"
                      title={creditor ? `${creditor} ~ ${b.sml_doc_no}` : b.sml_doc_no}
                    >
                      {creditor && (
                        <>
                          <span>{creditor}</span>
                          <span className="mx-1 text-muted-foreground">~</span>
                        </>
                      )}
                      <span className="font-mono">{b.sml_doc_no}</span>
                    </span>
                  ) : (
                    <span className="font-mono text-[13px] font-semibold leading-5 text-foreground">
                      {b.id.slice(0, 8)}…
                    </span>
                  )}
                  {b.bill_type === 'purchase' && (
                    <Badge
                      variant="secondary"
                      className="h-4 bg-warning/15 px-1.5 text-[10px] font-medium leading-none text-warning hover:bg-warning/20"
                      title="ซื้อ -> ใบสั่งซื้อ"
                    >
                      บิลซื้อ
                    </Badge>
                  )}
                  {b.bill_type === 'sale' && (
                    <Badge
                      variant="secondary"
                      className="h-4 bg-primary/10 px-1.5 text-[10px] font-medium leading-none text-primary hover:bg-primary/15"
                      title={b.document_route === 'saleinvoice' ? 'ขาย -> ขายสินค้าและบริการ' : 'ขาย -> ใบสั่งขาย'}
                    >
                      {b.document_route === 'saleinvoice' ? 'ขายสินค้าฯ' : 'บิลขาย'}
                    </Badge>
                  )}
                  <span className="text-[11px] leading-5 text-muted-foreground" title={displayDate.title}>
                    {displayDate.prefix && (
                      <span className="mr-1 text-[10px] font-medium text-info">{displayDate.prefix}</span>
                    )}
                    {displayDate.short}
                  </span>
                  {b.ai_confidence != null && b.ai_confidence > 0 && (
                    <span
                      className={cn(
                        'inline-flex items-center gap-0.5 rounded-full px-1.5 py-0.5 text-[10px] font-medium',
                        b.ai_confidence >= 0.9
                          ? 'bg-success/15 text-success'
                          : b.ai_confidence >= 0.7
                          ? 'bg-primary/10 text-primary'
                          : 'bg-warning/15 text-warning',
                      )}
                      title={`AI อ่านข้อมูลด้วยความมั่นใจ ${Math.round(b.ai_confidence * 100)}%`}
                    >
                      <Sparkles className="h-2.5 w-2.5" />
                      {Math.round(b.ai_confidence * 100)}%
                    </span>
                  )}
                </div>
                <span className="block h-px w-0 overflow-hidden">
                  {displayDate.long}
                </span>
                <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5">
                  {isShopeePurchaseBill(b) && (
                    <ShopeePurchaseSummary bill={b} />
                  )}
                  {isShopeeSalesBill(b) && (
                    <ShopeeSalesSummary bill={b} />
                  )}
                  <ShopeeShopLine bill={b} />
                  <EmailGroupLine bill={b} />
                </div>
              </div>
            )
          },
        },
        {
          key: 'source',
          header: 'ช่องทาง',
          className: 'px-3 py-1.5',
          cell: (b) => {
            const inbox = emailInboxLabel(b)
            return (
              <div className="flex min-w-0 flex-col gap-0.5">
                <BillSourceBadge source={b.source} className="px-1.5 py-0.5 text-[11px] leading-4" />
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
          className: 'px-3 py-1.5 text-right',
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
                <PrintPaymentMethodLine bill={b} />
              </div>
            )
          },
        },
        {
          key: 'status',
          header: 'สถานะบิล',
          headerClassName: 'text-center',
          className: 'px-3 py-1.5 text-center',
          cell: (b) => (
            <div className="flex justify-center whitespace-nowrap">
              <BillStatusBadge status={b.status} />
            </div>
          ),
        },
        ...(showShopeeStatusColumn
          ? [{
              key: 'shopee_status',
              header: 'สถานะคำสั่งซื้อ',
              headerClassName: 'text-center',
              className: 'px-3 py-1.5 text-center',
              cell: (b: Bill) => (
                <div className="flex justify-center">
                  {b.source === 'lazada_email' && !b.shopee_status ? (
                    <span className="text-xs text-muted-foreground/70">—</span>
                  ) : (
                    <ShopeeOrderStatusBadge event={b.shopee_status} title={shopeeStatusTitle(b)} />
                  )}
                </div>
              ),
            }]
          : []),
        {
          key: 'actions',
          header: 'จัดการ',
          headerClassName: 'text-right',
          className: 'px-3 py-1.5 text-right',
          cell: (b) => (
            <BillRowActions
              bill={b}
              canManage={canManage}
              canPermanentDelete={canPermanentDelete}
              onArchive={onArchive}
              onRestore={onRestore}
              onDelete={onDelete}
              onPermanentDelete={onPermanentDelete}
              canUpdatePurchaseCreditor={canUpdatePurchaseCreditor}
              purchaseCreditorLoadingBillId={purchaseCreditorLoadingBillId}
              onUpdatePurchaseCreditor={onUpdatePurchaseCreditor}
              canUpdatePrintPaymentMethod={canUpdatePrintPaymentMethod}
              printPaymentMethodLoadingBillId={printPaymentMethodLoadingBillId}
              onUpdatePrintPaymentMethod={onUpdatePrintPaymentMethod}
              printLoadingMessageID={printLoadingMessageID}
              onPrintEmail={onPrintEmail}
            />
          ),
        },
      ]}
      rowClassName={(b) =>
        b.archived_at
          ? 'bg-muted/25 opacity-80'
          : b.status === 'needs_review'
          ? 'bg-warning/[0.025]'
          : b.status === 'failed'
          ? 'bg-destructive/[0.025]'
          : ''
      }
    />
  )
}

function BillRowActions({
  bill,
  canManage,
  canPermanentDelete,
  onArchive,
  onRestore,
  onDelete,
  onPermanentDelete,
  canUpdatePurchaseCreditor,
  purchaseCreditorLoadingBillId,
  onUpdatePurchaseCreditor,
  canUpdatePrintPaymentMethod,
  printPaymentMethodLoadingBillId,
  onUpdatePrintPaymentMethod,
  printLoadingMessageID,
  onPrintEmail,
}: {
  bill: Bill
  canManage: boolean
  canPermanentDelete: boolean
  onArchive?: (bill: Bill) => void
  onRestore?: (bill: Bill) => void
  onDelete?: (bill: Bill) => void
  onPermanentDelete?: (bill: Bill) => void
  canUpdatePurchaseCreditor: boolean
  purchaseCreditorLoadingBillId?: string | null
  onUpdatePurchaseCreditor?: (bill: Bill) => void
  canUpdatePrintPaymentMethod: boolean
  printPaymentMethodLoadingBillId?: string | null
  onUpdatePrintPaymentMethod?: (bill: Bill) => void
  printLoadingMessageID?: string | null
  onPrintEmail?: (bill: Bill) => void
}) {
  const stop = (fn?: (bill: Bill) => void) => (e: MouseEvent) => {
    e.stopPropagation()
    fn?.(bill)
  }

  if (!canManage) return null

  if (bill.archived_at) {
    return (
      <div className="flex justify-end gap-1.5">
        <Button type="button" size="sm" variant="outline" className="h-7 px-2 text-xs" onClick={stop(onRestore)}>
          <RotateCcw className="mr-1 h-3.5 w-3.5" />
          กู้คืน
        </Button>
        {canPermanentDelete && (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="h-7 px-2 text-xs text-destructive hover:text-destructive"
            onClick={stop(onPermanentDelete)}
          >
            <Trash2 className="mr-1 h-3.5 w-3.5" />
            ลบถาวร
          </Button>
        )}
      </div>
    )
  }

  if (bill.status === 'sent' || bill.status === 'skipped') {
    const showUpdateCreditor = canShowPurchaseCreditorAction(bill, canUpdatePurchaseCreditor) && !!onUpdatePurchaseCreditor
    const loadingCreditor = purchaseCreditorLoadingBillId === bill.id
    const showUpdatePayment = canShowPrintPaymentMethodAction(bill, canUpdatePrintPaymentMethod) && !!onUpdatePrintPaymentMethod
    const loadingPayment = printPaymentMethodLoadingBillId === bill.id
    const showPrint = canShowEmailPrintAction(bill) && !!onPrintEmail
    const printReady = Boolean(bill.email_group?.print_ready)
    const printLoading = printLoadingMessageID === bill.email_group?.message_id
    const printReason = bill.email_group?.print_block_reason || bill.email_group?.print_policy_note || 'ยังไม่พร้อมพิมพ์'
    const hasSecondaryActions = showUpdateCreditor || showUpdatePayment || !!onArchive
    return (
      <div className="flex justify-end gap-1.5" onClick={(e) => e.stopPropagation()}>
        {showPrint && (
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="h-7 px-2 text-xs"
            onClick={stop(onPrintEmail)}
            disabled={!printReady || printLoading}
            title={printReady ? 'พิมพ์อีเมลต้นฉบับ' : printReason}
          >
            {printLoading ? (
              <Loader2 className="mr-1 h-3.5 w-3.5 animate-spin" />
            ) : (
              <Printer className="mr-1 h-3.5 w-3.5" />
            )}
            {printLoading ? 'กำลังพิมพ์...' : 'พิมพ์'}
          </Button>
        )}
        {hasSecondaryActions && (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                type="button"
                size="icon"
                variant="outline"
                className="h-7 w-8"
                title="จัดการเพิ่มเติม"
              >
                <MoreHorizontal className="h-3.5 w-3.5" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-[180px]">
              {showUpdateCreditor && (
                <DropdownMenuItem
                  disabled={loadingCreditor}
                  onSelect={() => onUpdatePurchaseCreditor?.(bill)}
                >
                  {loadingCreditor ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <UserCog className="h-3.5 w-3.5" />
                  )}
                  {loadingCreditor ? 'กำลังเปิด...' : 'แก้เจ้าหนี้'}
                </DropdownMenuItem>
              )}
              {showUpdatePayment && (
                <DropdownMenuItem
                  disabled={loadingPayment}
                  onSelect={() => onUpdatePrintPaymentMethod?.(bill)}
                >
                  {loadingPayment ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <CreditCard className="h-3.5 w-3.5" />
                  )}
                  {loadingPayment ? 'กำลังเปิด...' : 'วิธีชำระ'}
                </DropdownMenuItem>
              )}
              {(showUpdateCreditor || showUpdatePayment) && onArchive && (
                <DropdownMenuSeparator />
              )}
              {onArchive && (
                <DropdownMenuItem onSelect={() => onArchive(bill)}>
                  <Archive className="h-3.5 w-3.5" />
                  เก็บบิล
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
    )
  }

  return (
    <Button
      type="button"
      size="sm"
      variant="ghost"
      className="h-7 px-2 text-xs text-destructive hover:text-destructive"
      onClick={stop(onDelete)}
    >
      <Trash2 className="mr-1 h-3.5 w-3.5" />
      ลบบิล
    </Button>
  )
}

function canShowEmailPrintAction(bill: Bill) {
  return (
    bill.status === 'sent' &&
    bill.bill_type === 'purchase' &&
    (bill.source === 'shopee_shipped' || bill.source === 'lazada_email') &&
    !!bill.email_group?.has_printable_email &&
    !bill.archived_at
  )
}

function canShowPurchaseCreditorAction(bill: Bill, enabled: boolean) {
  return (
    enabled &&
    bill.status === 'sent' &&
    bill.bill_type === 'purchase' &&
    (bill.source === 'shopee_shipped' || bill.source === 'lazada_email') &&
    !!bill.sml_doc_no &&
    !bill.archived_at
  )
}

function canShowPrintPaymentMethodAction(bill: Bill, enabled: boolean) {
  return (
    enabled &&
    bill.status === 'sent' &&
    bill.bill_type === 'purchase' &&
    (bill.source === 'shopee_shipped' || bill.source === 'lazada_email') &&
    !!bill.sml_doc_no &&
    !bill.archived_at
  )
}

function purchaseCreditorLabel(bill: Bill): string {
  if (bill.bill_type !== 'purchase' || !bill.sml_doc_no) return ''
  const code = payloadString(bill.sml_payload, 'cust_code')
  const name = payloadString(bill.sml_payload, 'supplier_name') || payloadString(bill.sml_payload, 'party_name')
  if (code && name && code !== name) return `${code} ~ ${name}`
  return code || name || ''
}

function PrintPaymentMethodLine({ bill }: { bill: Bill }) {
  if (
    bill.bill_type !== 'purchase' ||
    (bill.source !== 'shopee_shipped' && bill.source !== 'lazada_email')
  ) {
    return null
  }
  const method = (bill.effective_print_payment_method || bill.print_payment_method || '').trim()
  if (!method) return null
  const ready = method.toUpperCase().startsWith('TT')
  return (
    <span
      className={cn(
        'inline-flex w-fit max-w-full items-center gap-1 rounded-md border px-2 py-0.5 text-[11px]',
        ready
          ? 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/60 dark:bg-emerald-950/30 dark:text-emerald-200'
          : 'border-warning/30 bg-warning/10 text-warning',
      )}
      title={`วิธีชำระเงินสำหรับปริ้น: ${method}`}
    >
      <CreditCard className="h-3 w-3 shrink-0" />
      <span className="truncate">{method}</span>
    </span>
  )
}

function payloadString(payload: Record<string, unknown> | null | undefined, key: string): string {
  const value = payload?.[key]
  return typeof value === 'string' ? value.trim() : ''
}

export function ShopeeOrderStatusBadge({
  event,
  title,
}: {
  event?: ShopeeOrderEvent | null
  title?: string
}) {
  if (!event) {
    return (
      <span className="inline-flex h-6 items-center rounded-full border border-border bg-muted/40 px-2 text-[11px] font-medium text-muted-foreground">
        ยังไม่มีสถานะ
      </span>
    )
  }

  const cls = shopeeStatusClass(event.event_type)
  return (
    <Badge
      variant="outline"
      className={`max-w-[160px] truncate px-2 py-0.5 text-[11px] font-semibold ${cls}`}
      title={title || [event.status_label, event.order_id, event.subject].filter(Boolean).join(' · ')}
    >
      {event.status_label}
    </Badge>
  )
}

function shopeeStatusClass(eventType: string): string {
  switch (eventType) {
    case 'delivered':
      return 'border-emerald-300 bg-emerald-50 text-emerald-700 hover:bg-emerald-50'
    case 'cancelled':
      return 'border-red-300 bg-red-50 text-red-700 hover:bg-red-50'
    case 'refund':
    case 'return':
      return 'border-amber-300 bg-amber-50 text-amber-700 hover:bg-amber-50'
    case 'picked_up':
      return 'border-indigo-300 bg-indigo-50 text-indigo-700 hover:bg-indigo-50'
    case 'shipped':
    case 'ready_to_ship':
    case 'payment_confirmed':
      return 'border-sky-300 bg-sky-50 text-sky-700 hover:bg-sky-50'
    default:
      return 'border-slate-300 bg-slate-50 text-slate-700 hover:bg-slate-50'
  }
}

function shopeeStatusTitle(bill: Bill): string {
  const event = bill.shopee_status
  if (!event) return ''
  const when = dayjs(event.email_date || event.created_at)
  const date = when.isValid() ? when.format('DD/MM/YYYY HH:mm') : ''
  return [event.status_label, event.order_id ? `Order ${event.order_id}` : '', date, event.subject]
    .filter(Boolean)
    .join(' · ')
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

function EmailGroupLine({ bill }: { bill: Bill }) {
  const group = bill.email_group
  if (!group?.message_id || !group.group_key) return null

  const orderCount = group.order_count ?? 0
  const isMultiOrder = orderCount > 1
  const hasOrderCount = orderCount >= 1
  const printCount = group.print_count ?? 0
  const hasPrintHistory = printCount > 0
  const printedAt = group.last_printed_at ? dayjs(group.last_printed_at) : null
  const printedAtLabel = printedAt?.isValid() ? printedAt.format('DD/MM/YYYY HH:mm') : ''
  const printedBy = group.last_printed_by_name || group.last_printed_by_email || ''
  const printTooltip = hasPrintHistory
    ? [
        `พิมพ์แล้ว ${printCount.toLocaleString('th-TH')} ครั้ง`,
        printedAtLabel ? `ล่าสุด ${printedAtLabel}` : '',
        printedBy ? `โดย ${printedBy}` : '',
      ].filter(Boolean).join(' · ')
    : ''
  const tooltip = [
    group.subject ? `Subject: ${group.subject}` : '',
    group.from ? `From: ${group.from}` : '',
    `Message-ID: ${group.message_id}`,
    printTooltip,
  ].filter(Boolean).join('\n')

  return (
    <div
      className={`inline-flex max-w-full items-center gap-1.5 rounded-md border px-1.5 py-px text-[11px] leading-4 ${
        isMultiOrder
          ? 'border-info/30 bg-info/10 text-info'
          : 'border-border/70 bg-muted/30 text-muted-foreground'
      }`}
      title={tooltip}
    >
      <Mail className="h-3 w-3 shrink-0" />
      <span className="shrink-0 font-mono">Email #{group.group_key}</span>
      {hasOrderCount && (
        <span className={`shrink-0 ${isMultiOrder ? 'font-medium' : 'font-normal'}`}>
          · {orderCount.toLocaleString('th-TH')} คำสั่งซื้อ
        </span>
      )}
      {hasPrintHistory && (
        <span className="inline-flex shrink-0 items-center gap-1 rounded bg-emerald-500/10 px-1 py-0 text-[10px] font-medium text-emerald-700 dark:text-emerald-200">
          <Printer className="h-2.5 w-2.5" aria-hidden="true" />
          พิมพ์แล้ว {printCount.toLocaleString('th-TH')}
        </span>
      )}
    </div>
  )
}

function ShopeeShopLine({ bill }: { bill: Bill }) {
  const raw = bill.raw_data
  const shopID = rawString(raw, 'shopee_shop_id')
  if (!shopID) return null
  const label = rawString(raw, 'shopee_shop_label') || 'Shopee shop'
  return (
    <div
      className="inline-flex max-w-full items-center gap-1.5 rounded-md border border-orange-200 bg-orange-50 px-1.5 py-px text-[11px] leading-4 text-orange-700"
      title={`${label} · shop_id=${shopID}`}
    >
      <Store className="h-3 w-3 shrink-0" />
      <span className="min-w-0 truncate">{label}</span>
      <span className="shrink-0 font-mono">· {shopID}</span>
    </div>
  )
}

function ShopeeSalesSummary({ bill }: { bill: Bill }) {
  const raw = bill.raw_data
  const orderID = shopeeOrderID(raw)
  const orderDate = rawString(raw, 'order_datetime') || rawString(raw, 'doc_date')
  const buyer = rawString(raw, 'customer_name') || rawString(raw, 'buyer_username')
  const itemCount = rawNumber(raw, 'item_count') ?? bill.items?.length ?? null

  return (
    <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-[11px] leading-4 text-muted-foreground">
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
    <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-[11px] leading-4 text-muted-foreground">
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
