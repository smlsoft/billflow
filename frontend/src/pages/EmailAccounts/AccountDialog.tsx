import { useEffect, useState } from 'react'
import {
  Check,
  ClipboardList,
  Clock,
  ExternalLink,
  Eye,
  EyeOff,
  FileText,
  FolderTree,
  HelpCircle,
  Loader2,
  Mail,
  Plug,
  ShoppingBag,
  Sparkles,
  X,
} from 'lucide-react'
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

const PHASE = Number(import.meta.env.VITE_PHASE ?? 99)

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
const SHOPEE_DEFAULT_SUBJECTS = ['ถูกจัดส่งแล้ว', 'ยืนยันการชำระเงินคำสั่งซื้อหมายเลข']

function newAccountDefaults(): FormState {
  if (PHASE < 2) {
    return {
      ...DEFAULTS,
      name: 'Shopee Inbox',
      channel: 'shopee',
      shopee_domains: SHOPEE_DEFAULT_DOMAINS,
      filter_subjects: SHOPEE_DEFAULT_SUBJECTS,
    }
  }
  return DEFAULTS
}

// ─── Provider/preset guides ──────────────────────────────────────────────────

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
        'กด Generate → ได้ password 16 ตัวอักษร — ลบเว้นวรรคออกได้',
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

// ─── Presets ────────────────────────────────────────────────────────────────

interface Preset {
  id: string
  icon: React.ComponentType<{ className?: string }>
  title: string
  subtitle: string
  apply: (current: FormState) => FormState
}

const PRESETS: Preset[] = [
  {
    id: 'gmail-shopee',
    icon: ShoppingBag,
    title: 'Gmail + Shopee',
    subtitle: 'ดึงอีเมลคำสั่งซื้อ + จัดส่งจาก Shopee',
    apply: (c) => ({
      ...c,
      host: 'imap.gmail.com',
      port: 993,
      mailbox: 'INBOX',
      channel: 'shopee',
      shopee_domains: SHOPEE_DEFAULT_DOMAINS,
      filter_subjects: SHOPEE_DEFAULT_SUBJECTS,
      lookback_days: 30,
      poll_interval_minutes: 5,
    }),
  },
  {
    id: 'gmail-general',
    icon: FileText,
    title: 'Gmail + PDF/Excel',
    subtitle: 'ดึงไฟล์แนบจากอีเมลทั่วไป (vendor/ใบสั่งซื้อ)',
    apply: (c) => ({
      ...c,
      host: 'imap.gmail.com',
      port: 993,
      mailbox: 'INBOX',
      channel: 'general',
      filter_subjects: ['PO', 'ใบสั่งซื้อ', 'Purchase Order'],
      shopee_domains: [],
      lookback_days: 30,
      poll_interval_minutes: 5,
    }),
  },
  {
    id: 'outlook-shopee',
    icon: Mail,
    title: 'Outlook + Shopee',
    subtitle: 'อีเมลธุรกิจ Microsoft 365 / Outlook',
    apply: (c) => ({
      ...c,
      host: 'imap-mail.outlook.com',
      port: 993,
      mailbox: 'INBOX',
      channel: 'shopee',
      shopee_domains: SHOPEE_DEFAULT_DOMAINS,
      filter_subjects: SHOPEE_DEFAULT_SUBJECTS,
      lookback_days: 30,
      poll_interval_minutes: 5,
    }),
  },
  {
    id: 'custom',
    icon: Sparkles,
    title: 'ตั้งค่าเอง',
    subtitle: 'กรอกทุกฟิลด์เอง — สำหรับ IMAP server อื่น ๆ',
    apply: (c) => c,
  },
]

function PresetCard({
  preset,
  selected,
  onClick,
}: {
  preset: Preset
  selected: boolean
  onClick: () => void
}) {
  const Icon = preset.icon
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'flex min-h-[86px] flex-col items-start gap-1 rounded-md border p-2.5 text-left transition-all',
        selected
          ? 'border-primary bg-primary/5 shadow-sm'
          : 'border-border hover:border-primary/40 hover:bg-accent/40',
      )}
    >
      <Icon
        className={cn(
          'h-4 w-4',
          selected ? 'text-primary' : 'text-muted-foreground',
        )}
      />
      <div className="text-sm font-semibold leading-tight">{preset.title}</div>
      <div className="text-[11px] leading-tight text-muted-foreground">
        {preset.subtitle}
      </div>
    </button>
  )
}

// ─── Form helpers ───────────────────────────────────────────────────────────

function csvToArray(s: string): string[] {
  return s.split(',').map((x) => x.trim()).filter(Boolean)
}

function arrayToCSV(a: string[]): string {
  return a.join(', ')
}

function fromAccount(a: IMAPAccount | null): FormState {
  if (!a) return newAccountDefaults()
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

// ─── Section header — visual grouping marker ─────────────────────────────────

function SectionHeader({
  icon: Icon,
  title,
  subtitle,
}: {
  icon: React.ComponentType<{ className?: string }>
  title: string
  subtitle?: string
}) {
  return (
    <div className="flex items-center gap-2 border-b border-border/60 pb-1.5">
      <Icon className="h-4 w-4 text-primary" />
      <div>
        <h4 className="text-sm font-semibold leading-none">{title}</h4>
        {subtitle && (
          <p className="mt-0.5 text-[11px] text-muted-foreground">{subtitle}</p>
        )}
      </div>
    </div>
  )
}

// Inline hint underneath an input — short single line in muted gray.
function Hint({ children }: { children: React.ReactNode }) {
  return <p className="text-[11px] leading-snug text-muted-foreground">{children}</p>
}

// ─── Dialog ─────────────────────────────────────────────────────────────────

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
    null | { ok: boolean; message: string; ms?: number }
  >(null)
  const [folders, setFolders] = useState<string[]>([])
  const [loadingFolders, setLoadingFolders] = useState(false)
  const [activePreset, setActivePreset] = useState<string | null>(null)

  const editing = account !== null

  useEffect(() => {
    if (open) {
      setForm(fromAccount(account))
      setTestResult(null)
      setFolders([])
      setShowPwd(false)
      setActivePreset(null)
    }
  }, [open, account])

  const set = <K extends keyof FormState>(k: K, v: FormState[K]) =>
    setForm((p) => ({ ...p, [k]: v }))

  const applyPreset = (p: Preset) => {
    setActivePreset(p.id)
    setForm((c) => p.apply(c))
  }

  const handleTest = async () => {
    setTesting(true)
    setTestResult(null)
    try {
      const url = editing
        ? `/api/settings/imap-accounts/test?id=${account!.id}`
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
        ? `/api/settings/imap-accounts/list-folders?id=${account!.id}`
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
    } catch {
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
        toast.success('เพิ่มกล่องเมลสำเร็จ')
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

  const isShopee = form.channel === 'shopee'
  const visiblePresets = PHASE < 2
    ? PRESETS.filter((p) => p.id === 'gmail-shopee' || p.id === 'outlook-shopee' || p.id === 'custom')
    : PRESETS

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="grid max-h-[92vh] max-w-4xl grid-rows-[auto_minmax(0,1fr)_auto] overflow-hidden">
        <DialogHeader>
          <DialogTitle>{editing ? 'แก้ไขกล่องเมล' : 'เพิ่มกล่องเมลใหม่'}</DialogTitle>
          <DialogDescription>
            {PHASE < 2
              ? 'ตั้งค่ากล่องเมลที่รับอีเมล Shopee เพื่อสร้างบิลซื้ออัตโนมัติ'
              : 'ตั้งค่าอีเมลสำหรับดึงบิลเข้า BillFlow — ดึงอีเมลได้ถี่สุดทุก 5 นาที'}
          </DialogDescription>
        </DialogHeader>

        <div className="-mx-6 space-y-4 overflow-y-auto px-6 py-1">
          {/* Preset cards — only on Add, not Edit */}
          {!editing && (
            <div className="space-y-2">
              <Label className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                เลือกรูปแบบเพื่อเริ่มต้นด่วน
              </Label>
              <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
                {visiblePresets.map((p) => (
                  <PresetCard
                    key={p.id}
                    preset={p}
                    selected={activePreset === p.id}
                    onClick={() => applyPreset(p)}
                  />
                ))}
              </div>
            </div>
          )}

          {/* ─── Section: การเชื่อมต่อ ─── */}
          <div className="space-y-2.5">
            <SectionHeader
              icon={Plug}
              title="การเชื่อมต่อ"
              subtitle="เซิร์ฟเวอร์อีเมลและข้อมูลเข้าสู่ระบบ"
            />

            <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="ac-name">ชื่อกล่องเมล</Label>
                <Input
                  id="ac-name"
                  value={form.name}
                  onChange={(e) => set('name', e.target.value)}
                  placeholder="เช่น Shopee Inbox"
                />
                <Hint>ชื่อภายในสำหรับแยกกล่องเมล — จะแสดงในตาราง</Hint>
              </div>
              {PHASE < 2 ? (
                <div className="space-y-1">
                  <Label>ประเภทอีเมล</Label>
                  <div className="flex h-10 items-center rounded-md border border-warning/30 bg-warning/5 px-3 text-sm font-medium text-foreground">
                    Shopee — ออเดอร์และบิลซื้อ
                  </div>
                  <Hint>Phase 1 ใช้เฉพาะอีเมล Shopee เพื่อสร้างบิลซื้อ</Hint>
                </div>
              ) : (
                <div className="space-y-1">
                  <Label>ประเภทอีเมล</Label>
                  <Select
                    value={form.channel}
                    onValueChange={(v) => set('channel', v as FormState['channel'])}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="general">ไฟล์แนบทั่วไป — PDF / รูป / Excel</SelectItem>
                      <SelectItem value="shopee">Shopee — ออเดอร์และบิลซื้อ</SelectItem>
                      <SelectItem value="lazada">Lazada — (กำลังพัฒนา)</SelectItem>
                    </SelectContent>
                  </Select>
                  <Hint>
                    เลือก <b>Shopee</b> ถ้าเป็นกล่องเมลสำหรับร้านค้า Shopee, <b>ไฟล์แนบทั่วไป</b>{' '}
                    สำหรับอีเมลที่มี PDF/Excel แนบ
                  </Hint>
                </div>
              )}
            </div>

            <div className="grid grid-cols-3 gap-2.5">
              <div className="col-span-2 space-y-1">
                  <Label htmlFor="ac-host">เซิร์ฟเวอร์อีเมล</Label>
                <Input
                  id="ac-host"
                  value={form.host}
                  onChange={(e) => set('host', e.target.value)}
                  placeholder="imap.gmail.com"
                />
                <Hint>
                  Gmail = <code>imap.gmail.com</code>, Outlook = <code>imap-mail.outlook.com</code>
                </Hint>
              </div>
              <div className="space-y-1">
                <Label htmlFor="ac-port">พอร์ต</Label>
                <Input
                  id="ac-port"
                  type="number"
                  value={form.port}
                  onChange={(e) => set('port', Number(e.target.value))}
                />
                <Hint>993 (TLS) ปกติ</Hint>
              </div>
            </div>

            <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="ac-user">อีเมล</Label>
                <Input
                  id="ac-user"
                  value={form.username}
                  onChange={(e) => set('username', e.target.value)}
                  placeholder="billing@company.com"
                  autoComplete="off"
                />
                <Hint>บัญชีอีเมลที่ BillFlow จะใช้ดึงข้อความ</Hint>
              </div>
              <div className="space-y-1">
                <div className="flex items-center justify-between">
                  <Label htmlFor="ac-pwd">
                    รหัสผ่าน
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
                    aria-label={showPwd ? 'ซ่อนรหัสผ่าน' : 'แสดงรหัสผ่าน'}
                  >
                    {showPwd ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                  </button>
                </div>
                <Hint>Gmail ห้ามใช้ password จริง — กดปุ่มข้างบนดูวิธีรับ App Password</Hint>
              </div>
            </div>
          </div>

          {/* ─── Section: ตำแหน่งอีเมล ─── */}
          <details className="group space-y-2.5 rounded-md border border-border/70 p-3" open>
            <summary className="list-none">
              <SectionHeader
                icon={ClipboardList}
                title="เลือกอีเมลที่จะดึง"
                subtitle="เลือกโฟลเดอร์และคำที่ใช้คัดอีเมลก่อนเข้าระบบ"
              />
            </summary>

            <div className="space-y-1">
              <Label>โฟลเดอร์อีเมล</Label>
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
              <Hint>
                ปกติใช้ <code>INBOX</code>. ถ้าตั้งกฎแยกอีเมล Shopee ไว้ในโฟลเดอร์อื่น ให้กดโหลดรายการแล้วเลือกโฟลเดอร์นั้น
              </Hint>
            </div>

            <div className="space-y-1">
              <Label>กรองผู้ส่ง</Label>
              <TagInput
                value={form.filter_from}
                onChange={(v) => set('filter_from', v)}
                placeholder="เพิ่มอีเมลหรือโดเมนผู้ส่ง แล้วกด Enter"
              />
              <Hint>
                ปล่อยว่างได้ เพราะส่วน Shopee ด้านล่างมีรายการผู้ส่งที่ยอมรับอยู่แล้ว
              </Hint>
            </div>

            <div className="space-y-1">
              <Label>กรองหัวข้ออีเมล</Label>
              <TagInput
                value={form.filter_subjects}
                onChange={(v) => set('filter_subjects', v)}
                placeholder="เช่น คำสั่งซื้อ, ถูกจัดส่งแล้ว, ยืนยันการชำระเงิน"
                lower
              />
      <Hint>
                Phase 1 แนะนำให้ใช้เฉพาะ <code>ถูกจัดส่งแล้ว</code> และ{' '}
                <code>ยืนยันการชำระเงินคำสั่งซื้อหมายเลข</code>. อย่าใส่คำกว้าง ๆ เช่น{' '}
                <code>ใบสั่งซื้อสินค้า เลขที่</code> เพราะจะดึงอีเมล PO ทั่วไปเข้ามาแล้ว AI อ่านไม่เจอรายการสินค้า
              </Hint>
            </div>
          </details>

          {/* ─── Section: Shopee config ─── */}
          <details
            open={isShopee}
            className={cn(
              'group space-y-2.5 rounded-md border p-3 transition-opacity',
              isShopee
                ? 'border-warning/30 bg-warning/5'
                : 'border-border bg-muted/20 opacity-60',
            )}
          >
            <summary className="list-none">
              <SectionHeader
                icon={ShoppingBag}
                title="Shopee — ตั้งค่าเฉพาะ"
                subtitle={
                  isShopee
                    ? 'กำหนดผู้ส่งของ Shopee เพื่อกันอีเมลปลอม'
                    : 'ส่วนนี้ใช้เฉพาะประเภท Shopee — เลือกด้านบนเพื่อเปิดใช้งาน'
                }
              />
            </summary>

            <div className="space-y-1">
              <Label>ผู้ส่ง Shopee ที่ยอมรับ</Label>
              <TagInput
                value={form.shopee_domains}
                onChange={(v) => set('shopee_domains', v)}
                placeholder={
                  isShopee
                    ? 'เพิ่ม domain หรืออีเมล แล้วกด Enter'
                    : 'ปิดใช้งาน — เปลี่ยน channel เป็น Shopee ก่อน'
                }
                lower
                className={cn(!isShopee && 'pointer-events-none')}
              />
              <Hint>
                BillFlow จะรับเฉพาะอีเมลจากโดเมนหรืออีเมลในรายการนี้ เพื่อกันอีเมลปลอม.
                ถ้ามีการ forward จากอีเมลอื่น ให้เพิ่มอีเมลผู้ forward เข้าไปได้
              </Hint>
            </div>
          </details>

          {/* ─── Section: ตารางเวลา ─── */}
          <details className="group space-y-2.5 rounded-md border border-border/70 p-3" open>
            <summary className="list-none">
              <SectionHeader
                icon={Clock}
                title="รอบการดึงอีเมล"
                subtitle="กำหนดช่วงย้อนหลังและความถี่ในการตรวจอีเมล"
              />
            </summary>

            <div className="grid grid-cols-3 gap-2.5">
              <div className="space-y-1">
                <Label htmlFor="ac-lookback">ดึงย้อนหลัง (วัน)</Label>
                <Input
                  id="ac-lookback"
                  type="number"
                  min={1}
                  max={90}
                  value={form.lookback_days}
                  onChange={(e) => set('lookback_days', Number(e.target.value))}
                />
                <Hint>
                  ดึงอีเมลย้อนหลังกี่วัน — แนะนำ 30 (ตั้งสูงเกินไป Gmail จะช้า)
                </Hint>
              </div>
              <div className="space-y-1">
                <Label htmlFor="ac-interval">ดึงทุกกี่นาที</Label>
                <Input
                  id="ac-interval"
                  type="number"
                  min={5}
                  value={form.poll_interval_minutes}
                  onChange={(e) => set('poll_interval_minutes', Number(e.target.value))}
                />
                <Hint>
                  {form.poll_interval_minutes < 5 ? (
                    <span className="text-destructive">
                      ขั้นต่ำ 5 นาที
                    </span>
                  ) : (
                    'แนะนำ 5 นาที'
                  )}
                </Hint>
              </div>
              <div className="space-y-1">
                <Label>เปิดใช้งาน</Label>
                <div className="flex h-9 items-center">
                  <Switch
                    checked={form.enabled}
                    onCheckedChange={(c) => set('enabled', c)}
                  />
                  <span className="ml-2 text-sm text-muted-foreground">
                    {form.enabled ? 'ใช้งาน' : 'ปิดชั่วคราว'}
                  </span>
                </div>
                <Hint>ปิดเพื่อหยุดดึงชั่วคราวโดยไม่ลบกล่องเมล</Hint>
              </div>
            </div>
          </details>

          {/* ─── Test result ─── */}
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

        <DialogFooter className="gap-2 border-t pt-3 sm:justify-between">
          <Button variant="outline" onClick={handleTest} disabled={testing || saving}>
            {testing ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Check className="h-4 w-4" />
            )}
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
