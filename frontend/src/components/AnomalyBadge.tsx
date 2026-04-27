import type { Anomaly } from '../types'

export default function AnomalyBadge({ anomaly }: { anomaly: Anomaly }) {
  const isError = anomaly.severity === 'error'
  return (
    <span
      title={anomaly.message}
      style={{
        display: 'inline-block',
        marginRight: 4,
        padding: '2px 8px',
        borderRadius: 12,
        fontSize: 10,
        background: isError ? '#ffebee' : '#fff8e1',
        color: isError ? '#b71c1c' : '#f57f17',
        cursor: 'help',
      }}
    >
      {isError ? '🚫' : '⚠️'} {anomaly.type.replace('_', ' ')}
    </span>
  )
}
