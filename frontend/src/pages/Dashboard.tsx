import { useEffect, useState } from 'react'
import { AlertTriangle, CheckCircle2, FileText, Sparkles } from 'lucide-react'
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import StatsCard from '@/components/StatsCard'
import InsightCard from '@/components/InsightCard'
import LearningProgress from '@/components/LearningProgress'
import { PageHeader } from '@/components/common/PageHeader'
import { StatCardSkeleton } from '@/components/common/LoadingSkeleton'
import client from '@/api/client'
import { useAuthStore } from '@/store/auth'
import type { DailyInsight, DashboardStats, MappingStats } from '@/types'
import { BILL_STATUS_LABEL } from '@/lib/labels'
import { ActionCards } from './Dashboard/ActionCards'

export default function Dashboard() {
  const [stats, setStats] = useState<DashboardStats | null>(null)
  const [insight, setInsight] = useState<DailyInsight | null>(null)
  const [mapStats, setMapStats] = useState<MappingStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [generating, setGenerating] = useState(false)
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

  const chartData = stats
    ? [
        { name: BILL_STATUS_LABEL.needs_review, value: awaitingReview, fill: 'hsl(var(--warning))' },
        { name: BILL_STATUS_LABEL.sent, value: stats.sml_success, fill: 'hsl(var(--success))' },
        { name: BILL_STATUS_LABEL.failed, value: stats.sml_failed, fill: 'hsl(var(--destructive))' },
      ]
    : []

  return (
    <div className="space-y-6">
      <PageHeader
        title="Dashboard"
        description="ภาพรวมระบบ BillFlow ณ วันนี้"
        actions={
          user?.role === 'admin' && (
            <Button size="sm" onClick={handleGenerate} disabled={generating}>
              <Sparkles className="h-4 w-4" />
              {generating ? 'กำลังสร้าง…' : 'สร้าง AI Insight'}
            </Button>
          )
        }
      />

      {/* "ต้อง action" row — quick links to whatever is waiting on the admin
          today. Failed bills + email inbox errors get an urgent accent + a
          pulsing dot so they're hard to ignore. */}
      <ActionCards stats={stats} loading={loading} />

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {loading ? (
          <>
            <StatCardSkeleton />
            <StatCardSkeleton />
            <StatCardSkeleton />
            <StatCardSkeleton />
          </>
        ) : (
          <>
            <StatsCard
              label="บิลทั้งหมด"
              value={stats?.total_bills ?? 0}
              icon={FileText}
              variant="primary"
            />
            <StatsCard
              label={BILL_STATUS_LABEL.needs_review}
              value={awaitingReview}
              icon={AlertTriangle}
              variant="warning"
            />
            <StatsCard
              label={BILL_STATUS_LABEL.sent}
              value={stats?.sml_success ?? 0}
              icon={CheckCircle2}
              variant="success"
            />
            <StatsCard
              label={BILL_STATUS_LABEL.failed}
              value={stats?.sml_failed ?? 0}
              icon={AlertTriangle}
              variant="danger"
            />
          </>
        )}
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card className="lg:col-span-2">
          <CardHeader>
            <CardTitle className="text-sm font-semibold">สถานะบิลทั้งหมด</CardTitle>
          </CardHeader>
          <CardContent>
            <ResponsiveContainer width="100%" height={240}>
              <BarChart data={chartData} barSize={36}>
                <CartesianGrid
                  strokeDasharray="3 3"
                  stroke="hsl(var(--border))"
                  vertical={false}
                />
                <XAxis
                  dataKey="name"
                  tick={{ fontSize: 12, fill: 'hsl(var(--muted-foreground))' }}
                  axisLine={false}
                  tickLine={false}
                />
                <YAxis
                  tick={{ fontSize: 12, fill: 'hsl(var(--muted-foreground))' }}
                  axisLine={false}
                  tickLine={false}
                  allowDecimals={false}
                />
                <Tooltip
                  contentStyle={{
                    background: 'hsl(var(--popover))',
                    border: '1px solid hsl(var(--border))',
                    borderRadius: 8,
                    fontSize: 13,
                    color: 'hsl(var(--popover-foreground))',
                  }}
                  cursor={{ fill: 'hsl(var(--muted) / 0.5)' }}
                  labelStyle={{ color: 'hsl(var(--foreground))' }}
                />
                <Bar dataKey="value" radius={[4, 4, 0, 0]}>
                  {chartData.map((entry, index) => (
                    <Cell key={index} fill={entry.fill} />
                  ))}
                </Bar>
              </BarChart>
            </ResponsiveContainer>
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
