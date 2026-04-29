import { useEffect, useState, useRef, useCallback, useMemo, useReducer } from 'react'
import {
  AlertCircle,
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
  Upload,
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
import type { CatalogItem } from '@/types'

interface CatalogStats {
  total: number
  embedded: number
  pending: number
  error: number
  index_size: number
  embed_running: boolean
}

interface ListResponse {
  data: CatalogItem[]
  total: number
  page: number
  per_page: number
}

type StatusFilter = '' | 'pending' | 'done' | 'error'
interface FetchParams { page: number; filter: StatusFilter; query: string }

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
    success: 'border-success/30 bg-success/5 text-success',
    warning: 'border-warning/30 bg-warning/5 text-warning',
    danger: 'border-destructive/30 bg-destructive/5 text-destructive',
    primary: 'border-primary/30 bg-primary/5 text-primary',
    muted: 'border-border bg-card text-foreground',
  }
  return (
    <Card className={cn('flex-1', styles[variant])}>
      <CardContent className="px-4 py-3">
        <p className="text-xl font-semibold tabular-nums">{value}</p>
        <p className="mt-0.5 text-[11px] font-medium uppercase tracking-wider text-muted-foreground">
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
  const fileRef = useRef<HTMLInputElement>(null)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const PER_PAGE = 50

  const fetchStats = useCallback(async () => {
    const res = await api.get<CatalogStats>('/api/catalog/stats')
    setStats(res.data)
    return res.data
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
  }, [fetchStats])

  useEffect(() => {
    if (stats?.embed_running) {
      pollRef.current = setInterval(async () => {
        const s = await fetchStats()
        if (!s.embed_running) {
          if (pollRef.current) clearInterval(pollRef.current)
          fetchItems(params)
        }
      }, 3000)
    } else {
      if (pollRef.current) clearInterval(pollRef.current)
    }
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [stats?.embed_running])

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
    setSyncing(true)
    try {
      const res = await api.post<{ synced: number }>('/api/catalog/sync')
      notify(`Sync สำเร็จ ${res.data.synced} รายการ`)
      fetchStats()
      fetchItems(params)
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      notify(msg ?? 'Sync ล้มเหลว', false)
    } finally {
      setSyncing(false)
    }
  }

  async function handleCSVUpload(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    const form = new FormData()
    form.append('file', file)
    try {
      const res = await api.post<{ synced: number }>('/api/catalog/import-csv', form, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
      notify(`Import CSV สำเร็จ ${res.data.synced} รายการ`)
      fetchStats()
      fetchItems(params)
    } catch {
      notify('Import CSV ล้มเหลว', false)
    } finally {
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  async function handleEmbedAll() {
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
    try {
      await api.post(`/api/catalog/${code}/embed`)
      notify(`Embed ${code} สำเร็จ`)
      fetchStats()
      fetchItems(params)
    } catch {
      notify(`Embed ${code} ล้มเหลว`, false)
    }
  }

  // Tracks which row is currently running an action so we can disable
  // its buttons and show a spinner without blocking the rest of the table.
  const [busyRow, setBusyRow] = useState<{ code: string; action: 'refresh' | 'delete' } | null>(null)
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
        notify(`ไม่พบ ${code} ใน SML — ลบจาก BillFlow ได้`, false)
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

  const tabs: Array<{ key: StatusFilter; label: string; count?: number }> = [
    { key: '', label: 'ทั้งหมด', count: stats?.total },
    { key: 'done', label: 'Embedded', count: stats?.embedded },
    { key: 'pending', label: 'Pending', count: stats?.pending },
    { key: 'error', label: 'Error', count: stats?.error },
  ]

  return (
    <div className="space-y-5">
      <PageHeader
        title="Catalog สินค้า SML"
        description="สินค้าจาก SML ใช้สำหรับ Smart Matching อีเมล Shopee + การจับคู่ในบิล"
        actions={
          <>
            <Button variant="outline" size="sm" onClick={handleSync} disabled={syncing}>
              <RotateCw className={cn('h-3.5 w-3.5', syncing && 'animate-spin')} />
              {syncing ? 'กำลัง Sync…' : 'Sync จาก SML'}
            </Button>
            <Button asChild variant="outline" size="sm">
              <label className="cursor-pointer">
                <Upload className="h-3.5 w-3.5" />
                Import CSV
                <input
                  ref={fileRef}
                  type="file"
                  accept=".csv"
                  onChange={handleCSVUpload}
                  className="sr-only"
                />
              </label>
            </Button>
            <Button size="sm" onClick={handleEmbedAll} disabled={isEmbedBusy}>
              {isEmbedBusy ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Sparkles className="h-3.5 w-3.5" />
              )}
              {isEmbedBusy ? 'กำลัง Embed…' : 'Embed All Pending'}
            </Button>
            <Button variant="outline" size="sm" onClick={handleReload}>
              <RefreshCcw className="h-3.5 w-3.5" />
              Reload Index
            </Button>
          </>
        }
      />

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
        <div className="flex flex-wrap gap-3">
          <StatChip label="สินค้าทั้งหมด" value={stats.total.toLocaleString()} variant="primary" />
          <StatChip label="Embedded" value={stats.embedded.toLocaleString()} variant="success" />
          <StatChip label="Pending" value={stats.pending.toLocaleString()} variant="warning" />
          <StatChip label="Index Size" value={stats.index_size.toLocaleString()} variant="primary" />
          {stats.embed_running ? (
            <Card className="flex-1 border-primary/30 bg-primary/5">
              <CardContent className="flex items-center gap-3 px-4 py-3">
                <Loader2 className="h-4 w-4 animate-spin text-primary" />
                <div>
                  <p className="text-sm font-medium text-primary">กำลัง Embed…</p>
                  <p className="text-xs text-muted-foreground">auto-refresh ทุก 3 วินาที</p>
                </div>
              </CardContent>
            </Card>
          ) : stats.error > 0 ? (
            <StatChip label="Error" value={stats.error.toLocaleString()} variant="danger" />
          ) : null}
        </div>
      )}

      {stats && stats.total > 0 && (
        <Card>
          <CardContent className="space-y-2 p-4">
            <div className="flex items-baseline justify-between text-sm">
              <span className="font-medium text-foreground">Embedding Progress</span>
              <span className="tabular-nums text-muted-foreground">
                {stats.embedded.toLocaleString()} / {stats.total.toLocaleString()} ({pct}%)
              </span>
            </div>
            <div className="h-2 w-full overflow-hidden rounded-full bg-muted">
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
              <TableHead className="w-[140px]">Item Code</TableHead>
              <TableHead>ชื่อสินค้า</TableHead>
              <TableHead className="w-[80px]">หน่วย</TableHead>
              <TableHead className="w-[100px] text-right">ราคา</TableHead>
              <TableHead className="w-[120px]">สถานะ</TableHead>
              <TableHead className="w-[120px]">Embedded At</TableHead>
              <TableHead className="w-[200px] text-right">Action</TableHead>
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
              items.map((item) => (
                <TableRow key={item.item_code}>
                  <TableCell className="font-mono text-xs font-medium">
                    {item.item_code}
                  </TableCell>
                  <TableCell>
                    <div className="text-sm">{item.item_name}</div>
                    {item.item_name2 && (
                      <div className="mt-0.5 text-xs text-muted-foreground">
                        {item.item_name2}
                      </div>
                    )}
                  </TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {item.unit_code || '—'}
                  </TableCell>
                  <TableCell className="text-right tabular-nums">
                    {item.sale_price != null
                      ? `฿${item.sale_price.toLocaleString()}`
                      : '—'}
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant="secondary"
                      className={cn(
                        item.embedding_status === 'done' &&
                          'bg-success/15 text-success hover:bg-success/20',
                        item.embedding_status === 'pending' &&
                          'bg-warning/15 text-warning hover:bg-warning/20',
                        item.embedding_status === 'error' &&
                          'bg-destructive/15 text-destructive hover:bg-destructive/20',
                      )}
                    >
                      {item.embedding_status === 'done' ? '✓ done' : item.embedding_status}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-xs tabular-nums text-muted-foreground">
                    {item.embedded_at
                      ? new Date(item.embedded_at).toLocaleDateString('th-TH')
                      : '—'}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex items-center justify-end gap-1">
                      {item.embedding_status !== 'done' && (
                        <Button
                          size="sm"
                          variant="outline"
                          className="h-7 px-2 text-xs"
                          onClick={() => handleEmbedOne(item.item_code)}
                        >
                          Embed
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
              ))
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
