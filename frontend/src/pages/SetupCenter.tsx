import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { ArrowRight, CheckCircle2, CircleAlert, CircleDot, RefreshCw } from 'lucide-react'

import client from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { PageHeader } from '@/components/common/PageHeader'
import { cn } from '@/lib/utils'

type SetupStep = {
  key: string
  title: string
  description: string
  href: string
  ready: boolean
  status: string
  missing?: string[]
}

type SetupStatus = {
  ready: boolean
  ready_count: number
  total_count: number
  pending_restart?: boolean
  steps: SetupStep[]
}

export default function SetupCenter() {
  const [status, setStatus] = useState<SetupStatus | null>(null)
  const [loading, setLoading] = useState(true)

  const load = async () => {
    setLoading(true)
    try {
      const res = await client.get<SetupStatus>('/api/setup/status')
      setStatus(res.data)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const nextStep = useMemo(
    () => status?.steps.find((s) => !s.ready) ?? status?.steps[0],
    [status],
  )
  const pct = status ? Math.round((status.ready_count / status.total_count) * 100) : 0

  return (
    <div className="space-y-5">
      <PageHeader
        title="เริ่มต้นใช้งาน"
        description="เช็กลำดับการตั้งค่าร้านนี้ให้พร้อมก่อนรับบิลและส่งเข้า SML"
        actions={
          <Button variant="outline" size="sm" onClick={load} disabled={loading}>
            <RefreshCw className={cn('h-4 w-4', loading && 'animate-spin')} />
            ตรวจสถานะใหม่
          </Button>
        }
      />

      <Card className={cn('border-border/70', status?.ready ? 'bg-success/[0.04]' : 'bg-warning/[0.05]')}>
        <CardContent className="flex flex-col gap-4 p-4 md:flex-row md:items-center md:justify-between">
          <div>
            <div className="flex items-center gap-2">
              {status?.ready ? (
                <CheckCircle2 className="h-5 w-5 text-success" />
              ) : (
                <CircleAlert className="h-5 w-5 text-warning" />
              )}
              <p className="text-sm font-semibold">
                {status?.ready ? 'ร้านนี้พร้อมใช้งานแล้ว' : 'ตั้งค่ายังไม่ครบ'}
              </p>
            </div>
            <p className="mt-1 text-xs text-muted-foreground">
              {status ? `${status.ready_count}/${status.total_count} ขั้นพร้อมใช้งาน` : 'กำลังตรวจสถานะ...'}
            </p>
          </div>
          <div className="h-2 w-full overflow-hidden rounded-full bg-muted md:max-w-xs">
            <div className="h-full rounded-full bg-primary transition-all" style={{ width: `${pct}%` }} />
          </div>
          {nextStep && !status?.ready && (
            <Button asChild size="sm">
              <Link to={nextStep.href}>
                ไปตั้งค่าขั้นต่อไป
                <ArrowRight className="h-4 w-4" />
              </Link>
            </Button>
          )}
        </CardContent>
      </Card>

      <div className="grid gap-3">
        {(status?.steps ?? []).map((step, index) => (
          <Card key={step.key} className={cn('border-border/70', step.ready && 'bg-success/[0.03]')}>
            <CardContent className="flex flex-col gap-3 p-4 md:flex-row md:items-center">
              <div className={cn(
                'flex h-9 w-9 shrink-0 items-center justify-center rounded-full border text-sm font-semibold',
                step.ready ? 'border-success/30 bg-success/10 text-success' : 'border-warning/30 bg-warning/10 text-warning',
              )}>
                {step.ready ? <CheckCircle2 className="h-4 w-4" /> : index + 1}
              </div>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <h2 className="text-sm font-semibold">{step.title}</h2>
                  <Badge variant={step.ready ? 'default' : 'outline'} className="h-5 px-1.5 text-[10px]">
                    {step.status}
                  </Badge>
                </div>
                <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{step.description}</p>
                {!!step.missing?.length && (
                  <div className="mt-2 flex flex-wrap gap-1">
                    {step.missing.map((m) => (
                      <Badge key={m} variant="secondary" className="h-5 px-1.5 text-[10px]">
                        <CircleDot className="h-3 w-3" />
                        {m}
                      </Badge>
                    ))}
                  </div>
                )}
              </div>
              <Button asChild variant={step.ready ? 'outline' : 'default'} size="sm" className="md:self-center">
                <Link to={step.href}>
                  {step.ready ? 'ตรวจดู' : 'ไปตั้งค่า'}
                  <ArrowRight className="h-4 w-4" />
                </Link>
              </Button>
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  )
}
