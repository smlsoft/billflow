import type { MappingStats } from '../types'
import type React from 'react'
import './LearningProgress.css'

const BrainIcon = () => (
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
    <path d="M9.5 2A2.5 2.5 0 0 1 12 4.5v15a2.5 2.5 0 0 1-4.96-.46 2.5 2.5 0 0 1-2.96-3.08 3 3 0 0 1-.34-5.58 2.5 2.5 0 0 1 1.32-4.24 2.5 2.5 0 0 1 1.98-3A2.5 2.5 0 0 1 9.5 2Z"/>
    <path d="M14.5 2A2.5 2.5 0 0 0 12 4.5v15a2.5 2.5 0 0 0 4.96-.46 2.5 2.5 0 0 0 2.96-3.08 3 3 0 0 0 .34-5.58 2.5 2.5 0 0 0-1.32-4.24 2.5 2.5 0 0 0-1.98-3A2.5 2.5 0 0 0 14.5 2Z"/>
  </svg>
)

export default function LearningProgress({ stats }: { stats: MappingStats }) {
  const total = stats.total || 1
  const autoConfirmedPct = Math.round((stats.auto_confirmed / total) * 100)
  const reviewPct = Math.round((stats.needs_review / total) * 100)

  return (
    <div className="learning-card">
      <h4 className="learning-card-title">
        <BrainIcon />
        F1 Learning Progress
      </h4>
      <div className="learning-row">
        <div className="learning-row-header">
          <span className="learning-row-label">Auto-confirmed</span>
          <span className="learning-row-value">{stats.auto_confirmed}/{stats.total} ({autoConfirmedPct}%)</span>
        </div>
        <div className="learning-bar-track">
          <div className="learning-bar-fill learning-bar-fill--success" style={{ '--bar-width': `${autoConfirmedPct}%` } as React.CSSProperties} />
        </div>
      </div>
      <div className="learning-row">
        <div className="learning-row-header">
          <span className="learning-row-label">รอตรวจสอบ</span>
          <span className="learning-row-value">{stats.needs_review} ({reviewPct}%)</span>
        </div>
        <div className="learning-bar-track">
          <div className="learning-bar-fill learning-bar-fill--warning" style={{ '--bar-width': `${reviewPct}%` } as React.CSSProperties} />
        </div>
      </div>
    </div>
  )
}
