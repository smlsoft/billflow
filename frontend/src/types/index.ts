// ─── User ───────────────────────────────────────────────────────────────────
export interface User {
  id: string
  email: string
  name: string
  role: 'admin' | 'staff' | 'viewer'
  created_at: string
}

// ─── Catalog ─────────────────────────────────────────────────────────────────
export interface CatalogMatch {
  item_code: string
  item_name: string
  unit_code: string
  score: number
}

export interface CatalogItem {
  item_code: string
  item_name: string
  item_name2?: string
  unit_code: string
  sale_price?: number | null
  embedding_status: 'pending' | 'done' | 'error'
  embedded_at?: string | null
}

// ─── Bill ────────────────────────────────────────────────────────────────────
export type BillStatus =
  | 'pending'
  | 'processing'
  | 'needs_review'
  | 'confirmed'
  | 'sent_to_sml'
  | 'sml_success'
  | 'sml_failed'
  | 'error'
  | 'sent'
  | 'failed'

export interface BillItem {
  id: string
  bill_id: string
  raw_name: string
  item_code?: string | null
  qty: number
  unit_code?: string | null
  price?: number | null
  mapped: boolean
  mapping_id?: string | null
  candidates?: CatalogMatch[] // top-5 catalog matches for needs_review items
}

export interface Bill {
  id: string
  bill_type: string
  source: string
  status: BillStatus
  raw_data?: Record<string, unknown> | null
  sml_doc_no?: string | null
  sml_order_id?: string | null
  sml_payload?: Record<string, unknown> | null
  sml_response?: Record<string, unknown> | null
  ai_confidence?: number
  anomalies?: Anomaly[]
  error_msg?: string | null
  items?: BillItem[]
  created_at: string
  sent_at?: string | null
  // computed in list view
  total_amount?: number | null
}

export interface BillListResponse {
  data: Bill[]
  total: number
  page: number
  per_page: number
}

// ─── Mapping ─────────────────────────────────────────────────────────────────
export interface Mapping {
  id: string
  raw_name: string
  item_code: string
  unit_code: string
  confidence: number
  source: 'manual' | 'ai_learned'
  usage_count: number
  last_used_at?: string | null
  created_at: string
}

export interface MappingStats {
  total: number
  auto_confirmed: number
  needs_review: number
}

// ─── Dashboard ───────────────────────────────────────────────────────────────
export interface DashboardStats {
  total_bills: number
  pending: number
  needs_review: number
  confirmed: number
  sml_success: number
  sml_failed: number
  total_amount: number
  today_bills: number
}

export interface DailyInsight {
  id: string
  insight: string
  date: string
  created_at: string
}

// ─── Anomaly ─────────────────────────────────────────────────────────────────
export interface Anomaly {
  type: 'qty_zero' | 'price_zero' | 'price_too_high' | 'price_too_low' | 'qty_suspicious' | 'new_item'
  message: string
  severity: 'error' | 'warning'
}

// ─── API Generic ─────────────────────────────────────────────────────────────
export interface APIError {
  error: string
}

// ─── Import (Phase 4) ────────────────────────────────────────────────────────
export interface BillPreview {
  bill_id: string
  order_id: string
  customer_name: string
  item_count: number
  mapped_count: number
  total_amount: number
  anomalies: Array<{ code: string; severity: 'block' | 'warn'; message: string }>
  has_block: boolean
}

export interface ImportUploadResponse {
  platform: string
  bill_type: string
  total: number
  bills: BillPreview[]
}

export interface ImportConfirmResponse {
  success: number
  failed: number
  errors: Array<{ bill_id: string; reason: string }>
}

export interface PlatformColumnMapping {
  id?: string
  platform: string
  field_name: string
  column_name: string
  updated_at?: string
}

