import { useEffect, useMemo, useRef, useState } from 'react'
import { Check, ChevronsUpDown, RefreshCw, Search } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
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
  const [total, setTotal] = useState(0)

  const endpoint = billType === 'purchase' ? '/api/sml/suppliers' : '/api/sml/customers'
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const fetchResults = useMemo(
    () =>
      (q: string) => {
        setLoading(true)
        client
          .get<{ data: Party[]; total: number; last_sync: string }>(
            `${endpoint}?search=${encodeURIComponent(q)}&limit=20`,
          )
          .then((r) => {
            setResults(r.data.data ?? [])
            setTotal(r.data.total ?? 0)
            setLastSync(r.data.last_sync)
          })
          .catch(() => setResults([]))
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
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => fetchResults(query), 250)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, open, fetchResults])

  const handleRefresh = async () => {
    setRefreshing(true)
    try {
      const r = await client.post<{
        customers: number
        suppliers: number
        last_sync: string
      }>('/api/sml/refresh-parties')
      setLastSync(r.data.last_sync)
      toast.success(`ซิงก์เสร็จ — ${r.data.customers} ลูกค้า / ${r.data.suppliers} ผู้ขาย`)
      fetchResults(query)
    } catch (e: any) {
      toast.error('รีเฟรชไม่สำเร็จ: ' + (e?.response?.data?.error ?? e?.message ?? 'unknown'))
    } finally {
      setRefreshing(false)
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
              เลือก{billType === 'purchase' ? 'ผู้ขาย' : 'ลูกค้า'}…
            </span>
          )}
          <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[480px] p-0" align="start">
        <div className="relative border-b border-border">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="ค้นหาด้วยรหัส / ชื่อ / เลขผู้เสียภาษี…"
            className="h-10 w-full bg-transparent px-9 text-sm placeholder:text-muted-foreground focus:outline-none"
          />
          {loading && (
            <div className="absolute right-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-muted-foreground" />
          )}
        </div>

        <div className="max-h-[320px] overflow-y-auto py-1">
          {results.length === 0 && !loading && (
            <div className="px-3 py-6 text-center text-sm text-muted-foreground">
              {query
                ? 'ไม่พบข้อมูล — ลองคำค้นอื่นหรือกดรีเฟรช'
                : billType === 'purchase'
                  ? 'ยังไม่มีผู้ขายในแคช — กดรีเฟรช'
                  : 'ยังไม่มีลูกค้าในแคช — กดรีเฟรช'}
            </div>
          )}
          {results.map((p) => {
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

        <div className="flex items-center justify-between gap-2 border-t border-border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
          <span>
            {total.toLocaleString()} รายการ
            {lastSync && (
              <> · ซิงก์ล่าสุด {dayjs(lastSync).format('HH:mm')}</>
            )}
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
      </PopoverContent>
    </Popover>
  )
}
