// Shared audit-log meta — single source of truth for action labels, tones,
// and one-line summaries. Used by /logs and the per-bill timeline view.
//
// Adding a new audit action means: (a) add an entry here so the UI shows a
// proper label/emoji/tone, and (b) optionally extend summarize() to render
// the action's detail fields nicely.

export interface AuditLog {
  id: string
  user_id?: string
  actor?: {
    id?: string
    name: string
    email?: string
    role?: string
    type: 'user' | 'system' | 'worker' | string
  }
  action: string
  target_id?: string
  source?: string
  level?: string
  duration_ms?: number
  trace_id?: string
  detail?: Record<string, any>
  created_at: string
}

export type Tone = 'success' | 'warning' | 'danger' | 'info' | 'muted' | 'primary'

export interface ActionMeta {
  label: string
  emoji: string
  tone: Tone
}

export function stockRequestDiagnosticMessage(detail: Record<string, any> | undefined): string {
  if (!detail) return ''
  const url = detail.stock_request_url ? String(detail.stock_request_url) : ''
  if (detail.message) return url ? `${String(detail.message)} → ${url}` : String(detail.message)
  const error = String(detail.error ?? '').toLowerCase()
  if (error.includes('http 404')) {
    const hint = 'ไม่พบ endpoint processstockrequest: SMLJavaWebService ต้องเป็นเวอร์ชันที่รองรับ processstockrequest'
    return url ? `${hint} → ${url}` : hint
  }
  return ''
}

export const ACTION_META: Record<string, ActionMeta> = {
  // Bill lifecycle
  bill_created: { label: 'สร้างบิล', emoji: '📥', tone: 'info' },
  bill_pending: { label: 'รอตรวจสอบ', emoji: '⏳', tone: 'warning' },
  bill_archived: { label: 'เก็บบิล', emoji: '🗄️', tone: 'muted' },
  bill_restored: { label: 'กู้คืนบิล', emoji: '♻️', tone: 'info' },
  bill_item_added: { label: 'เพิ่มรายการในบิล', emoji: '➕', tone: 'info' },
  bill_item_deleted: { label: 'ลบรายการในบิล', emoji: '➖', tone: 'muted' },
  bill_item_amounts_updated: { label: 'แก้ราคา/ส่วนลดรายการ', emoji: '💰', tone: 'info' },
  bill_print_payment_method_updated: { label: 'แก้วิธีชำระเงินสำหรับพิมพ์', emoji: '💳', tone: 'info' },
  google_drive_export_settings_updated: { label: 'แก้การตั้งค่า Google Drive อีเมล', emoji: '☁️', tone: 'info' },
  google_drive_email_export_queued: { label: 'เพิ่มอีเมลเข้าคิว Google Drive', emoji: '📤', tone: 'info' },
  google_drive_email_export_succeeded: { label: 'อัปโหลดอีเมลไป Google Drive สำเร็จ', emoji: '✅', tone: 'success' },
  google_drive_email_export_retry_scheduled: { label: 'รอลองอัปโหลด Google Drive ใหม่', emoji: '🔄', tone: 'warning' },
  google_drive_email_export_failed: { label: 'อัปโหลด Google Drive ไม่สำเร็จ', emoji: '⚠️', tone: 'danger' },
  google_drive_email_export_conflict: { label: 'ไฟล์ Google Drive ชื่อซ้ำ', emoji: '⚠️', tone: 'danger' },
  google_drive_email_export_skipped: { label: 'ข้ามอัปโหลด Google Drive', emoji: '⏭️', tone: 'muted' },
  google_drive_email_export_backfill_queued: { label: 'เพิ่มอีเมลย้อนหลังเข้าคิว Google Drive', emoji: '📤', tone: 'info' },
  google_drive_email_export_retried: { label: 'สั่งลองอัปโหลด Google Drive ใหม่', emoji: '🔄', tone: 'info' },
  google_drive_email_export_pdf_queued: { label: 'สั่งสร้าง PDF อีเมลเพื่ออัปโหลด Google Drive', emoji: '📄', tone: 'info' },
  bill_doc_no_regenerated: { label: 'ออกเลขเอกสารใหม่', emoji: '🔢', tone: 'primary' },
  bill_doc_no_regenerate_failed: { label: 'ออกเลขเอกสารใหม่ไม่สำเร็จ', emoji: '⚠️', tone: 'danger' },
  bill_doc_no_preview_failed: { label: 'ดึงเลขล่าสุดไม่สำเร็จ', emoji: '⚠️', tone: 'danger' },
  // SML push
  sml_sent: { label: 'ส่ง SML สำเร็จ', emoji: '✅', tone: 'success' },
  sml_failed: { label: 'ส่ง SML ล้มเหลว', emoji: '❌', tone: 'danger' },
  sml_erp_log_warning: { label: 'บันทึก Log SML ไม่ครบ', emoji: '⚠️', tone: 'warning' },
  sml_readiness_blocked: { label: 'SML ยังไม่พร้อม', emoji: '⚠️', tone: 'warning' },
  bill_purchase_creditor_updated: { label: 'แก้เจ้าหนี้ PO', emoji: '🧾', tone: 'success' },
  bill_purchase_creditor_update_failed: { label: 'แก้เจ้าหนี้ PO ไม่สำเร็จ', emoji: '⚠️', tone: 'danger' },
  sml_stock_recalc_ok: { label: 'คำนวณต้นทุนสต๊อก', emoji: '📊', tone: 'success' },
  sml_stock_recalc_failed: { label: 'คำนวณต้นทุนสต๊อกล้มเหลว', emoji: '⚠️', tone: 'warning' },
  // Mappings
  mapping_feedback: { label: 'ยืนยัน mapping', emoji: '🎯', tone: 'primary' },
  marketplace_alias_confirmed: { label: 'ยืนยันสินค้าจาก Marketplace', emoji: '🎯', tone: 'primary' },
  // Email/Shopee receive
  shopee_email_received: { label: 'รับอีเมล Shopee Order', emoji: '📧', tone: 'info' },
  shopee_shipped_received: { label: 'รับอีเมล Shopee Shipped', emoji: '📦', tone: 'info' },
  shopee_shipped_order_ids_rejected: { label: 'ไม่สร้างบิล: เลขคำสั่งซื้อไม่ตรงอีเมล', emoji: '⛔', tone: 'danger' },
  marketplace_email_group_reconciled: { label: 'ตรวจความครบอีเมลใหม่', emoji: '🔄', tone: 'info' },
  shopee_email_replay_requested: { label: 'สั่งอ่านอีเมล Shopee ซ้ำ', emoji: '🔄', tone: 'info' },
  lazada_email_received: { label: 'รับอีเมล Lazada', emoji: '📧', tone: 'info' },
  email_print_requested: { label: 'พิมพ์อีเมลต้นทาง', emoji: '🖨️', tone: 'info' },
  shopee_email_repair_previewed: { label: 'ตรวจอีเมลก่อนซ่อม', emoji: '🔎', tone: 'info' },
  shopee_email_repair_started: { label: 'เริ่มซ่อมจากอีเมลยืนยัน', emoji: '🛠️', tone: 'warning' },
  shopee_email_repair_completed: { label: 'ซ่อมจากอีเมลยืนยันเสร็จ', emoji: '✅', tone: 'success' },
  shopee_email_repair_failed: { label: 'ซ่อมจากอีเมลยืนยันไม่สำเร็จ', emoji: '⚠️', tone: 'danger' },
  shopee_email_repair_rebuilt_bill: { label: 'ซ่อมบิล Shopee จากอีเมลยืนยัน', emoji: '🛠️', tone: 'success' },
  lazada_email_repair_rebuilt_bill: { label: 'ซ่อมบิล Lazada จากอีเมลยืนยัน', emoji: '🛠️', tone: 'success' },
  credit_card_report_run_created: { label: 'สร้างรอบรายงานบัตรเครดิต', emoji: '📊', tone: 'primary' },
  credit_card_report_exported: { label: 'Export รายงานบัตรเครดิต', emoji: '📤', tone: 'success' },
  credit_card_report_print_requested: { label: 'พิมพ์ตามรายงานบัตรเครดิต', emoji: '🖨️', tone: 'info' },
  shopee_shipping_line_ensured: { label: 'เติมค่าขนส่ง Shopee', emoji: '🚚', tone: 'info' },
  marketplace_fee_line_ensured: { label: 'เติม Fee Marketplace', emoji: '🏷️', tone: 'info' },
  // Shopee Excel import
  shopee_import_preview: { label: 'พรีวิวไฟล์ Shopee Excel', emoji: '👁️', tone: 'muted' },
  shopee_import_done: { label: 'นำเข้า Shopee สำเร็จ', emoji: '📊', tone: 'success' },
  shopee_duplicate_merged: { label: 'รวมรายการ Shopee ซ้ำ', emoji: '🔁', tone: 'muted' },
  shopee_api_connection_updated: { label: 'แก้ไขการเชื่อมต่อ Shopee API', emoji: '🔌', tone: 'info' },
  shopee_api_preview_requested: { label: 'พรีวิว Shopee API', emoji: '👁️', tone: 'info' },
  shopee_settlement_preview_started: { label: 'เริ่มดึงรอบถอนเงิน Shopee', emoji: '📥', tone: 'info' },
  shopee_settlement_preview_completed: { label: 'ดึงรอบถอนเงิน Shopee เสร็จ', emoji: '✅', tone: 'success' },
  shopee_settlement_preview_failed: { label: 'ดึงรอบถอนเงิน Shopee ไม่สำเร็จ', emoji: '⚠️', tone: 'danger' },
  shopee_settlement_reconciled: { label: 'รีเฟรชผลรับชำระ Shopee', emoji: '🔄', tone: 'info' },
  shopee_settlement_send_blocked: { label: 'ส่งรับชำระ Shopee ถูกบล็อก', emoji: '⛔', tone: 'warning' },
  shopee_settlement_hidden: { label: 'ซ่อนงานรับชำระ Shopee', emoji: '🙈', tone: 'muted' },
  shopee_settlement_restored: { label: 'กู้คืนงานรับชำระ Shopee', emoji: '↩️', tone: 'info' },
  shopee_settlement_sent: { label: 'ส่งรับชำระ Shopee เข้า SML สำเร็จ', emoji: '🧾', tone: 'success' },
  shopee_settlement_defaults_updated: { label: 'แก้ไขค่ารับชำระ Shopee', emoji: '⚙️', tone: 'info' },
  lazada_import_preview: { label: 'พรีวิวไฟล์ Lazada Excel', emoji: '👁️', tone: 'muted' },
  lazada_import_done: { label: 'นำเข้า Lazada สำเร็จ', emoji: '📊', tone: 'success' },
  tiktok_import_preview: { label: 'พรีวิวไฟล์ TikTok Excel', emoji: '👁️', tone: 'muted' },
  tiktok_import_done: { label: 'นำเข้า TikTok สำเร็จ', emoji: '📊', tone: 'success' },
  // Catalog
  product_created: { label: 'สร้างสินค้าใน SML', emoji: '🆕', tone: 'primary' },
  catalog_refresh_one: { label: 'ดึงสินค้า SML รายตัว', emoji: '🔄', tone: 'info' },
  catalog_delete_one: { label: 'ลบสินค้าออกจาก Catalog', emoji: '🗑️', tone: 'muted' },
  hidden_item_code_detected: { label: 'พบรหัสสินค้ามีอักขระซ่อน', emoji: '⚠️', tone: 'warning' },
  sml_customer_created: { label: 'สร้างลูกค้าใน SML', emoji: '🆕', tone: 'primary' },
  sml_supplier_created: { label: 'สร้างผู้ขายใน SML', emoji: '🆕', tone: 'primary' },
  // Users
  user_created: { label: 'เพิ่มผู้ใช้ระบบ', emoji: '➕', tone: 'primary' },
  user_updated: { label: 'แก้ไขผู้ใช้ระบบ', emoji: '✏️', tone: 'info' },
  user_deleted: { label: 'ลบผู้ใช้ระบบ', emoji: '🗑️', tone: 'danger' },
  // Setup / demo maintenance
  demo_test_data_reset: { label: 'ล้างข้อมูลทดสอบ', emoji: '🧹', tone: 'warning' },
  // Channel defaults
  channel_default_updated: { label: 'แก้ไขเส้นทาง SML', emoji: '⚙️', tone: 'info' },
  channel_default_deleted: { label: 'ลบเส้นทาง SML', emoji: '🗑️', tone: 'muted' },
  channel_default_quick_setup: { label: 'ตั้งค่าเส้นทาง SML อัตโนมัติ', emoji: '🚀', tone: 'primary' },
  // LINE chat — admin actions (session 13-15)
  line_admin_reply: { label: 'ตอบลูกค้าใน LINE', emoji: '💬', tone: 'info' },
  line_admin_send_media: { label: 'ส่งรูปให้ลูกค้าใน LINE', emoji: '🖼️', tone: 'info' },
  line_conversation_status: { label: 'เปลี่ยนสถานะห้องแชท', emoji: '🏷️', tone: 'muted' },
  line_message_received: { label: 'ลูกค้าทักผ่าน LINE', emoji: '📨', tone: 'muted' },
  // LINE OA accounts (multi-OA)
  line_oa_created: { label: 'เพิ่ม LINE OA', emoji: '➕', tone: 'primary' },
  line_oa_updated: { label: 'แก้ไข LINE OA', emoji: '✏️', tone: 'info' },
  line_oa_deleted: { label: 'ลบ LINE OA', emoji: '🗑️', tone: 'danger' },
  // Chat CRM lite (Phase 4.7-4.9)
  chat_phone_saved: { label: 'บันทึกเบอร์ลูกค้า', emoji: '📞', tone: 'info' },
  chat_note_created: { label: 'เพิ่ม note ภายใน', emoji: '📝', tone: 'info' },
  chat_note_updated: { label: 'แก้ไข note ภายใน', emoji: '✏️', tone: 'muted' },
  chat_note_deleted: { label: 'ลบ note ภายใน', emoji: '🗑️', tone: 'muted' },
  chat_tag_created: { label: 'สร้าง chat tag', emoji: '🏷️', tone: 'primary' },
  chat_tag_updated: { label: 'แก้ไข chat tag', emoji: '✏️', tone: 'info' },
  chat_tag_deleted: { label: 'ลบ chat tag', emoji: '🗑️', tone: 'muted' },
  chat_conv_tags_set: { label: 'เปลี่ยน tag ของห้องแชท', emoji: '🔖', tone: 'muted' },
  chat_quick_reply_created: { label: 'เพิ่ม quick reply', emoji: '💡', tone: 'info' },
  chat_quick_reply_updated: { label: 'แก้ไข quick reply', emoji: '✏️', tone: 'muted' },
  chat_quick_reply_deleted: { label: 'ลบ quick reply', emoji: '🗑️', tone: 'muted' },
}

export const SOURCE_LABELS: Record<string, string> = {
  line: 'LINE',
  line_oa: 'LINE',
  email: 'Email',
  lazada: 'Lazada',
  lazada_email: 'Email บิลซื้อ Lazada',
  tiktok: 'TikTok Excel',
  shopee: 'Shopee',
  shopee_email: 'Shopee Email',
  shopee_excel: 'Shopee Excel',
  shopee_shipped: 'Shopee Shipped',
  manual: 'Manual',
  sml: 'SML',
  system: 'ระบบ',
  ui: 'หน้าจอผู้ใช้',
  setup: 'ตั้งค่าระบบ',
  settings: 'ตั้งค่าระบบ',
  channel_defaults: 'ตั้งค่าเอกสาร',
  catalog: 'สินค้า SML',
  shopee_api: 'Shopee API',
  shopee_settlement: 'รับชำระ Shopee',
  credit_card_report: 'รายงานบัตรเครดิต',
  google_drive: 'Google Drive',
}

export const SOURCE_TONE: Record<string, string> = {
  line: 'bg-success/10 text-success',
  email: 'bg-info/10 text-info',
  shopee: 'bg-warning/10 text-warning',
  shopee_email: 'bg-warning/10 text-warning',
  shopee_excel: 'bg-warning/10 text-warning',
  shopee_shipped: 'bg-warning/10 text-warning',
  lazada: 'bg-info/10 text-info',
  lazada_email: 'bg-info/10 text-info',
  tiktok: 'bg-muted text-foreground',
  sml: 'bg-primary/10 text-primary',
  system: 'bg-muted text-muted-foreground',
  setup: 'bg-warning/10 text-warning',
  channel_defaults: 'bg-muted text-muted-foreground',
  catalog: 'bg-muted text-muted-foreground',
  settings: 'bg-muted text-muted-foreground',
  ui: 'bg-primary/10 text-primary',
  shopee_api: 'bg-warning/10 text-warning',
  shopee_settlement: 'bg-success/10 text-success',
  credit_card_report: 'bg-primary/10 text-primary',
  google_drive: 'bg-info/10 text-info',
}

export const TONE_DOT: Record<Tone, string> = {
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-destructive',
  info: 'bg-info',
  muted: 'bg-muted-foreground/40',
  primary: 'bg-primary',
}

export function smlRouteLabel(route: unknown): string {
  const text = String(route ?? '')
  const normalized = text.toLowerCase()
  const map: Record<string, string> = {
    purchaseorder: 'ซื้อ -> ใบสั่งซื้อ',
    saleorder: 'ใบสั่งขาย',
    saleinvoice: 'ขาย -> ขายสินค้าและบริการ',
    salereserve: 'ใบสั่งจอง',
    sale_reserve: 'ใบสั่งจอง',
  }
  return map[normalized] ?? text
}

function auditViaLabel(value: unknown): string {
  const text = String(value ?? '')
  const map: Record<string, string> = {
    retry: 'ส่งจากหน้าบิล',
    bulk_job: 'ส่งแบบกลุ่ม',
    import: 'ส่งตอนนำเข้า',
  }
  return map[text] ?? text
}

export function humanizeAuditError(value: unknown): string {
  if (value == null || value === '') return ''
  const text = String(value)
  const lower = text.toLowerCase()
  if (lower.includes('request failed with status code')) {
    const code = text.match(/status code\s+(\d+)/i)?.[1]
    return code ? `ระบบเรียก API ไม่สำเร็จ (HTTP ${code})` : 'ระบบเรียก API ไม่สำเร็จ'
  }
  if (lower.includes('sml doc_no next')) {
    if (lower.includes('format must contain a # sequence block')) {
      return 'รูปแบบเลขเอกสารไม่มีชุด # สำหรับเลขรัน ให้ตรวจ prefix/running format ในเส้นทางเอกสาร SML'
    }
    const code = text.match(/HTTP\s+(\d+)/i)?.[1]
    const suffix = text.split(':').slice(1).join(':').trim()
    return [
      'ดึงเลข running ล่าสุดจาก SML ไม่สำเร็จ',
      code ? `(HTTP ${code})` : '',
      suffix && !suffix.toLowerCase().includes('sml doc_no next') ? suffix : '',
    ].filter(Boolean).join(' ')
  }
  if (lower.includes('duplicate key') || lower.includes('already exists')) {
    return 'เลขเอกสารซ้ำกับข้อมูลเดิมใน SML หรือใน BillFlow'
  }
  if (
    lower.includes('customer_count_failed') ||
    lower.includes('supplier_count_failed') ||
    lower.includes('count customers failed') ||
    lower.includes('count suppliers failed') ||
    lower.includes('timeout') ||
    lower.includes('deadline exceeded') ||
    lower.includes('eof') ||
    lower.includes('connection refused')
  ) {
    return 'เครื่อง SML/Postgres ของร้านนี้อาจยังไม่เปิดหรือเชื่อมต่อไม่ได้'
  }
  if (lower.includes('invalid credentials') || lower.includes('authenticationfailed')) {
    return 'ข้อมูลเข้าสู่ระบบหรือ App Password ไม่ถูกต้อง'
  }
  if (lower === 'bill not found') return 'ไม่พบบิลนี้แล้ว'
  if (lower.includes('record print event failed')) return 'บันทึกประวัติการพิมพ์อีเมลไม่สำเร็จ'
  if (lower.includes('failed to save alias')) return 'บันทึกการจับคู่สินค้าไม่สำเร็จ'
  if (lower.includes('failed to apply alias')) return 'นำการจับคู่สินค้าไปใช้กับบิลที่รอตรวจไม่สำเร็จ'
  return text
}

// summarize returns the 1-line human description of a log entry, derived
// from its detail fields. Falls back to '' when no shape matches; callers
// then either use ACTION_META.label alone or hide the summary slot.
export function summarize(log: AuditLog): string {
  const d = log.detail ?? {}
  switch (log.action) {
    case 'bill_created':
      if (d.flow === 'shopee_email' || d.flow === 'shopee_excel' || d.flow === 'tiktok_excel' || d.shopee_order_id || d.tiktok_order_id) {
        const items = d.items_count ?? d.items ?? ''
        const id = d.order_id ?? d.shopee_order_id ?? d.tiktok_order_id ?? ''
        return `ออเดอร์ ${id}${items ? ` · ${items} รายการ` : ''}`
      }
      if (d.from_text || d.flow === 'line_text') return 'จากข้อความ LINE'
      if (d.flow) return String(d.flow)
      return ''
    case 'sml_sent':
      return [d.doc_no, d.route ? smlRouteLabel(d.route) : '', d.via ? auditViaLabel(d.via) : ''].filter(Boolean).join(' · ')
    case 'sml_erp_log_warning':
      return [d.doc_no, d.route ? smlRouteLabel(d.route) : '', d.message].filter(Boolean).join(' · ')
    case 'sml_stock_recalc_failed':
      return [
        d.doc_no,
        d.route ? smlRouteLabel(d.route) : '',
        stockRequestDiagnosticMessage(d) || humanizeAuditError(d.error),
      ].filter(Boolean).join(' · ')
    case 'sml_failed': {
      const err = parseMaybeJSON(d.error)
      const route = err.route ?? d.route
      const docNo = err.doc_no_attempted ?? d.doc_no
      const message = humanizeAuditError(err.error ?? d.error ?? '')
      return [route ? smlRouteLabel(route) : '', docNo, message].filter(Boolean).join(' · ')
    }
    case 'sml_readiness_blocked':
      return [d.via ? auditViaLabel(d.via) : '', d.tenant ? `ฐานข้อมูล ${d.tenant}` : '', d.message].filter(Boolean).join(' · ')
    case 'bill_purchase_creditor_updated':
      return [
        d.doc_no,
        d.old_party_code && d.new_party_code ? `${d.old_party_code} → ${d.new_party_code}` : '',
        d.changed === false ? 'ไม่เปลี่ยนแปลง' : '',
      ].filter(Boolean).join(' · ')
    case 'bill_print_payment_method_updated':
      return [
        d.doc_no,
        d.payment_method ? `วิธีชำระ: ${String(d.payment_method)}` : '',
        typeof d.updated_count === 'number' ? `อัปเดต ${d.updated_count.toLocaleString()} บิล` : '',
      ].filter(Boolean).join(' · ')
    case 'bill_item_amounts_updated': {
      const money = (value: unknown) => {
        const amount = Number(value)
        if (!Number.isFinite(amount)) return ''
        return `฿${amount.toLocaleString('th-TH', { maximumFractionDigits: 2 })}`
      }
      return [
        d.raw_name ? String(d.raw_name) : '',
        d.old_price != null && d.new_price != null ? `ราคา ${money(d.old_price)} → ${money(d.new_price)}` : '',
        d.old_discount_amount != null && d.new_discount_amount != null
          ? `ส่วนลด ${money(d.old_discount_amount)} → ${money(d.new_discount_amount)}`
          : '',
        d.old_line_total != null && d.new_line_total != null
          ? `ยอดสุทธิ ${money(d.old_line_total)} → ${money(d.new_line_total)}`
          : '',
      ].filter(Boolean).join(' · ')
    }
    case 'google_drive_email_export_queued':
    case 'google_drive_email_export_succeeded':
      return [d.sml_doc_no, d.marketplace_order_id].filter(Boolean).join(' · ')
    case 'google_drive_email_export_retry_scheduled':
    case 'google_drive_email_export_failed':
    case 'google_drive_email_export_conflict':
    case 'google_drive_email_export_skipped':
      return [d.reason, d.next_attempt_at ? `ลองใหม่ ${String(d.next_attempt_at)}` : ''].filter(Boolean).join(' · ')
    case 'google_drive_email_export_backfill_queued':
      return [`${d.date_from ?? ''} ถึง ${d.date_to ?? ''}`, typeof d.queued === 'number' ? `เพิ่ม ${d.queued.toLocaleString()} บิล` : ''].filter(Boolean).join(' · ')
    case 'bill_purchase_creditor_update_failed':
      return [
        d.doc_no,
        d.old_party_code && d.new_party_code ? `${d.old_party_code} → ${d.new_party_code}` : '',
        humanizeAuditError(d.error),
      ].filter(Boolean).join(' · ')
    case 'bill_doc_no_regenerated':
      return [d.doc_no, d.route ? smlRouteLabel(d.route) : ''].filter(Boolean).join(' · ')
    case 'bill_doc_no_regenerate_failed':
    case 'bill_doc_no_preview_failed':
      return [d.route ? smlRouteLabel(d.route) : '', humanizeAuditError(d.error)].filter(Boolean).join(' · ')
    case 'shopee_import_done':
    case 'lazada_import_done':
    case 'tiktok_import_done':
      return `สำเร็จ ${d.success_count ?? 0} / ล้มเหลว ${d.fail_count ?? 0} (รวม ${d.total ?? 0})`
    case 'shopee_import_preview':
    case 'lazada_import_preview':
    case 'tiktok_import_preview':
      return `${d.filename ?? ''} — ${d.total_orders ?? 0} ออเดอร์${
        d.duplicate_count ? ` · ซ้ำ ${d.duplicate_count}` : ''
      }`
    case 'shopee_email_received':
    case 'shopee_shipped_received':
    case 'lazada_email_received':
      return d.subject ? String(d.subject) : ''
    case 'shopee_shipped_order_ids_rejected': {
      const unexpected = Array.isArray(d.unexpected_order_ids) ? d.unexpected_order_ids : []
      const missing = Array.isArray(d.missing_order_ids) ? d.missing_order_ids : []
      const duplicate = Array.isArray(d.duplicate_order_ids) ? d.duplicate_order_ids : []
      return [
        unexpected.length ? `เลขเกิน ${unexpected.join(', ')}` : '',
        missing.length ? `เลขหาย ${missing.join(', ')}` : '',
        duplicate.length ? `เลขซ้ำ ${duplicate.join(', ')}` : '',
      ].filter(Boolean).join(' · ')
    }
    case 'marketplace_email_group_reconciled':
      return [
        d.status === 'complete' ? 'ครบแล้ว' : d.status ? String(d.status) : '',
        d.resolved_order_count != null && d.expected_order_count != null
          ? `${Number(d.resolved_order_count).toLocaleString('th-TH')}/${Number(d.expected_order_count).toLocaleString('th-TH')} คำสั่งซื้อ`
          : '',
      ].filter(Boolean).join(' · ')
    case 'shopee_email_replay_requested':
      return [
        d.created != null ? `สร้างบิล ${Number(d.created).toLocaleString('th-TH')}` : '',
        d.updated_existing != null ? `อัปเดตเดิม ${Number(d.updated_existing).toLocaleString('th-TH')}` : '',
        d.failed != null && Number(d.failed) > 0 ? `ต้องตรวจ ${Number(d.failed).toLocaleString('th-TH')}` : '',
      ].filter(Boolean).join(' · ')
    case 'email_print_requested':
      return d.email_group_key ? `Email #${d.email_group_key}` : ''
    case 'shopee_email_repair_previewed':
      return [
        repairPlatformLabel(d.source),
        d.detected_order_count != null ? `พบ ${Number(d.detected_order_count).toLocaleString('th-TH')} คำสั่งซื้อ` : '',
        d.missing_count != null ? `ตกหล่น ${Number(d.missing_count).toLocaleString('th-TH')}` : '',
        d.rebuild_count != null && Number(d.rebuild_count) > 0 ? `ซ่อมเดิม ${Number(d.rebuild_count).toLocaleString('th-TH')}` : '',
        d.blocked_count != null && Number(d.blocked_count) > 0 ? `ข้าม ${Number(d.blocked_count).toLocaleString('th-TH')}` : '',
      ].filter(Boolean).join(' · ')
    case 'shopee_email_repair_started':
      return [
        repairPlatformLabel(d.source),
        d.expected_order_count != null ? `คาดว่า ${Number(d.expected_order_count).toLocaleString('th-TH')} คำสั่งซื้อ` : '',
        Array.isArray(d.expected_missing_ids) && d.expected_missing_ids.length > 0 ? `สร้างใหม่ ${d.expected_missing_ids.length}` : '',
        Array.isArray(d.expected_rebuild_ids) && d.expected_rebuild_ids.length > 0 ? `ซ่อมเดิม ${d.expected_rebuild_ids.length}` : '',
      ].filter(Boolean).join(' · ')
    case 'shopee_email_repair_completed':
      return [
        repairPlatformLabel(d.source),
        d.created_count != null ? `สร้างใหม่ ${Number(d.created_count).toLocaleString('th-TH')}` : '',
        d.rebuilt_count != null ? `ซ่อมเดิม ${Number(d.rebuilt_count).toLocaleString('th-TH')}` : '',
        d.skipped_count != null ? `ข้าม ${Number(d.skipped_count).toLocaleString('th-TH')}` : '',
      ].filter(Boolean).join(' · ')
    case 'shopee_email_repair_failed':
      return [repairPlatformLabel(d.source), humanizeAuditError(d.error)].filter(Boolean).join(' · ')
    case 'shopee_email_repair_rebuilt_bill':
    case 'lazada_email_repair_rebuilt_bill':
      return [
        d.order_id ? `คำสั่งซื้อ ${String(d.order_id).replace(/^#/, '')}` : '',
        d.seller_name,
        d.items_count != null ? `${Number(d.items_count).toLocaleString('th-TH')} รายการ` : '',
        d.status ? `สถานะ ${d.status}` : '',
      ].filter(Boolean).join(' · ')
    case 'credit_card_report_run_created':
      return [
        d.report_name,
        d.group_count != null ? `${Number(d.group_count).toLocaleString('th-TH')} ยอดรูด` : '',
        d.order_count != null ? `${Number(d.order_count).toLocaleString('th-TH')} คำสั่งซื้อ` : '',
        d.issue_group_count != null && Number(d.issue_group_count) > 0
          ? `ต้องตรวจ ${Number(d.issue_group_count).toLocaleString('th-TH')}`
          : '',
      ].filter(Boolean).join(' · ')
    case 'credit_card_report_exported':
      return [
        d.report_name,
        d.group_count != null ? `${Number(d.group_count).toLocaleString('th-TH')} ยอดรูด` : '',
        d.order_count != null ? `${Number(d.order_count).toLocaleString('th-TH')} คำสั่งซื้อ` : '',
        d.charge_total != null ? `ยอดรูด ${Number(d.charge_total).toLocaleString('th-TH')}` : '',
      ].filter(Boolean).join(' · ')
    case 'credit_card_report_print_requested':
      return [
        d.group_count != null ? `${Number(d.group_count).toLocaleString('th-TH')} ยอดรูด` : '',
        d.event_count != null ? `พิมพ์ ${Number(d.event_count).toLocaleString('th-TH')} ชุด` : '',
        d.skipped_count != null && Number(d.skipped_count) > 0
          ? `ข้าม ${Number(d.skipped_count).toLocaleString('th-TH')}`
          : '',
      ].filter(Boolean).join(' · ')
    case 'shopee_shipping_line_ensured':
    case 'marketplace_fee_line_ensured':
      return [d.item_code, d.price != null ? `ราคา ${Number(d.price).toLocaleString()}` : ''].filter(Boolean).join(' · ')
    case 'channel_default_quick_setup':
      return `ตั้งค่า ${d.applied_count ?? 0} ช่องทาง`
    case 'channel_default_updated':
    case 'channel_default_deleted':
      return [d.channel, d.bill_type, d.party_code].filter(Boolean).join(' / ')
    case 'product_created':
      return d.code ? `${d.code} — ${d.name ?? ''}` : ''
    case 'catalog_refresh_one':
      return [d.item_code, d.item_name].filter(Boolean).join(' — ')
    case 'catalog_delete_one':
      return d.item_code ? String(d.item_code) : ''
    case 'hidden_item_code_detected':
      return [d.item_code, d.clean_item_code ? `ควรเป็น ${d.clean_item_code}` : ''].filter(Boolean).join(' · ')
    case 'sml_customer_created':
    case 'sml_supplier_created':
      return [d.code, d.name].filter(Boolean).join(' — ')
    case 'user_created':
    case 'user_updated':
    case 'user_deleted':
      return [d.email, d.role].filter(Boolean).join(' · ')
    case 'marketplace_alias_confirmed':
      return [d.raw_name, '→', d.item_code, d.applied_items ? `ใช้กับ ${d.applied_items} รายการ` : ''].filter(Boolean).join(' ')
    case 'shopee_api_connection_updated':
      return [d.shop_id ? `ร้าน ${d.shop_id}` : '', d.label, d.disabled ? 'ปิดใช้งาน' : 'เปิดใช้งาน'].filter(Boolean).join(' · ')
    case 'shopee_api_preview_requested':
      return [d.shop_id ? `ร้าน ${d.shop_id}` : '', d.order_count != null ? `${d.order_count} ออเดอร์` : ''].filter(Boolean).join(' · ')
    case 'shopee_settlement_preview_started':
      return [
        settlementShopLabel(d),
        settlementReleaseRange(d),
      ].filter(Boolean).join(' · ')
    case 'shopee_settlement_preview_completed':
      return [
        settlementShopLabel(d),
        settlementReleaseRange(d),
        settlementCountSummary(d),
        d.message,
      ].filter(Boolean).join(' · ')
    case 'shopee_settlement_preview_failed':
      return [
        settlementShopLabel(d),
        settlementReleaseRange(d),
        humanizeAuditError(d.error ?? d.message),
      ].filter(Boolean).join(' · ')
    case 'shopee_settlement_reconciled':
      return [
        d.newly_blocked != null ? `block เพิ่ม ${Number(d.newly_blocked).toLocaleString('th-TH')} รายการ` : '',
        settlementCountSummary(d),
        d.message,
      ].filter(Boolean).join(' · ')
    case 'shopee_settlement_send_blocked':
      return [
        humanizeAuditError(d.error ?? d.message),
        settlementCountSummary(d),
        d.doc_format_code ? `ฟอร์ม ${d.doc_format_code}` : '',
        d.passbook_code ? `บัญชี ${d.passbook_code}` : '',
        d.expense_code ? `ค่าใช้จ่าย ${d.expense_code}` : '',
      ].filter(Boolean).join(' · ')
    case 'shopee_settlement_hidden':
      return [
        settlementShopLabel(d),
        settlementReleaseRange(d),
        d.hidden_reason ? `เหตุผล: ${d.hidden_reason}` : '',
      ].filter(Boolean).join(' · ')
    case 'shopee_settlement_restored':
      return [
        settlementShopLabel(d),
        settlementReleaseRange(d),
        'กลับมาแสดงในรายการปกติ',
      ].filter(Boolean).join(' · ')
    case 'shopee_settlement_sent':
      return [
        d.rc_doc_no,
        d.sent_count != null ? `ส่ง ${Number(d.sent_count).toLocaleString('th-TH')} รายการ` : '',
        d.blocked_after_reconcile_count != null && Number(d.blocked_after_reconcile_count) > 0
          ? `ข้ามหลังตรวจซ้ำ ${Number(d.blocked_after_reconcile_count).toLocaleString('th-TH')}`
          : '',
        d.doc_format_code ? `ฟอร์ม ${d.doc_format_code}` : '',
        d.passbook_code ? `บัญชี ${d.passbook_code}` : '',
        d.expense_code ? `ค่าใช้จ่าย ${d.expense_code}` : '',
      ].filter(Boolean).join(' · ')
    case 'shopee_settlement_defaults_updated':
      return [
        d.doc_format_code ? `ฟอร์ม ${d.doc_format_code}` : '',
        d.passbook_code ? `บัญชี ${d.passbook_code}` : '',
        d.expense_code ? `ค่าใช้จ่าย ${d.expense_code}` : 'ยังไม่ตั้งค่าใช้จ่าย',
      ].filter(Boolean).join(' · ')
    case 'shopee_duplicate_merged':
      return [d.order_id ?? d.shopee_order_id, d.bill_id ? `รวมเข้าบิล ${String(d.bill_id).slice(0, 8)}…` : ''].filter(Boolean).join(' · ')
    case 'demo_test_data_reset': {
      const docs = d.before_documents ?? {}
      const imports = d.before_imports ?? {}
      const totalDocs = Number(docs.total ?? 0)
      const logs = Number(d.before_logs ?? imports.audit_logs ?? 0)
      const preserved = [
        d.preserved_settings ? 'ตั้งค่า' : '',
        d.preserved_catalog ? 'สินค้า SML' : '',
        d.preserved_mappings ? 'ตารางจับคู่' : '',
        d.preserved_ai_usage_log ? 'ประวัติ AI' : '',
      ].filter(Boolean)
      const resetParts = [
        d.reset_doc_counter ? 'รีเซ็ตเลขรันเอกสาร' : '',
        d.reset_email_dedup ? 'ล้างประวัติอีเมลและรีเซ็ตตำแหน่งอ่านล่าสุด' : '',
      ].filter(Boolean)
      return [
        `ล้างบิลทดสอบ ${totalDocs.toLocaleString()} ใบ`,
        `ล้างประวัติการทำงานเดิม ${logs.toLocaleString()} รายการ`,
        preserved.length ? `เก็บไว้: ${preserved.join(', ')}` : '',
        resetParts.length ? resetParts.join(', ') : 'ไม่รีเซ็ตเลขรัน/ประวัติอีเมล',
      ].filter(Boolean).join(' · ')
    }
    case 'mapping_feedback':
      return [d.raw_name, '→', d.item_code].filter(Boolean).join(' ')
    case 'bill_item_added':
    case 'bill_item_deleted':
      return d.raw_name ? String(d.raw_name) : ''
    case 'bill_archived':
      return d.reason ? String(d.reason) : 'ซ่อนจาก queue ปกติ'
    case 'bill_restored':
      return 'นำกลับมาแสดงในรายการปกติ'
    // LINE / chat — short summaries from detail.
    case 'line_admin_reply':
      return d.text_preview ? `“${d.text_preview}”` : ''
    case 'line_admin_send_media': {
      const fname = d.filename ? String(d.filename) : 'รูปภาพ'
      const sizeKB = typeof d.size_bytes === 'number' ? Math.round(d.size_bytes / 1024) : 0
      return sizeKB > 0 ? `${fname} (${sizeKB.toLocaleString()} KB)` : fname
    }
    case 'line_message_received': {
      if (d.kind === 'text' && d.text_preview) return `“${d.text_preview}”`
      const fname = d.filename ? String(d.filename) : ''
      const sizeKB = typeof d.size_bytes === 'number' ? Math.round(d.size_bytes / 1024) : 0
      const kindLabel =
        d.kind === 'image' ? 'รูปภาพ' :
        d.kind === 'file'  ? 'ไฟล์' :
        d.kind === 'audio' ? 'เสียง' : ''
      const parts = [kindLabel, fname].filter(Boolean).join(' • ')
      return sizeKB > 0 ? `${parts} (${sizeKB.toLocaleString()} KB)` : parts
    }
    case 'line_conversation_status': {
      const map: Record<string, string> = {
        open: 'เปิดอีกครั้ง',
        resolved: 'ปิดเรื่อง',
        archived: 'Archive',
      }
      return d.status ? map[String(d.status)] ?? String(d.status) : ''
    }
    case 'line_oa_created':
    case 'line_oa_updated':
    case 'line_oa_deleted':
      return [d.name, d.basic_id].filter(Boolean).join(' · ')
    case 'chat_phone_saved':
      return d.phone ? String(d.phone) : 'เคลียร์เบอร์'
    case 'chat_note_created':
    case 'chat_note_updated':
    case 'chat_note_deleted':
      return d.body_preview ? String(d.body_preview) : ''
    case 'chat_tag_created':
    case 'chat_tag_updated':
    case 'chat_tag_deleted':
      return d.label ? `${d.label}${d.color ? ` (${d.color})` : ''}` : ''
    case 'chat_conv_tags_set': {
      const labels = Array.isArray(d.labels) ? d.labels : []
      return labels.length === 0 ? 'ลบ tag ทั้งหมด' : labels.join(', ')
    }
    case 'chat_quick_reply_created':
    case 'chat_quick_reply_updated':
    case 'chat_quick_reply_deleted':
      return d.label ? String(d.label) : ''
    default:
      return ''
  }
}

function settlementShopLabel(d: Record<string, any>): string {
  return [d.shop_label, d.shop_id ? `ร้าน ${d.shop_id}` : ''].filter(Boolean).join(' · ')
}

function repairPlatformLabel(value: unknown): string {
  const source = String(value ?? '')
  if (source.includes('lazada')) return 'Lazada'
  if (source.includes('shopee')) return 'Shopee'
  return ''
}

function settlementReleaseRange(d: Record<string, any>): string {
  const from = d.release_date_from || datePart(d.release_time_from)
  const to = d.release_date_to || datePart(d.release_time_to)
  if (!from && !to) return ''
  return `release ${[from, to].filter(Boolean).join(' - ')}`
}

function settlementCountSummary(d: Record<string, any>): string {
  const parts = [
    d.total_count != null ? `รวม ${Number(d.total_count).toLocaleString('th-TH')}` : '',
    d.ready_count != null ? `พร้อมส่ง ${Number(d.ready_count).toLocaleString('th-TH')}` : '',
    d.blocked_count != null ? `ต้องตรวจ ${Number(d.blocked_count).toLocaleString('th-TH')}` : '',
    d.sent_count != null ? `ส่งแล้ว ${Number(d.sent_count).toLocaleString('th-TH')}` : '',
  ].filter(Boolean)
  return parts.join(' · ')
}

function datePart(value: unknown): string {
  const text = String(value ?? '')
  return text.includes('T') ? text.slice(0, 10) : text
}

function parseMaybeJSON(value: unknown): Record<string, any> {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return value as Record<string, any>
  }
  if (typeof value !== 'string') return {}
  try {
    const parsed = JSON.parse(value)
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {}
  } catch {
    return {}
  }
}
