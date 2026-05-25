import { useEffect, useMemo, useRef, useState } from 'react'
import { ArrowLeft, Check, ChevronsUpDown, Plus, RefreshCw, Search } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import client from '@/api/client'
import { cn } from '@/lib/utils'
import dayjs from 'dayjs'

export interface Party {
  code: string
  name: string
  name_1?: string
  tax_id?: string
  telephone?: string
  address?: string
}

interface PartyPickerProps {
  billType: 'sale' | 'purchase'
  value: Party | null
  onChange: (p: Party) => void
  disabled?: boolean
}

// Searchable combobox over /api/sml/customers or /api/sml/suppliers.
// Backend caches both lists in memory + scores results by relevance.
export function PartyPicker({ billType, value, onChange, disabled }: PartyPickerProps) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<Party[]>([])
  const [loading, setLoading] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [lastSync, setLastSync] = useState<string | null>(null)
  const [syncStatus, setSyncStatus] = useState<string>('not_ready')
  const [syncError, setSyncError] = useState('')
  const [total, setTotal] = useState(0)
  const [createMode, setCreateMode] = useState(false)
  const [createCode, setCreateCode] = useState('')
  const [createName, setCreateName] = useState('')
  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState('')

  const endpoint = billType === 'purchase' ? '/api/sml/suppliers' : '/api/sml/customers'
  const partyLabel = billType === 'purchase' ? 'ผู้ขาย' : 'ลูกค้า'
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const fetchResults = useMemo(
    () =>
      (q: string) => {
        setLoading(true)
        client
          .get<{
            data: Party[]
            total: number
            last_sync: string | null
            status?: string
            error?: string
          }>(
            `${endpoint}?search=${encodeURIComponent(q)}&limit=20`,
          )
          .then((r) => {
            setResults(r.data.data ?? [])
            setTotal(r.data.total ?? 0)
            setLastSync(r.data.last_sync)
            setSyncStatus(r.data.status ?? (r.data.last_sync ? 'ok' : 'not_ready'))
            setSyncError(r.data.error ?? '')
          })
          .catch((e) => {
            setResults([])
            setSyncStatus('error')
            setSyncError(e?.response?.data?.error ?? e?.message ?? 'โหลดรายชื่อไม่สำเร็จ')
          })
          .finally(() => setLoading(false))
      },
    [endpoint],
  )

  // Initial fetch when popover opens
  useEffect(() => {
    if (!open) return
    fetchResults('')
  }, [open, fetchResults])

  // Debounced search on keystroke
  useEffect(() => {
    if (!open) return
    if (createMode) return
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => fetchResults(query), 250)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, open, fetchResults, createMode])

  const handleRefresh = async () => {
    setRefreshing(true)
    try {
      const r = await client.post<{
        customers: number
        suppliers: number
        last_sync: string | null
        status?: string
      }>('/api/sml/refresh-parties')
      setLastSync(r.data.last_sync)
      setSyncStatus(r.data.status ?? 'ok')
      setSyncError('')
      toast.success(`ซิงก์เสร็จ — ${r.data.customers} ลูกค้า / ${r.data.suppliers} ผู้ขาย`)
      fetchResults(query)
    } catch (e: any) {
      const msg = e?.response?.data?.error ?? e?.message ?? 'unknown'
      setSyncStatus('error')
      setSyncError(msg)
      toast.error('รีเฟรชไม่สำเร็จ: ' + msg)
    } finally {
      setRefreshing(false)
    }
  }

  const startCreate = () => {
    setCreateMode(true)
    setCreateError('')
    setCreateName(query.trim())
  }

  const handleCreate = async () => {
    const code = createCode.trim()
    const name = createName.trim()
    if (!code || !name) return
    setCreating(true)
    setCreateError('')
    try {
      const r = await client.post<{ party: Party }>(endpoint, { code, name_1: name })
      const created = r.data.party
      const party = {
        ...created,
        code: created.code || code,
        name: created.name || created.name_1 || name,
      }
      onChange(party)
      setResults((prev) => [party, ...prev.filter((p) => p.code !== party.code)])
      setCreateMode(false)
      setOpen(false)
      toast.success(`สร้าง${partyLabel}แล้ว`, { description: `${party.code} · ${party.name}` })
    } catch (e: any) {
      const msg = e?.response?.data?.error ?? e?.message ?? 'create failed'
      setCreateError(msg)
      toast.error(`สร้าง${partyLabel}ไม่สำเร็จ`, { description: msg })
    } finally {
      setCreating(false)
    }
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className="w-full justify-between font-normal"
          disabled={disabled}
        >
          {value ? (
            <span className="flex items-center gap-2 truncate text-left">
              <span className="font-mono text-xs text-muted-foreground">{value.code}</span>
              <span className="truncate">{value.name}</span>
            </span>
          ) : (
            <span className="text-muted-foreground">
              เลือก{partyLabel}…
            </span>
          )}
          <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[480px] p-0" align="start">
        {!createMode && (
          <div className="flex items-center gap-2 border-b border-border p-2">
            <div className="relative min-w-0 flex-1">
              <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
              <input
                autoFocus
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="ค้นหาด้วยรหัส / ชื่อ / เลขผู้เสียภาษี…"
                className="h-9 w-full rounded-md border border-input bg-background px-9 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
              />
              {loading && (
                <div className="absolute right-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-muted-foreground" />
              )}
            </div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-9 shrink-0 gap-1.5 px-2.5 text-xs"
              onClick={startCreate}
            >
              <Plus className="h-3.5 w-3.5" />
              สร้าง{partyLabel}ใหม่
            </Button>
          </div>
        )}

        <div className="max-h-[320px] overflow-y-auto py-1">
          {createMode && (
            <div className="space-y-3 px-3 py-3">
              <div className="space-y-1.5">
                <label className="text-xs text-muted-foreground">
                  รหัส{partyLabel} <span className="text-destructive">*</span>
                </label>
                <Input
                  autoFocus
                  value={createCode}
                  onChange={(e) => setCreateCode(e.target.value.trim().toUpperCase())}
                  placeholder={billType === 'purchase' ? 'เช่น VNEW01' : 'เช่น ARNEW01'}
                  className="font-mono"
                />
              </div>
              <div className="space-y-1.5">
                <label className="text-xs text-muted-foreground">
                  ชื่อ{partyLabel} <span className="text-destructive">*</span>
                </label>
                <Input
                  value={createName}
                  onChange={(e) => setCreateName(e.target.value)}
                  placeholder={`ชื่อ${partyLabel}ใน SML`}
                />
              </div>
              {createError && (
                <p className="text-xs text-destructive">{createError}</p>
              )}
              <div className="flex items-center justify-between gap-2 pt-1">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-8 gap-1.5 px-2 text-xs"
                  disabled={creating}
                  onClick={() => setCreateMode(false)}
                >
                  <ArrowLeft className="h-3.5 w-3.5" />
                  กลับไปค้นหา
                </Button>
                <Button
                  type="button"
                  size="sm"
                  className="h-8 gap-1.5 px-2 text-xs"
                  disabled={creating || !createCode.trim() || !createName.trim()}
                  onClick={handleCreate}
                >
                  <Plus className="h-3.5 w-3.5" />
                  {creating ? 'กำลังสร้าง...' : `สร้างและเลือก${partyLabel}`}
                </Button>
              </div>
            </div>
          )}
          {!createMode && results.length === 0 && !loading && (
            <div className="px-3 py-6 text-center text-sm text-muted-foreground">
              {query
                ? `ไม่พบข้อมูล — กดสร้าง${partyLabel}ใหม่หรือรีเฟรช`
                : billType === 'purchase'
                  ? 'ยังไม่มีผู้ขายในแคช — กดรีเฟรช'
                  : 'ยังไม่มีลูกค้าในแคช — กดรีเฟรช'}
            </div>
          )}
          {!createMode && results.map((p) => {
            const isSelected = value?.code === p.code
            return (
              <button
                key={p.code}
                type="button"
                onClick={() => {
                  onChange(p)
                  setOpen(false)
                }}
                className={cn(
                  'flex w-full items-start gap-3 px-3 py-2 text-left text-sm hover:bg-accent',
                  isSelected && 'bg-accent',
                )}
              >
                <Check
                  className={cn(
                    'mt-1 h-4 w-4 shrink-0',
                    isSelected ? 'opacity-100' : 'opacity-0',
                  )}
                />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-xs text-muted-foreground">
                      {p.code}
                    </span>
                    <span className="truncate font-medium">{p.name}</span>
                  </div>
                  {(p.tax_id || p.telephone || p.address) && (
                    <div className="mt-0.5 truncate text-xs text-muted-foreground">
                      {p.tax_id && <span>tax: {p.tax_id} · </span>}
                      {p.telephone && <span>โทร {p.telephone} · </span>}
                      {p.address && <span className="truncate">{p.address}</span>}
                    </div>
                  )}
                </div>
              </button>
            )
          })}
        </div>

        {!createMode && (
          <div className="flex items-center justify-between gap-2 border-t border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
            <span className={cn((syncStatus === 'error' || syncStatus === 'not_ready') && 'text-warning')}>
              {total.toLocaleString()} รายการ
              {lastSync ? (
                <> · ซิงก์ล่าสุด {dayjs(lastSync).format('HH:mm')}</>
              ) : (
                <> · ยังไม่เคยซิงก์สำเร็จ</>
              )}
              {syncStatus === 'error' && syncError ? <> · {syncError}</> : null}
            </span>
            <Button
              variant="ghost"
              size="sm"
              className="h-7 gap-1.5 px-2 text-xs"
              onClick={handleRefresh}
              disabled={refreshing}
            >
              <RefreshCw className={cn('h-3 w-3', refreshing && 'animate-spin')} />
              รีเฟรช
            </Button>
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}
