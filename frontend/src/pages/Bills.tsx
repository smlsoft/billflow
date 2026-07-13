import { useEffect, useMemo, useRef, useState, type FormEvent } from 'react'
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { AlertTriangle, ArrowDownUp, CheckCircle2, ChevronLeft, ChevronRight, Clock, Filter, Mail, Printer, Search, Send, Settings, Store, UploadCloud } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import BillTable from '@/components/BillTable'
import { BillSourceBadge } from '@/components/BillSourceBadge'
import { EmptyState } from '@/components/common/EmptyState'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { DateRangePicker } from '@/components/common/DateRangePicker'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { archiveBill, deleteBill, getEmailPrintCandidates, recordBulkEmailPrintEvents, restoreBill, useBills } from '@/hooks/useBills'
import { getBill, updateBillPrintPaymentMethod, updateBillPurchaseCreditor } from '@/hooks/useBills'
import { printArtifact, printArtifactsBatch, recordArtifactPrint, type ArtifactPrintContext } from './BillDetail/hooks/useArtifacts'
import { useAuth } from '@/hooks/useAuth'
import client from '@/api/client'
import { BulkSendDialog } from './BulkSendDialog'
import { UpdatePurchaseCreditorDialog } from './BillDetail/components/UpdatePurchaseCreditorDialog'
import { UpdatePrintPaymentMethodDialog } from './BillDetail/components/UpdatePrintPaymentMethodDialog'
import {
  BILL_SOURCE_LABEL,
  BILL_STATUS_LABEL,
  BILL_TYPE_LABEL,
  PAGE_TITLE,
} from '@/lib/labels'
import type { Bill, EmailPrintCandidate } from '@/types'
import type { Party } from '@/pages/ChannelDefaults/PartyPicker'
import { DEFAULT_MARKETPLACE_PRINT_PAYMENT_METHODS } from '@/pages/ChannelDefaults/labels'

const DEFAULT_PER_PAGE = 20
const PAGE_SIZE_OPTIONS = [20, 50, 100] as const
const BULK_BATCH_SIZE = 100
const ALL = '__all__'

interface InboxOption {
  id: string
  name: string
  username: string
}

interface ShopeeShopOption {
  id: string
  shop_id: number
  label: string
  shop_name?: string
  disabled_at?: string
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
const PURCHASE_SOURCE_OPTIONS = [
  { value: ALL, label: 'ทุกช่องทาง', description: 'รวม Email บิลซื้อ Shopee และ Lazada' },
  { value: 'shopee_shipped', label: BILL_SOURCE_LABEL.shopee_shipped, description: 'อีเมลคำสั่งซื้อ Shopee ที่ส่งไปใบสั่งซื้อ' },
  { value: 'lazada_email', label: BILL_SOURCE_LABEL.lazada_email, description: 'อีเมล Lazada ที่สร้างเป็นบิลซื้อรอตรวจ' },
]
const VALID_PURCHASE_SOURCES = PURCHASE_SOURCE_OPTIONS.map((o) => o.value)
const PAYMENT_METHOD_ALL_LABEL = 'ทุกวิธีชำระเงิน'
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
    source: '',
    sourceLabel: 'Email บิลซื้อ Shopee/Lazada',
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

function readURLPage(params: URLSearchParams): number {
  const n = Number(params.get('page'))
  return Number.isInteger(n) && n > 0 ? n : 1
}

function readURLPerPage(params: URLSearchParams): typeof PAGE_SIZE_OPTIONS[number] {
  const n = Number(params.get('per_page'))
  return PAGE_SIZE_OPTIONS.includes(n as typeof PAGE_SIZE_OPTIONS[number])
    ? n as typeof PAGE_SIZE_OPTIONS[number]
    : DEFAULT_PER_PAGE
}

export default function Bills({ mode = 'purchase-order' }: { mode?: BillsMode }) {
  const config = MODE_CONFIG[mode]
  const { user } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const [searchParams, setSearchParams] = useSearchParams()
  const lastSearchStringRef = useRef(searchParams.toString())
  const syncingFromURLRef = useRef(false)
  // Seed filters from the URL so deep-links/shared links keep the exact queue
  // view, including page and page size.
  const [page, setPage] = useState(() => readURLPage(searchParams))
  const [perPage, setPerPage] = useState<typeof PAGE_SIZE_OPTIONS[number]>(() => readURLPerPage(searchParams))
  const [pageJumpInput, setPageJumpInput] = useState(() => String(readURLPage(searchParams)))
  const [counts, setCounts] = useState({
    needs_review: 0,
    pending: 0,
    sent: 0,
    failed: 0,
    skipped: 0,
    total: 0,
    print_ready_orders: 0,
    print_ready_groups: 0,
  })
  const [status, setStatus] = useState<string>(() =>
    readURLFilter(searchParams, 'status', VALID_STATUSES),
  )
  const [shopeeStatus, setShopeeStatus] = useState<string>(() =>
    readURLFilter(searchParams, 'shopee_status', VALID_SHOPEE_STATUSES),
  )
  const [sourceFilter, setSourceFilter] = useState<string>(() =>
    readURLFilter(searchParams, 'source', VALID_PURCHASE_SOURCES),
  )
  const [printReadyOnly, setPrintReadyOnly] = useState(() => searchParams.get('print_ready') === '1')
  const [sourceFilterOpen, setSourceFilterOpen] = useState(false)
  const [shopeeShopId, setShopeeShopId] = useState(() => searchParams.get('shopee_shop_id') || ALL)
  const [emailAccountId, setEmailAccountId] = useState(() => searchParams.get('email_account_id') || ALL)
  const [inboxes, setInboxes] = useState<InboxOption[]>([])
  const [shopeeShops, setShopeeShops] = useState<ShopeeShopOption[]>([])
  const [searchInput, setSearchInput] = useState(() => searchParams.get('search') ?? '')
  const [search, setSearch] = useState(() => (searchParams.get('search') ?? '').trim())
  const [paymentMethodFilter, setPaymentMethodFilter] = useState(() => searchParams.get('print_payment_method')?.trim() || ALL)
  const [archiveMode, setArchiveMode] = useState<ArchiveMode>(() => readURLArchive(searchParams))
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>(() =>
    searchParams.get('sort_order') === 'asc' ? 'asc' : 'desc',
  )
  const [dateFrom, setDateFrom] = useState(() => searchParams.get('date_from') ?? '')
  const [dateTo, setDateTo] = useState(() => searchParams.get('date_to') ?? '')
  const [bulkOpen, setBulkOpen] = useState(false)
  const [bulkPrintOpen, setBulkPrintOpen] = useState(false)
  const [bulkPrintLoading, setBulkPrintLoading] = useState(false)
  const [bulkPrintCandidates, setBulkPrintCandidates] = useState<EmailPrintCandidate[]>([])
  const [bulkPrintTruncated, setBulkPrintTruncated] = useState(false)
  const [printLoadingMessageID, setPrintLoadingMessageID] = useState<string | null>(null)
  const [creditorDialogBill, setCreditorDialogBill] = useState<Bill | null>(null)
  const [creditorDialogLoading, setCreditorDialogLoading] = useState(false)
  const [creditorDialogLoadingBillId, setCreditorDialogLoadingBillId] = useState<string | null>(null)
  const [creditorUpdating, setCreditorUpdating] = useState(false)
  const [paymentDialogBill, setPaymentDialogBill] = useState<Bill | null>(null)
  const [paymentDialogLoading, setPaymentDialogLoading] = useState(false)
  const [paymentDialogLoadingBillId, setPaymentDialogLoadingBillId] = useState<string | null>(null)
  const [paymentUpdating, setPaymentUpdating] = useState(false)
  const [confirmAction, setConfirmAction] = useState<{
    kind: 'archive' | 'restore' | 'delete' | 'permanent'
    bill: Bill
  } | null>(null)
  const showPurchaseSourceFilter = mode === 'purchase-order'
  const showShopeeStatusFilter = mode === 'purchase-order'
  const showShopeeShopFilter = mode !== 'purchase-order'
  const canUseBillActions = user?.role === 'admin' || user?.role === 'staff'
  const canBulkSend = user?.role === 'admin'
  const canManageBillLifecycle = user?.role === 'admin'
  const canPermanentDelete = user?.role === 'admin'
  const canUpdatePurchaseCreditor = user?.role === 'admin'
  const canUpdatePrintPaymentMethod = user?.role === 'admin' || user?.role === 'staff'
  const activeSource = showPurchaseSourceFilter && sourceFilter !== ALL ? sourceFilter : config.source
  const activeSourceOption = PURCHASE_SOURCE_OPTIONS.find((o) => o.value === sourceFilter) ?? PURCHASE_SOURCE_OPTIONS[0]
  const activePaymentMethod = showPurchaseSourceFilter && paymentMethodFilter !== ALL ? paymentMethodFilter : ''

  const { data, loading, refetch } = useBills({
    page,
    per_page: perPage,
    include_total: true,
    status: status === ALL ? '' : status,
    shopee_status: showShopeeStatusFilter && shopeeStatus !== ALL ? shopeeStatus : '',
    source: activeSource,
    bill_type: config.billType,
    document_route: config.documentRoute,
    print_ready: showPurchaseSourceFilter && printReadyOnly,
    email_account_id: emailAccountId === ALL ? '' : emailAccountId,
    shopee_shop_id: showShopeeShopFilter && shopeeShopId !== ALL ? shopeeShopId : '',
    search,
    print_payment_method: activePaymentMethod,
    archived: archiveMode === 'active' ? '' : archiveMode,
    sort_order: sortOrder,
    date_from: dateFrom || undefined,
    date_to: dateTo || undefined,
  })
  const bills = data?.data ?? []
  const paymentMethodOptions = useMemo(() => {
    const values = new Set<string>()
    DEFAULT_MARKETPLACE_PRINT_PAYMENT_METHODS.forEach((method) => values.add(method))
    bills.forEach((bill) => {
      const method = effectivePrintPaymentMethodFromBill(bill)
      if (method) values.add(method)
    })
    if (activePaymentMethod) values.add(activePaymentMethod)
    return [
      { value: ALL, label: PAYMENT_METHOD_ALL_LABEL },
      ...Array.from(values).map((method) => ({ value: method, label: method })),
    ]
  }, [activePaymentMethod, bills])
  const selectedPaymentMethodLabel =
    paymentMethodOptions.find((o) => o.value === paymentMethodFilter)?.label ?? paymentMethodFilter
  const total = typeof data?.total === 'number' ? data.total : counts.total
  const totalPages = Math.max(1, Math.ceil(total / perPage))
  const pageStart = total === 0 ? 0 : (page - 1) * perPage + 1
  const pageEnd = total === 0 ? 0 : Math.min(page * perPage, total)
  const hasPreviousPage = page > 1
  const hasNextPage = page < totalPages
  const bulkCandidateCount = counts.pending
  const bulkStatusAllowed = status === ALL || status === 'pending'
  const bulkSourceAllowed = !showPurchaseSourceFilter || sourceFilter !== ALL
  const bulkDisabled = !canBulkSend || bulkCandidateCount === 0 || archiveMode !== 'active' || !bulkStatusAllowed || !bulkSourceAllowed
  const bulkButtonLabel =
    !canBulkSend
      ? 'ส่ง SML ใช้ได้เฉพาะผู้ดูแลระบบ'
      : archiveMode !== 'active'
      ? 'ส่ง SML ใช้ได้เฉพาะรายการปกติ'
      : !bulkSourceAllowed
        ? 'เลือกช่องทางก่อนส่ง SML แบบกลุ่ม'
      : !bulkStatusAllowed
        ? 'ส่ง SML ใช้ได้เมื่อดูทุกสถานะ/เอกสารสถานะพร้อมส่ง'
        : bulkCandidateCount > BULK_BATCH_SIZE
          ? `ส่ง SML เอกสารสถานะพร้อมส่งชุดแรก ${BULK_BATCH_SIZE}/${bulkCandidateCount.toLocaleString()} รายการ`
          : `ส่ง SML เอกสารสถานะพร้อมส่ง ${bulkCandidateCount.toLocaleString()} รายการ`
  const detailBasePath =
    mode === 'sale-invoice' ? '/sale-invoices' : mode === 'sales-order' ? '/sales-orders' : '/bills'
  const selectedInbox = inboxes.find((a) => a.id === emailAccountId)
  const selectedShopeeShop = shopeeShops.find((shop) => String(shop.shop_id) === shopeeShopId)
  const selectedShopeeStatus = SHOPEE_STATUS_OPTIONS.find((o) => o.value === shopeeStatus)?.label ?? SHOPEE_STATUS_OPTIONS[0].label
  const selectedArchiveLabel = ARCHIVE_OPTIONS.find((o) => o.value === archiveMode)?.label ?? ARCHIVE_OPTIONS[0].label
  const sourceSummary = showPurchaseSourceFilter
    ? activeSourceOption.label
    : (config.sourceLabel ?? BILL_SOURCE_LABEL[config.source] ?? 'ทุกช่องทาง')
  const inboxSummary = config.routeTo === '/settings/email'
    ? selectedInbox
      ? selectedInbox.name
      : 'ทุกกล่องอีเมล'
    : showShopeeShopFilter
      ? selectedShopeeShop?.label || selectedShopeeShop?.shop_name || (shopeeShopId !== ALL ? `Shopee ${shopeeShopId}` : 'ทุกร้าน Shopee')
      : 'ไม่จำกัด'
  const dateSummary =
    dateFrom && dateTo
      ? `${dateFrom} ถึง ${dateTo}`
      : dateFrom
        ? `ตั้งแต่ ${dateFrom}`
        : dateTo
          ? `ถึง ${dateTo}`
          : 'ไม่จำกัด'
  const activeFilterSummary = [
    `ช่องทาง: ${sourceSummary}`,
    showShopeeStatusFilter ? `สถานะคำสั่งซื้อ: ${selectedShopeeStatus}` : '',
    showPurchaseSourceFilter && printReadyOnly ? 'รอพิมพ์อีเมล' : '',
    showPurchaseSourceFilter && activePaymentMethod ? `วิธีชำระ: ${selectedPaymentMethodLabel}` : '',
    `กล่อง/ร้าน: ${inboxSummary}`,
    `วันที่: ${dateSummary}`,
    `มุมมอง: ${selectedArchiveLabel}`,
  ].filter(Boolean)

  const resetPage = (cb: () => void) => {
    cb()
    setPage(1)
  }

  const refreshAll = () => {
    setPage(1)
    refetch()
    fetchCounts()
  }

  const handlePerPageChange = (value: string) => {
    const next = Number(value)
    if (!PAGE_SIZE_OPTIONS.includes(next as typeof PAGE_SIZE_OPTIONS[number])) return
    setPerPage(next as typeof PAGE_SIZE_OPTIONS[number])
    setPage(1)
  }

  const handleJumpToPage = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const next = Number(pageJumpInput)
    if (!Number.isInteger(next) || next < 1) {
      setPageJumpInput(String(page))
      toast.error('เลขหน้าต้องเป็นจำนวนเต็มตั้งแต่ 1 ขึ้นไป')
      return
    }
    setPage(Math.min(next, totalPages))
  }

  const handleOpenBillDetail = (id: string) => {
    const returnTo = `${location.pathname}${location.search}`
    window.sessionStorage.setItem(`billflow:return:${id}`, returnTo)
    navigate(`${detailBasePath}/${id}`, { state: { returnTo } })
  }

  const fetchCounts = async () => {
    const params = new URLSearchParams()
    if (activeSource) params.set('source', activeSource)
    params.set('bill_type', config.billType)
    if (config.documentRoute) params.set('document_route', config.documentRoute)
    if (emailAccountId !== ALL) params.set('email_account_id', emailAccountId)
    if (archiveMode !== 'active') params.set('archived', archiveMode)
    if (showPurchaseSourceFilter && printReadyOnly) params.set('print_ready', '1')
    if (showShopeeStatusFilter && shopeeStatus !== ALL) params.set('shopee_status', shopeeStatus)
    if (showShopeeShopFilter && shopeeShopId !== ALL) params.set('shopee_shop_id', shopeeShopId)
    if (search) params.set('search', search)
    if (activePaymentMethod) params.set('print_payment_method', activePaymentMethod)
    if (dateFrom) params.set('date_from', dateFrom)
    if (dateTo) params.set('date_to', dateTo)
    const res = await client.get<typeof counts>(`/api/bills/counts?${params}`)
    setCounts(res.data)
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

  const handleOpenPurchaseCreditorDialog = async (bill: Bill) => {
    if (creditorDialogLoading) return
    const toastID = toast.loading('กำลังโหลดรายละเอียดบิล...')
    setCreditorDialogLoading(true)
    setCreditorDialogLoadingBillId(bill.id)
    try {
      const fullBill = await getBill(bill.id)
      setCreditorDialogBill(fullBill)
    } catch (err) {
      toast.error(
        err instanceof Error && err.message
          ? err.message
          : 'โหลดรายละเอียดบิลไม่สำเร็จ',
      )
    } finally {
      toast.dismiss(toastID)
      setCreditorDialogLoading(false)
      setCreditorDialogLoadingBillId(null)
    }
  }

  const handlePurchaseCreditorConfirm = async (party: Party) => {
    if (!creditorDialogBill || creditorUpdating) return
    setCreditorUpdating(true)
    try {
      const result = await updateBillPurchaseCreditor(creditorDialogBill.id, {
        party_code: party.code,
        party_name: party.name,
      })
      setCreditorDialogBill(null)
      toast.success(result.message || 'อัปเดตเจ้าหนี้ใน SML แล้ว', {
        description: result.warning || result.sml_update?.log_warning || undefined,
      })
      refreshAll()
    } catch (err) {
      toast.error(
        err instanceof Error && err.message
          ? err.message
          : 'อัปเดตเจ้าหนี้ใน SML ไม่สำเร็จ',
      )
    } finally {
      setCreditorUpdating(false)
    }
  }

  const handleOpenPrintPaymentMethodDialog = async (bill: Bill) => {
    if (paymentDialogLoading) return
    const toastID = toast.loading('กำลังโหลดรายละเอียดบิล...')
    setPaymentDialogLoading(true)
    setPaymentDialogLoadingBillId(bill.id)
    try {
      const fullBill = await getBill(bill.id)
      setPaymentDialogBill(fullBill)
    } catch (err) {
      toast.error(
        err instanceof Error && err.message
          ? err.message
          : 'โหลดรายละเอียดบิลไม่สำเร็จ',
      )
    } finally {
      toast.dismiss(toastID)
      setPaymentDialogLoading(false)
      setPaymentDialogLoadingBillId(null)
    }
  }

  const handlePrintPaymentMethodConfirm = async (paymentMethod: string, applyToEmailGroup: boolean) => {
    if (!paymentDialogBill || paymentUpdating) return
    setPaymentUpdating(true)
    try {
      const result = await updateBillPrintPaymentMethod(paymentDialogBill.id, {
        payment_method: paymentMethod,
        apply_to_email_group: applyToEmailGroup,
      })
      setPaymentDialogBill(null)
      toast.success(result.message || 'อัปเดตวิธีการชำระเงินสำหรับปริ้นแล้ว', {
        description: result.result?.updated_count
          ? `อัปเดต ${result.result.updated_count.toLocaleString('th-TH')} คำสั่งซื้อ`
          : undefined,
      })
      refreshAll()
    } catch (err) {
      toast.error(
        err instanceof Error && err.message
          ? err.message
          : 'อัปเดตวิธีการชำระเงินสำหรับปริ้นไม่สำเร็จ',
      )
    } finally {
      setPaymentUpdating(false)
    }
  }

  const currentPrintFilters = () => ({
    source: activeSource,
    bill_type: config.billType,
    document_route: config.documentRoute,
    email_account_id: emailAccountId === ALL ? '' : emailAccountId,
    shopee_status: showShopeeStatusFilter && shopeeStatus !== ALL ? shopeeStatus : '',
    shopee_shop_id: showShopeeShopFilter && shopeeShopId !== ALL ? shopeeShopId : '',
    archived: archiveMode === 'active' ? '' as const : archiveMode,
    search,
    print_payment_method: activePaymentMethod,
    date_from: dateFrom || undefined,
    date_to: dateTo || undefined,
    sort_order: sortOrder,
  })

  const handlePrintEmailBill = async (bill: Bill) => {
    if (!bill.email_group?.message_id || printLoadingMessageID) return
    setPrintLoadingMessageID(bill.email_group.message_id)
    const toastID = toast.loading('กำลังเตรียมเอกสารพิมพ์...')
    try {
      const fullBill = await getBill(bill.id)
      const artifactsRes = await client.get<{ data: Array<{ id: string; kind: string; filename: string }> }>(`/api/bills/${bill.id}/artifacts`)
      const artifact = (artifactsRes.data.data ?? []).find((a) => a.kind === 'email_html' || a.kind === 'email_text')
      if (!artifact) throw new Error('ไม่พบไฟล์อีเมลสำหรับพิมพ์')
      await recordArtifactPrint(bill.id, artifact.id)
      await printArtifact(bill.id, artifact.id, artifact.filename, printContextFromBill(fullBill))
      toast.success('เปิดหน้าพิมพ์แล้ว')
      if (printReadyOnly) {
        refetch()
      }
      fetchCounts()
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      toast.error(e?.response?.data?.error || e?.message || 'พิมพ์ไม่สำเร็จ')
    } finally {
      toast.dismiss(toastID)
      setPrintLoadingMessageID(null)
    }
  }

  const handleOpenBulkPrint = async () => {
    if (bulkPrintLoading) return
    setBulkPrintLoading(true)
    setBulkPrintOpen(true)
    setBulkPrintTruncated(false)
    try {
      const res = await getEmailPrintCandidates(currentPrintFilters())
      setBulkPrintCandidates(res.data ?? [])
      setBulkPrintTruncated(Boolean(res.truncated))
      if (res.truncated) {
        toast.warning(`พบอีเมลพร้อมพิมพ์เกิน ${res.limit.toLocaleString('th-TH')} ชุด กรุณากรองวันที่หรือช่องทางให้แคบลง`)
      }
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      toast.error(e?.response?.data?.error || e?.message || 'โหลดอีเมลพร้อมพิมพ์ไม่สำเร็จ')
      setBulkPrintOpen(false)
    } finally {
      setBulkPrintLoading(false)
    }
  }

  const handleConfirmBulkPrint = async () => {
    if (bulkPrintLoading || bulkPrintCandidates.length === 0 || bulkPrintTruncated) return
    setBulkPrintLoading(true)
    const toastID = toast.loading('กำลังบันทึกประวัติและเปิดหน้าพิมพ์...')
    try {
      await recordBulkEmailPrintEvents(bulkPrintCandidates.map((candidate) => ({
        bill_id: candidate.artifact_bill_id,
        artifact_id: candidate.artifact_id,
      })))
      await printArtifactsBatch(bulkPrintCandidates.map((candidate) => ({
        billID: candidate.artifact_bill_id,
        artID: candidate.artifact_id,
        filename: candidate.artifact_filename,
        printContext: printContextFromCandidate(candidate),
      })))
      toast.success(`เปิดหน้าพิมพ์ ${bulkPrintCandidates.length.toLocaleString('th-TH')} ชุดแล้ว`)
      setBulkPrintOpen(false)
      setBulkPrintCandidates([])
      setBulkPrintTruncated(false)
      refetch()
      fetchCounts()
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      toast.error(e?.response?.data?.error || e?.message || 'พิมพ์ทั้งหมดไม่สำเร็จ')
    } finally {
      toast.dismiss(toastID)
      setBulkPrintLoading(false)
    }
  }

  useEffect(() => {
    let alive = true
    client.get<{ data: InboxOption[] }>('/api/bills/email-inboxes')
      .then((res) => {
        if (alive) setInboxes(res.data.data ?? [])
      })
      .catch(() => {
        if (alive) setInboxes([])
      })
    client.get<{ data: ShopeeShopOption[] }>('/api/shopee-api/connections')
      .then((res) => {
        if (alive) setShopeeShops((res.data.data ?? []).filter((shop) => !shop.disabled_at))
      })
      .catch(() => {
        if (alive) setShopeeShops([])
      })
    return () => { alive = false }
  }, [])

  useEffect(() => {
    const timer = window.setTimeout(() => {
      const next = searchInput.trim()
      if (next !== search) {
        setSearch(next)
        setPage(1)
      }
    }, 300)
    return () => window.clearTimeout(timer)
  }, [searchInput, search])

  useEffect(() => {
    fetchCounts().catch(() => {
      setCounts({ needs_review: 0, pending: 0, sent: 0, failed: 0, skipped: 0, total: 0, print_ready_orders: 0, print_ready_groups: 0 })
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeSource, activePaymentMethod, config.billType, config.documentRoute, emailAccountId, archiveMode, shopeeStatus, shopeeShopId, search, printReadyOnly, dateFrom, dateTo])

  useEffect(() => {
    if (!loading && data && page > totalPages) {
      setPage(totalPages)
    }
  }, [data, loading, page, totalPages])

  useEffect(() => {
    setPageJumpInput(String(page))
  }, [page])

  useEffect(() => {
    const currentSearch = searchParams.toString()
    if (currentSearch === lastSearchStringRef.current) return
    lastSearchStringRef.current = currentSearch
    syncingFromURLRef.current = true

    const nextStatus = readURLFilter(searchParams, 'status', VALID_STATUSES)
    const nextShopeeStatus = readURLFilter(searchParams, 'shopee_status', VALID_SHOPEE_STATUSES)
    const nextSource = readURLFilter(searchParams, 'source', VALID_PURCHASE_SOURCES)
    const nextSearchInput = searchParams.get('search') ?? ''
    const nextPaymentMethod = searchParams.get('print_payment_method')?.trim() || ALL
    const nextArchive = readURLArchive(searchParams)
    const nextSortOrder = searchParams.get('sort_order') === 'asc' ? 'asc' : 'desc'
    const nextDateFrom = searchParams.get('date_from') ?? ''
    const nextDateTo = searchParams.get('date_to') ?? ''

    setStatus((prev) => (prev === nextStatus ? prev : nextStatus))
    setShopeeStatus((prev) => (prev === nextShopeeStatus ? prev : nextShopeeStatus))
    setSourceFilter((prev) => (prev === nextSource ? prev : nextSource))
    setPrintReadyOnly((prev) => (prev === (searchParams.get('print_ready') === '1') ? prev : searchParams.get('print_ready') === '1'))
    setShopeeShopId((prev) => (prev === (searchParams.get('shopee_shop_id') || ALL) ? prev : searchParams.get('shopee_shop_id') || ALL))
    setEmailAccountId((prev) => (prev === (searchParams.get('email_account_id') || ALL) ? prev : searchParams.get('email_account_id') || ALL))
    setSearchInput((prev) => (prev === nextSearchInput ? prev : nextSearchInput))
    setSearch((prev) => (prev === nextSearchInput.trim() ? prev : nextSearchInput.trim()))
    setPaymentMethodFilter((prev) => (prev === nextPaymentMethod ? prev : nextPaymentMethod))
    setArchiveMode((prev) => (prev === nextArchive ? prev : nextArchive))
    setSortOrder((prev) => (prev === nextSortOrder ? prev : nextSortOrder))
    setDateFrom((prev) => (prev === nextDateFrom ? prev : nextDateFrom))
    setDateTo((prev) => (prev === nextDateTo ? prev : nextDateTo))
    setPage((prev) => {
      const nextPage = readURLPage(searchParams)
      return prev === nextPage ? prev : nextPage
    })
    setPerPage((prev) => {
      const nextPerPage = readURLPerPage(searchParams)
      return prev === nextPerPage ? prev : nextPerPage
    })
  }, [searchParams])

  useEffect(() => {
    if (syncingFromURLRef.current) {
      syncingFromURLRef.current = false
      return
    }
    const next = new URLSearchParams(searchParams)
    if (status === ALL) next.delete('status')
    else next.set('status', status)
    if (showShopeeStatusFilter && shopeeStatus !== ALL) next.set('shopee_status', shopeeStatus)
    else next.delete('shopee_status')
    if (showPurchaseSourceFilter && sourceFilter !== ALL) next.set('source', sourceFilter)
    else next.delete('source')
    if (showPurchaseSourceFilter && printReadyOnly) next.set('print_ready', '1')
    else next.delete('print_ready')
    if (showPurchaseSourceFilter && activePaymentMethod) next.set('print_payment_method', activePaymentMethod)
    else next.delete('print_payment_method')
    if (showShopeeShopFilter && shopeeShopId !== ALL) next.set('shopee_shop_id', shopeeShopId)
    else next.delete('shopee_shop_id')
    if (archiveMode === 'active') next.delete('archived')
    else next.set('archived', archiveMode)
    if (emailAccountId === ALL) next.delete('email_account_id')
    else next.set('email_account_id', emailAccountId)
    if (search) next.set('search', search)
    else next.delete('search')
    if (sortOrder === 'asc') next.set('sort_order', 'asc')
    else next.delete('sort_order')
    if (dateFrom) next.set('date_from', dateFrom)
    else next.delete('date_from')
    if (dateTo) next.set('date_to', dateTo)
    else next.delete('date_to')
    if (page > 1) next.set('page', String(page))
    else next.delete('page')
    if (perPage !== DEFAULT_PER_PAGE) next.set('per_page', String(perPage))
    else next.delete('per_page')
    const nextString = next.toString()
    if (nextString !== searchParams.toString()) {
      lastSearchStringRef.current = nextString
      setSearchParams(next, { replace: true })
    }
  }, [
    status,
    shopeeStatus,
    sourceFilter,
    printReadyOnly,
    activePaymentMethod,
    archiveMode,
    emailAccountId,
    search,
    page,
    perPage,
    showShopeeStatusFilter,
    showPurchaseSourceFilter,
    showShopeeShopFilter,
    shopeeShopId,
    sortOrder,
    dateFrom,
    dateTo,
    searchParams,
    setSearchParams,
  ])

  return (
    <div className="space-y-3">
      <div className="overflow-hidden rounded-xl border border-border/70 bg-card shadow-sm">
        <div className="border-b border-border/70 bg-muted/20 px-3 py-2">
          <div className="flex flex-col gap-2 xl:flex-row xl:items-center xl:justify-between">
            <QueueSummaryStrip counts={counts} />
            <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-muted-foreground xl:justify-end">
              <span>{config.routeLabel}</span>
              <span>→</span>
              <span className="font-medium text-foreground">{config.destination}</span>
              <code className="rounded bg-background px-1.5 py-0.5 font-mono text-[10px] text-foreground">{config.docCode}</code>
              <Link to="/settings/channels" className="font-medium text-primary hover:underline">
                ตั้งค่า
              </Link>
            </div>
          </div>
        </div>

        <div className="space-y-2.5 p-3">
          <div className="flex flex-col gap-2 xl:flex-row xl:items-start xl:justify-between">
            <div className="min-w-0 space-y-1">
              <div className="text-[11px] font-medium text-muted-foreground">สถานะบิล</div>
              <SegmentedPillGroup
                options={STATUS_OPTIONS}
                value={status}
                onChange={(value) => resetPage(() => setStatus(value))}
                ariaLabel="กรองตามสถานะบิล"
              />
            </div>
            <div className="min-w-0 space-y-1 xl:max-w-[420px]">
              <div className="text-[11px] font-medium text-muted-foreground">มุมมอง</div>
              <SegmentedPillGroup
                options={ARCHIVE_OPTIONS}
                value={archiveMode}
                onChange={(value) => resetPage(() => setArchiveMode(value))}
                ariaLabel="กรองตามมุมมองบิลที่เก็บแล้ว"
              />
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <div className="relative min-w-[240px] flex-1">
              <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder={config.searchPlaceholder}
                value={searchInput}
                onChange={(e) => setSearchInput(e.target.value)}
                className="h-9 pl-8"
              />
            </div>

            {mode !== 'purchase-order' && (
              <span className="h-9 w-full rounded-md border border-border bg-background px-2.5 py-2 text-xs text-muted-foreground sm:w-auto">
                {(config.sourceLabel ?? BILL_SOURCE_LABEL[config.source])} · {BILL_TYPE_LABEL[config.billType]}
              </span>
            )}
            {showPurchaseSourceFilter && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-9 w-full justify-start gap-1.5 sm:w-[210px]"
                onClick={() => setSourceFilterOpen(true)}
                title="กรองตามช่องทางบิลซื้อ"
              >
                <Filter className="h-3.5 w-3.5" />
                <span className="truncate">{activeSourceOption.label}</span>
              </Button>
            )}
            {showPurchaseSourceFilter && (
              <select
                value={paymentMethodFilter}
                onChange={(e) => resetPage(() => setPaymentMethodFilter(e.target.value))}
                className="h-9 w-full min-w-0 rounded-md border border-border bg-background px-2.5 text-xs text-foreground sm:w-[210px]"
                aria-label="กรองตามวิธีชำระเงิน BillFlow"
                title="กรองตามวิธีชำระเงินที่ BillFlow ใช้สำหรับส่ง/ปริ้น"
              >
                {paymentMethodOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            )}
            {showShopeeStatusFilter && (
              <select
                value={shopeeStatus}
                onChange={(e) => resetPage(() => setShopeeStatus(e.target.value))}
                className="h-9 w-full min-w-0 rounded-md border border-border bg-background px-2.5 text-xs text-foreground sm:w-[210px]"
                aria-label="กรองตามสถานะคำสั่งซื้อ Shopee"
              >
                {SHOPEE_STATUS_OPTIONS.map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </select>
            )}
            <div className="w-full sm:w-[250px]">
              <DateRangePicker
                from={dateFrom}
                to={dateTo}
                onFromChange={(v) => resetPage(() => setDateFrom(v))}
                onToChange={(v) => resetPage(() => setDateTo(v))}
                className="h-9 w-full min-w-0 text-xs"
              />
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-9 gap-1.5 whitespace-nowrap"
              onClick={() => resetPage(() => setSortOrder((s) => (s === 'desc' ? 'asc' : 'desc')))}
              title={sortOrder === 'desc' ? 'เรียงจากใหม่ → เก่า (คลิกเพื่อสลับ)' : 'เรียงจากเก่า → ใหม่ (คลิกเพื่อสลับ)'}
            >
              <ArrowDownUp className="h-3.5 w-3.5" />
              {sortOrder === 'desc' ? 'ใหม่สุด' : 'เก่าสุด'}
            </Button>

            {showPurchaseSourceFilter && (
              <Button
                type="button"
                variant={printReadyOnly ? 'default' : 'outline'}
                size="sm"
                className="h-9 gap-1.5 whitespace-nowrap"
                onClick={() => resetPage(() => setPrintReadyOnly((v) => !v))}
                title="แสดงเฉพาะอีเมลที่ส่ง SML ครบ และยังไม่เคยบันทึกพิมพ์"
              >
                <Printer className="h-3.5 w-3.5" />
                รอพิมพ์อีเมล
                <span className={printReadyOnly ? 'text-primary-foreground/80' : 'text-muted-foreground'}>
                  {counts.print_ready_groups.toLocaleString('th-TH')}
                </span>
              </Button>
            )}

            {showShopeeShopFilter && shopeeShops.length > 0 && (
              <label className="flex h-9 w-full items-center gap-1.5 rounded-md border border-border bg-background px-2 text-xs text-muted-foreground sm:w-[260px]">
                <Store className="h-3.5 w-3.5 text-primary" />
                <select
                  value={shopeeShopId}
                  onChange={(e) => resetPage(() => setShopeeShopId(e.target.value))}
                  className="h-full min-w-0 flex-1 bg-transparent text-xs text-foreground outline-none"
                  aria-label="กรองตามร้าน Shopee"
                >
                  <option value={ALL}>ทุกร้าน Shopee</option>
                  {shopeeShops.map((shop) => (
                    <option key={shop.id} value={String(shop.shop_id)}>
                      {shop.label || shop.shop_name || 'Shopee shop'} · {shop.shop_id}
                    </option>
                  ))}
                </select>
              </label>
            )}
            {inboxes.length > 0 && config.routeTo === '/settings/email' && (
              <select
                value={emailAccountId}
                onChange={(e) => resetPage(() => setEmailAccountId(e.target.value))}
                className="h-9 w-full min-w-0 rounded-md border border-border bg-background px-2.5 text-xs text-foreground sm:w-[280px]"
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

            <div className="min-w-0 flex-1 truncate rounded-md bg-muted/45 px-2.5 py-2 text-[11px] text-muted-foreground">
              {activeFilterSummary.join(' · ')}
            </div>
          </div>

          <div className="flex flex-col gap-2 border-t border-border/70 pt-2 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-[11px] text-muted-foreground">
              {showPurchaseSourceFilter
                ? `อีเมลพร้อมพิมพ์ ${counts.print_ready_groups.toLocaleString('th-TH')} ชุด (${counts.print_ready_orders.toLocaleString('th-TH')} คำสั่งซื้อ) · ${printReadyNoteForList(bills)}`
                : counts.needs_review > 0 && archiveMode === 'active'
                  ? `ต้องตรวจสินค้า ${counts.needs_review.toLocaleString()} รายการ ไม่ถูกรวมใน bulk send`
                  : 'พร้อมทำงานจากตัวกรองปัจจุบัน'}
            </p>
            <div className="flex w-full flex-col gap-2 sm:ml-auto sm:w-auto sm:flex-row sm:items-center sm:justify-end">
              {showPurchaseSourceFilter && (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  className="w-full gap-1.5 whitespace-nowrap sm:w-auto"
                  disabled={archiveMode !== 'active' || counts.print_ready_groups === 0 || bulkPrintLoading}
                  onClick={handleOpenBulkPrint}
                  title="พิมพ์เฉพาะอีเมลที่ส่ง SML ครบ และยังไม่เคยบันทึกพิมพ์จาก filter ปัจจุบัน"
                >
                  <Printer className="h-3.5 w-3.5" />
                  พิมพ์อีเมลที่พร้อม
                </Button>
              )}
              <Button
                type="button"
                size="sm"
                className="w-full gap-1.5 whitespace-nowrap sm:w-auto"
                disabled={bulkDisabled}
                onClick={() => setBulkOpen(true)}
                title={
                  !canBulkSend
                    ? 'ส่ง SML แบบกลุ่มใช้ได้เฉพาะผู้ดูแลระบบ'
                    : archiveMode !== 'active'
                    ? 'Bulk send ปิดไว้เมื่อดูบิลที่เก็บแล้ว เพื่อไม่ส่งเอกสารย้อนหลังโดยไม่ตั้งใจ'
                    : !bulkSourceAllowed
                      ? 'เลือก Email บิลซื้อ Shopee หรือ Email บิลซื้อ Lazada ก่อน เพื่อไม่ส่งข้ามช่องทางโดยไม่ตั้งใจ'
                    : !bulkStatusAllowed
                      ? 'Bulk send ส่งเฉพาะเอกสารสถานะพร้อมส่ง จึงเปิดได้เมื่อเลือกทุกสถานะหรือสถานะพร้อมส่ง'
                    : counts.needs_review > 0
                      ? `มีรายการต้องตรวจสินค้า ${counts.needs_review.toLocaleString()} รายการ ปุ่มนี้ส่งเฉพาะเอกสารสถานะพร้อมส่ง`
                      : undefined
                }
              >
                <Send className="h-3.5 w-3.5" />
                <span className="truncate">{bulkButtonLabel}</span>
              </Button>
            </div>
          </div>
        </div>
      </div>

      {!loading && bills.length === 0 && !search && status === ALL && shopeeStatus === ALL && (!showPurchaseSourceFilter || sourceFilter === ALL) && archiveMode === 'active' ? (
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
          bills={bills}
          loading={loading}
          showShopeeStatusColumn={showShopeeStatusFilter}
          canUseOperationalActions={canUseBillActions}
          canManageLifecycle={canManageBillLifecycle}
          canPermanentDelete={canPermanentDelete}
          canUpdatePurchaseCreditor={canUpdatePurchaseCreditor}
          purchaseCreditorLoadingBillId={creditorDialogLoadingBillId}
          canUpdatePrintPaymentMethod={canUpdatePrintPaymentMethod}
          printPaymentMethodLoadingBillId={paymentDialogLoadingBillId}
          printLoadingMessageID={printLoadingMessageID}
          virtualize={perPage >= 100}
          onArchive={(bill: Bill) => setConfirmAction({ kind: 'archive', bill })}
          onRestore={(bill: Bill) => setConfirmAction({ kind: 'restore', bill })}
          onDelete={(bill: Bill) => setConfirmAction({ kind: 'delete', bill })}
          onPermanentDelete={(bill: Bill) => setConfirmAction({ kind: 'permanent', bill })}
          onUpdatePurchaseCreditor={handleOpenPurchaseCreditorDialog}
          onUpdatePrintPaymentMethod={handleOpenPrintPaymentMethodDialog}
          onPrintEmail={handlePrintEmailBill}
          onRowClick={handleOpenBillDetail}
        />
      )}

      <div className="flex flex-col gap-2 text-xs text-muted-foreground lg:flex-row lg:items-center lg:justify-between">
        <span>
          {total > 0
            ? `แสดง ${pageStart.toLocaleString()}-${pageEnd.toLocaleString()} จาก ${total.toLocaleString()} รายการ`
            : `แสดง ${bills.length.toLocaleString()} รายการ`}
        </span>
        <div className="flex flex-wrap items-center gap-2 lg:justify-end">
          <label className="inline-flex items-center gap-1.5">
            <span>ต่อหน้า</span>
            <select
              value={String(perPage)}
              onChange={(e) => handlePerPageChange(e.target.value)}
              className="h-8 rounded-md border border-border bg-background px-2 text-xs text-foreground"
              aria-label="จำนวนรายการต่อหน้า"
            >
              {PAGE_SIZE_OPTIONS.map((size) => (
                <option key={size} value={size}>
                  {size}
                </option>
              ))}
            </select>
          </label>
          <Button
            variant="outline"
            size="sm"
            disabled={!hasPreviousPage}
            onClick={() => setPage(1)}
          >
            หน้าแรก
          </Button>
          <Button
            variant="outline"
            size="sm"
            disabled={!hasPreviousPage}
            onClick={() => setPage((current) => Math.max(1, current - 1))}
          >
            <ChevronLeft className="h-3.5 w-3.5" />
            ก่อนหน้า
          </Button>
          <span className="min-w-[92px] text-center tabular-nums">
            หน้า {page.toLocaleString()} / {totalPages.toLocaleString()}
          </span>
          <form className="inline-flex items-center gap-1.5" onSubmit={handleJumpToPage}>
            <span>ไปหน้า</span>
            <Input
              type="number"
              inputMode="numeric"
              min={1}
              max={totalPages}
              value={pageJumpInput}
              onChange={(e) => setPageJumpInput(e.target.value)}
              className="h-8 w-20 px-2 text-center text-xs tabular-nums"
              aria-label="ไปหน้าที่"
            />
            <Button type="submit" variant="outline" size="sm" disabled={totalPages <= 1}>
              ไป
            </Button>
          </form>
          <Button
            variant="outline"
            size="sm"
            disabled={!hasNextPage}
            onClick={() => setPage((current) => Math.min(totalPages, current + 1))}
          >
            ถัดไป
            <ChevronRight className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      <BulkSendDialog
        open={bulkOpen}
        onOpenChange={setBulkOpen}
        title={config.title}
        billType={config.billType}
        filters={{
          source: activeSource,
          bill_type: config.billType,
          document_route: config.documentRoute,
          email_account_id: emailAccountId === ALL ? '' : emailAccountId,
          shopee_status: showShopeeStatusFilter && shopeeStatus !== ALL ? shopeeStatus : '',
          shopee_shop_id: showShopeeShopFilter && shopeeShopId !== ALL ? shopeeShopId : '',
          search,
          print_payment_method: activePaymentMethod,
        }}
        onDone={() => {
          setPage(1)
          refetch()
          fetchCounts()
        }}
      />

      <UpdatePurchaseCreditorDialog
        open={!!creditorDialogBill}
        bill={creditorDialogBill}
        submitting={creditorUpdating}
        onOpenChange={(open) => !open && setCreditorDialogBill(null)}
        onConfirm={handlePurchaseCreditorConfirm}
      />

      <UpdatePrintPaymentMethodDialog
        open={!!paymentDialogBill}
        bill={paymentDialogBill}
        submitting={paymentUpdating}
        onOpenChange={(open) => !open && setPaymentDialogBill(null)}
        onConfirm={handlePrintPaymentMethodConfirm}
      />

      <Dialog open={bulkPrintOpen} onOpenChange={(open) => {
        if (!open && !bulkPrintLoading) {
          setBulkPrintOpen(false)
          setBulkPrintCandidates([])
          setBulkPrintTruncated(false)
        }
      }}>
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>พิมพ์อีเมลที่พร้อม</DialogTitle>
            <DialogDescription>
              ตรวจเลข POL และวิธีการชำระเงินก่อนยืนยัน ระบบจะพิมพ์เฉพาะอีเมลที่ส่ง SML ครบและยังไม่เคยบันทึกพิมพ์
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            <div className="rounded-md border border-info/30 bg-info/5 px-3 py-2 text-xs text-info">
              {printReadyNoteForList(bills)}
            </div>
            {bulkPrintTruncated && (
              <div className="rounded-md border border-warning/35 bg-warning/10 px-3 py-2 text-xs text-warning">
                พบอีเมลพร้อมพิมพ์เกิน 100 ชุด กรุณากรองวันที่หรือช่องทางให้แคบลงก่อนพิมพ์ทั้งหมด เพื่อไม่พลาดบางชุดอีเมล
              </div>
            )}
            <div className="max-h-[420px] overflow-y-auto rounded-md border border-border">
              {bulkPrintLoading && bulkPrintCandidates.length === 0 ? (
                <div className="p-4 text-sm text-muted-foreground">กำลังโหลดอีเมลพร้อมพิมพ์...</div>
              ) : bulkPrintCandidates.length === 0 ? (
                <div className="p-4 text-sm text-muted-foreground">ไม่มีอีเมลที่พร้อมพิมพ์จาก filter ปัจจุบัน</div>
              ) : (
                <div className="divide-y divide-border">
                  {bulkPrintCandidates.map((candidate) => (
                    <div key={candidate.message_id} className="space-y-2 p-3">
                      <div className="flex items-center justify-between gap-2">
                        <div className="min-w-0">
                          <div className="truncate text-sm font-medium text-foreground">
                            Email #{candidate.group_key}
                          </div>
                          <div className="truncate text-[11px] text-muted-foreground">
                            {candidate.subject || candidate.from || candidate.message_id}
                          </div>
                        </div>
                        <span className="shrink-0 text-xs text-muted-foreground">
                          {candidate.orders.length.toLocaleString('th-TH')} order
                        </span>
                      </div>
                      <div className="space-y-1">
                        {candidate.orders.map((order) => (
                          <div key={order.bill_id} className="flex flex-wrap items-center justify-between gap-2 rounded bg-muted/35 px-2 py-1 text-xs">
                            <span className="font-mono text-foreground">{order.sml_doc_no}</span>
                            <span className="min-w-0 truncate text-muted-foreground">
                              {formatPrintPaymentMethod(order.effective_print_payment_method || order.print_payment_method)}
                            </span>
                          </div>
                        ))}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
            <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
              <Button
                type="button"
                variant="outline"
                disabled={bulkPrintLoading}
                onClick={() => {
                  setBulkPrintOpen(false)
                  setBulkPrintCandidates([])
                  setBulkPrintTruncated(false)
                }}
              >
                ยกเลิก
              </Button>
              <Button
                type="button"
                disabled={bulkPrintLoading || bulkPrintCandidates.length === 0 || bulkPrintTruncated}
                onClick={handleConfirmBulkPrint}
              >
                {bulkPrintLoading ? 'กำลังพิมพ์...' : `ยืนยันพิมพ์ ${bulkPrintCandidates.length.toLocaleString('th-TH')} ชุด`}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={sourceFilterOpen} onOpenChange={setSourceFilterOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>กรองตามช่องทาง</DialogTitle>
            <DialogDescription>
              เลือกช่องทางของบิลซื้อที่ต้องการดูและส่งเข้า SML
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            {PURCHASE_SOURCE_OPTIONS.map((option) => {
              const selected = sourceFilter === option.value
              return (
                <button
                  key={option.value}
                  type="button"
                  className={[
                    'flex w-full items-start justify-between gap-3 rounded-lg border p-3 text-left transition-colors',
                    selected
                      ? 'border-primary bg-primary/10 text-foreground'
                      : 'border-border bg-background text-foreground hover:bg-accent/70',
                  ].join(' ')}
                  onClick={() => {
                    resetPage(() => setSourceFilter(option.value))
                    setSourceFilterOpen(false)
                  }}
                >
                  <span className="min-w-0 space-y-1">
                    {option.value === ALL ? (
                      <span className="inline-flex w-fit items-center gap-1.5 rounded-full border border-border bg-muted px-2 py-1 text-xs font-medium text-muted-foreground">
                        <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/50" aria-hidden="true" />
                        {option.label}
                      </span>
                    ) : (
                      <BillSourceBadge source={option.value} />
                    )}
                    <span className="block text-xs leading-snug text-muted-foreground">
                      {option.description}
                    </span>
                  </span>
                  <span
                    className={[
                      'mt-1 h-2.5 w-2.5 shrink-0 rounded-full border',
                      selected ? 'border-primary bg-primary' : 'border-muted-foreground/40',
                    ].join(' ')}
                    aria-hidden="true"
                  />
                </button>
              )
            })}
          </div>
        </DialogContent>
      </Dialog>

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

function printContextFromBill(bill: Bill): ArtifactPrintContext {
  const related = bill.email_group?.related_bills ?? []
  const rows = related.length > 0
    ? related
    : [{
        order_id: String(bill.raw_data?.order_id ?? bill.raw_data?.shopee_order_id ?? ''),
        sml_doc_no: bill.sml_doc_no || '',
        party_code: payloadString(bill.sml_payload, 'cust_code'),
        party_name: payloadString(bill.sml_payload, 'supplier_name') || payloadString(bill.sml_payload, 'party_name'),
        print_payment_method: bill.print_payment_method || '',
        effective_print_payment_method: bill.effective_print_payment_method || '',
      }]
  return {
    orders: rows.map((row) => ({
      orderId: row.order_id || undefined,
      smlDocNo: row.sml_doc_no || undefined,
      partyCode: row.party_code || undefined,
      partyName: row.party_name || undefined,
      paymentMethod: row.effective_print_payment_method || row.print_payment_method || undefined,
    })),
  }
}

function printContextFromCandidate(candidate: EmailPrintCandidate): ArtifactPrintContext {
  return {
    orders: candidate.orders.map((order) => ({
      orderId: order.order_id || undefined,
      smlDocNo: order.sml_doc_no || undefined,
      partyCode: order.party_code || undefined,
      partyName: order.party_name || undefined,
      paymentMethod: order.effective_print_payment_method || order.print_payment_method || undefined,
    })),
  }
}

function payloadString(payload: Record<string, unknown> | null | undefined, key: string): string {
  const value = payload?.[key]
  return typeof value === 'string' ? value.trim() : ''
}

function effectivePrintPaymentMethodFromBill(bill: Bill): string {
  return (bill.effective_print_payment_method || bill.print_payment_method || '').trim()
}

function formatPrintPaymentMethod(method?: string): string {
  return (method ?? '').trim() || 'ยังไม่ได้เลือกวิธีชำระเงิน'
}

function printReadyNoteForList(bills: Bill[]): string {
  const policyNote = bills.find((bill) => bill.email_group?.print_policy_note)?.email_group?.print_policy_note
  if (policyNote) return `${policyNote} และยังไม่เคยบันทึกพิมพ์`
  return 'พร้อมพิมพ์ = ส่งเข้า SML ครบทุกคำสั่งซื้อในอีเมลเดียวกัน วิธีการชำระเงินขึ้นต้นด้วย TT และยังไม่เคยบันทึกพิมพ์'
}

function QueueSummaryStrip({
  counts,
}: {
  counts: {
    needs_review: number
    pending: number
    sent: number
    failed: number
  }
}) {
  return (
    <div className="grid w-full grid-cols-2 gap-1.5 lg:grid-cols-4 xl:max-w-[720px]">
      <QueueMetric label="ต้องตรวจสินค้า" value={counts.needs_review} icon={AlertTriangle} tone="warning" />
      <QueueMetric label="พร้อมส่ง" value={counts.pending} icon={Clock} tone="primary" />
      <QueueMetric label="ส่งแล้ว" value={counts.sent} icon={CheckCircle2} tone="success" />
      <QueueMetric label="ส่งไม่สำเร็จ" value={counts.failed} icon={Send} tone="danger" />
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
    <div className="flex min-w-0 items-center gap-2 rounded-lg border border-border/70 bg-background px-2.5 py-1.5">
      <div className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-md ${toneCls}`}>
        <Icon className="h-3.5 w-3.5" />
      </div>
      <div className="min-w-0">
        <p className="text-base font-semibold leading-5 tabular-nums text-foreground">{value.toLocaleString()}</p>
        <p className="truncate text-[11px] leading-4 text-muted-foreground">{label}</p>
      </div>
    </div>
  )
}

function SegmentedPillGroup<T extends string>({
  options,
  value,
  onChange,
  ariaLabel,
}: {
  options: readonly { value: T; label: string }[]
  value: T
  onChange: (value: T) => void
  ariaLabel: string
}) {
  return (
    <div
      className="inline-flex max-w-full flex-wrap gap-1 rounded-lg border border-border bg-muted/40 p-1"
      role="group"
      aria-label={ariaLabel}
    >
      {options.map((option) => {
        const selected = value === option.value
        return (
          <button
            key={option.value}
            type="button"
            onClick={() => onChange(option.value)}
            className={[
              'rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
              selected
                ? 'bg-primary text-primary-foreground shadow-sm'
                : 'text-muted-foreground hover:bg-background hover:text-foreground',
            ].join(' ')}
            aria-pressed={selected}
          >
            {option.label}
          </button>
        )
      })}
    </div>
  )
}
