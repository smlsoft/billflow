import { useEffect, useState } from 'react'
import client from '../api/client'
import type { DashboardStats, DailyInsight, MappingStats } from '../types'
import StatsCard from '../components/StatsCard'
import InsightCard from '../components/InsightCard'
import LearningProgress from '../components/LearningProgress'
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid, Cell } from 'recharts'
import { useAuthStore } from '../store/auth'
import toast from 'react-hot-toast'
import './Dashboard.css'

const BillsIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
    <polyline points="14,2 14,8 20,8"/>
    <line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/>
  </svg>
)

const ClockIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <circle cx="12" cy="12" r="10"/><polyline points="12,6 12,12 16,14"/>
  </svg>
)

const AlertTriIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
    <line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/>
  </svg>
)

const CheckIcon = () => (
  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/>
    <polyline points="22,4 12,14.01 9,11.01"/>
  </svg>
)

const SparkleIcon = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
  </svg>
)

export default function Dashboard() {
  const [stats, setStats] = useState<DashboardStats | null>(null)
  const [insight, setInsight] = useState<DailyInsight | null>(null)
  const [mapStats, setMapStats] = useState<MappingStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [generating, setGenerating] = useState(false)
  const user = useAuthStore((s) => s.user)

  const loadInsight = () =>
    client.get<{ data: DailyInsight[] }>('/api/dashboard/insights')
      .then((r) => setInsight(r.data.data?.[0] ?? null))
      .catch(() => null)

  useEffect(() => {
    Promise.all([
      client.get<DashboardStats>('/api/dashboard/stats').then((r) => setStats(r.data)).catch(() => null),
      loadInsight(),
      client.get<MappingStats>('/api/mappings/stats').then((r) => setMapStats(r.data)).catch(() => null),
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

  // pending + needs_review both mean "user must act" — combine in display.
  const awaitingReview = (stats?.pending ?? 0) + (stats?.needs_review ?? 0)

  const chartData = stats
    ? [
        { name: 'รอตรวจสอบ', value: awaitingReview,    fill: '#eab308' },
        { name: 'SML สำเร็จ', value: stats.sml_success,  fill: '#22c55e' },
        { name: 'SML ล้มเหลว',value: stats.sml_failed,   fill: '#ef4444' },
      ]
    : []

  return (
    <div>
      <div className="dashboard-header">
        <div>
          <h1 className="dashboard-title">Dashboard</h1>
          <p className="dashboard-subtitle">ภาพรวมระบบ BillFlow ณ วันนี้</p>
        </div>
        {user?.role === 'admin' && (
          <button className="btn btn-primary btn-sm" onClick={handleGenerate} disabled={generating}>
            <SparkleIcon />
            {generating ? 'กำลังสร้าง...' : 'สร้าง AI Insight'}
          </button>
        )}
      </div>

      {loading ? (
        <div className="dashboard-skeleton">
          <div className="skeleton dashboard-skeleton-card" />
          <div className="skeleton dashboard-skeleton-card" />
          <div className="skeleton dashboard-skeleton-card" />
          <div className="skeleton dashboard-skeleton-card" />
        </div>
      ) : (
        <div className="dashboard-stats-grid">
          <StatsCard
            label="บิลทั้งหมด"
            value={stats?.total_bills ?? 0}
            icon={<BillsIcon />}
            iconBg="#eef2ff"
            iconColor="#4f46e5"
          />
          <StatsCard
            label="รอตรวจสอบ"
            value={awaitingReview}
            icon={<AlertTriIcon />}
            iconBg="#fefce8"
            iconColor="#ca8a04"
          />
          <StatsCard
            label="SML สำเร็จ"
            value={stats?.sml_success ?? 0}
            icon={<CheckIcon />}
            iconBg="#f0fdf4"
            iconColor="#16a34a"
          />
          <StatsCard
            label="SML ล้มเหลว"
            value={stats?.sml_failed ?? 0}
            icon={<AlertTriIcon />}
            iconBg="#fef2f2"
            iconColor="#dc2626"
          />
        </div>
      )}

      <div className="dashboard-body">
        <div className="dashboard-chart-card">
          <h3 className="dashboard-chart-title">สถานะบิลทั้งหมด</h3>
          <ResponsiveContainer width="100%" height={240}>
            <BarChart data={chartData} barSize={36}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" vertical={false} />
              <XAxis dataKey="name" tick={{ fontSize: 12, fill: 'var(--color-text-muted)' }} axisLine={false} tickLine={false} />
              <YAxis tick={{ fontSize: 12, fill: 'var(--color-text-muted)' }} axisLine={false} tickLine={false} />
              <Tooltip
                contentStyle={{ background: 'var(--color-surface)', border: '1px solid var(--color-border)', borderRadius: 8, fontSize: 13 }}
                cursor={{ fill: 'var(--color-bg-hover)' }}
              />
              <Bar dataKey="value" radius={[4, 4, 0, 0]}>
                {chartData.map((entry, index) => (
                  <Cell key={index} fill={entry.fill} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>

        <div className="dashboard-sidebar">
          <InsightCard insight={insight} />
          {mapStats && <LearningProgress stats={mapStats} />}
        </div>
      </div>
    </div>
  )
}
