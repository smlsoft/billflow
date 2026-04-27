import './StatsCard.css'

interface Props {
  label: string
  value: string | number
  icon: React.ReactNode
  iconBg?: string
  iconColor?: string
}

export default function StatsCard({ label, value, icon, iconBg = '#eef2ff', iconColor = '#4f46e5' }: Props) {
  return (
    <div className="stats-card">
      <div className="stats-card-top">
        <p className="stats-card-label">{label}</p>
        <div
          className="stats-card-icon"
          style={{ '--icon-bg': iconBg, '--icon-color': iconColor } as React.CSSProperties}
        >
          {icon}
        </div>
      </div>
      <div className="stats-card-value">{value}</div>
    </div>
  )
}
