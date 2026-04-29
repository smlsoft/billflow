import { useEffect, useRef, useState } from 'react'
import dayjs from 'dayjs'
import relativeTime from 'dayjs/plugin/relativeTime'
import 'dayjs/locale/th'

import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import client from '@/api/client'
import { cn } from '@/lib/utils'
import type { ChatConversation } from './types'

dayjs.extend(relativeTime)
dayjs.locale('th')

interface Props {
  selectedID: string
  onSelect: (lineUserID: string) => void
  onSelectedConvChange?: (conv: ChatConversation | null) => void
}

const POLL_MS = 30_000

// ConversationList polls /api/admin/conversations every 30s and renders the
// inbox. Click a row to select it (URL ?u= updates via parent).
export function ConversationList({ selectedID, onSelect, onSelectedConvChange }: Props) {
  const [rows, setRows] = useState<ChatConversation[]>([])
  const [unreadOnly, setUnreadOnly] = useState(false)
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    let alive = true
    const fetchOnce = async (force = false) => {
      // Skip polling when the tab is hidden — admin not looking. force=true on
      // mount + visibilitychange so we always fetch fresh on regain focus.
      if (
        !force &&
        typeof document !== 'undefined' &&
        document.visibilityState === 'hidden'
      ) {
        return
      }
      try {
        const res = await client.get<{ data: ChatConversation[] | null }>(
          '/api/admin/conversations',
          { params: { unread: unreadOnly || undefined, limit: 100 } },
        )
        if (alive) {
          setRows(res.data.data ?? [])
        }
      } catch {
        /* silent — keep last good list */
      } finally {
        if (alive) setLoading(false)
      }
    }
    fetchOnce(true)
    intervalRef.current = setInterval(() => fetchOnce(false), POLL_MS)

    const onVisibility = () => {
      if (document.visibilityState === 'visible') {
        fetchOnce(true)
      }
    }
    document.addEventListener('visibilitychange', onVisibility)

    return () => {
      alive = false
      if (intervalRef.current) clearInterval(intervalRef.current)
      document.removeEventListener('visibilitychange', onVisibility)
    }
  }, [unreadOnly])

  // Keep parent informed of which conversation is currently selected, so it
  // can pre-fill the customer name in CreateBillPanel.
  useEffect(() => {
    if (!onSelectedConvChange) return
    const found = rows.find((r) => r.line_user_id === selectedID) ?? null
    onSelectedConvChange(found)
  }, [rows, selectedID, onSelectedConvChange])

  const filtered = search
    ? rows.filter((r) =>
        (r.display_name || r.line_user_id).toLowerCase().includes(search.toLowerCase()),
      )
    : rows

  return (
    <div className="flex min-h-0 flex-col rounded-lg border border-border bg-card">
      <div className="flex flex-col gap-2 border-b border-border p-3">
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="ค้นหาชื่อลูกค้า…"
          className="h-8 text-sm"
        />
        <div className="flex items-center justify-between text-xs">
          <Button
            variant={unreadOnly ? 'default' : 'outline'}
            size="sm"
            className="h-7 px-2 text-xs"
            onClick={() => setUnreadOnly((v) => !v)}
          >
            {unreadOnly ? 'แสดงทั้งหมด' : 'เฉพาะที่ยังไม่อ่าน'}
          </Button>
          <span className="text-muted-foreground">{filtered.length} รายการ</span>
        </div>
      </div>

      <ScrollArea className="flex-1">
        {loading && rows.length === 0 ? (
          <div className="p-4 text-center text-xs text-muted-foreground">กำลังโหลด…</div>
        ) : filtered.length === 0 ? (
          <div className="p-4 text-center text-xs text-muted-foreground">
            {unreadOnly ? 'ไม่มีข้อความที่ยังไม่ได้อ่าน' : 'ยังไม่มีบทสนทนา'}
          </div>
        ) : (
          <ul className="divide-y divide-border">
            {filtered.map((r) => {
              const active = r.line_user_id === selectedID
              const initials = (r.display_name || '?').slice(0, 2).toUpperCase()
              return (
                <li key={r.line_user_id}>
                  <button
                    type="button"
                    onClick={() => onSelect(r.line_user_id)}
                    className={cn(
                      'flex w-full items-start gap-2.5 px-3 py-2.5 text-left transition-colors',
                      active
                        ? 'bg-accent text-accent-foreground'
                        : 'hover:bg-accent/40',
                    )}
                  >
                    <Avatar className="h-9 w-9 shrink-0">
                      {r.picture_url && <AvatarImage src={r.picture_url} alt={r.display_name} />}
                      <AvatarFallback className="text-xs">{initials}</AvatarFallback>
                    </Avatar>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-baseline justify-between gap-2">
                        <span className="truncate text-sm font-medium">
                          {r.display_name || r.line_user_id.slice(0, 10)}
                        </span>
                        <span className="shrink-0 text-[10px] tabular-nums text-muted-foreground">
                          {dayjs(r.last_message_at).fromNow(true)}
                        </span>
                      </div>
                      <div className="flex items-center justify-between gap-2">
                        <span className="flex min-w-0 items-center gap-1.5">
                          {r.line_oa_name && (
                            <Badge
                              variant="outline"
                              className="h-4 shrink-0 px-1 text-[9px] font-normal"
                              title={`LINE OA: ${r.line_oa_name}`}
                            >
                              {r.line_oa_name}
                            </Badge>
                          )}
                          <span className="truncate font-mono text-[10px] text-muted-foreground">
                            {r.line_user_id}
                          </span>
                        </span>
                        {r.unread_admin_count > 0 && (
                          <Badge className="h-5 min-w-[20px] shrink-0 justify-center px-1.5 text-[10px]">
                            {r.unread_admin_count > 99 ? '99+' : r.unread_admin_count}
                          </Badge>
                        )}
                      </div>
                    </div>
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </ScrollArea>
    </div>
  )
}
