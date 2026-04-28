import { useEffect, useRef, useState } from 'react'
import { NavLink, useNavigate } from 'react-router-dom'
import {
  ChevronsLeft,
  ChevronsRight,
  Database,
  FileText,
  LayoutDashboard,
  LogOut,
  Mail,
  ScrollText,
  Settings,
  ShoppingBag,
  Upload,
  Workflow,
  type LucideIcon,
} from 'lucide-react'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Separator } from '@/components/ui/separator'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ThemeToggle } from '@/components/common/ThemeToggle'
import { useAuth } from '@/hooks/useAuth'
import { useUIStore } from '@/lib/ui-store'
import { cn } from '@/lib/utils'
import client from '@/api/client'

interface NavItem {
  to: string
  label: string
  icon: LucideIcon
  end?: boolean
  hasBadge?: boolean
}

interface NavGroup {
  label: string
  items: NavItem[]
}

const NAV_GROUPS: NavGroup[] = [
  {
    label: 'ภาพรวม',
    items: [
      { to: '/dashboard', label: 'Dashboard', icon: LayoutDashboard },
      { to: '/bills', label: 'บิลทั้งหมด', icon: FileText, hasBadge: true },
    ],
  },
  {
    label: 'นำเข้าข้อมูล',
    items: [
      { to: '/import', label: 'นำเข้า Lazada', icon: Upload, end: true },
      { to: '/import/shopee', label: 'นำเข้า Shopee', icon: ShoppingBag },
    ],
  },
  {
    label: 'จัดการระบบ',
    items: [
      { to: '/mappings', label: 'Mapping สินค้า', icon: Workflow },
      { to: '/settings/email', label: 'Email Inboxes', icon: Mail },
      { to: '/settings/catalog', label: 'Catalog SML', icon: Database },
      { to: '/logs', label: 'Activity Log', icon: ScrollText },
      { to: '/settings', label: 'ตั้งค่า', icon: Settings, end: true },
    ],
  },
]

const ROLE_LABEL: Record<string, string> = {
  admin: 'ผู้ดูแลระบบ',
  staff: 'พนักงาน',
  viewer: 'ผู้ดูข้อมูล',
}

export default function Sidebar() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const collapsed = useUIStore((s) => s.sidebarCollapsed)
  const toggle = useUIStore((s) => s.toggleSidebar)
  const [pendingCount, setPendingCount] = useState(0)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    const fetchPending = async () => {
      try {
        const res = await client.get<{ pending: number }>('/api/dashboard/stats')
        setPendingCount(res.data.pending ?? 0)
      } catch {
        /* silent */
      }
    }
    fetchPending()
    intervalRef.current = setInterval(fetchPending, 30_000)
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current)
    }
  }, [])

  // Hotkey [ to toggle sidebar
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLElement) {
        const tag = e.target.tagName
        if (tag === 'INPUT' || tag === 'TEXTAREA' || e.target.isContentEditable)
          return
      }
      if (e.key === '[' && !e.metaKey && !e.ctrlKey && !e.altKey) {
        e.preventDefault()
        toggle()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [toggle])

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  const initials =
    user?.name
      ? user.name
          .split(' ')
          .map((w) => w[0])
          .join('')
          .slice(0, 2)
          .toUpperCase()
      : '?'

  const sidebarWidth = collapsed ? 'w-14' : 'w-60'

  return (
    <TooltipProvider delayDuration={0}>
      <aside
        className={cn(
          'flex shrink-0 flex-col border-r border-border bg-card transition-[width] duration-150',
          sidebarWidth,
        )}
      >
        {/* Logo */}
        <div className={cn('flex h-14 items-center gap-2 px-3', collapsed && 'justify-center px-0')}>
          <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary text-primary-foreground">
            <FileText className="h-4 w-4" strokeWidth={2.25} />
          </div>
          {!collapsed && (
            <div className="min-w-0">
              <div className="truncate text-sm font-semibold leading-tight">BillFlow</div>
              <div className="truncate text-[10px] uppercase tracking-wider text-muted-foreground">
                AI Bill Processing
              </div>
            </div>
          )}
        </div>

        <Separator />

        {/* Nav */}
        <nav className="flex-1 overflow-y-auto p-2">
          {NAV_GROUPS.map((group, gi) => (
            <div key={group.label} className={cn('flex flex-col gap-0.5', gi > 0 && 'mt-4')}>
              {!collapsed && (
                <div className="px-2 pb-1 pt-2 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                  {group.label}
                </div>
              )}
              {collapsed && gi > 0 && <Separator className="my-2" />}

              {group.items.map((item) => {
                const Icon = item.icon
                const showBadge = item.hasBadge && pendingCount > 0

                const linkInner = (active: boolean) => (
                  <span
                    className={cn(
                      'group relative flex h-8 items-center gap-2.5 rounded-md px-2 text-sm transition-colors',
                      active
                        ? 'bg-accent text-accent-foreground font-medium'
                        : 'text-muted-foreground hover:bg-accent/60 hover:text-foreground',
                      collapsed && 'justify-center px-0',
                    )}
                  >
                    {active && !collapsed && (
                      <span className="absolute inset-y-1 left-0 w-0.5 rounded-r-full bg-primary" />
                    )}
                    <span className="relative">
                      <Icon className="h-[15px] w-[15px] shrink-0" strokeWidth={2} />
                      {showBadge && collapsed && (
                        <span className="absolute -right-0.5 -top-0.5 h-1.5 w-1.5 rounded-full bg-warning" />
                      )}
                    </span>
                    {!collapsed && (
                      <>
                        <span className="flex-1 truncate">{item.label}</span>
                        {showBadge && (
                          <Badge variant="secondary" className="h-5 min-w-[20px] justify-center px-1.5 text-[10px]">
                            {pendingCount > 99 ? '99+' : pendingCount}
                          </Badge>
                        )}
                      </>
                    )}
                  </span>
                )

                const link = (
                  <NavLink key={item.to} to={item.to} end={item.end}>
                    {({ isActive }) => linkInner(isActive)}
                  </NavLink>
                )

                if (!collapsed) return link
                return (
                  <Tooltip key={item.to}>
                    <TooltipTrigger asChild>{link}</TooltipTrigger>
                    <TooltipContent side="right" className="text-xs">
                      {item.label}
                    </TooltipContent>
                  </Tooltip>
                )
              })}
            </div>
          ))}
        </nav>

        <Separator />

        {/* Collapse toggle */}
        <div className="px-2 py-2">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={toggle}
            className={cn('h-8 w-full justify-start gap-2 px-2 text-xs text-muted-foreground', collapsed && 'justify-center px-0')}
            aria-label={collapsed ? 'ขยาย sidebar' : 'ยุบ sidebar'}
          >
            {collapsed ? <ChevronsRight className="h-4 w-4" /> : <ChevronsLeft className="h-4 w-4" />}
            {!collapsed && <span>ยุบเมนู</span>}
          </Button>
        </div>

        {/* User block */}
        <div className="border-t border-border p-2">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                className={cn(
                  'flex w-full items-center gap-2 rounded-md p-1.5 text-left transition-colors hover:bg-accent',
                  collapsed && 'justify-center',
                )}
                aria-label="เมนูผู้ใช้"
              >
                <Avatar className="h-7 w-7">
                  <AvatarFallback className="bg-primary text-primary-foreground text-[11px]">
                    {initials}
                  </AvatarFallback>
                </Avatar>
                {!collapsed && (
                  <div className="min-w-0 flex-1 leading-tight">
                    <div className="truncate text-xs font-medium">
                      {user?.name || user?.email}
                    </div>
                    <div className="truncate text-[10px] text-muted-foreground">
                      {ROLE_LABEL[user?.role ?? ''] ?? user?.role}
                    </div>
                  </div>
                )}
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" side="right" className="min-w-[200px]">
              <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">
                {user?.email}
              </DropdownMenuLabel>
              <DropdownMenuSeparator />
              <ThemeToggle variant="menu-item" />
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={handleLogout} className="gap-2 text-destructive focus:text-destructive">
                <LogOut className="h-3.5 w-3.5" />
                ออกจากระบบ
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </aside>
    </TooltipProvider>
  )
}
