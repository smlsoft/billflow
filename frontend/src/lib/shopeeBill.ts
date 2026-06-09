import type { Bill } from '@/types'

export const SHOPEE_SHIPPING_SOURCE_SKU = '__shopee_shipping__'
export const LAZADA_FEE_SOURCE_SKU = '__lazada_shipping_fee__'

export function rawString(raw: Record<string, unknown> | null | undefined, key: string): string {
  const value = raw?.[key]
  return typeof value === 'string' ? value : ''
}

export function rawNumber(raw: Record<string, unknown> | null | undefined, key: string): number | null {
  const value = raw?.[key]
  if (typeof value === 'number' && Number.isFinite(value)) return value
  if (typeof value === 'string') {
    const parsed = Number(value.replace(/,/g, ''))
    return Number.isFinite(parsed) ? parsed : null
  }
  return null
}

export function isShopeePurchaseBill(bill: Pick<Bill, 'source' | 'bill_type'>): boolean {
  return bill.source === 'shopee_shipped' && bill.bill_type === 'purchase'
}

export function isLazadaEmailPurchaseBill(bill: Pick<Bill, 'source' | 'bill_type'>): boolean {
  return bill.source === 'lazada_email' && bill.bill_type === 'purchase'
}

export function isMarketplacePurchaseBill(bill: Pick<Bill, 'source' | 'bill_type'>): boolean {
  return isShopeePurchaseBill(bill) || isLazadaEmailPurchaseBill(bill)
}

export function isShopeeSalesBill(bill: Pick<Bill, 'source' | 'bill_type'>): boolean {
  return (bill.source === 'shopee' || bill.source === 'lazada' || bill.source === 'tiktok') && bill.bill_type === 'sale'
}

export function isMarketplaceFeeSourceSKU(sourceSKU?: string | null): boolean {
  return sourceSKU === SHOPEE_SHIPPING_SOURCE_SKU || sourceSKU === LAZADA_FEE_SOURCE_SKU
}

export function shopeeOrderID(raw: Record<string, unknown> | null | undefined): string {
  return rawString(raw, 'order_id') || rawString(raw, 'shopee_order_id') || rawString(raw, 'lazada_order_id') || rawString(raw, 'tiktok_order_id')
}

export function shopeePayableTotal(bill: Bill): number | null {
  if (!isMarketplacePurchaseBill(bill) && !isShopeeSalesBill(bill)) return null
  return rawNumber(bill.raw_data, 'paid_total_amount')
}

export function shopeeGoodsTotal(bill: Bill): number | null {
  if (!isMarketplacePurchaseBill(bill)) return null
  return rawNumber(bill.raw_data, 'goods_total_amount')
}

export function shopeeCoinAmount(bill: Bill): number | null {
  if (!isShopeePurchaseBill(bill)) return null
  return rawNumber(bill.raw_data, 'shopee_coin_amount')
}

export function marketplaceShippingAmount(bill: Bill): number | null {
  if (!isMarketplacePurchaseBill(bill)) return null
  return rawNumber(bill.raw_data, 'shipping_amount')
}

export function marketplaceServiceFeeAmount(bill: Bill): number | null {
  if (!isLazadaEmailPurchaseBill(bill)) return null
  return rawNumber(bill.raw_data, 'service_fee_amount')
}

export function marketplaceCouponDiscountAmount(bill: Bill): number | null {
  if (!isMarketplacePurchaseBill(bill)) return null
  const direct = rawNumber(bill.raw_data, 'coupon_discount_amount')
  if (direct != null) return direct
  const summary = bill.raw_data?.discount_summary
  if (summary && typeof summary === 'object' && !Array.isArray(summary)) {
    const total = rawNumber(summary as Record<string, unknown>, 'total_discount_amount')
    if (total != null) return total
  }
  return null
}

export function marketplaceFeeAmount(bill: Bill): number {
  const shipping = marketplaceShippingAmount(bill) ?? 0
  const service = marketplaceServiceFeeAmount(bill) ?? 0
  return Math.round((shipping + service) * 100) / 100
}

export function marketplacePaymentMethod(bill: Bill): string {
  return rawString(bill.raw_data, 'payment_method')
}

export function marketplaceShippingMethod(bill: Bill): string {
  return rawString(bill.raw_data, 'shipping_method')
}

export function marketplaceReconciliationStatus(bill: Bill): string {
  return rawString(bill.raw_data, 'amount_reconciliation_status')
}

export function marketplaceReconciliationDelta(bill: Bill): number | null {
  return rawNumber(bill.raw_data, 'amount_reconciliation_delta')
}

export function money(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '—'
  return `฿${value.toLocaleString('th-TH')}`
}
