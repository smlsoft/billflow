import { useEffect, useState } from 'react'
import axios from 'axios'
import {
  AlertCircle,
  ChevronDown,
  ExternalLink,
  FileText,
  Info,
  ListFilter,
  Loader2,
  Mail,
  Pencil,
  PlayCircle,
  Plus,
  RotateCcw,
  Search,
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
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
const GMAIL_SECURITY_URL = 'https://myaccount.google.com/security'
const GMAIL_APP_PASSWORDS_URL = 'https://myaccount.google.com/apppasswords'
const GMAIL_IMAP_SETTINGS_URL = 'https://mail.google.com/mail/u/0/#settings/fwdandpop'

interface IMAPAccountFull extends IMAPAccount {
  last_polled_at?: string | null
  last_poll_status?: string | null
  last_poll_error?: string | null
  last_poll_messages?: number | null
  last_poll_found?: number | null
  last_poll_processed?: number | null
  last_poll_skipped?: number | null
  last_poll_details?: IMAPPollDetail[]
  last_seen_uid?: number
  last_poll_limited?: boolean
  last_poll_backlog?: number | null
  consecutive_failures?: number
}

interface IMAPPollDetail {
  uid?: number
  message_id?: string
  subject?: string
  from?: string
  email_date?: string
  status: 'processed' | 'skipped' | string
  reason_code?: string
  reason_label?: string
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
              Gmail ต้องเตรียม 3 อย่างก่อนเชื่อม: เปิด <b>2-Step Verification</b>, สร้าง{' '}
              <b>App Password 16 ตัวอักษร</b>, และเปิด <b>IMAP</b> ใน Gmail. วาง App Password
              แบบมีช่องว่างได้ เช่น <code>qzqq vwqb zydo dtsi</code> ระบบจะส่งเป็น{' '}
              <code>qzqqvwqbzydodtsi</code> อัตโนมัติ.
            </div>
            <div className="flex flex-wrap gap-2">
              {[
                ['เปิด 2-Step Verification', GMAIL_SECURITY_URL],
                ['สร้าง App Password', GMAIL_APP_PASSWORDS_URL],
                ['เปิด Gmail IMAP', GMAIL_IMAP_SETTINGS_URL],
              ].map(([label, href]) => (
                <a
                  key={href}
                  href={href}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex h-8 items-center gap-1 rounded-md border border-border bg-background px-2.5 text-xs font-medium text-foreground hover:bg-accent"
                >
                  {label}
                  <ExternalLink className="h-3 w-3 text-muted-foreground" />
                </a>
              ))}
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
  if (s === 'warning' || s === 'backlog' || s === 'partial') return 'warning'
  return 'danger'
}

function statusLabel(s?: string | null): string {
  if (!s) return 'ยังไม่ poll'
  switch (s) {
    case 'ok': return 'สำเร็จ'
    case 'warning': return 'มีคำเตือน'
    case 'backlog': return 'กำลังทยอยอ่าน'
    case 'partial': return 'อ่านได้บางส่วน'
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
  if (lower.includes('closed network connection') || lower.includes('fetch envelope')) {
    return 'Gmail ตัดการเชื่อมต่อระหว่างอ่านอีเมลจำนวนมาก ระบบบันทึกส่วนที่อ่านได้แล้วและจะทยอยทำต่อในรอบถัดไป ถ้าเจอบ่อยให้ลดช่วงย้อนหลังเหลือ 3-7 วัน'
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
  if (
    lower.includes('authenticationfailed') ||
    lower.includes('authenticate') ||
    lower.includes('invalid credentials') ||
    lower.includes('password')
  ) {
    return 'Gmail/IMAP ยืนยันตัวตนไม่ผ่าน: ให้ใช้ App Password 16 ตัวจาก Google ไม่ใช่รหัสผ่าน Gmail ปกติ, ตรวจว่าเปิด 2-Step Verification แล้ว, เปิด IMAP แล้ว, และวางรหัสได้แม้มีช่องว่างเพราะระบบจะลบให้'
  }
  return error
}

function pollProgressHint(a: IMAPAccountFull): string {
  const backlog = a.last_poll_backlog ?? 0
  if (a.last_poll_status === 'partial') {
    return backlog > 0
      ? `Gmail ตัดการเชื่อมต่อระหว่างอ่านเมลจำนวนมาก เหลือรอรอบถัดไปประมาณ ${backlog} ฉบับ`
      : 'Gmail ตัดการเชื่อมต่อระหว่างอ่านเมล แต่ระบบบันทึกส่วนที่อ่านได้แล้ว'
  }
  if (a.last_poll_status === 'backlog' || a.last_poll_limited) {
    return backlog > 0
      ? `กล่องนี้มีเมลรออ่านเยอะ ระบบจึงทยอยอ่านเป็นชุด เหลือประมาณ ${backlog} ฉบับ`
      : 'ระบบทยอยอ่านเมลเป็นชุดเพื่อไม่ให้ Gmail ตัดการเชื่อมต่อ'
  }
  const found = a.last_poll_found ?? 0
  if (found >= 300) {
    return 'รอบล่าสุดพบเมลจำนวนมาก ถ้าเป็นการตั้งค่าครั้งแรกแนะนำลดช่วงย้อนหลังเหลือ 3-7 วันหลังเก็บเมลเก่าเสร็จ'
  }
  return ''
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

function pollReasonSummary(details: IMAPPollDetail[]) {
  const counts = new Map<string, { label: string; count: number }>()
  details.forEach((d) => {
    if (d.status !== 'skipped') return
    const code = d.reason_code || d.reason_label || 'unknown'
    const label = d.reason_label || d.reason_code || 'ไม่ทราบเหตุผล'
    const current = counts.get(code)
    counts.set(code, { label, count: (current?.count ?? 0) + 1 })
  })
  return Array.from(counts.values()).sort((a, b) => b.count - a.count)
}

function pollDetailStatusLabel(status: string): string {
  switch (status) {
    case 'processed':
      return 'ประมวลผล'
    case 'skipped':
      return 'ข้าม'
    default:
      return status || 'ไม่ทราบสถานะ'
  }
}

function pollDetailStatusClass(status: string): string {
  switch (status) {
    case 'processed':
      return 'bg-success/10 text-success'
    case 'skipped':
      return 'bg-warning/15 text-warning'
    default:
      return 'bg-muted text-muted-foreground'
  }
}

function LatestPollDetailsButton({
  account,
  onOpen,
}: {
  account: IMAPAccountFull
  onOpen: (account: IMAPAccountFull) => void
}) {
  const details = account.last_poll_details ?? []
  if (details.length === 0) {
    return null
  }
  const processed = details.filter((d) => d.status === 'processed').length
  const skipped = details.filter((d) => d.status === 'skipped').length
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      className="mt-1 h-7 w-fit gap-1 px-2 text-[11px]"
      onClick={() => onOpen(account)}
    >
      <ListFilter className="h-3 w-3" />
      รายละเอียด {details.length}
      <span className="text-muted-foreground">
        {processed > 0 && ` / ผ่าน ${processed}`}
        {skipped > 0 && ` / ข้าม ${skipped}`}
      </span>
    </Button>
  )
}

function PollDetailsDialog({
  account,
  onOpenChange,
}: {
  account: IMAPAccountFull | null
  onOpenChange: (open: boolean) => void
}) {
  const [query, setQuery] = useState('')
  const [status, setStatus] = useState('all')
  const details = account?.last_poll_details ?? []
  const processed = details.filter((d) => d.status === 'processed').length
  const skipped = details.filter((d) => d.status === 'skipped').length
  const reasonSummary = pollReasonSummary(details)
  const filtered = details.filter((d) => {
    if (status !== 'all' && d.status !== status) return false
    const q = query.trim().toLowerCase()
    if (!q) return true
    return [d.subject, d.from, d.reason_label, d.reason_code, d.message_id, String(d.uid ?? '')]
      .join(' ')
      .toLowerCase()
      .includes(q)
  })

  return (
    <Dialog open={!!account} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[86vh] max-w-5xl overflow-hidden p-0">
        <DialogHeader className="border-b border-border px-5 py-4">
          <DialogTitle>รายละเอียดเมลรอบล่าสุด</DialogTitle>
          <DialogDescription>
            {account?.name} · {account?.username} · พบ {account?.last_poll_found ?? details.length} ฉบับ
          </DialogDescription>
        </DialogHeader>

        <div className="grid grid-cols-2 gap-2 border-b border-border bg-muted/30 px-5 py-3 text-xs sm:grid-cols-4">
          <div className="rounded-md border border-border bg-background p-2">
            <div className="text-muted-foreground">แสดงในรายละเอียด</div>
            <div className="mt-1 text-lg font-semibold tabular-nums">{details.length}</div>
          </div>
          <div className="rounded-md border border-border bg-background p-2">
            <div className="text-muted-foreground">ประมวลผล</div>
            <div className="mt-1 text-lg font-semibold tabular-nums text-success">{processed}</div>
          </div>
          <div className="rounded-md border border-border bg-background p-2">
            <div className="text-muted-foreground">ข้าม</div>
            <div className="mt-1 text-lg font-semibold tabular-nums text-warning">{skipped}</div>
          </div>
          <div className="rounded-md border border-border bg-background p-2">
            <div className="text-muted-foreground">รอรอบถัดไป</div>
            <div className="mt-1 text-lg font-semibold tabular-nums text-info">
              {account?.last_poll_backlog ?? 0}
            </div>
          </div>
        </div>

        {reasonSummary.length > 0 && (
          <div className="border-b border-border px-5 py-3">
            <div className="mb-2 flex items-center gap-2 text-xs font-semibold text-muted-foreground">
              <AlertCircle className="h-3.5 w-3.5" />
              สรุปเหตุผลที่ข้าม
            </div>
            <div className="flex flex-wrap gap-2">
              {reasonSummary.slice(0, 6).map((r) => (
                <span
                  key={r.label}
                  className="inline-flex items-center gap-2 rounded-md border border-border bg-background px-2.5 py-1 text-xs"
                >
                  <span className="max-w-[260px] truncate text-muted-foreground">{r.label}</span>
                  <span className="font-mono font-semibold tabular-nums text-warning">{r.count}</span>
                </span>
              ))}
            </div>
          </div>
        )}

        <div className="flex flex-col gap-2 border-b border-border px-5 py-3 sm:flex-row">
          <div className="relative flex-1">
            <Search className="pointer-events-none absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="ค้นหาเลขคำสั่งซื้อ, ผู้ส่ง, เหตุผลที่ข้าม"
              className="pl-8"
            />
          </div>
          <Select value={status} onValueChange={setStatus}>
            <SelectTrigger className="w-full sm:w-[180px]">
              <SelectValue placeholder="สถานะ" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">ทุกสถานะ</SelectItem>
              <SelectItem value="processed">ประมวลผล</SelectItem>
              <SelectItem value="skipped">ข้าม</SelectItem>
            </SelectContent>
          </Select>
        </div>

        <div className="max-h-[48vh] overflow-auto px-5 py-3">
          <div className="min-w-[820px] overflow-hidden rounded-md border border-border">
            <div className="grid grid-cols-[88px_1.4fr_1fr_130px_1.1fr] gap-3 border-b border-border bg-muted/50 px-3 py-2 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
              <span>สถานะ</span>
              <span>หัวข้อ</span>
              <span>ผู้ส่ง</span>
              <span>เวลาอีเมล</span>
              <span>เหตุผล</span>
            </div>
            {filtered.length === 0 ? (
              <div className="px-3 py-8 text-center text-sm text-muted-foreground">ไม่พบรายการที่ตรงกับเงื่อนไข</div>
            ) : (
              filtered.map((d, idx) => (
                <div
                  key={`${d.message_id || d.uid || idx}-${idx}`}
                  className="grid grid-cols-[88px_1.4fr_1fr_130px_1.1fr] gap-3 border-b border-border/70 px-3 py-2 text-xs last:border-0"
                >
                  <span className={cn('h-fit w-fit rounded-full px-2 py-0.5 text-[11px] font-semibold', pollDetailStatusClass(d.status))}>
                    {pollDetailStatusLabel(d.status)}
                  </span>
                  <div className="min-w-0">
                    <div className="truncate font-medium text-foreground" title={d.subject || undefined}>
                      {d.subject || 'ไม่มีหัวข้อ'}
                    </div>
                    <div className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground" title={d.message_id || undefined}>
                      {d.message_id || (d.uid ? `UID ${d.uid}` : '')}
                    </div>
                  </div>
                  <span className="truncate text-muted-foreground" title={d.from || undefined}>
                    {d.from || 'ไม่พบผู้ส่ง'}
                  </span>
                  <span className="tabular-nums text-muted-foreground">
                    {d.email_date ? dayjs(d.email_date).format('DD/MM/YY HH:mm') : '-'}
                  </span>
                  <span className="line-clamp-2 text-muted-foreground" title={d.reason_label || d.reason_code || undefined}>
                    {d.reason_label || d.reason_code || 'ไม่มีรายละเอียด'}
                  </span>
                </div>
              ))
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function ResetProgressDialog({
  account,
  onOpenChange,
  onConfirm,
}: {
  account: IMAPAccountFull | null
  onOpenChange: (open: boolean) => void
  onConfirm: (account: IMAPAccountFull, lookbackDays: number, pollNow: boolean) => Promise<void>
}) {
  const [lookbackDays, setLookbackDays] = useState('7')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (account) {
      setLookbackDays(String(Math.min(Math.max(account.lookback_days || 7, 1), 90)))
    }
  }, [account])

  const handleConfirm = async (pollNow: boolean) => {
    if (!account) return
    const days = Number(lookbackDays)
    if (!Number.isFinite(days) || days < 1 || days > 90) {
      toast.error('ช่วงย้อนหลังต้องอยู่ระหว่าง 1-90 วัน')
      return
    }
    setBusy(true)
    try {
      await onConfirm(account, Math.floor(days), pollNow)
      onOpenChange(false)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={!!account} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>เริ่มอ่านย้อนหลังใหม่</DialogTitle>
          <DialogDescription>
            รีเซ็ตตำแหน่งอ่านล่าสุดของ {account?.name} แล้วให้ระบบอ่านใหม่ตามช่วงย้อนหลังที่เลือก
            โดยยังใช้ประวัติ dedup เดิมเพื่อกันสร้างบิลซ้ำ
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <label className="block space-y-1.5 text-sm">
            <span className="font-medium">อ่านย้อนหลัง</span>
            <Input
              type="number"
              min={1}
              max={90}
              value={lookbackDays}
              onChange={(e) => setLookbackDays(e.target.value)}
            />
            <span className="block text-xs text-muted-foreground">
              แนะนำ 3-7 วันเมื่อต้องไล่เมล์จำนวนมาก ถ้าต้องย้อนนานกว่านี้ระบบจะทยอยอ่านเป็นหลายรอบ
            </span>
          </label>
          <div className="rounded-md border border-warning/30 bg-warning/5 px-3 py-2 text-xs text-warning">
            การรีเซ็ตนี้ไม่ลบ inbox และไม่ล้างประวัติเมล์ที่เคยสร้างบิลแล้ว จึงปลอดภัยกว่าการล้าง dedup ทั้งระบบ
          </div>
        </div>
        <DialogFooter className="gap-2 sm:gap-2">
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>
            ยกเลิก
          </Button>
          <Button type="button" variant="outline" onClick={() => handleConfirm(false)} disabled={busy}>
            รีเซ็ตอย่างเดียว
          </Button>
          <Button type="button" onClick={() => handleConfirm(true)} disabled={busy}>
            {busy ? 'กำลังดำเนินการ…' : 'รีเซ็ตและอ่านทันที'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export default function EmailAccounts() {
  const [accounts, setAccounts] = useState<IMAPAccountFull[]>([])
  const [loading, setLoading] = useState(true)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editing, setEditing] = useState<IMAPAccount | null>(null)
  const [deleteId, setDeleteId] = useState<string | null>(null)
  const [detailAccount, setDetailAccount] = useState<IMAPAccountFull | null>(null)
  const [resetAccount, setResetAccount] = useState<IMAPAccountFull | null>(null)
  const [pollingIds, setPollingIds] = useState<Set<string>>(new Set())

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
    if (pollingIds.has(a.id)) return
    setPollingIds((prev) => new Set(prev).add(a.id))
    const id = toast.loading(`กำลังดึงอีเมล ${a.name}…`)
    try {
      const res = await client.post<{
        status: string
        messages_found: number
        processed: number
        skipped: number
        backlog?: number
        limited?: boolean
        duration_ms: number
        error?: string
      }>(`/api/settings/imap-accounts/${a.id}/poll`, undefined, {
        timeout: 180000,
      })
      const r = res.data
      if (r.status === 'ok') {
        toast.success(
          `ดึงเสร็จ พบ ${r.messages_found} / ประมวลผล ${r.processed} / ข้าม ${r.skipped}`,
          { id },
        )
      } else if (r.status === 'backlog' || r.status === 'partial') {
        toast.warning(
          `ดึงได้บางส่วน พบ ${r.messages_found} / ประมวลผล ${r.processed} / ข้าม ${r.skipped}${r.backlog ? ` / เหลือประมาณ ${r.backlog}` : ''}`,
          { id },
        )
      } else {
        toast.error(`ดึงอีเมลไม่สำเร็จ: ${friendlyPollError(r.error || r.status)}`, { id })
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
    } finally {
      setPollingIds((prev) => {
        const next = new Set(prev)
        next.delete(a.id)
        return next
      })
    }
  }

  const handleResetProgress = async (a: IMAPAccountFull, lookbackDays: number, pollNow: boolean) => {
    const id = toast.loading(
      pollNow ? `กำลังรีเซ็ตและดึงอีเมล ${a.name}…` : `กำลังรีเซ็ตตำแหน่งอ่าน ${a.name}…`,
    )
    try {
      const res = await client.post<{
        status?: string
        messages_found?: number
        processed?: number
        skipped?: number
        backlog?: number
      }>(
        `/api/settings/imap-accounts/${a.id}/reset-progress`,
        { lookback_days: lookbackDays, poll_now: pollNow },
        { timeout: pollNow ? 180000 : 30000 },
      )
      if (pollNow && res.data.status) {
        toast.success(
          `เริ่มอ่านใหม่แล้ว พบ ${res.data.messages_found ?? 0} / ประมวลผล ${res.data.processed ?? 0} / ข้าม ${res.data.skipped ?? 0}${res.data.backlog ? ` / เหลือประมาณ ${res.data.backlog}` : ''}`,
          { id },
        )
      } else {
        toast.success('รีเซ็ตตำแหน่งอ่านแล้ว รอบถัดไปจะเริ่มอ่านย้อนหลังตามช่วงที่เลือก', { id })
      }
      fetchAll()
    } catch (e) {
      if (axios.isAxiosError(e) && e.code === 'ECONNABORTED') {
        toast.warning(
          'คำสั่งใช้เวลานาน ระบบอาจยังทำงานต่ออยู่ ให้รอสักครู่แล้วรีเฟรชสถานะ',
          { id },
        )
        fetchAll()
        return
      }
      const msg = axios.isAxiosError(e)
        ? e.response?.data?.error || e.message
        : ''
      toast.error(`รีเซ็ตไม่สำเร็จ${msg ? `: ${msg}` : ''}`, { id })
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
  const backlogAccounts = accounts.filter((a) => a.enabled && ((a.last_poll_backlog ?? 0) > 0 || a.last_poll_status === 'backlog' || a.last_poll_limited))

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

      {!loading && backlogAccounts.length > 0 && (
        <div className="space-y-2 rounded-md border border-info/35 bg-info/5 p-3 text-sm text-info">
          <div className="flex items-start gap-2 font-medium">
            <Info className="mt-0.5 h-4 w-4 shrink-0" />
            <span>มีกล่องเมลที่กำลังทยอยอ่านย้อนหลัง</span>
          </div>
          {backlogAccounts.map((a) => (
            <div key={a.id} className="ml-6 rounded border border-info/20 bg-background/70 px-2.5 py-2">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <div className="font-medium text-foreground">{a.name}</div>
                  <div className="mt-0.5 text-xs text-muted-foreground">
                    พบ {a.last_poll_found ?? 0} · ข้าม {a.last_poll_skipped ?? 0} · เหลือประมาณ {a.last_poll_backlog ?? 0}
                  </div>
                </div>
                <div className="flex gap-1">
                  {(a.last_poll_details?.length ?? 0) > 0 && (
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-7 px-2 text-xs"
                      onClick={() => setDetailAccount(a)}
                    >
                      <ListFilter className="h-3.5 w-3.5" />
                      ดูรายละเอียด
                    </Button>
                  )}
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-7 px-2 text-xs"
                    onClick={() => handlePollNow(a)}
                  >
                    <PlayCircle className="h-3.5 w-3.5" />
                    อ่านชุดถัดไป
                  </Button>
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-7 px-2 text-xs"
                    onClick={() => setResetAccount(a)}
                  >
                    <RotateCcw className="h-3.5 w-3.5" />
                    ตั้งช่วงย้อนหลัง
                  </Button>
                </div>
              </div>
              <p className="mt-1 text-xs text-muted-foreground">
                ระบบจำตำแหน่งล่าสุดไว้แล้วและจะอ่านต่ออัตโนมัติทุก {Math.round(a.poll_interval_seconds / 60)} นาที.
                ถ้าตัวเลขเหลือสูงมากหลังตั้งค่าครั้งแรก ให้ลดช่วงย้อนหลังเหลือ 3-7 วันหรือจำกัดคำกรองหัวข้อให้แคบลง.
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
                  const hint = pollProgressHint(a)
                  const dot = (
                    <StatusDot
                      variant={a.enabled ? statusVariant(a.last_poll_status) : 'muted'}
                      label={a.enabled ? statusLabel(a.last_poll_status) : 'ปิดใช้'}
                    />
                  )
                  if (!a.last_poll_error && !hint) return dot
                  return (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span className="cursor-help">{dot}</span>
                      </TooltipTrigger>
                      <TooltipContent className="max-w-md">
                        {hint && <p className="text-xs">{hint}</p>}
                        {a.last_poll_error && (
                          <p className={cn('text-xs', hint ? 'mt-1 font-mono text-muted-foreground' : 'font-mono')}>
                            {a.last_poll_error}
                          </p>
                        )}
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
                header: 'ผลรอบล่าสุด',
                cell: (a) => {
                  const found = a.last_poll_found
                  const processed = a.last_poll_processed ?? a.last_poll_messages
                  const skipped = a.last_poll_skipped
                  const backlog = a.last_poll_backlog
                  const hint = pollProgressHint(a)
                  const topReason = pollReasonSummary(a.last_poll_details ?? [])[0]
                  if (found == null && processed == null && skipped == null) {
                    return <span className="text-xs text-muted-foreground">—</span>
                  }
                  return (
                    <div className="flex flex-col gap-0.5 text-xs tabular-nums">
                      {found != null && (
                        <span>
                          <span className="text-muted-foreground">พบ</span>{' '}
                          <span className="font-mono">{found}</span>
                        </span>
                      )}
                      {processed != null && (
                        <span>
                          <span className="text-muted-foreground">ประมวลผล</span>{' '}
                          <span className="font-mono">{processed}</span>
                        </span>
                      )}
                      {skipped != null && skipped > 0 && (
                        <span className="text-warning">
                          <span>ข้าม</span> <span className="font-mono">{skipped}</span>
                        </span>
                      )}
                      {backlog != null && backlog > 0 && (
                        <span className="text-info">
                          <span>รอรอบถัดไป</span> <span className="font-mono">{backlog}</span>
                        </span>
                      )}
                      {hint && (
                        <span className="max-w-[240px] text-[11px] leading-snug text-muted-foreground">
                          {hint}
                        </span>
                      )}
                      {topReason && (
                        <span className="max-w-[240px] text-[11px] leading-snug text-muted-foreground">
                          สาเหตุหลัก: {topReason.label} ({topReason.count})
                        </span>
                      )}
                      <LatestPollDetailsButton account={a} onOpen={setDetailAccount} />
                    </div>
                  )
                },
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
                cell: (a) => {
                  const polling = pollingIds.has(a.id)
                  return (
                  <div className="flex justify-end gap-1">
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button
                          size="icon"
                          variant="ghost"
                          className="h-7 w-7"
                          onClick={() => handlePollNow(a)}
                          disabled={!a.enabled || polling}
                        >
                          {polling ? (
                            <Loader2 className="h-3.5 w-3.5 animate-spin" />
                          ) : (
                            <PlayCircle className="h-3.5 w-3.5" />
                          )}
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>{polling ? 'กำลังดึงอีเมลกล่องนี้' : 'ดึงอีเมลตอนนี้'}</TooltipContent>
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
                          className="h-7 w-7"
                          onClick={() => setResetAccount(a)}
                        >
                          <RotateCcw className="h-3.5 w-3.5" />
                        </Button>
                      </TooltipTrigger>
                      <TooltipContent>ตั้งช่วงย้อนหลัง / อ่านใหม่</TooltipContent>
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
                  )
                },
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
            กรุณาตรวจ host, เปิด IMAP, และใช้ App Password 16 ตัวอักษรแทนรหัสผ่าน Gmail ปกติ
          </span>
        </div>
      )}

      <AccountDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        account={editing}
        onSaved={fetchAll}
      />

      <PollDetailsDialog
        account={detailAccount}
        onOpenChange={(open) => {
          if (!open) setDetailAccount(null)
        }}
      />

      <ResetProgressDialog
        account={resetAccount}
        onOpenChange={(open) => {
          if (!open) setResetAccount(null)
        }}
        onConfirm={handleResetProgress}
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
