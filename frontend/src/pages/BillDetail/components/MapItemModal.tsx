import { useState, useEffect } from 'react'
import { Search, Plus, ArrowLeft, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
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
import { scoreBorderClass } from '../utils/formatters'

interface Props {
  open: boolean
  rawName: string
  currentCode: string
  currentUnit: string
  currentPrice: number
  onPick: (code: string, unitCode: string) => void
  onClose: () => void
}

export function MapItemModal({
  open,
  rawName,
  currentCode,
  currentUnit,
  currentPrice,
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
      <DialogContent className="max-w-xl max-h-[90vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Search className="h-4 w-4 text-muted-foreground" />
            เลือกสินค้าจาก SML Catalog
          </DialogTitle>
        </DialogHeader>

        {/* Raw name context */}
        <div className="rounded-md bg-muted/40 px-3 py-2 text-sm">
          <div className="text-xs text-muted-foreground mb-1">ชื่อสินค้า (raw):</div>
          <div className="font-medium break-words">{rawName}</div>
          {currentCode && (
            <div className="mt-1 text-xs text-muted-foreground">
              ปัจจุบัน:{' '}
              <code className="text-foreground font-mono">{currentCode}</code>
              {' '}({currentUnit || '—'})
            </div>
          )}
        </div>

        <Tabs value={tab} onValueChange={(v) => setTab(v as 'search' | 'create')}>
          <TabsList className="w-full">
            <TabsTrigger value="search" className="flex-1 gap-1.5">
              <Search className="h-3.5 w-3.5" /> ค้นหา
            </TabsTrigger>
            <TabsTrigger value="create" className="flex-1 gap-1.5">
              <Plus className="h-3.5 w-3.5" /> สร้างสินค้าใหม่
            </TabsTrigger>
          </TabsList>

          {/* ── Search tab ─────────────────────────────────────────────────── */}
          <TabsContent value="search" className="space-y-3 mt-3">
            <Input
              autoFocus
              placeholder="ค้นหาด้วยชื่อสินค้า..."
              value={query}
              onChange={(e) => setQuery(e.target.value)}
            />

            {searching && (
              <p className="text-sm text-muted-foreground">กำลังค้นหา...</p>
            )}
            {searchError && (
              <p className="text-sm text-destructive">{searchError}</p>
            )}

            {!searching && results.length === 0 && query.trim().length >= 2 && (
              <div className="rounded-md bg-muted/40 py-6 text-center text-sm text-muted-foreground">
                ไม่พบสินค้าที่ตรง
              </div>
            )}

            <div className="flex flex-col gap-2">
              {results.map((r) => (
                <button
                  key={r.item_code}
                  type="button"
                  onClick={() => {
                    onPick(r.item_code, r.unit_code)
                    onClose()
                  }}
                  className={cn(
                    'w-full text-left rounded-md border-2 bg-background px-3 py-2',
                    'hover:bg-muted/50 transition-colors cursor-pointer',
                    scoreBorderClass(r.score),
                  )}
                >
                  <div className="font-semibold text-sm font-mono">{r.item_code}</div>
                  <div className="text-sm text-muted-foreground mt-0.5">{r.item_name}</div>
                  <div className="text-xs text-muted-foreground mt-1">
                    หน่วย: {r.unit_code || '—'} · score: {(r.score * 100).toFixed(0)}%
                  </div>
                </button>
              ))}
            </div>

            <div className="flex items-center justify-between border-t pt-3">
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
                สร้างสินค้าใหม่
              </Button>
            </div>
          </TabsContent>

          {/* ── Create tab ─────────────────────────────────────────────────── */}
          <TabsContent value="create" className="space-y-3 mt-3">
            <div className="space-y-3">
              <div className="space-y-1.5">
                <label className="text-sm text-muted-foreground">
                  รหัสสินค้า (Item Code) <span className="text-destructive">*</span>
                </label>
                <Input
                  autoFocus
                  value={form.code}
                  placeholder="เช่น CON-99001 หรือ INGU-VIT-30ML"
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
                {creating ? 'กำลังสร้าง...' : 'สร้างและเลือกสินค้านี้'}
              </Button>
            </div>
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  )
}
