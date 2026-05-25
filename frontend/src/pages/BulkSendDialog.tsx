import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { AlertTriangle, CheckCircle2, Clipboard, ExternalLink, Loader2, RotateCcw, Send } from 'lucide-react'
import { toast } from 'sonner'

import client from '@/api/client'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { PartyPicker, type Party } from '@/pages/ChannelDefaults/PartyPicker'
import { ShelfPicker, WarehousePicker } from '@/pages/BillDetail/components/WarehousePicker'
import { REMARK2_NONE, SML_REMARK2_OPTIONS, normalizeRemark2, remark2Label, remark2PayloadValue } from '@/lib/smlRemark2'
import {
  createBulkSendJob,
  getActiveBulkSendJob,
  getBill,
  getBulkSendJob,
  retryFailedBulkSendJob,
  type BulkSendJob,
  type RetryBillPayload,
} from '@/hooks/useBills'
import type { Bill } from '@/types'
import { validateForSML, issueLabel } from '@/pages/BillDetail/utils/validation'

const BULK_BATCH_SIZE = 100

function currentTimeHHMM() {
  const now = new Date()
  return `${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}`
}

function errorMessage(err: unknown) {
  const maybe = err as { response?: { data?: { error?: string } }; message?: string }
  return maybe.response?.data?.error ?? maybe.message ?? 'ส่งไม่สำเร็จ'
}

function isActiveJob(job: BulkSendJob | null) {
  return job?.status === 'queued' || job?.status === 'running'
}

function shouldRestoreJob(job: BulkSendJob | null) {
  if (!job) return false
  if (isActiveJob(job)) return true
  const updated = new Date(job.updated_at || job.finished_at || job.created_at).getTime()
  return Number.isFinite(updated) && Date.now() - updated < 30 * 60 * 1000
}

function jobItemResult(status?: string): 'sent' | 'failed' | 'skipped' | undefined {
  if (status === 'sent') return 'sent'
  if (status === 'failed') return 'failed'
  if (status === 'skipped') return 'skipped'
  return undefined
}

function newClientRequestID() {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID()
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function sourceOrderNo(bill: Bill) {
  const raw = bill.raw_data ?? {}
  const value =
    bill.sml_order_id ||
    raw.order_id ||
    raw.shopee_order_id ||
    raw.lazada_order_id ||
    raw.tiktok_order_id
  return typeof value === 'string' && value.trim() ? value.trim() : bill.id.slice(0, 8)
}

function previewDocNo(bill: Bill) {
  return bill.sml_doc_no || bill.preview?.doc_no || ''
}

function incrementTrailingNumber(value: string, offset: number) {
  if (!value || offset <= 0) return value
  const match = value.match(/^(.*?)(\d+)(\D*)$/)
  if (!match) return value
  const [, prefix, digits, suffix] = match
  const next = String(Number(digits) + offset).padStart(digits.length, '0')
  return `${prefix}${next}${suffix}`
}

function vatTypeLabel(value: string) {
  if (value === '0') return 'แยกนอก'
  if (value === '1') return 'รวมใน'
  if (value === '2') return 'ศูนย์%'
  return 'ยังไม่เลือก'
}

const PURCHASE_INQUIRY_TYPE_OPTIONS = [
  { value: '0', label: '0 — ซื้อสินค้าเงินเชื่อ' },
  { value: '1', label: '1 — ซื้อสินค้าเงินสด' },
  { value: '3', label: '3 — ซื้อสินค้าเงินสด (สินค้าบริการ)' },
  { value: '4', label: '4 — ซื้อสินค้าเงินเชื่อ (สินค้าบริการ)' },
]

function billDetailPath(bill: Bill) {
  if (bill.bill_type !== 'sale') return `/bills/${bill.id}`
  const route = bill.document_route || bill.preview?.route
  return route === 'saleinvoice' ? `/sale-invoices/${bill.id}` : `/sales-orders/${bill.id}`
}

function bulkJobStorageKey(filters: Props['filters']) {
  return [
    'billflow',
    'bulk-sml-job',
    filters.source || '',
    filters.bill_type || '',
    filters.document_route || '',
    filters.shopee_shop_id || '',
  ].join(':')
}

function jobToCandidates(job: BulkSendJob): Candidate[] {
  return (job.items ?? []).map((item) => ({
    bill: {
      id: item.bill_id,
      bill_type: job.bill_type,
      source: job.source,
      document_route: job.document_route,
      status: item.status === 'sent' ? 'sent' : 'pending',
      sml_doc_no: item.doc_no || item.doc_no_attempted || undefined,
      created_at: item.created_at,
    } as Bill,
    ready: true,
    issues: [],
    result: jobItemResult(item.status),
    message: item.error,
  }))
}

type Candidate = {
  bill: Bill
  ready: boolean
  issues: string[]
  result?: 'sent' | 'failed' | 'skipped'
  message?: string
}

type DisplayRow = Candidate & {
  sequence: number | null
  orderNo: string
  docNo: string
  jobStatus?: string
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  billType: 'purchase' | 'sale'
  filters: {
    source: string
    bill_type: 'purchase' | 'sale'
    document_route?: string
    email_account_id?: string
    shopee_status?: string
    shopee_shop_id?: string
    search?: string
  }
  onDone?: () => void
}

export function BulkSendDialog({
  open,
  onOpenChange,
  title,
  billType,
  filters,
  onDone,
}: Props) {
  const [loading, setLoading] = useState(false)
  const [sending, setSending] = useState(false)
  const [party, setParty] = useState<Party | null>(null)
  const [docTime, setDocTime] = useState(currentTimeHHMM())
  const [whCode, setWhCode] = useState('')
  const [shelfCode, setShelfCode] = useState('')
  const [manualWarehouse, setManualWarehouse] = useState(false)
  const [vatTypeStr, setVatTypeStr] = useState('')
  const [vatRateStr, setVatRateStr] = useState('7')
  const [inquiryTypeStr, setInquiryTypeStr] = useState('')
  const [remark2Str, setRemark2Str] = useState(REMARK2_NONE)
  const [branchCode, setBranchCode] = useState('')
  const [saleCode, setSaleCode] = useState('')
  const [remark, setRemark] = useState('')
  const [candidates, setCandidates] = useState<Candidate[]>([])
  const [totalPending, setTotalPending] = useState(0)
  const [job, setJob] = useState<BulkSendJob | null>(null)
  const [jobError, setJobError] = useState('')
  const storageKey = useMemo(() => bulkJobStorageKey(filters), [filters.source, filters.bill_type, filters.document_route, filters.shopee_shop_id])

  const readyCount = candidates.filter((c) => c.ready).length
  const skippedCount = candidates.length - readyCount
  const activeJob = isActiveJob(job)
  const controlsLocked = sending || activeJob
  const jobItems = job?.items ?? []
  const jobItemByBillID = useMemo(() => {
    const map = new Map<string, NonNullable<BulkSendJob['items']>[number]>()
    jobItems.forEach((item) => map.set(item.bill_id, item))
    return map
  }, [jobItems])
  const sentCount = job ? job.sent_count : candidates.filter((c) => c.result === 'sent').length
  const failedCount = job ? job.failed_count : candidates.filter((c) => c.result === 'failed').length
  const jobSkippedCount = job ? job.skipped_count : candidates.filter((c) => c.result === 'skipped').length
  const remainingCount = job ? Math.max(job.total_count - sentCount - failedCount - jobSkippedCount, 0) : readyCount
  const progressTotal = job?.total_count ?? readyCount
  const progressDone = job ? sentCount + failedCount + jobSkippedCount : 0
  const progressPct = progressTotal > 0 ? Math.round((progressDone / progressTotal) * 100) : 0
  const displayRows = useMemo(() => {
    const previewOffsets = new Map<string, number>()
    let readySeq = 0
    return candidates.map((row) => {
      const jobItem = jobItemByBillID.get(row.bill.id)
      const baseDocNo = previewDocNo(row.bill)
      let docNo = ''
      let sequence: number | null = null

      if (jobItem) {
        sequence = jobItem.sequence
      } else if (row.ready) {
        readySeq += 1
        sequence = readySeq
      }

      if (jobItem?.doc_no) {
        docNo = jobItem.doc_no
      } else if (jobItem?.doc_no_attempted) {
        docNo = jobItem.doc_no_attempted
      } else if (row.bill.sml_doc_no) {
        docNo = row.bill.sml_doc_no
      } else if (row.ready && baseDocNo) {
        const offset = previewOffsets.get(baseDocNo) ?? 0
        docNo = incrementTrailingNumber(baseDocNo, offset)
        previewOffsets.set(baseDocNo, offset + 1)
      }

      return {
        ...row,
        result: jobItemResult(jobItem?.status) ?? row.result,
        message: jobItem?.error || row.message,
        sequence,
        orderNo: jobItem?.order_no || sourceOrderNo(row.bill),
        docNo,
        jobStatus: jobItem?.status,
      }
    }) satisfies DisplayRow[]
  }, [candidates, jobItemByBillID])
  const firstDocNo = displayRows.find((row) => row.ready && row.docNo)?.docNo
  const lastDocNo = [...displayRows].reverse().find((row) => row.ready && row.docNo)?.docNo
  const docNoRange =
    firstDocNo && lastDocNo && firstDocNo !== lastDocNo
      ? `${firstDocNo} - ${lastDocNo}`
      : firstDocNo || ''
  const parsedVatRate = Number(vatRateStr)
  const vatRateNum = Number.isFinite(parsedVatRate) ? parsedVatRate : 7
  const isPurchaseOrder = billType === 'purchase'
  const isShopeePurchaseBulk = isPurchaseOrder && filters.source === 'shopee_shipped'
  const canSend =
    readyCount > 0 &&
    !!party?.code &&
    whCode.trim() !== '' &&
    shelfCode.trim() !== '' &&
    vatTypeStr !== '' &&
    (!isPurchaseOrder || inquiryTypeStr !== '') &&
    docTime.trim() !== '' &&
    !controlsLocked &&
    !job
  const missingFields = [
    !party?.code ? (billType === 'sale' ? 'ลูกค้า' : 'ผู้ขาย') : '',
    whCode.trim() === '' ? 'คลัง' : '',
    shelfCode.trim() === '' ? 'พื้นที่เก็บ' : '',
    vatTypeStr === '' ? 'ประเภทภาษี' : '',
    isPurchaseOrder && inquiryTypeStr === '' ? 'ประเภทรายการ' : '',
    docTime.trim() === '' ? 'เวลาเอกสาร' : '',
  ].filter(Boolean)
  const failedRows = displayRows.filter((row) => row.result === 'failed')
  const completedRows = displayRows.filter((row) => row.result)
  const resultSkippedCount = (job ? job.skipped_count : candidates.filter((c) => c.result === 'skipped').length) + (job && !activeJob ? skippedCount : 0)
  const finished = job ? !activeJob : completedRows.length > 0 && !sending

  const destination = useMemo(() => {
    if (filters.document_route === 'saleinvoice') {
      return { label: 'ขาย -> ขายสินค้าและบริการ', code: 'SI' }
    }
    if (filters.document_route === 'saleorder') {
      return { label: 'ขาย -> ใบสั่งขาย', code: 'SO' }
    }
    return { label: 'ซื้อ -> ใบสั่งซื้อ', code: 'PO' }
  }, [filters.document_route])

  const resetFormDefaults = () => {
    setParty(null)
    setDocTime(currentTimeHHMM())
    setWhCode('')
    setShelfCode('')
    setManualWarehouse(false)
    setVatTypeStr('')
    setVatRateStr('7')
    setInquiryTypeStr('')
    setRemark2Str(REMARK2_NONE)
    setBranchCode('')
    setSaleCode('')
    setRemark('')
  }

  const applyPayloadToForm = (p?: RetryBillPayload) => {
    if (!p) {
      resetFormDefaults()
      return
    }
    setParty(p.party_code ? { code: p.party_code, name: p.party_name || '' } : null)
    setDocTime(p.doc_time || currentTimeHHMM())
    setWhCode(p.wh_code || '')
    setShelfCode(p.shelf_code || '')
    setManualWarehouse(false)
    setVatTypeStr(typeof p.vat_type === 'number' ? String(p.vat_type) : '')
    setVatRateStr(typeof p.vat_rate === 'number' ? String(p.vat_rate) : '7')
    setInquiryTypeStr(typeof p.inquiry_type === 'number' ? String(p.inquiry_type) : '')
    setRemark2Str(normalizeRemark2(p.remark_2))
    setBranchCode(p.branch_code || '')
    setSaleCode(p.sale_code || '')
    setRemark(p.remark || '')
  }

  const restoreJob = (next: BulkSendJob) => {
    setJob(next)
    setCandidates(jobToCandidates(next))
    setTotalPending(next.total_count)
    applyPayloadToForm(next.request_payload)
    try {
      window.localStorage.setItem(storageKey, next.id)
    } catch {
      // localStorage can be unavailable in some private contexts; backend active lookup still works.
    }
  }

  useEffect(() => {
    if (!open) return
    if (job) return
    let alive = true
    setLoading(true)
    setSending(false)
    setJobError('')
    resetFormDefaults()
    setCandidates([])
    setTotalPending(0)

    async function load() {
      try {
        const storedJobID = (() => {
          try {
            return window.localStorage.getItem(storageKey) || ''
          } catch {
            return ''
          }
        })()
        if (storedJobID) {
          try {
            const storedJob = await getBulkSendJob(storedJobID)
            if (!alive) return
            if (shouldRestoreJob(storedJob)) {
              restoreJob(storedJob)
              return
            }
            window.localStorage.removeItem(storageKey)
          } catch (err) {
            const maybe = err as { response?: { status?: number } }
            if (maybe.response?.status === 404) {
              try {
                window.localStorage.removeItem(storageKey)
              } catch {
                // ignore unavailable localStorage
              }
            } else if (alive) {
              setJobError(errorMessage(err))
            }
          }
        }

        const active = await getActiveBulkSendJob({
          source: filters.source,
          bill_type: filters.bill_type,
          document_route: filters.document_route,
          shopee_shop_id: filters.shopee_shop_id,
        })
        if (!alive) return
        if (active) {
          restoreJob(active)
          return
        }

        const params = new URLSearchParams({
          source: filters.source,
          bill_type: filters.bill_type,
          status: 'pending',
          page: '1',
          per_page: String(BULK_BATCH_SIZE),
          include_total: 'true',
        })
        if (filters.document_route) params.set('document_route', filters.document_route)
        if (filters.email_account_id) params.set('email_account_id', filters.email_account_id)
        if (filters.shopee_status) params.set('shopee_status', filters.shopee_status)
        if (filters.shopee_shop_id) params.set('shopee_shop_id', filters.shopee_shop_id)
        if (filters.search) params.set('search', filters.search)
        const res = await client.get<{ data: Bill[]; total: number }>(`/api/bills?${params}`)
        const list = res.data.data ?? []
        const details = await Promise.all(list.map((b) => getBill(b.id)))
        const rows = details.map((bill) => {
          const validation = validateForSML(bill)
          return {
            bill,
            ready: validation.canSend,
            issues: validation.issues.map((issue) => `${issue.count} รายการ${issueLabel(issue.kind)}`),
          }
        })
        if (!alive) return
        setTotalPending(res.data.total ?? rows.length)
        setCandidates(rows)
      } catch (err) {
        if (!alive) return
        setCandidates([{
          bill: {
            id: 'load-error',
            bill_type: billType,
            source: filters.source,
            status: 'failed',
            created_at: new Date().toISOString(),
          } as Bill,
          ready: false,
          issues: [err instanceof Error ? err.message : 'โหลดรายการไม่สำเร็จ'],
        }])
      } finally {
        if (alive) setLoading(false)
      }
    }
    load()
    return () => {
      alive = false
    }
  }, [
    open,
    filters.source,
    filters.bill_type,
    filters.document_route,
    filters.email_account_id,
    filters.shopee_status,
    filters.shopee_shop_id,
    filters.search,
    billType,
    job,
    storageKey,
  ])

  const payload = (): RetryBillPayload => ({
    party_code: party?.code,
    party_name: party?.name,
    remark: isShopeePurchaseBulk ? undefined : remark.trim() || undefined,
    remark_2: remark2PayloadValue(remark2Str),
    branch_code: branchCode.trim() || undefined,
    sale_code: saleCode.trim() || undefined,
    doc_time: docTime.trim(),
    wh_code: whCode.trim(),
    shelf_code: shelfCode.trim(),
    vat_type: Number(vatTypeStr),
    vat_rate: vatRateNum,
    inquiry_type: isPurchaseOrder ? Number(inquiryTypeStr) : undefined,
  })

  useEffect(() => {
    if (!job?.id) return
    if (!isActiveJob(job)) {
      setSending(false)
      return
    }
    let alive = true
    setSending(true)

    async function poll() {
      try {
        const next = await getBulkSendJob(job!.id)
        if (!alive) return
        setJob(next)
        setJobError('')
        if (!isActiveJob(next)) {
          setSending(false)
          onDone?.()
        }
      } catch (err) {
        if (!alive) return
        setJobError(errorMessage(err))
      }
    }

    void poll()
    const timer = window.setInterval(poll, 1000)
    return () => {
      alive = false
      window.clearInterval(timer)
    }
  }, [job?.id, job?.status, onDone])

  const copyFailureSummary = async () => {
    if (failedRows.length === 0) return
    const text = [
      `Bulk Send SML failed: ${title}`,
      `ปลายทาง: ${destination.label} (${destination.code})`,
      `ส่งสำเร็จ: ${sentCount}, ไม่สำเร็จ: ${failedCount}, ข้าม: ${resultSkippedCount}`,
      '',
      ...failedRows.map((row) => [
        `Order: ${row.orderNo}`,
        row.docNo ? `doc_no: ${row.docNo}` : '',
        `Bill: ${row.bill.id}`,
        `Error: ${row.message ?? 'ไม่ทราบสาเหตุ'}`,
      ].filter(Boolean).join('\n')),
    ].join('\n\n')
    try {
      await navigator.clipboard.writeText(text)
      toast.success('คัดลอก error summary แล้ว')
    } catch {
      toast.error('คัดลอกไม่สำเร็จ')
    }
  }

  const handleSend = async () => {
    if (!canSend) return
    setSending(true)
    setJobError('')
    try {
      const next = await createBulkSendJob({
        client_request_id: newClientRequestID(),
        bill_ids: candidates.filter((row) => row.ready).map((row) => row.bill.id),
        payload: payload(),
        filter_snapshot: filters,
        source: filters.source,
        bill_type: filters.bill_type,
        document_route: filters.document_route,
        title,
      })
      setJob(next)
      try {
        window.localStorage.setItem(storageKey, next.id)
      } catch {
        // backend active lookup covers resume when localStorage is unavailable.
      }
      toast.success('เริ่มส่ง SML แบบ batch แล้ว')
    } catch (err) {
      setJobError(errorMessage(err))
      toast.error(errorMessage(err))
      setSending(false)
    }
  }

  const handleRetryFailed = async () => {
    if (!job || failedCount === 0 || activeJob) return
    setSending(true)
    setJobError('')
    try {
      const next = await retryFailedBulkSendJob(job.id, newClientRequestID())
      setJob(next)
      setCandidates(jobToCandidates(next))
      try {
        window.localStorage.setItem(storageKey, next.id)
      } catch {
        // ignore unavailable localStorage
      }
      toast.success('เริ่ม retry เฉพาะรายการที่ไม่สำเร็จแล้ว')
    } catch (err) {
      setJobError(errorMessage(err))
      toast.error(errorMessage(err))
      setSending(false)
    }
  }

  const resetJobResult = () => {
    if (activeJob) return
    setJob(null)
    setJobError('')
    setCandidates((prev) => prev.map((row) => ({ ...row, result: undefined, message: undefined })))
    try {
      window.localStorage.removeItem(storageKey)
    } catch {
      // ignore unavailable localStorage
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="grid max-h-[92vh] grid-rows-[auto_minmax(0,1fr)_auto] sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>ส่ง SML รายการพร้อมส่ง: {title}</DialogTitle>
        </DialogHeader>

        <div className="-mx-6 space-y-4 overflow-y-auto px-6 py-2">
          <div className="rounded-md border border-info/25 bg-info/[0.04] px-3 py-2 text-xs text-muted-foreground">
            <div className="font-medium text-foreground">
              ปลายทาง SML: {destination.label} · {destination.code}
            </div>
            <div className="mt-0.5">
              ระบบจะส่งเฉพาะเอกสารสถานะพร้อมส่ง และใช้ค่าชุดนี้ร่วมกันทุกเอกสารในรอบนี้
            </div>
          </div>
          {totalPending > candidates.length && (
            <div className="rounded-md border border-warning/35 bg-warning/[0.07] px-3 py-2 text-xs text-warning">
              รอบนี้โหลดมา {candidates.length} จาก {totalPending} รายการเพื่อให้ระบบทำงานนิ่ง
              หลังส่งชุดแรกเสร็จ ให้เปิด dialog นี้อีกครั้งเพื่อส่งรายการที่เหลือ
            </div>
          )}
          {job && (
            <div className="rounded-md border border-info/25 bg-info/[0.04] px-3 py-2.5 text-xs">
              <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
                <div className="font-medium text-foreground">
                  Bulk job <span className="font-mono">{job.id.slice(0, 8)}</span>
                </div>
                <div className="text-muted-foreground">
                  {activeJob ? 'กำลังส่งทีละบิล' : job.status === 'completed' ? 'เสร็จสมบูรณ์' : job.status === 'completed_with_errors' ? 'เสร็จพร้อมรายการไม่สำเร็จ' : 'หยุดด้วยข้อผิดพลาด'}
                </div>
              </div>
              <div className="h-2 overflow-hidden rounded-full bg-muted">
                <div className="h-full bg-info transition-all" style={{ width: `${progressPct}%` }} />
              </div>
              <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-muted-foreground">
                <span>สำเร็จ {sentCount}</span>
                <span>ไม่สำเร็จ {failedCount}</span>
                <span>ข้าม {jobSkippedCount}</span>
                <span>เหลือ {remainingCount}</span>
                <span>{progressPct}%</span>
              </div>
              {jobError && (
                <div className="mt-2 rounded-md border border-warning/35 bg-warning/[0.07] px-2 py-1.5 text-warning">
                  Poll job ไม่สำเร็จชั่วคราว: {jobError}
                </div>
              )}
            </div>
          )}

          <div className="rounded-md border border-border bg-card px-3 py-2.5 text-xs">
            <div className="mb-2 flex items-center justify-between gap-2">
              <div className="font-medium text-foreground">สรุปก่อนส่ง</div>
              <div className="text-muted-foreground">
                พร้อมส่ง {readyCount} · ข้าม {skippedCount}
              </div>
            </div>
            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
              <SummaryItem
                label={billType === 'sale' ? 'ลูกค้า' : 'ผู้ขาย'}
                value={party?.code ? `${party.code} · ${party.name}` : 'ยังไม่เลือก'}
                muted={!party?.code}
              />
              <SummaryItem label="ปลายทาง" value={`${destination.label} · ${destination.code}`} />
              <SummaryItem label="doc_no" value={docNoRange || 'รอ preview'} mono muted={!docNoRange} />
              <SummaryItem
                label="คลัง / พื้นที่เก็บ"
                value={whCode && shelfCode ? `${whCode} / ${shelfCode}` : 'ยังไม่ครบ'}
                mono
                muted={!whCode || !shelfCode}
              />
              <SummaryItem
                label="VAT"
                value={`${vatTypeLabel(vatTypeStr)} · ${vatRateStr || '-'}%`}
                muted={!vatTypeStr || !vatRateStr}
              />
              {isPurchaseOrder && (
                <SummaryItem
                  label="ประเภทรายการ"
                  value={PURCHASE_INQUIRY_TYPE_OPTIONS.find((option) => option.value === inquiryTypeStr)?.label || 'ยังไม่เลือก'}
                  muted={!inquiryTypeStr}
                />
              )}
              <SummaryItem
                label="สถานะเอกสาร"
                value={remark2Label(remark2PayloadValue(remark2Str))}
                muted={remark2Str === REMARK2_NONE}
              />
              <SummaryItem label="เวลาเอกสาร" value={docTime || 'ยังไม่ระบุ'} mono muted={!docTime} />
            </div>
          </div>

          <div className="space-y-1.5">
            <Label>{billType === 'sale' ? 'ลูกค้า' : 'ผู้ขาย'} <span className="text-destructive">*</span></Label>
            <PartyPicker billType={billType} value={party} onChange={setParty} disabled={controlsLocked} />
            {!party?.code && (
              <p className="text-[11px] text-warning">
                ต้องเลือก{billType === 'sale' ? 'ลูกค้า' : 'ผู้ขาย'}ก่อนส่งเข้า SML
              </p>
            )}
          </div>

          <div className="grid gap-2.5 rounded-md border border-border bg-muted/20 p-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label className="text-xs">เวลาเอกสาร <span className="text-destructive">*</span></Label>
              <Input
                value={docTime}
                onChange={(e) => setDocTime(e.target.value)}
                placeholder="เช่น 09:00"
                className="font-mono"
                disabled={controlsLocked}
              />
            </div>

            <div className="space-y-1">
              <div className="flex items-center justify-between gap-2">
                <Label className="text-xs">คลัง <span className="text-destructive">*</span></Label>
                <Button type="button" variant="ghost" size="sm" className="h-6 px-1.5 text-[11px]" onClick={() => setManualWarehouse((v) => !v)} disabled={controlsLocked}>
                  {manualWarehouse ? 'เลือกจาก SML' : 'พิมพ์รหัสเอง'}
                </Button>
              </div>
              {manualWarehouse ? (
                <Input
                  value={whCode}
                  onChange={(e) => {
                    setWhCode(e.target.value.toUpperCase())
                    setShelfCode('')
                  }}
                  placeholder="เช่น WH-01"
                  className="font-mono"
                  disabled={controlsLocked}
                />
              ) : (
                <WarehousePicker
                  value={whCode}
                  disabled={controlsLocked}
                  onChange={(warehouse) => {
                    setWhCode(warehouse.code)
                    setShelfCode('')
                  }}
                />
              )}
            </div>
            <div className="space-y-1">
              <Label className="text-xs">พื้นที่เก็บ <span className="text-destructive">*</span></Label>
              {manualWarehouse ? (
                <Input
                  value={shelfCode}
                  onChange={(e) => setShelfCode(e.target.value.toUpperCase())}
                  placeholder="เช่น SH-01"
                  className="font-mono"
                  disabled={controlsLocked}
                />
              ) : (
                <ShelfPicker warehouseCode={whCode} value={shelfCode} onChange={(shelf) => setShelfCode(shelf.code)} disabled={controlsLocked} />
              )}
            </div>

            <div className="space-y-1">
              <Label className="text-xs">ประเภทภาษี <span className="text-destructive">*</span></Label>
              <Select value={vatTypeStr} onValueChange={setVatTypeStr} disabled={controlsLocked}>
                <SelectTrigger className="h-9 text-sm">
                  <SelectValue placeholder="เลือกประเภทภาษี" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="0">0 — แยกนอก</SelectItem>
                  <SelectItem value="1">1 — รวมใน</SelectItem>
                  <SelectItem value="2">2 — ศูนย์%</SelectItem>
                </SelectContent>
              </Select>
            </div>

            {isPurchaseOrder && (
              <div className="space-y-1">
                <Label className="text-xs">ประเภทรายการ <span className="text-destructive">*</span></Label>
                <Select value={inquiryTypeStr} onValueChange={setInquiryTypeStr} disabled={controlsLocked}>
                  <SelectTrigger className="h-9 text-sm">
                    <SelectValue placeholder="เลือกประเภทรายการ" />
                  </SelectTrigger>
                  <SelectContent>
                    {PURCHASE_INQUIRY_TYPE_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
            <div className="space-y-1">
              <Label className="text-xs">สถานะเอกสาร</Label>
              <Select value={remark2Str} onValueChange={setRemark2Str} disabled={controlsLocked}>
                <SelectTrigger className="h-9 text-sm">
                  <SelectValue placeholder="ไม่ระบุ" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={REMARK2_NONE}>ไม่ระบุ</SelectItem>
                  {SML_REMARK2_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="rounded-md bg-background/70 px-2.5 py-1.5 text-[11px] text-muted-foreground sm:col-span-2">
              อัตราภาษีใช้ {vatRateNum}% จากค่าเริ่มต้นของระบบเพื่อกัน user กรอกผิด
            </div>
            <details className="space-y-2 rounded-md border border-border bg-background px-3 py-2 sm:col-span-2">
              <summary className="cursor-pointer text-xs font-medium text-muted-foreground">
                ตัวเลือกเพิ่มเติม: Branch code / Sale code (ไม่บังคับ)
              </summary>
              <div className="mt-3 grid gap-3 sm:grid-cols-2">
                <div className="space-y-1">
                  <Label className="text-xs">Branch code</Label>
                  <Input value={branchCode} onChange={(e) => setBranchCode(e.target.value)} className="font-mono" placeholder="ปล่อยว่างได้" disabled={controlsLocked} />
                </div>
                <div className="space-y-1">
                  <Label className="text-xs">Sale code</Label>
                  <Input value={saleCode} onChange={(e) => setSaleCode(e.target.value)} className="font-mono" placeholder="ปล่อยว่างได้" disabled={controlsLocked} />
                </div>
              </div>
            </details>
            <div className="rounded-md bg-background/70 px-2.5 py-1.5 text-[11px] text-muted-foreground sm:col-span-2">
              เลือกคลังจาก SML หรือพิมพ์รหัสเองได้ เลข doc_no ด้านล่างเป็นเลขคาดการณ์จาก SML ล่าสุดและจะจองเลขจริงอีกครั้งเมื่อกดส่ง
            </div>
          </div>

          {isShopeePurchaseBulk ? (
            <div className="rounded-md border border-info/25 bg-info/[0.04] px-3 py-2 text-xs text-muted-foreground">
              บิลซื้อ Shopee จะส่ง remark เป็นผู้ขายจากอีเมลของแต่ละบิลอัตโนมัติ และส่งเลขคำสั่งซื้อเข้า remark_5
            </div>
          ) : (
            <div className="space-y-1.5">
              <Label htmlFor="bulk-remark">หมายเหตุ</Label>
              <textarea
                id="bulk-remark"
                value={remark}
                onChange={(e) => setRemark(e.target.value)}
                rows={3}
                className="w-full resize-none rounded-md border border-input bg-background px-3 py-2 text-sm placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring"
                placeholder="หมายเหตุสำหรับ SML (ถ้ามี)"
                disabled={controlsLocked}
              />
            </div>
          )}

          <div className="overflow-hidden rounded-md border border-border">
            <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border bg-muted/20 px-3 py-2 text-xs">
              <div>
                <div className="font-medium text-foreground">ตรวจรายการพร้อมส่งและเลขเอกสาร</div>
                <div className="mt-0.5 text-[11px] text-muted-foreground">
                  doc_no เป็นเลขคาดการณ์จาก SML ล่าสุด ถ้า SML แจ้งเลขซ้ำให้กดออกเลขใหม่ที่หน้า detail ของบิลนั้น
                </div>
              </div>
              <div className="flex flex-wrap justify-end gap-2 text-muted-foreground">
                <span>พร้อมส่ง {readyCount}</span>
                <span>ต้องข้าม {skippedCount}</span>
                {docNoRange && <span className="font-mono text-foreground">{docNoRange}</span>}
                {totalPending > candidates.length && <span>โหลด {BULK_BATCH_SIZE}/{totalPending}</span>}
              </div>
            </div>
            {!loading && candidates.length > 0 && (
              <div className="hidden grid-cols-[54px_minmax(0,1fr)_180px_86px] gap-2 border-b border-border bg-background px-3 py-2 text-[11px] font-medium text-muted-foreground sm:grid">
                <div>ลำดับ</div>
                <div>เอกสาร</div>
                <div>doc_no ที่จะได้</div>
                <div className="text-right">สถานะ</div>
              </div>
            )}
            <div className="max-h-64 overflow-y-auto divide-y divide-border">
              {loading ? (
                <div className="flex items-center gap-2 px-3 py-4 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  กำลังตรวจเอกสารพร้อมส่ง…
                </div>
              ) : candidates.length === 0 ? (
                <div className="px-3 py-4 text-sm text-muted-foreground">ไม่มีเอกสารสถานะพร้อมส่งในเมนูนี้</div>
              ) : (
                displayRows.map((row) => (
                  <div key={row.bill.id} className="grid gap-2 px-3 py-2.5 text-xs sm:grid-cols-[54px_minmax(0,1fr)_180px_86px] sm:items-center">
                    <div className="hidden sm:block">
                      {row.sequence ? (
                        <span className="inline-flex h-7 min-w-7 items-center justify-center rounded-full bg-info/10 px-2 font-mono font-semibold text-info">
                          {row.sequence}
                        </span>
                      ) : (
                        <span className="text-muted-foreground">-</span>
                      )}
                    </div>
                    <div className="min-w-0 space-y-0.5">
                      <div className="truncate font-medium text-foreground">
                        {row.sequence ? <span className="mr-1 text-muted-foreground sm:hidden">#{row.sequence}</span> : null}
                        Order <span className="font-mono">{row.orderNo}</span>
                      </div>
                      <div className="truncate text-muted-foreground">
                        {row.message ?? (row.ready ? 'ผ่านการตรวจความพร้อม' : row.issues.join(' · '))}
                      </div>
                    </div>
                    <div className="min-w-0">
                      <div className="inline-flex max-w-full items-center rounded-md border border-border bg-background px-2.5 py-1 font-mono font-semibold text-foreground">
                        <span className="truncate">{row.ready ? (row.docNo || 'รอออกเลข') : 'ไม่ส่ง'}</span>
                      </div>
                    </div>
                    <div className="flex items-center gap-1 justify-end">
                      {row.result === 'sent' ? (
                        <span className="inline-flex items-center gap-1 text-success"><CheckCircle2 className="h-3.5 w-3.5" />สำเร็จ</span>
                      ) : row.result === 'failed' ? (
                        <span className="inline-flex items-center gap-1 text-destructive"><AlertTriangle className="h-3.5 w-3.5" />ไม่สำเร็จ</span>
                      ) : row.jobStatus === 'running' ? (
                        <span className="inline-flex items-center gap-1 text-info"><Loader2 className="h-3.5 w-3.5 animate-spin" />กำลังส่ง</span>
                      ) : row.jobStatus === 'queued' ? (
                        <span className="text-muted-foreground">รอคิว</span>
                      ) : row.ready ? (
                        <span className="text-success">พร้อม</span>
                      ) : (
                        <span className="text-warning">ข้าม</span>
                      )}
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>
          {finished && (
            <div className={[
              'rounded-md border px-3 py-2.5 text-xs',
              failedCount > 0
                ? 'border-destructive/35 bg-destructive/[0.06]'
                : 'border-success/35 bg-success/[0.06]',
            ].join(' ')}>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <div className="font-medium text-foreground">ผลการส่งรอบนี้</div>
                  <div className="mt-0.5 text-muted-foreground">
                    สำเร็จ {sentCount} · ไม่สำเร็จ {failedCount} · ข้าม {resultSkippedCount}
                  </div>
                </div>
                {failedRows.length > 0 && (
                  <div className="flex flex-wrap gap-2">
                    <Button type="button" size="sm" className="h-8 gap-1.5" onClick={handleRetryFailed} disabled={controlsLocked}>
                      <RotateCcw className="h-3.5 w-3.5" />
                      Retry failed
                    </Button>
                    <Button type="button" size="sm" variant="outline" className="h-8 gap-1.5" onClick={copyFailureSummary}>
                      <Clipboard className="h-3.5 w-3.5" />
                      คัดลอก error
                    </Button>
                    <Button asChild size="sm" variant="outline" className="h-8 gap-1.5">
                      <Link to={billDetailPath(failedRows[0].bill)}>
                        <ExternalLink className="h-3.5 w-3.5" />
                        ดูบิลแรกที่ไม่สำเร็จ
                      </Link>
                    </Button>
                  </div>
                )}
              </div>
              {failedRows.length > 0 && (
                <div className="mt-2 space-y-1">
                  {failedRows.slice(0, 3).map((row) => (
                    <div key={row.bill.id} className="rounded-md border border-border/70 bg-background/70 px-2 py-1.5">
                      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                        <span className="font-mono font-medium text-foreground">{row.orderNo}</span>
                        {row.docNo && <span className="font-mono text-muted-foreground">{row.docNo}</span>}
                      </div>
                      <div className="mt-0.5 line-clamp-2 text-destructive">
                        {row.message ?? 'ส่งไม่สำเร็จ'}
                      </div>
                    </div>
                  ))}
                  {failedRows.length > 3 && (
                    <div className="text-muted-foreground">
                      ยังมีรายการไม่สำเร็จอีก {failedRows.length - 3} รายการ ใช้ปุ่มคัดลอก error เพื่อส่งให้ทีมตรวจได้
                    </div>
                  )}
                </div>
              )}
            </div>
          )}
          {missingFields.length > 0 && (
            <div className="flex items-start gap-2 rounded-md border border-warning/35 bg-warning/[0.07] px-3 py-2 text-xs text-warning">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <div>ต้องกรอกเพิ่มก่อนส่ง: {missingFields.join(', ')}</div>
            </div>
          )}
        </div>

        <DialogFooter className="items-center gap-2 sm:justify-between">
          <div className="text-xs text-muted-foreground">
            {activeJob ? `ส่งแล้ว ${sentCount} · ไม่สำเร็จ ${failedCount} · เหลือ ${remainingCount}` : finished ? `สำเร็จ ${sentCount} · ไม่สำเร็จ ${failedCount}` : 'ตรวจ doc_no ในรายการก่อนกดส่ง'}
          </div>
          <div className="flex gap-2">
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              ปิด
            </Button>
            {finished ? (
              <Button type="button" onClick={resetJobResult} variant="outline" disabled={activeJob}>
                โหลดรายการใหม่
              </Button>
            ) : (
              <Button type="button" onClick={handleSend} disabled={!canSend} className="gap-2">
                {sending ? <Loader2 className="h-4 w-4 animate-spin" /> : <Send className="h-4 w-4" />}
                ส่ง SML {readyCount} รายการ
              </Button>
            )}
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function SummaryItem({
  label,
  value,
  mono,
  muted,
}: {
  label: string
  value: string
  mono?: boolean
  muted?: boolean
}) {
  return (
    <div className="min-w-0 rounded-md border border-border/70 bg-background px-2.5 py-2">
      <div className="text-[10px] font-medium uppercase text-muted-foreground">{label}</div>
      <div className={[
        'mt-0.5 truncate font-medium',
        mono ? 'font-mono' : '',
        muted ? 'text-muted-foreground' : 'text-foreground',
      ].join(' ')}>
        {value}
      </div>
    </div>
  )
}
