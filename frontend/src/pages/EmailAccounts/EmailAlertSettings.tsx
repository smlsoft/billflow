import { useCallback, useEffect, useRef, useState } from 'react'
import { Bell, RefreshCw, X } from 'lucide-react'
import { toast } from 'sonner'
import client from '@/api/client'
import { useAuth } from '@/hooks/useAuth'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { newShippingCheckRequestID, shippingError } from '@/lib/shipping-checks'

interface AlertConfig {
  enabled: boolean
  host: string
  port: number
  tls_mode: string
  username: string
  from: string
  password?: string
  has_password: boolean
  recipients: string[]
  kinds: string[]
  sources: string[]
  account_ids: string[]
  mode: string
  daily_hour: number
  base_url: string
  revision: string
}
interface AlertAttempt {
  sender?: string
  id: string
  recipient: string
  item_count: number
  is_test: boolean
  status: string
  error: string
  created_at: string
  retryable: boolean
  retry_pending: boolean
}
interface AlertHistory {
  data: AlertAttempt[]
  has_more: boolean
  queued: number
}
interface Inbox {
  id: string
  name: string
  username: string
  channel: string
}
const kinds = [
  ['shipping_mismatch', 'ข้อมูลจัดส่งไม่ตรง'],
  ['shipping_unverifiable', 'ตรวจจัดส่งไม่ได้'],
  ['email_incomplete', 'อ่านอีเมลไม่ครบ'],
  ['sml_failed', 'ส่ง SML ล้มเหลว'],
]
const statusLabels: Record<string, string> = {
  sending: 'กำลังส่ง',
  sent: 'SMTP รับข้อความแล้ว',
  failed: 'ส่งไม่สำเร็จ',
  uncertain: 'ยืนยันผลไม่ได้',
  cancelled: 'ยกเลิกก่อนส่ง',
}
const endpoint = '/api/settings/email-alerts'
const defaultAlertURL = import.meta.env.VITE_EMAIL_ALERT_DEFAULT_URL || ''

export function EmailAlertSettings({ accounts }: { accounts: Inbox[] }) {
  const { user } = useAuth()
  const admin = user?.role === 'admin'
  const [open, setOpen] = useState(false)
  const [provider, setProvider] = useState('gmail')
  const [config, setConfig] = useState<AlertConfig | null>(null)
  const [error, setError] = useState('')
  const [dirty, setDirty] = useState(false)
  const [busy, setBusy] = useState(false)
  const [recipientInput, setRecipientInput] = useState('')
  const [sendToSender, setSendToSender] = useState(true)
  const [history, setHistory] = useState<AlertHistory | null>(null)
  const [historyError, setHistoryError] = useState('')
  const [page, setPage] = useState(1)
  const [retry, setRetry] = useState<AlertAttempt | null>(null)
  const historyRequest = useRef(0)
  const testRequest = useRef<string | null>(null)
  const load = useCallback(async () => {
    setError('')
    try {
      const { data } = await client.get<AlertConfig>(endpoint)
      setConfig({
        ...data,
        password: '',
        base_url: data.base_url || defaultAlertURL,
      })
      setSendToSender(true)
      setProvider(data.host === 'smtp.gmail.com' ? 'gmail' : 'smtp')
      setDirty(false)
    } catch (e) {
      setError(shippingError(e))
    }
  }, [])
  const loadHistory = useCallback(async () => {
    const request = ++historyRequest.current
    try {
      const { data } = await client.get<AlertHistory>(
        `${endpoint}/history?page=${page}`,
      )
      if (historyRequest.current === request) {
        setHistory(data)
        setHistoryError('')
      }
    } catch (e) {
      if (historyRequest.current === request) setHistoryError(shippingError(e))
    }
  }, [page])
  useEffect(() => {
    if (admin) void load()
  }, [admin, load])
  useEffect(() => {
    if (!open || !admin) return
    void loadHistory()
    const timer = window.setInterval(() => void loadHistory(), 15_000)
    return () => {
      window.clearInterval(timer)
      historyRequest.current++
    }
  }, [open, admin, loadHistory])
  const update = (value: Partial<AlertConfig>) => {
    setConfig((c) => (c ? { ...c, ...value } : c))
    setDirty(true)
    setError('')
  }
  const toggle = (
    field: 'kinds' | 'sources' | 'account_ids',
    value: string,
  ) => {
    if (!config) return
    update({
      [field]: config[field].includes(value)
        ? config[field].filter((v) => v !== value)
        : [...config[field], value],
    })
  }
  const addRecipients = () => {
    if (!config) return
    const incoming = recipientInput
      .split(/[,;\n]+/)
      .map((v) => v.trim().toLowerCase())
      .filter(Boolean)
    const values = [...new Set([...config.recipients, ...incoming])]
    if (values.length > 20) {
      setError('เพิ่มผู้รับได้ไม่เกิน 20 คน')
      return
    }
    update({ recipients: values })
    setRecipientInput('')
  }
  const canDefaultRecipient =
    !!defaultAlertURL &&
    !!config &&
    !config.revision &&
    provider === 'gmail' &&
    config.recipients.length === 0
  const senderRecipient =
    canDefaultRecipient && sendToSender ? config?.username.trim() : undefined
  const save = async () => {
    if (!config) return
    if (recipientInput.trim()) {
      setError('กดเพิ่มผู้รับก่อนบันทึก')
      return
    }
    setBusy(true)
    setError('')
    try {
      const payload = {
        ...config,
        recipients: senderRecipient ? [senderRecipient] : config.recipients,
        password: config.password || undefined,
      }
      const { data } = await client.put<AlertConfig>(endpoint, payload)
      setConfig({ ...data, password: '' })
      setProvider(data.host === 'smtp.gmail.com' ? 'gmail' : 'smtp')
      setDirty(false)
      toast.success('บันทึกการแจ้งเตือนอีเมลแล้ว')
    } catch (e) {
      setError(shippingError(e))
    } finally {
      setBusy(false)
    }
  }
  const test = async () => {
    setBusy(true)
    setError('')
    try {
      testRequest.current ||= newShippingCheckRequestID()
      await client.post(`${endpoint}/test`, { request_id: testRequest.current })
      testRequest.current = null
      toast.success(
        'เข้าคิวอีเมลทดสอบแล้ว ระบบเริ่มส่งตามรอบประมาณ 1 นาที โปรดตรวจประวัติ/กล่องจดหมาย',
      )
      await loadHistory()
    } catch (e) {
      setError(shippingError(e))
    } finally {
      setBusy(false)
    }
  }
  if (!admin) return null
  return (
    <>
      <Card className="flex flex-wrap items-center justify-between gap-3 p-4">
        <div>
          <h2 className="flex items-center gap-2 font-medium">
            <Bell className="h-4 w-4" />
            แจ้งเตือนทางอีเมล{' '}
            <Badge variant="outline">
              {dirty
                ? 'มีค่าที่ยังไม่บันทึก'
                : config
                  ? config.enabled
                    ? 'เปิดใช้งาน'
                    : 'ปิดใช้งาน'
                  : 'ยังไม่ได้โหลด'}
            </Badge>
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            แจ้งผู้รับผิดชอบเมื่อพบปัญหา หรือสรุปงานค้างรายวัน · ตั้งค่าได้เฉพาะ
            Admin
          </p>
          {!open && error && (
            <p role="alert" className="mt-1 text-sm text-destructive">
              {error}
            </p>
          )}
        </div>
        <Button
          variant="outline"
          onClick={() => {
            setOpen(true)
            if (!config) void load()
          }}
        >
          ตั้งค่าการแจ้งเตือน
        </Button>
      </Card>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-h-[90vh] max-w-4xl overflow-y-auto">
          <DialogHeader>
            <DialogTitle>แจ้งเตือนความผิดปกติทางอีเมล</DialogTitle>
            <DialogDescription>
              ตั้งค่าบัญชีส่ง ผู้รับ และประเภทปัญหา แยกจากกล่องอีเมลรับบิล
            </DialogDescription>
          </DialogHeader>
          {!config ? (
            <div role="status">
              {error || 'กำลังโหลด…'}
              <Button variant="outline" onClick={() => void load()}>
                ลองโหลดใหม่
              </Button>
            </div>
          ) : (
            <div className="space-y-5">
              <section
                className="space-y-3 rounded-lg border p-3"
                aria-label="บัญชีส่งอีเมล"
              >
                <h3 className="font-medium">บัญชีส่งอีเมล (SMTP)</h3>
                <Field label="ผู้ให้บริการบัญชีส่ง" id="alert-provider">
                  <Select
                    value={provider}
                    onValueChange={(value) => {
                      setProvider(value)
                      update({
                        host: value === 'gmail' ? 'smtp.gmail.com' : '',
                        port: 587,
                        tls_mode: 'starttls',
                        from: config.username,
                        password: '',
                      })
                    }}
                  >
                    <SelectTrigger id="alert-provider">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="gmail">
                        Gmail / Google Workspace · App Password
                      </SelectItem>
                      <SelectItem value="smtp">
                        SMTP ของบริษัท / ผู้ให้บริการอื่น
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </Field>
                {provider === 'gmail' && (
                  <p className="text-xs text-muted-foreground">
                    กรอกอีเมลที่ใช้ส่งและ App Password 16
                    ตัวเหมือนการตั้งกล่องรับบิล ระบบเตรียมค่า Gmail ให้แล้ว
                    บัญชีต้องเปิดยืนยัน 2 ขั้นตอนและอนุญาต App Password ·{' '}
                    <a
                      className="text-primary underline"
                      href="https://support.google.com/accounts/answer/185833?hl=th"
                      target="_blank"
                      rel="noreferrer"
                    >
                      วิธีสร้าง App Password
                    </a>
                  </p>
                )}
                <div className="grid gap-3 sm:grid-cols-2">
                  {provider === 'smtp' && (
                    <>
                      <Field label="SMTP host" id="alert-host">
                        <Input
                          id="alert-host"
                          value={config.host}
                          placeholder="smtp.gmail.com"
                          onChange={(e) => update({ host: e.target.value })}
                        />
                      </Field>
                      <Field label="การเชื่อมต่อ" id="alert-tls">
                        <Select
                          value={config.tls_mode}
                          onValueChange={(v) =>
                            update({
                              tls_mode: v,
                              port: v === 'tls' ? 465 : 587,
                            })
                          }
                        >
                          <SelectTrigger id="alert-tls">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="starttls">
                              STARTTLS · พอร์ต 587
                            </SelectItem>
                            <SelectItem value="tls">TLS · พอร์ต 465</SelectItem>
                          </SelectContent>
                        </Select>
                      </Field>
                    </>
                  )}
                  <Field
                    label={
                      provider === 'gmail'
                        ? 'อีเมลที่ใช้ส่ง (Gmail / Google Workspace)'
                        : 'บัญชี SMTP'
                    }
                    id="alert-username"
                  >
                    <Input
                      id="alert-username"
                      autoComplete="off"
                      value={config.username}
                      onChange={(e) =>
                        update(
                          provider === 'gmail'
                            ? { username: e.target.value, from: e.target.value }
                            : { username: e.target.value },
                        )
                      }
                    />
                  </Field>
                  <Field
                    label={
                      provider === 'gmail'
                        ? 'App Password 16 ตัว'
                        : 'รหัสผ่าน SMTP / App Password'
                    }
                    id="alert-password"
                  >
                    <Input
                      id="alert-password"
                      type="password"
                      autoComplete="new-password"
                      value={config.password || ''}
                      placeholder={
                        config.has_password
                          ? 'มีรหัสผ่านแล้ว เว้นว่างเพื่อใช้ค่าเดิม'
                          : 'ยังไม่ได้ตั้งรหัสผ่าน'
                      }
                      onChange={(e) => update({ password: e.target.value })}
                    />
                  </Field>
                  {provider === 'smtp' && (
                    <Field label="อีเมลผู้ส่ง" id="alert-from">
                      <Input
                        id="alert-from"
                        type="email"
                        value={config.from}
                        onChange={(e) => update({ from: e.target.value })}
                      />
                    </Field>
                  )}
                  <Field
                    label="URL หน้า BillFlow สำหรับลิงก์ในอีเมล"
                    id="alert-url"
                  >
                    <Input
                      id="alert-url"
                      value={config.base_url}
                      placeholder="http://ชื่อเซิร์ฟเวอร์:9985"
                      onChange={(e) => update({ base_url: e.target.value })}
                    />
                  </Field>
                </div>
                <p className="text-xs text-muted-foreground">
                  กรอกรหัสผ่านผ่าน HTTPS หรือหน้า localhost บนเซิร์ฟเวอร์
                  ใช้บัญชีส่งของบริษัทและ TLS ที่ตรวจใบรับรองได้ ต้องขอ SMTP/App
                  Password จากผู้ดูแลอีเมลก่อน
                  ไม่ใช้รหัสผ่านกล่องรับบิลโดยอัตโนมัติ
                </p>
              </section>
              <section className="space-y-3" aria-label="ผู้รับแจ้งเตือน">
                <h3 className="font-medium">ผู้รับแจ้งเตือน (สูงสุด 20 คน)</h3>
                {canDefaultRecipient && (
                  <label className="flex items-center gap-2 text-sm">
                    <Checkbox
                      checked={sendToSender}
                      onCheckedChange={(checked) => {
                        setSendToSender(checked === true)
                        setDirty(true)
                      }}
                    />
                    ใช้ Gmail ที่กรอกเป็นผู้รับเริ่มต้น
                  </label>
                )}
                <div className="flex flex-wrap gap-2">
                  <Input
                    className="min-w-0 flex-1"
                    aria-label="เพิ่มอีเมลผู้รับ"
                    placeholder="name@company.com คั่นหลายคนด้วยจุลภาค"
                    value={recipientInput}
                    onChange={(e) => setRecipientInput(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault()
                        addRecipients()
                      }
                    }}
                  />
                  <Button variant="outline" onClick={addRecipients}>
                    เพิ่มผู้รับ
                  </Button>
                </div>
                <div className="flex flex-wrap gap-2">
                  {config.recipients.map((v) => (
                    <Button
                      key={v}
                      size="sm"
                      variant="secondary"
                      className="max-w-full"
                      aria-label={`ลบผู้รับ ${v}`}
                      onClick={() =>
                        update({
                          recipients: config.recipients.filter((x) => x !== v),
                        })
                      }
                    >
                      <span className="truncate">{v}</span>
                      <X className="h-3 w-3" />
                    </Button>
                  ))}
                </div>
                <p className="text-xs text-muted-foreground">
                  การนำผู้รับออกมีผลต่อคิวที่ยังไม่ส่ง
                  ไม่สามารถเรียกคืนอีเมลที่ส่งแล้วหรือกำลังส่งได้
                </p>
                {!config.recipients.length && !senderRecipient && (
                  <p className="text-sm text-muted-foreground">
                    ยังไม่มีผู้รับ จึงยังเปิดส่งอัตโนมัติไม่ได้
                  </p>
                )}
              </section>
              <section className="space-y-3" aria-label="เงื่อนไขแจ้งเตือน">
                <h3 className="font-medium">แจ้งเตือนเรื่องใด</h3>
                <div className="grid gap-2 sm:grid-cols-2">
                  {kinds.map(([value, label]) => (
                    <Choice
                      key={value}
                      label={label}
                      checked={config.kinds.includes(value)}
                      onChange={() => toggle('kinds', value)}
                    />
                  ))}
                </div>
                <h4 className="text-sm font-medium">
                  ช่องทาง (ไม่เลือก = ทุกช่องทางที่เกี่ยวข้อง)
                </h4>
                <div className="flex flex-wrap gap-4">
                  {[
                    ['shopee_shipped', 'Email บิลซื้อ Shopee'],
                    ['lazada_email', 'Email บิลซื้อ Lazada'],
                  ].map(([value, label]) => (
                    <Choice
                      key={value}
                      label={label}
                      checked={config.sources.includes(value)}
                      onChange={() => toggle('sources', value)}
                    />
                  ))}
                </div>
                <h4 className="text-sm font-medium">
                  กล่องอีเมล (ไม่เลือก = ทุกกล่อง)
                </h4>
                <div className="max-h-40 space-y-2 overflow-y-auto rounded-md border p-3">
                  {accounts
                    .filter((a) => ['shopee', 'lazada'].includes(a.channel))
                    .map((a) => (
                      <Choice
                        key={a.id}
                        label={`${a.name} · ${a.channel}`}
                        checked={config.account_ids.includes(a.id)}
                        onChange={() => toggle('account_ids', a.id)}
                      />
                    ))}
                </div>
                <p className="text-xs text-muted-foreground">
                  หากเลือกทั้งช่องทางและกล่อง รายการต้องตรงทั้งสองเงื่อนไข
                  การส่ง SML
                  ล้มเหลวของช่องทางอื่นจะไม่รวมเมื่อจำกัดเฉพาะกล่องอีเมล
                </p>
                <div className="grid gap-3 sm:grid-cols-2">
                  <Field label="รูปแบบแจ้งเตือน" id="alert-mode">
                    <Select
                      value={config.mode}
                      onValueChange={(mode) => update({ mode })}
                    >
                      <SelectTrigger id="alert-mode">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="immediate">
                          ปัญหาใหม่ · รวมเป็นรอบประมาณทุก 1 นาที
                        </SelectItem>
                        <SelectItem value="daily">
                          สรุปปัญหาที่ยังค้างรายวัน
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </Field>
                  {config.mode === 'daily' && (
                    <Field label="เวลาสรุป (เวลาไทย)" id="alert-hour">
                      <Select
                        value={String(config.daily_hour)}
                        onValueChange={(v) => update({ daily_hour: Number(v) })}
                      >
                        <SelectTrigger id="alert-hour">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {Array.from({ length: 24 }, (_, i) => (
                            <SelectItem key={i} value={String(i)}>
                              {String(i).padStart(2, '0')}:00
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </Field>
                  )}
                </div>
              </section>
              <div className="flex items-start gap-3 rounded-md border p-3">
                <Switch
                  id="alert-enabled"
                  checked={config.enabled}
                  onCheckedChange={(enabled) => update({ enabled })}
                />
                <div>
                  <Label htmlFor="alert-enabled">
                    เปิดส่งแจ้งเตือนอัตโนมัติ
                  </Label>
                  <p className="mt-1 text-xs text-muted-foreground">
                    เมื่อเปิดจะรวมปัญหาที่ยังค้างตามเงื่อนไขด้วย
                    ปัญหาเดิมไม่ส่งซ้ำทุกครั้งที่กดตรวจ
                    หากระบบตรวจพบว่าปิดปัญหาแล้วกลับมาผิดปกติจะแจ้งใหม่
                    รายวันส่งวันละครั้งเมื่อมีงานค้าง
                  </p>
                </div>
              </div>
              <p className="text-xs text-muted-foreground">
                ตรวจพบเมื่อ BillFlow ได้รับและประมวลผลข้อมูลแล้ว
                ไม่ได้ครอบคลุมทุกปัญหา และหากเซิร์ฟเวอร์หยุดทำงาน
                ระบบนี้ส่งแจ้งเตือนไม่ได้
              </p>
              {error && (
                <p role="alert" className="text-sm text-destructive">
                  {error}
                </p>
              )}
              <div className="flex flex-wrap items-center gap-2">
                <Button disabled={busy} onClick={() => void save()}>
                  {busy ? 'กำลังดำเนินการ…' : 'บันทึกการแจ้งเตือน'}
                </Button>
                <Button
                  variant="outline"
                  disabled={
                    busy || dirty || !config.revision || !config.has_password
                  }
                  onClick={() => void test()}
                >
                  ส่งอีเมลทดสอบ
                </Button>
                <Button
                  variant="ghost"
                  disabled={busy}
                  onClick={() => void load()}
                >
                  โหลดค่าล่าสุด
                </Button>
                {dirty && (
                  <span className="text-xs text-muted-foreground">
                    มีค่าที่ยังไม่บันทึก · บันทึกก่อนส่งทดสอบ
                  </span>
                )}
              </div>
            </div>
          )}
          <section
            className="mt-3 space-y-3 border-t pt-4"
            aria-label="ประวัติอีเมลแจ้งเตือน"
          >
            <div className="flex flex-wrap items-center justify-between gap-2">
              <h3 className="font-medium">
                ประวัติการส่ง{' '}
                {history ? `· คิวปัญหารอส่ง ${history.queued}` : ''}
              </h3>
              <Button
                size="sm"
                variant="outline"
                onClick={() => void loadHistory()}
              >
                <RefreshCw className="h-3.5 w-3.5" />
                รีเฟรชประวัติ
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              “SMTP รับข้อความแล้ว” ไม่ได้ยืนยันว่าเข้ากล่องหลักหรืออ่านแล้ว
              การลองใหม่กรณียืนยันผลไม่ได้อาจทำให้ผู้รับได้อีเมลซ้ำ
            </p>
            {historyError ? (
              <p role="alert" className="text-sm text-destructive">
                {historyError}
              </p>
            ) : !history ? (
              <p>กำลังโหลดประวัติ…</p>
            ) : !history.data.length ? (
              <p className="text-sm text-muted-foreground">
                ยังไม่มีประวัติ ลองส่งอีเมลทดสอบหลังบันทึกบัญชีส่ง
              </p>
            ) : (
              history.data.map((item) => (
                <div
                  key={item.id}
                  className="space-y-1 rounded-md border p-3 text-sm"
                >
                  <div className="flex flex-wrap justify-between gap-2">
                    <span className="break-all font-medium">
                      {item.recipient}
                    </span>
                    <Badge variant="outline">
                      {statusLabels[item.status] || item.status}
                      {item.retry_pending ? ' · รอลองใหม่อัตโนมัติ' : ''}
                    </Badge>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {new Date(item.created_at).toLocaleString('th-TH')} ·{' '}
                    {item.is_test ? 'ทดสอบ' : `${item.item_count} ปัญหา`}
                    {item.sender ? ` · ส่งจาก ${item.sender}` : ''}
                  </p>
                  {item.error && <p className="text-xs">{item.error}</p>}
                  {item.retryable && (
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => setRetry(item)}
                    >
                      ลองส่งใหม่
                    </Button>
                  )}
                </div>
              ))
            )}
            <div className="flex items-center justify-end gap-2">
              <Button
                size="sm"
                variant="outline"
                disabled={page === 1}
                onClick={() => setPage((p) => p - 1)}
              >
                ก่อนหน้า
              </Button>
              <span className="text-xs">หน้า {page}</span>
              <Button
                size="sm"
                variant="outline"
                disabled={!history?.has_more}
                onClick={() => setPage((p) => p + 1)}
              >
                ถัดไป
              </Button>
            </div>
          </section>
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={retry !== null}
        onOpenChange={(v) => {
          if (!v) setRetry(null)
        }}
        title="ลองส่งอีเมลแจ้งเตือนใหม่"
        description={
          retry?.status === 'uncertain'
            ? 'กรุณาตรวจกล่องจดหมายผู้รับก่อน ระบบไม่ทราบว่าครั้งก่อนส่งถึงแล้วหรือไม่ การลองใหม่อาจทำให้ผู้รับได้รับซ้ำ'
            : 'ระบบจะตรวจเงื่อนไขและปัญหาปัจจุบันอีกครั้งก่อนส่ง หากแก้ไขแล้วหรือผู้รับถูกนำออก จะยกเลิกก่อนส่ง'
        }
        confirmLabel="ยืนยันลองใหม่"
        onConfirm={async () => {
          if (!retry) return
          try {
            await client.post(`${endpoint}/history/${retry.id}/retry`, {
              confirm_duplicate_risk: retry.status === 'uncertain',
            })
            toast.success('เข้าคิวลองส่งใหม่แล้ว')
            await loadHistory()
          } catch (e) {
            setError(shippingError(e))
          }
        }}
      />
    </>
  )
}
function Field({
  label,
  id,
  children,
}: {
  label: string
  id: string
  children: React.ReactNode
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      {children}
    </div>
  )
}
function Choice({
  label,
  checked,
  onChange,
}: {
  label: string
  checked: boolean
  onChange: () => void
}) {
  return (
    <label className="flex items-center gap-2 text-sm">
      <Checkbox checked={checked} onCheckedChange={onChange} />
      <span>{label}</span>
    </label>
  )
}
