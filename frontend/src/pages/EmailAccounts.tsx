import { useEffect, useState } from 'react'
import axios from 'axios'
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

const PHASE = Number(import.meta.env.VITE_PHASE ?? 99)

interface IMAPAccountFull extends IMAPAccount {
  last_polled_at?: string | null
  last_poll_status?: string | null
  last_poll_error?: string | null
  last_poll_messages?: number | null
  consecutive_failures?: number
}

const CHANNEL_META: Record<string, { label: string; cls: string }> = {
  general: { label: 'ไฟล์แนบทั่วไป', cls: 'bg-secondary text-secondary-foreground' },
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
          <span>BillFlow ดึงอีเมลแบบไหน</span>
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
              ระบบจะอ่านอีเมลจากกล่องที่เพิ่มไว้ แล้วสร้างบิลให้อัตโนมัติ.
              สำหรับ Phase 1 ให้ใช้กล่องเมลที่รับอีเมลจาก Shopee เป็นหลัก:
            </p>
            <div className={cn('grid grid-cols-1 gap-3', PHASE >= 2 && 'sm:grid-cols-2')}>
              <div className="rounded-md border border-border bg-card p-3">
                <div className="mb-1 flex items-center gap-2 text-sm font-semibold">
                  <ShoppingBag className="h-4 w-4 text-warning" />
                  กล่องเมล Shopee
                </div>
                <p className="text-xs text-muted-foreground">
                  สำหรับ Gmail/Outlook ที่มีอีเมลจาก Shopee — Phase 1 ใช้สร้างบิลซื้อจากอีเมล:
                </p>
                <ul className="mt-1.5 space-y-0.5 pl-4 text-xs">
                  <li className="list-disc">
                    Subject "<b>ถูกจัดส่งแล้ว</b>" หรือ "<b>ยืนยันการชำระเงิน</b>" → บิลซื้อ
                  </li>
                  <li className="list-disc">
                    บิลที่สร้างแล้วจะไปตรวจต่อที่หน้า <b>ใบสั่งซื้อ</b>
                  </li>
                </ul>
              </div>
              {PHASE >= 2 && (
                <div className="rounded-md border border-border bg-card p-3">
                  <div className="mb-1 flex items-center gap-2 text-sm font-semibold">
                    <FileText className="h-4 w-4 text-info" />
                    กล่องเมลไฟล์แนบทั่วไป
                  </div>
                  <p className="text-xs text-muted-foreground">
                    สำหรับ PDF / Excel แนบจากผู้ขายทั่วไป ใช้ใน Phase ถัดไป
                  </p>
                  <ul className="mt-1.5 space-y-0.5 pl-4 text-xs">
                    <li className="list-disc">
                      ระบบอ่านข้อมูลจากไฟล์แนบแล้วสร้างบิล
                    </li>
                    <li className="list-disc">
                      รองรับ .pdf, .jpg, .png, .xls, .xlsx
                    </li>
                  </ul>
                </div>
              )}
            </div>
            <div className="rounded-md bg-warning/10 px-3 py-2 text-xs text-warning">
              Gmail ต้องใช้ <b>App Password</b> (16 หลัก) ไม่ใช่ password จริง —
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
  if (s === 'warning') return 'warning'
  return 'danger'
}

function statusLabel(s?: string | null): string {
  if (!s) return 'ยังไม่ poll'
  switch (s) {
    case 'ok': return 'สำเร็จ'
    case 'warning': return 'มีคำเตือน'
    case 'connect_failed': return 'เชื่อมต่อไม่ได้'
    case 'auth_failed': return 'รหัสผ่านผิด'
    case 'select_failed': return 'folder ไม่มี'
    case 'search_failed': return 'search ผิดพลาด'
    case 'fetch_failed': return 'fetch ผิดพลาด'
    default: return s
  }
}

function friendlyPollError(error?: string | null): string {
  if (!error) return ''
  const lower = error.toLowerCase()
  if (lower.includes('ถูกข้าม') && lower.includes('ผู้ส่งที่ยอมรับ')) {
    return error
  }
  if (lower.includes('shopee_channel_non_shopee_from')) {
    return 'มีอีเมลเข้ามา แต่ผู้ส่งไม่อยู่ในรายการที่ยอมรับ ให้กดแก้ไขแล้วเพิ่มอีเมลหรือโดเมนผู้ส่ง หรือเว้นว่างถ้าต้องการรับทุกผู้ส่งที่ผ่านคำกรองหัวข้อ'
  }
  if (lower.includes('empty items')) {
    return 'มีอีเมลที่ผ่านคำกรองหัวข้อเข้ามา แต่ไม่ใช่รูปแบบบิลซื้อ Shopee ที่ระบบอ่านได้ แนะนำให้กดแก้ไขกล่องเมล แล้วเหลือคำกรองเฉพาะ "ถูกจัดส่งแล้ว" และ "ยืนยันการชำระเงินคำสั่งซื้อหมายเลข"'
  }
  if (lower.includes('empty orders')) {
    return 'มีอีเมลที่ผ่านคำกรองหัวข้อเข้ามา แต่ไม่พบเลขคำสั่งซื้อ Shopee แนะนำให้ตรวจคำกรองหัวข้อในกล่องเมลนี้'
  }
  if (lower.includes('openrouter') || lower.includes('credit') || lower.includes('quota')) {
    return 'ระบบ AI ยังประมวลผลไม่ได้ กรุณาตรวจเครดิตหรือการเชื่อมต่อ OpenRouter'
  }
  return error
}

function compactList(value?: string | null, maxItems = 2): string {
  const items = (value ?? '')
    .split(',')
    .map((x) => x.trim())
    .filter(Boolean)
  if (items.length === 0) return 'ยังไม่กำหนด'
  if (items.length <= maxItems) return items.join(', ')
  return `${items.slice(0, maxItems).join(', ')} +${items.length - maxItems}`
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
      toast.error('โหลดรายการอีเมลไม่สำเร็จ')
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
    const id = toast.loading(`กำลังดึงอีเมล ${a.name}…`)
    try {
      const res = await client.post<{
        status: string
        messages_found: number
        processed: number
        duration_ms: number
        error?: string
      }>(`/api/settings/imap-accounts/${a.id}/poll`, undefined, {
        timeout: 180000,
      })
      const r = res.data
      if (r.status === 'ok') {
        toast.success(
          `ดึงเสร็จ ${r.processed}/${r.messages_found} ราย (${r.duration_ms} ms)`,
          { id },
        )
      } else {
        toast.error(`ดึงอีเมลไม่สำเร็จ: ${r.error || r.status}`, { id })
      }
      fetchAll()
    } catch (e) {
      if (axios.isAxiosError(e) && e.code === 'ECONNABORTED') {
        toast.warning(
          'คำสั่งดึงอีเมลใช้เวลานาน ระบบอาจยังทำงานต่ออยู่ ให้รอสักครู่แล้วรีเฟรชสถานะ',
          { id },
        )
        fetchAll()
        return
      }
      const msg = axios.isAxiosError(e)
        ? e.response?.data?.error || e.message
        : ''
      toast.error(`ดึงอีเมลไม่สำเร็จ${msg ? `: ${msg}` : ''}`, { id })
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
      เพิ่มกล่องเมล
    </Button>
  )

  const warningAccounts = accounts.filter((a) => a.last_poll_status === 'warning' && a.last_poll_error)

  return (
    <div className="space-y-5">
      <PageHeader
        title="กล่องอีเมลรับบิล"
        description="กล่องเมลที่ใช้ดึงอีเมล Shopee และสร้างบิลซื้ออัตโนมัติ"
        actions={headerActions}
      />

      <HelpBanner />

      {!loading && warningAccounts.length > 0 && (
        <div className="space-y-2 rounded-md border border-warning/40 bg-warning/5 p-3 text-sm text-warning">
          <div className="flex items-start gap-2 font-medium">
            <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
            <span>มีอีเมลที่ดึงได้ แต่สร้างบิลไม่สำเร็จ</span>
          </div>
          {warningAccounts.map((a) => (
            <div key={a.id} className="ml-6 rounded border border-warning/20 bg-background/60 px-2.5 py-2">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="font-medium text-foreground">{a.name}</div>
                <Button
                  variant="outline"
                  size="sm"
                  className="h-7 px-2 text-xs"
                  onClick={() => handleEdit(a)}
                >
                  แก้คำกรอง
                </Button>
              </div>
              <p className="mt-1 line-clamp-3 break-words text-xs text-muted-foreground">
                {friendlyPollError(a.last_poll_error)}
              </p>
            </div>
          ))}
        </div>
      )}

      {!loading && accounts.length === 0 ? (
        <EmptyState
          icon={Mail}
          title="ยังไม่มีกล่องเมล"
          description="เพิ่มกล่องเมล Shopee เพื่อเริ่มดึงอีเมลและสร้างบิลซื้ออัตโนมัติ"
          action={
            <Button onClick={handleAdd}>
              <Plus className="h-4 w-4" />
              เพิ่มกล่องเมลแรก
            </Button>
          }
        />
      ) : (
        <TooltipProvider delayDuration={0}>
          <DataTable<IMAPAccountFull>
            data={accounts}
            loading={loading}
            empty="ยังไม่มีกล่องเมล"
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
                header: 'ประเภท',
                cell: (a) => {
                  const meta = CHANNEL_META[a.channel] ?? CHANNEL_META.general
                  return (
                    <div className="flex flex-col gap-1">
                      <Badge variant="secondary" className={cn('w-fit', meta.cls)}>
                        {meta.label}
                      </Badge>
                      {a.channel === 'shopee' && (
                        <span
                          className={cn(
                            'max-w-[260px] truncate text-[11px]',
                            a.shopee_domains ? 'text-muted-foreground' : 'text-info',
                          )}
                          title={a.shopee_domains || 'เว้นว่าง = รับทุกผู้ส่งที่ผ่านคำกรองหัวข้อ'}
                        >
                          ผู้ส่งที่ยอมรับ: {a.shopee_domains ? compactList(a.shopee_domains) : 'ทุกผู้ส่ง'}
                        </span>
                      )}
                    </div>
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
                header: 'ดึงล่าสุด',
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
                header: 'สร้างบิล',
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
                header: 'รอบดึง',
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
                      <TooltipContent>ดึงอีเมลตอนนี้</TooltipContent>
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
            มีกล่องเมลที่ดึงไม่สำเร็จ 3 ครั้งติด — ผู้ดูแลได้รับ LINE แจ้งเตือนแล้ว
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
