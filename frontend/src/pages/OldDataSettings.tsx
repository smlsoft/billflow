import { useEffect, useState } from 'react'
import { AlertTriangle, Archive, Trash2, RefreshCw, Database } from 'lucide-react'
import { toast } from 'sonner'
import api from '@/api/client'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { PageHeader } from '@/components/common/PageHeader'

interface Summary {
  archive_days: number
  purge_days: number
  bills: { to_archive: number; to_purge: number; archived: number }
  audit_logs: TableMetrics
  ai_usage_logs: TableMetrics
  chat_messages: TableMetrics
  db_size_mb: number
  policy?: {
    hot_log_days: number
    auto_archive_days: number
    summary_days: number
    purge_mode: string
  }
}

interface TableMetrics {
  to_purge: number
  rows: number
  size_mb: number
  oldest_at?: string | null
}

const emptyTableMetrics: TableMetrics = {
  to_purge: 0,
  rows: 0,
  size_mb: 0,
  oldest_at: null,
}

export default function OldDataSettings() {
  const [archiveDays, setArchiveDays] = useState(180)
  const [purgeDays, setPurgeDays] = useState(730)
  const [summary, setSummary] = useState<Summary | null>(null)
  const [loading, setLoading] = useState(false)
  const [archiving, setArchiving] = useState(false)
  const [purging, setPurging] = useState(false)

  const [confirmPurge, setConfirmPurge] = useState(false)
  const [purgeBills, setPurgeBills] = useState(false)
  const [purgeAudit, setPurgeAudit] = useState(false)
  const [purgeAI, setPurgeAI] = useState(false)
  const [purgeChat, setPurgeChat] = useState(false)

  const fetchSummary = async () => {
    setLoading(true)
    try {
      const res = await api.get('/bills/old-data/summary', {
        params: { archive_days: archiveDays, purge_days: purgeDays },
      })
      setSummary(res.data)
    } catch {
      toast.error('โหลดข้อมูลไม่ได้')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { fetchSummary() }, [])

  const handleArchive = async () => {
    setArchiving(true)
    try {
      const res = await api.post('/bills/old-data/archive', { archive_days: archiveDays })
      toast.success(`เก็บบิลสำเร็จ ${res.data.archived} รายการ`)
      fetchSummary()
    } catch (e: any) {
      toast.error(e?.response?.data?.error || 'archive ไม่สำเร็จ')
    } finally {
      setArchiving(false)
    }
  }

  const handlePurge = async () => {
    setPurging(true)
    setConfirmPurge(false)
    try {
      const res = await api.post('/bills/old-data/purge', {
        purge_days: purgeDays,
        purge_bills: purgeBills,
        purge_audit: purgeAudit,
        purge_ai: purgeAI,
        purge_chat: purgeChat,
      })
      const parts: string[] = []
      if (res.data.purged_bills != null) parts.push(`บิล ${res.data.purged_bills} รายการ`)
      if (res.data.purged_audit_logs != null) parts.push(`audit log ${res.data.purged_audit_logs} รายการ`)
      if (res.data.purged_ai_usage_logs != null) parts.push(`AI usage ${res.data.purged_ai_usage_logs} รายการ`)
      if (res.data.purged_chat_messages != null) parts.push(`chat ${res.data.purged_chat_messages} ข้อความ`)
      toast.success(`ลบข้อมูลสำเร็จ: ${parts.join(', ')}`)
      fetchSummary()
    } catch (e: any) {
      toast.error(e?.response?.data?.error || 'purge ไม่สำเร็จ')
    } finally {
      setPurging(false)
    }
  }

  const anyPurgeSelected = purgeBills || purgeAudit || purgeAI || purgeChat

  // Safe accessors — API may return partial data during loading transitions
  const toArchive = summary?.bills?.to_archive ?? 0
  const toPurgeBills = summary?.bills?.to_purge ?? 0
  const archivedCount = summary?.bills?.archived ?? 0
  const auditMetrics = summary?.audit_logs ?? emptyTableMetrics
  const aiUsageMetrics = summary?.ai_usage_logs ?? emptyTableMetrics
  const chatMetrics = summary?.chat_messages ?? emptyTableMetrics
  const toPurgeAudit = auditMetrics.to_purge
  const toPurgeAI = aiUsageMetrics.to_purge
  const toPurgeChat = chatMetrics.to_purge
  const dbSizeMB = summary?.db_size_mb ?? 0

  return (
    <div className="p-6 max-w-3xl mx-auto space-y-6">
      <PageHeader
        title="จัดการข้อมูลเก่า"
        description="Archive บิลเก่าเพื่อซ่อนจาก queue, หรือลบข้อมูลที่ไม่ต้องการออกจาก database เพื่อประหยัด disk"
      />

      {/* Threshold config */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm">กำหนด Threshold</CardTitle>
          <CardDescription>เลือกจำนวนวันสำหรับแต่ละ operation แล้วกด "คำนวณใหม่"</CardDescription>
        </CardHeader>
        <CardContent className="grid grid-cols-2 gap-4">
          <div className="space-y-1.5">
            <Label htmlFor="archive-days">Archive บิลที่เก่ากว่า (วัน)</Label>
            <Input
              id="archive-days"
              type="number"
              min={30}
              value={archiveDays}
              onChange={e => setArchiveDays(Number(e.target.value))}
            />
            <p className="text-[11px] text-muted-foreground">บิล sent/failed ที่เก่ากว่านี้จะถูก archive (ซ่อนจาก queue ปกติ)</p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="purge-days">Purge ข้อมูลที่เก่ากว่า (วัน)</Label>
            <Input
              id="purge-days"
              type="number"
              min={90}
              value={purgeDays}
              onChange={e => setPurgeDays(Number(e.target.value))}
            />
            <p className="text-[11px] text-muted-foreground">ข้อมูลที่เก่ากว่านี้จะถูกลบออกจาก DB จริง (ย้อนกลับไม่ได้)</p>
          </div>
          <div className="col-span-2">
            <Button variant="outline" size="sm" onClick={fetchSummary} disabled={loading}>
              <RefreshCw className={`h-3.5 w-3.5 mr-1.5 ${loading ? 'animate-spin' : ''}`} />
              คำนวณใหม่
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Summary */}
      {summary && (
        <div className="grid grid-cols-2 gap-4">
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm flex items-center gap-2">
                <Database className="h-4 w-4 text-muted-foreground" />
                ขนาด Database
              </CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-2xl font-semibold tabular-nums">
                {dbSizeMB.toFixed(1)}{' '}
                <span className="text-sm font-normal text-muted-foreground">MB</span>
              </p>
            </CardContent>
          </Card>
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm flex items-center gap-2">
                <Archive className="h-4 w-4 text-muted-foreground" />
                บิลที่ Archive แล้ว
              </CardTitle>
            </CardHeader>
            <CardContent>
              <p className="text-2xl font-semibold tabular-nums">{archivedCount.toLocaleString()}</p>
            </CardContent>
          </Card>
        </div>
      )}

      {/* Archive section */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm flex items-center gap-2">
            <Archive className="h-4 w-4" />
            Archive บิลเก่า
          </CardTitle>
          <CardDescription>
            ซ่อนบิลเก่าออกจาก queue ปกติ — ข้อมูลยังอยู่ใน DB ค้นหาได้ ย้อนกลับได้
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {summary && (
            <div className="rounded-lg border bg-muted/30 p-4">
              <p className="text-sm">
                บิล <span className="font-semibold tabular-nums text-foreground">{toArchive.toLocaleString()}</span> รายการ
                ที่มีสถานะ sent/failed/skipped และเก่ากว่า{' '}
                <span className="font-semibold">{summary.archive_days}</span> วัน จะถูก archive
              </p>
            </div>
          )}
          <Button
            onClick={handleArchive}
            disabled={archiving || !summary || toArchive === 0}
            variant="outline"
          >
            <Archive className={`h-4 w-4 mr-2 ${archiving ? 'animate-pulse' : ''}`} />
            {archiving ? 'กำลัง Archive…' : `Archive ${toArchive.toLocaleString()} รายการ`}
          </Button>
        </CardContent>
      </Card>

      {/* Purge section */}
      <Card className="border-destructive/30">
        <CardHeader>
          <CardTitle className="text-sm flex items-center gap-2 text-destructive">
            <Trash2 className="h-4 w-4" />
            Purge ข้อมูลเก่า (ย้อนกลับไม่ได้)
          </CardTitle>
          <CardDescription>
            ลบข้อมูลออกจาก database จริง — ใช้เพื่อประหยัด disk เท่านั้น ข้อมูลหายถาวร
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {summary?.policy && (
            <div className="rounded-lg border bg-info/5 p-3 text-xs text-muted-foreground">
              Retention ปัจจุบัน: detailed logs {summary.policy.hot_log_days} วัน · auto-archive sent/skipped {summary.policy.auto_archive_days} วัน · summary {summary.policy.summary_days} วัน · purge mode {summary.policy.purge_mode}
            </div>
          )}
          {summary && (
            <div className="rounded-lg border bg-muted/30 p-4 space-y-2 text-sm">
              <p className="font-medium text-muted-foreground">ข้อมูลที่จะถูกลบเมื่อเก่ากว่า {summary.purge_days} วัน:</p>
              <ul className="space-y-1 text-muted-foreground">
                <li>บิลทั้งหมด: <span className="font-semibold text-foreground tabular-nums">{toPurgeBills.toLocaleString()}</span> รายการ</li>
                <li>Audit log: <span className="font-semibold text-foreground tabular-nums">{toPurgeAudit.toLocaleString()}</span> / {auditMetrics.rows.toLocaleString()} รายการ · {auditMetrics.size_mb.toFixed(1)} MB</li>
                <li>AI usage: <span className="font-semibold text-foreground tabular-nums">{toPurgeAI.toLocaleString()}</span> / {aiUsageMetrics.rows.toLocaleString()} รายการ · {aiUsageMetrics.size_mb.toFixed(1)} MB</li>
                <li>Chat messages: <span className="font-semibold text-foreground tabular-nums">{toPurgeChat.toLocaleString()}</span> / {chatMetrics.rows.toLocaleString()} ข้อความ · {chatMetrics.size_mb.toFixed(1)} MB</li>
              </ul>
            </div>
          )}

          <div className="space-y-2">
            <p className="text-xs font-medium text-muted-foreground">เลือกข้อมูลที่ต้องการลบ:</p>
            <div className="flex flex-col gap-2">
              <label className="flex items-center gap-2 text-sm cursor-pointer">
                <Checkbox checked={purgeBills} onCheckedChange={v => setPurgeBills(!!v)} />
                บิล (bills + bill_items)
              </label>
              <label className="flex items-center gap-2 text-sm cursor-pointer">
                <Checkbox checked={purgeAudit} onCheckedChange={v => setPurgeAudit(!!v)} />
                Audit log
              </label>
              <label className="flex items-center gap-2 text-sm cursor-pointer">
                <Checkbox checked={purgeAI} onCheckedChange={v => setPurgeAI(!!v)} />
                AI usage log
              </label>
              <label className="flex items-center gap-2 text-sm cursor-pointer">
                <Checkbox checked={purgeChat} onCheckedChange={v => setPurgeChat(!!v)} />
                Chat messages
              </label>
            </div>
          </div>

          <Button
            variant="destructive"
            onClick={() => setConfirmPurge(true)}
            disabled={purging || !anyPurgeSelected}
          >
            <Trash2 className="h-4 w-4 mr-2" />
            {purging ? 'กำลังลบ…' : 'Purge ข้อมูลเก่า'}
          </Button>
        </CardContent>
      </Card>

      {/* Confirm purge dialog */}
      <Dialog open={confirmPurge} onOpenChange={setConfirmPurge}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-destructive">
              <AlertTriangle className="h-5 w-5" />
              ยืนยันการลบข้อมูล
            </DialogTitle>
            <DialogDescription className="space-y-2">
              <span className="block">
                ข้อมูลที่เก่ากว่า <strong>{purgeDays} วัน</strong> จะถูกลบออกจาก database ถาวร
              </span>
              <span className="block text-destructive font-medium">
                ไม่สามารถย้อนกลับได้ — ตรวจสอบให้แน่ใจก่อนดำเนินการ
              </span>
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmPurge(false)}>ยกเลิก</Button>
            <Button variant="destructive" onClick={handlePurge}>
              ยืนยัน ลบข้อมูล
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
