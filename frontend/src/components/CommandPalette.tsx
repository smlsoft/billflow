import { useEffect, useState, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Database,
  Bot,
  ClipboardCheck,
  FileText,
  LayoutDashboard,
  LogOut,
  Mail,
  Moon,
  PanelLeftClose,
  ScrollText,
  Search,
  Settings,
  ShoppingBag,
  Sparkles,
  Sun,
  Upload,
  Workflow,
} from 'lucide-react'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
  CommandShortcut,
} from '@/components/ui/command'
import client from '@/api/client'
import { useAuth } from '@/hooks/useAuth'
import { useTheme } from '@/lib/theme'
import { useUIStore } from '@/lib/ui-store'
import { billSourceLabel, billStatusLabel } from '@/lib/labels'
import { ENABLE_LAZADA_EXCEL, ENABLE_SALES_ORDERS, ENABLE_SHOPEE_EXCEL, ENABLE_TIKTOK_EXCEL } from '@/lib/featureFlags'
import type { Bill } from '@/types'

const PHASE = Number(import.meta.env.VITE_PHASE ?? 99)

interface RecentBill {
  id: string
  bill_type: string
  document_route?: string
  sml_doc_no?: string | null
  source: string
  status: string
  created_at: string
}

const NAV_ITEMS: Array<{
  to: string
  label: string
  icon: React.ComponentType<{ className?: string }>
  minPhase?: number
  enabled?: boolean
}> = [
  { to: '/setup', label: 'เริ่มต้นใช้งาน', icon: ClipboardCheck },
  { to: '/dashboard', label: 'ภาพรวม', icon: LayoutDashboard },
  { to: '/bills', label: 'ใบสั่งซื้อ', icon: FileText },
  { to: '/sales-orders', label: 'ใบสั่งขาย', icon: ShoppingBag, enabled: ENABLE_SALES_ORDERS },
  { to: '/sale-invoices', label: 'ขายสินค้าและบริการ', icon: ShoppingBag, enabled: ENABLE_SALES_ORDERS },
  { to: '/settings/email', label: 'กล่องอีเมลรับบิล', icon: Mail },
  { to: '/import/shopee', label: 'Shopee Excel', icon: Upload, enabled: ENABLE_SHOPEE_EXCEL },
  { to: '/import/lazada', label: 'Lazada Excel', icon: Upload, enabled: ENABLE_LAZADA_EXCEL && ENABLE_SALES_ORDERS },
  { to: '/import/tiktok', label: 'TikTok Excel', icon: Upload, enabled: ENABLE_TIKTOK_EXCEL && ENABLE_SALES_ORDERS },
  { to: '/mappings', label: 'ตารางจับคู่สินค้า', icon: Workflow },
  { to: '/settings/catalog', label: 'สินค้าใน SML', icon: Database },
  { to: '/settings/ai-usage', label: 'การใช้งาน AI', icon: Bot },
  { to: '/logs', label: 'ประวัติการทำงาน', icon: ScrollText },
  { to: '/settings', label: 'ตั้งค่า', icon: Settings },
]

export function CommandPalette({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (o: boolean) => void
}) {
  const navigate = useNavigate()
  const { user, logout } = useAuth()
  const { setTheme } = useTheme()
  const toggleSidebar = useUIStore((s) => s.toggleSidebar)
  const [query, setQuery] = useState('')
  const [recent, setRecent] = useState<RecentBill[]>([])
  const [searched, setSearched] = useState<RecentBill[]>([])
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const isAdmin = user?.role === 'admin'

  useEffect(() => {
    if (!open) return
    let alive = true
    client
      .get<{ data: Bill[] }>('/api/bills', { params: { per_page: 5 } })
      .then((r) => {
        if (!alive) return
        setRecent(
          (r.data.data ?? []).map((b) => ({
            id: b.id,
            bill_type: b.bill_type,
            sml_doc_no: b.sml_doc_no,
            source: b.source,
            status: b.status,
            created_at: b.created_at,
          })),
        )
      })
      .catch(() => null)
    return () => {
      alive = false
    }
  }, [open])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    if (query.trim().length < 2) {
      setSearched([])
      return
    }
    debounceRef.current = setTimeout(() => {
      client
        .get<{ data: Bill[] }>('/api/bills', { params: { search: query.trim(), per_page: 8 } })
        .then((r) =>
          setSearched(
            (r.data.data ?? []).map((b) => ({
              id: b.id,
              bill_type: b.bill_type,
              sml_doc_no: b.sml_doc_no,
              source: b.source,
              status: b.status,
              created_at: b.created_at,
            })),
          ),
        )
        .catch(() => setSearched([]))
    }, 250)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query])

  const close = () => {
    onOpenChange(false)
    setQuery('')
  }

  const go = (to: string) => {
    navigate(to)
    close()
  }

  const generateInsight = async () => {
    close()
    const { toast } = await import('sonner')
    const id = toast.loading('กำลังสร้างสรุปรายวัน…')
    try {
      await client.post('/api/dashboard/insights/generate')
      toast.success('สร้างสรุปรายวันสำเร็จ', { id })
    } catch {
      toast.error('ไม่สามารถสร้างได้', { id })
    }
  }

  const handleLogout = () => {
    close()
    logout()
    navigate('/login')
  }

  const billsToShow = query.trim().length >= 2 ? searched : recent

  return (
    <CommandDialog open={open} onOpenChange={onOpenChange}>
      <CommandInput
        placeholder="พิมพ์เพื่อค้นหาคำสั่ง บิล หรือหน้า…"
        value={query}
        onValueChange={setQuery}
      />
      <CommandList>
        <CommandEmpty>ไม่พบรายการ</CommandEmpty>

        <CommandGroup heading="ไปหน้า">
          {NAV_ITEMS.filter((item) => item.enabled !== false && (!item.minPhase || PHASE >= item.minPhase)).map((item) => {
            const Icon = item.icon
            return (
              <CommandItem
                key={item.to}
                value={`nav ${item.label} ${item.to}`}
                onSelect={() => go(item.to)}
              >
                <Icon className="h-4 w-4" />
                <span>{item.label}</span>
              </CommandItem>
            )
          })}
        </CommandGroup>

        <CommandSeparator />

        <CommandGroup heading="คำสั่ง">
          {PHASE >= 2 && isAdmin && (
            <CommandItem value="cmd insight" onSelect={generateInsight}>
              <Sparkles className="h-4 w-4" />
              <span>สร้างสรุปรายวัน</span>
            </CommandItem>
          )}
          <CommandItem
            value="cmd theme light"
            onSelect={() => {
              setTheme('light')
              close()
            }}
          >
            <Sun className="h-4 w-4" />
            <span>เปลี่ยนเป็นธีมสว่าง</span>
          </CommandItem>
          <CommandItem
            value="cmd theme dark"
            onSelect={() => {
              setTheme('dark')
              close()
            }}
          >
            <Moon className="h-4 w-4" />
            <span>เปลี่ยนเป็นธีมมืด</span>
          </CommandItem>
          <CommandItem
            value="cmd toggle sidebar"
            onSelect={() => {
              toggleSidebar()
              close()
            }}
          >
            <PanelLeftClose className="h-4 w-4" />
            <span>ยุบ / ขยายเมนู</span>
            <CommandShortcut>[</CommandShortcut>
          </CommandItem>
          <CommandItem value="cmd logout" onSelect={handleLogout}>
            <LogOut className="h-4 w-4" />
            <span>ออกจากระบบ</span>
          </CommandItem>
        </CommandGroup>

        {billsToShow.length > 0 && (
          <>
            <CommandSeparator />
            <CommandGroup
              heading={query.trim().length >= 2 ? 'ผลค้นหาบิล' : 'บิลล่าสุด'}
            >
              {billsToShow.map((b) => {
                const detailPath =
                  b.bill_type !== 'sale'
                    ? `/bills/${b.id}`
                    : b.document_route === 'saleinvoice'
                      ? `/sale-invoices/${b.id}`
                      : `/sales-orders/${b.id}`
                return (
                  <CommandItem
                    key={b.id}
                    value={`bill ${b.sml_doc_no ?? ''} ${b.id} ${b.source}`}
                    onSelect={() => go(detailPath)}
                  >
                    <Search className="h-4 w-4" />
                    <span className="font-mono text-xs">
                      {b.sml_doc_no ?? b.id.slice(0, 8) + '…'}
                    </span>
                    <span className="ml-2 text-xs text-muted-foreground">
                      {billSourceLabel(b.source)} · {billStatusLabel(b.status, true)}
                    </span>
                  </CommandItem>
                )
              })}
            </CommandGroup>
          </>
        )}
      </CommandList>
    </CommandDialog>
  )
}
