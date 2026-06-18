import { Fragment, useCallback, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Download,
  FileSpreadsheet,
  Loader2,
  Printer,
  RefreshCw,
} from 'lucide-react'
import { toast } from 'sonner'

import { PageHeader } from '@/components/common/PageHeader'
import { EmptyState } from '@/components/common/EmptyState'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
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
  CreditCardReportPreview,
  CreditCardReportRun,
} from '@/types'

const ALL = 'all'

const SOURCE_OPTIONS = [
  { value: ALL, label: 'ทุกช่องทาง' },
  { value: 'shopee_shipped', label: 'Shopee' },
  { value: 'lazada_email', label: 'Lazada' },
]

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

function IssueChips({ group }: { group: CreditCardReportGroup }) {
  if (group.issues.length === 0) {
    return (
      <Badge variant="outline" className="border-success/30 bg-success/10 text-success">
        พร้อม
      </Badge>
    )
  }
  return (
    <div className="flex flex-wrap gap-1">
      {group.issues.map((issue) => (
        <Badge key={`${group.group_id}-${issue.code}`} variant="outline" className={issueTone(issue.code)}>
          {issue.message}
        </Badge>
      ))}
    </div>
  )
}

function StatCard({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <Card className="shadow-none">
      <CardContent className="p-4">
        <div className="text-xs text-muted-foreground">{label}</div>
        <div className="mt-1 text-xl font-semibold tabular-nums">{value}</div>
        {hint && <div className="mt-1 text-xs text-muted-foreground">{hint}</div>}
      </CardContent>
    </Card>
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
  const [busy, setBusy] = useState('')

  const selectedRows = useMemo(() => selectedGroups(preview, selected), [preview, selected])
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

  const createSnapshotRun = async () => {
    if (!preview || selected.size === 0) {
      toast.error('กรุณาเลือกรายการอย่างน้อย 1 ยอดรูด')
      return null
    }
    if (selectedIssueCount > 0) {
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
      const run = await createSnapshotRun()
      if (run) await exportRun(run)
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
  }

  const anyBusy = Boolean(busy) || loadingPreview

  return (
    <div className="space-y-5">
      <PageHeader
        title="รายงานบัตรเครดิต"
        description="Export ข้อมูลยอดรูดจาก BillFlow เพื่อให้ทีมบัญชีนำไปเทียบ statement เอง โดยไม่ import statement เข้าระบบ"
        actions={
          <Button variant="outline" onClick={refreshRuns} disabled={loadingRuns}>
            <RefreshCw className={cn('mr-2 h-4 w-4', loadingRuns && 'animate-spin')} />
            รีเฟรช
          </Button>
        }
      />

      <Alert className="border-info/25 bg-info/5">
        <AlertTriangle className="h-4 w-4" />
        <AlertTitle>รายงานนี้อ้างอิงข้อมูล BillFlow เท่านั้น</AlertTitle>
        <AlertDescription>
          ยอดคืนสินค้า/ยอดติดลบที่มีเฉพาะใน statement ธนาคารจะไม่ถูกสร้างเป็นบิลในรอบนี้ และวันหัว-ท้ายรอบสามารถติ๊กเลือกเฉพาะกลุ่มยอดรูดที่ต้องการก่อน export ได้
        </AlertDescription>
      </Alert>

      <Card className="shadow-none">
        <CardHeader className="pb-3">
          <CardTitle className="text-base">ตัวกรองรายงาน</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid gap-3 lg:grid-cols-[160px_160px_170px_170px_minmax(180px,1fr)_auto]">
            <div>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">วันที่เริ่มต้น</label>
              <Input
                type="date"
                value={filter.date_from}
                onChange={(e) => setFilterValue('date_from', e.target.value)}
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">วันที่สิ้นสุด</label>
              <Input
                type="date"
                value={filter.date_to}
                onChange={(e) => setFilterValue('date_to', e.target.value)}
              />
            </div>
            <div>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">บัตร / วิธีชำระเงิน</label>
              <Select value={filter.payment_method || ALL} onValueChange={(value) => setFilterValue('payment_method', value)}>
                <SelectTrigger>
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
            <div>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">ช่องทาง</label>
              <Select value={filter.source || ALL} onValueChange={(value) => setFilterValue('source', value)}>
                <SelectTrigger>
                  <SelectValue placeholder="ทุกช่องทาง" />
                </SelectTrigger>
                <SelectContent>
                  {SOURCE_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <label className="flex min-h-10 items-center gap-3 rounded-md border px-3 py-2 text-sm">
              <Switch
                checked={Boolean(filter.include_incomplete)}
                onCheckedChange={(checked) => setFilterValue('include_incomplete', checked)}
              />
              <span>
                รวมข้อมูลไม่ครบ
                <span className="block text-xs text-muted-foreground">เช่น ยังไม่มียอดรูดหรือ group key</span>
              </span>
            </label>
            <div className="flex items-end">
              <Button className="w-full" onClick={loadPreview} disabled={loadingPreview}>
                {loadingPreview ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <RefreshCw className="mr-2 h-4 w-4" />}
                Preview
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
        <StatCard label="ยอดรูดที่เลือก" value={numberLabel(selectedRows.length)} hint={`จากทั้งหมด ${numberLabel(preview?.summary.group_count ?? 0)} กลุ่ม`} />
        <StatCard label="คำสั่งซื้อที่เลือก" value={numberLabel(selectedRows.reduce((sum, group) => sum + group.order_count, 0))} />
        <StatCard label="ยอดรูดรวม" value={money(selectedChargeTotal)} />
        <StatCard label="ยอดบิลรวม" value={money(selectedOrderTotal)} />
        <StatCard label="ต้องตรวจสอบ" value={numberLabel(selectedIssueCount)} hint="ยัง export ได้หลังยืนยัน" />
      </div>

      <Card className="shadow-none">
        <CardHeader className="gap-3 pb-3 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <CardTitle className="text-base">กลุ่มยอดรูดจาก BillFlow</CardTitle>
            <div className="mt-1 text-sm text-muted-foreground">
              เลือกเป็นกลุ่มยอดรูดเท่านั้น เพื่อไม่ให้ยอดใน statement แตกเป็นราย order ผิดกลุ่ม
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" onClick={selectAllPreview} disabled={!preview || preview.groups.length === 0}>
              เลือกทั้งหมด
            </Button>
            <Button variant="outline" size="sm" onClick={clearSelection} disabled={selected.size === 0}>
              ล้างที่เลือก
            </Button>
            <Button variant="outline" size="sm" onClick={handleCreateAndPrint} disabled={!preview || selected.size === 0 || anyBusy}>
              {busy === 'create-print' || busy === 'print' ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Printer className="mr-2 h-4 w-4" />}
              พิมพ์ตามลำดับ
            </Button>
            <Button size="sm" onClick={handleCreateAndExport} disabled={!preview || selected.size === 0 || anyBusy}>
              {busy === 'create-export' || busy === 'export' ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : <Download className="mr-2 h-4 w-4" />}
              สร้างรอบและ Export
            </Button>
          </div>
        </CardHeader>
        <CardContent className="pt-0">
          {!preview && !loadingPreview && (
            <EmptyState
              icon={FileSpreadsheet}
              title="ยังไม่มี preview รายงาน"
              description="เลือกช่วงวันที่และบัตร แล้วกด Preview เพื่อดูยอดรูดจาก BillFlow"
            />
          )}
          {loadingPreview && (
            <div className="py-12 text-center text-sm text-muted-foreground">
              <Loader2 className="mx-auto mb-3 h-6 w-6 animate-spin" />
              กำลังโหลดข้อมูลรายงาน...
            </div>
          )}
          {preview && preview.groups.length === 0 && !loadingPreview && (
            <EmptyState
              icon={FileSpreadsheet}
              title="ไม่พบกลุ่มยอดรูดในช่วงนี้"
              description="ลองขยายช่วงวันที่ เปลี่ยนบัตร หรือเปิดรวมข้อมูลไม่ครบ"
            />
          )}
          {preview && preview.groups.length > 0 && (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/50">
                    <TableHead className="w-10 px-3"></TableHead>
                    <TableHead className="w-10 px-3"></TableHead>
                    <TableHead>วันที่/เวลา BillFlow</TableHead>
                    <TableHead>ช่องทาง</TableHead>
                    <TableHead>วิธีชำระเงิน</TableHead>
                    <TableHead className="text-right">ยอดรูด</TableHead>
                    <TableHead className="text-right">ยอดบิล</TableHead>
                    <TableHead className="text-right">ส่วนต่าง</TableHead>
                    <TableHead>POL / Order</TableHead>
                    <TableHead>สถานะ</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {preview.groups.map((group, idx) => {
                    const isSelected = selected.has(group.group_id)
                    const isExpanded = expanded.has(group.group_id)
                    return (
                      <Fragment key={group.group_id}>
                        <TableRow key={group.group_id} data-state={isSelected ? 'selected' : undefined}>
                          <TableCell className="px-3">
                            <Checkbox
                              checked={isSelected}
                              onCheckedChange={(checked) => toggleSelected(group.group_id, Boolean(checked))}
                              aria-label={`เลือกยอดรูด ${idx + 1}`}
                            />
                          </TableCell>
                          <TableCell className="px-3">
                            <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => toggleExpanded(group.group_id)}>
                              {isExpanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
                            </Button>
                          </TableCell>
                          <TableCell className="min-w-[170px]">
                            <div className="font-medium tabular-nums">{dateTimeLabel(group.charge_time)}</div>
                            <div className="text-xs text-muted-foreground">ลำดับ {idx + 1}</div>
                          </TableCell>
                          <TableCell>
                            <Badge variant="outline" className={cn(
                              group.source === 'lazada_email' && 'border-indigo-300 bg-indigo-50 text-indigo-700',
                              group.source === 'shopee_shipped' && 'border-orange-300 bg-orange-50 text-orange-700',
                            )}>
                              {group.source_label}
                            </Badge>
                          </TableCell>
                          <TableCell className="max-w-[180px]">
                            <div className="truncate font-medium" title={group.payment_methods.join(', ') || '-'}>
                              {group.payment_methods.join(', ') || '-'}
                            </div>
                          </TableCell>
                          <TableCell className="text-right font-semibold tabular-nums">{money(group.charge_amount)}</TableCell>
                          <TableCell className="text-right tabular-nums">{money(group.order_total)}</TableCell>
                          <TableCell className={cn('text-right tabular-nums', Math.abs(group.diff ?? 0) > 0.01 && 'font-semibold text-warning')}>
                            {money(group.diff)}
                          </TableCell>
                          <TableCell>
                            <div className="font-medium tabular-nums">
                              POL {numberLabel(group.pol_count)} / {numberLabel(group.order_count)}
                            </div>
                            <div className="text-xs text-muted-foreground">{numberLabel(group.order_count)} คำสั่งซื้อ</div>
                          </TableCell>
                          <TableCell className="min-w-[220px]">
                            <IssueChips group={group} />
                          </TableCell>
                        </TableRow>
                        {isExpanded && (
                          <TableRow key={`${group.group_id}-orders`} className="bg-muted/20 hover:bg-muted/20">
                            <TableCell colSpan={10} className="p-0">
                              <div className="px-12 py-3">
                                <div className="rounded-md border bg-background">
                                  <Table>
                                    <TableHeader>
                                      <TableRow>
                                        <TableHead>POL</TableHead>
                                        <TableHead>เลขคำสั่งซื้อ</TableHead>
                                        <TableHead>ผู้ขาย</TableHead>
                                        <TableHead>วิธีชำระเงิน</TableHead>
                                        <TableHead className="text-right">ยอดบิล</TableHead>
                                        <TableHead>doc_ref</TableHead>
                                        <TableHead>สถานะ</TableHead>
                                      </TableRow>
                                    </TableHeader>
                                    <TableBody>
                                      {group.orders.map((order) => (
                                        <TableRow key={order.bill_id}>
                                          <TableCell className="font-mono">
                                            {order.sml_doc_no || <span className="text-muted-foreground">ยังไม่มี POL</span>}
                                          </TableCell>
                                          <TableCell>
                                            <Link className="font-mono text-primary hover:underline" to={`/bills/${order.bill_id}`}>
                                              {order.order_id || order.bill_id.slice(0, 8)}
                                            </Link>
                                          </TableCell>
                                          <TableCell className="max-w-[260px] truncate" title={order.seller_name}>
                                            {order.seller_name || '-'}
                                          </TableCell>
                                          <TableCell>
                                            {order.effective_print_payment_method || order.print_payment_method || '-'}
                                          </TableCell>
                                          <TableCell className="text-right tabular-nums">{money(order.order_total)}</TableCell>
                                          <TableCell className="font-mono">{order.doc_ref || '-'}</TableCell>
                                          <TableCell>{order.status}</TableCell>
                                        </TableRow>
                                      ))}
                                    </TableBody>
                                  </Table>
                                </div>
                                {!group.print_ready && (
                                  <div className="mt-2 text-xs text-muted-foreground">
                                    พิมพ์จากรายงานไม่ได้: {group.print_block_reason || 'ยังไม่พร้อมพิมพ์'}
                                  </div>
                                )}
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
          )}
        </CardContent>
      </Card>

      <Card className="shadow-none">
        <CardHeader className="pb-3">
          <CardTitle className="text-base">รอบรายงานล่าสุด</CardTitle>
        </CardHeader>
        <CardContent>
          {runs.length === 0 && !loadingRuns ? (
            <div className="text-sm text-muted-foreground">ยังไม่มีรอบรายงานที่บันทึกไว้</div>
          ) : (
            <div className="space-y-2">
              {runs.map((run) => (
                <div
                  key={run.id}
                  className={cn(
                    'flex flex-col gap-3 rounded-md border p-3 md:flex-row md:items-center md:justify-between',
                    activeRun?.id === run.id && 'border-primary/40 bg-primary/5',
                  )}
                >
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <div className="truncate font-medium">{run.report_name || 'รายงานบัตรเครดิต'}</div>
                      {run.exported_at && <Badge variant="outline" className="border-success/30 bg-success/10 text-success"><CheckCircle2 className="mr-1 h-3 w-3" />export แล้ว</Badge>}
                      {run.printed_at && <Badge variant="outline">พิมพ์แล้ว</Badge>}
                    </div>
                    <div className="mt-1 text-xs text-muted-foreground">
                      {run.filters.date_from} ถึง {run.filters.date_to} · {run.filters.payment_method || 'ทุกบัตร'} · {numberLabel(run.summary.group_count)} ยอดรูด · {money(run.summary.charge_total)}
                    </div>
                  </div>
                  <div className="flex flex-wrap gap-2">
                    <Button variant="outline" size="sm" onClick={() => openRun(run)}>เปิด snapshot</Button>
                    <Button variant="outline" size="sm" onClick={() => printRun(run)} disabled={Boolean(busy)}>
                      <Printer className="mr-2 h-4 w-4" />
                      พิมพ์
                    </Button>
                    <Button size="sm" onClick={() => exportRun(run)} disabled={Boolean(busy)}>
                      <Download className="mr-2 h-4 w-4" />
                      Export
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
