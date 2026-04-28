import { useEffect, useState } from 'react'
import {
  AlertCircle,
  ChevronDown,
  FileText,
  Info,
  Mail,
  Pencil,
  PlayCircle,
  Plus,
  ShoppingBag,
  Trash2,
} from 'lucide-react'
import dayjs from 'dayjs'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { DataTable } from '@/components/common/DataTable'
import { EmptyState } from '@/components/common/EmptyState'
import { PageHeader } from '@/components/common/PageHeader'
import { StatusDot } from '@/components/common/StatusDot'
import client from '@/api/client'
import { cn } from '@/lib/utils'
import type { IMAPAccount } from '@/pages/EmailAccounts/AccountDialog'
import { AccountDialog } from '@/pages/EmailAccounts/AccountDialog'

interface IMAPAccountFull extends IMAPAccount {
  last_polled_at?: string | null
  last_poll_status?: string | null
  last_poll_error?: string | null
  last_poll_messages?: number | null
  consecutive_failures?: number
}

const CHANNEL_META: Record<string, { label: string; cls: string }> = {
  general: { label: 'General', cls: 'bg-secondary text-secondary-foreground' },
  shopee:  { label: 'Shopee',  cls: 'bg-warning/15 text-warning hover:bg-warning/20' },
  lazada:  { label: 'Lazada',  cls: 'bg-info/15 text-info hover:bg-info/20' },
}

// Help banner shown above the table — collapsible so it doesn't clutter the
// view once admins are familiar. Default open on first visit (state lives in
// component memory; resets per page reload).
function HelpBanner() {
  const [open, setOpen] = useState(true)
  return (
    <Card className="border-info/30 bg-info/5">
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger className="flex w-full items-center gap-2 px-4 py-3 text-left text-sm font-medium text-foreground hover:bg-info/10">
          <Info className="h-4 w-4 text-info" />
          <span>BillFlow ดึงอีเมลแบบไหน + เลือก channel ยังไง</span>
          <ChevronDown
            className={cn(
              'ml-auto h-4 w-4 text-muted-foreground transition-transform',
              open && 'rotate-180',
            )}
          />
        </CollapsibleTrigger>
        <CollapsibleContent>
          <CardContent className="space-y-3 border-t border-info/20 px-4 pt-3 text-sm">
            <p className="text-muted-foreground">
              ระบบดึงอีเมลจาก inbox ที่คุณเพิ่มไว้ มาสร้างบิลอัตโนมัติ —
              ทุก inbox มี <b>channel</b> กำหนดว่าจะ route อีเมลไปทาง flow ไหน:
            </p>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="rounded-md border border-border bg-card p-3">
                <div className="mb-1 flex items-center gap-2 text-sm font-semibold">
                  <ShoppingBag className="h-4 w-4 text-warning" />
                  Channel: <span className="text-warning">Shopee</span>
                </div>
                <p className="text-xs text-muted-foreground">
                  สำหรับ Gmail/Outlook ที่มีอีเมลจาก Shopee — ระบบจะแยก:
                </p>
                <ul className="mt-1.5 space-y-0.5 pl-4 text-xs">
                  <li className="list-disc">
                    Subject "<b>คำสั่งซื้อ #...</b>" → บิลขาย (saleinvoice)
                  </li>
                  <li className="list-disc">
                    Subject "<b>ถูกจัดส่งแล้ว</b>" หรือ "<b>ยืนยันการชำระเงิน</b>" →
                    ใบสั่งซื้อ/สั่งจอง (PO)
                  </li>
                </ul>
              </div>
              <div className="rounded-md border border-border bg-card p-3">
                <div className="mb-1 flex items-center gap-2 text-sm font-semibold">
                  <FileText className="h-4 w-4 text-info" />
                  Channel: <span className="text-info">General</span>
                </div>
                <p className="text-xs text-muted-foreground">
                  สำหรับ inbox ที่มี PDF / Excel แนบ (เช่น email vendor)
                </p>
                <ul className="mt-1.5 space-y-0.5 pl-4 text-xs">
                  <li className="list-disc">
                    AI extract ข้อมูลจาก PDF → สร้างบิล
                  </li>
                  <li className="list-disc">
                    รองรับ .pdf, .jpg, .png, .xls, .xlsx
                  </li>
                </ul>
              </div>
            </div>
            <div className="rounded-md bg-warning/10 px-3 py-2 text-xs text-warning">
              ⚠️ Gmail ต้องใช้ <b>App Password</b> (16 หลัก) ไม่ใช่ password จริง —
              ในฟอร์มเพิ่ม inbox มีปุ่ม "วิธีรับ App Password" ข้างช่อง Password
            </div>
          </CardContent>
        </CollapsibleContent>
      </Collapsible>
    </Card>
  )
}

function statusVariant(s?: string | null): 'success' | 'warning' | 'danger' | 'muted' {
  if (!s) return 'muted'
  if (s === 'ok') return 'success'
  return 'danger'
}

function statusLabel(s?: string | null): string {
  if (!s) return 'ยังไม่ poll'
  switch (s) {
    case 'ok': return 'สำเร็จ'
    case 'connect_failed': return 'เชื่อมต่อไม่ได้'
    case 'auth_failed': return 'รหัสผ่านผิด'
    case 'select_failed': return 'folder ไม่มี'
    case 'search_failed': return 'search ผิดพลาด'
    case 'fetch_failed': return 'fetch ผิดพลาด'
    default: return s
  }
}

export default function EmailAccounts() {
  const [accounts, setAccounts] = useState<IMAPAccountFull[]>([])
  const [loading, setLoading] = useState(true)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<IMAPAccount | null>(null)
  const [deleteId, setDeleteId] = useState<string | null>(null)

  const fetchAll = async () => {
    try {
      const res = await client.get<{ data: IMAPAccountFull[] }>('/api/settings/imap-accounts')
      setAccounts(res.data.data ?? [])
    } catch {
      toast.error('โหลดรายการ inbox ไม่สำเร็จ')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    fetchAll()
    // Auto-refresh status every 30s — same cadence as the sidebar pending count.
    const t = setInterval(fetchAll, 30_000)
    return () => clearInterval(t)
  }, [])

  const handleAdd = () => {
    setEditing(null)
    setDialogOpen(true)
  }

  const handleEdit = (a: IMAPAccountFull) => {
    setEditing(a)
    setDialogOpen(true)
  }

  const handlePollNow = async (a: IMAPAccountFull) => {
    const id = toast.loading(`Poll ${a.name}…`)
    try {
      const res = await client.post<{
        status: string
        messages_found: number
        processed: number
        duration_ms: number
        error?: string
      }>(`/api/settings/imap-accounts/${a.id}/poll`)
      const r = res.data
      if (r.status === 'ok') {
        toast.success(
          `Poll เสร็จ ${r.processed}/${r.messages_found} ราย (${r.duration_ms} ms)`,
          { id },
        )
      } else {
        toast.error(`Poll fail: ${r.error || r.status}`, { id })
      }
      fetchAll()
    } catch {
      toast.error('Poll ไม่สำเร็จ', { id })
    }
  }

  const handleDelete = async () => {
    if (!deleteId) return
    try {
      await client.delete(`/api/settings/imap-accounts/${deleteId}`)
      toast.success('ลบสำเร็จ')
      fetchAll()
    } catch {
      toast.error('ลบไม่สำเร็จ')
    }
  }

  const headerActions = (
    <Button size="sm" onClick={handleAdd}>
      <Plus className="h-4 w-4" />
      เพิ่ม Inbox
    </Button>
  )

  return (
    <div className="space-y-5">
      <PageHeader
        title="Email Inboxes"
        description="ดึงอีเมลจากหลาย mailbox มาสร้างบิลอัตโนมัติ — แก้ config ได้โดยไม่ต้อง deploy"
        actions={headerActions}
      />

      <HelpBanner />

      {!loading && accounts.length === 0 ? (
        <EmptyState
          icon={Mail}
          title="ยังไม่มี inbox"
          description="เพิ่ม inbox แรกเพื่อเริ่มดึงบิลจากอีเมล (ถ้าเคยตั้ง .env IMAP_* ไว้ — copy ค่ามาใส่ที่นี่ครั้งเดียว แล้ว .env เหล่านั้นจะไม่ถูกใช้อีก)"
          action={
            <Button onClick={handleAdd}>
              <Plus className="h-4 w-4" />
              เพิ่ม Inbox แรก
            </Button>
          }
        />
      ) : (
        <TooltipProvider delayDuration={0}>
          <DataTable<IMAPAccountFull>
            data={accounts}
            loading={loading}
            empty="ยังไม่มี inbox"
            columns={[
              {
                key: 'name',
                header: 'ชื่อ',
                cell: (a) => (
                  <div className="flex flex-col">
                    <span className="font-medium">{a.name}</span>
                    <span className="text-xs text-muted-foreground">{a.username}</span>
                  </div>
                ),
              },
              {
                key: 'channel',
                header: 'Channel',
                cell: (a) => {
                  const meta = CHANNEL_META[a.channel] ?? CHANNEL_META.general
                  return (
                    <Badge variant="secondary" className={meta.cls}>
                      {meta.label}
                    </Badge>
                  )
                },
              },
              {
                key: 'status',
                header: 'สถานะ',
                cell: (a) => {
                  const dot = (
                    <StatusDot
                      variant={a.enabled ? statusVariant(a.last_poll_status) : 'muted'}
                      label={a.enabled ? statusLabel(a.last_poll_status) : 'ปิดใช้'}
                    />
                  )
                  if (!a.last_poll_error) return dot
                  return (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span className="cursor-help">{dot}</span>
                      </TooltipTrigger>
                      <TooltipContent className="max-w-md">
                        <p className="font-mono text-xs">{a.last_poll_error}</p>
                        {a.consecutive_failures != null && a.consecutive_failures > 0 && (
                          <p className="mt-1 text-xs text-muted-foreground">
                            fail {a.consecutive_failures} ครั้งติด
                          </p>
                        )}
                      </TooltipContent>
                    </Tooltip>
                  )
                },
              },
              {
                key: 'last_poll',
                header: 'Last poll',
                cell: (a) =>
                  a.last_polled_at ? (
                    <span className="text-xs tabular-nums text-muted-foreground">
                      {dayjs(a.last_polled_at).format('DD/MM/YY HH:mm:ss')}
                    </span>
                  ) : (
                    <span className="text-xs italic text-muted-foreground">—</span>
                  ),
              },
              {
                key: 'msgs',
                header: 'Msgs',
                headerClassName: 'text-right',
                className: 'text-right',
                cell: (a) =>
                  a.last_poll_messages != null ? (
                    <span className="font-mono text-xs">{a.last_poll_messages}</span>
                  ) : (
                    <span className="text-xs text-muted-foreground">—</span>
                  ),
              },
              {
                key: 'interval',
                header: 'Interval',
                cell: (a) => (
                  <span className="text-xs tabular-nums text-muted-foreground">
                    {Math.round(a.poll_interval_seconds / 60)} นาที
                  </span>
                ),
              },
              {
                key: 'actions',
                header: '',
                headerClassName: 'text-right',
                className: 'text-right',
                cell: (a) => (
                  <div className="flex justify-end gap-1">
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          size="icon"
                          variant="ghost"
                          className="h-7 w-7"
                          onClick={() => handlePollNow(a)}
                          disabled={!a.enabled}
                        >
                          <PlayCircle className="h-3.5 w-3.5" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>Poll ทันที</TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          size="icon"
                          variant="ghost"
                          className="h-7 w-7"
                          onClick={() => handleEdit(a)}
                        >
                          <Pencil className="h-3.5 w-3.5" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>แก้ไข</TooltipContent>
                    </Tooltip>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          size="icon"
                          variant="ghost"
                          className="h-7 w-7 text-muted-foreground hover:text-destructive"
                          onClick={() => setDeleteId(a.id)}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>ลบ</TooltipContent>
                    </Tooltip>
                  </div>
                ),
              },
            ]}
          />
        </TooltipProvider>
      )}

      {accounts.some((a) => a.consecutive_failures != null && a.consecutive_failures >= 3) && (
        <div className="flex items-start gap-2 rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
          <AlertCircle className="h-4 w-4 shrink-0" />
          <span>
            มี inbox ที่ poll fail ≥ 3 ครั้งติด — admin ได้รับ LINE notify แล้ว
            กรุณาแก้ password หรือ host ให้ถูกต้อง
          </span>
        </div>
      )}

      <AccountDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        account={editing}
        onSaved={fetchAll}
      />

      <ConfirmDialog
        open={deleteId !== null}
        onOpenChange={(o) => !o && setDeleteId(null)}
        title="ลบ inbox นี้?"
        description="หลังลบ inbox จะไม่ถูก poll อีก แต่บิลที่สร้างไว้แล้วยังอยู่"
        variant="destructive"
        confirmLabel="ลบ"
        onConfirm={handleDelete}
      />
    </div>
  )
}
