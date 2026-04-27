import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import dayjs from 'dayjs'
import { Copy, Filter, ScrollText } from 'lucide-react'

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
import { EmptyState } from '@/components/common/EmptyState'
import { JsonViewer } from '@/components/common/JsonViewer'
import { PageHeader } from '@/components/common/PageHeader'
import { StatusDot } from '@/components/common/StatusDot'
import api from '@/api/client'
import { cn } from '@/lib/utils'

interface AuditLog {
  id: string
  user_id?: string
  action: string
  target_id?: string
  source?: string
  level?: string
  duration_ms?: number
  trace_id?: string
  detail?: Record<string, unknown>
  created_at: string
}

interface LogsResponse {
  data: AuditLog[]
  total: number
  page: number
  page_size: number
}

const ALL = '__all__'

const ACTION_CONFIG: Record<
  string,
  { label: string; emoji: string; tone: 'success' | 'warning' | 'danger' | 'info' | 'muted' | 'primary' }
> = {
  bill_created: { label: 'สร้างบิล', emoji: '📥', tone: 'info' },
  sml_sent: { label: 'ส่ง SML สำเร็จ', emoji: '✅', tone: 'success' },
  sml_failed: { label: 'SML ล้มเหลว', emoji: '❌', tone: 'danger' },
  bill_pending: { label: 'รอ confirm', emoji: '⏳', tone: 'warning' },
  bill_retry: { label: 'Retry', emoji: '🔄', tone: 'primary' },
}

const SOURCE_LABELS: Record<string, string> = {
  line: 'LINE',
  line_oa: 'LINE',
  email: 'Email',
  lazada: 'Lazada',
  shopee: 'Shopee',
  shopee_excel: 'Shopee',
  manual: 'Manual',
  sml: 'SML',
  system: 'System',
}

function DurationBadge({ ms }: { ms?: number }) {
  if (!ms) return null
  const tone =
    ms > 3000 ? 'text-destructive' : ms > 1000 ? 'text-warning' : 'text-muted-foreground'
  return <span className={cn('font-mono text-[11px] tabular-nums', tone)}>{ms}ms</span>
}

function TraceIdChip({ id }: { id?: string }) {
  if (!id) return null
  const short = id.length > 20 ? id.slice(0, 16) + '…' : id
  return (
    <button
      type="button"
      className="group inline-flex items-center gap-1 font-mono text-[11px] text-muted-foreground hover:text-foreground"
      onClick={() => navigator.clipboard?.writeText(id)}
      title={`คัดลอก trace_id: ${id}`}
    >
      {short}
      <Copy className="h-3 w-3 opacity-0 transition-opacity group-hover:opacity-100" />
    </button>
  )
}

function LogRow({ log }: { log: AuditLog }) {
  const cfg = ACTION_CONFIG[log.action] ?? {
    label: log.action,
    emoji: '•',
    tone: 'muted' as const,
  }
  const source = log.source || (log.detail?.source as string) || ''
  const docNo = (log.detail?.doc_no as string) || ''
  const errMsg = (log.detail?.error as string) || ''
  const via = (log.detail?.via as string) || ''
  const isError = log.level === 'error'

  return (
    <Card
      className={cn(
        'border-l-2 transition-colors',
        isError
          ? 'border-l-destructive bg-destructive/5'
          : 'border-l-transparent',
      )}
    >
      <CardContent className="space-y-2 p-4">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs">
          <StatusDot variant={cfg.tone} label={`${cfg.emoji} ${cfg.label}`} />
          {source && (
            <Badge variant="secondary" className="h-5 px-1.5 text-[10px] font-medium">
              {SOURCE_LABELS[source] ?? source}
            </Badge>
          )}
          {via && (
            <span className="text-muted-foreground">
              via <span className="font-medium text-foreground">{via}</span>
            </span>
          )}
          {docNo && (
            <span className="font-mono text-[11px] font-medium text-foreground">{docNo}</span>
          )}
          {log.level && log.level !== 'info' && (
            <Badge
              variant={isError ? 'destructive' : 'secondary'}
              className="h-5 px-1.5 text-[10px] font-medium uppercase"
            >
              {log.level}
            </Badge>
          )}
          <DurationBadge ms={log.duration_ms} />
          <TraceIdChip id={log.trace_id} />
          <span className="ml-auto text-[11px] tabular-nums text-muted-foreground">
            {dayjs(log.created_at).format('DD/MM/YY HH:mm:ss')}
          </span>
        </div>

        {log.target_id && (
          <p className="text-xs text-muted-foreground">
            Bill ID:{' '}
            <Link
              to={`/bills/${log.target_id}`}
              className="font-mono text-primary hover:underline"
            >
              {log.target_id}
            </Link>
          </p>
        )}

        {errMsg && (
          <p className="rounded-md bg-destructive/10 px-3 py-1.5 font-mono text-xs text-destructive">
            {errMsg}
          </p>
        )}

        <div className="space-y-1.5">
          {log.detail?.raw_data != null && (
            <JsonViewer title="raw_data" data={log.detail.raw_data} />
          )}
          {log.detail?.sml_payload != null && (
            <JsonViewer title="sml_payload" data={log.detail.sml_payload} />
          )}
          {log.detail?.sml_response != null && (
            <JsonViewer title="sml_response" data={log.detail.sml_response} />
          )}
        </div>
      </CardContent>
    </Card>
  )
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
  const pageSize = 30

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
    source !== ALL || action !== ALL || level !== ALL || dateFrom || dateTo

  const resetFilters = () => {
    setSource(ALL)
    setAction(ALL)
    setLevel(ALL)
    setDateFrom('')
    setDateTo('')
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="Activity Log"
        description="บันทึกทุก event — รับอะไรมา ส่งอะไรไป SML ผลลัพธ์เป็นอย่างไร"
      />

      <Card>
        <CardContent className="flex flex-wrap items-end gap-3 p-4">
          <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground">Channel</Label>
            <Select value={source} onValueChange={setSource}>
              <SelectTrigger className="h-9 w-[140px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>ทั้งหมด</SelectItem>
                <SelectItem value="line">LINE</SelectItem>
                <SelectItem value="email">Email</SelectItem>
                <SelectItem value="lazada">Lazada</SelectItem>
                <SelectItem value="shopee_excel">Shopee</SelectItem>
                <SelectItem value="sml">SML</SelectItem>
                <SelectItem value="system">System</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground">Action</Label>
            <Select value={action} onValueChange={setAction}>
              <SelectTrigger className="h-9 w-[180px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>ทั้งหมด</SelectItem>
                <SelectItem value="bill_created">สร้างบิล</SelectItem>
                <SelectItem value="sml_sent">ส่ง SML สำเร็จ</SelectItem>
                <SelectItem value="sml_failed">SML ล้มเหลว</SelectItem>
                <SelectItem value="bill_pending">รอ confirm</SelectItem>
                <SelectItem value="shopee_import_done">Shopee นำเข้าสำเร็จ</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground">Level</Label>
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
            <Label className="text-xs text-muted-foreground" htmlFor="d-from">ตั้งแต่</Label>
            <Input
              id="d-from"
              type="date"
              value={dateFrom}
              onChange={(e) => setDateFrom(e.target.value)}
              className="h-9 w-[150px]"
            />
          </div>
          <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground" htmlFor="d-to">ถึง</Label>
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

      <p className="text-xs text-muted-foreground">
        {loading ? 'กำลังโหลด…' : `พบ ${total.toLocaleString()} รายการ`}
      </p>

      <div className="space-y-2">
        {loading
          ? Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-24 w-full rounded-lg" />
            ))
          : logs.length === 0
            ? (
                <EmptyState
                  icon={ScrollText}
                  title="ยังไม่มี activity"
                  description={
                    hasFilters
                      ? 'ลองล้างตัวกรองหรือขยายช่วงวันที่'
                      : 'เมื่อระบบทำงานจะมีประวัติแสดงที่นี่'
                  }
                />
              )
            : logs.map((log) => <LogRow key={log.id} log={log} />)}
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
  )
}
