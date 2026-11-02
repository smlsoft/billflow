import type { Bill } from '@/types'

export interface PaymentMethodChannel {
  channel: string
  bill_type: string
  print_policy?: { payment_methods?: string[] } | null
}

export function billPaymentMethods(
  rows: readonly PaymentMethodChannel[] | null,
  source: string,
  bills: readonly Pick<Bill, 'effective_print_payment_method' | 'print_payment_method'>[],
  selected: string,
  fallback: readonly string[],
): string[] {
  const methods = new Set<string>()
  const add = (values: readonly string[]) => values.forEach((value) => {
    const method = value.trim()
    if (method) methods.add(method)
  })
  const policies = (rows ?? []).filter((row) =>
    row.bill_type === 'purchase'
    && (row.channel === 'shopee_shipped' || row.channel === 'lazada_email')
    && (!source || row.channel === source),
  )
  if (policies.length === 0) add(fallback)
  policies.forEach((row) => {
    const configured = row.print_policy?.payment_methods
    const valid = Array.isArray(configured) ? configured.filter((value) => typeof value === 'string' && value.trim()) : []
    add(valid.length ? valid : fallback)
  })
  bills.forEach((bill) => {
    const method = (bill.effective_print_payment_method || bill.print_payment_method || '').trim()
    if (method) methods.add(method)
  })
  if (selected) methods.add(selected)
  return Array.from(methods)
}
