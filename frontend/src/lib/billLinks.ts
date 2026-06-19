import type { Bill } from '@/types'
import { isMarketplacePurchaseBill, rawString } from './shopeeBill'

export function billDetailPath(bill: Pick<Bill, 'id' | 'bill_type' | 'document_route'>): string {
  if (bill.bill_type === 'sale') {
    return bill.document_route === 'saleinvoice'
      ? `/sale-invoices/${bill.id}`
      : `/sales-orders/${bill.id}`
  }
  return `/bills/${bill.id}`
}

export function billDetailURL(bill: Pick<Bill, 'id' | 'bill_type' | 'document_route'>): string {
  return `${window.location.origin}${billDetailPath(bill)}`
}

export function marketplaceOrderURL(
  bill: Pick<Bill, 'source' | 'bill_type' | 'raw_data'>,
): string {
  if (!isMarketplacePurchaseBill(bill)) return ''
  return rawString(bill.raw_data, 'marketplace_order_url')
}

export function copyURLForBill(
  bill: Pick<Bill, 'id' | 'bill_type' | 'document_route' | 'source' | 'raw_data'>,
): { url: string; kind: 'marketplace' | 'billflow' } {
  const externalURL = marketplaceOrderURL(bill)
  if (externalURL) return { url: externalURL, kind: 'marketplace' }
  return { url: billDetailURL(bill), kind: 'billflow' }
}
