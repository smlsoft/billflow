import { useEffect, useState } from 'react'
import { AlertTriangle, CheckCircle2, Cloud, FileText, RefreshCw, RotateCw, Save, Upload } from 'lucide-react'
import { toast } from 'sonner'

import client from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { PageHeader } from '@/components/common/PageHeader'
import { useAuth } from '@/hooks/useAuth'
import { cn } from '@/lib/utils'

type ExportStatus = {
  enabled: boolean
  root_folder: string
  start_date: string
  remote: string
  output_format: 'pdf' | 'html'
  runtime_ready: boolean
  runtime_error?: string
  max_attempts: number
}

type ExportJob = {
  id: string
  bill_id: string
  source_channel: string
  order_date: string
  payment_token: string
  sml_doc_no: string
  marketplace_order_id: string
  charge_amount: string
  output_format: 'pdf' | 'html'
  remote_path: string
  status: 'queued' | 'running' | 'succeeded' | 'failed' | 'skipped' | 'conflict'
  attempt_count: number
  next_attempt_at: string
  uploaded_at?: string | null
  last_error?: string
  render_warning?: string
  updated_at: string
}

type JobCounts = { queued: number; running: number; succeeded: number; failed: number; conflict: number }
type JobsResponse = { data: ExportJob[]; counts: JobCounts }
type BackfillPreview = { date_from: string; date_to: string; candidate_count: number; limited: boolean; limit: number }

const STATUS_META: Record<ExportJob['status'], { label: string; cls: string }> = {
  queued: { label: 'รออัปโหลด', cls: 'bg-warning/10 text-warning border-warning/30' },
  running: { label: 'กำลังอัปโหลด', cls: 'bg-primary/10 text-primary border-primary/30' },
  succeeded: { label: 'สำเร็จ', cls: 'bg-success/10 text-success border-success/30' },
  failed: { label: 'ไม่สำเร็จ', cls: 'bg-destructive/10 text-destructive border-destructive/30' },
  skipped: { label: 'ข้าม', cls: 'bg-secondary text-secondary-foreground border-border' },
  conflict: { label: 'ต้องตรวจ', cls: 'bg-destructive/10 text-destructive border-destructive/30' },
}

function today() {
  return new Date().toISOString().slice(0, 10)
}

function dateTime(value?: string | null) {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString('th-TH', { dateStyle: 'short', timeStyle: 'short' })
}

export default function GoogleDriveSettings() {
  const { user } = useAuth()
  const [status, setStatus] = useState<ExportStatus | null>(null)
  const [rootFolder, setRootFolder] = useState('')
  const [startDate, setStartDate] = useState('')
  const [enabled, setEnabled] = useState(false)
  const [jobs, setJobs] = useState<ExportJob[]>([])
  const [counts, setCounts] = useState<JobCounts | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState(false)
  const [retrying, setRetrying] = useState<string | null>(null)
  const [converting, setConverting] = useState<string | null>(null)
  const [dateFrom, setDateFrom] = useState(today())
  const [dateTo, setDateTo] = useState(today())
  const [preview, setPreview] = useState<BackfillPreview | null>(null)
  const [previewing, setPreviewing] = useState(false)
  const [queueing, setQueueing] = useState(false)
  const [confirmBackfill, setConfirmBackfill] = useState(false)

  const load = async (showError = false) => {
    try {
      const [settingsRes, jobsRes] = await Promise.all([
        client.get<ExportStatus>('/api/settings/google-drive'),
        client.get<JobsResponse>('/api/settings/google-drive/jobs?limit=50'),
      ])
      setStatus(settingsRes.data)
      setRootFolder(settingsRes.data.root_folder ?? '')
      setStartDate(settingsRes.data.start_date ?? '')
      setEnabled(settingsRes.data.enabled)
      setJobs(jobsRes.data.data ?? [])
      setCounts(jobsRes.data.counts ?? null)
    } catch {
      if (showError) toast.error('โหลดการตั้งค่า Google Drive ไม่สำเร็จ')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (user?.role === 'admin') {
      void load(true)
      return
    }
    setLoading(false)
  }, [user?.role])

  const save = async () => {
    setSaving(true)
    try {
      const res = await client.put<ExportStatus>('/api/settings/google-drive', { enabled, root_folder: rootFolder, start_date: startDate }, { timeout: 45000 })
      setStatus(res.data)
      setRootFolder(res.data.root_folder)
      setStartDate(res.data.start_date ?? '')
      setEnabled(res.data.enabled)
      toast.success(res.data.enabled ? 'เปิดอัปโหลดอีเมลไป Google Drive แล้ว' : 'ปิดอัปโหลดอีเมลไป Google Drive แล้ว')
    } catch (error: any) {
      toast.error(error?.response?.data?.error ?? 'บันทึกการตั้งค่าไม่สำเร็จ')
    } finally {
      setSaving(false)
    }
  }

  const testConnection = async () => {
    setTesting(true)
    try {
      const res = await client.post<{ ok: boolean; detail?: string }>('/api/settings/google-drive/test', { root_folder: rootFolder }, { timeout: 45000 })
      toast.success(res.data.detail ?? 'เชื่อมต่อ Google Drive สำเร็จ')
    } catch (error: any) {
      toast.error(error?.response?.data?.error ?? 'ทดสอบ Google Drive ไม่สำเร็จ')
    } finally {
      setTesting(false)
    }
  }

  const previewBackfill = async () => {
    setPreviewing(true)
    try {
      const res = await client.post<BackfillPreview>('/api/settings/google-drive/backfill/preview', { date_from: dateFrom, date_to: dateTo })
      setPreview(res.data)
      if (res.data.limited) toast.warning(`พบมากกว่า ${res.data.limit} บิล กรุณาแบ่งช่วงวันที่ให้สั้นลง`)
    } catch (error: any) {
      setPreview(null)
      toast.error(error?.response?.data?.error ?? 'ตรวจรายการย้อนหลังไม่สำเร็จ')
    } finally {
      setPreviewing(false)
    }
  }

  const enqueueBackfill = async () => {
    setQueueing(true)
    try {
      const res = await client.post<{ queued: number; already_queued: number; skipped: number }>('/api/settings/google-drive/backfill', { date_from: dateFrom, date_to: dateTo })
      toast.success(`เพิ่มงานอัปโหลด ${res.data.queued} บิลแล้ว${res.data.already_queued ? ` · มีในคิวอยู่แล้ว ${res.data.already_queued}` : ''}`)
      setPreview(null)
      await load()
    } catch (error: any) {
      toast.error(error?.response?.data?.error ?? 'เพิ่มงานย้อนหลังไม่สำเร็จ')
    } finally {
      setQueueing(false)
    }
  }

  const retry = async (jobID: string) => {
    setRetrying(jobID)
    try {
      await client.post(`/api/settings/google-drive/jobs/${jobID}/retry`)
      toast.success('เพิ่มงานลองอัปโหลดใหม่แล้ว')
      await load()
    } catch (error: any) {
      toast.error(error?.response?.data?.error ?? 'สั่งลองใหม่ไม่สำเร็จ')
    } finally {
      setRetrying(null)
    }
  }

  const convertToPDF = async (jobID: string) => {
    setConverting(jobID)
    try {
      await client.post(`/api/settings/google-drive/jobs/${jobID}/pdf`)
      toast.success('เพิ่มงานสร้าง PDF แล้ว โดยจะเก็บไฟล์ HTML เดิมไว้')
      await load()
    } catch (error: any) {
      toast.error(error?.response?.data?.error ?? 'สั่งสร้าง PDF ไม่สำเร็จ')
    } finally {
      setConverting(null)
    }
  }

  const settingsChanged = status == null || status.enabled !== enabled || status.root_folder !== rootFolder || status.start_date !== startDate
  const canQueueBackfill = Boolean(status?.enabled && status.runtime_ready && preview && !preview.limited && preview.candidate_count > 0)

  if (user?.role !== 'admin') {
    return <div className="p-6"><PageHeader title="Google Drive อีเมล" description="เฉพาะผู้ดูแลระบบเท่านั้น" /></div>
  }

  return (
    <div className="space-y-5">
      <PageHeader
        title="Google Drive อีเมล"
        description="เก็บ PDF ของอีเมล Shopee และ Lazada หลังส่ง SML สำเร็จ"
        actions={
          <Button variant="outline" size="icon" onClick={() => void load(true)} disabled={loading || saving} title="รีเฟรชสถานะ">
            <RefreshCw className={cn('h-4 w-4', loading && 'animate-spin')} />
          </Button>
        }
      />

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-sm"><Cloud className="h-4 w-4 text-primary" /> การเชื่อมต่อและโฟลเดอร์หลัก</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className={cn('flex gap-3 rounded-md border p-3 text-sm', status?.runtime_ready ? 'border-success/30 bg-success/[0.05]' : 'border-warning/35 bg-warning/[0.07]')}>
            {status?.runtime_ready ? <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-success" /> : <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" />}
            <div>
              <p className="font-medium">{status?.runtime_ready ? `พร้อมสร้าง ${status.output_format?.toUpperCase() ?? 'PDF'} ด้วย remote ${status.remote}` : 'ยังไม่พร้อมใช้งานบน server'}</p>
              <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">
                {status?.runtime_ready ? 'ไฟล์ PDF ใช้ข้อมูลและรูปแบบเดียวกับหน้าดูอีเมลใน BillFlow โดยไม่เก็บ Google token ในระบบ' : status?.runtime_error ?? 'กำลังตรวจการตั้งค่า server'}
              </p>
            </div>
          </div>

          <div className="flex items-center justify-between gap-4 rounded-md border p-3">
            <div>
              <Label htmlFor="google-drive-enabled" className="text-sm font-medium">อัปโหลดอัตโนมัติหลังส่ง SML</Label>
              <p className="mt-0.5 text-xs text-muted-foreground">เฉพาะใบสั่งซื้อจาก Shopee และ Lazada ที่ส่ง SML สำเร็จแล้ว ระบบจะสร้าง PDF ให้เปิดดูได้ทันทีใน Drive</p>
            </div>
            <Switch id="google-drive-enabled" checked={enabled} onCheckedChange={setEnabled} disabled={loading || saving} />
          </div>

          <div className="max-w-2xl space-y-2">
            <Label htmlFor="google-drive-root">โฟลเดอร์หลักบน Google Drive</Label>
            <Input id="google-drive-root" value={rootFolder} onChange={(event) => setRootFolder(event.target.value)} placeholder="BillFlow Email/Thaisunsport" disabled={loading || saving} />
            <p className="text-xs leading-relaxed text-muted-foreground">ระบบจะแยกต่อเป็น ปี/เดือน/วัน/Shopee หรือ Lazada/วิธีชำระเงิน และตั้งชื่อไฟล์จากข้อมูล SML กับคำสั่งซื้อ</p>
          </div>

          <div className="max-w-xs space-y-2">
            <Label htmlFor="google-drive-start-date">เริ่มเก็บตั้งแต่วันที่สั่งซื้อ</Label>
            <Input id="google-drive-start-date" type="date" value={startDate} onChange={(event) => setStartDate(event.target.value)} disabled={loading || saving} />
            <p className="text-xs leading-relaxed text-muted-foreground">บิลก่อนวันที่ตั้งไว้จะไม่ถูกเพิ่มเข้าคิว แม้เพิ่งส่ง SML ย้อนหลัง</p>
          </div>

          <div className="flex flex-wrap gap-2">
            <Button variant="outline" onClick={testConnection} disabled={testing || saving || !rootFolder.trim()}>
              <Cloud className={cn('h-4 w-4', testing && 'animate-pulse')} />{testing ? 'กำลังทดสอบ...' : 'ทดสอบ Google Drive'}
            </Button>
            <Button onClick={save} disabled={saving || testing || !settingsChanged || !rootFolder.trim()}>
              <Save className="h-4 w-4" />{saving ? 'กำลังบันทึก...' : 'บันทึกการตั้งค่า'}
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-3"><CardTitle className="text-sm">อัปโหลดอีเมลย้อนหลัง</CardTitle></CardHeader>
        <CardContent className="space-y-3">
          <div className="flex flex-wrap items-end gap-3">
            <div className="space-y-1.5"><Label htmlFor="google-drive-from">วันที่สั่งซื้อเริ่มต้น</Label><Input id="google-drive-from" type="date" value={dateFrom} onChange={(event) => setDateFrom(event.target.value)} /></div>
            <div className="space-y-1.5"><Label htmlFor="google-drive-to">วันที่สั่งซื้อสิ้นสุด</Label><Input id="google-drive-to" type="date" value={dateTo} onChange={(event) => setDateTo(event.target.value)} /></div>
            <Button variant="outline" onClick={previewBackfill} disabled={previewing || queueing || !dateFrom || !dateTo}><Upload className="h-4 w-4" />{previewing ? 'กำลังตรวจ...' : 'ตรวจรายการ'}</Button>
          </div>
          {preview && <div className="flex flex-wrap items-center gap-2 rounded-md border bg-muted/30 p-3 text-sm">
            <span>พบ {preview.candidate_count.toLocaleString()} บิลที่ส่ง SML แล้วในช่วงวันที่เลือก</span>
            {preview.limited ? <Badge variant="destructive">เกิน {preview.limit} บิล</Badge> : <Button size="sm" onClick={() => setConfirmBackfill(true)} disabled={!canQueueBackfill || queueing}>เพิ่มเข้าคิวอัปโหลด</Button>}
          </div>}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex-row items-center justify-between space-y-0 pb-3">
          <CardTitle className="text-sm">ประวัติอัปโหลด</CardTitle>
          {counts && <div className="flex flex-wrap gap-1.5 text-xs"><Badge variant="outline">รอ {counts.queued}</Badge><Badge variant="outline">กำลังทำ {counts.running}</Badge><Badge variant="outline">สำเร็จ {counts.succeeded}</Badge>{(counts.failed + counts.conflict) > 0 && <Badge variant="destructive">ต้องตรวจ {counts.failed + counts.conflict}</Badge>}</div>}
        </CardHeader>
        <CardContent className="px-0 pb-0">
          {jobs.length === 0 ? <div className="px-6 pb-6 text-sm text-muted-foreground">ยังไม่มีงานอัปโหลดอีเมล</div> : <div className="overflow-x-auto"><table className="w-full min-w-[900px] text-sm"><thead className="border-y bg-muted/35 text-left text-xs text-muted-foreground"><tr><th className="px-6 py-3 font-medium">เอกสาร</th><th className="px-3 py-3 font-medium">วันที่/ช่องทาง</th><th className="px-3 py-3 font-medium">สถานะ</th><th className="px-3 py-3 font-medium">อัปเดตล่าสุด</th><th className="px-3 py-3 font-medium">รายละเอียด</th><th className="px-6 py-3 text-right font-medium">จัดการ</th></tr></thead><tbody>{jobs.map((job) => {
            const meta = STATUS_META[job.status]
            return <tr key={job.id} className="border-b last:border-0"><td className="px-6 py-3"><div className="flex items-center gap-1.5"><p className="font-medium">{job.sml_doc_no}</p><Badge variant="outline" className="text-[10px]">{job.output_format.toUpperCase()}</Badge></div><p className="mt-0.5 font-mono text-xs text-muted-foreground">{job.marketplace_order_id}</p></td><td className="px-3 py-3"><p>{job.order_date}</p><p className="mt-0.5 text-xs text-muted-foreground">{job.source_channel} · {job.payment_token}</p></td><td className="px-3 py-3"><Badge variant="outline" className={meta.cls}>{meta.label}</Badge></td><td className="px-3 py-3 text-xs text-muted-foreground">{dateTime(job.uploaded_at ?? job.updated_at)}</td><td className="max-w-[300px] px-3 py-3 text-xs text-muted-foreground"><p className="truncate" title={job.remote_path}>{job.remote_path}</p>{job.last_error && <p className="mt-1 text-destructive">{job.last_error}</p>}{job.render_warning && <p className="mt-1 text-warning" title={job.render_warning}>ตรวจรูป: {job.render_warning}</p>}</td><td className="px-6 py-3 text-right"><div className="flex justify-end gap-2">{job.status === 'succeeded' && job.output_format === 'html' && <Button variant="outline" size="sm" onClick={() => void convertToPDF(job.id)} disabled={converting === job.id}><FileText className={cn('h-3.5 w-3.5', converting === job.id && 'animate-pulse')} />สร้าง PDF</Button>}{['failed', 'conflict', 'skipped'].includes(job.status) && <Button variant="outline" size="sm" onClick={() => void retry(job.id)} disabled={retrying === job.id}><RotateCw className={cn('h-3.5 w-3.5', retrying === job.id && 'animate-spin')} />ลองใหม่</Button>}</div></td></tr>
          })}</tbody></table></div>}
        </CardContent>
      </Card>

      <ConfirmDialog open={confirmBackfill} onOpenChange={setConfirmBackfill} title="เพิ่มงานอัปโหลดอีเมลย้อนหลัง" description={`ระบบจะเพิ่ม ${preview?.candidate_count ?? 0} บิลเข้าคิวอัปโหลด โดยไม่ส่ง SML ซ้ำและไม่เขียนทับไฟล์ชื่อเดิม`} confirmLabel="เพิ่มเข้าคิว" onConfirm={enqueueBackfill} />
    </div>
  )
}
