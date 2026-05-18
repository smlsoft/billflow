import { useEffect, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { AlertTriangle, CheckCircle2, Clock, Info, Mail, Search, Send, Settings, UploadCloud } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import BillTable from '@/components/BillTable'
import { EmptyState } from '@/components/common/EmptyState'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { archiveBill, deleteBill, restoreBill, useBills } from '@/hooks/useBills'
import { useAuth } from '@/hooks/useAuth'
import client from '@/api/client'
import { BulkSendDialog } from './BulkSendDialog'
import {
  BILL_SOURCE_LABEL,
  BILL_STATUS_LABEL,
  BILL_TYPE_LABEL,
  PAGE_TITLE,
} from '@/lib/labels'
import type { Bill } from '@/types'

const PER_PAGE = 20
const ALL = '__all__'

interface InboxOption {
  id: string
  name: string
  username: string
}

// Filter options pull labels from lib/labels.ts so Bills, Dashboard, and
// Logs all show identical status names — no more "ล้มเหลว" vs "ส่ง SML
// ล้มเหลว" drift.
const STATUS_OPTIONS = [
  { value: ALL, label: 'ทุกสถานะ' },
  ...['pending', 'needs_review', 'sent', 'failed', 'skipped'].map((s) => ({
    value: s,
    label: BILL_STATUS_LABEL[s],
  })),
]

// Valid filter values used to validate URL query string against typos.
const VALID_STATUSES = STATUS_OPTIONS.map((o) => o.value)

const SHOPEE_STATUS_OPTIONS = [
  { value: ALL, label: 'ทุกสถานะคำสั่งซื้อ' },
  { value: 'shipped', label: 'ถูกจัดส่งแล้ว' },
  { value: 'payment_confirmed', label: 'ยืนยันการชำระเงินแล้ว' },
  { value: 'ready_to_ship', label: 'เตรียมจัดส่ง' },
  { value: 'picked_up', label: 'คนขับเข้ารับ' },
  { value: 'delivered', label: 'จัดส่งสำเร็จ' },
  { value: 'cancelled', label: 'ยกเลิก' },
  { value: 'refund', label: 'คืนเงิน' },
  { value: 'return', label: 'คืนสินค้า' },
]

const VALID_SHOPEE_STATUSES = SHOPEE_STATUS_OPTIONS.map((o) => o.value)
const ARCHIVE_OPTIONS = [
  { value: 'active', label: 'รายการปกติ' },
  { value: 'include', label: 'รวมบิลที่เก็บแล้ว' },
  { value: 'only', label: 'บิลที่เก็บแล้ว' },
] as const
type ArchiveMode = typeof ARCHIVE_OPTIONS[number]['value']

type BillsMode = 'purchase-order' | 'sales-order' | 'sale-invoice'

const MODE_CONFIG: Record<BillsMode, {
  title: string
  description: string
  source: string
  sourceLabel?: string
  billType: 'purchase' | 'sale'
  documentRoute?: string
  destination: string
  docCode: string
  routeLabel: string
  routeTo: string
  emptyTitle: string
  emptyDescription: string
  emptyActionLabel: string
  emptyActionTo: string
  emptySecondaryLabel?: string
  emptySecondaryTo?: string
  searchPlaceholder: string
}> = {
  'purchase-order': {
    title: PAGE_TITLE.bills,
    description: 'ตรวจข้อมูลจากกล่องอีเมลรับบิลที่ตั้งค่าไว้ แล้วสร้างเป็นใบสั่งซื้อเพื่อส่งเข้า SML',
    source: 'shopee_shipped',
    billType: 'purchase',
    destination: 'ซื้อ -> ใบสั่งซื้อ',
    docCode: 'PO',
    routeLabel: 'กล่องอีเมลรับบิล',
    routeTo: '/settings/email',
    emptyTitle: 'ยังไม่มีใบสั่งซื้อ',
    emptyDescription: 'เมื่อ BillFlow อ่านอีเมลรับบิลจากกล่องที่ตั้งค่าไว้ เอกสารซื้อจะเข้าคิวที่นี่ให้ตรวจสินค้าและส่งเข้า SML',
    emptyActionLabel: 'ไปตั้งค่ากล่องอีเมล',
    emptyActionTo: '/settings/email',
    emptySecondaryLabel: 'ตรวจหน้าเริ่มต้นใช้งาน',
    emptySecondaryTo: '/setup',
    searchPlaceholder: 'ค้นหาเลขบิล / เลขคำสั่งซื้อ / ผู้ขาย…',
  },
  'sales-order': {
    title: PAGE_TITLE.salesOrders,
    description: 'ตรวจข้อมูลจาก Marketplace Excel ที่นำเข้า แล้วสร้างเป็นใบสั่งขายเพื่อส่งเข้า SML',
    source: '',
    sourceLabel: 'Marketplace Excel',
    billType: 'sale',
    documentRoute: 'saleorder',
    destination: 'ขาย -> ใบสั่งขาย',
    docCode: 'SR',
    routeLabel: 'Marketplace Excel',
    routeTo: '/import/shopee',
    emptyTitle: 'ยังไม่มีใบสั่งขาย',
    emptyDescription: 'นำเข้าไฟล์ Shopee, Lazada หรือ TikTok แล้วเอกสารที่ตั้งปลายทางเป็นใบสั่งขายจะมาอยู่หน้านี้',
    emptyActionLabel: 'นำเข้าไฟล์ Marketplace',
    emptyActionTo: '/import/shopee',
    emptySecondaryLabel: 'ตั้งค่าเส้นทาง SML',
    emptySecondaryTo: '/settings/channels',
    searchPlaceholder: 'ค้นหาเลขบิล / เลขคำสั่งซื้อ / ลูกค้า…',
  },
  'sale-invoice': {
    title: PAGE_TITLE.saleInvoices,
    description: 'ตรวจข้อมูลจาก Marketplace Excel ที่นำเข้า แล้วสร้างเป็นเอกสารขายสินค้าและบริการเพื่อส่งเข้า SML',
    source: '',
    sourceLabel: 'Marketplace Excel',
    billType: 'sale',
    documentRoute: 'saleinvoice',
    destination: 'ขาย -> ขายสินค้าและบริการ',
    docCode: 'SI',
    routeLabel: 'Marketplace Excel',
    routeTo: '/import/shopee',
    emptyTitle: 'ยังไม่มีเอกสารขายสินค้าและบริการ',
    emptyDescription: 'นำเข้าไฟล์ Shopee, Lazada หรือ TikTok แล้วเลือกปลายทาง SML เป็นขายสินค้าและบริการ เอกสารจะมาอยู่หน้านี้',
    emptyActionLabel: 'นำเข้าไฟล์ Marketplace',
    emptyActionTo: '/import/shopee',
    emptySecondaryLabel: 'ตั้งค่าเส้นทาง SML',
    emptySecondaryTo: '/settings/channels',
    searchPlaceholder: 'ค้นหาเลขบิล / เลขคำสั่งซื้อ / ลูกค้า…',
  },
}

function readURLFilter(params: URLSearchParams, key: string, valid: string[]): string {
  const v = params.get(key) ?? ''
  return v && valid.includes(v) ? v : ALL
}

function readURLArchive(params: URLSearchParams): ArchiveMode {
  const v = params.get('archived')
  return v === 'include' || v === 'only' ? v : 'active'
}

export default function Bills({ mode = 'purchase-order' }: { mode?: BillsMode }) {
  const config = MODE_CONFIG[mode]
  const { user } = useAuth()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  // Seed filters from the URL so deep-links from the Dashboard ("บิลล้มเหลว"
  // shortcut → /bills?status=failed) land pre-filtered. After that, filters
  // are local state — admin can change them without bouncing the URL.
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<string>(() =>
    readURLFilter(searchParams, 'status', VALID_STATUSES),
  )
  const [shopeeStatus, setShopeeStatus] = useState<string>(() =>
    readURLFilter(searchParams, 'shopee_status', VALID_SHOPEE_STATUSES),
  )
  const [emailAccountId, setEmailAccountId] = useState(ALL)
  const [inboxes, setInboxes] = useState<InboxOption[]>([])
  const [search, setSearch] = useState('')
  const [archiveMode, setArchiveMode] = useState<ArchiveMode>(() => readURLArchive(searchParams))
  const [bulkOpen, setBulkOpen] = useState(false)
  const [confirmAction, setConfirmAction] = useState<{
    kind: 'archive' | 'restore' | 'delete' | 'permanent'
    bill: Bill
  } | null>(null)
  const showShopeeStatusFilter = mode === 'purchase-order'
  const canManageBills = user?.role === 'admin' || user?.role === 'staff'
  const canPermanentDelete = user?.role === 'admin'

  const { data, loading, refetch } = useBills({
    page,
    per_page: PER_PAGE,
    status: status === ALL ? '' : status,
    shopee_status: showShopeeStatusFilter && shopeeStatus !== ALL ? shopeeStatus : '',
    source: config.source,
    bill_type: config.billType,
    document_route: config.documentRoute,
    email_account_id: emailAccountId === ALL ? '' : emailAccountId,
    search,
    archived: archiveMode === 'active' ? '' : archiveMode,
  })
  const needsReviewCount = useBills({
    page: 1,
    per_page: 1,
    status: 'needs_review',
    source: config.source,
    bill_type: config.billType,
    document_route: config.documentRoute,
  })
  const pendingCount = useBills({
    page: 1,
    per_page: 1,
    status: 'pending',
    source: config.source,
    bill_type: config.billType,
    document_route: config.documentRoute,
  })
  const sentCount = useBills({
    page: 1,
    per_page: 1,
    status: 'sent',
    source: config.source,
    bill_type: config.billType,
    document_route: config.documentRoute,
  })
  const failedCount = useBills({
    page: 1,
    per_page: 1,
    status: 'failed',
    source: config.source,
    bill_type: config.billType,
    document_route: config.documentRoute,
  })

  const totalPages = data ? Math.max(1, Math.ceil(data.total / PER_PAGE)) : 1
  const hasMore = data ? page * PER_PAGE < data.total : false
  const bulkCandidateCount = (pendingCount.data?.total ?? 0) + (needsReviewCount.data?.total ?? 0)
  const detailBasePath =
    mode === 'sale-invoice' ? '/sale-invoices' : mode === 'sales-order' ? '/sales-orders' : '/bills'

  const resetPage = (cb: () => void) => {
    cb()
    setPage(1)
  }

  const refreshAll = () => {
    refetch()
    pendingCount.refetch()
    needsReviewCount.refetch()
    sentCount.refetch()
    failedCount.refetch()
  }

  const handleConfirmedAction = async () => {
    if (!confirmAction) return
    const { kind, bill } = confirmAction
    try {
      if (kind === 'archive') {
        await archiveBill(bill.id, 'ผู้ใช้เก็บบิลจากหน้ารายการ')
        toast.success('เก็บบิลแล้ว')
      } else if (kind === 'restore') {
        await restoreBill(bill.id)
        toast.success('กู้คืนบิลแล้ว')
      } else {
        await deleteBill(bill.id)
        toast.success(kind === 'permanent' ? 'ลบถาวรแล้ว' : 'ลบบิลแล้ว')
      }
      setConfirmAction(null)
      refreshAll()
    } catch (err: unknown) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      toast.error(e?.response?.data?.error || e?.message || 'ทำรายการไม่สำเร็จ')
    }
  }

  useEffect(() => {
    let alive = true
    client.get<{ data: InboxOption[] }>('/api/settings/imap-accounts')
      .then((res) => {
        if (alive) setInboxes(res.data.data ?? [])
      })
      .catch(() => {
        if (alive) setInboxes([])
      })
    return () => { alive = false }
  }, [])

  useEffect(() => {
    const next = new URLSearchParams(searchParams)
    if (status === ALL) next.delete('status')
    else next.set('status', status)
    if (showShopeeStatusFilter && shopeeStatus !== ALL) next.set('shopee_status', shopeeStatus)
    else next.delete('shopee_status')
    if (archiveMode === 'active') next.delete('archived')
    else next.set('archived', archiveMode)
    const nextString = next.toString()
    if (nextString !== searchParams.toString()) {
      setSearchParams(next, { replace: true })
    }
  }, [status, shopeeStatus, archiveMode, showShopeeStatusFilter, searchParams, setSearchParams])

  return (
    <div className="space-y-5">
      <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
        <QueueMetric label="ต้องตรวจสินค้า" value={needsReviewCount.data?.total ?? 0} icon={AlertTriangle} tone="warning" />
        <QueueMetric label="พร้อมส่ง" value={pendingCount.data?.total ?? 0} icon={Clock} tone="primary" />
        <QueueMetric label="ส่งแล้ว" value={sentCount.data?.total ?? 0} icon={CheckCircle2} tone="success" />
        <QueueMetric label="ส่งไม่สำเร็จ" value={failedCount.data?.total ?? 0} icon={Send} tone="danger" />
      </div>

      <div className="rounded-xl border border-border/70 bg-card p-3 shadow-sm">
        <div className="mb-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
          <Info className="h-3.5 w-3.5 shrink-0 text-primary" />
          <Link to={config.routeTo} className="font-medium text-primary hover:underline">
            {config.routeLabel}
          </Link>
          <span>→</span>
          <span className="font-medium text-foreground">{config.destination}</span>
          <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-[11px] text-foreground">
            {config.docCode}
          </code>
          <Link
            to="/settings/channels"
            className="font-medium text-primary hover:underline sm:ml-auto"
          >
            ตั้งค่าเส้นทาง
          </Link>
        </div>

        <div className="mb-2 flex flex-wrap gap-1.5">
          {STATUS_OPTIONS.map((o) => (
            <button
              key={o.value}
              type="button"
              onClick={() => resetPage(() => setStatus(o.value))}
              className={[
                'rounded-full border px-2.5 py-1 text-xs font-medium transition-colors',
                status === o.value
                  ? 'border-primary bg-primary text-primary-foreground'
                  : 'border-border bg-background text-muted-foreground hover:bg-accent/70 hover:text-foreground',
              ].join(' ')}
            >
              {o.label}
            </button>
          ))}
        </div>

        <div className="mb-2 flex flex-wrap gap-1.5">
          {ARCHIVE_OPTIONS.map((o) => (
            <button
              key={o.value}
              type="button"
              onClick={() => resetPage(() => setArchiveMode(o.value))}
              className={[
                'rounded-full border px-2.5 py-1 text-xs font-medium transition-colors',
                archiveMode === o.value
                  ? 'border-primary bg-primary text-primary-foreground'
                  : 'border-border bg-background text-muted-foreground hover:bg-accent/70 hover:text-foreground',
              ].join(' ')}
            >
              {o.label}
            </button>
          ))}
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <div className="relative w-full max-w-sm">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder={config.searchPlaceholder}
              value={search}
              onChange={(e) => resetPage(() => setSearch(e.target.value))}
              className="h-9 pl-8"
            />
          </div>

          {mode !== 'purchase-order' && (
            <span className="w-full rounded-md border border-border bg-background px-2.5 py-2 text-xs text-muted-foreground sm:w-auto">
              {(config.sourceLabel ?? BILL_SOURCE_LABEL[config.source])} · {BILL_TYPE_LABEL[config.billType]}
            </span>
          )}
          {showShopeeStatusFilter && (
            <select
              value={shopeeStatus}
              onChange={(e) => resetPage(() => setShopeeStatus(e.target.value))}
              className="h-9 w-full min-w-0 rounded-md border border-border bg-background px-2.5 text-xs text-foreground sm:w-auto"
              aria-label="กรองตามสถานะคำสั่งซื้อ Shopee"
            >
              {SHOPEE_STATUS_OPTIONS.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          )}
          {inboxes.length > 0 && config.routeTo === '/settings/email' && (
            <select
              value={emailAccountId}
              onChange={(e) => resetPage(() => setEmailAccountId(e.target.value))}
              className="h-9 w-full min-w-0 rounded-md border border-border bg-background px-2.5 text-xs text-foreground sm:w-auto"
              aria-label="กรองตามกล่องอีเมล"
            >
              <option value={ALL}>ทุกกล่องอีเมล</option>
              {inboxes.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name} · {a.username}
                </option>
              ))}
            </select>
          )}
          <Button
            type="button"
            size="sm"
            className="w-full min-w-0 justify-center gap-1.5 sm:ml-auto sm:w-auto"
            disabled={bulkCandidateCount === 0 || archiveMode === 'only'}
            onClick={() => setBulkOpen(true)}
          >
            <Send className="h-3.5 w-3.5" />
            <span className="truncate">
              ส่ง SML ทั้งหมด {bulkCandidateCount.toLocaleString()} รายการ
            </span>
          </Button>
        </div>
      </div>

      {!loading && (data?.total ?? 0) === 0 && !search && status === ALL && shopeeStatus === ALL && archiveMode === 'active' ? (
        <EmptyState
          icon={mode === 'purchase-order' ? Mail : UploadCloud}
          title={config.emptyTitle}
          description={config.emptyDescription}
          action={
            <div className="flex flex-wrap justify-center gap-2">
              <Button asChild>
                <Link to={config.emptyActionTo}>
                  {mode === 'purchase-order' ? <Settings className="h-4 w-4" /> : <UploadCloud className="h-4 w-4" />}
                  {config.emptyActionLabel}
                </Link>
              </Button>
              {config.emptySecondaryLabel && config.emptySecondaryTo && (
                <Button asChild variant="outline">
                  <Link to={config.emptySecondaryTo}>{config.emptySecondaryLabel}</Link>
                </Button>
              )}
            </div>
          }
        />
      ) : (
        <BillTable
          bills={data?.data ?? []}
          loading={loading}
          showShopeeStatusColumn={showShopeeStatusFilter}
          canManage={canManageBills}
          canPermanentDelete={canPermanentDelete}
          onArchive={(bill: Bill) => setConfirmAction({ kind: 'archive', bill })}
          onRestore={(bill: Bill) => setConfirmAction({ kind: 'restore', bill })}
          onDelete={(bill: Bill) => setConfirmAction({ kind: 'delete', bill })}
          onPermanentDelete={(bill: Bill) => setConfirmAction({ kind: 'permanent', bill })}
          onRowClick={(id) => navigate(`${detailBasePath}/${id}`)}
        />
      )}

      <div className="flex items-center justify-between text-xs text-muted-foreground">
        <span>ทั้งหมด {(data?.total ?? 0).toLocaleString()} รายการ</span>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            disabled={page <= 1}
            onClick={() => setPage((p) => p - 1)}
          >
            ก่อนหน้า
          </Button>
          <span className="px-1 tabular-nums text-foreground">
            หน้า {page} / {totalPages}
          </span>
          <Button
            variant="outline"
            size="sm"
            disabled={!hasMore}
            onClick={() => setPage((p) => p + 1)}
          >
            ถัดไป
          </Button>
        </div>
      </div>

      <BulkSendDialog
        open={bulkOpen}
        onOpenChange={setBulkOpen}
        title={config.title}
        billType={config.billType}
        filters={{
          source: config.source,
          bill_type: config.billType,
          document_route: config.documentRoute,
        }}
        onDone={() => {
          refetch()
          pendingCount.refetch()
          needsReviewCount.refetch()
          sentCount.refetch()
          failedCount.refetch()
        }}
      />

      <ConfirmDialog
        open={confirmAction !== null}
        onOpenChange={(open) => !open && setConfirmAction(null)}
        title={confirmActionTitle(confirmAction)}
        description={confirmActionDescription(confirmAction)}
        confirmLabel={confirmAction?.kind === 'permanent' ? 'ลบถาวร' : confirmAction?.kind === 'delete' ? 'ลบบิล' : confirmAction?.kind === 'restore' ? 'กู้คืน' : 'เก็บบิล'}
        variant={confirmAction?.kind === 'delete' || confirmAction?.kind === 'permanent' ? 'destructive' : 'default'}
        onConfirm={handleConfirmedAction}
      />
    </div>
  )
}

function confirmActionTitle(action: { kind: 'archive' | 'restore' | 'delete' | 'permanent'; bill: Bill } | null) {
  if (!action) return ''
  if (action.kind === 'archive') return 'เก็บบิลนี้?'
  if (action.kind === 'restore') return 'กู้คืนบิลนี้?'
  if (action.kind === 'permanent') return 'ลบถาวร?'
  return 'ลบบิลนี้?'
}

function confirmActionDescription(action: { kind: 'archive' | 'restore' | 'delete' | 'permanent'; bill: Bill } | null) {
  if (!action) return ''
  const doc = action.bill.sml_doc_no || action.bill.id.slice(0, 8)
  if (action.kind === 'archive') return `เก็บบิล ${doc} ออกจากหน้างานประจำ แต่ยังค้นย้อนหลังและดู logs ได้`
  if (action.kind === 'restore') return `นำบิล ${doc} กลับมาแสดงในรายการปกติ`
  if (action.kind === 'permanent') return `ลบบิล ${doc} และไฟล์แนบถาวร คืนไม่ได้ แต่ logs จะยังเก็บข้อมูลสำคัญไว้`
  return `ลบบิล ${doc} ที่ยังไม่ได้ส่งเข้า SML พร้อมรายการสินค้าและไฟล์แนบ`
}

function QueueMetric({
  label,
  value,
  icon: Icon,
  tone,
}: {
  label: string
  value: number
  icon: typeof AlertTriangle
  tone: 'primary' | 'warning' | 'success' | 'danger'
}) {
  const toneCls = {
    primary: 'bg-primary/10 text-primary',
    warning: 'bg-warning/10 text-warning',
    success: 'bg-success/10 text-success',
    danger: 'bg-destructive/10 text-destructive',
  }[tone]
  return (
    <div className="flex items-center gap-3 rounded-xl border border-border/70 bg-card p-3 shadow-sm">
      <div className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-lg ${toneCls}`}>
        <Icon className="h-4 w-4" />
      </div>
      <div className="min-w-0">
        <p className="text-xl font-semibold leading-6 tabular-nums">{value.toLocaleString()}</p>
        <p className="text-xs text-muted-foreground">{label}</p>
      </div>
    </div>
  )
}
