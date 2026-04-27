import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
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

const PER_PAGE = 20
const ALL = '__all__'

const STATUS_OPTIONS = [
  { value: ALL, label: 'ทุกสถานะ' },
  { value: 'pending', label: 'รอดำเนินการ' },
  { value: 'needs_review', label: 'รอตรวจสอบ' },
  { value: 'sent', label: 'SML สำเร็จ' },
  { value: 'failed', label: 'ล้มเหลว' },
  { value: 'skipped', label: 'ข้ามแล้ว' },
]

const SOURCE_OPTIONS = [
  { value: ALL, label: 'ทุกช่องทาง' },
  { value: 'line', label: 'LINE OA' },
  { value: 'email', label: 'Email' },
  { value: 'shopee', label: 'Shopee Excel' },
  { value: 'shopee_email', label: 'Shopee Order' },
  { value: 'shopee_shipped', label: 'Shopee Shipped (PO)' },
  { value: 'lazada', label: 'Lazada' },
  { value: 'manual', label: 'Manual' },
]

const BILL_TYPE_OPTIONS = [
  { value: ALL, label: 'ทุกประเภท' },
  { value: 'sale', label: 'บิลขาย' },
  { value: 'purchase', label: 'บิลซื้อ (PO)' },
]

export default function Bills() {
  const navigate = useNavigate()
  const [page, setPage] = useState(1)
  const [status, setStatus] = useState<string>(ALL)
  const [source, setSource] = useState<string>(ALL)
  const [billType, setBillType] = useState<string>(ALL)
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
      <PageHeader title="รายการบิล" description="ติดตามและจัดการบิลทั้งหมดในระบบ" />

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
