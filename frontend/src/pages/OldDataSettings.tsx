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
  audit_logs: { to_purge: number }
  chat_messages: { to_purge: number }
  db_size_mb: number
}

export default function OldDataSettings() {
  const [archiveDays, setArchiveDays] = useState(180)
  const [purgeDays, setPurgeDays] = useState(730)
  const [summary, setSummary] = useState<Summary | null>(null)
  const [loading, setLoading] = useState(false)
  const [archiving, setArchiving] = useState(false)
  const [purging, setPurging] = useState(false)

  const [confirmPurge, setConfirmPurge] = useState(false)
  const [purgeBills, setPurgeBills] = useState(true)
  const [purgeAudit, setPurgeAudit] = useState(false)
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
        purge_chat: purgeChat,
      })
      const parts = []
      if (res.data.purged_bills != null) parts.push(`บิล ${res.data.purged_bills} รายการ`)
      if (res.data.purged_audit_logs != null) parts.push(`audit log ${res.data.purged_audit_logs} รายการ`)
      if (res.data.purged_chat_messages != null) parts.push(`chat ${res.data.purged_chat_messages} ข้อความ`)
      toast.success(`ลบข้อมูลสำเร็จ: ${parts.join(', ')}`)
      fetchSummary()
    } catch (e: any) {
      toast.error(e?.response?.data?.error || 'purge ไม่สำเร็จ')
    } finally {
      setPurging(false)
    }
  }

  const anyPurgeSelected = purgeBills || purgeAudit || purgeChat

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
              <p className="text-2xl font-semibold tabular-nums">{summary.db_size_mb.toFixed(1)} <span className="text-sm font-normal text-muted-foreground">MB</span></p>
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
              <p className="text-2xl font-semibold tabular-nums">{summary.bills.archived.toLocaleString()}</p>
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
                บิล <span className="font-semibold tabular-nums text-foreground">{summary.bills.to_archive.toLocaleString()}</span> รายการ
                ที่มีสถานะ sent/failed/skipped และเก่ากว่า{' '}
                <span className="font-semibold">{summary.archive_days}</span> วัน จะถูก archive
              </p>
            </div>
          )}
          <Button
            onClick={handleArchive}
            disabled={archiving || !summary || summary.bills.to_archive === 0}
            variant="outline"
          >
            <Archive className={`h-4 w-4 mr-2 ${archiving ? 'animate-pulse' : ''}`} />
            {archiving ? 'กำลัง Archive…' : `Archive ${summary?.bills.to_archive.toLocaleString() ?? 0} รายการ`}
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
          {summary && (
            <div className="rounded-lg border bg-muted/30 p-4 space-y-2 text-sm">
              <p className="font-medium text-muted-foreground">ข้อมูลที่จะถูกลบเมื่อเก่ากว่า {summary.purge_days} วัน:</p>
              <ul className="space-y-1 text-muted-foreground">
                <li>บิลทั้งหมด: <span className="font-semibold text-foreground tabular-nums">{summary.bills.to_purge.toLocaleString()}</span> รายการ</li>
                <li>Audit log: <span className="font-semibold text-foreground tabular-nums">{summary.audit_logs.to_purge.toLocaleString()}</span> รายการ</li>
                <li>Chat messages: <span className="font-semibold text-foreground tabular-nums">{summary.chat_messages.to_purge.toLocaleString()}</span> ข้อความ</li>
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
            <Button
              variant="destructive"
              onClick={handlePurge}
            >
              ยืนยัน ลบข้อมูล
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
