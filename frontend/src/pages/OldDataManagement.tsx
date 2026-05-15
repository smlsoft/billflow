import { useEffect, useState } from 'react'
import { Archive, Database, RefreshCw, Trash2 } from 'lucide-react'
import { toast } from 'sonner'

import client from '@/api/client'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { EmptyState } from '@/components/common/EmptyState'
import { PageHeader } from '@/components/common/PageHeader'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuth } from '@/hooks/useAuth'

interface OldDataSummary {
  active_total: number
  archived_total: number
  sent_older_than_90_days: number
  sent_older_than_180_days: number
  sent_older_than_365_days: number
  purge_eligible_730_days: number
  archived_artifact_count: number
}

export default function OldDataManagement() {
  const { user } = useAuth()
  const isAdmin = user?.role === 'admin'
  const [summary, setSummary] = useState<OldDataSummary | null>(null)
  const [loading, setLoading] = useState(true)
  const [archiveDays, setArchiveDays] = useState(180)
  const [purgeDays, setPurgeDays] = useState(730)
  const [action, setAction] = useState<'archive' | 'purge' | null>(null)

  const load = async () => {
    setLoading(true)
    try {
      const res = await client.get<{ data: OldDataSummary }>('/api/bills/old-data/summary')
      setSummary(res.data.data)
    } catch {
      toast.error('โหลดข้อมูลเก่าไม่สำเร็จ')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const runAction = async () => {
    if (!action) return
    try {
      if (action === 'archive') {
        const res = await client.post<{ archived: number }>('/api/bills/archive-old', {
          older_than_days: archiveDays,
        })
        toast.success(`เก็บบิลแล้ว ${res.data.archived ?? 0} รายการ`)
      } else {
        const res = await client.post<{ purged: number; failed?: string[] }>('/api/bills/purge-old', {
          older_than_days: purgeDays,
          confirm: 'DELETE',
        })
        toast.success(`ลบถาวรแล้ว ${res.data.purged ?? 0} รายการ`)
        if ((res.data.failed ?? []).length > 0) {
          toast.warning(`มี ${res.data.failed?.length} รายการที่ลบไม่สำเร็จ`)
        }
      }
      setAction(null)
      await load()
    } catch (err: unknown) {
      const e = err as { response?: { data?: { error?: string } }; message?: string }
      toast.error(e?.response?.data?.error || e?.message || 'ทำรายการไม่สำเร็จ')
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title="จัดการข้อมูลเก่า"
        description="เก็บบิลเก่าออกจากหน้างานประจำ และลบถาวรเฉพาะข้อมูลที่เก็บไว้นานแล้ว"
      />

      <div className="grid gap-3 md:grid-cols-4">
        <SummaryCard title="รายการปกติ" value={summary?.active_total} loading={loading} />
        <SummaryCard title="บิลที่เก็บแล้ว" value={summary?.archived_total} loading={loading} />
        <SummaryCard title="เก่ากว่า 6 เดือน" value={summary?.sent_older_than_180_days} loading={loading} />
        <SummaryCard title="พร้อมลบถาวร" value={summary?.purge_eligible_730_days} loading={loading} />
      </div>

      <Card>
        <CardHeader className="border-b">
          <CardTitle className="flex items-center gap-2 text-base">
            <Archive className="h-4 w-4 text-primary" />
            เก็บบิลที่ส่งสำเร็จแล้ว
          </CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 p-4 lg:grid-cols-[minmax(0,1fr)_260px]">
          <div className="space-y-2 text-sm text-muted-foreground">
            <p>
              เก็บบิลจะซ่อนบิลออกจากหน้างานประจำ แต่ยังเปิดดูย้อนหลัง ค้นหา และดูประวัติการทำงานได้
            </p>
            <p>
              ระบบจะเก็บเฉพาะบิลสถานะส่งเข้า SML แล้วหรือข้ามแล้ว ไม่แตะบิลที่ยังต้องตรวจ/พร้อมส่ง/ส่งไม่สำเร็จ
            </p>
          </div>
          <div className="space-y-2">
            <label className="text-xs font-medium text-muted-foreground">เก่ากว่า (วัน)</label>
            <Input
              type="number"
              min={30}
              value={archiveDays}
              onChange={(e) => setArchiveDays(Number(e.target.value) || 180)}
            />
            <Button className="w-full" onClick={() => setAction('archive')} disabled={!isAdmin}>
              <Archive className="mr-2 h-4 w-4" />
              เก็บบิลที่ส่งสำเร็จแล้ว
            </Button>
            {!isAdmin && <p className="text-xs text-muted-foreground">เฉพาะผู้ดูแลระบบเท่านั้น</p>}
          </div>
        </CardContent>
      </Card>

      <Card className="border-destructive/30">
        <CardHeader className="border-b">
          <CardTitle className="flex items-center gap-2 text-base text-destructive">
            <Trash2 className="h-4 w-4" />
            ลบถาวร
          </CardTitle>
        </CardHeader>
        <CardContent className="grid gap-4 p-4 lg:grid-cols-[minmax(0,1fr)_260px]">
          <div className="space-y-2 text-sm text-muted-foreground">
            <p>
              ลบถาวรจะลบข้อมูลบิลและไฟล์แนบ คืนไม่ได้ ใช้เฉพาะบิลที่เก็บไว้นานแล้วเท่านั้น
            </p>
            <p>
              Logs จะยังอยู่พร้อมข้อมูลสำคัญ เช่น เลขบิล เลข SML ช่องทาง และผู้ทำรายการ
            </p>
            {summary && summary.archived_artifact_count > 0 && (
              <p>มีไฟล์แนบในบิลที่เก็บแล้ว {summary.archived_artifact_count.toLocaleString()} รายการ</p>
            )}
          </div>
          <div className="space-y-2">
            <label className="text-xs font-medium text-muted-foreground">เก็บไว้นานกว่า (วัน)</label>
            <Input
              type="number"
              min={365}
              value={purgeDays}
              onChange={(e) => setPurgeDays(Number(e.target.value) || 730)}
            />
            <Button variant="destructive" className="w-full" onClick={() => setAction('purge')} disabled={!isAdmin}>
              <Trash2 className="mr-2 h-4 w-4" />
              ลบถาวร
            </Button>
            {!isAdmin && <p className="text-xs text-muted-foreground">เฉพาะผู้ดูแลระบบเท่านั้น</p>}
          </div>
        </CardContent>
      </Card>

      {!loading && !summary && (
        <EmptyState
          icon={Database}
          title="ยังโหลดสรุปข้อมูลไม่ได้"
          description="ลองรีเฟรชอีกครั้ง หรือตรวจสอบ backend/logs"
          action={<Button onClick={load}><RefreshCw className="h-4 w-4" /> รีเฟรช</Button>}
        />
      )}

      <ConfirmDialog
        open={action !== null}
        onOpenChange={(open) => !open && setAction(null)}
        title={action === 'purge' ? 'ยืนยันลบถาวร?' : 'ยืนยันเก็บบิลเก่า?'}
        description={
          action === 'purge'
            ? `จะลบถาวรเฉพาะบิลที่เก็บแล้วเก่ากว่า ${purgeDays} วัน คืนไม่ได้`
            : `จะเก็บบิลที่ส่งสำเร็จแล้วหรือข้ามแล้ว เก่ากว่า ${archiveDays} วัน`
        }
        confirmLabel={action === 'purge' ? 'ลบถาวร' : 'เก็บบิล'}
        variant={action === 'purge' ? 'destructive' : 'default'}
        onConfirm={runAction}
      />
    </div>
  )
}

function SummaryCard({ title, value, loading }: { title: string; value?: number; loading: boolean }) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        {loading ? <Skeleton className="h-8 w-20" /> : <div className="text-2xl font-bold tabular-nums">{(value ?? 0).toLocaleString()}</div>}
      </CardContent>
    </Card>
  )
}
