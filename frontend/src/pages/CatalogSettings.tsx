import { useEffect, useState, useRef, useCallback, useMemo, useReducer } from 'react'
import { Link } from 'react-router-dom'
import {
  AlertCircle,
  AlertTriangle,
  BookOpen,
  ChevronLeft,
  ChevronRight,
  Database,
  Loader2,
  RefreshCcw,
  RefreshCw,
  RotateCw,
  Search,
  Sparkles,
  Trash2,
  X,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { PageHeader } from '@/components/common/PageHeader'
import api from '@/api/client'
import { cn } from '@/lib/utils'
import { PAGE_TITLE } from '@/lib/labels'
import type { CatalogItem } from '@/types'

interface CatalogStats {
  total: number
  embedded: number
  pending: number
  error: number
  missing: number
  last_sync_at?: string | null
  index_size: number
  embed_running: boolean
  sync_running?: boolean
  sync_status?: {
    running: boolean
    started_at?: string
    finished_at?: string
    count: number
    missing?: number
    error?: string
  }
}

interface ListResponse {
  data: CatalogItem[]
  total: number
  page: number
  per_page: number
}

interface InstanceSettingsStatus {
  pending_restart?: boolean
  pending_restart_settings?: string[]
}

type StatusFilter = '' | 'pending' | 'done' | 'error' | 'missing'
interface FetchParams { page: number; filter: StatusFilter; query: string }

function fmtDateTime(value?: string | null) {
  if (!value) return '—'
  return new Date(value).toLocaleString('th-TH', {
    year: '2-digit',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function Pagination({
  page,
  total,
  perPage,
  onChange,
}: {
  page: number
  total: number
  perPage: number
  onChange: (p: number) => void
}) {
  const totalPages = Math.max(1, Math.ceil(total / perPage))
  if (totalPages <= 1) return null

  const pages: (number | '…')[] = []
  if (totalPages <= 7) {
    for (let i = 1; i <= totalPages; i++) pages.push(i)
  } else {
    pages.push(1)
    if (page > 3) pages.push('…')
    for (let i = Math.max(2, page - 1); i <= Math.min(totalPages - 1, page + 1); i++) pages.push(i)
    if (page < totalPages - 2) pages.push('…')
    pages.push(totalPages)
  }

  return (
    <div className="flex items-center gap-1">
      <Button
        size="icon"
        variant="outline"
        className="h-7 w-7"
        disabled={page <= 1}
        onClick={() => onChange(page - 1)}
        aria-label="หน้าก่อน"
      >
        <ChevronLeft className="h-3.5 w-3.5" />
      </Button>
      {pages.map((p, i) =>
        p === '…' ? (
          <span key={`e${i}`} className="px-1 text-xs text-muted-foreground">
            …
          </span>
        ) : (
          <Button
            key={p}
            size="sm"
            variant={page === p ? 'default' : 'ghost'}
            className="h-7 min-w-[28px] px-2 text-xs"
            onClick={() => onChange(p as number)}
          >
            {p}
          </Button>
        ),
      )}
      <Button
        size="icon"
        variant="outline"
        className="h-7 w-7"
        disabled={page >= totalPages}
        onClick={() => onChange(page + 1)}
        aria-label="หน้าถัดไป"
      >
        <ChevronRight className="h-3.5 w-3.5" />
      </Button>
    </div>
  )
}

function StatChip({
  label,
  value,
  variant = 'muted',
}: {
  label: string
  value: number | string
  variant?: 'success' | 'warning' | 'danger' | 'primary' | 'muted'
}) {
  const styles: Record<typeof variant, string> = {
    success: 'border-success/25 bg-success/5 text-success',
    warning: 'border-warning/25 bg-warning/5 text-warning',
    danger: 'border-destructive/25 bg-destructive/5 text-destructive',
    primary: 'border-primary/25 bg-primary/5 text-primary',
    muted: 'border-border bg-card text-foreground',
  }
  return (
    <Card className={cn('min-w-[150px] flex-1 shadow-none', styles[variant])}>
      <CardContent className="flex items-baseline justify-between gap-3 px-3 py-2.5">
        <p className="text-lg font-semibold tabular-nums leading-none">{value}</p>
        <p className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
          {label}
        </p>
      </CardContent>
    </Card>
  )
}

export default function CatalogSettings() {
  const [stats, setStats] = useState<CatalogStats | null>(null)
  const [items, setItems] = useState<CatalogItem[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [syncing, setSyncing] = useState(false)
  const [embedding, setEmbedding] = useState(false)
  const [message, setMessage] = useState<{ text: string; ok: boolean } | null>(null)
  const [pendingRestart, setPendingRestart] = useState(false)
  const [pendingRestartKeys, setPendingRestartKeys] = useState<string[]>([])
  const [draft, setDraft] = useState('')
  const [params, setParams] = useReducer(
    (_prev: FetchParams, next: Partial<FetchParams> & { reset?: boolean }) => {
      const base = next.reset ? { page: 1, filter: '' as StatusFilter, query: '' } : _prev
      return {
        ...base,
        page: next.page ?? 1,
        filter: next.filter ?? base.filter,
        query: next.query ?? base.query,
      }
    },
    { page: 1, filter: '', query: '' },
  )
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const PER_PAGE = 50

  const fetchStats = useCallback(async () => {
    const res = await api.get<CatalogStats>('/api/catalog/stats')
    setStats(res.data)
    return res.data
  }, [])

  const fetchInstanceStatus = useCallback(async () => {
    try {
      const res = await api.get<InstanceSettingsStatus>('/api/settings/instance')
      const pending = !!res.data.pending_restart
      setPendingRestart(pending)
      setPendingRestartKeys(res.data.pending_restart_settings ?? [])
      return pending
    } catch {
      setPendingRestart(false)
      setPendingRestartKeys([])
      return false
    }
  }, [])

  const fetchItems = useCallback(async (p: FetchParams) => {
    setLoading(true)
    try {
      const reqParams: Record<string, unknown> = { page: p.page, per_page: PER_PAGE }
      if (p.filter) reqParams.status = p.filter
      if (p.query.trim()) reqParams.q = p.query.trim()
      const res = await api.get<ListResponse>('/api/catalog', { params: reqParams })
      setItems(res.data.data ?? [])
      setTotal(res.data.total ?? 0)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchItems(params)
  }, [params, fetchItems])

  useEffect(() => {
    fetchStats()
    fetchInstanceStatus()
  }, [fetchStats, fetchInstanceStatus])

  useEffect(() => {
    if (stats?.embed_running || stats?.sync_running) {
      pollRef.current = setInterval(async () => {
        const s = await fetchStats()
        if (!s.embed_running && !s.sync_running) {
          if (pollRef.current) clearInterval(pollRef.current)
          fetchItems(params)
          if (s.sync_status?.error) {
            notify(`Sync ล้มเหลว: ${s.sync_status.error}`, false)
          } else if (stats?.sync_running && s.sync_status) {
            const missing = s.sync_status.missing ? ` · ไม่พบใน SML ${s.sync_status.missing} รายการ` : ''
            notify(`ซิงก์สินค้าสำเร็จ ${s.sync_status.count} รายการ${missing}`)
          }
        }
      }, 3000)
    } else {
      if (pollRef.current) clearInterval(pollRef.current)
    }
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [stats?.embed_running, stats?.sync_running])

  function notify(text: string, ok = true) {
    setMessage({ text, ok })
    setTimeout(() => setMessage(null), 4000)
  }

  function handleFilterChange(f: StatusFilter) {
    setDraft('')
    setParams({ filter: f, page: 1, query: '' })
  }

  function commitSearch(q: string) {
    setParams({ query: q.trim(), page: 1 })
  }

  function handleSearchKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') commitSearch(draft)
    if (e.key === 'Escape') {
      setDraft('')
      commitSearch('')
    }
  }

  async function handleSync() {
    if (isSyncBusy) return
    const hasPendingRestart = await fetchInstanceStatus()
    if (hasPendingRestart) {
      notify('มีการเปลี่ยนค่า SML ที่ยังไม่ได้รีสตาร์ท กรุณาไปที่การเชื่อมต่อระบบก่อน Sync', false)
      return
    }
    setSyncing(true)
    try {
      const res = await api.post<{ message?: string; sync_running?: boolean }>('/api/catalog/sync')
      notify(res.data.message === 'catalog sync already running' ? 'กำลังซิงก์สินค้าอยู่แล้ว' : 'เริ่มซิงก์สินค้าจาก SML แล้ว')
      fetchStats()
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string; pending_restart?: boolean; pending_restart_settings?: string[] } } })?.response?.data
      if (msg?.pending_restart) {
        setPendingRestart(true)
        setPendingRestartKeys(msg.pending_restart_settings ?? [])
      }
      notify(msg?.error ?? 'Sync ล้มเหลว', false)
    } finally {
      setSyncing(false)
    }
  }

  async function handleEmbedAll() {
    if (isEmbedBusy) return
    setEmbedding(true)
    try {
      const res = await api.post<{ message: string }>('/api/catalog/embed-all')
      notify(res.data.message ?? 'เริ่ม embed แล้ว')
      fetchStats()
    } catch {
      notify('Embed ล้มเหลว', false)
    } finally {
      setEmbedding(false)
    }
  }

  async function handleReload() {
    try {
      await api.post('/api/catalog/reload-index')
      notify('Reload index สำเร็จ')
      fetchStats()
    } catch {
      notify('Reload ล้มเหลว', false)
    }
  }

  async function handleEmbedOne(code: string) {
    if (busyRow?.code === code) return
    setBusyRow({ code, action: 'embed' })
    try {
      await api.post(`/api/catalog/${code}/embed`)
      notify(`Embed ${code} สำเร็จ`)
      fetchStats()
      fetchItems(params)
    } catch {
      notify(`Embed ${code} ล้มเหลว`, false)
    } finally {
      setBusyRow(null)
    }
  }

  // Tracks which row is currently running an action so we can disable
  // its buttons and show a spinner without blocking the rest of the table.
  const [busyRow, setBusyRow] = useState<{ code: string; action: 'embed' | 'refresh' | 'delete' } | null>(null)
  const [pendingDelete, setPendingDelete] = useState<string | null>(null)

  async function handleRefreshOne(code: string) {
    setBusyRow({ code, action: 'refresh' })
    try {
      await api.post(`/api/catalog/${code}/refresh`)
      notify(`รีเฟรช ${code} จาก SML สำเร็จ`)
      fetchItems(params)
    } catch (err: unknown) {
      const e = err as { response?: { status?: number; data?: { error?: string; not_found?: boolean } } }
      if (e?.response?.data?.not_found) {
        notify(`ไม่พบ ${code} ใน SML — ทำเครื่องหมายว่าไม่พบใน SML แล้ว`, false)
        fetchStats()
        fetchItems(params)
      } else {
        notify(e?.response?.data?.error ?? `รีเฟรช ${code} ล้มเหลว`, false)
      }
    } finally {
      setBusyRow(null)
    }
  }

  async function handleDeleteOne(code: string) {
    setBusyRow({ code, action: 'delete' })
    try {
      await api.delete(`/api/catalog/${code}`)
      notify(`ลบ ${code} จาก BillFlow แล้ว (SML ไม่ถูกแตะ)`)
      fetchStats()
      fetchItems(params)
    } catch (err: unknown) {
      const e = err as { response?: { data?: { error?: string } } }
      notify(e?.response?.data?.error ?? `ลบ ${code} ล้มเหลว`, false)
    } finally {
      setBusyRow(null)
      setPendingDelete(null)
    }
  }

  const pct = useMemo(
    () => (stats && stats.total > 0 ? Math.round((stats.embedded / stats.total) * 100) : 0),
    [stats],
  )

  const isEmbedBusy = embedding || (stats?.embed_running ?? false)
  const isSyncBusy = syncing || (stats?.sync_running ?? false)

  const tabs: Array<{ key: StatusFilter; label: string; count?: number }> = [
    { key: '', label: 'ทั้งหมด', count: stats?.total },
    { key: 'done', label: 'พร้อมจับคู่', count: stats?.embedded },
    { key: 'pending', label: 'รอเตรียมข้อมูล', count: stats?.pending },
    { key: 'error', label: 'มีปัญหา', count: stats?.error },
    { key: 'missing', label: 'ไม่พบใน SML', count: stats?.missing },
  ]

  return (
    <div className="space-y-4">
      <PageHeader
        title={PAGE_TITLE.catalog}
        description="รายการสินค้าจาก SML สำหรับจับคู่สินค้าแบบ manual-first กดซิงก์เมื่อมีการเพิ่ม ลบ หรือแก้ไขสินค้าใน SML"
        actions={
          <>
            <Button variant="outline" size="sm" onClick={handleSync} disabled={isSyncBusy || pendingRestart}>
              <RotateCw className={cn('h-3.5 w-3.5', isSyncBusy && 'animate-spin')} />
              {pendingRestart ? 'รอรีสตาร์ท SML' : isSyncBusy ? 'กำลังซิงก์…' : 'ซิงก์สินค้าจาก SML'}
            </Button>
            <Button size="sm" onClick={handleEmbedAll} disabled={isEmbedBusy}>
              {isEmbedBusy ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Sparkles className="h-3.5 w-3.5" />
              )}
              {isEmbedBusy ? 'กำลังเตรียมข้อมูล…' : 'สร้างข้อมูลจับคู่'}
            </Button>
            <Button variant="outline" size="sm" onClick={handleReload}>
              <RefreshCcw className="h-3.5 w-3.5" />
              โหลดรายการใหม่
            </Button>
          </>
        }
      />

      {pendingRestart && (
        <div className="rounded-lg border border-warning/35 bg-warning/[0.07] p-3 text-sm">
          <div className="flex items-start gap-2.5">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
            <div className="min-w-0 flex-1">
              <p className="font-medium text-foreground">ยัง Sync สินค้าไม่ได้ เพราะมีค่า SML ที่รอรีสตาร์ท</p>
              <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">
                ไปที่ <Link to="/settings/instance" className="font-medium text-primary hover:underline">การเชื่อมต่อระบบ</Link> แล้วกด “รีสตาร์ทและใช้ค่าทันที” ก่อน เพื่อให้ BillFlow ใช้ headers ชุดล่าสุด
              </p>
              {pendingRestartKeys.length > 0 && (
                <div className="mt-2 flex flex-wrap gap-1">
                  {pendingRestartKeys.map((key) => (
                    <Badge key={key} variant="outline" className="h-5 px-1.5 text-[10px]">
                      {key}
                    </Badge>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {stats?.sync_running && (
        <div className="rounded-lg border border-primary/25 bg-primary/[0.06] p-3 text-sm">
          <div className="flex items-start gap-2.5">
            <Loader2 className="mt-0.5 h-4 w-4 shrink-0 animate-spin text-primary" />
            <div>
              <p className="font-medium text-foreground">กำลัง Sync สินค้าจาก SML</p>
              <p className="mt-0.5 text-xs text-muted-foreground">
                งานนี้อาจใช้เวลาหลายนาที ปิดหน้านี้ได้ ระบบจะทำต่อบน server
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Catalog vs Mappings explainer — without this admins assume the two
          features are the same. Catalog is the local SML product cache;
          Mappings is the human-curated alias table. */}
      <details className="group rounded-lg border border-info/25 bg-info/[0.035] text-sm">
        <summary className="flex cursor-pointer list-none items-center justify-between gap-3 px-3.5 py-2.5">
          <span className="inline-flex min-w-0 items-center gap-2.5">
            <BookOpen className="h-4 w-4 shrink-0 text-info" strokeWidth={2.25} />
            <span className="font-medium text-foreground">Catalog คือรายการสินค้า SML ที่ BillFlow เก็บไว้สำหรับค้นหาและจับคู่</span>
          </span>
          <span className="text-[11px] text-primary group-open:hidden">รายละเอียด</span>
          <span className="hidden text-[11px] text-muted-foreground group-open:inline">ย่อ</span>
        </summary>
        <div className="border-t border-info/15 px-3.5 py-3">
          <div className="flex items-start gap-2.5">
          <BookOpen className="mt-0.5 h-4 w-4 shrink-0 text-info" strokeWidth={2.25} />
          <div className="min-w-0 flex-1 space-y-1.5">
            <p className="text-[13px] leading-relaxed text-muted-foreground">
              <span className="font-medium text-foreground">รายการสินค้าจาก SML</span>
              ที่ BillFlow ใช้เทียบกับชื่อสินค้าจากอีเมลและ marketplace แล้วแนะนำรหัสสินค้าในหน้าบิล
            </p>
            <div className="text-[12px] leading-relaxed text-muted-foreground">
              <span className="font-medium text-foreground">เมื่อแก้สินค้าใน SML:</span>
              <span className="ml-1">
                กด <span className="font-medium text-foreground">ซิงก์สินค้าจาก SML</span> แล้วค่อยกด{' '}
                <span className="font-medium text-foreground">สร้างข้อมูลจับคู่</span> เฉพาะรายการที่ยังรอเตรียมข้อมูล
              </span>
            </div>
            <p className="text-[12px] text-muted-foreground">
              ต่างจาก{' '}
              <Link to="/mappings" className="font-medium text-primary hover:underline">
                ตารางจับคู่สินค้า
              </Link>{' '}
              ที่เก็บชื่อสินค้าที่เคยแก้แล้ว — ใช้คู่กันแต่คนละขั้นตอน
            </p>
          </div>
        </div>
        </div>
      </details>

      <div className="rounded-lg border border-info/20 bg-info/[0.035] px-3.5 py-2.5 text-xs leading-relaxed text-muted-foreground">
        ค่าเริ่มต้นเป็นแบบกดซิงก์เอง ไม่มีการดึงสินค้าทั้งหมดถี่ ๆ อัตโนมัติ เพื่อลดภาระ SML, database และ token เตรียมข้อมูลค้นหา
      </div>

      {message && (
        <div
          className={cn(
            'fixed right-4 top-4 z-50 flex items-center gap-2 rounded-md border px-4 py-2.5 text-sm font-medium shadow-md',
            message.ok
              ? 'border-success/30 bg-success/10 text-success'
              : 'border-destructive/30 bg-destructive/10 text-destructive',
          )}
        >
          {message.ok ? '✓' : <AlertCircle className="h-4 w-4" />}
          {message.text}
        </div>
      )}

      {stats && (
        <div className="flex flex-wrap gap-2">
          <StatChip label="สินค้าใช้งานได้" value={stats.total.toLocaleString()} variant="primary" />
          <StatChip label="พร้อมจับคู่" value={stats.embedded.toLocaleString()} variant="success" />
          <StatChip label="รอเตรียมข้อมูล" value={stats.pending.toLocaleString()} variant="warning" />
          <StatChip label="โหลดไว้ใช้งาน" value={stats.index_size.toLocaleString()} variant="primary" />
          <StatChip label="ไม่พบใน SML" value={stats.missing.toLocaleString()} variant={stats.missing > 0 ? 'danger' : 'muted'} />
          <StatChip label="ซิงก์ล่าสุด" value={fmtDateTime(stats.last_sync_at)} variant="muted" />
          {stats.embed_running ? (
            <Card className="flex-1 border-primary/30 bg-primary/5">
              <CardContent className="flex items-center gap-3 px-4 py-3">
                <Loader2 className="h-4 w-4 animate-spin text-primary" />
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium text-primary">กำลังเตรียมข้อมูลจับคู่…</p>
                  <p className="text-[11px] text-muted-foreground">
                    รายการสินค้าเยอะ อาจใช้เวลาหลายนาที · ปิดหน้านี้ได้ · ระบบอัปเดตสถานะให้เอง
                  </p>
                </div>
              </CardContent>
            </Card>
          ) : stats.error > 0 ? (
            <StatChip label="มีปัญหา" value={stats.error.toLocaleString()} variant="danger" />
          ) : null}
        </div>
      )}

      {stats && stats.total > 0 && (
        <Card className="shadow-none">
          <CardContent className="space-y-2 px-3 py-2.5">
            <div className="flex items-baseline justify-between text-xs">
              <span className="font-medium text-foreground">ความพร้อมในการจับคู่</span>
              <span className="tabular-nums text-muted-foreground">
                {stats.embedded.toLocaleString()} / {stats.total.toLocaleString()} ({pct}%)
              </span>
            </div>
            <div className="h-1.5 w-full overflow-hidden rounded-full bg-muted">
              <div
                className="h-full rounded-full bg-success transition-all"
                style={{ width: `${pct}%` }}
              />
            </div>
          </CardContent>
        </Card>
      )}

      {/* Toolbar */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-1 rounded-md border border-border bg-card p-0.5">
          {tabs.map(({ key, label, count }) => {
            const active = params.filter === key
            return (
              <button
                key={key}
                type="button"
                onClick={() => handleFilterChange(key)}
                className={cn(
                  'inline-flex items-center gap-1.5 rounded px-2.5 py-1 text-xs font-medium transition-colors',
                  active
                    ? 'bg-accent text-accent-foreground'
                    : 'text-muted-foreground hover:text-foreground',
                )}
              >
                {label}
                {count != null && count > 0 && (
                  <Badge
                    variant="secondary"
                    className={cn(
                      'h-4 px-1 text-[10px] tabular-nums',
                      key === 'pending' && 'bg-warning/15 text-warning',
                      key === 'error' && 'bg-destructive/15 text-destructive',
                      key === 'missing' && 'bg-destructive/15 text-destructive',
                    )}
                  >
                    {count > 9999 ? '9999+' : count}
                  </Badge>
                )}
              </button>
            )
          })}
        </div>

        <div className="relative w-full max-w-sm">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="ค้นหา… (Enter เพื่อค้นหา)"
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={handleSearchKeyDown}
            className="h-9 pl-8 pr-16"
          />
          {draft && (
            <button
              type="button"
              className="absolute right-12 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
              onClick={() => {
                setDraft('')
                commitSearch('')
              }}
              aria-label="ล้างการค้นหา"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          )}
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="absolute right-1 top-1/2 h-7 -translate-y-1/2 px-2 text-xs"
            onClick={() => commitSearch(draft)}
          >
            ค้นหา
          </Button>
        </div>
      </div>

      <div className="overflow-hidden rounded-lg border border-border bg-card">
        <Table>
          <TableHeader>
            <TableRow className="bg-muted/40">
              <TableHead className="w-[140px]">รหัสสินค้า</TableHead>
              <TableHead>ชื่อสินค้า</TableHead>
              <TableHead className="w-[80px]">หน่วย</TableHead>
              <TableHead className="w-[100px] text-right">ราคา</TableHead>
              <TableHead className="w-[120px]">สถานะ</TableHead>
              <TableHead className="w-[140px]">ซิงก์ล่าสุด</TableHead>
              <TableHead className="w-[200px] text-right">จัดการ</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              <TableRow>
                <TableCell colSpan={7} className="py-12 text-center text-sm text-muted-foreground">
                  <Loader2 className="mx-auto mb-2 h-5 w-5 animate-spin" />
                  กำลังโหลด…
                </TableCell>
              </TableRow>
            ) : items.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="py-12 text-center text-sm">
                  <Database className="mx-auto mb-2 h-8 w-8 text-muted-foreground/50" />
                  <p className="text-muted-foreground">
                    {params.query
                      ? `ไม่พบสินค้าที่ตรงกับ "${params.query}"`
                      : 'ไม่มีข้อมูล'}
                  </p>
                </TableCell>
              </TableRow>
            ) : (
              items.map((item) => {
                const active = item.is_active !== false
                const price = item.price ?? item.sale_price
                return (
                <TableRow key={item.item_code} className={cn('h-12', !active && 'bg-destructive/[0.03]')}>
                  <TableCell className="py-2 font-mono text-xs font-medium">
                    {item.item_code}
                  </TableCell>
                  <TableCell className="py-2">
                    <div className="line-clamp-2 text-sm leading-5">{item.item_name}</div>
                    {item.item_name2 && (
                      <div className="mt-0.5 line-clamp-1 text-xs text-muted-foreground">
                        {item.item_name2}
                      </div>
                    )}
                  </TableCell>
                  <TableCell className="py-2 text-xs text-muted-foreground">
                    {item.unit_code || '—'}
                  </TableCell>
                  <TableCell className="py-2 text-right tabular-nums">
                    {price != null
                      ? `฿${price.toLocaleString()}`
                      : '—'}
                  </TableCell>
                  <TableCell className="py-2">
                    <Badge
                      variant="secondary"
                      className={cn(
                        !active &&
                          'bg-destructive/15 text-destructive hover:bg-destructive/20',
                        active &&
                        item.embedding_status === 'done' &&
                          'bg-success/15 text-success hover:bg-success/20',
                        active &&
                        item.embedding_status === 'pending' &&
                          'bg-warning/15 text-warning hover:bg-warning/20',
                        active &&
                        item.embedding_status === 'error' &&
                          'bg-destructive/15 text-destructive hover:bg-destructive/20',
                      )}
                    >
                      {!active
                        ? 'ไม่พบใน SML'
                        : item.embedding_status === 'done'
                        ? 'พร้อมจับคู่'
                        : item.embedding_status === 'pending'
                          ? 'รอเตรียมข้อมูล'
                          : 'มีปัญหา'}
                    </Badge>
                  </TableCell>
                  <TableCell className="py-2 text-xs tabular-nums text-muted-foreground">
                    {active ? fmtDateTime(item.synced_at) : fmtDateTime(item.missing_at)}
                  </TableCell>
                  <TableCell className="py-2 text-right">
                    <div className="flex items-center justify-end gap-1">
                      {active && item.embedding_status !== 'done' && (
                        <Button
                          size="sm"
                          variant="outline"
                          className="h-7 px-2 text-xs"
                          disabled={busyRow?.code === item.item_code}
                          onClick={() => handleEmbedOne(item.item_code)}
                        >
                          {busyRow?.code === item.item_code && busyRow.action === 'embed' ? 'กำลังทำ…' : 'เตรียมข้อมูล'}
                        </Button>
                      )}
                      <Button
                        size="sm"
                        variant="ghost"
                        className="h-7 px-2"
                        title="รีเฟรชจาก SML — ดึงชื่อ/หน่วย/balance จาก SML 248 ใหม่"
                        disabled={busyRow?.code === item.item_code}
                        onClick={() => handleRefreshOne(item.item_code)}
                      >
                        {busyRow?.code === item.item_code && busyRow.action === 'refresh' ? (
                          <Loader2 className="h-3.5 w-3.5 animate-spin" />
                        ) : (
                          <RefreshCw className="h-3.5 w-3.5" />
                        )}
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        className="h-7 px-2 text-destructive hover:text-destructive"
                        title="ลบจาก BillFlow (SML ไม่ถูกแตะ)"
                        disabled={busyRow?.code === item.item_code}
                        onClick={() => setPendingDelete(item.item_code)}
                      >
                        {busyRow?.code === item.item_code && busyRow.action === 'delete' ? (
                          <Loader2 className="h-3.5 w-3.5 animate-spin" />
                        ) : (
                          <Trash2 className="h-3.5 w-3.5" />
                        )}
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              )})
            )}
          </TableBody>
        </Table>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-muted-foreground">
        <span>
          {loading
            ? 'กำลังโหลด…'
            : `${total.toLocaleString()} รายการ · หน้า ${params.page} / ${Math.max(1, Math.ceil(total / PER_PAGE))}`}
        </span>
        <Pagination
          page={params.page}
          total={total}
          perPage={PER_PAGE}
          onChange={(p) => setParams({ page: p })}
        />
      </div>

      <ConfirmDialog
        open={!!pendingDelete}
        onOpenChange={(v) => !v && setPendingDelete(null)}
        title="ลบสินค้าออกจาก Catalog"
        description={
          pendingDelete
            ? `ลบ ${pendingDelete} ออกจาก BillFlow catalog? — SML 248 จะไม่ถูกแตะ ทำงานเฉพาะ BillFlow ฝั่งเดียว`
            : ''
        }
        confirmLabel="ลบ"
        variant="destructive"
        onConfirm={() => {
          if (pendingDelete) handleDeleteOne(pendingDelete)
        }}
      />
    </div>
  )
}
