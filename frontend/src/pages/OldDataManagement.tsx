import { useCallback, useEffect, useState } from 'react'
import { Archive, Database, RefreshCw, ShieldCheck, Trash2 } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { toast } from 'sonner'

import client from '@/api/client'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { EmptyState } from '@/components/common/EmptyState'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { useAuth } from '@/hooks/useAuth'
import { cn } from '@/lib/utils'

interface OldDataSummary {
  active_total: number
  archived_total: number
  sent_older_than_90_days: number
  sent_older_than_180_days: number
  sent_older_than_365_days: number
  purge_eligible_730_days: number
  archive_eligible_count: number
  purge_eligible_count: number
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

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const res = await client.get<{ data: OldDataSummary }>('/api/bills/old-data/summary', {
        params: {
          archive_days: archiveDays,
          purge_days: purgeDays,
        },
      })
      setSummary(res.data.data)
    } catch {
      toast.error('โหลดข้อมูลเก่าไม่สำเร็จ')
    } finally {
      setLoading(false)
    }
  }, [archiveDays, purgeDays])

  useEffect(() => {
    const timer = setTimeout(() => void load(), 250)
    return () => clearTimeout(timer)
  }, [load])

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

  const archiveEligible = summary?.archive_eligible_count ?? 0
  const purgeEligible = summary?.purge_eligible_count ?? 0

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-2xl font-semibold tracking-normal text-foreground">จัดการข้อมูลเก่า</h1>
          <p className="mt-1 max-w-3xl text-sm leading-6 text-muted-foreground">
            ใช้เคลียร์หน้าทำงานให้เบาลงโดยไม่เสียประวัติ: เก็บบิลคือซ่อนจากรายการปกติ ส่วนลบถาวรใช้กับบิลที่เก็บไว้นานแล้วเท่านั้น
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void load()} disabled={loading}>
          <RefreshCw className={cn('h-3.5 w-3.5', loading && 'animate-spin')} />
          รีเฟรช
        </Button>
      </div>

      <div className="rounded-lg border bg-card px-3 py-2">
        <div className="flex flex-wrap items-center gap-x-5 gap-y-2 text-sm">
          <SummaryItem label="รายการปกติ" value={summary?.active_total} loading={loading} />
          <SummaryItem label="บิลที่เก็บแล้ว" value={summary?.archived_total} loading={loading} />
          <SummaryItem label="เก่ากว่า 6 เดือน" value={summary?.sent_older_than_180_days} loading={loading} />
          <SummaryItem label="พร้อมลบถาวร" value={summary?.purge_eligible_730_days} loading={loading} />
          {summary && summary.archived_artifact_count > 0 && (
            <Badge variant="outline" className="h-6">
              ไฟล์แนบในบิลที่เก็บแล้ว {summary.archived_artifact_count.toLocaleString()} รายการ
            </Badge>
          )}
        </div>
      </div>

      <div className="rounded-lg border bg-card">
        <OperationRow
          icon={Archive}
          title="เก็บบิล"
          tone="primary"
          description="ซ่อนบิลที่ส่ง SML สำเร็จแล้วหรือข้ามแล้วออกจากหน้างานประจำ แต่ยังค้นกลับมา เปิดดู และดู logs ได้"
          helper="ไม่แตะบิลที่ยังต้องตรวจ พร้อมส่ง หรือส่งไม่สำเร็จ"
          daysLabel="เก่ากว่า (วัน)"
          days={archiveDays}
          min={30}
          onDaysChange={setArchiveDays}
          previewCount={archiveEligible}
          previewLabel="บิลที่จะถูกเก็บ"
          buttonLabel="เก็บบิล"
          disabled={!isAdmin || archiveEligible === 0}
          onClick={() => setAction('archive')}
        />

        <div className="border-t" />

        <OperationRow
          icon={Trash2}
          title="ลบถาวร"
          tone="danger"
          description="ลบข้อมูลบิลและไฟล์แนบ คืนไม่ได้ ใช้เฉพาะบิลที่เก็บไว้นานแล้วเพื่อลดข้อมูลสะสมระยะยาว"
          helper="Logs สำคัญยังอยู่ เช่น เลขบิล เลข SML ช่องทาง และผู้ทำรายการ"
          daysLabel="เก็บไว้นานกว่า (วัน)"
          days={purgeDays}
          min={365}
          onDaysChange={setPurgeDays}
          previewCount={purgeEligible}
          previewLabel="บิลที่จะถูกลบถาวร"
          buttonLabel="ลบถาวร"
          disabled={!isAdmin || purgeEligible === 0}
          onClick={() => setAction('purge')}
        />
      </div>

      {!isAdmin && (
        <div className="flex items-center gap-2 rounded-lg border bg-muted/30 px-3 py-2 text-sm text-muted-foreground">
          <ShieldCheck className="h-4 w-4" />
          เฉพาะผู้ดูแลระบบเท่านั้นที่ทำรายการเก็บบิลเก่าและลบถาวรได้
        </div>
      )}

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
            ? `จะลบถาวรเฉพาะบิลที่เก็บแล้วเก่ากว่า ${purgeDays} วัน จำนวน ${purgeEligible.toLocaleString()} รายการ คืนไม่ได้`
            : `จะเก็บบิลที่ส่งสำเร็จแล้วหรือข้ามแล้ว เก่ากว่า ${archiveDays} วัน จำนวน ${archiveEligible.toLocaleString()} รายการ`
        }
        confirmLabel={action === 'purge' ? 'ลบถาวร' : 'เก็บบิล'}
        variant={action === 'purge' ? 'destructive' : 'default'}
        onConfirm={runAction}
      />
    </div>
  )
}

function SummaryItem({ label, value, loading }: { label: string; value?: number; loading: boolean }) {
  return (
    <span className="inline-flex items-baseline gap-1.5">
      {loading ? (
        <Skeleton className="h-5 w-12" />
      ) : (
        <span className="text-lg font-semibold tabular-nums text-foreground">{(value ?? 0).toLocaleString()}</span>
      )}
      <span className="text-xs text-muted-foreground">{label}</span>
    </span>
  )
}

function OperationRow({
  icon: Icon,
  title,
  tone,
  description,
  helper,
  daysLabel,
  days,
  min,
  onDaysChange,
  previewCount,
  previewLabel,
  buttonLabel,
  disabled,
  onClick,
}: {
  icon: LucideIcon
  title: string
  tone: 'primary' | 'danger'
  description: string
  helper: string
  daysLabel: string
  days: number
  min: number
  onDaysChange: (value: number) => void
  previewCount: number
  previewLabel: string
  buttonLabel: string
  disabled: boolean
  onClick: () => void
}) {
  const isDanger = tone === 'danger'
  return (
    <div className="grid gap-3 p-3 lg:grid-cols-[minmax(0,1fr)_150px_160px_190px] lg:items-center">
      <div className="min-w-0">
        <div className={cn('flex items-center gap-2 font-medium', isDanger ? 'text-destructive' : 'text-foreground')}>
          <Icon className={cn('h-4 w-4', isDanger ? 'text-destructive' : 'text-primary')} />
          {title}
        </div>
        <p className="mt-1 text-sm leading-6 text-muted-foreground">{description}</p>
        <p className="text-xs text-muted-foreground">{helper}</p>
      </div>

      <div>
        <label className="text-xs font-medium text-muted-foreground">{daysLabel}</label>
        <Input
          type="number"
          min={min}
          value={days}
          onChange={(e) => onDaysChange(Number(e.target.value) || min)}
          className="mt-1 h-9"
        />
      </div>

      <div className="rounded-md border bg-muted/30 px-3 py-2">
        <div className="text-lg font-semibold tabular-nums">{previewCount.toLocaleString()}</div>
        <div className="text-xs text-muted-foreground">{previewLabel}</div>
      </div>

      <Button
        variant={isDanger ? 'destructive' : 'default'}
        className="w-full"
        disabled={disabled}
        onClick={onClick}
      >
        <Icon className="h-4 w-4" />
        {buttonLabel}
      </Button>
    </div>
  )
}
