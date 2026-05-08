import { useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { AlertTriangle, CheckCircle2, Clock, Info, Search, Send } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import BillTable from '@/components/BillTable'
import { PageHeader } from '@/components/common/PageHeader'
import { useBills } from '@/hooks/useBills'
import { BulkSendDialog } from './BulkSendDialog'
import {
  BILL_SOURCE_LABEL,
  BILL_STATUS_LABEL,
  BILL_TYPE_LABEL,
  PAGE_TITLE,
} from '@/lib/labels'

const PER_PAGE = 20
const ALL = '__all__'

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

type BillsMode = 'purchase-order' | 'sales-order' | 'sale-invoice'

const MODE_CONFIG: Record<BillsMode, {
  title: string
  description: string
  source: string
  billType: 'purchase' | 'sale'
  documentRoute?: string
  destination: string
  docCode: string
  routeLabel: string
  routeTo: string
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
    searchPlaceholder: 'ค้นหาเลขบิล / เลขคำสั่งซื้อ / ผู้ขาย…',
  },
  'sales-order': {
    title: PAGE_TITLE.salesOrders,
    description: 'ตรวจข้อมูลจาก Shopee Excel ที่นำเข้า แล้วสร้างเป็นใบสั่งขายเพื่อส่งเข้า SML',
    source: 'shopee',
    billType: 'sale',
    documentRoute: 'saleorder',
    destination: 'ขาย -> ใบสั่งขาย',
    docCode: 'SR',
    routeLabel: 'Shopee Excel',
    routeTo: '/import/shopee',
    searchPlaceholder: 'ค้นหาเลขบิล / เลขคำสั่งซื้อ / ลูกค้า…',
  },
  'sale-invoice': {
    title: PAGE_TITLE.saleInvoices,
    description: 'ตรวจข้อมูลจาก Shopee Excel ที่นำเข้า แล้วสร้างเป็นเอกสารขายสินค้าและบริการเพื่อส่งเข้า SML',
    source: 'shopee',
    billType: 'sale',
    documentRoute: 'saleinvoice',
    destination: 'ขาย -> ขายสินค้าและบริการ',
    docCode: 'SI',
    routeLabel: 'Shopee Excel',
    routeTo: '/import/shopee',
    searchPlaceholder: 'ค้นหาเลขบิล / เลขคำสั่งซื้อ / ลูกค้า…',
  },
}

function readURLFilter(params: URLSearchParams, key: string, valid: string[]): string {
  const v = params.get(key) ?? ''
  return v && valid.includes(v) ? v : ALL
}

export default function Bills({ mode = 'purchase-order' }: { mode?: BillsMode }) {
  const config = MODE_CONFIG[mode]
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  // Seed filters from the URL so deep-links from the Dashboard ("บิลล้มเหลว"
  // shortcut → /bills?status=failed) land pre-filtered. After that, filters
  // are local state — admin can change them without bouncing the URL.
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<string>(() =>
    readURLFilter(searchParams, 'status', VALID_STATUSES),
  )
  const [search, setSearch] = useState('')
  const [bulkOpen, setBulkOpen] = useState(false)

  const { data, loading, refetch } = useBills({
    page,
    per_page: PER_PAGE,
    status: status === ALL ? '' : status,
    source: config.source,
    bill_type: config.billType,
    document_route: config.documentRoute,
    search,
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
  const detailBasePath =
    mode === 'sale-invoice' ? '/sale-invoices' : mode === 'sales-order' ? '/sales-orders' : '/bills'

  const resetPage = (cb: () => void) => {
    cb()
    setPage(1)
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title={config.title}
        description={config.description}
      />

      <div className="flex flex-wrap items-center gap-2 rounded-xl border border-info/25 bg-info/5 px-4 py-3 text-sm">
        <Info className="h-4 w-4 shrink-0 text-info" />
        <span className="text-muted-foreground">ช่องทางเข้า:</span>
        <Link to={config.routeTo} className="font-medium text-primary hover:underline">
          {config.routeLabel}
        </Link>
        <span className="text-muted-foreground">→ ปลายทาง SML:</span>
        <span className="font-medium text-foreground">{config.destination}</span>
        <code className="rounded bg-background px-1.5 py-0.5 font-mono text-xs text-foreground">
          {config.docCode}
        </code>
        <Link
          to="/settings/channels"
          className="ml-auto text-xs font-medium text-primary hover:underline"
        >
          ตั้งค่าเส้นทาง
        </Link>
      </div>

      <div className="grid grid-cols-2 gap-2 lg:grid-cols-4">
        <QueueMetric label="ต้องตรวจสินค้า" value={needsReviewCount.data?.total ?? 0} icon={AlertTriangle} tone="warning" />
        <QueueMetric label="พร้อมส่ง" value={pendingCount.data?.total ?? 0} icon={Clock} tone="primary" />
        <QueueMetric label="ส่งแล้ว" value={sentCount.data?.total ?? 0} icon={CheckCircle2} tone="success" />
        <QueueMetric label="ส่งไม่สำเร็จ" value={failedCount.data?.total ?? 0} icon={Send} tone="danger" />
      </div>

      <div className="rounded-xl border border-border/70 bg-card p-3 shadow-sm">
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

        <span className="rounded-md border border-border bg-background px-2.5 py-2 text-xs text-muted-foreground">
          {BILL_SOURCE_LABEL[config.source]} · {BILL_TYPE_LABEL[config.billType]}
        </span>
        <Button
          type="button"
          size="sm"
          className="ml-auto gap-1.5"
          disabled={(pendingCount.data?.total ?? 0) === 0}
          onClick={() => setBulkOpen(true)}
        >
          <Send className="h-3.5 w-3.5" />
          ส่ง SML ทั้งหมด {(pendingCount.data?.total ?? 0).toLocaleString()} รายการ
        </Button>
        </div>
      </div>

      <BillTable
        bills={data?.data ?? []}
        loading={loading}
        onRowClick={(id) => navigate(`${detailBasePath}/${id}`)}
      />

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
    </div>
  )
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
