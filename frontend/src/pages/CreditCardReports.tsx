import { Fragment, useCallback, useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import {
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Download,
  FileSpreadsheet,
  History,
  Info,
  Loader2,
  Printer,
  RefreshCw,
} from 'lucide-react'
import { toast } from 'sonner'

import { EmptyState } from '@/components/common/EmptyState'
import { DateRangePicker } from '@/components/common/DateRangePicker'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
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
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  createCreditCardReportRun,
  exportCreditCardReportRun,
  listCreditCardReportRuns,
  previewCreditCardReport,
  recordCreditCardReportPrintEvents,
} from '@/hooks/useCreditCardReports'
import { DEFAULT_MARKETPLACE_PRINT_PAYMENT_METHODS } from '@/pages/ChannelDefaults/labels'
import { printArtifactsBatch } from '@/pages/BillDetail/hooks/useArtifacts'
import { cn } from '@/lib/utils'
import type {
  CreditCardReportFilter,
  CreditCardReportGroup,
  CreditCardReportIssue,
  CreditCardReportPreview,
  CreditCardReportRun,
} from '@/types'

const ALL = 'all'

const SOURCE_OPTIONS = [
  { value: ALL, label: 'ทุกช่องทาง' },
  { value: 'shopee_shipped', label: 'Shopee' },
  { value: 'lazada_email', label: 'Lazada' },
]

const INCLUDE_INCOMPLETE_HELP =
  'ปิดไว้จะแสดงเฉพาะยอดรูดบัตรที่พร้อมเทียบ statement. เปิดเพื่อรวมรายการที่ยังไม่มียอดรูดบัตรหรือยังจัดกลุ่มยอดรูดไม่ได้ สำหรับตรวจแก้ข้อมูลก่อน export.'

const CHARGE_AMOUNT_HELP =
  'ยอดรูดบัตรคือยอดรวมของกลุ่มจากอีเมล ไม่ใช่ผลรวมเฉพาะคำสั่งซื้อที่มี POL แล้ว และยังไม่รวมยอดคืนเงิน/ยอดติดลบจาก statement.'

const EMAIL_TIME_HELP =
  'วันที่/เวลาจากอีเมลที่ BillFlow ใช้เรียงและจับกลุ่มยอดรูดบัตร ไม่ใช่เวลาจาก statement ธนาคาร.'

function todayISO() {
  return new Date().toISOString().slice(0, 10)
}

function firstDayOfMonthISO() {
  const d = new Date()
  d.setDate(1)
  return d.toISOString().slice(0, 10)
}

function money(value?: number | null) {
  if (value === null || value === undefined || Number.isNaN(value)) return '-'
  return value.toLocaleString('th-TH', { style: 'currency', currency: 'THB' })
}

function numberLabel(value: number) {
  return value.toLocaleString('th-TH')
}

function displayOrderID(value?: string) {
  return (value ?? '').trim().replace(/^#+/, '')
}

function dateTimeLabel(value?: string) {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString('th-TH', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function compactDateTimeLabel(value?: string) {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  return d.toLocaleString('th-TH', {
    year: '2-digit',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  })
}

function groupSelectedDefaults(preview: CreditCardReportPreview) {
  return new Set(
    preview.groups
      .filter((group) => group.charge_amount !== null && group.charge_amount !== undefined)
      .map((group) => group.group_id),
  )
}

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

function reportNameFromFilter(filter: CreditCardReportFilter) {
  const method = filter.payment_method && filter.payment_method !== ALL ? filter.payment_method : 'ทุกบัตร'
  return `รายงานบัตรเครดิต ${method} ${filter.date_from} ถึง ${filter.date_to}`
}

function selectedGroups(preview: CreditCardReportPreview | null, selected: Set<string>) {
  if (!preview) return []
  return preview.groups.filter((group) => selected.has(group.group_id))
}

function issueTone(issueCode: string) {
  if (issueCode === 'amount_mismatch') return 'border-warning/30 bg-warning/10 text-warning'
  if (issueCode === 'missing_pol' || issueCode === 'missing_charge_amount' || issueCode === 'missing_group_key') {
    return 'border-destructive/30 bg-destructive/10 text-destructive'
  }
  return 'border-muted-foreground/20 bg-muted text-muted-foreground'
}

function issueShortLabel(issue: CreditCardReportIssue) {
  if (issue.code === 'amount_mismatch') return 'ยอดไม่ตรง'
  if (issue.code === 'missing_pol') return 'ยังไม่มี POL'
  if (issue.code === 'missing_charge_amount') return 'ไม่มียอดรูดบัตร'
  if (issue.code === 'missing_group_key') return 'ยังไม่จัดกลุ่ม'
  if (issue.code === 'missing_payment_method') return 'วิธีชำระ'
  if (issue.code === 'mixed_payment_method') return 'หลายวิธีชำระ'
  return issue.message
}

function HeaderHelp({ children, help, align = 'left' }: { children: ReactNode; help?: string; align?: 'left' | 'right' }) {
  if (!help) return <>{children}</>
  return (
    <div className={cn('inline-flex items-center gap-1', align === 'right' && 'justify-end')}>
      <span>{children}</span>
      <TooltipProvider delayDuration={120}>
        <Tooltip>
          <TooltipTrigger asChild>
            <Info className="h-3.5 w-3.5 cursor-help text-muted-foreground" />
          </TooltipTrigger>
          <TooltipContent className="max-w-xs text-xs leading-relaxed">
            {help}
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
    </div>
  )
}

function IssueChips({ group }: { group: CreditCardReportGroup }) {
  if (group.issues.length === 0) {
    return (
      <Badge variant="outline" className="h-5 px-1.5 text-[11px] border-success/30 bg-success/10 text-success">
        พร้อม
      </Badge>
    )
  }
  const visibleIssues = group.issues.slice(0, 2)
  const hiddenIssueCount = group.issues.length - visibleIssues.length
  return (
    <div className="flex max-w-[160px] flex-wrap gap-1">
      {visibleIssues.map((issue) => (
        <Badge
          key={`${group.group_id}-${issue.code}`}
          variant="outline"
          className={cn('h-5 max-w-full px-1.5 text-[11px]', issueTone(issue.code))}
          title={issue.message}
        >
          <span className="truncate">{issueShortLabel(issue)}</span>
        </Badge>
      ))}
      {hiddenIssueCount > 0 && (
        <Badge
          variant="outline"
          className="h-5 px-1.5 text-[11px] text-muted-foreground"
          title={group.issues.map((issue) => issue.message).join(', ')}
        >
          +{hiddenIssueCount}
        </Badge>
      )}
    </div>
  )
}

function SummaryItem({ label, value, tone }: { label: string; value: string; tone?: 'warn' }) {
  return (
    <div className="flex items-baseline gap-1.5 whitespace-nowrap">
      <span className="text-[11px] text-muted-foreground">{label}</span>
      <span className={cn('text-sm font-semibold tabular-nums text-foreground', tone === 'warn' && 'text-warning')}>
        {value}
      </span>
    </div>
  )
}

export default function CreditCardReports() {
  const [filter, setFilter] = useState<CreditCardReportFilter>({
    date_from: firstDayOfMonthISO(),
    date_to: todayISO(),
    payment_method: ALL,
    source: ALL,
    include_incomplete: false,
  })
  const [preview, setPreview] = useState<CreditCardReportPreview | null>(null)
  const [runs, setRuns] = useState<CreditCardReportRun[]>([])
  const [activeRun, setActiveRun] = useState<CreditCardReportRun | null>(null)
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [loadingPreview, setLoadingPreview] = useState(false)
  const [loadingRuns, setLoadingRuns] = useState(false)
  const [showRuns, setShowRuns] = useState(false)
  const [exportDialogOpen, setExportDialogOpen] = useState(false)
  const [busy, setBusy] = useState('')

  const selectedRows = useMemo(() => selectedGroups(preview, selected), [preview, selected])
  const selectedOrderCount = useMemo(
    () => selectedRows.reduce((sum, group) => sum + group.order_count, 0),
    [selectedRows],
  )
  const selectedChargeTotal = useMemo(
    () => selectedRows.reduce((sum, group) => sum + (group.charge_amount ?? 0), 0),
    [selectedRows],
  )
  const selectedOrderTotal = useMemo(
    () => selectedRows.reduce((sum, group) => sum + group.order_total, 0),
    [selectedRows],
  )
  const selectedIssueCount = useMemo(
    () => selectedRows.filter((group) => group.issues.length > 0).length,
    [selectedRows],
  )

  const refreshRuns = useCallback(async () => {
    setLoadingRuns(true)
    try {
      setRuns(await listCreditCardReportRuns())
    } catch {
      toast.error('โหลดประวัติรอบรายงานไม่สำเร็จ')
    } finally {
      setLoadingRuns(false)
    }
  }, [])

  const loadPreview = useCallback(async () => {
    setLoadingPreview(true)
    try {
      const data = await previewCreditCardReport(filter)
      setPreview(data)
      setSelected(groupSelectedDefaults(data))
      setExpanded(new Set())
      setActiveRun(null)
      if (data.truncated) {
        toast.warning(`ผลลัพธ์เกิน ${numberLabel(data.limit)} กลุ่ม กรุณาลดช่วงวันที่หรือกรองบัตรให้แคบลง`)
      }
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      toast.error(e?.response?.data?.error || e?.message || 'โหลด preview รายงานไม่สำเร็จ')
    } finally {
      setLoadingPreview(false)
    }
  }, [filter])

  useEffect(() => {
    refreshRuns()
    loadPreview()
  }, [])

  const setFilterValue = (key: keyof CreditCardReportFilter, value: string | boolean) => {
    setFilter((current) => ({ ...current, [key]: value }))
  }

  const toggleSelected = (groupID: string, checked: boolean) => {
    setSelected((current) => {
      const next = new Set(current)
      if (checked) next.add(groupID)
      else next.delete(groupID)
      return next
    })
  }

  const toggleExpanded = (groupID: string) => {
    setExpanded((current) => {
      const next = new Set(current)
      if (next.has(groupID)) next.delete(groupID)
      else next.add(groupID)
      return next
    })
  }

  const selectAllPreview = () => {
    if (!preview) return
    setSelected(new Set(preview.groups.map((group) => group.group_id)))
  }

  const clearSelection = () => setSelected(new Set())

  const createSnapshotRun = async (options: { skipIssueConfirm?: boolean } = {}) => {
    if (!preview || selected.size === 0) {
      toast.error('กรุณาเลือกรายการอย่างน้อย 1 ยอดรูดบัตร')
      return null
    }
    if (selectedIssueCount > 0 && !options.skipIssueConfirm) {
      const ok = window.confirm(`มี ${numberLabel(selectedIssueCount)} กลุ่มที่ต้องตรวจสอบ ต้องการสร้างรอบรายงานต่อหรือไม่`)
      if (!ok) return null
    }
    const run = await createCreditCardReportRun({
      report_name: reportNameFromFilter(filter),
      filters: filter,
      selected_group_ids: Array.from(selected),
    })
    setActiveRun(run)
    setPreview(run.snapshot)
    setSelected(new Set(run.snapshot.groups.map((group) => group.group_id)))
    await refreshRuns()
    return run
  }

  const exportRun = async (run: CreditCardReportRun) => {
    setBusy('export')
    const toastID = toast.loading('กำลังสร้างไฟล์ Excel...')
    try {
      const { blob, filename } = await exportCreditCardReportRun(run.id)
      downloadBlob(blob, filename)
      toast.success('ดาวน์โหลดรายงานบัตรเครดิตแล้ว')
      await refreshRuns()
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      toast.error(e?.response?.data?.error || e?.message || 'Export Excel ไม่สำเร็จ')
    } finally {
      toast.dismiss(toastID)
      setBusy('')
    }
  }

  const handleCreateAndExport = async () => {
    setBusy('create-export')
    try {
      const run = await createSnapshotRun({ skipIssueConfirm: true })
      if (run) {
        await exportRun(run)
        setExportDialogOpen(false)
      }
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      toast.error(e?.response?.data?.error || e?.message || 'สร้างรอบรายงานไม่สำเร็จ')
    } finally {
      setBusy('')
    }
  }

  const printRun = async (run: CreditCardReportRun) => {
    const printable = run.snapshot.groups
      .filter((group) => group.print_ready)
      .flatMap((group) => (group.print_artifacts ?? []).map((artifact) => ({
        billID: artifact.bill_id,
        artID: artifact.artifact_id,
        filename: artifact.filename,
        printContext: {
          orders: artifact.orders.map((order) => ({
            orderId: order.order_id,
            smlDocNo: order.sml_doc_no,
            partyName: order.party_name,
            paymentMethod: order.payment_method,
          })),
        },
      })))
    if (printable.length === 0) {
      toast.error('รอบรายงานนี้ยังไม่มีรายการที่พร้อมพิมพ์')
      return
    }
    setBusy('print')
    const toastID = toast.loading('กำลังบันทึกประวัติและเปิดหน้าพิมพ์...')
    try {
      const res = await recordCreditCardReportPrintEvents(run.id)
      if (res.skipped?.length) {
        toast.warning(`ข้าม ${numberLabel(res.skipped.length)} กลุ่มที่ยังไม่พร้อมพิมพ์`)
      }
      const recorded = new Set((res.data ?? []).map((event) => `${event.bill_id}:${event.artifact_id || ''}`))
      const itemsToPrint = printable.filter((item) => recorded.has(`${item.billID}:${item.artID}`))
      if (itemsToPrint.length === 0) {
        toast.error('ไม่มีรายการที่ backend อนุญาตให้พิมพ์ในรอบนี้')
        return
      }
      await printArtifactsBatch(itemsToPrint)
      toast.success(`เปิดหน้าพิมพ์ ${numberLabel(itemsToPrint.length)} ชุดตามลำดับรายงานแล้ว`)
      await refreshRuns()
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      toast.error(e?.response?.data?.error || e?.message || 'พิมพ์ตามรายงานไม่สำเร็จ')
    } finally {
      toast.dismiss(toastID)
      setBusy('')
    }
  }

  const handleCreateAndPrint = async () => {
    setBusy('create-print')
    try {
      const run = await createSnapshotRun()
      if (run) await printRun(run)
    } catch (err) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      toast.error(e?.response?.data?.error || e?.message || 'สร้างรอบรายงานไม่สำเร็จ')
    } finally {
      setBusy('')
    }
  }

  const openRun = (run: CreditCardReportRun) => {
    setActiveRun(run)
    setPreview(run.snapshot)
    setFilter(run.filters)
    setSelected(new Set(run.snapshot.groups.map((group) => group.group_id)))
    setExpanded(new Set())
    setShowRuns(false)
  }

  const anyBusy = Boolean(busy) || loadingPreview

  return (
    <div className="flex h-[calc(100vh-6rem)] min-h-[560px] flex-col gap-2">
      <div className="shrink-0 rounded-lg border bg-card shadow-none">
        <div className="flex min-h-10 items-center justify-between gap-3 border-b px-3 py-1.5">
          <div className="flex min-w-0 items-center gap-3">
            <h1 className="shrink-0 text-lg font-semibold tracking-tight text-foreground">รายงานบัตรเครดิต</h1>
            <div className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
              <Info className="h-3.5 w-3.5 shrink-0" />
              <span className="truncate">ข้อมูลจาก BillFlow สำหรับเทียบ statement เอง, ยังไม่รวมยอดคืนเงิน/ยอดติดลบจาก statement</span>
            </div>
          </div>
          <div className="relative flex shrink-0 items-center gap-2">
            <Button variant="outline" size="sm" className="h-8 px-2.5 text-xs" onClick={() => setShowRuns((value) => !value)}>
              <History className="mr-1.5 h-3.5 w-3.5" />
              รอบล่าสุด
            </Button>
            <Button variant="outline" size="sm" className="h-8 px-2.5 text-xs" onClick={refreshRuns} disabled={loadingRuns}>
              <RefreshCw className={cn('mr-1.5 h-3.5 w-3.5', loadingRuns && 'animate-spin')} />
              รีเฟรช
            </Button>
            {showRuns && (
              <div className="absolute right-0 top-9 z-40 w-[min(620px,calc(100vw-18rem))] rounded-md border bg-popover p-2 text-popover-foreground shadow-lg">
                <div className="mb-2 flex items-center justify-between gap-2 px-1">
                  <div className="text-sm font-semibold">รอบรายงานล่าสุด</div>
                  {loadingRuns && <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />}
                </div>
                {runs.length === 0 && !loadingRuns ? (
                  <div className="px-2 py-6 text-center text-sm text-muted-foreground">ยังไม่มีรอบรายงานที่บันทึกไว้</div>
                ) : (
                  <div className="max-h-[360px] space-y-1 overflow-y-auto">
                    {runs.map((run) => (
                      <div
                        key={run.id}
                        className={cn(
                          'flex items-center justify-between gap-3 rounded-md border px-2 py-2 text-sm',
                          activeRun?.id === run.id && 'border-primary/40 bg-primary/5',
                        )}
                      >
                        <div className="min-w-0">
                          <div className="flex min-w-0 items-center gap-2">
                            <div className="truncate font-medium">{run.report_name || 'รายงานบัตรเครดิต'}</div>
                            {run.exported_at && (
                              <Badge variant="outline" className="h-5 shrink-0 px-1.5 text-[11px] border-success/30 bg-success/10 text-success">
                                <CheckCircle2 className="mr-1 h-3 w-3" />
                                export
                              </Badge>
                            )}
                            {run.printed_at && <Badge variant="outline" className="h-5 shrink-0 px-1.5 text-[11px]">พิมพ์</Badge>}
                          </div>
                          <div className="mt-0.5 truncate text-xs text-muted-foreground">
                            {run.filters.date_from} ถึง {run.filters.date_to} · {run.filters.payment_method || 'ทุกบัตร'} · {numberLabel(run.summary.group_count)} ยอดรูดบัตร · {money(run.summary.charge_total)}
                          </div>
                        </div>
                        <div className="flex shrink-0 gap-1">
                          <Button variant="outline" size="sm" className="h-7 px-2 text-xs" onClick={() => openRun(run)}>เปิด</Button>
                          <Button variant="outline" size="sm" className="h-7 px-2 text-xs" onClick={() => printRun(run)} disabled={Boolean(busy)}>
                            <Printer className="h-3.5 w-3.5" />
                          </Button>
                          <Button size="sm" className="h-7 px-2 text-xs" onClick={() => exportRun(run)} disabled={Boolean(busy)}>
                            <Download className="h-3.5 w-3.5" />
                          </Button>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2 px-3 py-2">
          <div className="w-full sm:w-[250px]">
            <DateRangePicker
              from={filter.date_from}
              to={filter.date_to}
              onFromChange={(value) => setFilterValue('date_from', value)}
              onToChange={(value) => setFilterValue('date_to', value)}
              className="h-7 w-full min-w-0 text-xs"
              description="ใช้กรองรายการยอดรูดบัตรตามวันที่/เวลาจากอีเมล"
              clearLabel="ล้างช่วงวันที่รายงาน"
            />
          </div>
          <div className="flex items-center gap-1.5">
            <span className="text-[11px] font-medium text-muted-foreground">บัตร</span>
            <Select value={filter.payment_method || ALL} onValueChange={(value) => setFilterValue('payment_method', value)}>
              <SelectTrigger className="h-7 w-[132px] px-2 text-xs">
                <SelectValue placeholder="ทุกบัตร" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={ALL}>ทุกบัตร</SelectItem>
                {DEFAULT_MARKETPLACE_PRINT_PAYMENT_METHODS.filter((method) => method.startsWith('TT')).map((method) => (
                  <SelectItem key={method} value={method}>{method}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center gap-1.5">
            <span className="text-[11px] font-medium text-muted-foreground">ช่องทาง</span>
            <Select value={filter.source || ALL} onValueChange={(value) => setFilterValue('source', value)}>
              <SelectTrigger className="h-7 w-[128px] px-2 text-xs">
                <SelectValue placeholder="ทุกช่องทาง" />
              </SelectTrigger>
              <SelectContent>
                {SOURCE_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div
            className="flex h-7 items-center gap-1.5 rounded-md border px-2 text-xs"
            title={INCLUDE_INCOMPLETE_HELP}
          >
            <label className="flex items-center gap-1.5">
              <Switch
                checked={Boolean(filter.include_incomplete)}
                onCheckedChange={(checked) => setFilterValue('include_incomplete', checked)}
                aria-label="แสดงรายการต้องตรวจเพิ่ม"
              />
              <span>แสดงรายการต้องตรวจเพิ่ม</span>
            </label>
            <TooltipProvider delayDuration={120}>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Info className="h-3.5 w-3.5 cursor-help text-muted-foreground" />
                </TooltipTrigger>
                <TooltipContent className="max-w-xs text-xs leading-relaxed">
                  {INCLUDE_INCOMPLETE_HELP}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          </div>
          <Button className="h-7 px-3 text-xs" onClick={loadPreview} disabled={loadingPreview}>
            {loadingPreview ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : <RefreshCw className="mr-1.5 h-3.5 w-3.5" />}
            Preview
          </Button>
        </div>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 border-t bg-muted/20 px-3 py-1.5">
          <SummaryItem label="เลือก" value={`${numberLabel(selectedRows.length)}/${numberLabel(preview?.summary.group_count ?? 0)} ยอดรูดบัตร`} />
          <SummaryItem label="คำสั่งซื้อ" value={numberLabel(selectedOrderCount)} />
          <SummaryItem label="ยอดรูดบัตร" value={money(selectedChargeTotal)} />
          <SummaryItem label="ยอดรวมบิลใน BillFlow" value={money(selectedOrderTotal)} />
          <SummaryItem label="ต้องตรวจ" value={numberLabel(selectedIssueCount)} tone={selectedIssueCount > 0 ? 'warn' : undefined} />
          {activeRun && (
            <div className="min-w-0 truncate text-xs text-muted-foreground">
              snapshot: <span className="font-medium text-foreground">{activeRun.report_name || activeRun.id.slice(0, 8)}</span>
            </div>
          )}
        </div>
      </div>

      <div className="flex min-h-0 flex-1 flex-col rounded-lg border bg-card shadow-none">
        <div className="flex shrink-0 items-center justify-between gap-3 border-b px-3 py-2">
          <div className="min-w-0">
            <div className="text-sm font-semibold">รายการยอดรูดบัตร</div>
            <div className="truncate text-xs text-muted-foreground">เลือกทั้งยอดรูดบัตร เพื่อไม่ให้ยอด statement แตกเป็นราย order</div>
          </div>
          <div className="flex shrink-0 flex-wrap gap-2">
            <Button variant="outline" size="sm" className="h-8 px-2.5 text-xs" onClick={selectAllPreview} disabled={!preview || preview.groups.length === 0}>
              เลือกทั้งหมด
            </Button>
            <Button variant="outline" size="sm" className="h-8 px-2.5 text-xs" onClick={clearSelection} disabled={selected.size === 0}>
              ล้างที่เลือก
            </Button>
            <Button variant="outline" size="sm" className="h-8 px-2.5 text-xs" onClick={handleCreateAndPrint} disabled={!preview || selected.size === 0 || anyBusy}>
              {busy === 'create-print' || busy === 'print' ? <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" /> : <Printer className="mr-2 h-3.5 w-3.5" />}
              พิมพ์รายการที่เลือก
            </Button>
            <Button size="sm" className="h-8 px-2.5 text-xs" onClick={() => setExportDialogOpen(true)} disabled={!preview || selected.size === 0 || anyBusy}>
              {busy === 'create-export' || busy === 'export' ? <Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" /> : <Download className="mr-2 h-3.5 w-3.5" />}
              Export Excel
            </Button>
          </div>
        </div>
        <div className="min-h-0 flex-1 overflow-auto">
          {!preview && !loadingPreview && (
            <div className="flex h-full items-center justify-center p-6">
              <EmptyState
                icon={FileSpreadsheet}
                title="ยังไม่มี preview รายงาน"
                description="เลือกช่วงวันที่และบัตร แล้วกด Preview เพื่อดูยอดรูดบัตรจาก BillFlow"
              />
            </div>
          )}
          {loadingPreview && (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              <div className="text-center">
                <Loader2 className="mx-auto mb-3 h-6 w-6 animate-spin" />
                กำลังโหลดข้อมูลรายงาน...
              </div>
            </div>
          )}
          {preview && preview.groups.length === 0 && !loadingPreview && (
            <div className="flex h-full items-center justify-center p-6">
              <EmptyState
                icon={FileSpreadsheet}
                title="ไม่พบรายการยอดรูดบัตรในช่วงนี้"
                description="ลองขยายช่วงวันที่ เปลี่ยนบัตร หรือเปิดแสดงรายการต้องตรวจเพิ่ม"
              />
            </div>
          )}
          {preview && preview.groups.length > 0 && (
            <table className="w-full min-w-[980px] border-collapse text-xs">
              <thead className="sticky top-0 z-20 bg-muted/95 shadow-[0_1px_0_hsl(var(--border))]">
                <tr>
                  <th className="h-8 w-8 px-2 text-left font-medium text-muted-foreground"></th>
                  <th className="h-8 w-8 px-1 text-left font-medium text-muted-foreground"></th>
                  <th className="h-8 w-[162px] px-2 text-left font-medium text-muted-foreground">
                    <HeaderHelp help={EMAIL_TIME_HELP}>วันที่/เวลาจากอีเมล</HeaderHelp>
                  </th>
                  <th className="h-8 w-[84px] px-2 text-left font-medium text-muted-foreground">ช่องทาง</th>
                  <th className="h-8 w-[110px] px-2 text-left font-medium text-muted-foreground">วิธีชำระ</th>
                  <th className="h-8 w-[120px] px-2 text-right font-medium text-muted-foreground">
                    <HeaderHelp help={CHARGE_AMOUNT_HELP} align="right">ยอดรูดบัตร</HeaderHelp>
                  </th>
                  <th className="h-8 w-[138px] px-2 text-right font-medium text-muted-foreground">ยอดรวมบิลใน BillFlow</th>
                  <th className="h-8 w-[112px] px-2 text-right font-medium text-muted-foreground">ต่างจากยอดรูด</th>
                  <th className="h-8 w-[88px] px-2 text-left font-medium text-muted-foreground">POL</th>
                  <th className="h-8 w-[170px] px-2 text-left font-medium text-muted-foreground">สถานะ</th>
                </tr>
              </thead>
              <tbody>
                  {preview.groups.map((group, idx) => {
                    const isSelected = selected.has(group.group_id)
                    const isExpanded = expanded.has(group.group_id)
                    return (
                      <Fragment key={group.group_id}>
                        <tr
                          key={group.group_id}
                          data-state={isSelected ? 'selected' : undefined}
                          className="border-b transition-colors hover:bg-muted/40 data-[state=selected]:bg-muted/60"
                        >
                          <td className="px-2 py-1.5 align-middle">
                            <Checkbox
                              checked={isSelected}
                              onCheckedChange={(checked) => toggleSelected(group.group_id, Boolean(checked))}
                              aria-label={`เลือกยอดรูดบัตร ${idx + 1}`}
                            />
                          </td>
                          <td className="px-1 py-1.5 align-middle">
                            <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => toggleExpanded(group.group_id)} title="ดูคำสั่งซื้อในยอดนี้">
                              {isExpanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
                            </Button>
                          </td>
                          <td className="px-2 py-1.5 align-middle" title={dateTimeLabel(group.charge_time)}>
                            <span className="font-medium tabular-nums">{compactDateTimeLabel(group.charge_time)}</span>
                            <span className="ml-1 text-[11px] text-muted-foreground">#{idx + 1}</span>
                          </td>
                          <td className="px-2 py-1.5 align-middle">
                            <Badge variant="outline" className={cn(
                              'h-5 px-1.5 text-[11px]',
                              group.source === 'lazada_email' && 'border-indigo-300 bg-indigo-50 text-indigo-700',
                              group.source === 'shopee_shipped' && 'border-orange-300 bg-orange-50 text-orange-700',
                            )}>
                              {group.source_label}
                            </Badge>
                          </td>
                          <td className="max-w-[110px] px-2 py-1.5 align-middle">
                            <div className="truncate font-medium" title={group.payment_methods.join(', ') || '-'}>
                              {group.payment_methods.join(', ') || '-'}
                            </div>
                          </td>
                          <td className="px-2 py-1.5 text-right align-middle font-semibold tabular-nums">{money(group.charge_amount)}</td>
                          <td className="px-2 py-1.5 text-right align-middle tabular-nums">{money(group.order_total)}</td>
                          <td className={cn('px-2 py-1.5 text-right align-middle tabular-nums', Math.abs(group.diff ?? 0) > 0.01 && 'font-semibold text-warning')}>
                            {money(group.diff)}
                          </td>
                          <td className="px-2 py-1.5 align-middle" title={`${numberLabel(group.pol_count)} POL จาก ${numberLabel(group.order_count)} คำสั่งซื้อ`}>
                            <span className="font-medium tabular-nums">POL {numberLabel(group.pol_count)}/{numberLabel(group.order_count)}</span>
                          </td>
                          <td className="px-2 py-1.5 align-middle">
                            <IssueChips group={group} />
                          </td>
                        </tr>
                        {isExpanded && (
                          <tr key={`${group.group_id}-orders`} className="border-b bg-muted/20">
                            <td colSpan={10} className="p-0">
                              <div className="px-10 py-2">
                                {group.issues.length > 0 && (
                                  <div className="mb-2 flex flex-wrap gap-1">
                                    {group.issues.map((issue) => (
                                      <Badge key={`${group.group_id}-detail-${issue.code}`} variant="outline" className={cn('h-5 px-1.5 text-[11px]', issueTone(issue.code))}>
                                        {issue.message}
                                      </Badge>
                                    ))}
                                  </div>
                                )}
                                <div className="max-h-56 overflow-auto rounded-md border bg-background">
                                  <table className="w-full min-w-[840px] border-collapse text-xs">
                                    <thead className="sticky top-0 z-10 bg-background shadow-[0_1px_0_hsl(var(--border))]">
                                      <tr>
                                        <th className="h-8 px-2 text-left font-medium text-muted-foreground">POL</th>
                                        <th className="h-8 px-2 text-left font-medium text-muted-foreground">เลขคำสั่งซื้อ</th>
                                        <th className="h-8 px-2 text-left font-medium text-muted-foreground">ผู้ขาย</th>
                                        <th className="h-8 px-2 text-left font-medium text-muted-foreground">วิธีชำระ</th>
                                        <th className="h-8 px-2 text-right font-medium text-muted-foreground">ยอดรวมบิลใน BillFlow</th>
                                        <th className="h-8 px-2 text-left font-medium text-muted-foreground">doc_ref</th>
                                        <th className="h-8 px-2 text-left font-medium text-muted-foreground">สถานะ</th>
                                      </tr>
                                    </thead>
                                    <tbody>
                                      {group.orders.map((order) => (
                                        <tr key={order.bill_id} className="border-b last:border-0">
                                          <td className="px-2 py-2 font-mono">
                                            {order.sml_doc_no || <span className="text-muted-foreground">ยังไม่มี POL</span>}
                                          </td>
                                          <td className="px-2 py-2">
                                            <Link className="font-mono text-primary hover:underline" to={`/bills/${order.bill_id}`}>
                                              {displayOrderID(order.order_id) || order.bill_id.slice(0, 8)}
                                            </Link>
                                          </td>
                                          <td className="max-w-[220px] truncate px-2 py-2" title={order.seller_name}>
                                            {order.seller_name || '-'}
                                          </td>
                                          <td className="px-2 py-2">
                                            {order.effective_print_payment_method || order.print_payment_method || '-'}
                                          </td>
                                          <td className="px-2 py-2 text-right tabular-nums">{money(order.order_total)}</td>
                                          <td className="px-2 py-2 font-mono">{order.doc_ref || '-'}</td>
                                          <td className="px-2 py-2">{order.status}</td>
                                        </tr>
                                      ))}
                                    </tbody>
                                  </table>
                                </div>
                                {!group.print_ready && (
                                  <div className="mt-2 text-xs text-muted-foreground">
                                    พิมพ์จากรายงานไม่ได้: {group.print_block_reason || 'ยังไม่พร้อมพิมพ์'}
                                  </div>
                                )}
                              </div>
                            </td>
                          </tr>
                        )}
                      </Fragment>
                    )
                  })}
              </tbody>
            </table>
          )}
        </div>
      </div>
      <Dialog open={exportDialogOpen} onOpenChange={(open) => {
        if (!anyBusy) setExportDialogOpen(open)
      }}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>ยืนยัน Export Excel</DialogTitle>
            <DialogDescription className="leading-relaxed">
              ระบบจะสร้าง snapshot รอบรายงานจากรายการที่เลือก แล้วดาวน์โหลดไฟล์ Excel จาก snapshot นี้
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-3">
            <div className="grid grid-cols-2 gap-2 text-sm sm:grid-cols-4">
              <div className="rounded-md border px-3 py-2">
                <div className="text-[11px] text-muted-foreground">ยอดรูดบัตร</div>
                <div className="mt-1 font-semibold tabular-nums">{numberLabel(selectedRows.length)}</div>
              </div>
              <div className="rounded-md border px-3 py-2">
                <div className="text-[11px] text-muted-foreground">คำสั่งซื้อ</div>
                <div className="mt-1 font-semibold tabular-nums">{numberLabel(selectedOrderCount)}</div>
              </div>
              <div className="rounded-md border px-3 py-2">
                <div className="text-[11px] text-muted-foreground">ยอดรูดบัตรรวม</div>
                <div className="mt-1 font-semibold tabular-nums">{money(selectedChargeTotal)}</div>
              </div>
              <div className="rounded-md border px-3 py-2">
                <div className="text-[11px] text-muted-foreground">ต้องตรวจ</div>
                <div className={cn('mt-1 font-semibold tabular-nums', selectedIssueCount > 0 && 'text-warning')}>
                  {numberLabel(selectedIssueCount)}
                </div>
              </div>
            </div>

            {selectedIssueCount > 0 ? (
              <div className="rounded-md border border-warning/30 bg-warning/10 px-3 py-2 text-xs leading-relaxed text-warning">
                มีรายการที่ต้องตรวจอยู่ในชุดนี้ เช่น ยังไม่มี POL, ยอดต่าง, หรือข้อมูลวิธีชำระไม่ครบ รายการเหล่านี้จะถูกใส่ในไฟล์ Excel เพื่อให้บัญชีตรวจต่อ
              </div>
            ) : (
              <div className="rounded-md border border-success/30 bg-success/10 px-3 py-2 text-xs leading-relaxed text-success">
                รายการที่เลือกไม่มี issue ที่ระบบเตือนใน preview ปัจจุบัน
              </div>
            )}
          </div>

          <DialogFooter className="gap-2 sm:gap-2">
            <Button type="button" variant="outline" onClick={() => setExportDialogOpen(false)} disabled={anyBusy}>
              ยกเลิก
            </Button>
            <Button type="button" onClick={handleCreateAndExport} disabled={anyBusy || selected.size === 0}>
              {busy === 'create-export' || busy === 'export' ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : (
                <Download className="mr-2 h-4 w-4" />
              )}
              Export Excel
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
