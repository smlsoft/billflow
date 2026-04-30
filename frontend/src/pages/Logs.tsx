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
import { toast } from 'sonner'

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
import {
  ACTION_META,
  SOURCE_LABELS,
  SOURCE_TONE,
  TONE_DOT,
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
  const errMsg = String(log.detail?.error ?? '')
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
        'rounded-lg border bg-card transition-colors',
        isError
          ? 'border-destructive/30 bg-destructive/5'
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
        className="flex w-full cursor-pointer items-start gap-3 px-4 py-3 text-left focus:outline-none focus-visible:ring-2 focus-visible:ring-primary/40"
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
                'mt-0.5 line-clamp-2 text-xs',
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
        </div>
      </div>

      {expanded && (
        <div className="space-y-3 border-t border-border bg-muted/20 px-4 py-3">
          {errMsg && (
            <div className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2">
              <div className="mb-0.5 flex items-center justify-between gap-2">
                <span className="text-[10px] uppercase tracking-wide text-destructive/70">
                  error
                </span>
                {/* Inline retry — visible only on sml_failed rows tied to a bill.
                    Saves the trip to /bills/:id just to click Retry. */}
                {canRetry && (
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleRetry}
                    disabled={retrying}
                    className="h-6 gap-1 px-2 text-[11px]"
                  >
                    <RotateCw className={cn('h-3 w-3', retrying && 'animate-spin')} />
                    {retrying ? 'กำลัง retry…' : 'Retry บิลนี้'}
                  </Button>
                )}
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
