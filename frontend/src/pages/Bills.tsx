import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Search } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import BillTable from '@/components/BillTable'
import { PageHeader } from '@/components/common/PageHeader'
import { useBills } from '@/hooks/useBills'
import {
  BILL_SOURCE_LABEL,
  BILL_STATUS_LABEL,
  BILL_TYPE_LABEL,
  PAGE_TITLE,
} from '@/lib/labels'

const PER_PAGE = 20
const ALL = '__all__'
const PHASE = Number(import.meta.env.VITE_PHASE ?? 99)

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

const SOURCE_OPTIONS =
  PHASE < 2
    ? [
        { value: ALL, label: 'ทุกช่องทาง' },
        { value: 'email', label: BILL_SOURCE_LABEL.email },
        { value: 'shopee_email', label: BILL_SOURCE_LABEL.shopee_email },
        { value: 'shopee_shipped', label: BILL_SOURCE_LABEL.shopee_shipped },
      ]
    : [
        { value: ALL, label: 'ทุกช่องทาง' },
        ...Object.entries(BILL_SOURCE_LABEL).map(([value, label]) => ({ value, label })),
      ]

const BILL_TYPE_OPTIONS = [
  { value: ALL, label: 'ทุกประเภท' },
  ...Object.entries(BILL_TYPE_LABEL).map(([value, label]) => ({ value, label })),
]

// Valid filter values used to validate URL query string against typos.
const VALID_STATUSES = STATUS_OPTIONS.map((o) => o.value)
const VALID_SOURCES = SOURCE_OPTIONS.map((o) => o.value)
const VALID_BILL_TYPES = BILL_TYPE_OPTIONS.map((o) => o.value)

function readURLFilter(params: URLSearchParams, key: string, valid: string[]): string {
  const v = params.get(key) ?? ''
  return v && valid.includes(v) ? v : ALL
}

export default function Bills() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  // Seed filters from the URL so deep-links from the Dashboard ("บิลล้มเหลว"
  // shortcut → /bills?status=failed) land pre-filtered. After that, filters
  // are local state — admin can change them without bouncing the URL.
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<string>(() =>
    readURLFilter(searchParams, 'status', VALID_STATUSES),
  )
  const [source, setSource] = useState<string>(() =>
    readURLFilter(searchParams, 'source', VALID_SOURCES),
  )
  const [billType, setBillType] = useState<string>(() =>
    readURLFilter(searchParams, 'bill_type', VALID_BILL_TYPES),
  )
  const [search, setSearch] = useState('')

  const { data, loading } = useBills({
    page,
    per_page: PER_PAGE,
    status: status === ALL ? '' : status,
    source: source === ALL ? '' : source,
    bill_type: billType === ALL ? '' : billType,
    search,
  })

  const totalPages = data ? Math.max(1, Math.ceil(data.total / PER_PAGE)) : 1
  const hasMore = data ? page * PER_PAGE < data.total : false

  const resetPage = (cb: () => void) => {
    cb()
    setPage(1)
  }

  return (
    <div className="space-y-5">
      <PageHeader title={PAGE_TITLE.bills} description="ติดตามและจัดการบิลทุกช่องทางในระบบ" />

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative w-full max-w-xs">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="ค้นหาผู้ขาย / เลขบิล…"
            value={search}
            onChange={(e) => resetPage(() => setSearch(e.target.value))}
            className="h-9 pl-8"
          />
        </div>

        <Select value={status} onValueChange={(v) => resetPage(() => setStatus(v))}>
          <SelectTrigger className="h-9 w-[170px]">
            <SelectValue placeholder="สถานะ" />
          </SelectTrigger>
          <SelectContent>
            {STATUS_OPTIONS.map((o) => (
              <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={source} onValueChange={(v) => resetPage(() => setSource(v))}>
          <SelectTrigger className="h-9 w-[180px]">
            <SelectValue placeholder="ช่องทาง" />
          </SelectTrigger>
          <SelectContent>
            {SOURCE_OPTIONS.map((o) => (
              <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select value={billType} onValueChange={(v) => resetPage(() => setBillType(v))}>
          <SelectTrigger className="h-9 w-[150px]">
            <SelectValue placeholder="ประเภท" />
          </SelectTrigger>
          <SelectContent>
            {BILL_TYPE_OPTIONS.map((o) => (
              <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <BillTable
        bills={data?.data ?? []}
        loading={loading}
        onRowClick={(id) => navigate(`/bills/${id}`)}
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
    </div>
  )
}
