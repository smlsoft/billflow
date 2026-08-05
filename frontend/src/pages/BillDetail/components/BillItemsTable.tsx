import { AlertTriangle, CheckCircle2, Info } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableHeader,
  TableHead,
  TableBody,
  TableRow,
} from '@/components/ui/table'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { Bill, BillItem } from '@/types'
import {
  isLazadaEmailPurchaseBill,
  isMarketplaceFeeSourceSKU,
  isMarketplacePurchaseBill,
  isShopeePurchaseBill,
  isShopeeSalesBill,
  marketplaceFeeAmount,
  money,
  shopeeCoinAmount,
  shopeePayableTotal,
} from '@/lib/shopeeBill'
import { hasInvalidPrice } from '../utils/validation'
import { BillItemRow, type DiscountInfo } from './BillItemRow'

interface Props {
  bill: Bill
  canEdit: boolean
  canDeleteItems: boolean
  canEditMarketplaceAmounts: boolean
  onItemDeleted: (itemId: string) => void
  onItemAdded: (item: BillItem) => void
  onRefresh: () => Promise<unknown>
  // BillTotal's "ดู →" link sets this to the offending item id; the matching
  // row briefly flashes (1.5s) so admin's eye is drawn to the right place
  // even when the items list is long.
  highlightItemId?: string | null
}

interface DiscountSummary {
  shopee_discount_amount?: number
  shop_discount_amount?: number
  coupon_discount_amount?: number
  total_discount_amount?: number
  shopee_discount_codes?: string[]
  shop_discount_codes?: string[]
}

function discountSummaryFromBill(bill: Bill): DiscountSummary | null {
  const value = bill.raw_data?.discount_summary
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  return value as DiscountSummary
}

const MONEY_TOLERANCE = 0.01

type TableTotalStatus = 'matched' | 'mismatch' | 'missing_email_total' | 'missing_fee_line'

interface MarketplaceTableTotals {
  goodsAmount: number
  discountAmount: number
  feeAmount: number
  tableTotal: number
  emailTotal: number | null
  delta: number | null
  rawFeeAmount: number
  missingFeeLine: boolean
  status: TableTotalStatus
}

function roundMoney(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.round(value * 100) / 100
}

function marketplaceTableTotals(bill: Bill, items: BillItem[]): MarketplaceTableTotals {
  let goodsAmount = 0
  let discountAmount = 0
  let feeAmount = 0

  for (const item of items) {
    const gross = roundMoney((item.qty ?? 0) * (item.price ?? 0))
    const discount = roundMoney(Math.max(item.discount_amount ?? 0, 0))
    const net = roundMoney(Math.max(gross - discount, 0))

    if (isMarketplaceFeeSourceSKU(item.source_sku)) {
      feeAmount = roundMoney(feeAmount + net)
      continue
    }

    goodsAmount = roundMoney(goodsAmount + gross)
    discountAmount = roundMoney(discountAmount + discount)
  }

  const tableTotal = roundMoney(goodsAmount - discountAmount + feeAmount)
  const emailTotal = shopeePayableTotal(bill)
  const delta = emailTotal == null ? null : roundMoney(tableTotal - emailTotal)
  const rawFeeAmount = marketplaceFeeAmount(bill)
  const missingFeeLine = rawFeeAmount > MONEY_TOLERANCE && feeAmount <= MONEY_TOLERANCE
  const status: TableTotalStatus = missingFeeLine
    ? 'missing_fee_line'
    : emailTotal == null
    ? 'missing_email_total'
    : Math.abs(delta ?? 0) <= MONEY_TOLERANCE
    ? 'matched'
    : 'mismatch'

  return {
    goodsAmount,
    discountAmount,
    feeAmount,
    tableTotal,
    emailTotal,
    delta,
    rawFeeAmount,
    missingFeeLine,
    status,
  }
}

function MarketplaceTableTotalSummary({
  bill,
  items,
  isLazadaPurchase,
}: {
  bill: Bill
  items: BillItem[]
  isLazadaPurchase: boolean
}) {
  const totals = marketplaceTableTotals(bill, items)
  const isSent = bill.status === 'sent'
  const discountLabel = isLazadaPurchase ? 'คูปอง' : 'ส่วนลด/coin'
  const feeLabel = isLazadaPurchase ? 'ค่าส่ง/fee' : 'ค่าส่ง'
  const deltaAbs = totals.delta == null ? null : Math.abs(totals.delta)
  const statusTone = {
    matched: 'border-success/30 bg-success/10 text-success',
    mismatch: 'border-warning/40 bg-warning/10 text-warning',
    missing_email_total: 'border-border bg-muted/50 text-muted-foreground',
    missing_fee_line: 'border-warning/40 bg-warning/10 text-warning',
  }[totals.status]
  const statusIcon = totals.status === 'matched'
    ? <CheckCircle2 className="h-3.5 w-3.5" />
    : totals.status === 'missing_email_total'
    ? <Info className="h-3.5 w-3.5" />
    : <AlertTriangle className="h-3.5 w-3.5" />
  const statusLabel = totals.status === 'matched'
    ? 'ตรงกับอีเมล'
    : totals.status === 'missing_fee_line'
    ? `${feeLabel} ยังไม่อยู่ในตาราง`
    : totals.status === 'missing_email_total'
    ? 'ยังเทียบยอดอีเมลไม่ได้'
    : `ต่างจากอีเมล ${money(deltaAbs)}`
  const statusDetail = totals.status === 'matched'
    ? 'รวมท้ายตารางตรงกับยอดชำระในอีเมล'
    : totals.status === 'missing_fee_line'
    ? `อีเมลมี${feeLabel} ${money(totals.rawFeeAmount)} แต่ตารางยังไม่มีรายการ ${feeLabel} จึงยังไม่ถือว่าตรง`
    : totals.status === 'missing_email_total'
    ? 'ไม่พบยอดชำระในอีเมล จึงแสดงได้เฉพาะยอดรวมจากรายการในตาราง'
    : totals.delta != null && totals.delta > 0
    ? `ยอดท้ายตารางมากกว่าอีเมล ${money(deltaAbs)}`
    : `ยอดท้ายตารางน้อยกว่าอีเมล ${money(deltaAbs)}`

  return (
    <div className="border-t border-border/70 bg-muted/20 px-4 py-3 sm:px-5">
      <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="text-sm font-semibold text-foreground">
              {isSent ? 'ยอดที่ส่ง/ตรวจสอบแล้ว' : 'ผลรวมท้ายตาราง'}
            </h4>
            <span className={`inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-xs font-medium ${statusTone}`}>
              {statusIcon}
              {statusLabel}
            </span>
          </div>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            สูตร: ยอดสินค้า + {feeLabel} - {discountLabel} = รวมท้ายตาราง
            {isSent && ' · บิลนี้ส่ง SML แล้ว ตัวเลขนี้ใช้ตรวจสอบยอดที่ส่ง ไม่แก้ย้อนหลัง'}
          </p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {statusDetail}
          </p>
        </div>

        <div className="grid min-w-0 grid-cols-2 gap-2 text-xs sm:grid-cols-5 lg:min-w-[620px]">
          <div className="rounded-md border border-border/70 bg-background px-3 py-2">
            <div className="text-muted-foreground">ยอดสินค้า</div>
            <div className="mt-1 font-semibold tabular-nums text-foreground">{money(totals.goodsAmount)}</div>
          </div>
          <div className="rounded-md border border-border/70 bg-background px-3 py-2">
            <div className="text-muted-foreground">{discountLabel}</div>
            <div className="mt-1 font-semibold tabular-nums text-success">-{money(totals.discountAmount)}</div>
          </div>
          <div className="rounded-md border border-border/70 bg-background px-3 py-2">
            <div className="text-muted-foreground">{feeLabel}</div>
            <div className="mt-1 font-semibold tabular-nums text-foreground">{money(totals.feeAmount)}</div>
          </div>
          <div className="rounded-md border border-border/70 bg-background px-3 py-2">
            <div className="text-muted-foreground">รวมท้ายตาราง</div>
            <div className="mt-1 font-semibold tabular-nums text-foreground">{money(totals.tableTotal)}</div>
          </div>
          <div className="rounded-md border border-border/70 bg-background px-3 py-2">
            <div className="text-muted-foreground">ยอดในอีเมล</div>
            <div className="mt-1 font-semibold tabular-nums text-foreground">{money(totals.emailTotal)}</div>
          </div>
        </div>
      </div>
    </div>
  )
}

export function BillItemsTable({
  bill,
  canEdit,
  canDeleteItems,
  canEditMarketplaceAmounts,
  onItemDeleted,
  onItemAdded,
  onRefresh,
  highlightItemId,
}: Props) {
  const items = bill.items ?? []
  const rawNameLabel = isShopeeSalesBill(bill) ? 'ชื่อสินค้าจาก Excel' : 'ชื่อสินค้าจากอีเมล'
  const isShopeePurchase = isShopeePurchaseBill(bill)
  const isLazadaPurchase = isLazadaEmailPurchaseBill(bill)
  const lockSourceAmounts = isMarketplacePurchaseBill(bill)
  const showDiscountColumn = isMarketplacePurchaseBill(bill)
  const discountSummary = showDiscountColumn ? discountSummaryFromBill(bill) : null
  const totalDiscount = discountSummary?.total_discount_amount ?? 0
  const coinAmt = isShopeePurchase ? (shopeeCoinAmount(bill) ?? 0) : 0
  const effectiveDiscount = totalDiscount + coinAmt
  const itemDiscountTotal = items.reduce((sum, item) => sum + (item.discount_amount ?? 0), 0)

  // gross รวม ทุก item ยกเว้น shipping — ใช้แสดงใน tooltip ของแต่ละ row
  const grossTotal = items
    .filter((item) => !isMarketplaceFeeSourceSKU(item.source_sku))
    .reduce((sum, item) => sum + (item.qty ?? 0) * (item.price ?? 0), 0)
  const rowDiscountInfo: DiscountInfo | undefined = showDiscountColumn && effectiveDiscount > 0
    ? {
        effectiveDiscount,
        couponDiscount: totalDiscount,
        coinAmount: coinAmt,
        grossTotal,
        platform: isLazadaPurchase ? 'lazada' : 'shopee',
      }
    : undefined
  const parsedDiscountNotApplied = bill.status === 'sent' && totalDiscount > 0 && itemDiscountTotal <= 0
  const discountCodes = [
    ...(discountSummary?.shopee_discount_codes ?? []),
    ...(discountSummary?.shop_discount_codes ?? []),
  ]
  const visibleColumnCount = canEdit
    ? showDiscountColumn ? 10 : 9
    : showDiscountColumn ? 9 : 8
  const issueCount = items.filter((item) => {
    return (
      !item.item_code ||
      item.mapped !== true ||
      !item.unit_code ||
      !item.qty ||
      item.qty <= 0 ||
      hasInvalidPrice(item)
    )
  }).length

  return (
    <Card className="rounded-2xl border-border/70 shadow-sm">
      <CardHeader className="flex flex-row items-start justify-between gap-3 pb-3">
        <div>
          <CardTitle className="text-sm font-semibold">
            รายการสินค้า ({items.length} รายการ)
          </CardTitle>
          <p className="mt-1 text-xs text-muted-foreground">
            ตรวจรหัสสินค้า หน่วย จำนวน และราคาให้ครบก่อนส่งเข้า SML
          </p>
          {showDiscountColumn && (
            <div className="mt-2 max-w-3xl rounded-md border border-info/20 bg-info/5 px-3 py-2 text-xs leading-5 text-muted-foreground">
              <div className="flex items-start gap-1.5">
                <div className="flex-1">
                  <span className="font-medium text-foreground">ส่วนลด:</span>{' '}
                  {parsedDiscountNotApplied
                    ? `${money(totalDiscount)} พบในอีเมล แต่บิลนี้ส่ง SML แล้ว ระบบไม่แก้ย้อนหลัง`
                    : effectiveDiscount > 0
                    ? <>
                        {money(effectiveDiscount)} รวมทั้งหมด
                        {isLazadaPurchase ? (
                          <> (คูปอง Lazada {money(discountSummary?.coupon_discount_amount ?? totalDiscount)})</>
                        ) : (
                          <>
                            {' ('}โค้ด Shopee {money(discountSummary?.shopee_discount_amount ?? 0)}
                            {(discountSummary?.shop_discount_amount ?? 0) > 0 && <> + ร้านค้า {money(discountSummary?.shop_discount_amount ?? 0)}</>}
                            {coinAmt > 0 && <> + Shopee Coin <span className="text-info font-medium">{money(coinAmt)}</span></>}
                            {')'}
                          </>
                        )}
                      </>
                    : 'ไม่พบส่วนลดในอีเมลนี้'}
                  {!parsedDiscountNotApplied && effectiveDiscount > 0 && ' · กระจายตาม % มูลค่าสินค้าแต่ละรายการ ไม่รวมค่าจัดส่ง/fee'}
                  {discountCodes.length > 0 && (
                    <span className="ml-1">· โค้ด: {discountCodes.join(', ')}</span>
                  )}
                </div>
                {!parsedDiscountNotApplied && effectiveDiscount > 0 && (
                  <TooltipProvider delayDuration={100}>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Info className="mt-0.5 h-3.5 w-3.5 shrink-0 cursor-pointer text-info/70 hover:text-info" />
                      </TooltipTrigger>
                      <TooltipContent side="top" className="max-w-xs text-xs leading-relaxed">
                        <p className="font-semibold mb-1">วิธีคำนวณส่วนลด</p>
                        {isLazadaPurchase ? (
                          <>
                            <p>1. ใช้คูปองส่วนลดจากสรุปยอด Lazada เท่านั้น</p>
                            <p>2. ส่วนลดต่อ item = คูปอง × (ราคา item / ราคารวมทุก item)</p>
                            <p>3. ไม่รวมค่าจัดส่งหรือ service fee ในฐานคำนวณส่วนลด</p>
                          </>
                        ) : (
                          <>
                            <p>1. Coin = ยอดสินค้า − โค้ดส่วนลด − (ยอดชำระ − ค่าส่ง)</p>
                            <p>2. ส่วนลดรวม = โค้ดส่วนลด + Coin</p>
                            <p>3. ส่วนลดต่อ item = ส่วนลดรวม × (ราคา item / ราคารวมทุก item)</p>
                            {coinAmt > 0 && (
                              <p className="mt-1 text-info">Shopee Coin {money(coinAmt)} ถูกรวมในส่วนลดแล้ว</p>
                            )}
                          </>
                        )}
                      </TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                )}
              </div>
            </div>
          )}
        </div>
        {issueCount > 0 ? (
          <span className="rounded-md bg-warning/10 px-2 py-1 text-xs font-medium text-warning">
            ต้องแก้ {issueCount} รายการ
          </span>
        ) : items.length > 0 ? (
          <span className="rounded-md bg-success/10 px-2 py-1 text-xs font-medium text-success">
            พร้อมส่ง
          </span>
        ) : null}
      </CardHeader>
      <CardContent className="p-0">
        <div className="overflow-x-auto">
          <Table className={showDiscountColumn ? 'min-w-[1260px]' : 'min-w-[1130px]'}>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[360px]">{rawNameLabel}</TableHead>
                <TableHead className="w-[220px]">รหัสสินค้า SML</TableHead>
                <TableHead className="w-[300px]">ชื่อสินค้าใน SML</TableHead>
                <TableHead className="w-[130px] text-center">ความมั่นใจ</TableHead>
                <TableHead className="w-[110px] text-right">จำนวน</TableHead>
                <TableHead className="w-[120px]">หน่วย</TableHead>
                <TableHead className="w-[140px] text-right">ราคา</TableHead>
                {showDiscountColumn && (
                  <TableHead className="w-[130px] text-right">ส่วนลด</TableHead>
                )}
                <TableHead className="w-[140px] text-right">รวม</TableHead>
                {canEdit && <TableHead className="w-[220px] text-center">จัดการ</TableHead>}
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((item) => (
                <BillItemRow
                  key={item.id}
                  item={item}
                  billId={bill.id}
                  editable={canEdit}
                  canDelete={canDeleteItems}
                  canEditMarketplaceAmounts={canEditMarketplaceAmounts}
                  lockSourceAmounts={lockSourceAmounts}
                  onDeleted={onItemDeleted}
                  onRefresh={onRefresh}
                  highlighted={item.id === highlightItemId}
                  rawNameLabel={rawNameLabel}
                  showDiscountColumn={showDiscountColumn}
                  tableColumnCount={visibleColumnCount}
                  discountInfo={rowDiscountInfo}
                />
              ))}
              {items.length === 0 && (
                <TableRow>
                  <td
                    colSpan={visibleColumnCount}
                    className="py-8 text-center text-sm text-muted-foreground"
                  >
                    ยังไม่มีรายการสินค้า
                  </td>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
        {showDiscountColumn && items.length > 0 && (
          <MarketplaceTableTotalSummary
            bill={bill}
            items={items}
            isLazadaPurchase={isLazadaPurchase}
          />
        )}
      </CardContent>
    </Card>
  )
}
