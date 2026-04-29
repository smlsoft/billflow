import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import 'dayjs/locale/th'
import {
  ChevronDown,
  Code2,
  Copy,
  Filter,
  RotateCw,
  ScrollText,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { EmptyState } from '@/components/common/EmptyState'
import { JsonViewer } from '@/components/common/JsonViewer'
import { PageHeader } from '@/components/common/PageHeader'
import api from '@/api/client'
import { cn } from '@/lib/utils'

dayjs.extend(relativeTime)
dayjs.locale('th')

interface AuditLog {
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

interface LogsResponse {
  data: AuditLog[]
  total: number
  page: number
  page_size: number
}

const ALL = '__all__'

type Tone = 'success' | 'warning' | 'danger' | 'info' | 'muted' | 'primary'

interface ActionMeta {
  label: string
  emoji: string
  tone: Tone
}

// All known actions emitted by the backend handlers. Adding new actions here
// upgrades them from "• raw_action_name" to a friendly Thai row.
const ACTION_META: Record<string, ActionMeta> = {
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

const SOURCE_LABELS: Record<string, string> = {
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

const SOURCE_TONE: Record<string, string> = {
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

const TONE_DOT: Record<Tone, string> = {
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-destructive',
  info: 'bg-info',
  muted: 'bg-muted-foreground/40',
  primary: 'bg-primary',
}

// summarize returns the 1-line human description of a log entry, derived from
// detail fields. Falls back to action name when no shape matches.
function summarize(log: AuditLog): string {
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
    // LINE / chat — short summaries from detail
    case 'line_admin_reply':
    case 'line_admin_send_media':
      return d.delivery_method === 'reply' ? 'ฟรี (Reply API)' : 'Push (นับ quota)'
    case 'line_conversation_status':
      return d.status ? String(d.status) : ''
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

function relTime(iso: string): string {
  const d = dayjs(iso)
  const diffMin = dayjs().diff(d, 'minute')
  if (diffMin < 60) return d.fromNow()
  if (dayjs().isSame(d, 'day')) return `วันนี้ ${d.format('HH:mm')}`
  if (dayjs().subtract(1, 'day').isSame(d, 'day')) return `เมื่อวาน ${d.format('HH:mm')}`
  return d.format('DD/MM/YY HH:mm')
}

function CopyChip({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <button
      type="button"
      className="group inline-flex items-center gap-1 rounded-md bg-muted px-1.5 py-0.5 font-mono text-[10px] text-muted-foreground hover:bg-muted/70"
      onClick={(e) => {
        e.stopPropagation()
        navigator.clipboard?.writeText(value)
        setCopied(true)
        setTimeout(() => setCopied(false), 1200)
      }}
      title={`คัดลอก ${label}: ${value}`}
    >
      <span className="text-[9px] uppercase opacity-60">{label}</span>
      <span>{copied ? 'copied' : value.length > 16 ? value.slice(0, 12) + '…' : value}</span>
      <Copy className="h-2.5 w-2.5 opacity-0 transition-opacity group-hover:opacity-100" />
    </button>
  )
}

function LogRow({ log }: { log: AuditLog }) {
  const [expanded, setExpanded] = useState(false)
  const [showRaw, setShowRaw] = useState(false)

  const meta = ACTION_META[log.action] ?? {
    label: log.action,
    emoji: '•',
    tone: 'muted' as Tone,
  }
  const summary = summarize(log)
  const isError = log.level === 'error'
  const source = log.source ?? ''
  const errMsg = String(log.detail?.error ?? '')
  const docNo = String(log.detail?.doc_no ?? '')

  return (
    <div
      className={cn(
        'rounded-lg border bg-card transition-colors',
        isError
          ? 'border-destructive/30 bg-destructive/5'
          : 'border-border hover:bg-accent/30',
      )}
    >
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        className="flex w-full items-start gap-3 px-4 py-3 text-left"
      >
        <span
          className={cn(
            'mt-1 inline-block h-2 w-2 shrink-0 rounded-full',
            TONE_DOT[meta.tone],
          )}
        />

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span className="text-base leading-none">{meta.emoji}</span>
            <span className="text-sm font-medium text-foreground">{meta.label}</span>
            {source && SOURCE_LABELS[source] && (
              <Badge
                variant="secondary"
                className={cn(
                  'h-5 px-1.5 text-[10px] font-medium',
                  SOURCE_TONE[source] ?? 'bg-muted text-muted-foreground',
                )}
              >
                {SOURCE_LABELS[source]}
              </Badge>
            )}
            {docNo && (
              <span className="font-mono text-[11px] font-medium text-foreground">
                {docNo}
              </span>
            )}
            {log.level && log.level !== 'info' && (
              <Badge
                variant={isError ? 'destructive' : 'secondary'}
                className="h-5 px-1.5 text-[10px] font-medium uppercase"
              >
                {log.level}
              </Badge>
            )}
          </div>
          {summary && (
            <p
              className={cn(
                'mt-0.5 line-clamp-2 text-xs',
                isError ? 'text-destructive' : 'text-muted-foreground',
              )}
            >
              {summary}
            </p>
          )}
        </div>

        <div className="flex shrink-0 flex-col items-end gap-0.5 text-right">
          <Tooltip delayDuration={300}>
            <TooltipTrigger asChild>
              <span className="text-[11px] tabular-nums text-muted-foreground">
                {relTime(log.created_at)}
              </span>
            </TooltipTrigger>
            <TooltipContent side="left">
              {dayjs(log.created_at).format('DD/MM/YYYY HH:mm:ss')}
            </TooltipContent>
          </Tooltip>
          {log.duration_ms != null && (
            <span
              className={cn(
                'font-mono text-[10px] tabular-nums',
                log.duration_ms > 3000
                  ? 'text-destructive'
                  : log.duration_ms > 1000
                    ? 'text-warning'
                    : 'text-muted-foreground/70',
              )}
            >
              {log.duration_ms}ms
            </span>
          )}
          <ChevronDown
            className={cn(
              'h-3.5 w-3.5 text-muted-foreground transition-transform',
              expanded && 'rotate-180',
            )}
          />
        </div>
      </button>

      {expanded && (
        <div className="space-y-3 border-t border-border bg-muted/20 px-4 py-3">
          {errMsg && (
            <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2">
              <div className="mb-0.5 text-[10px] uppercase tracking-wide text-destructive/70">
                error
              </div>
              <pre className="whitespace-pre-wrap break-words font-mono text-xs text-destructive">
                {errMsg}
              </pre>
            </div>
          )}

          <dl className="grid grid-cols-1 gap-x-4 gap-y-1.5 text-xs sm:grid-cols-2">
            {log.target_id && (
              <DetailRow label="Bill">
                <Link
                  to={`/bills/${log.target_id}`}
                  className="font-mono text-primary hover:underline"
                  onClick={(e) => e.stopPropagation()}
                >
                  {log.target_id.slice(0, 8)}…
                </Link>
              </DetailRow>
            )}
            {docNo && (
              <DetailRow label="doc_no">
                <span className="font-mono">{docNo}</span>
              </DetailRow>
            )}
            {log.detail?.route && (
              <DetailRow label="route">
                <span className="font-mono">{String(log.detail.route)}</span>
              </DetailRow>
            )}
            {log.detail?.via && (
              <DetailRow label="via">
                <span className="font-mono">{String(log.detail.via)}</span>
              </DetailRow>
            )}
            {log.user_id && (
              <DetailRow label="user">
                <span className="font-mono">{log.user_id.slice(0, 8)}…</span>
              </DetailRow>
            )}
            {log.trace_id && (
              <DetailRow label="trace">
                <CopyChip value={log.trace_id} label="trace" />
              </DetailRow>
            )}
          </dl>

          <div>
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation()
                setShowRaw((v) => !v)
              }}
              className="inline-flex items-center gap-1.5 text-[11px] text-muted-foreground hover:text-foreground"
            >
              <Code2 className="h-3 w-3" />
              {showRaw ? 'ซ่อน raw JSON' : 'ดู raw JSON'}
            </button>
            {showRaw && (
              <div className="mt-2">
                <JsonViewer title="detail" data={log.detail ?? {}} />
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function DetailRow({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex items-center gap-2">
      <dt className="text-[10px] uppercase tracking-wide text-muted-foreground">
        {label}
      </dt>
      <dd className="text-foreground">{children}</dd>
    </div>
  )
}

interface DateGroup {
  key: string
  label: string
  items: AuditLog[]
}

function groupByDate(logs: AuditLog[]): DateGroup[] {
  const today = dayjs().startOf('day')
  const yesterday = today.subtract(1, 'day')
  const groups: Record<string, DateGroup> = {}

  for (const log of logs) {
    const d = dayjs(log.created_at).startOf('day')
    let key: string
    let label: string
    if (d.isSame(today)) {
      key = 'today'
      label = 'วันนี้'
    } else if (d.isSame(yesterday)) {
      key = 'yesterday'
      label = 'เมื่อวาน'
    } else {
      key = d.format('YYYY-MM-DD')
      label = d.format('D MMM YYYY')
    }
    if (!groups[key]) groups[key] = { key, label, items: [] }
    groups[key].items.push(log)
  }

  return Object.values(groups)
}

export default function Logs() {
  const [logs, setLogs] = useState<AuditLog[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [source, setSource] = useState<string>(ALL)
  const [action, setAction] = useState<string>(ALL)
  const [level, setLevel] = useState<string>(ALL)
  const [dateFrom, setDateFrom] = useState('')
  const [dateTo, setDateTo] = useState('')
  const pageSize = 50

  const load = async (p = page) => {
    setLoading(true)
    try {
      const params: Record<string, string | number> = { page: p, page_size: pageSize }
      if (source !== ALL) params.source = source
      if (action !== ALL) params.action = action
      if (level !== ALL) params.level = level
      if (dateFrom) params.date_from = dateFrom
      if (dateTo) params.date_to = dateTo
      const res = await api.get<LogsResponse>('/api/logs', { params })
      setLogs(res.data.data || [])
      setTotal(res.data.total || 0)
      setPage(p)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load(1)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [source, action, level, dateFrom, dateTo])

  const totalPages = Math.max(1, Math.ceil(total / pageSize))
  const hasFilters =
    source !== ALL || action !== ALL || level !== ALL || !!dateFrom || !!dateTo

  const resetFilters = () => {
    setSource(ALL)
    setAction(ALL)
    setLevel(ALL)
    setDateFrom('')
    setDateTo('')
  }

  // Stats: count errors + warnings within current page result for quick scan
  const errorCount = useMemo(
    () => logs.filter((l) => l.level === 'error').length,
    [logs],
  )
  const warnCount = useMemo(
    () => logs.filter((l) => l.level === 'warn').length,
    [logs],
  )

  const grouped = useMemo(() => groupByDate(logs), [logs])

  return (
    <TooltipProvider>
      <div className="space-y-5">
        <PageHeader
          title="Activity Log"
          description="ประวัติทุก event — สร้างบิล / ส่ง SML / ผลลัพธ์ — คลิกแถวเพื่อดูรายละเอียด"
          actions={
            <Button
              variant="outline"
              size="sm"
              className="gap-1.5"
              onClick={() => load(page)}
              disabled={loading}
            >
              <RotateCw className={cn('h-3.5 w-3.5', loading && 'animate-spin')} />
              รีเฟรช
            </Button>
          }
        />

        <Card>
          <CardContent className="flex flex-wrap items-end gap-3 p-4">
            <div className="space-y-1.5">
              <Label className="text-xs text-muted-foreground">ช่องทาง</Label>
              <Select value={source} onValueChange={setSource}>
                <SelectTrigger className="h-9 w-[150px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL}>ทั้งหมด</SelectItem>
                  <SelectItem value="line">LINE</SelectItem>
                  <SelectItem value="email">Email</SelectItem>
                  <SelectItem value="shopee_email">Shopee Email</SelectItem>
                  <SelectItem value="shopee_shipped">Shopee Shipped</SelectItem>
                  <SelectItem value="shopee_excel">Shopee Excel</SelectItem>
                  <SelectItem value="lazada">Lazada</SelectItem>
                  <SelectItem value="sml">SML</SelectItem>
                  <SelectItem value="catalog">Catalog</SelectItem>
                  <SelectItem value="channel_defaults">Settings</SelectItem>
                  <SelectItem value="system">System</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-muted-foreground">Action</Label>
              <Select value={action} onValueChange={setAction}>
                <SelectTrigger className="h-9 w-[200px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL}>ทั้งหมด</SelectItem>
                  {Object.entries(ACTION_META).map(([key, meta]) => (
                    <SelectItem key={key} value={key}>
                      {meta.emoji} {meta.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-muted-foreground">ระดับ</Label>
              <Select value={level} onValueChange={setLevel}>
                <SelectTrigger className="h-9 w-[120px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL}>ทั้งหมด</SelectItem>
                  <SelectItem value="info">info</SelectItem>
                  <SelectItem value="warn">warn</SelectItem>
                  <SelectItem value="error">error</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-muted-foreground" htmlFor="d-from">
                ตั้งแต่
              </Label>
              <Input
                id="d-from"
                type="date"
                value={dateFrom}
                onChange={(e) => setDateFrom(e.target.value)}
                className="h-9 w-[150px]"
              />
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-muted-foreground" htmlFor="d-to">
                ถึง
              </Label>
              <Input
                id="d-to"
                type="date"
                value={dateTo}
                onChange={(e) => setDateTo(e.target.value)}
                className="h-9 w-[150px]"
              />
            </div>
            {hasFilters && (
              <Button variant="ghost" size="sm" onClick={resetFilters} className="ml-auto">
                <Filter className="h-3.5 w-3.5" />
                ล้างตัวกรอง
              </Button>
            )}
          </CardContent>
        </Card>

        <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
          <span>
            พบ <span className="font-medium text-foreground">{total.toLocaleString()}</span> รายการ
          </span>
          {errorCount > 0 && (
            <span className="text-destructive">· error {errorCount}</span>
          )}
          {warnCount > 0 && <span className="text-warning">· warn {warnCount}</span>}
        </div>

        <div className="space-y-4">
          {loading ? (
            <div className="space-y-2">
              {Array.from({ length: 8 }).map((_, i) => (
                <Skeleton key={i} className="h-16 w-full rounded-lg" />
              ))}
            </div>
          ) : logs.length === 0 ? (
            <EmptyState
              icon={ScrollText}
              title="ยังไม่มี activity"
              description={
                hasFilters
                  ? 'ลองล้างตัวกรองหรือขยายช่วงวันที่'
                  : 'เมื่อระบบทำงานจะมีประวัติแสดงที่นี่'
              }
            />
          ) : (
            grouped.map((g) => (
              <div key={g.key} className="space-y-1.5">
                <div className="flex items-center gap-2 px-1 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
                  <span>{g.label}</span>
                  <span className="text-muted-foreground/60">· {g.items.length}</span>
                  <div className="h-px flex-1 bg-border" />
                </div>
                <div className="space-y-1.5">
                  {g.items.map((log) => (
                    <LogRow key={log.id} log={log} />
                  ))}
                </div>
              </div>
            ))
          )}
        </div>

        {totalPages > 1 && (
          <div className="flex items-center justify-end gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={page <= 1}
              onClick={() => load(page - 1)}
            >
              ก่อนหน้า
            </Button>
            <span className="px-2 text-xs tabular-nums text-muted-foreground">
              หน้า {page} / {totalPages}
            </span>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages}
              onClick={() => load(page + 1)}
            >
              ถัดไป
            </Button>
          </div>
        )}
      </div>
    </TooltipProvider>
  )
}
