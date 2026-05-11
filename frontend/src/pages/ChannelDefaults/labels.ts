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
  | 'tiktok'
  | 'manual'

export interface ChannelDefaultRow {
  channel: string
  bill_type: 'sale' | 'purchase'
  party_code: string
  party_name: string
  party_phone: string
  party_address: string
  party_tax_id: string
  doc_format_code: string
  endpoint: string  // '' | URL or path; backend keyword-detects which client to use
  doc_prefix: string         // e.g. "BF-SO"
  doc_running_format: string // e.g. "YYMM####"
  branch_code: string
  sale_code: string
  unit_code: string
  doc_time: string
  // Inventory + VAT overrides (sentinel: '' / -1 = "use server default")
  wh_code: string
  shelf_code: string
  vat_type: number   // -1 = use default; 0=แยกนอก, 1=รวมใน, 2=ศูนย์%
  vat_rate: number   // -1 = use default; else percent (e.g. 7)
  updated_by?: string | null
  updated_at?: string
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
  | ''  // auto by (channel, bill_type)
  | 'saleorder'
  | 'saleinvoice'
  | 'purchaseorder'
  | 'sale_reserve'

export const ENDPOINT_OPTIONS: Array<{
  value: EndpointKind
  label: string
  apiPath: string
  takesDocFormat: boolean
  docFormatHint: string
  description: string
}> = [
  {
    value: 'saleorder',
    label: 'ขาย -> ใบสั่งขาย',
    apiPath: '/SMLJavaRESTService/v3/api/saleorder',
    takesDocFormat: true,
    docFormatHint: 'SR',
    description: 'SML 248 — สำหรับบิลขาย Marketplace Excel → เก็บที่เมนู "ใบสั่งขาย" ใน SML',
  },
  {
    value: 'saleinvoice',
    label: 'ขาย -> ขายสินค้าและบริการ',
    apiPath: '/SMLJavaRESTService/saleinvoice/v4',
    takesDocFormat: true,
    docFormatHint: 'SI',
    description: 'SML 248 — สำหรับบิลขาย Marketplace Excel → เก็บที่เมนู "ขายสินค้าและบริการ" ใน SML',
  },
  {
    value: 'purchaseorder',
    label: 'ซื้อ -> ใบสั่งซื้อ',
    apiPath: '/SMLJavaRESTService/v3/api/purchaseorder',
    takesDocFormat: true,
    docFormatHint: 'PO',
    description: 'SML 248 — สำหรับบิลซื้อ (Shopee shipped/pay-now) → เมนู "ใบสั่งซื้อ"',
  },
  {
    value: 'sale_reserve',
    label: 'ใบจอง (sale_reserve)',
    apiPath: '/api/sale_reserve',
    takesDocFormat: false,
    docFormatHint: '',
    description: 'SML 213 — JSON-RPC สำหรับ LINE OA / Email → เมนู "ใบจอง" ใน SML',
  },
]

// EndpointInfo describes which SML API a (channel, bill_type) bill posts to.
// Used as read-only metadata in the UI so admins can see the routing without
// digging into code.
export interface EndpointInfo {
  label: string         // ใบสั่งขาย / ซื้อ -> ใบสั่งซื้อ / ใบจอง
  apiPath: string       // /SMLJavaRESTService/v3/api/saleorder
  takesDocFormat: boolean
  docFormatHint: string // suggested values, e.g. "SR / RU"
}

export interface SmlDestinationOption {
  value: Exclude<EndpointKind, ''>
  billType: 'sale' | 'purchase'
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
    apiPath: '/SMLJavaRESTService/v3/api/saleorder',
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
    apiPath: '/SMLJavaRESTService/saleinvoice/v4',
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
    apiPath: '/SMLJavaRESTService/v3/api/purchaseorder',
    docFormatCode: 'PO',
    docPrefix: 'BF-PO',
    docRunningFormat: 'YYMM####',
    statusLabel: 'ทดสอบผ่านแล้ว',
    description: 'ส่งบิลซื้อ Shopee เข้าเมนู ซื้อ -> ใบสั่งซื้อ ใน SML',
    phase1Enabled: true,
  },
]

export function destinationFor(
  channel: ChannelKey,
  billType: 'sale' | 'purchase',
  endpoint = '',
  docFormatCode = '',
): SmlDestinationOption | undefined {
  const kind = resolveEndpointKind(endpoint, channel, billType)
  return SML_DESTINATION_OPTIONS.find((option) => {
    if (option.value !== kind) return false
    if (!docFormatCode) return true
    return option.docFormatCode.toLowerCase() === docFormatCode.toLowerCase()
  }) ?? SML_DESTINATION_OPTIONS.find((option) => option.value === kind)
}

export function phase1DestinationOptions(
  billType?: 'sale' | 'purchase',
): SmlDestinationOption[] {
  return SML_DESTINATION_OPTIONS.filter((option) => (
    option.phase1Enabled && (!billType || option.billType === billType)
  ))
}

// resolveEndpointKind mirrors resolveEndpoint() in handlers/bills.go —
// detects which SML client a free-form URL/path will dispatch to by keyword
// match. Empty / no-match → fall back to channel+bill_type default.
export function resolveEndpointKind(
  override: string,
  channel: ChannelKey,
  billType: 'sale' | 'purchase',
): Exclude<EndpointKind, ''> {
  const lower = (override || '').toLowerCase()
  if (lower.includes('purchaseorder')) return 'purchaseorder'
  if (lower.includes('saleinvoice')) return 'saleinvoice'
  if (lower.includes('saleorder')) return 'saleorder'
  if (lower.includes('sale_reserve')) return 'sale_reserve'
  // No keyword match → default by channel+bill_type
  if (channel === 'shopee_shipped' || billType === 'purchase') return 'purchaseorder'
  if (channel === 'shopee' || channel === 'shopee_email' || channel === 'lazada' || channel === 'tiktok') return 'saleorder'
  return 'sale_reserve'
}

// endpointFor returns the resolved SML routing metadata for display.
export function endpointFor(
  channel: ChannelKey,
  billType: 'sale' | 'purchase',
  override = '',
): EndpointInfo {
  const kind = resolveEndpointKind(override, channel, billType)
  const opt = ENDPOINT_OPTIONS.find((o) => o.value === kind)
  if (!opt) {
    return {
      label: 'sale_reserve',
      apiPath: '/api/sale_reserve',
      takesDocFormat: false,
      docFormatHint: '',
    }
  }
  return {
    label: opt.label,
    apiPath: opt.apiPath,
    takesDocFormat: opt.takesDocFormat,
    docFormatHint: opt.docFormatHint,
  }
}

export const CHANNEL_LABELS: Record<ChannelKey, string> = {
  line: 'LINE OA',
  email: 'Email',
  shopee: 'Shopee Excel',
  shopee_email: 'Shopee Order',
  shopee_shipped: 'Email บิลซื้อ Shopee',
  lazada: 'Lazada Excel',
  tiktok: 'TikTok Excel',
  manual: 'Manual',
}

// All (channel, bill_type) combos the backend accepts. Order matters —
// this is the order rows appear in the table when there's no DB row yet.
export const CHANNEL_SLOTS: Array<{
  channel: ChannelKey
  bill_type: 'sale' | 'purchase'
}> = [
  { channel: 'line', bill_type: 'sale' },
  { channel: 'email', bill_type: 'sale' },
  { channel: 'email', bill_type: 'purchase' },
  { channel: 'shopee', bill_type: 'sale' },
  { channel: 'shopee_email', bill_type: 'sale' },
  { channel: 'shopee_shipped', bill_type: 'purchase' },
  { channel: 'lazada', bill_type: 'sale' },
  { channel: 'lazada', bill_type: 'purchase' },
  { channel: 'tiktok', bill_type: 'sale' },
]

export function channelHelp(channel: ChannelKey, isPurchase: boolean): string {
  switch (channel) {
    case 'line':
      return 'ทุกบิลที่เข้ามาทางช่องทางนี้จะใช้ชื่อลูกค้านี้เสมอ — กันไม่ให้ SML สร้างลูกค้าใหม่ทุกครั้งที่ลูกค้าทักเข้ามา'
    case 'email':
      return isPurchase
        ? 'Phase 1 ใช้แถวนี้สำหรับ Email → บิลซื้อ: เลือกผู้ขาย default ที่จะส่งเข้า SML purchaseorder'
        : 'ทุกบิลขายที่เข้าทาง Email จะใช้ชื่อลูกค้านี้เสมอ'
    case 'shopee':
    case 'shopee_email':
      return 'รหัสลูกค้านี้จะถูกส่งเป็น cust_code ตอนสร้างใบสั่งขาย (saleorder) ใน SML'
    case 'shopee_shipped':
      return 'Email บิลซื้อ Shopee จะส่งเข้า SML ตามปลายทางที่เลือก ส่วนผู้ขายจริงเลือกตอนส่งบิลแต่ละใบ'
    case 'lazada':
      return isPurchase
        ? 'ใช้รหัสนี้กับบิลซื้อ Lazada (Phase 4b)'
        : 'รหัสลูกค้านี้จะถูกส่งเป็น cust_code ตอนสร้างใบสั่งขาย Lazada ใน SML'
    case 'tiktok':
      return 'รหัสลูกค้านี้จะถูกส่งเป็น cust_code ตอนสร้างใบสั่งขาย TikTok ใน SML'
    default:
      return ''
  }
}
