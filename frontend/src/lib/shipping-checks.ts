export type ShippingStatus = 'match' | 'mismatch' | 'unverifiable'
export interface ShippingProfile {
  id: string
  name: string
  house: string
  moo: string
  district: string
  amphoe: string
  province: string
  postcode: string
  district_aliases: string[]
  amphoe_aliases: string[]
  province_aliases: string[]
}
export interface ShippingPolicy {
  account_id: string
  profile_id: string
  recipients: string[]
  enabled: boolean
  enabled_at?: string
}
export interface ShippingConfig {
  profiles: ShippingProfile[]
  policies: ShippingPolicy[]
  revision: string
}
export interface ShippingResult {
  trigger?: 'manual'
  checked_by?: string
  status: ShippingStatus
  reason?: string
  recipient: string
  address: string
  version: string
  findings?: {
    field: string
    expected: string
    actual: string
    status: ShippingStatus
  }[]
}
export interface ShippingEntry {
  id: string
  message_id: string
  artifact_id: string
  artifact_bill_id: string
  created_at: string
  result: ShippingResult
  policy: { profile: ShippingProfile; recipients: string[]; revision: string }
}
export const shippingLabels: Record<ShippingStatus, string> = {
  match: 'ตรงตามที่กำหนด',
  mismatch: 'ข้อมูลจัดส่งไม่ตรง',
  unverifiable: 'ตรวจสอบไม่ได้',
}
export function shippingSummary(
  raw: Record<string, unknown> | null | undefined,
):
  | {
      status?: ShippingStatus
      attention?: boolean
      reason?: string
      has_warning?: boolean
    }
  | undefined {
  const value = raw?.shipping_check_summary
  if (!value || typeof value !== 'object') return undefined
  return value as {
    status?: ShippingStatus
    attention?: boolean
    reason?: string
    has_warning?: boolean
  }
}
export function shippingError(e: unknown) {
  const err = e as { response?: { data?: { error?: string } } }
  return err.response?.data?.error || 'โหลดข้อมูลไม่สำเร็จ กรุณาลองใหม่'
}

export interface ShippingSourceDetails {
  recipient: string
  address: string
  reason?: string
  artifact_id?: string
  artifact_bill_id?: string
  manual_check_available: boolean
  manual_check_reason?: string
}

// getRandomValues also works on customer HTTP deployments where randomUUID is
// unavailable. This is a request deduplication id, never an authentication token.
export function newShippingCheckRequestID(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  bytes[6] = (bytes[6] & 0x0f) | 0x40
  bytes[8] = (bytes[8] & 0x3f) | 0x80
  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}
