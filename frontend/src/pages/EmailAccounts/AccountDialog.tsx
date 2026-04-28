import { useEffect, useState } from 'react'
import { Check, ExternalLink, Eye, EyeOff, FolderTree, HelpCircle, Loader2, X } from 'lucide-react'
import { toast } from 'sonner'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { TagInput } from '@/components/common/TagInput'
import client from '@/api/client'
import { cn } from '@/lib/utils'

export interface IMAPAccount {
  id: string
  name: string
  host: string
  port: number
  username: string
  password?: string
  mailbox: string
  filter_from: string
  filter_subjects: string
  channel: 'general' | 'shopee' | 'lazada'
  shopee_domains: string
  lookback_days: number
  poll_interval_seconds: number
  enabled: boolean
}

interface FormState {
  name: string
  host: string
  port: number
  username: string
  password: string
  mailbox: string
  filter_from: string[]
  filter_subjects: string[]
  channel: 'general' | 'shopee' | 'lazada'
  shopee_domains: string[]
  lookback_days: number
  poll_interval_minutes: number
  enabled: boolean
}

const DEFAULTS: FormState = {
  name: '',
  host: 'imap.gmail.com',
  port: 993,
  username: '',
  password: '',
  mailbox: 'INBOX',
  filter_from: [],
  filter_subjects: [],
  channel: 'general',
  shopee_domains: [],
  lookback_days: 30,
  poll_interval_minutes: 5,
  enabled: true,
}

const SHOPEE_DEFAULT_DOMAINS = ['shopee.co.th', 'mail.shopee.co.th', 'noreply.shopee.co.th']
const SHOPEE_DEFAULT_SUBJECTS = ['คำสั่งซื้อ', 'ถูกจัดส่งแล้ว']

interface ProviderGuide {
  name: string
  url: string
  steps: string[]
  note?: string
}

const PROVIDER_GUIDES: Array<{ match: RegExp; guide: ProviderGuide }> = [
  {
    match: /gmail|google/i,
    guide: {
      name: 'Gmail',
      url: 'https://myaccount.google.com/apppasswords',
      steps: [
        'เข้า Google Account → Security',
        'เปิด 2-Step Verification ก่อน (จำเป็น — ไม่งั้นเมนู App passwords จะไม่ปรากฏ)',
        'เปิดหน้า App passwords ตาม link ด้านบน',
        'เลือก App = "Mail", Device = ระบุชื่อเช่น "BillFlow"',
        'กด Generate → ได้ password 16 ตัวอักษร (ตัวอย่าง: abcd efgh ijkl mnop) — ลบเว้นวรรคออกได้',
        'Copy ครั้งเดียวเท่านั้น — ปิดหน้าแล้วดูไม่ได้อีก',
      ],
      note: 'ห้ามใช้ password Google จริง — ใช้ App Password (16 หลัก) เท่านั้น',
    },
  },
  {
    match: /outlook|hotmail|live|office365/i,
    guide: {
      name: 'Outlook / Microsoft 365',
      url: 'https://account.microsoft.com/security/app-passwords',
      steps: [
        'เข้า account.microsoft.com → Security',
        'เปิด Two-step verification ก่อน',
        'เลือก "Create a new app password"',
        'ได้ password มาใช้งาน — copy ทั้งสตริง',
      ],
      note: 'Outlook IMAP host = imap-mail.outlook.com, port 993',
    },
  },
]

function getProviderGuide(host: string): ProviderGuide | null {
  for (const { match, guide } of PROVIDER_GUIDES) {
    if (match.test(host)) return guide
  }
  return null
}

function AppPasswordHelp({ host }: { host: string }) {
  const guide = getProviderGuide(host)
  return (
    <Popover>
      <PopoverTrigger asChild>
        <button
          type="button"
          className="inline-flex items-center gap-1 text-xs text-primary hover:underline"
        >
          <HelpCircle className="h-3 w-3" />
          วิธีรับ App Password
        </button>
      </PopoverTrigger>
      <PopoverContent className="w-[420px]" align="end">
        {guide ? (
          <div className="space-y-3">
            <div>
              <h4 className="text-sm font-semibold">วิธีสร้าง App Password ({guide.name})</h4>
              <a
                href={guide.url}
                target="_blank"
                rel="noopener noreferrer"
                className="mt-1 inline-flex items-center gap-1 text-xs text-primary hover:underline"
              >
                {guide.url}
                <ExternalLink className="h-3 w-3" />
              </a>
            </div>
            <ol className="space-y-1.5 pl-4 text-xs text-foreground">
              {guide.steps.map((s, i) => (
                <li key={i} className="list-decimal">
                  {s}
                </li>
              ))}
            </ol>
            {guide.note && (
              <p className="rounded-md bg-warning/10 px-2.5 py-1.5 text-xs text-warning">
                ⚠️ {guide.note}
              </p>
            )}
          </div>
        ) : (
          <div className="space-y-2 text-xs">
            <h4 className="text-sm font-semibold">App Password</h4>
            <p>
              IMAP server แต่ละที่อาจต้อง <b>App Password</b> (token แทน password จริง)
              ดูคู่มือของผู้ให้บริการ — มักจะอยู่ที่หน้า "Security" หรือ "Two-step verification"
            </p>
            <p className="text-muted-foreground">
              ตัวอย่าง: Gmail = myaccount.google.com/apppasswords, Outlook =
              account.microsoft.com/security/app-passwords
            </p>
          </div>
        )}
      </PopoverContent>
    </Popover>
  )
}

function csvToArray(s: string): string[] {
  return s
    .split(',')
    .map((x) => x.trim())
    .filter(Boolean)
}

function arrayToCSV(a: string[]): string {
  return a.join(', ')
}

function fromAccount(a: IMAPAccount | null): FormState {
  if (!a) return DEFAULTS
  return {
    name: a.name,
    host: a.host,
    port: a.port,
    username: a.username,
    password: '',
    mailbox: a.mailbox,
    filter_from: csvToArray(a.filter_from),
    filter_subjects: csvToArray(a.filter_subjects),
    channel: a.channel,
    shopee_domains: csvToArray(a.shopee_domains),
    lookback_days: a.lookback_days,
    poll_interval_minutes: Math.max(5, Math.round(a.poll_interval_seconds / 60)),
    enabled: a.enabled,
  }
}

function toUpsert(f: FormState) {
  return {
    name: f.name,
    host: f.host,
    port: f.port,
    username: f.username,
    password: f.password,
    mailbox: f.mailbox || 'INBOX',
    filter_from: arrayToCSV(f.filter_from),
    filter_subjects: arrayToCSV(f.filter_subjects),
    channel: f.channel,
    shopee_domains: arrayToCSV(f.shopee_domains),
    lookback_days: f.lookback_days,
    poll_interval_seconds: Math.max(300, f.poll_interval_minutes * 60),
    enabled: f.enabled,
  }
}

export function AccountDialog({
  open,
  onOpenChange,
  account,
  onSaved,
}: {
  open: boolean
  onOpenChange: (o: boolean) => void
  account: IMAPAccount | null
  onSaved: () => void
}) {
  const [form, setForm] = useState<FormState>(DEFAULTS)
  const [showPwd, setShowPwd] = useState(false)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<
    | null
    | { ok: boolean; message: string; ms?: number }
  >(null)
  const [folders, setFolders] = useState<string[]>([])
  const [loadingFolders, setLoadingFolders] = useState(false)

  const editing = account !== null

  useEffect(() => {
    if (open) {
      setForm(fromAccount(account))
      setTestResult(null)
      setFolders([])
      setShowPwd(false)
    }
  }, [open, account])

  // First time switching to shopee, pre-fill defaults so admin doesn't have
  // to remember the three Shopee domains.
  useEffect(() => {
    if (form.channel === 'shopee') {
      if (form.shopee_domains.length === 0) {
        setForm((p) => ({ ...p, shopee_domains: SHOPEE_DEFAULT_DOMAINS }))
      }
      if (form.filter_subjects.length === 0) {
        setForm((p) => ({ ...p, filter_subjects: SHOPEE_DEFAULT_SUBJECTS }))
      }
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [form.channel])

  const set = <K extends keyof FormState>(k: K, v: FormState[K]) =>
    setForm((p) => ({ ...p, [k]: v }))

  const handleTest = async () => {
    setTesting(true)
    setTestResult(null)
    try {
      const url = editing
        ? `/api/settings/imap-accounts/test?id=${account.id}`
        : '/api/settings/imap-accounts/test'
      const res = await client.post<{ ok: boolean; error?: string; duration_ms?: number }>(
        url,
        toUpsert(form),
      )
      setTestResult({
        ok: res.data.ok,
        message: res.data.ok
          ? 'เชื่อมต่อสำเร็จ'
          : res.data.error || 'เชื่อมต่อไม่สำเร็จ',
        ms: res.data.duration_ms,
      })
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      setTestResult({ ok: false, message: msg || 'request failed' })
    } finally {
      setTesting(false)
    }
  }

  const handleListFolders = async () => {
    setLoadingFolders(true)
    try {
      const url = editing
        ? `/api/settings/imap-accounts/list-folders?id=${account.id}`
        : '/api/settings/imap-accounts/list-folders'
      const res = await client.post<{ folders?: string[]; error?: string }>(
        url,
        toUpsert(form),
      )
      const list = res.data.folders ?? []
      setFolders(list)
      if (res.data.error) {
        toast.error('โหลด folders ไม่สำเร็จ — ' + res.data.error)
      }
    } catch (e: unknown) {
      toast.error('โหลด folders ไม่สำเร็จ')
    } finally {
      setLoadingFolders(false)
    }
  }

  const handleSave = async () => {
    if (!editing && !form.password) {
      toast.error('กรุณากรอก password')
      return
    }
    if (form.poll_interval_minutes < 5) {
      toast.error('Poll interval ต้องไม่ต่ำกว่า 5 นาที')
      return
    }
    setSaving(true)
    try {
      const body = toUpsert(form)
      if (editing) {
        await client.put(`/api/settings/imap-accounts/${account!.id}`, body)
        toast.success('บันทึกแล้ว')
      } else {
        await client.post('/api/settings/imap-accounts', body)
        toast.success('เพิ่ม inbox สำเร็จ')
      }
      onSaved()
      onOpenChange(false)
    } catch (e: unknown) {
      const msg = (e as { response?: { data?: { error?: string } } })?.response?.data?.error
      toast.error('บันทึกไม่สำเร็จ: ' + (msg || 'unknown'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl max-h-[92vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{editing ? 'แก้ไข Inbox' : 'เพิ่ม Inbox ใหม่'}</DialogTitle>
          <DialogDescription>
            ตั้งค่า IMAP สำหรับดึงอีเมลเข้าระบบ — Poll interval ขั้นต่ำ 5 นาที
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="ac-name">ชื่อ</Label>
              <Input
                id="ac-name"
                value={form.name}
                onChange={(e) => set('name', e.target.value)}
                placeholder="เช่น Main inbox"
              />
            </div>
            <div className="space-y-1.5">
              <Label>Channel</Label>
              <Select
                value={form.channel}
                onValueChange={(v) => set('channel', v as FormState['channel'])}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="general">General (PDF / รูปภาพ)</SelectItem>
                  <SelectItem value="shopee">Shopee (Order + Shipped)</SelectItem>
                  <SelectItem value="lazada">Lazada (WIP)</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>

          <div className="grid grid-cols-3 gap-3">
            <div className="col-span-2 space-y-1.5">
              <Label htmlFor="ac-host">Host</Label>
              <Input
                id="ac-host"
                value={form.host}
                onChange={(e) => set('host', e.target.value)}
                placeholder="imap.gmail.com"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ac-port">Port</Label>
              <Input
                id="ac-port"
                type="number"
                value={form.port}
                onChange={(e) => set('port', Number(e.target.value))}
              />
            </div>
          </div>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="ac-user">Username (อีเมล)</Label>
              <Input
                id="ac-user"
                value={form.username}
                onChange={(e) => set('username', e.target.value)}
                placeholder="billing@company.com"
                autoComplete="off"
              />
            </div>
            <div className="space-y-1.5">
              <div className="flex items-center justify-between">
                <Label htmlFor="ac-pwd">
                  Password
                  {editing && (
                    <span className="ml-1 text-xs font-normal text-muted-foreground">
                      (เว้นว่างถ้าไม่เปลี่ยน)
                    </span>
                  )}
                </Label>
                <AppPasswordHelp host={form.host} />
              </div>
              <div className="relative">
                <Input
                  id="ac-pwd"
                  type={showPwd ? 'text' : 'password'}
                  value={form.password}
                  onChange={(e) => set('password', e.target.value)}
                  placeholder={editing ? '••••••••' : 'App Password 16 หลัก'}
                  autoComplete="off"
                  className="pr-9"
                />
                <button
                  type="button"
                  className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                  onClick={() => setShowPwd((p) => !p)}
                  aria-label={showPwd ? 'ซ่อน password' : 'แสดง password'}
                >
                  {showPwd ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                </button>
              </div>
            </div>
          </div>

          <div className="space-y-1.5">
            <Label>Mailbox folder</Label>
            <div className="flex gap-2">
              {folders.length > 0 ? (
                <Select value={form.mailbox} onValueChange={(v) => set('mailbox', v)}>
                  <SelectTrigger className="flex-1">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {folders.map((f) => (
                      <SelectItem key={f} value={f}>
                        {f}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              ) : (
                <Input
                  value={form.mailbox}
                  onChange={(e) => set('mailbox', e.target.value)}
                  placeholder="INBOX"
                  className="flex-1"
                />
              )}
              <Button
                type="button"
                variant="outline"
                onClick={handleListFolders}
                disabled={loadingFolders || !form.username || !form.host}
              >
                {loadingFolders ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <FolderTree className="h-4 w-4" />
                )}
                โหลดรายการ
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              ถ้ามีกฎ filter ใน Gmail/Outlook ส่งอีเมลเฉพาะไป folder อื่น เลือกได้
            </p>
          </div>

          <div className="space-y-1.5">
            <Label>Filter From (กรองอีเมลตามผู้ส่ง — ว่าง = ทุกคน)</Label>
            <TagInput
              value={form.filter_from}
              onChange={(v) => set('filter_from', v)}
              placeholder="vendor@company.com"
            />
          </div>

          <div className="space-y-1.5">
            <Label>Filter Subjects (keyword ใน subject — ว่าง = ทุก subject)</Label>
            <TagInput
              value={form.filter_subjects}
              onChange={(v) => set('filter_subjects', v)}
              placeholder="เช่น PO, ใบสั่งซื้อ, ถูกจัดส่งแล้ว"
              lower
            />
          </div>

          {form.channel === 'shopee' && (
            <div className="space-y-1.5">
              <Label>Shopee email domains (ผู้ส่งจาก domain ใดบ้าง)</Label>
              <TagInput
                value={form.shopee_domains}
                onChange={(v) => set('shopee_domains', v)}
                placeholder="shopee.co.th"
                lower
              />
              <p className="text-xs text-muted-foreground">
                Default: shopee.co.th, mail.shopee.co.th, noreply.shopee.co.th
              </p>
            </div>
          )}

          <div className="grid grid-cols-3 gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="ac-lookback">Lookback (วัน)</Label>
              <Input
                id="ac-lookback"
                type="number"
                min={1}
                max={90}
                value={form.lookback_days}
                onChange={(e) => set('lookback_days', Number(e.target.value))}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ac-interval">Poll interval (นาที)</Label>
              <Input
                id="ac-interval"
                type="number"
                min={5}
                value={form.poll_interval_minutes}
                onChange={(e) => set('poll_interval_minutes', Number(e.target.value))}
              />
              <p className="text-xs text-muted-foreground">ขั้นต่ำ 5 นาที</p>
            </div>
            <div className="space-y-1.5">
              <Label>Enabled</Label>
              <div className="flex h-9 items-center">
                <Switch
                  checked={form.enabled}
                  onCheckedChange={(c) => set('enabled', c)}
                />
                <span className="ml-2 text-sm text-muted-foreground">
                  {form.enabled ? 'เปิดใช้งาน' : 'ปิดใช้งาน'}
                </span>
              </div>
            </div>
          </div>

          {testResult && (
            <Alert
              variant={testResult.ok ? 'default' : 'destructive'}
              className={cn(testResult.ok && 'border-success/30 bg-success/5 text-success')}
            >
              {testResult.ok ? (
                <Check className="h-4 w-4" />
              ) : (
                <X className="h-4 w-4" />
              )}
              <AlertDescription>
                {testResult.message}
                {testResult.ms !== undefined && (
                  <span className="ml-2 text-xs opacity-70">
                    ({testResult.ms} ms)
                  </span>
                )}
              </AlertDescription>
            </Alert>
          )}
        </div>

        <DialogFooter className="gap-2 sm:justify-between">
          <Button variant="outline" onClick={handleTest} disabled={testing || saving}>
            {testing ? <Loader2 className="h-4 w-4 animate-spin" /> : <Check className="h-4 w-4" />}
            ทดสอบการเชื่อมต่อ
          </Button>
          <div className="flex gap-2">
            <Button variant="ghost" onClick={() => onOpenChange(false)} disabled={saving}>
              ยกเลิก
            </Button>
            <Button onClick={handleSave} disabled={saving}>
              {saving && <Loader2 className="h-4 w-4 animate-spin" />}
              บันทึก
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
