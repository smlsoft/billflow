import { useState, useEffect } from 'react'
import { ArrowLeft, CheckCircle2, Plus, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import api from '@/api/client'
import type { CatalogMatch } from '@/types'
import { scoreBorderClass, scoreStyle } from '../utils/formatters'

interface Props {
  open: boolean
  rawName: string
  currentCode: string
  currentUnit: string
  currentPrice: number
  rawNameLabel?: string
  onPick: (code: string, unitCode: string) => void
  onClose: () => void
}

function ScorePill({ score, recommended = false }: { score: number; recommended?: boolean }) {
  const pct = Math.round(score * 100)
  const s = scoreStyle(score)
  return (
    <div className="flex min-w-[92px] flex-col items-end gap-1">
      <span
        className={cn(
          'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-bold tabular-nums',
          s.bg,
          s.color,
        )}
      >
        {recommended && <CheckCircle2 className="h-3.5 w-3.5" />}
        {pct}%
      </span>
      <div className="h-1 w-20 overflow-hidden rounded-full bg-muted">
        <div
          className={cn(
            'h-full rounded-full',
            score >= 0.85 ? 'bg-success' : score >= 0.6 ? 'bg-warning' : 'bg-destructive',
          )}
          style={{ width: `${Math.max(4, Math.min(100, pct))}%` }}
        />
      </div>
    </div>
  )
}

export function MapItemModal({
  open,
  rawName,
  currentCode,
  currentUnit,
  currentPrice,
  rawNameLabel = 'ชื่อสินค้าจากต้นทาง',
  onPick,
  onClose,
}: Props) {
  const [tab, setTab] = useState<'search' | 'create'>('search')

  // ── Search state ─────────────────────────────────────────────────────────────
  const [query, setQuery] = useState(rawName.slice(0, 80))
  const [results, setResults] = useState<CatalogMatch[]>([])
  const [searching, setSearching] = useState(false)
  const [searchError, setSearchError] = useState('')

  // ── Create state ─────────────────────────────────────────────────────────────
  const [form, setForm] = useState({
    code: '',
    name: rawName.slice(0, 80),
    unit_code: currentUnit || 'ชิ้น',
    price: String(currentPrice || 0),
  })
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState('')

  // Debounced search → /api/catalog/search
  useEffect(() => {
    if (tab !== 'search') return
    const q = query.trim()
    if (q.length < 2) {
      setResults([])
      return
    }
    const handle = setTimeout(async () => {
      setSearching(true)
      setSearchError('')
      try {
        const res = await api.get<{ results: CatalogMatch[] }>(
          '/api/catalog/search',
          { params: { q, top: 10 } },
        )
        setResults(res.data.results ?? [])
      } catch (err: unknown) {
        setSearchError(err instanceof Error ? err.message : 'search failed')
      } finally {
        setSearching(false)
      }
    }, 300)
    return () => clearTimeout(handle)
  }, [query, tab])

  const handleCreate = async () => {
    setCreating(true)
    setCreateError('')
    try {
      const payload = {
        code: form.code.trim(),
        name: form.name.trim(),
        unit_code: form.unit_code.trim(),
        price: Number(form.price) || 0,
      }
      const res = await api.post<{ code: string; unit_code: string }>(
        '/api/catalog/products',
        payload,
      )
      onPick(res.data.code, res.data.unit_code)
      onClose()
    } catch (err: unknown) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      setCreateError(e?.response?.data?.error || e?.message || 'create failed')
    } finally {
      setCreating(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="grid max-h-[90vh] max-w-3xl grid-rows-[auto_auto_minmax(0,1fr)] overflow-hidden">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Search className="h-4 w-4 text-muted-foreground" />
            เลือกสินค้าจาก SML
          </DialogTitle>
        </DialogHeader>

        {/* Raw name context */}
        <div className="rounded-md border border-border bg-muted/30 px-3 py-2 text-sm">
          <div className="mb-1 text-xs font-medium text-muted-foreground">{rawNameLabel}</div>
          <div className="line-clamp-2 break-words font-medium leading-5">{rawName}</div>
          {currentCode && (
            <div className="mt-1.5 text-xs text-muted-foreground">
              เลือกไว้ตอนนี้:{' '}
              <code className="text-foreground font-mono">{currentCode}</code>
              {' '}({currentUnit || '—'})
            </div>
          )}
        </div>

        <Tabs
          value={tab}
          onValueChange={(v) => setTab(v as 'search' | 'create')}
          className="grid min-h-0 grid-rows-[auto_minmax(0,1fr)]"
        >
          <TabsList className="w-full">
            <TabsTrigger value="search" className="flex-1 gap-1.5">
              <Search className="h-3.5 w-3.5" /> ค้นหาจาก SML
            </TabsTrigger>
            <TabsTrigger value="create" className="flex-1 gap-1.5">
              <Plus className="h-3.5 w-3.5" /> เพิ่มสินค้าใหม่
            </TabsTrigger>
          </TabsList>

          {/* ── Search tab ─────────────────────────────────────────────────── */}
          <TabsContent value="search" className="mt-3 grid min-h-0 grid-rows-[auto_auto_minmax(0,1fr)_auto] gap-2.5">
            <Input
              autoFocus
              placeholder="ค้นหาด้วยชื่อหรือรหัสสินค้า"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />

            {searching && (
              <p className="text-sm text-muted-foreground">กำลังค้นหา...</p>
            )}
            {searchError && (
              <p className="text-sm text-destructive">{searchError}</p>
            )}

            {!searching && results.length > 0 && (
              <div className="rounded-md bg-muted/30 px-3 py-1.5 text-xs text-muted-foreground">
                รายการแรกคือคำแนะนำหลัก ส่วนรายการคะแนนต่ำเป็นตัวเลือกสำรอง
              </div>
            )}

            {!searching && results.length === 0 && query.trim().length >= 2 && (
              <div className="rounded-md bg-muted/40 py-6 text-center text-sm text-muted-foreground">
                ไม่พบสินค้าที่ตรง
              </div>
            )}

            <div className="min-h-0 space-y-1.5 overflow-y-auto pr-1">
              {results.map((r, index) => {
                const recommended = index === 0 && r.score >= 0.75
                const lowScore = r.score < 0.6
                return (
                <button
                  key={r.item_code}
                  type="button"
                  onClick={() => {
                    onPick(r.item_code, r.unit_code)
                    onClose()
                  }}
                  className={cn(
                    'w-full rounded-md border bg-background px-3 py-2 text-left',
                    'cursor-pointer transition-colors hover:bg-muted/40',
                    recommended && 'border-success/60 bg-success/[0.04]',
                    !recommended && lowScore && 'border-border/80',
                    !recommended && !lowScore && scoreBorderClass(r.score),
                  )}
                >
                  <div className="flex items-start justify-between gap-4">
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-mono text-sm font-semibold text-foreground">
                          {r.item_code}
                        </span>
                        {recommended && (
                          <Badge className="h-5 bg-success text-[10px] text-success-foreground">
                            แนะนำ
                          </Badge>
                        )}
                        <Badge variant="outline" className="h-5 px-1.5 text-[10px]">
                          หน่วย {r.unit_code || '—'}
                        </Badge>
                      </div>
                      <div className="mt-1 line-clamp-2 break-words text-sm leading-5 text-foreground">
                        {r.item_name}
                      </div>
                    </div>
                    <div className="flex shrink-0 flex-col items-end gap-2">
                      <ScorePill score={r.score} recommended={recommended} />
                      <span className="rounded-md bg-primary/10 px-2 py-0.5 text-[11px] font-medium text-primary">
                        เลือก
                      </span>
                    </div>
                  </div>
                </button>
                )
              })}
            </div>

            <div className="flex items-center justify-between border-t pt-2">
              <span className="text-sm text-muted-foreground">ไม่เจอที่ตรง?</span>
              <Button
                type="button"
                size="sm"
                onClick={() => {
                  setForm((f) => ({ ...f, name: query.trim() || rawName.slice(0, 80) }))
                  setTab('create')
                }}
              >
                <Plus className="h-3.5 w-3.5" />
                เพิ่มสินค้าใหม่
              </Button>
            </div>
          </TabsContent>

          {/* ── Create tab ─────────────────────────────────────────────────── */}
          <TabsContent value="create" className="mt-3 space-y-3 overflow-y-auto pr-1">
            <div className="space-y-3">
              <div className="space-y-1.5">
                <label className="text-sm text-muted-foreground">
                  รหัสสินค้า <span className="text-destructive">*</span>
                </label>
                <Input
                  autoFocus
                  value={form.code}
                  placeholder="เช่น BF-99001 หรือ INGU-VIT-30ML"
                  onChange={(e) => setForm((f) => ({ ...f, code: e.target.value }))}
                />
              </div>

              <div className="space-y-1.5">
                <label className="text-sm text-muted-foreground">
                  ชื่อสินค้า <span className="text-destructive">*</span>
                </label>
                <Input
                  value={form.name}
                  onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-1.5">
                  <label className="text-sm text-muted-foreground">
                    หน่วย <span className="text-destructive">*</span>
                  </label>
                  <Input
                    value={form.unit_code}
                    placeholder="เช่น ชิ้น, ถุง, กระป๋อง"
                    onChange={(e) => setForm((f) => ({ ...f, unit_code: e.target.value }))}
                  />
                </div>
                <div className="space-y-1.5">
                  <label className="text-sm text-muted-foreground">ราคา/หน่วย</label>
                  <Input
                    type="number"
                    step="any"
                    value={form.price}
                    onChange={(e) => setForm((f) => ({ ...f, price: e.target.value }))}
                  />
                </div>
              </div>

              {createError && (
                <p className="text-sm text-destructive">{createError}</p>
              )}
            </div>

            <div className="flex items-center justify-between pt-2">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                disabled={creating}
                onClick={() => setTab('search')}
              >
                <ArrowLeft className="h-3.5 w-3.5" />
                กลับไปค้นหา
              </Button>
              <Button
                type="button"
                disabled={
                  creating ||
                  !form.code.trim() ||
                  !form.name.trim() ||
                  !form.unit_code.trim()
                }
                onClick={handleCreate}
              >
                {creating ? 'กำลังเพิ่ม...' : 'เพิ่มและเลือกสินค้านี้'}
              </Button>
            </div>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  )
}
