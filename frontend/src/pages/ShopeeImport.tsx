import { useState, useRef, useEffect, Fragment, type ComponentType } from 'react'
import { Link } from 'react-router-dom'
import {
  AlertCircle,
  AlertTriangle,
  ArrowRight,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Clock3,
  Database,
  FileSpreadsheet,
  Info,
  Loader2,
  Pencil,
  PlugZap,
  Power,
  RefreshCw,
  Save,
  ShieldCheck,
  Store,
  X,
} from 'lucide-react'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { PageHeader } from '@/components/common/PageHeader'
import client from '@/api/client'
import { cn } from '@/lib/utils'

interface ShopeeConfig {
  server_url: string
  guid: string
  provider: string
  config_file_name: string
  database_name: string
  doc_format_code: string
  endpoint?: string
  cust_code: string
  sale_code: string
  branch_code: string
  wh_code: string
  shelf_code: string
  unit_code: string
  vat_type: number
  vat_rate: number
  doc_time: string
}

interface ShopeeOrderItem {
  sku: string
  product_name: string
  option_name?: string
  raw_name: string
  price: number
  qty: number
  no_sku?: boolean
}
interface ShopeeOrder {
  order_id: string
  doc_date: string
  order_datetime?: string
  payment_time?: string
  payment_channel?: string
  buyer_username?: string
  tracking_no?: string
  status: string
  items: ShopeeOrderItem[]
  item_count: number
  total_qty: number
  paid_amount?: number
  order_total_amount?: number
  item_gross_amount?: number
  line_paid_amount?: number
  shipping_amount?: number
  discount_amount?: number
  no_sku_item_count?: number
  has_no_sku?: boolean
  multi_line?: boolean
  amount_mismatch?: boolean
  existing_bill_id?: string
  shopee_shop_id?: string
  shopee_connection_id?: string
  shopee_shop_label?: string
  duplicate: boolean
}
interface ImportPreflight {
  new_orders: number
  duplicate_orders: number
  skipped_rows: number
  no_sku_orders: number
  no_sku_items: number
  multi_item_orders: number
  amount_mismatch_orders: number
}
interface PreviewResponse {
  orders: ShopeeOrder[]
  warnings: string[]
  total_orders: number
  new_count: number
  duplicate_count: number
  skipped_count: number
  import_run_id?: string
  preflight: ImportPreflight
  file_token?: string
  more?: boolean
  next_cursor?: string
}
interface ConfirmResult {
  order_id: string
  success: boolean
  bill_id?: string
  doc_no?: string
  message?: string
}
interface ImportRunSummary {
  id: string
  filename: string
  period_start?: string
  period_end?: string
  total_orders: number
  new_orders: number
  duplicate_orders: number
  skipped_orders: number
  warning_count: number
  created_count: number
  failed_count: number
  status: 'preview' | 'confirmed' | 'failed'
  created_at: string
  confirmed_at?: string
}

type ShopeeAPIReadinessCheckStatus = 'ok' | 'warning' | 'blocked'

interface ShopeeAPIReadinessCheck {
  key: string
  label: string
  status: ShopeeAPIReadinessCheckStatus
  detail?: string
}

interface ShopeeAPIStatus {
  enabled: boolean
  configured: boolean
  environment: string
  base_url?: string
  partner_id?: number
  redirect_url?: string
  connected: boolean
  shop_id?: number
  shop_name?: string
  access_expires_at?: string
  refresh_expires_at?: string
  last_sync_at?: string
  last_sync_status?: string
  last_sync_error?: string
  token_state?: string
  can_connect?: boolean
  can_fetch?: boolean
  blocking_reason?: string
  checks?: ShopeeAPIReadinessCheck[]
}

interface ShopeeAPIConnection {
  id: string
  shop_id: number
  merchant_id?: number
  shop_name?: string
  label: string
  environment: string
  access_expires_at: string
  refresh_expires_at: string
  disabled_at?: string
  last_sync_at?: string
  last_sync_status?: string
  last_sync_error?: string
  last_error_code?: string
  token_state: string
  can_fetch: boolean
  connected_at?: string
  updated_at?: string
}

function fmt(n: number) {
  return n.toLocaleString('th-TH', { minimumFractionDigits: 2 })
}

function fmtDateTime(s: string) {
  if (!s) return '—'
  return new Date(s).toLocaleString('th-TH', {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function shopeeDestination(config?: ShopeeConfig | null) {
  const docFormat = (config?.doc_format_code ?? '').trim().toUpperCase()
  const endpoint = (config?.endpoint ?? '').toLowerCase()
  const isSaleInvoice = endpoint.includes('saleinvoice') || docFormat === 'SI'
  return isSaleInvoice
    ? {
        documentName: 'เอกสารขายสินค้าและบริการ',
        shortName: 'ขายสินค้าและบริการ',
        smlPath: 'ขาย -> ขายสินค้าและบริการ',
        action: 'สร้างเอกสารขายสินค้าและบริการ',
        done: 'สร้างเอกสารขายสินค้าและบริการแล้ว',
        listPath: '/sale-invoices',
        listName: 'ขายสินค้าและบริการ',
      }
    : {
        documentName: 'ใบสั่งขาย',
        shortName: 'ใบสั่งขาย',
        smlPath: 'ขาย -> ใบสั่งขาย',
        action: 'สร้างใบสั่งขาย',
        done: 'สร้างใบสั่งขายแล้ว',
        listPath: '/sales-orders',
        listName: 'ใบสั่งขาย',
      }
}

type APIReadinessTone = 'success' | 'warning' | 'danger' | 'muted'

interface APIReadinessStep {
  label: string
  done: boolean
  status?: ShopeeAPIReadinessCheckStatus
  detail?: string
}

interface APIReadiness {
  title: string
  description: string
  tone: APIReadinessTone
  steps: APIReadinessStep[]
}

function isLiveAPI(status: ShopeeAPIStatus) {
  return (status.environment || '').toLowerCase() === 'live'
}

function shopeeAPIReadiness(status: ShopeeAPIStatus): APIReadiness {
  const live = isLiveAPI(status)
  const steps: APIReadinessStep[] =
    status.checks && status.checks.length > 0
      ? status.checks.map((check) => ({
          label: check.label,
          done: check.status === 'ok',
          status: check.status,
          detail: check.detail,
        }))
      : [
          { label: 'เปิด Shopee Open API บน server', done: status.enabled },
          { label: 'ตั้งค่า Partner ID / Key', done: status.configured },
          { label: 'ใช้ Live key หลัง Shopee approve', done: live },
          { label: 'เชื่อมร้านผ่าน OAuth', done: status.connected },
        ]
  const hasBlocked = steps.some((s) => s.status === 'blocked')
  const hasWarning = steps.some((s) => s.status === 'warning' || !s.done)

  if (!status.enabled) {
    return {
      title: 'Shopee Open API ยังปิดอยู่',
      description: 'เปิด SHOPEE_OPEN_API_ENABLED=true ก่อนเริ่มเชื่อมร้าน',
      tone: 'danger',
      steps,
    }
  }
  if (hasBlocked) {
    return {
      title: 'Shopee API ยังไม่พร้อมใช้งาน',
      description: status.blocking_reason || 'มีรายการ preflight ที่ต้องแก้ก่อนเชื่อมต่อหรือดึง order',
      tone: 'danger',
      steps,
    }
  }
  if (!status.configured) {
    return {
      title: 'ยังไม่ได้ตั้งค่า key บน server',
      description: 'ต้องใส่ Partner ID และ Partner Key ให้ครบก่อนสร้างลิงก์ OAuth',
      tone: 'warning',
      steps,
    }
  }
  if (!live && !status.connected) {
    return {
      title: 'พร้อมระดับ sandbox แต่ยังไม่พร้อมร้านจริง',
      description: 'ตอนนี้ใช้ test key อยู่ ร้านจริงต้องรอ Shopee approve Go-Live แล้วเปลี่ยนเป็น live key ก่อนเชื่อม',
      tone: 'warning',
      steps,
    }
  }
  if (live && !status.connected) {
    return {
      title: 'พร้อมเชื่อมร้านจริง',
      description: 'ตรวจว่า Redirect URL Domain ใน Shopee Console ตรงกับ public URL ปัจจุบัน แล้วกดเชื่อมต่อ Shopee API',
      tone: 'success',
      steps,
    }
  }
  if (hasWarning) {
    return {
      title: live ? 'พร้อมใช้งานแต่มีข้อควรตรวจ' : 'พร้อมระดับ sandbox แต่ยังไม่ใช่ live',
      description: status.blocking_reason || 'ตรวจ warning ก่อนใช้งานจริง โดยเฉพาะ token, redirect และ sync ล่าสุด',
      tone: 'warning',
      steps,
    }
  }
  return {
    title: live ? 'เชื่อมร้านจริงแล้ว' : 'เชื่อม sandbox แล้ว',
    description: 'ดึง order แบบ preview-only ก่อนสร้างบิล เพื่อให้ตรวจข้อมูลก่อนส่งเข้า SML',
    tone: live ? 'success' : 'warning',
    steps,
  }
}

function readinessToneClass(tone: APIReadinessTone) {
  if (tone === 'success') return 'border-success/30 bg-success/5 text-success'
  if (tone === 'danger') return 'border-destructive/30 bg-destructive/5 text-destructive'
  if (tone === 'warning') return 'border-warning/35 bg-warning/10 text-warning'
  return 'border-border bg-muted/30 text-muted-foreground'
}

function readinessStepIcon(step: APIReadinessStep) {
  if (step.status === 'blocked') return <AlertCircle className="h-3.5 w-3.5 shrink-0 text-destructive" />
  if (step.status === 'warning' || !step.done) return <Clock3 className="h-3.5 w-3.5 shrink-0 text-warning" />
  return <CheckCircle2 className="h-3.5 w-3.5 shrink-0 text-success" />
}

function tokenStateLabel(v?: string) {
  switch (v) {
    case 'access_valid':
      return 'พร้อมใช้'
    case 'access_expiring':
      return 'ใกล้ refresh'
    case 'refresh_required':
      return 'ต้อง refresh'
    case 'refresh_expired':
      return 'หมดอายุ'
    default:
      return '—'
  }
}

function apiRangeError(from: string, to: string) {
  if (!from || !to) return 'เลือกวันที่เริ่มต้นและสิ้นสุดให้ครบ'
  const fromDate = new Date(`${from}T00:00:00`)
  const toDate = new Date(`${to}T00:00:00`)
  if (Number.isNaN(fromDate.getTime()) || Number.isNaN(toDate.getTime())) {
    return 'รูปแบบวันที่ไม่ถูกต้อง'
  }
  if (toDate < fromDate) return 'วันที่สิ้นสุดต้องไม่ก่อนวันที่เริ่มต้น'
  const days = Math.floor((toDate.getTime() - fromDate.getTime()) / 86400000) + 1
  if (days > 15) return 'Shopee API จำกัดการดึงข้อมูลไม่เกิน 15 วันต่อครั้ง'
  return ''
}

function apiErrorMessage(err: unknown, fallback: string) {
  const data = (err as { response?: { data?: { error?: string; error_code?: string } } })?.response?.data
  const raw = data?.error ?? ''
  const lower = raw.toLowerCase()
  switch (data?.error_code) {
    case 'not_configured':
      return 'Shopee Open API ยังไม่ได้ตั้งค่า Partner ID/Key บน server'
    case 'redirect_not_ready':
      return 'Redirect URL ยังไม่พร้อม ให้ตรวจ PUBLIC_BASE_URL และ Shopee Console ว่าตรงกัน'
    case 'not_connected':
      return 'ยังไม่ได้เชื่อมต่อร้าน Shopee ให้รอ Go-Live approve แล้วกดเชื่อมต่อ API'
    case 'bad_signature':
      return 'Shopee ปฏิเสธ signature ให้ตรวจ Partner ID/Key และ sandbox/live base URL'
    case 'token_error':
      return 'Shopee token ใช้งานไม่ได้หรือหมดอายุ ให้กดเชื่อมต่อร้านใหม่'
    case 'permission_denied':
      return 'Shopee ยังไม่อนุญาตสิทธิ์นี้ ให้ตรวจสถานะ Go-Live และ permission ของแอป'
    case 'rate_limited':
      return 'Shopee rate limit ให้รอสักครู่แล้วลองใหม่'
    case 'network_timeout':
      return 'เชื่อมต่อ Shopee ชั่วคราวไม่สำเร็จ ให้ลองใหม่อีกครั้ง'
  }
  if (lower.includes('wrong sign') || lower.includes('error_sign')) {
    return 'Shopee ปฏิเสธ signature ให้ตรวจ Partner ID/Key และ sandbox/live base URL'
  }
  if (lower.includes('redirect') && lower.includes('domain')) {
    return 'Shopee ปฏิเสธ Redirect URL Domain ให้ตรวจ public URL ใน Console ให้ตรงกับ server'
  }
  if (lower.includes('ยังไม่ได้เชื่อมต่อร้าน')) {
    return 'ยังไม่ได้เชื่อมต่อร้าน Shopee ให้กดเชื่อมต่อ API ก่อนดึง order'
  }
  if (lower.includes('token')) {
    return 'Shopee token ใช้งานไม่ได้หรือหมดอายุ ให้กดเชื่อมต่อร้านใหม่'
  }
  if (lower.includes('permission') || lower.includes('forbidden')) {
    return 'Shopee ยังไม่อนุญาตสิทธิ์นี้ ให้ตรวจสถานะ Go-Live และ permission ของแอป'
  }
  if (lower.includes('rate') || lower.includes('429')) {
    return 'Shopee rate limit ให้รอสักครู่แล้วลองใหม่'
  }
  return raw || fallback
}

function SummaryCard({
  label,
  value,
  variant = 'muted',
}: {
  label: string
  value: number
  variant?: 'success' | 'danger' | 'primary' | 'muted'
}) {
  const tone: Record<typeof variant, string> = {
    success: 'border-success/30 bg-success/5 text-success',
    danger: 'border-destructive/30 bg-destructive/5 text-destructive',
    primary: 'border-primary/30 bg-primary/5 text-primary',
    muted: 'border-border bg-muted/30 text-foreground',
  }
  return (
    <Card className={cn('text-center', tone[variant])}>
      <CardContent className="p-4">
        <p className="text-3xl font-semibold tabular-nums">{value}</p>
        <p className="mt-1 text-xs font-medium text-muted-foreground">{label}</p>
      </CardContent>
    </Card>
  )
}

function StatusTile({
  label,
  value,
  description,
  icon: Icon,
  tone = 'muted',
}: {
  label: string
  value: string
  description: string
  icon: ComponentType<{ className?: string }>
  tone?: 'success' | 'warning' | 'primary' | 'muted'
}) {
  const toneClass = {
    success: 'border-success/25 bg-success/[0.04] text-success',
    warning: 'border-warning/30 bg-warning/[0.08] text-warning',
    primary: 'border-primary/25 bg-primary/[0.04] text-primary',
    muted: 'border-border bg-card text-foreground',
  }[tone]

  return (
    <div className={cn('rounded-lg border px-4 py-3', toneClass)}>
      <div className="flex items-start gap-3">
        <Icon className="mt-0.5 h-4 w-4 shrink-0" />
        <div className="min-w-0">
          <p className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
            {label}
          </p>
          <p className="mt-1 text-sm font-semibold leading-5 text-foreground">{value}</p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">{description}</p>
        </div>
      </div>
    </div>
  )
}

type Step = 'idle' | 'uploading' | 'preview' | 'confirming' | 'done'

export default function ShopeeImport() {
  const fileRef = useRef<HTMLInputElement>(null)
  const [step, setStep] = useState<Step>('idle')
  const [config, setConfig] = useState<ShopeeConfig | null>(null)
  const [preview, setPreview] = useState<PreviewResponse | null>(null)
  const [selectedIDs, setSelectedIDs] = useState<Set<string>>(new Set())
  const [results, setResults] = useState<{
    success_count: number
    fail_count: number
    results: ConfirmResult[]
  } | null>(null)
  const [error, setError] = useState('')
  const [expandedOrders, setExpandedOrders] = useState<Set<string>>(new Set())
  const [recentRuns, setRecentRuns] = useState<ImportRunSummary[]>([])
  const [confirmElapsed, setConfirmElapsed] = useState(0)
  const [previewSource, setPreviewSource] = useState<'excel' | 'api'>('excel')
  const [apiStatus, setAPIStatus] = useState<ShopeeAPIStatus | null>(null)
  const [apiConnections, setAPIConnections] = useState<ShopeeAPIConnection[]>([])
  const [selectedConnectionID, setSelectedConnectionID] = useState('')
  const [editingConnectionID, setEditingConnectionID] = useState('')
  const [editingLabel, setEditingLabel] = useState('')
  const [apiBusy, setAPIBusy] = useState(false)
  const [apiFrom, setAPIFrom] = useState(() => {
    const d = new Date()
    d.setDate(d.getDate() - 7)
    return d.toISOString().slice(0, 10)
  })
  const [apiTo, setAPITo] = useState(() => new Date().toISOString().slice(0, 10))

  // Track config load + ready states separately so preflight UI can render
  // a missing-config banner BEFORE admin uploads a file. Without this, file
  // upload silently succeeds → preview works → confirm fails late with a
  // confusing "config missing" error.
  const [configLoading, setConfigLoading] = useState(true)
  const configReady = !configLoading
  const channelReady = configReady && Boolean(config?.cust_code?.trim())
  const destination = shopeeDestination(config)

  const fallbackConfig: ShopeeConfig = {
    server_url: '',
    guid: '',
    provider: '',
    config_file_name: '',
    database_name: '',
    doc_format_code: 'SR',
    endpoint: '',
    cust_code: '',
    sale_code: '',
    branch_code: '',
    wh_code: '',
    shelf_code: '',
    unit_code: '',
    vat_type: -1,
    vat_rate: -1,
    doc_time: '',
  }

  useEffect(() => {
    let alive = true
    client
      .get<ShopeeConfig>('/api/settings/shopee-config')
      .then((res) => {
        if (alive) setConfig(res.data)
      })
      .catch(() => {
        if (alive) setError('โหลด config ไม่ได้')
      })
      .finally(() => {
        if (alive) setConfigLoading(false)
      })
    client
      .get<{ runs: ImportRunSummary[] }>('/api/import/shopee/runs?limit=5')
      .then((res) => {
        if (alive) setRecentRuns(res.data.runs ?? [])
      })
      .catch(() => undefined)
    client
      .get<ShopeeAPIStatus>('/api/settings/shopee-api/status')
      .then((res) => {
        if (alive) setAPIStatus(res.data)
      })
      .catch(() => undefined)
    client
      .get<{ data: ShopeeAPIConnection[] }>('/api/shopee-api/connections')
      .then((res) => {
        if (!alive) return
        const rows = res.data.data ?? []
        setAPIConnections(rows)
        const firstActive = rows.find((c) => !c.disabled_at)
        if (firstActive) setSelectedConnectionID((current) => current || firstActive.id)
      })
      .catch(() => undefined)
    return () => {
      alive = false
    }
  }, [])

  useEffect(() => {
    if (step !== 'confirming') {
      setConfirmElapsed(0)
      return
    }
    const startedAt = Date.now()
    const timer = window.setInterval(() => {
      setConfirmElapsed(Math.floor((Date.now() - startedAt) / 1000))
    }, 1000)
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
      event.returnValue = ''
    }
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => {
      window.clearInterval(timer)
      window.removeEventListener('beforeunload', onBeforeUnload)
    }
  }, [step])

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return
    e.target.value = ''
    setStep('uploading')
    setError('')
    setPreview(null)
    setResults(null)
    const form = new FormData()
    form.append('file', file)
    if (selectedConnectionID) form.append('connection_id', selectedConnectionID)
    try {
      const res = await client.post<PreviewResponse>(
        '/api/import/shopee/preview',
        form,
        { headers: { 'Content-Type': 'multipart/form-data' } },
      )
      setPreviewSource('excel')
      setPreview(res.data)
      setSelectedIDs(
        new Set(res.data.orders.filter((o) => !o.duplicate).map((o) => o.order_id)),
      )
      setStep('preview')
    } catch (err: unknown) {
      setError(
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
          'อัปโหลดไฟล์ไม่ได้',
      )
      setStep('idle')
    }
  }

  const refreshAPIStatus = async () => {
    try {
      const res = await client.get<ShopeeAPIStatus>('/api/settings/shopee-api/status')
      setAPIStatus(res.data)
    } catch {
      setError('โหลดสถานะ Shopee API ไม่ได้')
    }
  }

  const refreshAPIConnections = async () => {
    try {
      const res = await client.get<{ data: ShopeeAPIConnection[] }>('/api/shopee-api/connections')
      const rows = res.data.data ?? []
      setAPIConnections(rows)
      const active = rows.filter((c) => !c.disabled_at)
      setSelectedConnectionID((current) => {
        if (current && active.some((c) => c.id === current)) return current
        return active[0]?.id ?? ''
      })
    } catch {
      setError('โหลดรายการร้าน Shopee ไม่ได้')
    }
  }

  const handleRefreshAPISection = async () => {
    setError('')
    await Promise.all([refreshAPIStatus(), refreshAPIConnections()])
  }

  const handleConnectAPI = async () => {
    setError('')
    setAPIBusy(true)
    try {
      const res = await client.post<{ auth_url: string }>('/api/shopee-api/auth-url')
      window.location.href = res.data.auth_url
    } catch (err: unknown) {
      setError(apiErrorMessage(err, 'สร้างลิงก์เชื่อมต่อ Shopee API ไม่ได้'))
    } finally {
      setAPIBusy(false)
    }
  }

  const startEditConnection = (conn: ShopeeAPIConnection) => {
    setEditingConnectionID(conn.id)
    setEditingLabel(conn.label || String(conn.shop_id))
  }

  const saveConnectionLabel = async (conn: ShopeeAPIConnection) => {
    const next = editingLabel.trim()
    if (!next) {
      setError('ชื่อร้านต้องไม่ว่าง')
      return
    }
    setAPIBusy(true)
    setError('')
    try {
      await client.patch(`/api/shopee-api/connections/${conn.id}`, { label: next })
      setEditingConnectionID('')
      setEditingLabel('')
      await refreshAPIConnections()
    } catch (err: unknown) {
      setError(apiErrorMessage(err, 'แก้ไขชื่อร้าน Shopee ไม่ได้'))
    } finally {
      setAPIBusy(false)
    }
  }

  const toggleConnectionDisabled = async (conn: ShopeeAPIConnection) => {
    setAPIBusy(true)
    setError('')
    try {
      await client.patch(`/api/shopee-api/connections/${conn.id}`, { disabled: !conn.disabled_at })
      await refreshAPIConnections()
    } catch (err: unknown) {
      setError(apiErrorMessage(err, 'อัปเดตร้าน Shopee ไม่ได้'))
    } finally {
      setAPIBusy(false)
    }
  }

  const handleFetchAPI = async () => {
    setStep('uploading')
    setError('')
    setPreview(null)
    setResults(null)
    setAPIBusy(true)
    try {
      const res = await client.post<PreviewResponse>('/api/import/shopee/api/preview', {
        connection_id: selectedConnectionID,
        time_from: apiFrom,
        time_to: apiTo,
        time_range_field: 'create_time',
        page_size: 20,
      })
      setPreviewSource('api')
      setPreview(res.data)
      setSelectedIDs(
        new Set(res.data.orders.filter((o) => !o.duplicate).map((o) => o.order_id)),
      )
      setStep('preview')
      void refreshAPIStatus()
      void refreshAPIConnections()
    } catch (err: unknown) {
      setError(apiErrorMessage(err, 'ดึง order จาก Shopee API ไม่ได้'))
      setStep('idle')
      void refreshAPIStatus()
      void refreshAPIConnections()
    } finally {
      setAPIBusy(false)
    }
  }

  const handleConfirm = async () => {
    if (!preview || selectedIDs.size === 0) return
    setStep('confirming')
    setError('')
    try {
      const res = await client.post('/api/import/shopee/confirm', {
        config: config ?? fallbackConfig,
        order_ids: Array.from(selectedIDs),
        orders: preview.orders,
        file_token: preview.file_token,
        import_run_id: preview.import_run_id,
        source_flow: previewSource === 'api' ? 'shopee_api' : 'shopee_excel',
        connection_id: selectedConnectionID,
      }, { timeout: 120000 })
      setResults(res.data)
      setStep('done')
    } catch (err: unknown) {
      setError(
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
          'ส่งข้อมูลไม่ได้',
      )
      setStep('preview')
    }
  }

  const toggleOrder = (id: string) =>
    setSelectedIDs((p) => {
      const s = new Set(p)
      if (s.has(id)) s.delete(id)
      else s.add(id)
      return s
    })
  const toggleAll = () => {
    if (!preview) return
    const nonDup = preview.orders.filter((o) => !o.duplicate).map((o) => o.order_id)
    setSelectedIDs(selectedIDs.size === nonDup.length ? new Set() : new Set(nonDup))
  }
  const toggleExpand = (id: string) =>
    setExpandedOrders((p) => {
      const s = new Set(p)
      if (s.has(id)) s.delete(id)
      else s.add(id)
      return s
    })
  const apiReadiness = apiStatus ? shopeeAPIReadiness(apiStatus) : null
  const apiLive = apiStatus ? isLiveAPI(apiStatus) : false
  const apiDateError = apiRangeError(apiFrom, apiTo)
  const activeConnections = apiConnections.filter((c) => !c.disabled_at)
  const selectedConnection = activeConnections.find((c) => c.id === selectedConnectionID) ?? null
  const needsShopSelection = activeConnections.length > 1 && !selectedConnection
  const selectedShopHint = selectedConnection
    ? `${selectedConnection.label || selectedConnection.shop_name || 'Shopee shop'} · ${selectedConnection.shop_id}`
    : activeConnections.length === 0
      ? 'ยังไม่มีร้านที่เชื่อมต่อ'
      : 'เลือกร้าน Shopee ก่อนดึง order'
  const apiWaitingForLive =
    !!apiStatus && apiStatus.enabled && apiStatus.configured && !apiLive && !apiStatus.connected
  const apiCanConnect = apiStatus?.can_connect ?? (!!apiStatus?.configured && !apiWaitingForLive)
  const apiCanFetch = selectedConnection?.can_fetch ?? false
  const apiConnectDisabled = apiBusy || !apiCanConnect
  const apiFetchDisabled = apiBusy || !apiCanFetch || !!apiDateError || needsShopSelection
  const apiConnectLabel = apiWaitingForLive
    ? 'รอ Shopee approve'
    : activeConnections.length > 0
      ? 'เชื่อมร้านเพิ่ม / reconnect'
      : 'เชื่อมต่อ Shopee API'
  const apiLastSyncError = apiStatus?.last_sync_error
    ? apiErrorMessage({ response: { data: { error: apiStatus.last_sync_error } } }, apiStatus.last_sync_error)
    : ''
  const sourceSelectionVisible = step === 'idle' || step === 'uploading'

  return (
    <div className="space-y-5">
      <PageHeader
        title="Shopee"
        description={`เลือก Shopee shop แล้วดึง API หรืออัปโหลด Excel เพื่อสร้าง${destination.documentName}สำหรับตรวจและส่งเข้า SML`}
      />

      <div className="grid gap-3 md:grid-cols-3">
        <StatusTile
          icon={activeConnections.length > 0 ? CheckCircle2 : Clock3}
          label="สถานะ API"
          value={apiReadiness?.title ?? 'กำลังโหลดสถานะ'}
          description={
            apiWaitingForLive
              ? 'รอ Go-Live approve แล้วเปลี่ยนเป็น live key'
              : activeConnections.length > 0
                ? `เชื่อมแล้ว ${activeConnections.length} ร้าน · ${selectedShopHint}`
                : 'ตรวจ key และ redirect ก่อนเชื่อมร้าน'
          }
          tone={activeConnections.length > 0 ? 'success' : apiWaitingForLive ? 'warning' : 'muted'}
        />
        <StatusTile
          icon={FileSpreadsheet}
          label="ใช้งานได้ตอนนี้"
          value={activeConnections.length > 0 ? 'API + Excel' : 'Excel fallback'}
          description="Excel ยังใช้เป็นทางสำรองได้เมื่อ API หรือ token มีปัญหา"
          tone="primary"
        />
        <StatusTile
          icon={ShieldCheck}
          label="ปลายทาง"
          value={destination.shortName}
          description={destination.smlPath}
          tone="muted"
        />
      </div>

      <input
        ref={fileRef}
        type="file"
        accept=".xlsx"
        className="sr-only"
        onChange={handleFileChange}
      />

      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {sourceSelectionVisible && (
        <div className="grid items-start gap-4 lg:grid-cols-[minmax(0,1.35fr)_minmax(300px,0.65fr)]">
          {apiStatus && (
            <Card>
              <CardHeader className="pb-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <CardTitle className="flex items-center gap-2 text-sm">
                    <PlugZap className="h-4 w-4 text-primary" />
                    Shopee Open API
                    <Badge variant={activeConnections.length > 0 ? 'default' : 'secondary'}>
                      {activeConnections.length > 0 ? `เชื่อม ${activeConnections.length} ร้าน` : 'ยังไม่เชื่อมต่อ'}
                    </Badge>
                    <Badge variant="outline">{apiStatus.environment || 'sandbox'}</Badge>
                  </CardTitle>
                  <Button variant="ghost" size="sm" onClick={handleRefreshAPISection} disabled={apiBusy}>
                    <RefreshCw className="h-4 w-4" />
                    รีเฟรช
                  </Button>
                </div>
              </CardHeader>
              <CardContent className="space-y-4 pt-0">
                {apiReadiness && (
                  <div className={cn('rounded-md border px-3 py-2 text-xs', readinessToneClass(apiReadiness.tone))}>
                    <div className="flex items-start gap-2">
                      {apiReadiness.tone === 'success' ? (
                        <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0" />
                      ) : (
                        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
                      )}
                      <div className="min-w-0 flex-1">
                        <p className="font-medium">{apiReadiness.title}</p>
                        <p className="mt-0.5 text-muted-foreground">{apiReadiness.description}</p>
                        <div className="mt-2 grid gap-1.5 text-muted-foreground sm:grid-cols-2">
                          {apiReadiness.steps.map((s) => (
                            <div key={s.label} className="flex min-w-0 items-start gap-1.5">
                              {readinessStepIcon(s)}
                              <span className="min-w-0">
                                <span>{s.label}</span>
                                {s.detail && (
                                  <span className="block truncate text-[11px]" title={s.detail}>
                                    {s.detail}
                                  </span>
                                )}
                              </span>
                            </div>
                          ))}
                        </div>
                      </div>
                    </div>
                  </div>
                )}

                <div className="rounded-md border border-border bg-muted/20 p-3">
                  <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
                    <div className="flex items-center gap-2 text-sm font-medium text-foreground">
                      <Store className="h-4 w-4 text-primary" />
                      ร้าน Shopee ที่เชื่อมต่อ
                    </div>
                    {activeConnections.length > 0 && (
                      <label className="flex items-center gap-2 text-xs text-muted-foreground">
                        ใช้งานร้าน
                        <select
                          value={selectedConnectionID}
                          onChange={(e) => setSelectedConnectionID(e.target.value)}
                          className="h-8 min-w-[220px] rounded-md border border-border bg-background px-2 text-xs text-foreground"
                        >
                          <option value="">เลือกร้าน</option>
                          {activeConnections.map((conn) => (
                            <option key={conn.id} value={conn.id}>
                              {conn.label || conn.shop_name || 'Shopee shop'} · {conn.shop_id}
                            </option>
                          ))}
                        </select>
                      </label>
                    )}
                  </div>
                  {apiConnections.length === 0 ? (
                    <p className="text-xs text-muted-foreground">
                      ยังไม่มีร้านที่เชื่อมต่อ กด “เชื่อมต่อ Shopee API” เพื่อ authorize ร้านแรก
                    </p>
                  ) : (
                    <div className="space-y-2">
                      {apiConnections.map((conn) => {
                        const editing = editingConnectionID === conn.id
                        const disabled = Boolean(conn.disabled_at)
                        return (
                          <div
                            key={conn.id}
                            className={cn(
                              'grid gap-2 rounded-md border border-border bg-background p-2 text-xs md:grid-cols-[minmax(0,1fr)_auto]',
                              disabled && 'opacity-60',
                            )}
                          >
                            <div className="min-w-0">
                              {editing ? (
                                <input
                                  value={editingLabel}
                                  onChange={(e) => setEditingLabel(e.target.value)}
                                  className="h-8 w-full rounded-md border border-input bg-background px-2 text-xs text-foreground"
                                  maxLength={120}
                                />
                              ) : (
                                <div className="flex min-w-0 flex-wrap items-center gap-1.5">
                                  <span className="truncate font-medium text-foreground">
                                    {conn.label || conn.shop_name || 'Shopee shop'}
                                  </span>
                                  <Badge variant="outline" className="font-mono text-[10px]">
                                    {conn.shop_id}
                                  </Badge>
                                  {selectedConnectionID === conn.id && !disabled && (
                                    <Badge className="text-[10px]">กำลังใช้</Badge>
                                  )}
                                  {disabled && (
                                    <Badge variant="secondary" className="text-[10px]">
                                      ปิดใช้งาน
                                    </Badge>
                                  )}
                                </div>
                              )}
                              <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
                                <span>Token: {tokenStateLabel(conn.token_state)}</span>
                                <span>Last sync: {conn.last_sync_at ? fmtDateTime(conn.last_sync_at) : '—'}</span>
                                {conn.last_sync_error && (
                                  <span className="text-destructive" title={conn.last_sync_error}>
                                    sync error
                                  </span>
                                )}
                              </div>
                            </div>
                            <div className="flex items-center justify-end gap-1">
                              {editing ? (
                                <>
                                  <Button type="button" size="sm" variant="outline" className="h-8 px-2" onClick={() => saveConnectionLabel(conn)} disabled={apiBusy}>
                                    <Save className="h-3.5 w-3.5" />
                                    บันทึก
                                  </Button>
                                  <Button type="button" size="sm" variant="ghost" className="h-8 px-2" onClick={() => setEditingConnectionID('')} disabled={apiBusy}>
                                    <X className="h-3.5 w-3.5" />
                                  </Button>
                                </>
                              ) : (
                                <>
                                  <Button type="button" size="sm" variant="ghost" className="h-8 px-2" onClick={() => startEditConnection(conn)} disabled={apiBusy}>
                                    <Pencil className="h-3.5 w-3.5" />
                                    ชื่อ
                                  </Button>
                                  <Button type="button" size="sm" variant={disabled ? 'outline' : 'ghost'} className="h-8 px-2" onClick={() => toggleConnectionDisabled(conn)} disabled={apiBusy}>
                                    <Power className="h-3.5 w-3.5" />
                                    {disabled ? 'เปิดใช้' : 'ปิดใช้'}
                                  </Button>
                                </>
                              )}
                            </div>
                          </div>
                        )
                      })}
                    </div>
                  )}
                </div>

                <div className="grid gap-2 text-xs text-muted-foreground md:grid-cols-3 xl:grid-cols-6">
                  <div className="rounded-md border border-border p-3">
                    <p className="font-medium text-foreground">Partner</p>
                    <p className="mt-1 font-mono">{apiStatus.partner_id || 'ยังไม่ได้ตั้งค่า'}</p>
                  </div>
                  <div className="rounded-md border border-border p-3">
                    <p className="font-medium text-foreground">Selected shop</p>
                    <p className="mt-1 truncate font-mono" title={selectedShopHint}>{selectedConnection?.shop_id || '—'}</p>
                  </div>
                  <div className="rounded-md border border-border p-3">
                    <p className="font-medium text-foreground">Base URL</p>
                    <p className="mt-1 truncate font-mono" title={apiStatus.base_url || '—'}>
                      {apiStatus.base_url || '—'}
                    </p>
                  </div>
                  <div className="rounded-md border border-border p-3">
                    <p className="font-medium text-foreground">Redirect</p>
                    <p className="mt-1 truncate font-mono" title={apiStatus.redirect_url || '—'}>
                      {apiStatus.redirect_url || '—'}
                    </p>
                  </div>
                  <div className="rounded-md border border-border p-3">
                    <p className="font-medium text-foreground">Token</p>
                    <p className="mt-1">{tokenStateLabel(apiStatus.token_state)}</p>
                  </div>
                  <div className="rounded-md border border-border p-3">
                    <p className="font-medium text-foreground">Last sync</p>
                    <p className="mt-1">
                      {apiStatus.last_sync_at ? fmtDateTime(apiStatus.last_sync_at) : '—'}
                    </p>
                  </div>
                </div>
                {!apiStatus.configured && (
                  <Alert>
                    <AlertTriangle className="h-4 w-4" />
                    <AlertTitle>ยังไม่ได้ตั้งค่า Shopee Open API บน server</AlertTitle>
                    <AlertDescription>
                      ต้องตั้งค่า partner_id และ partner_key ก่อนเชื่อมต่อร้าน
                    </AlertDescription>
                  </Alert>
                )}
                {apiStatus.blocking_reason && (
                  <Alert>
                    <Info className="h-4 w-4" />
                    <AlertTitle>สิ่งที่ต้องทำต่อ</AlertTitle>
                    <AlertDescription>{apiStatus.blocking_reason}</AlertDescription>
                  </Alert>
                )}
                <div className="rounded-md border border-border bg-muted/20 p-3">
                  <div className="flex flex-wrap items-end gap-2">
                    <Button
                      variant={apiStatus.connected ? 'outline' : 'default'}
                      size="sm"
                      onClick={handleConnectAPI}
                      disabled={apiConnectDisabled}
                      title={apiStatus.blocking_reason || undefined}
                    >
                      {apiBusy ? <Loader2 className="h-4 w-4 animate-spin" /> : <PlugZap className="h-4 w-4" />}
                      {apiConnectLabel}
                    </Button>
                    <label className="text-xs text-muted-foreground">
                      จาก
                      <input
                        type="date"
                        value={apiFrom}
                        onChange={(e) => setAPIFrom(e.target.value)}
                        className="ml-2 h-8 rounded-md border border-input bg-background px-2 text-xs text-foreground"
                      />
                    </label>
                    <label className="text-xs text-muted-foreground">
                      ถึง
                      <input
                        type="date"
                        value={apiTo}
                        onChange={(e) => setAPITo(e.target.value)}
                        className="ml-2 h-8 rounded-md border border-input bg-background px-2 text-xs text-foreground"
                      />
                    </label>
                    <Button
                      size="sm"
                      onClick={handleFetchAPI}
                      disabled={apiFetchDisabled}
                      title={apiDateError || (needsShopSelection ? 'เลือกร้าน Shopee ก่อนดึง order' : apiStatus.blocking_reason) || undefined}
                    >
                      {apiBusy ? <Loader2 className="h-4 w-4 animate-spin" /> : <Database className="h-4 w-4" />}
                      ดึงออเดอร์จาก API
                    </Button>
                  </div>
                  {(apiDateError || apiLastSyncError || needsShopSelection) && (
                    <div className="mt-2 space-y-1 text-xs">
                      {apiDateError && <p className="text-warning">{apiDateError}</p>}
                      {needsShopSelection && <p className="text-warning">เลือกร้าน Shopee ก่อนดึง order เพื่อกัน import ผิดร้าน</p>}
                      {apiLastSyncError && <p className="text-destructive">{apiLastSyncError}</p>}
                    </div>
                  )}
                </div>
              </CardContent>
            </Card>
          )}

          <div className="space-y-4">
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="flex items-center gap-2 text-sm">
                  <FileSpreadsheet className="h-4 w-4 text-primary" />
                  Excel fallback
                  <Badge variant="outline">ใช้ได้ตอนนี้</Badge>
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-0">
                <div
                  className={cn(
                    'flex min-h-[180px] flex-col items-center justify-center rounded-lg border-2 border-dashed border-border bg-muted/20 p-6 text-center',
                    step === 'uploading' && !apiBusy && 'opacity-60',
                  )}
                >
                  {step === 'uploading' && !apiBusy ? (
                    <p className="text-sm text-muted-foreground">กำลังวิเคราะห์ไฟล์…</p>
                  ) : (
                    <>
                      <FileSpreadsheet className="mb-3 h-10 w-10 text-muted-foreground" />
                      <p className="text-sm font-medium text-foreground">
                        ไฟล์ Excel (.xlsx) จาก Shopee Seller Center
                      </p>
                      <p className="mt-1 text-[11px] leading-5 text-muted-foreground">
                        ระบบจะ preview และข้าม Order ID ที่มีใน BillFlow แล้ว
                      </p>
                      <Button
                        className="mt-4"
                        onClick={() => fileRef.current?.click()}
                        disabled={!channelReady}
                        title={!configReady ? 'กำลังเตรียมหน้า import' : !channelReady ? 'ยังไม่ได้ตั้งค่า shopee/sale ในช่องทางรับข้อมูล' : undefined}
                      >
                        {configLoading ? 'กำลังโหลด config…' : 'เลือกไฟล์ Shopee'}
                      </Button>
                    </>
                  )}
                </div>
                {configReady && !channelReady && (
                  <Alert className="mt-3">
                    <AlertTriangle className="h-4 w-4" />
                    <AlertTitle>ยังไม่ได้ตั้งค่า Shopee sale defaults</AlertTitle>
                    <AlertDescription>
                      ตั้งค่าช่องทาง Shopee สำหรับบิลขายก่อนนำเข้า เพื่อให้บิลรู้ว่าจะใช้ลูกค้า คลัง ชั้น และ VAT ใด
                      <Button asChild variant="link" className="h-auto px-1 py-0 text-xs">
                        <Link to="/settings/channels">ไปตั้งค่าตอนนี้</Link>
                      </Button>
                    </AlertDescription>
                  </Alert>
                )}
              </CardContent>
            </Card>

            {recentRuns.length > 0 && (
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="flex items-center gap-2 text-sm">
                    <Clock3 className="h-4 w-4 text-muted-foreground" />
                    ประวัติการนำเข้าล่าสุด
                  </CardTitle>
                </CardHeader>
                <CardContent className="space-y-2 pt-0">
                  {recentRuns.map((run) => (
                    <div
                      key={run.id}
                      className="grid gap-2 rounded-md border border-border px-3 py-2 text-xs"
                    >
                      <div className="min-w-0">
                        <div className="truncate font-medium text-foreground">
                          {run.filename || 'Shopee Import'}
                        </div>
                        <div className="mt-0.5 text-muted-foreground">
                          {fmtDateTime(run.created_at)}
                          {run.period_start && run.period_end
                            ? ` · ${run.period_start} ถึง ${run.period_end}`
                            : ''}
                        </div>
                      </div>
                      <div className="flex flex-wrap items-center gap-1">
                        <Badge variant={run.status === 'confirmed' ? 'default' : 'secondary'}>
                          {run.status === 'confirmed' ? 'สร้างแล้ว' : 'Preview'}
                        </Badge>
                        <Badge variant="outline">ใหม่ {run.new_orders}</Badge>
                        <Badge variant="outline">ซ้ำ {run.duplicate_orders}</Badge>
                        <Badge variant="outline">ข้าม {run.skipped_orders}</Badge>
                      </div>
                    </div>
                  ))}
                </CardContent>
              </Card>
            )}
          </div>
        </div>
      )}

      {step === 'preview' && preview && (
        <>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-4">
            <SummaryCard
              label="Orders ทั้งหมด"
              value={preview.total_orders}
              variant="primary"
            />
            <SummaryCard
              label="Order ใหม่"
              value={preview.preflight?.new_orders ?? preview.new_count}
              variant="success"
            />
            <SummaryCard
              label="เลือกแล้ว"
              value={selectedIDs.size}
              variant="success"
            />
            <SummaryCard
              label="ซ้ำ (ข้ามไป)"
              value={preview.duplicate_count}
              variant="muted"
            />
          </div>

          <Alert>
            <Info className="h-4 w-4" />
            <AlertTitle>นโยบายการนำเข้าซ้ำ</AlertTitle>
            <AlertDescription>
              ถ้าไฟล์หรือ API ครอบคลุมช่วงวันที่เดิม ระบบจะสร้างเฉพาะ Order ID ที่ยังไม่มีในร้าน Shopee เดียวกัน และจะข้ามรายการซ้ำโดยไม่เขียนทับบิลเดิม
            </AlertDescription>
          </Alert>

          {(preview.orders.some((o) => o.shopee_shop_id) || selectedConnection) && (
            <Alert>
              <Store className="h-4 w-4" />
              <AlertTitle>ร้านที่ใช้กับ preview นี้</AlertTitle>
              <AlertDescription>
                {preview.orders.find((o) => o.shopee_shop_label)?.shopee_shop_label || selectedConnection?.label || 'Shopee shop'}
                {' · '}
                {preview.orders.find((o) => o.shopee_shop_id)?.shopee_shop_id || selectedConnection?.shop_id}
              </AlertDescription>
            </Alert>
          )}

          {(preview.preflight?.no_sku_items ?? 0) > 0 && (
            <Alert>
              <AlertTriangle className="h-4 w-4" />
              <AlertTitle>ไฟล์นี้ไม่มี SKU บางรายการ</AlertTitle>
              <AlertDescription>
                พบ {preview.preflight.no_sku_items} รายการสินค้าใน {preview.preflight.no_sku_orders} order ที่ไม่มี SKU ระบบจะใช้ชื่อสินค้า + ตัวเลือกสินค้าเป็นข้อมูลจับคู่แทน
              </AlertDescription>
            </Alert>
          )}

          {(preview.warnings ?? []).length > 0 && (
            <Alert>
              <AlertTriangle className="h-4 w-4" />
              <AlertTitle>คำเตือน ({preview.warnings.length} รายการ)</AlertTitle>
              <AlertDescription>
                <ul className="mt-1 list-disc pl-5 text-xs">
                  {preview.warnings.map((w, i) => (
                    <li key={i}>{w}</li>
                  ))}
                </ul>
              </AlertDescription>
            </Alert>
          )}

          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" size="sm" onClick={toggleAll}>
              {selectedIDs.size === preview.orders.filter((o) => !o.duplicate).length
                ? 'ยกเลิกทั้งหมด'
                : 'เลือกทั้งหมด'}
            </Button>
            <Button
              size="sm"
              disabled={selectedIDs.size === 0}
              onClick={handleConfirm}
            >
              {destination.action} {selectedIDs.size} รายการ
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setStep('idle')
                setPreview(null)
              }}
            >
              เลือกไฟล์ใหม่
            </Button>
          </div>

          <div className="overflow-hidden rounded-lg border border-border bg-card">
            <Table>
              <TableHeader>
                <TableRow className="bg-muted/40">
                  <TableHead className="w-10">
                    <Checkbox
                      checked={
                        selectedIDs.size ===
                        preview.orders.filter((o) => !o.duplicate).length
                      }
                      onCheckedChange={toggleAll}
                      aria-label="เลือกทั้งหมด"
                    />
                  </TableHead>
                  <TableHead>Order ID</TableHead>
                  <TableHead>วันที่</TableHead>
                  <TableHead>ผู้ซื้อ</TableHead>
                  <TableHead>สถานะ</TableHead>
                  <TableHead>สินค้า</TableHead>
                  <TableHead className="text-right">Qty รวม</TableHead>
                  <TableHead className="text-right">ยอดชำระ</TableHead>
                  <TableHead>Preflight</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {preview.orders.map((order) => {
                  const expanded = expandedOrders.has(order.order_id)
                  return (
                    <Fragment key={order.order_id}>
                      <TableRow
                        className={cn(
                          order.duplicate && 'bg-muted/30 text-muted-foreground',
                        )}
                      >
                        <TableCell>
                          <Checkbox
                            checked={selectedIDs.has(order.order_id)}
                            disabled={order.duplicate}
                            onCheckedChange={() => toggleOrder(order.order_id)}
                            aria-label={`เลือก order ${order.order_id}`}
                          />
                        </TableCell>
                        <TableCell>
                          <div className="space-y-1">
                            <button
                              type="button"
                              className="inline-flex items-center gap-1 font-mono text-xs font-medium text-foreground hover:text-primary"
                              onClick={() => toggleExpand(order.order_id)}
                            >
                              {expanded ? (
                                <ChevronDown className="h-3 w-3" />
                              ) : (
                                <ChevronRight className="h-3 w-3" />
                              )}
                              {order.order_id}
                            </button>
                            {order.shopee_shop_id && (
                              <div className="flex max-w-[220px] items-center gap-1 text-[11px] text-muted-foreground">
                                <Store className="h-3 w-3 shrink-0" />
                                <span className="truncate">
                                  {order.shopee_shop_label || 'Shopee shop'} · {order.shopee_shop_id}
                                </span>
                              </div>
                            )}
                          </div>
                        </TableCell>
                        <TableCell className="text-xs tabular-nums text-muted-foreground">
                          {order.order_datetime || order.doc_date}
                        </TableCell>
                        <TableCell className="max-w-[160px] truncate text-xs text-muted-foreground">
                          {order.buyer_username || '—'}
                        </TableCell>
                        <TableCell>
                          <Badge variant="secondary" className="text-xs font-normal">
                            {order.status}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-xs">
                          {order.item_count} รายการ
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {order.total_qty}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {order.paid_amount != null ? `฿${fmt(order.paid_amount)}` : '—'}
                        </TableCell>
                        <TableCell>
                          <div className="flex flex-wrap gap-1">
                          {order.duplicate && (
                            <Badge variant="secondary" className="bg-warning/15 text-warning hover:bg-warning/20">
                              มีในระบบแล้ว
                            </Badge>
                          )}
                          {order.has_no_sku && (
                            <Badge variant="outline" className="border-warning/40 text-warning">
                              ไม่มี SKU {order.no_sku_item_count}
                            </Badge>
                          )}
                          {order.multi_line && (
                            <Badge variant="outline">หลายรายการ</Badge>
                          )}
                          {order.amount_mismatch && (
                            <Badge variant="outline">
                              มีส่วนต่างยอด
                            </Badge>
                          )}
                          </div>
                        </TableCell>
                      </TableRow>
                      {expanded && (
                        <TableRow>
                          <TableCell colSpan={9} className="bg-muted/20 p-0">
                            <div className="overflow-hidden border-l-2 border-primary/40">
                              <Table>
                                <TableHeader>
                                  <TableRow className="bg-muted/30">
                                    <TableHead className="text-[10px] uppercase">SKU</TableHead>
                                    <TableHead className="text-[10px] uppercase">ชื่อสินค้า</TableHead>
                                    <TableHead className="text-[10px] uppercase">ตัวเลือก</TableHead>
                                    <TableHead className="text-right text-[10px] uppercase">ราคา</TableHead>
                                    <TableHead className="text-right text-[10px] uppercase">จำนวน</TableHead>
                                  </TableRow>
                                </TableHeader>
                                <TableBody>
                                  {order.items.map((item, i) => (
                                    <TableRow key={i}>
                                      <TableCell className="font-mono text-xs">
                                        {item.sku || (
                                          <Badge variant="outline" className="border-warning/40 text-warning">
                                            ไม่มี SKU
                                          </Badge>
                                        )}
                                      </TableCell>
                                      <TableCell className="text-sm">
                                        <div>{item.raw_name || item.product_name}</div>
                                        {item.raw_name && item.raw_name !== item.product_name && (
                                          <div className="mt-1 text-[11px] text-muted-foreground">
                                            ต้นทาง: {item.product_name}
                                          </div>
                                        )}
                                      </TableCell>
                                      <TableCell className="text-xs text-muted-foreground">
                                        {item.option_name || '—'}
                                      </TableCell>
                                      <TableCell className="text-right tabular-nums">
                                        {fmt(item.price)}
                                      </TableCell>
                                      <TableCell className="text-right tabular-nums">
                                        {item.qty}
                                      </TableCell>
                                    </TableRow>
                                  ))}
                                </TableBody>
                              </Table>
                            </div>
                          </TableCell>
                        </TableRow>
                      )}
                    </Fragment>
                  )
                })}
              </TableBody>
            </Table>
          </div>
        </>
      )}

      {step === 'confirming' && (
        <Card className="overflow-hidden">
          <CardContent className="p-0">
            <div className="border-b border-border bg-muted/30 px-5 py-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <p className="text-sm font-semibold text-foreground">
                    กำลัง{destination.action}จาก {previewSource === 'api' ? 'Shopee API' : 'Shopee Excel'}
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    ระบบกำลังจับคู่สินค้าและสร้างเอกสารไว้รอตรวจ ยังไม่ส่งเข้า SML อัตโนมัติ
                  </p>
                </div>
                <Badge variant="secondary" className="gap-1.5">
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  {confirmElapsed}s
                </Badge>
              </div>
              <div className="mt-4 h-2 overflow-hidden rounded-full bg-background">
                <div
                  className="h-full rounded-full bg-primary transition-all duration-500"
                  style={{ width: `${Math.min(92, 18 + confirmElapsed * 2)}%` }}
                />
              </div>
            </div>
            <div className="grid gap-3 p-5 sm:grid-cols-3">
              <div className="rounded-md border border-border p-3">
                <Database className="h-4 w-4 text-primary" />
                <p className="mt-2 text-xs font-medium">Order ที่เลือก</p>
                <p className="mt-1 text-2xl font-semibold tabular-nums">
                  {selectedIDs.size}
                </p>
              </div>
              <div className="rounded-md border border-border p-3">
                <ShieldCheck className="h-4 w-4 text-success" />
                <p className="mt-2 text-xs font-medium">กันนำเข้าซ้ำ</p>
                <p className="mt-1 text-xs text-muted-foreground">
                  Order ID ที่มีแล้วจะถูกข้าม ไม่เขียนทับบิลเดิม
                </p>
              </div>
              <div className="rounded-md border border-border p-3">
                <Clock3 className="h-4 w-4 text-warning" />
                <p className="mt-2 text-xs font-medium">กรุณารอหน้านี้</p>
                <p className="mt-1 text-xs text-muted-foreground">
                  ถ้าเปลี่ยนเมนู งานบน server อาจยังทำต่อ ให้ดูผลย้อนหลังในประวัติการนำเข้า
                </p>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {step === 'done' && results && (
        <>
          <Alert>
            <CheckCircle2 className="h-4 w-4 text-success" />
            <AlertTitle>{destination.done} {results.success_count} รายการ</AlertTitle>
            <AlertDescription>
              ระบบ map สินค้าให้เบื้องต้น แต่ <b>ยังไม่ส่ง SML</b> — กรุณาไปที่เมนู{destination.listName}เพื่อตรวจสินค้า
              หน่วย จำนวน ราคา และส่งเข้า SML ปลายทาง {destination.smlPath}
            </AlertDescription>
          </Alert>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <SummaryCard
              label="สร้างเอกสารสำเร็จ"
              value={results.success_count}
              variant="success"
            />
            <SummaryCard
              label="ข้าม / ล้มเหลว"
              value={results.fail_count}
              variant="danger"
            />
            <SummaryCard
              label="ทั้งหมด"
              value={results.results.length}
              variant="primary"
            />
          </div>

          <div className="flex gap-2">
            <Button asChild>
              <Link to={destination.listPath}>
                ไปตรวจ{destination.listName}
                <ArrowRight className="h-4 w-4" />
              </Link>
            </Button>
            <Button
              variant="ghost"
              onClick={() => {
                setStep('idle')
                setPreview(null)
                setResults(null)
              }}
            >
              นำเข้าไฟล์ใหม่
            </Button>
          </div>

          <Card>
            <CardHeader>
              <CardTitle className="text-sm">รายละเอียดผลลัพธ์</CardTitle>
            </CardHeader>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/40">
                    <TableHead>Order ID</TableHead>
                    <TableHead>ผล</TableHead>
                    <TableHead>หมายเหตุ</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {results.results.map((r) => (
                    <TableRow key={r.order_id}>
                      <TableCell className="font-mono text-xs">{r.order_id}</TableCell>
                      <TableCell>
                        {r.success ? (
                          r.bill_id ? (
                            <Link
                              to={`${destination.listPath}/${r.bill_id}`}
                              className="inline-flex items-center gap-1 font-medium text-success hover:underline"
                            >
                              เปิดรายละเอียด
                              <ArrowRight className="h-3 w-3" />
                            </Link>
                          ) : (
                            <span className="font-medium text-success">สำเร็จ</span>
                          )
                        ) : (
                          r.bill_id ? (
                            <Link
                              to={`${destination.listPath}/${r.bill_id}`}
                              className="inline-flex items-center gap-1 font-medium text-warning hover:underline"
                            >
                              เปิดรายการเดิม
                              <ArrowRight className="h-3 w-3" />
                            </Link>
                          ) : (
                            <span className="font-medium text-destructive">
                              ข้าม / ล้มเหลว
                            </span>
                          )
                        )}
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        {r.message}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )
}
