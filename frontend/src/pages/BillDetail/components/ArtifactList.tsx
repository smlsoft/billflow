import { useEffect, useState } from 'react'
import { createPortal } from 'react-dom'
import { AlertTriangle, CheckCircle2, Download, ExternalLink, Eye, History, Info, Loader2, Paperclip, Printer, Wrench, X } from 'lucide-react'
import { Link } from 'react-router-dom'
import axios from 'axios'
import { toast } from 'sonner'
import dayjs from 'dayjs'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useArtifacts, openArtifact, printArtifact, recordArtifactPrint } from '../hooks/useArtifacts'
import type { ArtifactPrintContext, BillArtifact } from '../hooks/useArtifacts'
import { KIND_META, fmtSize, isUserVisibleArtifact } from '../utils/formatters'
import api from '@/api/client'
import type { BillEmailGroup, BillEmailRelatedBill, EmailPrintEvent, MarketplaceEmailGroupOrder } from '@/types'

interface Props {
  billId: string
  billStatus?: string
  billSource?: string
  smlDocNo?: string
  orderID?: string
  printPaymentMethod?: string
  effectivePrintPaymentMethod?: string
  emailGroup?: BillEmailGroup | null
  smlPayload?: Record<string, unknown> | null
  onReload?: () => Promise<unknown>
  canRepairMarketplaceEmail?: boolean
  autoOpenRepair?: boolean
}

type PrintReadiness = {
  canPrint: boolean
  reason: string
  printContext: ArtifactPrintContext
}

type PrintAPIError = {
  message: string
  description?: string
  shouldReload?: boolean
}

type ShopeeEmailRepairPreview = {
  bill_id: string
  source?: string
  message_id: string
  artifact_id: string
  subject: string
  input_subject?: string
  detected_order_count: number
  existing_count: number
  missing_count: number
  rebuild_count?: number
  blocked_count?: number
  detected_order_ids: string[]
  existing_order_ids: string[]
  missing_order_ids: string[]
  rebuild_order_ids?: string[]
  blocked_order_ids?: string[]
  email_total: number
  has_stale_tombstones?: boolean
  stale_tombstone_order_ids?: string[]
  warnings?: string[]
  can_repair: boolean
}

type ShopeeEmailRepairJob = {
  id: string
  bill_id: string
  message_id: string
  status: 'queued' | 'running' | 'succeeded' | 'failed'
  error?: string
  created_count?: number
  rebuilt_count?: number
  skipped_count?: number
  missing_count?: number
  created_bill_ids?: string[]
  created_order_ids?: string[]
  rebuilt_bill_ids?: string[]
  rebuilt_order_ids?: string[]
  missing_order_ids?: string[]
  progress?: EmailRepairJobProgress
}

type EmailRepairJobProgress = {
  percent?: number
  stage?: string
  label?: string
  current?: number
  total?: number
  current_order_id?: string
}

// EmailPreviewModal renders HTML email content in a sandboxed iframe so the
// browser treats it as a rendered email (layout, images, Thai text) instead of
// a raw text dump in a new tab.
function EmailPreviewModal({
  billId,
  artId,
  filename,
  displayName,
  emailGroup,
  printReadiness,
  onPrinted,
  onClose,
}: {
  billId: string
  artId: string
  filename: string
  displayName: string
  emailGroup?: BillEmailGroup | null
  printReadiness: PrintReadiness
  onPrinted: (artId: string, filename: string) => Promise<void>
  onClose: () => void
}) {
  const [src, setSrc] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  // Fetch once on mount — inject CSS to strip body margin and force all links
  // to open in a new tab (Shopee emails embed <a> without target="_blank").
  useEffect(() => {
    let alive = true
    let objectURL = ''
    api
      .get(`/api/bills/${billId}/artifacts/${artId}/preview`, { responseType: 'blob' })
      .then((res) => {
        if (!alive) return
        const ct = (res.headers['content-type'] ?? '').toString() || 'text/html; charset=utf-8'
        return res.data.text().then((html: string) => {
          // Reset body margin so the email starts at the top of the iframe,
          // and patch every <a> to open in a new tab.
          const resetCss = `<style>*{box-sizing:border-box}html,body{margin:0!important;padding:0!important;background:#fff!important}img{display:block;max-width:100%}table{margin:0!important}</style>`
          const patched = html
            .replace(/<head([^>]*)>/i, `<head$1>${resetCss}`)
            .replace(/<a\s/gi, '<a target="_blank" rel="noopener noreferrer" ')
          const blob = new Blob([patched], { type: ct })
          objectURL = URL.createObjectURL(blob)
          if (alive) setSrc(objectURL)
        })
      })
      .catch(() => {
        if (alive) toast.error('เปิดตัวอย่างอีเมลไม่สำเร็จ')
      })
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => {
      alive = false
      if (objectURL) URL.revokeObjectURL(objectURL)
    }
  }, [artId, billId])

  const handlePrint = async () => {
    await onPrinted(artId, filename)
  }

  const handleClose = () => {
    onClose()
  }

  const duplicateNote = emailDuplicateNote(emailGroup)

  return createPortal(
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/75 p-4"
      onClick={handleClose}
    >
      <div
        className="relative flex h-[90vh] w-full max-w-4xl flex-col overflow-hidden rounded-lg border border-border bg-background text-foreground shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between gap-3 border-b border-border bg-card px-4 py-2">
          <div className="min-w-0">
            <span className="block truncate text-sm font-medium text-foreground">{displayName}</span>
            <span className="mt-0.5 block truncate text-[11px] text-muted-foreground">{filename}</span>
            {duplicateNote && (
              <span className="mt-0.5 block truncate text-xs text-warning">{duplicateNote}</span>
            )}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {printReadiness.canPrint ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                className="h-8 gap-1.5 bg-background"
                onClick={handlePrint}
              >
                <Printer className="h-3.5 w-3.5" />
                พิมพ์
              </Button>
            ) : (
              <TooltipProvider delayDuration={100}>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span className="cursor-not-allowed">
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="h-8 gap-1.5 pointer-events-none bg-background opacity-40"
                        disabled
                      >
                        <Printer className="h-3.5 w-3.5" />
                        พิมพ์
                      </Button>
                    </span>
                  </TooltipTrigger>
                  <TooltipContent side="bottom" className="text-xs">
                    {printReadiness.reason}
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            )}
            <button
              type="button"
              onClick={handleClose}
              title="ปิด"
              className="rounded p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>
        <div className="flex-1 overflow-hidden bg-muted">
          {loading && (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              กำลังโหลด...
            </div>
          )}
          {src && (
            <iframe
              src={src}
              title={filename}
              className="h-full w-full border-0 bg-white"
              sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox"
              referrerPolicy="no-referrer"
            />
          )}
        </div>
      </div>
    </div>,
    document.body
  )
}

function ShopeeEmailRepairDialog({
  billId,
  open,
  onOpenChange,
  onReload,
}: {
  billId: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onReload?: () => Promise<unknown>
}) {
  const [preview, setPreview] = useState<ShopeeEmailRepairPreview | null>(null)
  const [job, setJob] = useState<ShopeeEmailRepairJob | null>(null)
  const [loadingPreview, setLoadingPreview] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [subjectInput, setSubjectInput] = useState('')

  useEffect(() => {
    if (!open) {
      setPreview(null)
      setJob(null)
      setError('')
      setSubjectInput('')
      return
    }
    let alive = true
    const loadPreview = async () => {
      setLoadingPreview(true)
      setError('')
      try {
        const res = await api.get<{ data: ShopeeEmailRepairPreview }>(
          `/api/bills/${billId}/shopee-email-repair/preview`,
        )
        if (!alive) return
        setPreview(res.data.data)
        setSubjectInput((current) => current || res.data.data.subject || '')
      } catch (err) {
        if (!alive) return
        setError(apiErrorMessage(err, 'ตรวจอีเมลต้นฉบับไม่สำเร็จ'))
      } finally {
        if (alive) setLoadingPreview(false)
      }
    }
    void loadPreview()
    return () => {
      alive = false
    }
  }, [billId, open])

  const handlePreview = async () => {
    setLoadingPreview(true)
    setError('')
    setJob(null)
    try {
      const res = await api.get<{ data: ShopeeEmailRepairPreview }>(
        `/api/bills/${billId}/shopee-email-repair/preview`,
        { params: subjectInput.trim() ? { subject: subjectInput.trim() } : undefined },
      )
      setPreview(res.data.data)
      setSubjectInput(res.data.data.subject || subjectInput)
    } catch (err) {
      setError(apiErrorMessage(err, 'ตรวจอีเมลต้นฉบับไม่สำเร็จ'))
    } finally {
      setLoadingPreview(false)
    }
  }

  const polling = job?.status === 'queued' || job?.status === 'running'
  useEffect(() => {
    if (!open || !job?.id || !polling) return
    let alive = true
    const timer = window.setInterval(() => {
      api
        .get<{ data: ShopeeEmailRepairJob }>(`/api/bills/${billId}/shopee-email-repair/jobs/${job.id}`)
        .then((res) => {
          if (!alive) return
          setJob(res.data.data)
          if (res.data.data.status === 'succeeded') {
            void onReload?.()
          }
        })
        .catch((err) => {
          if (!alive) return
          setError(apiErrorMessage(err, 'โหลดสถานะงานซ่อมไม่สำเร็จ'))
        })
    }, 1500)
    return () => {
      alive = false
      window.clearInterval(timer)
    }
  }, [billId, job?.id, onReload, open, polling])

  const handleCreateJob = async () => {
    const rebuildIDs = preview?.rebuild_order_ids ?? []
    if (!preview || submitting || !preview.can_repair || (preview.missing_count === 0 && rebuildIDs.length === 0)) return
    setSubmitting(true)
    setError('')
    try {
      const res = await api.post<{ data: ShopeeEmailRepairJob }>(
        `/api/bills/${billId}/shopee-email-repair/jobs`,
        {
          expected_order_count: preview.detected_order_count,
          expected_total: preview.email_total,
          expected_missing_order_ids: preview.missing_order_ids,
          expected_rebuild_order_ids: rebuildIDs,
          subject: preview.subject,
        },
      )
      setJob(res.data.data)
      toast.success('เริ่มซ่อมคำสั่งซื้อจากอีเมลแล้ว')
    } catch (err) {
      setError(apiErrorMessage(err, 'เริ่มงานซ่อมไม่สำเร็จ'))
    } finally {
      setSubmitting(false)
    }
  }

  const createdBillIds = job?.created_bill_ids ?? []
  const rebuiltBillIds = job?.rebuilt_bill_ids ?? []
  const jobProgress = repairJobProgress(job)
  const missingIDs = preview?.missing_order_ids ?? []
  const rebuildIDs = preview?.rebuild_order_ids ?? []
  const blockedIDs = preview?.blocked_order_ids ?? []
  const hasWarnings = (preview?.warnings?.length ?? 0) > 0

  return (
    <Dialog open={open} onOpenChange={(next) => !submitting && onOpenChange(next)}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>ซ่อมคำสั่งซื้อจากอีเมลยืนยัน</DialogTitle>
          <DialogDescription>
            ระบุหัวข้ออีเมลยืนยัน เพื่อสร้างรายการที่ตกหล่นและซ่อมบิลเดิมจากอีเมลต้นฉบับที่ถูกต้อง
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <label className="text-sm font-medium text-foreground" htmlFor="shopee-repair-subject">
              หัวข้ออีเมลยืนยัน
            </label>
            <div className="flex gap-2">
              <Textarea
                id="shopee-repair-subject"
                value={subjectInput}
                onChange={(event) => setSubjectInput(event.target.value)}
                placeholder={'Shopee: ยืนยันการชำระเงินคำสั่งซื้อหมายเลข #260608HPC8A42A\nLazada: ยืนยันคำสั่งซื้อหมายเลข 1094738208195692'}
                className="min-h-[58px] resize-none text-sm"
                disabled={loadingPreview || submitting || polling}
              />
              <Button
                type="button"
                variant="outline"
                className="h-[58px] shrink-0 gap-1.5"
                onClick={handlePreview}
                disabled={loadingPreview || submitting || polling}
              >
                {loadingPreview ? <Loader2 className="h-4 w-4 animate-spin" /> : <Eye className="h-4 w-4" />}
                ตรวจสอบ
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              ถ้าไม่ระบุ ระบบจะลองใช้อีเมลยืนยันการชำระเงินที่แนบกับบิลนี้ก่อน
            </p>
          </div>

          {loadingPreview && (
            <div className="flex items-center gap-2 rounded-md border border-border bg-muted/25 px-3 py-3 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              กำลังตรวจอีเมลต้นฉบับ...
            </div>
          )}

          {error && (
            <div className="flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          {preview && (
            <>
              <div className="grid gap-2 sm:grid-cols-4">
                <RepairMetric label="พบในอีเมล" value={preview.detected_order_count} suffix="คำสั่งซื้อ" />
                <RepairMetric label="มีใน BillFlow แล้ว" value={preview.existing_count} suffix="ใบ" />
                <RepairMetric label="ตกหล่น" value={preview.missing_count} suffix="ใบ" tone={preview.missing_count > 0 ? 'warning' : 'success'} />
                <RepairMetric label="ยอดรวมในอีเมล" value={formatMoney(preview.email_total)} />
                <RepairMetric label="ซ่อมจากเมลยืนยัน" value={preview.rebuild_count ?? 0} suffix="ใบ" tone={(preview.rebuild_count ?? 0) > 0 ? 'warning' : 'success'} />
                <RepairMetric label="แก้อัตโนมัติไม่ได้" value={preview.blocked_count ?? 0} suffix="ใบ" tone={(preview.blocked_count ?? 0) > 0 ? 'warning' : 'success'} />
              </div>

              <div className="rounded-md border border-border bg-muted/20 px-3 py-2">
                <div className="text-[11px] text-muted-foreground">อีเมลที่จะใช้เป็นต้นฉบับ</div>
                <div className="mt-1 break-words text-sm font-medium text-foreground">{preview.subject}</div>
              </div>

              <div className="rounded-md border border-border bg-muted/20 p-3">
                <div className="mb-2 flex items-center justify-between gap-2">
                  <div className="text-sm font-medium text-foreground">รายการที่ตกหล่น</div>
                  {preview.has_stale_tombstones && (
                    <Badge variant="secondary" className="bg-warning/10 text-warning">
                      มี processed เก่า
                    </Badge>
                  )}
                </div>
                {missingIDs.length > 0 ? (
                  <div className="flex flex-wrap gap-1.5">
                    {missingIDs.map((id) => (
                      <span key={id} className="rounded border border-border bg-background px-2 py-1 font-mono text-xs">
                        {id}
                      </span>
                    ))}
                  </div>
                ) : (
                  <div className="flex items-center gap-2 text-sm text-emerald-600 dark:text-emerald-300">
                    <CheckCircle2 className="h-4 w-4" />
                    อีเมลนี้มีบิลครบแล้ว
                  </div>
                )}
              </div>

              {rebuildIDs.length > 0 && (
                <div className="rounded-md border border-warning/40 bg-warning/10 p-3">
                  <div className="mb-2 text-sm font-medium text-warning">บิลเดิมที่จะซ่อมจากอีเมลยืนยัน</div>
                  <div className="flex flex-wrap gap-1.5">
                    {rebuildIDs.map((id) => (
                      <span key={id} className="rounded border border-warning/40 bg-background px-2 py-1 font-mono text-xs text-warning">
                        {id}
                      </span>
                    ))}
                  </div>
                </div>
              )}

              {blockedIDs.length > 0 && (
                <div className="rounded-md border border-border bg-muted/20 p-3">
                  <div className="mb-2 text-sm font-medium text-muted-foreground">ส่ง SML แล้วหรือซ่อมอัตโนมัติไม่ได้</div>
                  <div className="flex flex-wrap gap-1.5">
                    {blockedIDs.map((id) => (
                      <span key={id} className="rounded border border-border bg-background px-2 py-1 font-mono text-xs text-muted-foreground">
                        {id}
                      </span>
                    ))}
                  </div>
                </div>
              )}

              {hasWarnings && (
                <div className="space-y-1 rounded-md border border-warning/40 bg-warning/10 px-3 py-2 text-xs text-warning">
                  {(preview.warnings ?? []).map((warning) => (
                    <div key={warning}>{warning}</div>
                  ))}
                </div>
              )}
            </>
          )}

          {job && (
            <div className="rounded-md border border-border bg-background p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div>
                  <div className="text-sm font-medium text-foreground">สถานะงานซ่อม</div>
                  <div className="font-mono text-[11px] text-muted-foreground">{job.id}</div>
                </div>
                <JobStatusBadge status={job.status} />
              </div>
              <div className="mt-3 space-y-2">
                <div className="flex items-center justify-between gap-3 text-xs">
                  <span className="text-muted-foreground">
                    {jobProgress.label}
                    {jobProgress.current_order_id ? (
                      <span className="ml-1 font-mono text-foreground">{jobProgress.current_order_id}</span>
                    ) : null}
                  </span>
                  <span className="font-semibold tabular-nums text-foreground">{jobProgress.percent}%</span>
                </div>
                <div
                  className="h-2 overflow-hidden rounded-full bg-muted"
                  role="progressbar"
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-valuenow={jobProgress.percent}
                >
                  <div
                    className="h-full rounded-full bg-primary transition-all duration-500 ease-out"
                    style={{ width: `${jobProgress.percent}%` }}
                  />
                </div>
                {jobProgress.total ? (
                  <div className="text-[11px] text-muted-foreground">
                    ทำไปแล้ว {jobProgress.current ?? 0}/{jobProgress.total} คำสั่งซื้อ
                  </div>
                ) : null}
              </div>
              {job.status === 'failed' && job.error && (
                <div className="mt-2 rounded-md bg-destructive/10 px-2 py-1 text-xs text-destructive">
                  {job.error}
                </div>
              )}
              {job.status === 'succeeded' && (
                <div className="mt-3 space-y-2">
                  <div className="text-sm text-muted-foreground">
                    สร้างใหม่ {job.created_count ?? createdBillIds.length} ใบ, ซ่อมบิลเดิม {job.rebuilt_count ?? rebuiltBillIds.length} ใบ, ข้ามใบที่มีอยู่แล้ว {job.skipped_count ?? 0} ใบ
                  </div>
                  {createdBillIds.length > 0 && (
                    <div className="flex flex-wrap gap-2">
                      {createdBillIds.map((id, index) => (
                        <Button
                          key={id}
                          type="button"
                          variant="outline"
                          size="sm"
                          className="h-8 gap-1.5"
                          onClick={() => window.open(`/bills/${id}`, '_blank', 'noopener,noreferrer')}
                        >
                          <ExternalLink className="h-3.5 w-3.5" />
                          เปิดบิลใหม่ {index + 1}
                        </Button>
                      ))}
                    </div>
                  )}
                  {rebuiltBillIds.length > 0 && (
                    <div className="flex flex-wrap gap-2">
                      {rebuiltBillIds.map((id, index) => (
                        <Button
                          key={id}
                          type="button"
                          variant="outline"
                          size="sm"
                          className="h-8 gap-1.5"
                          onClick={() => window.open(`/bills/${id}`, '_blank', 'noopener,noreferrer')}
                        >
                          <ExternalLink className="h-3.5 w-3.5" />
                          เปิดบิลที่ซ่อม {index + 1}
                        </Button>
                      ))}
                    </div>
                  )}
                </div>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={submitting}>
            ปิด
          </Button>
          {preview && (preview.missing_count > 0 || (preview.rebuild_count ?? 0) > 0) && job?.status !== 'succeeded' && (
            <Button
              type="button"
              className="gap-1.5"
              onClick={handleCreateJob}
              disabled={submitting || polling || !preview.can_repair}
            >
              {submitting || polling ? <Loader2 className="h-4 w-4 animate-spin" /> : <Wrench className="h-4 w-4" />}
              ซ่อมจากอีเมลยืนยัน
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function RepairMetric({
  label,
  value,
  suffix,
  tone = 'default',
}: {
  label: string
  value: number | string
  suffix?: string
  tone?: 'default' | 'warning' | 'success'
}) {
  const toneClass =
    tone === 'warning'
      ? 'text-warning'
      : tone === 'success'
        ? 'text-emerald-600 dark:text-emerald-300'
        : 'text-foreground'
  return (
    <div className="rounded-md border border-border bg-muted/20 px-3 py-2">
      <div className="text-[11px] text-muted-foreground">{label}</div>
      <div className={`mt-0.5 text-lg font-semibold tabular-nums ${toneClass}`}>
        {typeof value === 'number' ? value.toLocaleString('th-TH') : value}
        {suffix && <span className="ml-1 text-xs font-normal text-muted-foreground">{suffix}</span>}
      </div>
    </div>
  )
}

function JobStatusBadge({ status }: { status: ShopeeEmailRepairJob['status'] }) {
  if (status === 'succeeded') {
    return <Badge className="bg-emerald-600 text-white">สำเร็จ</Badge>
  }
  if (status === 'failed') {
    return <Badge variant="destructive">ไม่สำเร็จ</Badge>
  }
  return (
    <Badge variant="secondary" className="gap-1">
      <Loader2 className="h-3 w-3 animate-spin" />
      กำลังซ่อม
    </Badge>
  )
}

function repairJobProgress(job: ShopeeEmailRepairJob | null): Required<Pick<EmailRepairJobProgress, 'percent' | 'label'>> & EmailRepairJobProgress {
  const p = job?.progress ?? {}
  const rawPercent =
    typeof p.percent === 'number'
      ? p.percent
      : job?.status === 'succeeded' || job?.status === 'failed'
        ? 100
        : job?.status === 'running'
          ? 5
          : 0
  const percent = Math.max(0, Math.min(100, Math.round(rawPercent)))
  const fallbackLabel =
    job?.status === 'succeeded'
      ? 'ซ่อมคำสั่งซื้อจากอีเมลเสร็จแล้ว'
      : job?.status === 'failed'
        ? 'งานซ่อมไม่สำเร็จ'
        : job?.status === 'running'
          ? 'กำลังซ่อมคำสั่งซื้อจากอีเมลยืนยัน'
          : 'รอเริ่มงานซ่อม'
  return {
    ...p,
    percent,
    label: p.label || fallbackLabel,
  }
}

function RepairEmailHelp() {
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <span
            tabIndex={0}
            className="inline-flex h-7 w-7 items-center justify-center rounded-full text-muted-foreground hover:bg-muted hover:text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
            aria-label="คำอธิบายฟังก์ชันซ่อมจากอีเมลยืนยัน"
          >
            <Info className="h-3.5 w-3.5" />
          </span>
        </TooltipTrigger>
        <TooltipContent side="left" className="max-w-xs text-xs leading-relaxed">
          ใช้เมื่อ Shopee/Lazada มีอีเมลยืนยันที่ถูกต้อง แต่ BillFlow สร้างบิลไม่ครบ หรือบิลเดิมถูกสร้างจากอีเมลผิดฉบับ ระบบจะตรวจจากหัวข้ออีเมลก่อน และซ่อมเฉพาะบิลที่ยังไม่ส่ง SML
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

export function ArtifactList({
  billId,
  billStatus,
  billSource,
  smlDocNo,
  orderID,
  printPaymentMethod,
  effectivePrintPaymentMethod,
  emailGroup,
  smlPayload,
  onReload,
  canRepairMarketplaceEmail = false,
  autoOpenRepair = false,
}: Props) {
  const { items, loading } = useArtifacts(billId)
  const [previewArt, setPreviewArt] = useState<{ id: string; filename: string; contentType: string; displayName: string } | null>(null)
  const [printEvents, setPrintEvents] = useState<EmailPrintEvent[]>(emailGroup?.print_events ?? [])
  const [repairOpen, setRepairOpen] = useState(false)

  useEffect(() => {
    if (canRepairMarketplaceEmail && autoOpenRepair) {
      setRepairOpen(true)
    }
  }, [autoOpenRepair, canRepairMarketplaceEmail])

  useEffect(() => {
    setPrintEvents(emailGroup?.print_events ?? [])
  }, [emailGroup?.message_id, emailGroup?.print_events])

  const printReadiness = buildPrintReadiness({
    billStatus,
    billSource,
    smlDocNo,
    orderID,
    printPaymentMethod,
    effectivePrintPaymentMethod,
    emailGroup,
    smlPayload,
  })

  const handlePrintArtifact = async (artId: string, filename: string) => {
    if (!printReadiness.canPrint) {
      toast.warning(printReadiness.reason)
      return
    }

    try {
      const event = await recordArtifactPrint(billId, artId)
      setPrintEvents((prev) => [event, ...prev.filter((p) => p.id !== event.id)])
    } catch (err) {
      console.error('record artifact print failed', err)
      const parsed = parsePrintAPIError(err)
      toast.error(parsed.message, parsed.description ? { description: parsed.description } : undefined)
      if (parsed.shouldReload) {
        void onReload?.()
      }
      return
    }

    try {
      await printArtifact(billId, artId, filename, printReadiness.printContext)
      toast.success('บันทึกประวัติการพิมพ์แล้ว')
    } catch (err) {
      console.error('artifact print failed', err)
      toast.warning('บันทึกประวัติการพิมพ์แล้ว แต่เปิดหน้าพิมพ์ไม่สำเร็จ')
    }
  }

  if (loading) return null

  const visibleItems = normalizeVisibleArtifacts(items.filter((a) => isUserVisibleArtifact(a.kind)))

  if (visibleItems.length === 0) {
    return (
      <>
        <Card className="rounded-2xl border-border/70 shadow-sm">
          <CardHeader className="pb-3">
            <div className="flex flex-wrap items-start justify-between gap-2">
              <CardTitle className="flex items-center gap-2 text-sm font-semibold">
                <Paperclip className="h-4 w-4 text-muted-foreground" />
                หลักฐานต้นฉบับ (0)
              </CardTitle>
              {canRepairMarketplaceEmail && (
                <div className="flex items-center gap-1">
                  <RepairEmailHelp />
                  <Button type="button" variant="outline" size="sm" className="h-8 gap-1.5" onClick={() => setRepairOpen(true)}>
                    <Wrench className="h-3.5 w-3.5" />
                    ซ่อมจากอีเมลยืนยัน
                  </Button>
                </div>
              )}
            </div>
          </CardHeader>
          <CardContent className="pt-0">
            <p className="text-xs text-muted-foreground">
              ไม่มีไฟล์หลักฐานสำหรับแสดง
            </p>
          </CardContent>
        </Card>
        {canRepairMarketplaceEmail && (
          <ShopeeEmailRepairDialog
            billId={billId}
            open={repairOpen}
            onOpenChange={setRepairOpen}
            onReload={onReload}
          />
        )}
      </>
    )
  }

  const duplicateNote = emailDuplicateNote(emailGroup)

  return (
    <>
      <Card className="rounded-2xl border-border/70 shadow-sm">
        <CardHeader className="pb-3">
          <div className="flex flex-wrap items-start justify-between gap-2">
            <div>
              <CardTitle className="flex items-center gap-2 text-sm font-semibold">
                <Paperclip className="h-4 w-4 text-muted-foreground" />
                หลักฐานต้นฉบับ ({visibleItems.length})
              </CardTitle>
              <p className="mt-1 text-xs text-muted-foreground">
                เปิดดูเฉพาะเมื่อต้องย้อนตรวจหลักฐานต้นฉบับ
              </p>
            </div>
            {canRepairMarketplaceEmail && (
              <div className="flex items-center gap-1">
                <RepairEmailHelp />
                <Button type="button" variant="outline" size="sm" className="h-8 gap-1.5" onClick={() => setRepairOpen(true)}>
                  <Wrench className="h-3.5 w-3.5" />
                  ซ่อมจากอีเมลยืนยัน
                </Button>
              </div>
            )}
          </div>
        </CardHeader>
        <CardContent className="space-y-3 pt-0">
          <EmailGroupContext
            billId={billId}
            emailGroup={emailGroup}
            printEvents={printEvents}
          />

          <div className="space-y-1">
            {visibleItems.map((a) => {
              const meta = KIND_META[a.kind] ?? { icon: '', label: a.kind, desc: '' }
              const display = artifactDisplay(a, meta)
              const ct = a.content_type ?? ''
              const isHtml = ct.startsWith('text/html') || a.kind === 'email_html' || a.kind === 'email_text'
              const isPrintableEmail = a.kind === 'email_html' || a.kind === 'email_text'
              const previewable =
                ct === 'application/pdf' ||
                ct.startsWith('image/') ||
                ct.startsWith('text/') ||
                ct === 'application/json'

              const handlePreview = () => {
                if (isHtml) {
                  setPreviewArt({ id: a.id, filename: a.filename, contentType: ct, displayName: display.label })
                } else {
                  openArtifact(billId, a.id, a.filename, 'preview')
                }
              }

              return (
                <div
                  key={a.id}
                  className="flex items-start gap-3 border-b border-border/50 py-3 last:border-0"
                >
                  <span className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
                    <Paperclip className="h-4 w-4" />
                  </span>
                  <div className="flex-1 min-w-0">
                    <div className="font-medium text-sm break-words">{display.label}</div>
                    {display.desc && <div className="mt-0.5 line-clamp-2 text-xs leading-snug text-muted-foreground">{display.desc}</div>}
                    {isPrintableEmail && duplicateNote && (
                      <div className="mt-1 rounded-md bg-warning/10 px-2 py-1 text-xs leading-snug text-warning">
                        {duplicateNote}
                      </div>
                    )}
                    <div className="mt-1 font-mono text-[11px] text-muted-foreground/70">
                      {a.filename} · {fmtSize(a.size_bytes)} ·{' '}
                      {dayjs(a.created_at).format('DD/MM/YY HH:mm')}
                    </div>
                  </div>
                  <div className="flex shrink-0 gap-2">
                    {previewable && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        className="h-8 gap-1.5"
                        title={a.sha256 ? `SHA256: ${a.sha256.slice(0, 16)}…` : ''}
                        onClick={handlePreview}
                      >
                        <Eye className="h-3.5 w-3.5" />
                        ดู
                      </Button>
                    )}
                    {isPrintableEmail && (
                      printReadiness.canPrint ? (
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          className="h-8 gap-1.5"
                          title={a.sha256 ? `SHA256: ${a.sha256.slice(0, 16)}…` : ''}
                          onClick={() => handlePrintArtifact(a.id, a.filename)}
                        >
                          <Printer className="h-3.5 w-3.5" />
                          พิมพ์
                        </Button>
                      ) : (
                        <TooltipProvider delayDuration={100}>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <span className="cursor-not-allowed">
                                <Button
                                  type="button"
                                  variant="outline"
                                  size="sm"
                                  className="h-8 gap-1.5 pointer-events-none opacity-40"
                                  disabled
                                >
                                  <Printer className="h-3.5 w-3.5" />
                                  พิมพ์
                                </Button>
                              </span>
                            </TooltipTrigger>
                            <TooltipContent side="top" className="text-xs">
                              {printReadiness.reason}
                            </TooltipContent>
                          </Tooltip>
                        </TooltipProvider>
                      )
                    )}
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="h-8 gap-1.5"
                      title={a.sha256 ? `SHA256: ${a.sha256.slice(0, 16)}…` : ''}
                      onClick={() => openArtifact(billId, a.id, a.filename, 'download')}
                    >
                      <Download className="h-3.5 w-3.5" />
                      ดาวน์โหลด
                    </Button>
                  </div>
                </div>
              )
            })}
          </div>
        </CardContent>
      </Card>

      {previewArt && (
        <EmailPreviewModal
          billId={billId}
          artId={previewArt.id}
          filename={previewArt.filename}
          displayName={previewArt.displayName}
          emailGroup={emailGroup}
          printReadiness={printReadiness}
          onPrinted={handlePrintArtifact}
          onClose={() => setPreviewArt(null)}
        />
      )}
      {canRepairMarketplaceEmail && (
        <ShopeeEmailRepairDialog
          billId={billId}
          open={repairOpen}
          onOpenChange={setRepairOpen}
          onReload={onReload}
        />
      )}
    </>
  )
}

function artifactDisplay(
  artifact: BillArtifact,
  meta: { icon: string; label: string; desc: string },
): { label: string; desc: string } {
  if (artifact.kind !== 'email_html' && artifact.kind !== 'email_text') {
    return { label: meta.label, desc: meta.desc }
  }

  const subject = metaString(artifact.source_meta?.subject)
  const eventType = metaString(artifact.source_meta?.event_type)

  return {
    label: emailEvidenceLabel(subject, eventType),
    desc: subject || meta.desc,
  }
}

function normalizeVisibleArtifacts(items: BillArtifact[]): BillArtifact[] {
  const byKey = new Map<string, BillArtifact>()
  for (const item of items) {
    const key = artifactDedupeKey(item)
    const existing = byKey.get(key)
    if (!existing || preferArtifact(item, existing)) {
      byKey.set(key, item)
    }
  }
  return Array.from(byKey.values()).sort(compareArtifacts)
}

function artifactDedupeKey(artifact: BillArtifact): string {
  if (artifact.kind !== 'email_html' && artifact.kind !== 'email_text') {
    return artifact.id
  }
  const messageID = metaString(artifact.source_meta?.message_id)
  if (!messageID) {
    return artifact.id
  }
  return `email:${messageID}:${emailEvidenceRank(artifact)}`
}

function preferArtifact(next: BillArtifact, current: BillArtifact): boolean {
  if (next.kind === 'email_html' && current.kind !== 'email_html') return true
  if (next.kind !== 'email_html' && current.kind === 'email_html') return false
  return Date.parse(next.created_at) < Date.parse(current.created_at)
}

function compareArtifacts(a: BillArtifact, b: BillArtifact): number {
  const rankDelta = emailEvidenceRank(a) - emailEvidenceRank(b)
  if (rankDelta !== 0) return rankDelta
  const timeDelta = Date.parse(a.created_at) - Date.parse(b.created_at)
  if (timeDelta !== 0) return timeDelta
  return a.id.localeCompare(b.id)
}

function emailEvidenceRank(artifact: BillArtifact): number {
  if (artifact.kind !== 'email_html' && artifact.kind !== 'email_text') {
    return 20
  }
  const subject = metaString(artifact.source_meta?.subject)
  const eventType = metaString(artifact.source_meta?.event_type)
  if (
    eventType === 'payment_confirmed' ||
    subject.includes('ยืนยันการชำระเงิน') ||
    subject.includes('ยืนยันคำสั่งซื้อ')
  ) {
    return 0
  }
  if (eventType === 'shipped' || subject.includes('ถูกจัดส่งแล้ว')) {
    return 10
  }
  return 15
}

function emailEvidenceLabel(subject: string, eventType: string): string {
  if (eventType === 'payment_confirmed' || subject.includes('ยืนยันการชำระเงิน')) {
    return 'อีเมลยืนยันการชำระเงิน'
  }
  if (subject.includes('ยืนยันคำสั่งซื้อ')) {
    return 'อีเมลยืนยันคำสั่งซื้อ'
  }
  if (eventType === 'shipped' || subject.includes('ถูกจัดส่งแล้ว')) {
    return 'อีเมลแจ้งจัดส่ง'
  }
  return 'อีเมลต้นฉบับ'
}

function metaString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function buildPrintReadiness({
  billStatus,
  billSource,
  smlDocNo,
  orderID,
  printPaymentMethod,
  effectivePrintPaymentMethod,
  emailGroup,
  smlPayload,
}: {
  billStatus?: string
  billSource?: string
  smlDocNo?: string
  orderID?: string
  printPaymentMethod?: string
  effectivePrintPaymentMethod?: string
  emailGroup?: BillEmailGroup | null
  smlPayload?: Record<string, unknown> | null
}): PrintReadiness {
  const currentParty = {
    partyCode: metaString(smlPayload?.cust_code),
    partyName: metaString(smlPayload?.supplier_name) || metaString(smlPayload?.party_name),
  }
  const marketplace = isMarketplaceEmailSource(billSource)
  const related = (emailGroup?.related_bills ?? []).filter((b) =>
    isMarketplaceEmailSource(b.source) && b.bill_type === 'purchase'
  )
  const printContext = buildArtifactPrintContext({
    related,
    orderID,
    smlDocNo,
    currentParty,
    currentPaymentMethod: effectivePrintPaymentMethod || printPaymentMethod || '',
  })

  if (billStatus !== 'sent') {
    return {
      canPrint: false,
      reason: 'ส่งเอกสารเข้า SML ก่อนถึงจะพิมพ์ได้',
      printContext,
    }
  }
  if (!smlDocNo) {
    return {
      canPrint: false,
      reason: 'ยังไม่มีเลขเอกสาร SML',
      printContext,
    }
  }
  if (marketplace) {
    if (emailGroup?.print_policy_note && !emailGroup.print_ready && emailGroup.print_block_reason) {
      return {
        canPrint: false,
        reason: emailGroup.print_block_reason,
        printContext,
      }
    }
    const missing = related
      .filter((b) => !metaString(b.sml_doc_no))
      .map((b) => b.order_id || b.id.slice(0, 8))
    if (missing.length > 0) {
      return {
        canPrint: false,
        reason: formatMissingSMLDocReason(missing),
        printContext,
      }
    }
  }
  return {
    canPrint: true,
    reason: 'พร้อมพิมพ์อีเมลต้นฉบับ',
    printContext,
  }
}

function buildArtifactPrintContext({
  related,
  orderID,
  smlDocNo,
  currentParty,
  currentPaymentMethod,
}: {
  related: BillEmailRelatedBill[]
  orderID?: string
  smlDocNo?: string
  currentParty: { partyCode?: string; partyName?: string }
  currentPaymentMethod?: string
}): ArtifactPrintContext {
  const sourceRows = related.length > 0
    ? related
    : [
        {
          id: '',
          order_id: orderID,
          source: '',
          bill_type: 'purchase',
          status: 'sent' as const,
          sml_doc_no: smlDocNo,
          created_at: '',
          is_current: true,
          party_code: currentParty.partyCode,
          party_name: currentParty.partyName,
          effective_print_payment_method: currentPaymentMethod,
        },
      ]

  return {
    orders: sourceRows.map((b) => ({
      orderId: b.order_id || undefined,
      smlDocNo: b.sml_doc_no || undefined,
      partyCode: b.party_code || (b.is_current ? currentParty.partyCode : undefined),
      partyName: b.party_name || (b.is_current ? currentParty.partyName : undefined),
      paymentMethod: b.effective_print_payment_method || b.print_payment_method || (b.is_current ? currentPaymentMethod : undefined),
    })),
  }
}

function isMarketplaceEmailSource(source?: string): boolean {
  return source === 'shopee_shipped' || source === 'lazada_email'
}

function formatMissingSMLDocReason(missingOrders: string[]): string {
  const visible = missingOrders.slice(0, 5)
  const suffix = missingOrders.length > visible.length ? ' ...' : ''
  return `ยังขาดเลข SML ${missingOrders.length.toLocaleString('th-TH')} คำสั่งซื้อ: ${visible.join(', ')}${suffix}`
}

function parsePrintAPIError(err: unknown): PrintAPIError {
  if (axios.isAxiosError(err)) {
    const data = err.response?.data as {
      error?: string
      missing_orders?: string[]
      missing_count?: number
      missing_payment_method_orders?: string[]
      non_matching_payment_method_orders?: string[]
    } | undefined
    const message = data?.error || 'บันทึกประวัติการพิมพ์ไม่สำเร็จ'
    if (err.response?.status === 400 && Array.isArray(data?.missing_orders)) {
      return {
        message,
        description: formatMissingSMLDocReason(data.missing_orders),
        shouldReload: true,
      }
    }
    if (err.response?.status === 400 && Array.isArray(data?.missing_payment_method_orders)) {
      return {
        message,
        description: formatPaymentMethodMissingReason(data.missing_payment_method_orders),
        shouldReload: true,
      }
    }
    if (err.response?.status === 400 && Array.isArray(data?.non_matching_payment_method_orders)) {
      return {
        message,
        description: formatPaymentMethodPolicyReason(data.non_matching_payment_method_orders),
        shouldReload: true,
      }
    }
    return { message }
  }
  return { message: 'บันทึกประวัติการพิมพ์ไม่สำเร็จ' }
}

function apiErrorMessage(err: unknown, fallback: string): string {
  if (axios.isAxiosError(err)) {
    const data = err.response?.data as { error?: string; message?: string } | undefined
    return data?.error || data?.message || fallback
  }
  return err instanceof Error && err.message ? err.message : fallback
}

function formatPaymentMethodMissingReason(orderIDs: string[]): string {
  const visible = orderIDs.slice(0, 5)
  const suffix = orderIDs.length > visible.length ? ' ...' : ''
  return `ยังไม่ได้เลือกวิธีการชำระเงิน ${orderIDs.length.toLocaleString('th-TH')} คำสั่งซื้อ: ${visible.join(', ')}${suffix}`
}

function formatPaymentMethodPolicyReason(orderIDs: string[]): string {
  const visible = orderIDs.slice(0, 5)
  const suffix = orderIDs.length > visible.length ? ' ...' : ''
  return `วิธีการชำระเงินไม่ตรงเงื่อนไข ${orderIDs.length.toLocaleString('th-TH')} คำสั่งซื้อ: ${visible.join(', ')}${suffix}`
}

function EmailGroupContext({
  billId,
  emailGroup,
  printEvents,
}: {
  billId: string
  emailGroup?: BillEmailGroup | null
  printEvents: EmailPrintEvent[]
}) {
  if (!emailGroup?.message_id) return null

  const related = emailGroup.related_bills ?? []
  const showRelated = related.length > 1
  const showHistory = emailGroup.has_printable_email
  const hasIngestionState = Boolean(emailGroup.ingestion_status && (emailGroup.expected_order_count ?? 0) > 0)
  const isIncomplete = hasIngestionState && emailGroup.ingestion_status !== 'complete'
  const expectedCount = emailGroup.expected_order_count ?? emailGroup.order_count ?? 0
  const resolvedCount = emailGroup.resolved_order_count ?? emailGroup.order_count ?? 0
  if (!showRelated && !showHistory && !hasIngestionState) return null

  return (
    <div className="space-y-3 border-b border-border/50 pb-3">
      {hasIngestionState && (
        <div className={`rounded-md border px-3 py-2 text-xs ${
          isIncomplete
            ? 'border-warning/40 bg-warning/10 text-warning'
            : 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-200'
        }`}>
          <div className="flex items-center gap-1.5 font-medium">
            {isIncomplete ? <AlertTriangle className="h-3.5 w-3.5" /> : <CheckCircle2 className="h-3.5 w-3.5" />}
            {emailGroup.ingestion_status === 'processing'
              ? 'กำลังอ่านคำสั่งซื้อจากอีเมลต้นฉบับ'
              : isIncomplete
                ? `อีเมลนี้อ่านได้ไม่ครบ ${resolvedCount.toLocaleString('th-TH')}/${expectedCount.toLocaleString('th-TH')} คำสั่งซื้อ`
                : `อีเมลนี้ครบ ${resolvedCount.toLocaleString('th-TH')}/${expectedCount.toLocaleString('th-TH')} คำสั่งซื้อแล้ว`}
          </div>
          {isIncomplete && (
            <div className="mt-1 text-warning/90">
              ระบบจะไม่อนุญาตให้พิมพ์อีเมลกลุ่มนี้ และเมื่อเปิดการป้องกันในระบบ จะไม่อนุญาตให้ส่งเข้า SML จนกว่าจะกู้ข้อมูลครบ
            </div>
          )}
          {emailGroup.ingestion_orders && emailGroup.ingestion_orders.length > 0 && (
            <EmailGroupOrderList orders={emailGroup.ingestion_orders} currentBillID={billId} />
          )}
        </div>
      )}
      {emailGroup.print_policy_note && (
        <div className="rounded-md border border-info/30 bg-info/5 px-3 py-2 text-xs text-info">
          {emailGroup.print_policy_note}
          {!emailGroup.print_ready && emailGroup.print_block_reason && (
            <span className="ml-1 font-medium text-warning">
              {emailGroup.print_block_reason}
            </span>
          )}
        </div>
      )}
      {showRelated && (
        <div>
          <div className="mb-1.5 flex items-center justify-between gap-2">
            <div className="text-xs font-medium text-foreground">
              บิลอื่นจาก Email #{emailGroup.group_key}
            </div>
            <span className="text-[11px] text-muted-foreground">
              {related.length.toLocaleString('th-TH')} คำสั่งซื้อ
            </span>
          </div>
          <div className="max-h-32 space-y-1 overflow-y-auto pr-1">
            {related.map((b) => (
              <Link
                key={b.id}
                to={billPath(b)}
                className={`flex items-center justify-between gap-2 rounded-md px-2 py-1.5 text-xs hover:bg-muted ${
                  b.id === billId ? 'bg-info/10 text-info' : 'text-muted-foreground'
                }`}
              >
                <span className="min-w-0 truncate">
                  <span className="font-mono text-foreground">{b.order_id || b.id.slice(0, 8)}</span>
                  {b.sml_doc_no && (
                    <span className="ml-1 font-mono text-[11px] text-muted-foreground">→ {b.sml_doc_no}</span>
                  )}
                  {formatRelatedParty(b) && <span className="text-muted-foreground"> · {formatRelatedParty(b)}</span>}
                  {b.id === billId && <span className="ml-1 font-medium">(บิลนี้)</span>}
                </span>
                <span className="flex shrink-0 items-center gap-2">
                  <span className="tabular-nums">{formatMoney(b.total_amount ?? 0)}</span>
                  <ExternalLink className="h-3 w-3" />
                </span>
              </Link>
            ))}
          </div>
        </div>
      )}

      {showHistory && (
        <div>
          <div className="mb-1.5 flex items-center gap-1.5 text-xs font-medium text-foreground">
            <History className="h-3.5 w-3.5 text-muted-foreground" />
            ประวัติการพิมพ์เมลต้นฉบับ
          </div>
          {printEvents.length > 0 ? (
            <div className="space-y-1 text-xs text-muted-foreground">
              {printEvents.slice(0, 5).map((event) => (
                <div key={event.id} className="flex items-center justify-between gap-2">
                  <span className="min-w-0 truncate">
                    {event.requested_by_name || event.requested_by_email || 'ผู้ใช้ระบบ'}
                  </span>
                  <span className="shrink-0 tabular-nums">
                    {dayjs(event.created_at).format('DD/MM/YY HH:mm')}
                  </span>
                </div>
              ))}
            </div>
          ) : (
            <div className="text-xs text-muted-foreground">
              ยังไม่มีประวัติการพิมพ์สำหรับเมลฉบับนี้
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function EmailGroupOrderList({ orders, currentBillID }: { orders: MarketplaceEmailGroupOrder[]; currentBillID: string }) {
  return (
    <div className="mt-2 max-h-28 space-y-1 overflow-y-auto border-t border-current/20 pt-2 text-[11px]">
      {orders.map((order) => {
        const complete = order.status === 'created' || order.status === 'existing'
        const label = complete ? 'พร้อมตรวจ' : order.status === 'failed' ? 'อ่านไม่สำเร็จ' : order.status === 'archived' ? 'เก็บบิลแล้ว' : 'ยังไม่พบบิล'
        const content = (
          <>
            <span className="font-mono">{order.order_id}</span>
            <span className="ml-1">{label}</span>
          </>
        )
        if (order.bill_id) {
          return (
            <Link
              key={order.order_id}
              to={`/bills/${order.bill_id}`}
              className={`flex items-center justify-between rounded px-1 py-0.5 hover:bg-background/50 ${order.bill_id === currentBillID ? 'font-medium' : ''}`}
            >
              {content}
              <ExternalLink className="h-3 w-3" />
            </Link>
          )
        }
        return <div key={order.order_id} className="rounded px-1 py-0.5">{content}</div>
      })}
    </div>
  )
}

function emailDuplicateNote(emailGroup?: BillEmailGroup | null): string {
  const count = emailGroup?.order_count ?? 0
  if (count <= 1) return ''
  return `เมลฉบับนี้สร้าง ${count.toLocaleString('th-TH')} คำสั่งซื้อ การพิมพ์จากหลายบิลจะได้เอกสารซ้ำกัน`
}

function billPath(b: BillEmailRelatedBill): string {
  if (b.bill_type !== 'sale') return `/bills/${b.id}`
  if (b.document_route === 'saleinvoice') return `/sale-invoices/${b.id}`
  return `/sales-orders/${b.id}`
}

function formatRelatedParty(b: BillEmailRelatedBill): string {
  if (b.party_code && b.party_name) return `${b.party_code} ~ ${b.party_name}`
  return b.party_code || b.party_name || ''
}

function formatMoney(value: number): string {
  return new Intl.NumberFormat('th-TH', {
    style: 'currency',
    currency: 'THB',
    maximumFractionDigits: 0,
  }).format(value)
}
