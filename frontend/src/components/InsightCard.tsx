import type { DailyInsight } from '../types'
import dayjs from 'dayjs'
import './InsightCard.css'

const SparkleIcon = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/>
  </svg>
)

export default function InsightCard({ insight }: { insight: DailyInsight | null }) {
  return (
    <div className="insight-card">
      <div className="insight-card-header">
        <span className="insight-card-label">
          <SparkleIcon />
          AI Insight ประจำวัน
        </span>
        {insight && (
          <span className="insight-card-date">{dayjs(insight.date).format('DD/MM/YYYY')}</span>
        )}
      </div>
      {insight
        ? <p className="insight-card-body">{insight.insight}</p>
        : <p className="insight-card-empty">ยังไม่มี insight วันนี้ กด "สร้าง AI Insight" เพื่อสร้าง</p>
      }
    </div>
  )
}
