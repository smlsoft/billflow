import { AlertTriangle, RefreshCw, Send } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { isSMLReady, smlBlockedMessage } from '@/lib/sml-readiness'
import {
  isLazadaEmailPurchaseBill,
  isMarketplaceFeeSourceSKU,
  isMarketplacePurchaseBill,
  isShopeePurchaseBill,
  isShopeeSalesBill,
  marketplaceCouponDiscountAmount,
  marketplaceFeeAmount,
  marketplacePaymentMethod,
  marketplaceReconciliationDelta,
  marketplaceReconciliationStatus,
  marketplaceServiceFeeAmount,
  marketplaceShippingAmount,
  marketplaceShippingMethod,
  money,
  shopeeCoinAmount,
  shopeeGoodsTotal,
  shopeePayableTotal,
} from '@/lib/shopeeBill'
import type { Bill, SMLReadiness } from '@/types'
import { issueLabel, type ValidationResult } from '../utils/validation'

interface Props {
  bill: Bill
  total: number
  retrying: boolean
  onRetry: () => void
  // Frontend-side validation against backend retry rules. When canSend=false
  // the Send button is disabled + a warning card lists the offending issues.
  // Each issue can be clicked to scroll/highlight the first row that hit it.
  validation: ValidationResult
  onJumpToItem: (itemId: string | null) => void
  // expectedRoute / expectedDocFormat — preview of what'll happen when admin
  // clicks Send. Surfaces the SML route + doc_no pattern BEFORE the round-trip
  // so admins can spot misconfigured channels (e.g. shopee bill routed to
  // sale_reserve because endpoint string doesn't match the keywords).
  expectedRoute?: string
  expectedEndpoint?: string
  expectedDocFormat?: string
  smlReadiness?: SMLReadiness | null
  smlReadinessLoading?: boolean
  canSendToSML?: boolean
}

const ROUTE_LABEL: Record<string, string> = {
  sale_reserve: 'ใบสั่งจอง',
  saleorder: 'ใบสั่งขาย',
  saleinvoice: 'ขาย -> ขายสินค้าและบริการ',
  purchaseorder: 'ซื้อ -> ใบสั่งซื้อ',
}

const ROUTE_SHORT_LABEL: Record<string, string> = {
  sale_reserve: 'ใบสั่งจอง',
  saleorder: 'ใบสั่งขาย',
  saleinvoice: 'ใบขาย',
  purchaseorder: 'ใบสั่งซื้อ',
}

export function BillTotal({
  bill,
  total,
  retrying,
  onRetry,
  validation,
  onJumpToItem,
  expectedRoute,
  expectedEndpoint,
  expectedDocFormat,
  smlReadiness,
  smlReadinessLoading = false,
  canSendToSML = true,
}: Props) {
  const canShowSendButton =
    canSendToSML &&
    (bill.status === 'failed' ||
      bill.status === 'pending' ||
      bill.status === 'needs_review')
  const isPurchase = bill.bill_type === 'purchase'
  const isShopeePurchase = isShopeePurchaseBill(bill)
  const isLazadaPurchase = isLazadaEmailPurchaseBill(bill)
  const isMarketplacePurchase = isMarketplacePurchaseBill(bill)
  const isShopeeSale = isShopeeSalesBill(bill)
  const isFailed = bill.status === 'failed'
  const goodsTotal = shopeeGoodsTotal(bill) ?? total
  const payableTotal = shopeePayableTotal(bill)
  const coinAmount = shopeeCoinAmount(bill)
  const shippingAmount = marketplaceShippingAmount(bill)
  const serviceFeeAmount = marketplaceServiceFeeAmount(bill)
  const couponDiscountAmount = marketplaceCouponDiscountAmount(bill)
  const feeAmount = marketplaceFeeAmount(bill)
  const paymentMethod = marketplacePaymentMethod(bill)
  const shippingMethod = marketplaceShippingMethod(bill)
  const reconciliationStatus = marketplaceReconciliationStatus(bill)
  const reconciliationDelta = marketplaceReconciliationDelta(bill)
  const hasMarketplaceFeeLine = (bill.items ?? []).some((item) => isMarketplaceFeeSourceSKU(item.source_sku))
  const missingLazadaFeeLine = isLazadaPurchase && feeAmount > 0 && !hasMarketplaceFeeLine
  const blockedByLazadaAmount = isLazadaPurchase && reconciliationStatus !== 'ok'

  // Send is enabled only when validation passes AND we're not mid-retry.
  // The disabled state is communicated by both the button's :disabled state
  // and the warning card above (which is the "why" — the button alone
  // wouldn't tell the admin what to fix).
  const totalDiscount = isMarketplacePurchase
    ? (bill.items ?? []).reduce((s, i) => s + (i.discount_amount ?? 0), 0)
    : 0

  const smlReady = isSMLReady(smlReadiness)
  const enabled = validation.canSend && smlReady && !retrying && !missingLazadaFeeLine && !blockedByLazadaAmount
  const readyText = missingLazadaFeeLine
    ? 'ยังส่งไม่ได้ — ต้องตั้งค่าสินค้า SML สำหรับค่าจัดส่ง/fee Lazada หรือเติมรายการ fee ให้ครบก่อน'
    : blockedByLazadaAmount
    ? 'ยังส่งไม่ได้ — ยอด Lazada ยัง reconcile ไม่ผ่าน กรุณาตรวจสรุปยอดจากอีเมล'
    : !smlReady
    ? (smlReadinessLoading ? 'กำลังตรวจสถานะ SML ของร้านนี้' : 'SML ของร้านนี้ยังไม่พร้อม กรุณาตรวจการเชื่อมต่อก่อนส่ง')
    : validation.canSend
    ? 'รายการครบแล้ว พร้อมเลือกผู้ขาย/คลัง/ภาษีและส่งเข้า SML'
    : `ยังต้องแก้ ${validation.issues.length} จุดก่อนส่งเข้า SML`

  const routeShortLabel = expectedRoute ? (ROUTE_SHORT_LABEL[expectedRoute] ?? expectedRoute) : (isPurchase ? 'ใบสั่งซื้อ' : '')
  const buttonLabel = retrying
    ? 'กำลังส่ง...'
    : isFailed
      ? `ลองส่งใหม่${routeShortLabel ? ` (${routeShortLabel})` : ''}`
      : `ส่งเข้า SML${routeShortLabel ? ` (${routeShortLabel})` : ''}`

  return (
    <Card className="rounded-xl border-border/70 shadow-sm">
      <CardContent className="space-y-2.5 px-5 py-3">
        {/* Top row — total + send button */}
        <div className="flex flex-wrap items-center justify-between gap-4">
          <div>
            <div className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
              {isMarketplacePurchase || isShopeeSale ? 'ยอดที่ต้องชำระทั้งหมด' : 'ยอดรวมทั้งหมด'}
            </div>
            <div className="mt-0.5 text-xl font-bold tabular-nums tracking-tight">
              {money(isMarketplacePurchase || isShopeeSale ? payableTotal ?? total : total)}
            </div>
            {isShopeePurchase && (
              <div className="mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-[11px] text-muted-foreground">
                <span>ยอดสินค้า {money(goodsTotal)}</span>
                {totalDiscount > 0 && (
                  <span className="text-success">
                    ส่วนลด -{money(totalDiscount)}
                    {coinAmount != null && coinAmount > 0 && (
                      <span className="text-info"> (รวม Coin {money(coinAmount)})</span>
                    )}
                  </span>
                )}
                {payableTotal != null && payableTotal !== goodsTotal && (
                  <span>รวมค่าส่งแล้ว</span>
                )}
              </div>
            )}
            {isLazadaPurchase && (
              <div className="mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-[11px] text-muted-foreground">
                <span>ยอดสินค้า {money(goodsTotal)}</span>
                <span>ค่าส่ง {money(shippingAmount)}</span>
                {(serviceFeeAmount ?? 0) > 0 && (
                  <span>service fee {money(serviceFeeAmount)}</span>
                )}
                {(couponDiscountAmount ?? 0) > 0 && (
                  <span className="text-success">คูปอง -{money(couponDiscountAmount)}</span>
                )}
                <span className={reconciliationStatus === 'ok' ? 'text-success' : 'text-warning'}>
                  reconcile {reconciliationStatus || 'missing'}
                  {reconciliationDelta != null && reconciliationDelta !== 0 && ` (${money(reconciliationDelta)})`}
                </span>
                {paymentMethod && <span>ชำระเงิน: {paymentMethod}</span>}
                {shippingMethod && <span>จัดส่ง: {shippingMethod}</span>}
              </div>
            )}
            {canShowSendButton && (
              <p className={cn(
                'mt-0.5 text-xs',
                validation.canSend && smlReady ? 'text-success' : 'text-warning',
              )}>
                {readyText}
              </p>
            )}
          </div>

          {canShowSendButton && (
            <div className="flex flex-col items-end gap-1.5">
              <TooltipProvider delayDuration={150}>
                <Tooltip>
                  <TooltipTrigger asChild>
                    {/* Wrap button in a span so a disabled button still
                        receives hover events (raw <button disabled> swallows
                        them, which means the tooltip wouldn't fire on the
                        very state we most need to explain). */}
                    <span className={!enabled ? 'cursor-not-allowed' : ''}>
                      <Button
                        type="button"
                        onClick={onRetry}
                        disabled={!enabled}
                        variant={isFailed ? 'destructive' : 'default'}
                        className="h-10 shrink-0 gap-2 rounded-lg px-4"
                      >
                        {retrying ? (
                          <RefreshCw className="h-4 w-4 animate-spin" />
                        ) : isFailed ? (
                          <RefreshCw className="h-4 w-4" />
                        ) : (
                          <Send className="h-4 w-4" />
                        )}
                        {buttonLabel}
                      </Button>
                    </span>
                  </TooltipTrigger>
                  {/* Tooltip only renders content when the button is disabled
                      because of validation — when retrying, the button text
                      already explains itself ("กำลังส่ง..."). */}
                  {(!validation.canSend || !smlReady || missingLazadaFeeLine || blockedByLazadaAmount) && (
                    <TooltipContent side="left" className="max-w-xs">
                      {missingLazadaFeeLine || blockedByLazadaAmount
                        ? readyText
                        : !smlReady
                        ? smlBlockedMessage(smlReadiness)
                        : `ยังส่งไม่ได้ — พบ ${validation.issues.length} ปัญหา · ตรวจรหัสสินค้า การยืนยัน หน่วย จำนวน และราคา`}
                    </TooltipContent>
                  )}
                </Tooltip>
              </TooltipProvider>

              {/* Route preview — always visible when send area is shown so
                  admin can see the routing even before validation passes.
                  Dimmed when button is disabled to signal "preview only". */}
              {canShowSendButton && expectedRoute && (
                <div className={cn("max-w-[340px] text-right text-[10px] leading-4 tabular-nums text-muted-foreground", !enabled && "opacity-50")}>
                  ปลายทาง SML:{' '}
                  <span className="font-medium text-foreground">
                    {ROUTE_LABEL[expectedRoute] ?? expectedRoute}
                  </span>
                  {expectedDocFormat && (
                    <>
                      {' '}· รหัสเอกสาร{' '}
                      <code className="rounded bg-muted px-1 py-0.5 font-mono">
                        {expectedDocFormat}
                      </code>
                    </>
                  )}
                  {expectedEndpoint && expectedEndpoint.startsWith('http') && (
                    <div
                      className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground/70"
                      title={expectedEndpoint}
                    >
                      {expectedEndpoint}
                    </div>
                  )}
                </div>
              )}
            </div>
          )}
        </div>

        {/* Validation warning card — only renders when there are issues to
            fix. Each issue links to the first offending row. Sits between
            the total + button summary and the items table so admin sees
            "what to do" before they look down at items. */}
        {canShowSendButton && !smlReady && (
          <div className="rounded-md border border-warning/40 bg-warning/[0.07] px-3 py-2">
            <div className="flex items-start gap-2">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" strokeWidth={2.25} />
              <div className="min-w-0 flex-1">
                <div className="text-sm font-semibold text-foreground">ยังส่ง SML ไม่ได้ — ฐานข้อมูลร้านยังไม่พร้อม</div>
                <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">
                  {smlBlockedMessage(smlReadiness)} เปิดเครื่อง SML/Postgres ของร้านนี้ แล้วกดตรวจอีกครั้งบนแถบแจ้งเตือนด้านบน
                </p>
              </div>
            </div>
          </div>
        )}

        {canShowSendButton && !validation.canSend && (
          <div
            className={cn(
              'rounded-md border border-warning/40 bg-warning/[0.06] px-3 py-2',
            )}
          >
            <div className="flex items-start gap-2">
              <AlertTriangle
                className="mt-0.5 h-4 w-4 shrink-0 text-warning"
                strokeWidth={2.25}
              />
              <div className="min-w-0 flex-1 space-y-1.5">
                <div className="text-sm font-semibold text-foreground">
                  ยังส่ง SML ไม่ได้ — พบ {validation.issues.length}{' '}
                  ปัญหาที่ต้องแก้
                </div>
                <ul className="space-y-0.5 text-[13px]">
                  {validation.issues.map((issue) => (
                    <li
                      key={issue.kind}
                      className="flex items-baseline gap-1.5"
                    >
                      <span className="text-muted-foreground/60">•</span>
                      <span className="flex-1 text-foreground">
                        <span className="font-medium tabular-nums">
                          {issue.count}
                        </span>{' '}
                        {issue.kind === 'no_items'
                          ? issueLabel(issue.kind)
                          : `รายการ${issueLabel(issue.kind)}`}
                      </span>
                      {issue.firstItemId && (
                        <button
                          type="button"
                          onClick={() => onJumpToItem(issue.firstItemId)}
                          className="shrink-0 rounded-md bg-primary/10 px-2 py-1 text-[11px] font-medium text-primary hover:bg-primary/15"
                        >
                          ไปแก้รายการ
                        </button>
                      )}
                    </li>
                  ))}
                </ul>
              </div>
            </div>
          </div>
        )}
        {canShowSendButton && (missingLazadaFeeLine || blockedByLazadaAmount) && (
          <div className="rounded-md border border-warning/40 bg-warning/[0.07] px-3 py-2">
            <div className="flex items-start gap-2">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" strokeWidth={2.25} />
              <div className="min-w-0 flex-1">
                <div className="text-sm font-semibold text-foreground">ยังส่ง Lazada เข้า SML ไม่ได้</div>
                <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">
                  {missingLazadaFeeLine
                    ? `ยอดอีเมลมีค่าจัดส่ง/fee ${money(feeAmount)} แต่ยังไม่มีรายการสินค้า SML สำหรับ fee line`
                    : `สูตรยอด Lazada ยังไม่ผ่าน (${reconciliationStatus || 'missing'}${reconciliationDelta != null ? `, delta ${money(reconciliationDelta)}` : ''})`}
                </p>
              </div>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
