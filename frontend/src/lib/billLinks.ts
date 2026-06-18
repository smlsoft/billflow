import type { Bill } from '@/types'

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
