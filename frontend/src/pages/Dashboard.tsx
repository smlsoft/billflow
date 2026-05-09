import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { AlertTriangle, CheckCircle2, FileText, Mail, Send, ShoppingBag, Sparkles } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import InsightCard from '@/components/InsightCard'
import LearningProgress from '@/components/LearningProgress'
import { PageHeader } from '@/components/common/PageHeader'
import client from '@/api/client'
import { useAuthStore } from '@/store/auth'
import { ENABLE_SALES_ORDERS, ENABLE_SHOPEE_EXCEL } from '@/lib/featureFlags'
import type { DailyInsight, DashboardStats, MappingStats } from '@/types'
import { ActionCards } from './Dashboard/ActionCards'

const PHASE = Number(import.meta.env.VITE_PHASE ?? 99)

type SetupStatus = { ready: boolean; ready_count: number; total_count: number }

export default function Dashboard() {
  const [stats, setStats] = useState<DashboardStats | null>(null)
  const [insight, setInsight] = useState<DailyInsight | null>(null)
  const [mapStats, setMapStats] = useState<MappingStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [generating, setGenerating] = useState(false)
  const [setupStatus, setSetupStatus] = useState<SetupStatus | null>(null)
  const user = useAuthStore((s) => s.user)

  const loadInsight = () =>
    client
      .get<{ data: DailyInsight[] }>('/api/dashboard/insights')
      .then((r) => setInsight(r.data.data?.[0] ?? null))
      .catch(() => null)

  useEffect(() => {
    Promise.all([
      client
        .get<DashboardStats>('/api/dashboard/stats')
        .then((r) => setStats(r.data))
        .catch(() => null),
      loadInsight(),
      client
        .get<MappingStats>('/api/mappings/stats')
        .then((r) => setMapStats(r.data))
        .catch(() => null),
      client
        .get<SetupStatus>('/api/setup/status')
        .then((r) => setSetupStatus(r.data))
        .catch(() => null),
    ]).finally(() => setLoading(false))
  }, [])

  const handleGenerate = async () => {
    setGenerating(true)
    try {
      await client.post('/api/dashboard/insights/generate')
      await loadInsight()
      toast.success('สร้าง Insight สำเร็จ')
    } catch {
      toast.error('ไม่สามารถสร้าง Insight ได้')
    } finally {
      setGenerating(false)
    }
  }

  const awaitingReview = (stats?.pending ?? 0) + (stats?.needs_review ?? 0)

  return (
    <div className="space-y-5">
      <PageHeader
        title="BillFlow Review Desk"
        description={ENABLE_SALES_ORDERS ? 'โต๊ะงานสำหรับตรวจเอกสารจากทุกช่องทาง แล้วส่งเป็นใบสั่งซื้อ/ใบสั่งขายเข้า SML' : 'โต๊ะงานสำหรับตรวจบิลซื้อจากอีเมล แล้วส่งเป็นใบสั่งซื้อเข้า SML'}
        actions={
          PHASE >= 2 && user?.role === 'admin' && (
            <Button size="sm" onClick={handleGenerate} disabled={generating}>
              <Sparkles className="h-4 w-4" />
              {generating ? 'กำลังสร้าง…' : 'สร้าง AI Insight'}
            </Button>
          )
        }
      />

      {setupStatus && !setupStatus.ready && (
        <Card className="border-warning/35 bg-warning/[0.07]">
          <CardContent className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex items-start gap-2.5">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
              <div>
                <p className="text-sm font-semibold">ระบบยังตั้งค่าไม่ครบ</p>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  พร้อมแล้ว {setupStatus.ready_count}/{setupStatus.total_count} ขั้น กรุณาตรวจหน้าเริ่มต้นใช้งานก่อนเริ่มรับบิลจริง
                </p>
              </div>
            </div>
            <Button asChild size="sm">
              <Link to="/setup">ไปที่เริ่มต้นใช้งาน</Link>
            </Button>
          </CardContent>
        </Card>
      )}

      {setupStatus?.ready && !loading && (stats?.total_bills ?? 0) === 0 && (
        <Card className="border-primary/25 bg-primary/[0.04]">
          <CardContent className="flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex items-start gap-2.5">
              <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-primary" />
              <div>
                <p className="text-sm font-semibold">ระบบพร้อมแล้ว แต่ยังไม่มีเอกสาร</p>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  เริ่มจากนำเข้า Shopee Excel หรือกดดึงอีเมล Shopee เพื่อสร้างใบสั่งซื้อเข้าคิวตรวจ
                </p>
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              {ENABLE_SHOPEE_EXCEL && ENABLE_SALES_ORDERS && (
                <Button asChild size="sm">
                  <Link to="/import/shopee">นำเข้า Shopee Excel</Link>
                </Button>
              )}
              <Button asChild size="sm" variant="outline">
                <Link to="/settings/email">ดึงอีเมลรับบิล</Link>
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      <Card className="overflow-hidden rounded-2xl border-border/70 bg-card shadow-sm">
        <CardContent className="grid gap-0 p-0 lg:grid-cols-[1.15fr_0.85fr]">
          <div className="border-b border-border/70 p-5 lg:border-b-0 lg:border-r">
            <div className="mb-4 flex items-center justify-between gap-3">
              <div>
                <p className="text-xs font-semibold uppercase tracking-wide text-primary">
                  งานที่ต้องทำตอนนี้
                </p>
                <h2 className="mt-1 text-xl font-semibold tracking-tight">
                  ตรวจคิวเอกสารตามช่องทางให้จบในที่เดียว
                </h2>
              </div>
              <div className="rounded-full bg-primary/10 px-3 py-1 text-xs font-medium text-primary">
                Phase 1+
              </div>
            </div>
            <ActionCards stats={stats} loading={loading} />
          </div>
          <div className="grid grid-cols-2 gap-px bg-border/70">
            <DeskMetric
              label="บิลในระบบ"
              value={stats?.total_bills ?? 0}
              icon={FileText}
              loading={loading}
            />
            <DeskMetric
              label="ต้องจัดการ"
              value={awaitingReview}
              icon={AlertTriangle}
              tone="warning"
              loading={loading}
            />
            <DeskMetric
              label="ส่งแล้ว"
              value={stats?.sml_success ?? 0}
              icon={CheckCircle2}
              tone="success"
              loading={loading}
            />
            <DeskMetric
              label="อีเมลมีปัญหา"
              value={stats?.email_inbox_errors ?? 0}
              icon={Mail}
              tone="danger"
              loading={loading}
            />
          </div>
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[1fr_360px]">
        <Card className="rounded-2xl border-border/70 shadow-sm">
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-sm font-semibold">
              <Send className="h-4 w-4 text-primary" />
              เส้นทางงานเอกสาร
            </CardTitle>
          </CardHeader>
          <CardContent className="grid gap-3 sm:grid-cols-2">
            <FlowStep
              icon={FileText}
              title="Email บิลซื้อ Shopee"
              desc="กล่องอีเมลรับบิล → ใบสั่งซื้อ → ซื้อ -> ใบสั่งซื้อ"
            />
            {ENABLE_SHOPEE_EXCEL && ENABLE_SALES_ORDERS && (
              <FlowStep
                icon={ShoppingBag}
                title="Shopee Excel"
                desc="นำเข้าไฟล์จาก Seller Center → แยกตามปลายทางที่ตั้งไว้ → ใบสั่งขายหรือขายสินค้าและบริการ"
              />
            )}
          </CardContent>
        </Card>

        <div className="space-y-4">
          <InsightCard insight={insight} />
          {mapStats && <LearningProgress stats={mapStats} />}
        </div>
      </div>
    </div>
  )
}

function DeskMetric({
  label,
  value,
  icon: Icon,
  tone = 'primary',
  loading,
}: {
  label: string
  value: number
  icon: typeof FileText
  tone?: 'primary' | 'warning' | 'success' | 'danger'
  loading: boolean
}) {
  const toneCls = {
    primary: 'text-primary bg-primary/10',
    warning: 'text-warning bg-warning/10',
    success: 'text-success bg-success/10',
    danger: 'text-destructive bg-destructive/10',
  }[tone]
  return (
    <div className="bg-card p-5">
      <div className={`mb-4 flex h-9 w-9 items-center justify-center rounded-lg ${toneCls}`}>
        <Icon className="h-4 w-4" />
      </div>
      <p className="text-2xl font-semibold tabular-nums">{loading ? '—' : value.toLocaleString()}</p>
      <p className="mt-1 text-xs text-muted-foreground">{label}</p>
    </div>
  )
}

function FlowStep({
  icon: Icon,
  title,
  desc,
}: {
  icon: typeof FileText
  title: string
  desc: string
}) {
  return (
    <div className="rounded-xl border border-border/70 bg-muted/25 p-4">
      <div className="mb-3 flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10 text-primary">
        <Icon className="h-4 w-4" />
      </div>
      <p className="text-sm font-semibold">{title}</p>
      <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{desc}</p>
    </div>
  )
}
