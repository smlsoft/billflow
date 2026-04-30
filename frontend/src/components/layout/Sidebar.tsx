import { useCallback, useEffect, useRef, useState } from 'react'
import { NavLink, useNavigate } from 'react-router-dom'
import {
  Building2,
  ChevronsLeft,
  ChevronsRight,
  Database,
  FileText,
  LayoutDashboard,
  LogOut,
  Mail,
  MessageSquare,
  MessageSquareQuote,
  ScrollText,
  Settings,
  ShoppingBag,
  Tag,
  Upload,
  Workflow,
  type LucideIcon,
} from 'lucide-react'

import { useChatEvents } from '@/hooks/useChatEvents'
import { useEventsStore, type EventsConnectionState } from '@/lib/events-store'
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
  // hasBadge identifies which counter feeds the badge:
  //   "bills"    → pending bill count (existing)
  //   "messages" → unread chat conversation count (Phase 3)
  // Boolean true is treated as "bills" for backward compat with existing code.
  hasBadge?: boolean | 'bills' | 'messages'
  // Optional English/short hint shown beneath the label in the collapsed-mode
  // tooltip — helps when admins ask dev "เปิด Quick Replies ที่ไหน" since the
  // visible label is now Thai-first.
  hint?: string
}

interface NavGroup {
  label: string
  items: NavItem[]
}

// NAV_GROUPS — ordered by daily-frequency. Top groups (Overview / Bills /
// Chat) are what staff touch every day; bottom groups (Master Data / System
// Settings) are setup-once. Within each group, the most-used items lead.
//
// Labels lean Thai-first; the `hint` field provides the English/setup name
// in tooltips so a dev or new admin can connect Thai labels back to the
// underlying feature.
const NAV_GROUPS: NavGroup[] = [
  {
    label: 'ภาพรวม',
    items: [
      { to: '/dashboard', label: 'Dashboard', icon: LayoutDashboard, hint: 'หน้าแรก' },
      { to: '/logs', label: 'ประวัติการทำงาน', icon: ScrollText, hint: 'Activity Log' },
    ],
  },
  {
    label: 'บิลขาย / ซื้อ',
    items: [
      { to: '/bills', label: 'บิลทั้งหมด', icon: FileText, hasBadge: 'bills', hint: 'รวมทุก channel' },
      { to: '/import', label: 'นำเข้า Lazada', icon: Upload, end: true, hint: 'Excel จาก Lazada' },
      { to: '/import/shopee', label: 'นำเข้า Shopee', icon: ShoppingBag, hint: 'Excel จาก Shopee' },
    ],
  },
  {
    label: 'แชทลูกค้า',
    items: [
      { to: '/messages', label: 'ข้อความลูกค้า', icon: MessageSquare, hasBadge: 'messages', hint: 'Inbox รวมทุก OA' },
      { to: '/settings/line-oa', label: 'บัญชี LINE OA', icon: MessageSquare, end: true, hint: 'LINE OA Accounts' },
      { to: '/settings/quick-replies', label: 'ข้อความสำเร็จรูป', icon: MessageSquareQuote, end: true, hint: 'Quick Replies' },
      { to: '/settings/chat-tags', label: 'ป้ายลูกค้า', icon: Tag, end: true, hint: 'Chat Tags' },
    ],
  },
  {
    label: 'ข้อมูลตั้งต้น',
    items: [
      { to: '/mappings', label: 'ตารางจับคู่สินค้า', icon: Workflow, hint: 'Item Mapping (raw_name → SML code)' },
      { to: '/settings/catalog', label: 'สินค้าใน SML', icon: Database, hint: 'SML Catalog' },
      { to: '/settings/channels', label: 'ลูกค้า / ผู้ขาย default', icon: Building2, hint: 'Channel Defaults (per-channel party_code)' },
    ],
  },
  {
    label: 'ตั้งค่าระบบ',
    items: [
      { to: '/settings/email', label: 'อีเมลรับบิล', icon: Mail, hint: 'IMAP Inboxes' },
      { to: '/settings', label: 'ตั้งค่าทั่วไป', icon: Settings, end: true, hint: 'General Settings' },
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
  const [unreadMessages, setUnreadMessages] = useState(0)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // Bills pending count + unread messages. SSE pushes unread changes
  // (UnreadChanged event) so the badge updates instantly when admin opens
  // a thread or a customer messages in. The 60s poll exists as a safety
  // net to refresh pending count (which has no SSE source) and to recover
  // if the SSE stream silently drops.
  const fetchStats = useCallback(async () => {
    if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
      return
    }
    try {
      const res = await client.get<{ pending: number; unread_messages?: number }>(
        '/api/dashboard/stats',
      )
      setPendingCount(res.data.pending ?? 0)
      setUnreadMessages(res.data.unread_messages ?? 0)
    } catch {
      /* silent */
    }
  }, [])

  useEffect(() => {
    fetchStats()
    intervalRef.current = setInterval(fetchStats, 60_000)

    const onVisibility = () => {
      if (document.visibilityState === 'visible') {
        fetchStats()
      }
    }
    document.addEventListener('visibilitychange', onVisibility)

    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current)
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [fetchStats])

  // SSE — instant unread badge updates. Server publishes UnreadChanged on
  // mark-read + on every inbound webhook.
  useChatEvents({
    onUnreadChanged: useCallback((p: { total: number }) => {
      setUnreadMessages(p.total ?? 0)
    }, []),
  })

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
                const badgeKind =
                  item.hasBadge === true ? 'bills' : item.hasBadge || null
                const badgeCount =
                  badgeKind === 'messages'
                    ? unreadMessages
                    : badgeKind === 'bills'
                      ? pendingCount
                      : 0
                const showBadge = !!badgeKind && badgeCount > 0

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
                            {badgeCount > 99 ? '99+' : badgeCount}
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
                      <div className="font-medium">{item.label}</div>
                      {item.hint && (
                        <div className="mt-0.5 text-[10px] text-muted-foreground">
                          {item.hint}
                        </div>
                      )}
                    </TooltipContent>
                  </Tooltip>
                )
              })}
            </div>
          ))}
        </nav>

        <Separator />

        {/* Real-time connection state indicator. Reads from the shared
            events-store; tooltip explains what each state means. Hidden
            when sidebar collapsed — the dot still shows so admins notice
            'reconnecting' / 'offline'. */}
        <div className={cn('px-2 py-1.5', collapsed ? 'flex justify-center' : '')}>
          <ConnectionDot collapsed={collapsed} />
        </div>

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

// ConnectionDot renders the live SSE connection status as a small colored
// dot ± label. Reading from the events-store keeps state in one place;
// every page that uses Layout (i.e. all authenticated routes) sees the
// same indicator.
const STATE_META: Record<EventsConnectionState, { label: string; cls: string; tip: string }> = {
  connecting: {
    label: 'กำลังเชื่อมต่อ…',
    cls: 'bg-muted-foreground/40',
    tip: 'กำลังเปิดการเชื่อมต่อ real-time',
  },
  live: {
    label: 'Live',
    cls: 'bg-success',
    tip: 'รับข้อความ real-time แล้ว — ไม่ต้องรีเฟรช',
  },
  reconnecting: {
    label: 'กำลังเชื่อมต่อใหม่',
    cls: 'bg-warning',
    tip: 'การเชื่อมต่อหลุด — ระบบกำลังลองใหม่ (ระหว่างนี้จะใช้ polling สำรอง)',
  },
  offline: {
    label: 'Offline',
    cls: 'bg-destructive',
    tip: 'ขาดการเชื่อมต่อ real-time — ใช้ polling สำรอง (อัปเดตทุก 60 วินาที)',
  },
}

function ConnectionDot({ collapsed }: { collapsed: boolean }) {
  const status = useEventsStore((s) => s.status)
  const meta = STATE_META[status]
  const dot = (
    <span
      className={cn(
        'inline-block h-2 w-2 shrink-0 rounded-full',
        meta.cls,
        status === 'connecting' || status === 'reconnecting' ? 'animate-pulse' : '',
      )}
    />
  )
  if (collapsed) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="cursor-help">{dot}</span>
        </TooltipTrigger>
        <TooltipContent side="right" className="text-xs">
          {meta.tip}
        </TooltipContent>
      </Tooltip>
    )
  }
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="flex w-full cursor-help items-center gap-1.5 px-2 text-[10px] uppercase tracking-wider text-muted-foreground">
          {dot}
          <span>{meta.label}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent side="right" className="text-xs">
        {meta.tip}
      </TooltipContent>
    </Tooltip>
  )
}
