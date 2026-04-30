// Shared audit-log meta — single source of truth for action labels, tones,
// and one-line summaries. Used by /logs and the per-bill timeline view.
//
// Adding a new audit action means: (a) add an entry here so the UI shows a
// proper label/emoji/tone, and (b) optionally extend summarize() to render
// the action's detail fields nicely.

export interface AuditLog {
  id: string
  user_id?: string
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

export const ACTION_META: Record<string, ActionMeta> = {
  // Bill lifecycle
  bill_created: { label: 'สร้างบิล', emoji: '📥', tone: 'info' },
  bill_pending: { label: 'รอตรวจสอบ', emoji: '⏳', tone: 'warning' },
  bill_item_added: { label: 'เพิ่มรายการในบิล', emoji: '➕', tone: 'info' },
  bill_item_deleted: { label: 'ลบรายการในบิล', emoji: '➖', tone: 'muted' },
  // SML push
  sml_sent: { label: 'ส่ง SML สำเร็จ', emoji: '✅', tone: 'success' },
  sml_failed: { label: 'ส่ง SML ล้มเหลว', emoji: '❌', tone: 'danger' },
  // Mappings
  mapping_feedback: { label: 'ยืนยัน mapping', emoji: '🎯', tone: 'primary' },
  // Email/Shopee receive
  shopee_email_received: { label: 'รับอีเมล Shopee Order', emoji: '📧', tone: 'info' },
  shopee_shipped_received: { label: 'รับอีเมล Shopee Shipped', emoji: '📦', tone: 'info' },
  // Shopee Excel import
  shopee_import_preview: { label: 'พรีวิวไฟล์ Shopee Excel', emoji: '👁️', tone: 'muted' },
  shopee_import_done: { label: 'นำเข้า Shopee สำเร็จ', emoji: '📊', tone: 'success' },
  // Catalog
  product_created: { label: 'สร้างสินค้าใน SML', emoji: '🆕', tone: 'primary' },
  // Channel defaults
  channel_default_updated: { label: 'แก้ไขลูกค้า default', emoji: '⚙️', tone: 'info' },
  channel_default_deleted: { label: 'ลบลูกค้า default', emoji: '🗑️', tone: 'muted' },
  channel_default_quick_setup: { label: 'ตั้งค่าลูกค้า default อัตโนมัติ', emoji: '🚀', tone: 'primary' },
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
  shopee: 'Shopee',
  shopee_email: 'Shopee Email',
  shopee_excel: 'Shopee Excel',
  shopee_shipped: 'Shopee Shipped',
  manual: 'Manual',
  sml: 'SML',
  system: 'System',
  channel_defaults: 'Settings',
  catalog: 'Catalog',
}

export const SOURCE_TONE: Record<string, string> = {
  line: 'bg-success/10 text-success',
  email: 'bg-info/10 text-info',
  shopee: 'bg-warning/10 text-warning',
  shopee_email: 'bg-warning/10 text-warning',
  shopee_excel: 'bg-warning/10 text-warning',
  shopee_shipped: 'bg-warning/10 text-warning',
  lazada: 'bg-info/10 text-info',
  sml: 'bg-primary/10 text-primary',
  system: 'bg-muted text-muted-foreground',
  channel_defaults: 'bg-muted text-muted-foreground',
  catalog: 'bg-muted text-muted-foreground',
}

export const TONE_DOT: Record<Tone, string> = {
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-destructive',
  info: 'bg-info',
  muted: 'bg-muted-foreground/40',
  primary: 'bg-primary',
}

// summarize returns the 1-line human description of a log entry, derived
// from its detail fields. Falls back to '' when no shape matches; callers
// then either use ACTION_META.label alone or hide the summary slot.
export function summarize(log: AuditLog): string {
  const d = log.detail ?? {}
  switch (log.action) {
    case 'bill_created':
      if (d.flow === 'shopee_email' || d.flow === 'shopee_excel' || d.shopee_order_id) {
        const items = d.items_count ?? d.items ?? ''
        const id = d.order_id ?? d.shopee_order_id ?? ''
        return `order ${id}${items ? ` · ${items} รายการ` : ''}`
      }
      if (d.from_text || d.flow === 'line_text') return 'จากข้อความ LINE'
      if (d.flow) return String(d.flow)
      return ''
    case 'sml_sent':
      return [d.doc_no, d.route].filter(Boolean).join(' · ')
    case 'sml_failed': {
      const err = String(d.error ?? '')
      return err.length > 140 ? err.slice(0, 140) + '…' : err
    }
    case 'shopee_import_done':
      return `สำเร็จ ${d.success_count ?? 0} / ล้มเหลว ${d.fail_count ?? 0} (รวม ${d.total ?? 0})`
    case 'shopee_import_preview':
      return `${d.filename ?? ''} — ${d.total_orders ?? 0} order${
        d.duplicate_count ? ` · ซ้ำ ${d.duplicate_count}` : ''
      }`
    case 'shopee_email_received':
    case 'shopee_shipped_received':
      return d.subject ? String(d.subject) : ''
    case 'channel_default_quick_setup':
      return `ตั้งค่า ${d.applied_count ?? 0} channel`
    case 'channel_default_updated':
    case 'channel_default_deleted':
      return [d.channel, d.bill_type, d.party_code].filter(Boolean).join(' / ')
    case 'product_created':
      return d.code ? `${d.code} — ${d.name ?? ''}` : ''
    case 'mapping_feedback':
      return [d.raw_name, '→', d.item_code].filter(Boolean).join(' ')
    case 'bill_item_added':
    case 'bill_item_deleted':
      return d.raw_name ? String(d.raw_name) : ''
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
