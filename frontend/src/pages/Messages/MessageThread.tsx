import { useEffect, useRef, useState, useCallback } from 'react'
import { Bell, BellOff, Plus, RefreshCw } from 'lucide-react'
import { toast } from 'sonner'
import dayjs from 'dayjs'

import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import client from '@/api/client'
import { MessageBubble } from './MessageBubble'
import { Composer } from './Composer'
import { CustomerHistoryPanel } from './CustomerHistoryPanel'
import { useNotifications } from './useNotifications'
import type { ChatConversation, ChatMessage } from './types'

interface Props {
  lineUserID: string
  conversation: ChatConversation | null
  onOpenCreateBill: () => void
  onExtractMedia: (messageId: string, kind: string) => void
}

const ACTIVE_POLL_MS = 5_000

// MessageThread renders the right pane: header (customer info + เปิดบิลขาย),
// scrollable message list (poll 5s, delta-fetch via ?since=lastSeen), and
// composer at the bottom. Calls mark-read once per conversation switch.
export function MessageThread({
  lineUserID,
  conversation,
  onOpenCreateBill,
  onExtractMedia,
}: Props) {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [loading, setLoading] = useState(true)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const scrollRootRef = useRef<HTMLDivElement | null>(null)
  const lastSeenRef = useRef<string>('')
  const notif = useNotifications()

  // Auto-scroll to bottom when new messages arrive AND user is already near
  // the bottom (within 80px). Otherwise leave the scroll position alone so we
  // don't yank an admin who's reading older history.
  const isNearBottom = useCallback(() => {
    const root = scrollRootRef.current
    if (!root) return true
    const distanceFromBottom = root.scrollHeight - root.scrollTop - root.clientHeight
    return distanceFromBottom < 80
  }, [])

  const scrollToBottom = useCallback(() => {
    const root = scrollRootRef.current
    if (!root) return
    root.scrollTop = root.scrollHeight
  }, [])

  const fetchInitial = useCallback(async () => {
    setLoading(true)
    try {
      const res = await client.get<{ data: ChatMessage[] | null }>(
        `/api/admin/conversations/${encodeURIComponent(lineUserID)}/messages`,
        { params: { limit: 100 } },
      )
      const rows = res.data.data ?? []
      setMessages(rows)
      lastSeenRef.current = rows.length > 0 ? rows[rows.length - 1].created_at : ''
      // Defer to next paint so DOM exists before we scroll.
      setTimeout(scrollToBottom, 0)
    } catch (e: any) {
      toast.error('โหลดข้อความล้มเหลว: ' + (e?.message ?? 'unknown'))
    } finally {
      setLoading(false)
    }
  }, [lineUserID, scrollToBottom])

  const fetchDelta = useCallback(async () => {
    if (!lastSeenRef.current) return
    // Skip polling when the tab is hidden — saves a round-trip every 5s when
    // admin has BillFlow open in a background tab.
    if (typeof document !== 'undefined' && document.visibilityState === 'hidden') {
      return
    }
    try {
      const res = await client.get<{ data: ChatMessage[] | null }>(
        `/api/admin/conversations/${encodeURIComponent(lineUserID)}/messages`,
        { params: { since: lastSeenRef.current, limit: 100 } },
      )
      const newRows = res.data.data ?? []
      if (newRows.length === 0) return
      const stick = isNearBottom()
      setMessages((prev) => [...prev, ...newRows])
      lastSeenRef.current = newRows[newRows.length - 1].created_at

      // Notify on incoming messages — sound + (when tab hidden) browser
      // notification. Skip outgoing/system rows.
      const incoming = newRows.filter((m) => m.direction === 'incoming')
      if (incoming.length > 0) {
        const last = incoming[incoming.length - 1]
        const senderName = conversation?.display_name || lineUserID.slice(0, 12)
        const preview =
          last.kind === 'text'
            ? last.text_content
            : last.kind === 'image'
              ? '📷 รูปภาพ'
              : last.kind === 'file'
                ? '📄 ไฟล์'
                : last.kind === 'audio'
                  ? '🎙 voice'
                  : '(ข้อความใหม่)'
        notif.notify(senderName, preview.slice(0, 80))
      }

      if (stick) {
        setTimeout(scrollToBottom, 0)
      }
    } catch {
      /* silent */
    }
  }, [lineUserID, isNearBottom, scrollToBottom, conversation, notif])

  // Fetch + start polling whenever the active conversation changes.
  useEffect(() => {
    fetchInitial()
    // Mark-read fires once per open. Server zeroes unread_admin_count,
    // sidebar badge updates on next 30s tick.
    client
      .post(`/api/admin/conversations/${encodeURIComponent(lineUserID)}/mark-read`)
      .catch(() => {
        /* silent */
      })

    intervalRef.current = setInterval(fetchDelta, ACTIVE_POLL_MS)
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current)
    }
  }, [lineUserID, fetchInitial, fetchDelta])

  const handleSend = useCallback(
    async (text: string) => {
      // Optimistic insert so the bubble appears instantly.
      const optimistic: ChatMessage = {
        id: 'tmp-' + Date.now(),
        line_user_id: lineUserID,
        direction: 'outgoing',
        kind: 'text',
        text_content: text,
        delivery_status: 'pending',
        created_at: new Date().toISOString(),
      }
      setMessages((prev) => [...prev, optimistic])
      setTimeout(scrollToBottom, 0)
      try {
        const res = await client.post<{ message: ChatMessage; delivery: string }>(
          `/api/admin/conversations/${encodeURIComponent(lineUserID)}/messages`,
          { text },
        )
        // Replace optimistic with the persisted row.
        setMessages((prev) =>
          prev.map((m) => (m.id === optimistic.id ? res.data.message : m)),
        )
        lastSeenRef.current = res.data.message.created_at
        if (res.data.delivery === 'failed') {
          toast.error('ส่งไม่สำเร็จ — กดที่ ⚠ บนข้อความเพื่อดูรายละเอียด')
        }
      } catch (e: any) {
        setMessages((prev) =>
          prev.map((m) =>
            m.id === optimistic.id
              ? { ...m, delivery_status: 'failed', delivery_error: e?.message ?? 'unknown' }
              : m,
          ),
        )
        toast.error('ส่งไม่สำเร็จ: ' + (e?.message ?? 'unknown'))
      }
    },
    [lineUserID, scrollToBottom],
  )

  const initials = (conversation?.display_name || '?').slice(0, 2).toUpperCase()

  return (
    <div className="flex min-h-0 flex-col rounded-lg border border-border bg-card">
      {/* Header */}
      <div className="flex items-center justify-between gap-2 border-b border-border px-4 py-3">
        <div className="flex min-w-0 items-center gap-3">
          <Avatar className="h-9 w-9 shrink-0">
            {conversation?.picture_url && (
              <AvatarImage src={conversation.picture_url} alt={conversation.display_name} />
            )}
            <AvatarFallback className="text-xs">{initials}</AvatarFallback>
          </Avatar>
          <div className="min-w-0">
            <div className="truncate text-sm font-medium">
              {conversation?.display_name || lineUserID.slice(0, 12)}
            </div>
            <div className="truncate font-mono text-[10px] text-muted-foreground">
              {lineUserID}
              {conversation?.last_message_at && (
                <> · ล่าสุด {dayjs(conversation.last_message_at).format('DD/MM HH:mm')}</>
              )}
            </div>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          <Button
            variant="ghost"
            size="sm"
            className="h-8 px-2"
            onClick={notif.toggle}
            title={notif.enabled ? 'ปิดเสียง / desktop notification' : 'เปิดเสียง / desktop notification'}
          >
            {notif.enabled ? (
              <Bell className="h-3.5 w-3.5" />
            ) : (
              <BellOff className="h-3.5 w-3.5 text-muted-foreground" />
            )}
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="h-8 gap-1.5"
            onClick={fetchInitial}
            title="รีเฟรชข้อความ"
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </Button>
          <Button size="sm" className="h-8 gap-1.5" onClick={onOpenCreateBill}>
            <Plus className="h-3.5 w-3.5" />
            เปิดบิลขาย
          </Button>
        </div>
      </div>

      {/* Customer history (Phase 4.5) — past bills from this LINE user */}
      <CustomerHistoryPanel lineUserID={lineUserID} />

      {/* Scrollable message list — native scrollbar so we can keep a direct ref */}
      <div ref={scrollRootRef} className="flex-1 overflow-y-auto">
        <div className="flex flex-col gap-2 p-4">
          {loading && messages.length === 0 ? (
            <div className="text-center text-xs text-muted-foreground">กำลังโหลด…</div>
          ) : messages.length === 0 ? (
            <div className="py-8 text-center text-xs text-muted-foreground">
              ยังไม่มีข้อความในบทสนทนานี้
            </div>
          ) : (
            messages.map((m) => (
              <MessageBubble key={m.id} message={m} onExtract={onExtractMedia} />
            ))
          )}
        </div>
      </div>

      {/* Composer */}
      <Composer onSend={handleSend} />
    </div>
  )
}
