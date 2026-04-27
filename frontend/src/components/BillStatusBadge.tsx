import type { BillStatus } from '../types'
import './BillStatusBadge.css'

const CONFIG: Record<string, { label: string }> = {
  pending:      { label: 'รอดำเนินการ' },
  processing:   { label: 'กำลังประมวลผล' },
  needs_review: { label: 'รอตรวจสอบ' },
  confirmed:    { label: 'ยืนยันแล้ว' },
  sent_to_sml:  { label: 'กำลังส่ง SML' },
  sml_success:  { label: 'SML สำเร็จ' },
  sml_failed:   { label: 'SML ล้มเหลว' },
  sent:         { label: 'SML สำเร็จ' },
  failed:       { label: 'ล้มเหลว' },
  error:        { label: 'ข้อผิดพลาด' },
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
