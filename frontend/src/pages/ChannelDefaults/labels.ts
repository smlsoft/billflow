// Shared types + labels for the /settings/channels page.
// Channel × bill_type keys are the contract with the backend
// (channel_defaults table CHECK constraint).

export type ChannelKey =
  | 'line'
  | 'email'
  | 'shopee'
  | 'shopee_email'
  | 'shopee_shipped'
  | 'lazada'
  | 'lazada_email'
  | 'tiktok'
  | 'manual'
  | 'shopee_settlement'

export type ChannelBillType = 'sale' | 'purchase' | 'ar_receipt'

export interface MarketplacePrintPolicy {
  requires_all_orders_sml_doc: boolean
  payment_method_prefix_enabled: boolean
  payment_method_prefixes: string[]
  payment_methods: string[]
}

export interface ChannelDefaultRow {
  channel: string
  bill_type: ChannelBillType
  party_code: string
  party_name: string
  party_phone: string
  party_address: string
  party_tax_id: string
  doc_format_code: string
  endpoint: string  // tested API path selected from SML_DESTINATION_OPTIONS
  doc_prefix: string         // e.g. "BF-SO"
  doc_running_format: string // e.g. "YYMM####"
  branch_code: string
  sale_code: string
  unit_code: string
  doc_time: string
  shipping_item_enabled?: boolean
  shipping_item_code?: string
  shipping_item_unit_code?: string
  passbook_code?: string
  passbook_name?: string
  bank_code?: string
  bank_branch?: string
  expense_code?: string
  expense_name?: string
  // Inventory + VAT overrides (sentinel: '' / -1 = "use server default")
  wh_code: string
  shelf_code: string
  vat_type: number      // -1 = use default; 0=แยกนอก, 1=รวมใน, 2=ศูนย์%
  vat_rate: number      // -1 = use default; else percent (e.g. 7)
  inquiry_type: number  // -1 = use default; 0-3 = ประเภทรายการ
  remark_2?: string     // '' = ไม่ระบุ; 'tax'|'notax'|'re'
  print_policy?: MarketplacePrintPolicy
  updated_by?: string | null
  updated_at?: string
}

export function supportsMarketplacePrintPolicy(row: Pick<ChannelDefaultRow, 'channel' | 'bill_type'>): boolean {
  return row.bill_type === 'purchase' && (row.channel === 'shopee_shipped' || row.channel === 'lazada_email')
}

export const DEFAULT_MARKETPLACE_PRINT_PAYMENT_METHODS = [
  'TT2789',
  'TT9630',
  'TT0972',
  'TT9628',
  'TT5128',
  'TT5432',
  'TT3086',
  'TT8456',
  'โอน Kbank',
  'โอน TTB5074',
  'โอน KTB',
  'โอน BBL',
  'โอน TTB1135',
]

export function defaultMarketplacePrintPolicy(): MarketplacePrintPolicy {
  return {
    requires_all_orders_sml_doc: true,
    payment_method_prefix_enabled: true,
    payment_method_prefixes: ['TT'],
    payment_methods: DEFAULT_MARKETPLACE_PRINT_PAYMENT_METHODS,
  }
}

export function normalizeMarketplacePrintPolicy(policy?: Partial<MarketplacePrintPolicy> | null): MarketplacePrintPolicy {
  const fallback = defaultMarketplacePrintPolicy()
  const prefixes = Array.isArray(policy?.payment_method_prefixes)
    ? Array.from(new Set(policy.payment_method_prefixes.map((p) => String(p).trim().toUpperCase()).filter(Boolean)))
    : fallback.payment_method_prefixes
  const methods = Array.isArray(policy?.payment_methods)
    ? Array.from(new Set(policy.payment_methods.map((p) => String(p).trim()).filter(Boolean)))
    : fallback.payment_methods
  return {
    requires_all_orders_sml_doc: typeof policy?.requires_all_orders_sml_doc === 'boolean'
      ? policy.requires_all_orders_sml_doc
      : fallback.requires_all_orders_sml_doc,
    payment_method_prefix_enabled: typeof policy?.payment_method_prefix_enabled === 'boolean'
      ? policy.payment_method_prefix_enabled
      : fallback.payment_method_prefix_enabled,
    payment_method_prefixes: prefixes.length > 0 ? prefixes : fallback.payment_method_prefixes,
    payment_methods: methods.length > 0 ? methods : fallback.payment_methods,
  }
}

export function marketplacePrintPolicyNote(policy?: Partial<MarketplacePrintPolicy> | null): string {
  const p = normalizeMarketplacePrintPolicy(policy)
  const parts: string[] = []
  if (p.requires_all_orders_sml_doc) {
    parts.push('ส่งเข้า SML ครบทุกคำสั่งซื้อในอีเมลเดียวกัน')
  }
  if (p.payment_method_prefix_enabled) {
    parts.push(`วิธีการชำระเงินขึ้นต้นด้วย ${p.payment_method_prefixes.join(', ')}`)
  }
  return parts.length > 0 ? `พร้อมปริ้น = ${parts.join(' และ ')}` : 'พร้อมปริ้นตามเงื่อนไขเอกสารที่ส่งเข้า SML แล้ว'
}

// previewDocNo renders a sample doc_no with seq=1 — mirrors the backend
// repository.GenerateDocNo logic (kept in sync; do not diverge).
export function previewDocNo(prefix: string, format: string, now = new Date()): string {
  if (!prefix) prefix = 'BF'
  if (!format) format = 'YYMM####'
  const yyyy = String(now.getFullYear())
  const yy = String(now.getFullYear() % 100).padStart(2, '0')
  const mm = String(now.getMonth() + 1).padStart(2, '0')
  const dd = String(now.getDate()).padStart(2, '0')
  let out = format
    .replace(/YYYY/g, yyyy)
    .replace(/YY/g, yy)
    .replace(/MM/g, mm)
    .replace(/DD/g, dd)
  out = out.replace(/#+/, (m: string) => '1'.padStart(m.length, '0'))
  return prefix + out
}

// docNoPatternWarning checks if the chosen prefix+format combo will hit the
// SML bug we discovered (UI silently drops docs whose doc_no contains a
// `-YYYY` or `-YYMM` segment after a hyphen). Returns warning text or ''.
export function docNoPatternWarning(prefix: string, format: string): string {
  if (!prefix || !format) return ''
  // Failure pattern: prefix ends with '-' AND format starts with YY/YYYY
  if (prefix.endsWith('-') && (format.startsWith('YYYY') || format.startsWith('YY'))) {
    return 'รูปแบบนี้อาจถูก SML ปฏิเสธ — เคยพบ bug ที่ doc_no มี "-YYMM" หรือ "-YYYYMM" ตามหลังเครื่องหมาย "-" จะ save ผ่านแต่กดดูไม่ได้. แนะนำให้ลบ "-" ท้าย prefix ออก (เช่น "BF-SO" + "YYMM####" → "BF-SO260400001")'
  }
  return ''
}

export type EndpointKind =
  | 'saleorder'
  | 'saleinvoice'
  | 'purchaseorder'
  | 'arreceipt'

export interface SmlDestinationOption {
  value: EndpointKind
  billType: ChannelBillType
  label: string
  apiPath: string
  docFormatCode: string
  docPrefix: string
  docRunningFormat: string
  statusLabel: string
  description: string
  phase1Enabled: boolean
}

// Business-level SML destinations. The backend still stores endpoint +
// doc_format_code, but admins pick from tested destinations instead of typing
// raw paths. Add new rows here only after that SML menu has been tested.
export const SML_DESTINATION_OPTIONS: SmlDestinationOption[] = [
  {
    value: 'saleorder',
    billType: 'sale',
    label: 'ขาย -> ใบสั่งขาย',
    apiPath: '/api/v1/ic/sale-orders',
    docFormatCode: 'SR',
    docPrefix: 'BF-SO',
    docRunningFormat: 'YYMM####',
    statusLabel: 'ทดสอบผ่านแล้ว',
    description: 'ส่งบิลขาย Marketplace Excel เข้าเมนู ขาย -> ใบสั่งขาย ใน SML',
    phase1Enabled: true,
  },
  {
    value: 'saleinvoice',
    billType: 'sale',
    label: 'ขาย -> ขายสินค้าและบริการ',
    apiPath: '/api/v1/ic/sale-invoices',
    docFormatCode: 'SI',
    docPrefix: 'BF-INV',
    docRunningFormat: 'YYMM####',
    statusLabel: 'ทดสอบผ่านแล้ว',
    description: 'ส่งบิลขาย Marketplace Excel เข้าเมนู ขาย -> ขายสินค้าและบริการ ใน SML',
    phase1Enabled: true,
  },
  {
    value: 'purchaseorder',
    billType: 'purchase',
    label: 'ซื้อ -> ใบสั่งซื้อ',
    apiPath: '/api/v1/ic/purchase-orders',
    docFormatCode: 'PO',
    docPrefix: 'BF-PO',
    docRunningFormat: 'YYMM####',
    statusLabel: 'ทดสอบผ่านแล้ว',
    description: 'ส่งบิลซื้อจากอีเมล Marketplace เข้าเมนู ซื้อ -> ใบสั่งซื้อ ใน SML',
    phase1Enabled: true,
  },
  {
    value: 'arreceipt',
    billType: 'ar_receipt',
    label: 'ลูกหนี้ -> รับชำระหนี้',
    apiPath: '/api/v1/ar/receipts',
    docFormatCode: 'RC',
    docPrefix: 'RC',
    docRunningFormat: '@YYMM####',
    statusLabel: 'ทดสอบผ่านแล้ว',
    description: 'ส่งรายการ Shopee payout เข้าเมนู ลูกหนี้ -> รับชำระหนี้ ใน SML',
    phase1Enabled: true,
  },
]

export function destinationFor(
  channel: ChannelKey,
  billType: ChannelBillType,
  endpoint = '',
  docFormatCode = '',
): SmlDestinationOption | undefined {
  const kind = destinationKindFor(endpoint, channel, billType)
  return SML_DESTINATION_OPTIONS.find((option) => {
    if (option.value !== kind) return false
    if (!docFormatCode) return true
    return option.docFormatCode.toLowerCase() === docFormatCode.toLowerCase()
  }) ?? SML_DESTINATION_OPTIONS.find((option) => option.value === kind)
}

export function destinationOptionsFor(
  billType?: ChannelBillType,
): SmlDestinationOption[] {
  return SML_DESTINATION_OPTIONS.filter((option) => (
    option.phase1Enabled && (!billType || option.billType === billType)
  ))
}

// destinationKindFor resolves existing backend rows to one of the tested
// dropdown destinations. Empty / unknown endpoints fall back to the default
// destination for the channel and bill type.
export function destinationKindFor(
  override: string,
  channel: ChannelKey,
  billType: ChannelBillType,
): EndpointKind {
  const lower = (override || '').toLowerCase()
  if (channel === 'shopee_settlement' || billType === 'ar_receipt' || lower.includes('ar/receipts')) return 'arreceipt'
  if (lower.includes('purchaseorder') || lower.includes('purchase-orders')) return 'purchaseorder'
  if (lower.includes('saleinvoice') || lower.includes('sale-invoices')) return 'saleinvoice'
  if (lower.includes('saleorder') || lower.includes('sale-orders')) return 'saleorder'
  // No keyword match → default by channel+bill_type
  if (channel === 'shopee_shipped' || channel === 'lazada_email' || billType === 'purchase') return 'purchaseorder'
  if (channel === 'shopee' || channel === 'shopee_email' || channel === 'lazada' || channel === 'tiktok') return 'saleorder'
  return 'saleorder'
}

export const CHANNEL_LABELS: Record<ChannelKey, string> = {
  line: 'LINE OA',
  email: 'Email',
  shopee: 'Shopee',
  shopee_email: 'Shopee Order',
  shopee_shipped: 'Email บิลซื้อ Shopee',
  lazada: 'Lazada Excel',
  lazada_email: 'Email บิลซื้อ Lazada',
  tiktok: 'TikTok Excel',
  manual: 'Manual',
  shopee_settlement: 'Shopee รับชำระหนี้',
}
