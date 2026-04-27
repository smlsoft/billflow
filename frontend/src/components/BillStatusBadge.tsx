import type { BillStatus } from '../types'
import './BillStatusBadge.css'

const CONFIG: Record<string, { label: string }> = {
  pending:      { label: 'รอดำเนินการ' },
  needs_review: { label: 'รอตรวจสอบ' },
  sent:         { label: 'SML สำเร็จ' },
  failed:       { label: 'ล้มเหลว' },
  skipped:      { label: 'ข้ามแล้ว' },
}

export default function BillStatusBadge({ status }: { status: BillStatus | string }) {
  const cfg = CONFIG[status] ?? { label: status }
  return (
    <span className={`bill-status-badge bill-status-badge--${status}`}>
      <span className="bill-status-dot" />
      {cfg.label}
    </span>
  )
}
