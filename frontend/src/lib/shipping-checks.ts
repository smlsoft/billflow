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
