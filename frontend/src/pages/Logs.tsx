import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import 'dayjs/locale/th'
import {
  AlertTriangle,
  ChevronDown,
  CheckCircle2,
  Code2,
  Copy,
  Database,
  FileText,
  Filter,
  RotateCw,
  ScrollText,
} from 'lucide-react'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
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
import { DateRangePicker } from '@/components/common/DateRangePicker'
import { JsonViewer } from '@/components/common/JsonViewer'
import { PageHeader } from '@/components/common/PageHeader'
import api from '@/api/client'
import { cn } from '@/lib/utils'
import {
  ACTION_META,
  SOURCE_LABELS,
  SOURCE_TONE,
  TONE_DOT,
  smlRouteLabel,
  type ActionMeta,
  type AuditLog,
  type Tone,
  summarize,
} from '@/lib/audit-log-meta'

dayjs.extend(relativeTime)
dayjs.locale('th')

interface LogsResponse {
  data: AuditLog[]
  total: number
  page: number
  page_size: number
}

const ALL = '__all__'
const PHASE = Number(import.meta.env.VITE_PHASE ?? 99)

// Action keys that belong to Phase 2+ (LINE chat, chat tags, etc.)
const PHASE2_ACTIONS = new Set([
  'line_admin_reply', 'line_admin_send_media', 'line_conversation_status',
  'line_message_received', 'line_oa_created', 'line_oa_updated', 'line_oa_deleted',
  'chat_phone_saved', 'chat_note_created', 'chat_note_updated', 'chat_note_deleted',
  'chat_tag_created', 'chat_tag_updated', 'chat_tag_deleted', 'chat_conv_tags_set',
  'chat_quick_reply_created', 'chat_quick_reply_updated', 'chat_quick_reply_deleted',
])


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

interface LogFact {
  label: string
  value?: React.ReactNode
  mono?: boolean
  copyValue?: string
  tone?: 'normal' | 'danger' | 'muted'
}

function parseDetailError(log: AuditLog): Record<string, any> {
  const err = log.detail?.error
  if (err && typeof err === 'object' && !Array.isArray(err)) return err
  if (typeof err !== 'string') return {}
  try {
    const parsed = JSON.parse(err)
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {}
  } catch {
    return {}
  }
}

function compact(value: unknown, max = 90): string {
  if (value == null || value === '') return ''
  const text = String(value)
  return text.length > max ? `${text.slice(0, max)}…` : text
}

function makeFacts(log: AuditLog): LogFact[] {
  const d = log.detail ?? {}
  const parsedError = parseDetailError(log)
  const facts: LogFact[] = []

  if (log.target_id) {
    facts.push({
      label: 'บิล',
      value: (
        <Link
          to={`/bills/${log.target_id}`}
          className="font-mono text-primary hover:underline"
          onClick={(e) => e.stopPropagation()}
        >
          {log.target_id.slice(0, 8)}…
        </Link>
      ),
      copyValue: log.target_id,
    })
  }

  const docNo = d.doc_no ?? parsedError.doc_no_attempted
  if (docNo) facts.push({ label: 'เลขเอกสาร SML', value: docNo, mono: true, copyValue: String(docNo) })
  if (d.route ?? parsedError.route) {
    const route = d.route ?? parsedError.route
    facts.push({ label: 'ปลายทาง SML', value: smlRouteLabel(route), mono: false })
  }
  if (d.via) facts.push({ label: 'วิธีส่ง', value: d.via, mono: true })
  if (d.subject) facts.push({ label: 'หัวข้ออีเมล', value: compact(d.subject, 140) })
  if (d.message_id) facts.push({ label: 'Message ID', value: compact(d.message_id, 64), mono: true, copyValue: String(d.message_id) })
  if (d.raw_name) facts.push({ label: 'ชื่อจากบิล', value: compact(d.raw_name, 140) })
  if (d.item_code ?? d.code) facts.push({ label: 'รหัสสินค้า', value: d.item_code ?? d.code, mono: true, copyValue: String(d.item_code ?? d.code) })
  if (d.unit_code) facts.push({ label: 'หน่วย', value: d.unit_code })
  if (d.name) facts.push({ label: 'ชื่อสินค้า', value: compact(d.name, 140) })
  if (d.channel) facts.push({ label: 'ช่องทาง', value: d.channel })
  if (d.bill_type) facts.push({ label: 'ประเภทบิล', value: d.bill_type })
  if (d.party_code) facts.push({ label: 'คู่ค้า', value: d.party_code, mono: true })
  if (log.duration_ms != null) facts.push({ label: 'เวลาใช้', value: `${log.duration_ms.toLocaleString()}ms`, mono: true })
  if (log.trace_id) facts.push({ label: 'Trace', value: <CopyChip value={log.trace_id} label="trace" /> })

  return facts.filter((fact) => fact.value != null && fact.value !== '')
}

function LogExpandedSummary({
  log,
  onRetry,
  retrying,
  canRetry,
}: {
  log: AuditLog
  onRetry: (e: React.MouseEvent) => void
  retrying: boolean
  canRetry: boolean
}) {
  const d = log.detail ?? {}
  const parsedError = parseDetailError(log)
  const errorMessage = parsedError.error ?? d.error
  const facts = makeFacts(log)
  const isSmlSent = log.action === 'sml_sent'
  const isSmlFailed = log.action === 'sml_failed'

  return (
    <div className="space-y-2">
      {isSmlSent && (
        <div className="flex items-start gap-2 rounded-md border border-success/25 bg-success/10 px-3 py-2">
          <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-success" />
          <div className="min-w-0">
            <div className="text-sm font-semibold text-foreground">ส่งเข้า SML สำเร็จ</div>
            <p className="mt-0.5 text-xs text-muted-foreground">
              ระบบบันทึกเลขเอกสารและ response ไว้แล้ว ตรวจต่อได้จากหน้ารายละเอียดบิล
            </p>
          </div>
        </div>
      )}

      {isSmlFailed && (
        <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2">
          <div className="flex items-start justify-between gap-3">
            <div className="flex min-w-0 gap-2">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-destructive" />
              <div className="min-w-0">
                <div className="text-sm font-semibold text-destructive">ส่งเข้า SML ไม่สำเร็จ</div>
                {errorMessage && (
                  <p className="mt-1 whitespace-pre-wrap break-words font-mono text-xs text-destructive">
                    {String(errorMessage)}
                  </p>
                )}
              </div>
            </div>
            {canRetry && (
              <Button
                variant="outline"
                size="sm"
                onClick={onRetry}
                disabled={retrying}
                className="h-7 shrink-0 gap-1.5 px-2 text-[11px]"
              >
                <RotateCw className={cn('h-3 w-3', retrying && 'animate-spin')} />
                {retrying ? 'กำลัง retry…' : 'Retry'}
              </Button>
            )}
          </div>
        </div>
      )}

      {!isSmlSent && !isSmlFailed && (log.action.includes('email') || log.action.includes('shopee')) && (
        <div className="flex items-start gap-2 rounded-md border bg-background px-3 py-2">
          <FileText className="mt-0.5 h-4 w-4 shrink-0 text-info" />
          <div className="min-w-0">
            <div className="text-sm font-semibold text-foreground">ข้อมูลจากช่องทางต้นทาง</div>
            <p className="mt-0.5 text-xs text-muted-foreground">
              ใช้ดูว่าอีเมลหรือไฟล์ใดเป็นต้นทางของบิล และใช้ trace กลับตอนตรวจซ้ำ
            </p>
          </div>
        </div>
      )}

      <div className="grid gap-1.5 sm:grid-cols-2 xl:grid-cols-3">
        {facts.map((fact, idx) => (
          <div key={`${fact.label}-${idx}`} className="rounded-md border bg-background px-2.5 py-1.5">
            <div className="text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
              {fact.label}
            </div>
            <div
              className={cn(
                'mt-0.5 min-w-0 break-words text-sm leading-5 text-foreground',
                fact.mono && 'font-mono text-xs',
                fact.tone === 'danger' && 'text-destructive',
                fact.tone === 'muted' && 'text-muted-foreground',
              )}
            >
              {fact.value}
            </div>
            {fact.copyValue && fact.label !== 'Trace' && (
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation()
                  navigator.clipboard?.writeText(fact.copyValue ?? '')
                }}
                className="mt-1 text-[10px] text-muted-foreground hover:text-foreground"
              >
                คัดลอก
              </button>
            )}
          </div>
        ))}
      </div>

      {log.action === 'sml_sent' && (
        <div className="flex items-center gap-2 rounded-md bg-muted/50 px-3 py-2 text-xs text-muted-foreground">
          <Database className="h-3.5 w-3.5" />
          <span>ดูข้อมูลที่ส่งและผลตอบกลับฉบับเต็มได้ในหน้ารายละเอียดบิล</span>
        </div>
      )}
    </div>
  )
}

function LogRow({ log, onRetried }: { log: AuditLog; onRetried: () => void }) {
  const [expanded, setExpanded] = useState(false)
  const [showRaw, setShowRaw] = useState(false)
  const [retrying, setRetrying] = useState(false)

  const meta = ACTION_META[log.action] ?? {
    label: log.action,
    emoji: '•',
    tone: 'muted' as Tone,
  }
  const summary = summarize(log)
  const isError = log.level === 'error'
  const source = log.source ?? ''
  const docNo = String(log.detail?.doc_no ?? '')
  // Inline retry available only on sml_failed rows that have a bill target.
  const canRetry = log.action === 'sml_failed' && !!log.target_id

  const handleRetry = async (e: React.MouseEvent) => {
    e.stopPropagation()
    if (!log.target_id || retrying) return
    setRetrying(true)
    try {
      await api.post(`/api/bills/${log.target_id}/retry`)
      toast.success('ส่งใหม่สำเร็จ — โหลด log ใหม่')
      onRetried()
    } catch (err: any) {
      toast.error(
        'Retry ล้มเหลว: ' +
          (err?.response?.data?.error ?? err?.message ?? 'unknown'),
      )
    } finally {
      setRetrying(false)
    }
  }

  return (
    <div
      className={cn(
        'rounded-md border bg-card transition-colors',
        isError
          ? 'border-destructive/30 bg-destructive/5'
          : expanded
            ? 'border-primary/25 bg-primary/[0.025]'
            : 'border-border hover:bg-accent/30',
      )}
    >
      {/* Row is a div not a button so we can nest a Retry <button> inside
          (HTML doesn't allow button-in-button). Keyboard a11y: Enter/Space
          toggle expanded, role=button + tabIndex for screen readers. */}
      <div
        role="button"
        tabIndex={0}
        onClick={() => setExpanded((v) => !v)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            setExpanded((v) => !v)
          }
        }}
        className="flex w-full cursor-pointer items-start gap-2.5 px-3 py-2 text-left focus:outline-none focus-visible:ring-2 focus-visible:ring-primary/40"
      >
        <span
          className={cn(
            'mt-1 inline-block h-2 w-2 shrink-0 rounded-full',
            TONE_DOT[meta.tone],
          )}
        />

        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span className="text-sm leading-none">{meta.emoji}</span>
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
            {/* Delivery-method chip for LINE outgoing — tells admin at a glance
                whether the message used the free Reply API or paid Push quota. */}
            {(log.action === 'line_admin_reply' || log.action === 'line_admin_send_media') &&
              log.detail?.delivery_method === 'reply' && (
                <Badge
                  variant="secondary"
                  className="h-5 px-1.5 text-[10px] font-medium bg-success/15 text-success"
                  title="ส่งผ่าน Reply API — ไม่นับ quota"
                >
                  ฟรี
                </Badge>
              )}
            {(log.action === 'line_admin_reply' || log.action === 'line_admin_send_media') &&
              log.detail?.delivery_method === 'push' && (
                <Badge
                  variant="secondary"
                  className="h-5 px-1.5 text-[10px] font-medium"
                  title="ส่งผ่าน Push API — นับ quota เดือนนี้"
                >
                  Push
                </Badge>
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
                'mt-0.5 line-clamp-1 text-xs',
                isError ? 'text-destructive' : 'text-muted-foreground',
              )}
            >
              {summary}
            </p>
          )}
        </div>

        <div className="flex shrink-0 items-center gap-2">
          {/* Inline retry — visible at row level (not just expanded) on
              sml_failed rows. Saves the click to expand + the trip to
              /bills/:id. Stop click bubbling so the row doesn't toggle. */}
          {canRetry && !expanded && (
            <Tooltip delayDuration={300}>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={handleRetry}
                  disabled={retrying}
                  className="h-7 w-7 shrink-0 p-0 text-destructive hover:bg-destructive/10 hover:text-destructive"
                >
                  <RotateCw className={cn('h-3.5 w-3.5', retrying && 'animate-spin')} />
                </Button>
              </TooltipTrigger>
              <TooltipContent side="left">
                Retry บิลนี้
              </TooltipContent>
            </Tooltip>
          )}
          <div className="flex flex-col items-end gap-0.5 text-right">
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
                  log.duration_ms > 30000
                    ? 'text-destructive'
                    : log.duration_ms > 10000
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
        </div>
      </div>

      {expanded && (
        <div className="space-y-2 border-t border-border bg-muted/20 px-3 py-2.5">
          <LogExpandedSummary
            log={log}
            onRetry={handleRetry}
            retrying={retrying}
            canRetry={canRetry}
          />

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
              {showRaw ? 'ซ่อนข้อมูลดิบ' : 'ดูข้อมูลดิบ JSON'}
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
      <div className="space-y-4">
        <PageHeader
          title="ประวัติการทำงาน"
          description="ตรวจย้อนหลังว่าระบบดึงอีเมล สร้างบิล และส่งเข้า SML สำเร็จหรือไม่"
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

        <Card className="shadow-none">
          <CardContent className="flex flex-wrap items-end gap-2 p-3">
            <div className="space-y-1.5">
              <Label className="text-xs text-muted-foreground">ช่องทาง</Label>
              <Select value={source} onValueChange={setSource}>
                <SelectTrigger className="h-8 w-[142px] text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL}>ทั้งหมด</SelectItem>
                  {PHASE >= 2 && <SelectItem value="line">LINE</SelectItem>}
                  <SelectItem value="email">Email</SelectItem>
                  <SelectItem value="shopee_email">Shopee Email</SelectItem>
                  <SelectItem value="shopee_shipped">Shopee Shipped</SelectItem>
                  {PHASE >= 2 && <SelectItem value="shopee_excel">Shopee Excel</SelectItem>}
                  {PHASE >= 2 && <SelectItem value="lazada">Lazada</SelectItem>}
                  <SelectItem value="sml">SML</SelectItem>
                  <SelectItem value="catalog">Catalog</SelectItem>
                  <SelectItem value="channel_defaults">Settings</SelectItem>
                  <SelectItem value="system">System</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-muted-foreground">เหตุการณ์</Label>
              <Select value={action} onValueChange={setAction}>
                <SelectTrigger className="h-8 w-[190px] text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL}>ทั้งหมด</SelectItem>
                  {Object.entries(ACTION_META)
                    .filter(([key]) => PHASE >= 2 || !PHASE2_ACTIONS.has(key))
                    .map(([key, meta]) => (
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
                <SelectTrigger className="h-8 w-[112px] text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={ALL}>ทั้งหมด</SelectItem>
                  <SelectItem value="info">info</SelectItem>
                  <SelectItem value="warn">คำเตือน</SelectItem>
                  <SelectItem value="error">ผิดพลาด</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-1.5">
              <Label className="text-xs text-muted-foreground">
                วันที่
              </Label>
              <DateRangePicker
                from={dateFrom}
                to={dateTo}
                onFromChange={setDateFrom}
                onToChange={setDateTo}
                className="h-8 min-w-[190px] text-xs"
              />
            </div>
            {hasFilters && (
              <Button variant="ghost" size="sm" onClick={resetFilters} className="ml-auto h-8 text-xs">
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
            <span className="text-destructive">· ผิดพลาด {errorCount}</span>
          )}
          {warnCount > 0 && <span className="text-warning">· คำเตือน {warnCount}</span>}
        </div>

        <div className="space-y-3">
          {loading ? (
            <div className="space-y-2">
              {Array.from({ length: 8 }).map((_, i) => (
                <Skeleton key={i} className="h-16 w-full rounded-lg" />
              ))}
            </div>
          ) : logs.length === 0 ? (
            <EmptyState
              icon={ScrollText}
              title="ยังไม่มีประวัติ"
              description={
                hasFilters
                  ? 'ลองล้างตัวกรองหรือขยายช่วงวันที่'
                  : 'เมื่อระบบทำงานจะมีประวัติแสดงที่นี่'
              }
            />
          ) : (
            grouped.map((g) => (
              <div key={g.key} className="space-y-1">
                <div className="flex items-center gap-2 px-1 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
                  <span>{g.label}</span>
                  <span className="text-muted-foreground/60">· {g.items.length}</span>
                  <div className="h-px flex-1 bg-border" />
                </div>
                <div className="space-y-1">
                  {g.items.map((log) => (
                    <LogRow key={log.id} log={log} onRetried={() => load(page)} />
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
